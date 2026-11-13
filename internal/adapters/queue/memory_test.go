package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestMemoryQueueProcessesJobs(t *testing.T) {
	q := NewMemory(2, 2, 8, slog.Default())
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
	if err := q.Enqueue(ctx, ports.Job{ID: "1", Kind: "echo", Payload: []byte(`{"V":"a"}`)}, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-done:
		if v != "a" {
			t.Fatalf("payload=%q", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job was not processed")
	}
	stats, err := q.Stats(ctx)
	if err != nil || stats.Processed < 1 {
		t.Fatalf("stats: %v %+v", err, stats)
	}
	// Unknown kinds fail without blocking the queue.
	if err := q.Enqueue(ctx, ports.Job{ID: "2", Kind: "nope", Payload: []byte(`{}`)}, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	q.Stop(context.Background())
	if err := q.Enqueue(ctx, ports.Job{ID: "3", Kind: "echo"}, 0); err == nil {
		t.Fatal("enqueue after stop should fail")
	}
}
