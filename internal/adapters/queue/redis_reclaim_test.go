package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestRedisQueueReclaimsStaleInFlight(t *testing.T) {
	mr := miniredis.RunT(t)
	q, err := NewRedis(config.QueueConfig{Workers: 1, MaxAttempts: 2, PollInterval: config.Duration(20 * time.Millisecond),
		VisibilityTimeout: config.Duration(100 * time.Millisecond)}, config.RedisConfig{Addr: mr.Addr()}, "reclaim", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	q.Register("stale", func(ctx context.Context, job ports.Job) error {
		done <- struct{}{}
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	raw, _ := json.Marshal(ports.Job{ID: "stale-1", Kind: "stale", Payload: []byte(`{}`)})
	// Simulate a worker that popped the job and died before finishing.
	if _, err := mr.Lpush(q.processingKey(), string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := mr.ZAdd(q.inflightKey(), float64(time.Now().Add(-time.Minute).UnixMilli()), string(raw)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stale in-flight job was not reclaimed")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if q.reclaimed.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if q.reclaimed.Load() < 1 {
		t.Fatal("reclaim counter not incremented")
	}
	q.Stop(context.Background())
}
