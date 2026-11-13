package sqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestReclaimStaleRunningTask(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	now := time.Now().UTC()
	task := &domain.Task{ID: "stale", Kind: "reminder", Payload: []byte(`{}`),
		RunAt: now.Add(-time.Hour), MaxAttempts: 3}
	if err := r.Tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Tasks.ClaimDue(ctx, now, 10); err != nil {
		t.Fatal(err)
	}
	// Simulate a worker crash an hour ago.
	if _, err := r.DB.ExecContext(ctx, r.DB.Rebind(`UPDATE tasks SET started_at = ? WHERE id = ?`),
		ft(now.Add(-time.Hour)), "stale"); err != nil {
		t.Fatal(err)
	}
	n, err := r.Tasks.ReclaimStale(ctx, now.Add(-15*time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("reclaim: %v %d", err, n)
	}
	got, _ := r.Tasks.Get(ctx, "stale")
	if got.Status != "pending" || got.Attempts != 1 {
		t.Fatalf("reclaimed task: %+v", got)
	}
	if n, err := r.Tasks.ReclaimStale(ctx, now); err != nil || n != 0 {
		t.Fatalf("second reclaim should be a no-op: %v %d", err, n)
	}
}
