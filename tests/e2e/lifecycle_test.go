package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestAppLifecycleAndJobs(t *testing.T) {
	e := newEnv(t)
	_, me := e.do("GET", "/api/v1/auth/me", e.token, nil)
	uid := me["user"].(map[string]any)["id"].(string)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.app.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	enqueue := func(kind string, payload map[string]any) {
		raw, _ := json.Marshal(payload)
		if err := e.app.Queue.Enqueue(ctx, ports.Job{ID: "job-" + kind, Kind: kind, Payload: raw}, 0); err != nil {
			t.Fatalf("enqueue %s: %v", kind, err)
		}
	}
	enqueue("memory.extract", map[string]any{
		"user_id": uid, "session_id": "ses_test", "user_text": "My name is Grace and I love sailing",
	})
	enqueue("reminder", map[string]any{"user_id": uid, "prompt": "drink water", "task_id": "t1"})
	enqueue("automation.prompt", map[string]any{"user_id": uid, "prompt": "say hello", "deliver": "memory"})
	enqueue("memory.maintenance", map[string]any{})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, list := e.do("GET", "/api/v1/memories?limit=50", e.token, nil)
		if total, ok := list["total"].(float64); ok && total >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_, list := e.do("GET", "/api/v1/memories?limit=50", e.token, nil)
	if total, _ := list["total"].(float64); total < 2 {
		t.Fatalf("jobs did not persist memories: %v", list)
	}
	if stats, err := e.app.Queue.Stats(ctx); err != nil || stats.Processed == 0 {
		t.Fatalf("queue stats: %v %+v", err, stats)
	}
	// Scheduler polling must have started without errors; stopping must be clean.
	cancel()
	e.app.Close(context.Background())
}
