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

func TestRedisQueue(t *testing.T) {
	mr := miniredis.RunT(t)
	q, err := NewRedis(config.QueueConfig{Workers: 1, MaxAttempts: 2, PollInterval: config.Duration(50 * time.Millisecond)},
		config.RedisConfig{Addr: mr.Addr()}, "test", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 4)
	q.Register("echo", func(ctx context.Context, job ports.Job) error {
		var payload struct{ V string }
		_ = json.Unmarshal(job.Payload, &payload)
		done <- payload.V
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	raw, _ := json.Marshal(map[string]string{"V": "one"})
	if err := q.Enqueue(ctx, ports.Job{ID: "1", Kind: "echo", Payload: raw}, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-done:
		if v != "one" {
			t.Fatalf("payload %q", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("job not processed")
	}
	raw2, _ := json.Marshal(map[string]string{"V": "two"})
	if err := q.Enqueue(ctx, ports.Job{ID: "2", Kind: "echo", Payload: raw2}, 150*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-done:
		if v != "two" {
			t.Fatalf("delayed payload %q", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delayed job not promoted")
	}
	var stats ports.QueueStats
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stats, err = q.Stats(ctx)
		if err == nil && stats.Processed >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || stats.Processed < 2 {
		t.Fatalf("stats: %v %+v", err, stats)
	}
	q.Stop(context.Background())
	if _, err := NewRedis(config.QueueConfig{}, config.RedisConfig{Addr: "127.0.0.1:1"}, "x", nil); err == nil {
		t.Fatal("unreachable redis accepted")
	}
}

func TestRedisQueueRecoversOrphans(t *testing.T) {
	mr := miniredis.RunT(t)
	q, err := NewRedis(config.QueueConfig{Workers: 1, MaxAttempts: 2, PollInterval: config.Duration(30 * time.Millisecond)},
		config.RedisConfig{Addr: mr.Addr()}, "recover", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	q.Register("once", func(ctx context.Context, job ports.Job) error {
		done <- struct{}{}
		return nil
	})
	// Simulate a job left in the processing list by a previous crash.
	raw, _ := json.Marshal(ports.Job{ID: "orphan", Kind: "once", Payload: []byte(`{}`)})
	if _, err := mr.Lpush(q.processingKey(), string(raw)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("orphaned job was not recovered")
	}
	q.Stop(context.Background())
}
