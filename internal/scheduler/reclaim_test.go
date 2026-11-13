package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestStartReclaimsStaleTask(t *testing.T) {
	svc, repos := newService(t)
	ctx := context.Background()
	task := &domain.Task{ID: "stale-task", Kind: "reminder", Payload: []byte(`{"user_id":"u1"}`),
		RunAt: time.Now().Add(-time.Hour), MaxAttempts: 2}
	if err := repos.Tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Tasks.ClaimDue(ctx, time.Now(), 5); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.DB.ExecContext(ctx, repos.DB.Rebind(`UPDATE tasks SET started_at = ? WHERE id = ?`),
		time.Now().Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000000000Z"), "stale-task"); err != nil {
		t.Fatal(err)
	}
	svc.cfg.StaleAfter = config.Duration(time.Minute)
	svc.Start(ctx)
	defer svc.Stop(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := repos.Tasks.Get(ctx, "stale-task")
		if err == nil && got.Status != "running" {
			if got.Status != "pending" && got.Status != "done" {
				t.Fatalf("unexpected reclaimed status: %s", got.Status)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("stale running task was not reclaimed")
}
