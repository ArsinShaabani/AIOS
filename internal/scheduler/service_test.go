package scheduler

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/queue"
	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func newService(t *testing.T) (*Service, *sqlstore.Repositories) {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "sched.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })
	q := queue.NewMemory(1, 2, 8, slog.Default())
	return New(repos.Tasks, q, config.SchedulerConfig{PollInterval: config.Duration(10 * time.Millisecond), MaxConcurrent: 2}, slog.Default()), repos
}

func TestCreateListCancelReminder(t *testing.T) {
	svc, repos := newService(t)
	ctx := context.Background()
	at := time.Now().Add(time.Hour).UTC()
	id, err := svc.CreateReminder(ctx, "u1", "call mum", at, "agent_scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("no id")
	}
	items, err := svc.ListReminders(ctx, "u1")
	if err != nil || len(items) != 1 || items[0].Prompt != "call mum" {
		t.Fatalf("list reminders: %v %+v", err, items)
	}
	if other, _ := svc.ListReminders(ctx, "u2"); len(other) != 0 {
		t.Fatal("reminders leaked across users")
	}
	if err := svc.Cancel(ctx, "u2", id, false); !platform.IsForbidden(err) {
		t.Fatalf("cancel by another user must be forbidden: %v", err)
	}
	if err := svc.Cancel(ctx, "u2", id, true); err != nil {
		t.Fatalf("privileged cancel failed: %v", err)
	}
	task, _ := repos.Tasks.Get(ctx, id)
	if task.Status != "done" {
		t.Fatalf("task status=%s", task.Status)
	}
	if _, err := svc.CreateReminder(ctx, "u1", "past", time.Now().Add(-time.Hour), ""); err == nil {
		t.Fatal("past reminder accepted")
	}
	if _, err := svc.CreateReminder(ctx, "", "anonymous", at, ""); err == nil {
		t.Fatal("reminder without user accepted")
	}
}

func TestSchedulePromptAndStats(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id, err := svc.SchedulePrompt(ctx, "u1", "summarise the day", time.Now().Add(30*time.Minute), "agent_automation", "mqtt")
	if err != nil || id == "" {
		t.Fatalf("schedule prompt: %v", err)
	}
	stats, err := svc.Stats(ctx)
	if err != nil || stats["pending"] < 1 {
		t.Fatalf("stats: %v %+v", err, stats)
	}
	tasks, total, err := svc.Tasks(ctx, "pending", platform.Page{Limit: 10})
	if err != nil || total < 1 || len(tasks) == 0 || tasks[0].Kind != "automation.prompt" {
		t.Fatalf("tasks: %v total=%d %+v", err, total, tasks)
	}
}

func TestDispatchDue(t *testing.T) {
	svc, repos := newService(t)
	ctx := context.Background()
	// Insert an already-due task directly.
	if err := repos.Tasks.Create(ctx, &domain.Task{
		ID: "t-due", Kind: "reminder", Payload: []byte(`{"user_id":"u1","prompt":"now"}`),
		RunAt: time.Now().Add(-time.Minute), MaxAttempts: 3,
	}); err != nil {
		t.Fatal(err)
	}
	svc.dispatchDue(ctx)
	task, err := repos.Tasks.Get(ctx, "t-due")
	if err != nil || task.Status != "done" {
		t.Fatalf("due task not dispatched: %v %+v", err, task)
	}
}
