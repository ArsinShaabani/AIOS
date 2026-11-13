package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

// CreateReminder schedules a reminder.
func (s *Service) CreateReminder(ctx context.Context, userID, prompt string, at time.Time, agentID string) (string, error) {
	return s.create(ctx, "reminder", userID, map[string]any{
		"user_id": userID, "prompt": prompt, "agent_id": agentID, "deliver": "memory",
		"created_at": time.Now().UTC(),
	}, at)
}

// SchedulePrompt schedules a delayed model run.
func (s *Service) SchedulePrompt(ctx context.Context, userID, prompt string, at time.Time, agentID, deliver string) (string, error) {
	if deliver == "" {
		deliver = "memory"
	}
	return s.create(ctx, "automation.prompt", userID, map[string]any{
		"user_id": userID, "prompt": prompt, "agent_id": agentID, "deliver": deliver,
		"created_at": time.Now().UTC(),
	}, at)
}

// EnqueueTask is the generic entry point for integrations and webhooks.
func (s *Service) EnqueueTask(ctx context.Context, kind, userID string, payload map[string]any, at time.Time, priority int) (string, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["user_id"] = userID
	return s.createWith(ctx, kind, payload, at, priority)
}

func (s *Service) create(ctx context.Context, kind, userID string, payload map[string]any, at time.Time) (string, error) {
	if userID == "" {
		return "", platform.Unauthorized("schedule.user", "authentication required")
	}
	return s.createWith(ctx, kind, payload, at, 0)
}

func (s *Service) createWith(ctx context.Context, kind string, payload map[string]any, at time.Time, priority int) (string, error) {
	if at.Before(time.Now().Add(-time.Minute)) && kind != "memory.maintenance" {
		return "", platform.Invalid("schedule.past", "scheduled time must be in the future")
	}
	if at.After(time.Now().AddDate(1, 0, 0)) {
		return "", platform.Invalid("schedule.far", "scheduled time is more than a year away")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", platform.Wrap(platform.KindInvalid, "schedule.payload", "cannot encode payload", err)
	}
	task := &domain.Task{
		ID: platform.NewID("task"), Kind: kind, Payload: raw, Status: "pending",
		Priority: priority, MaxAttempts: 3, RunAt: at.UTC(), CreatedAt: time.Now().UTC(),
	}
	if err := s.tasks.Create(ctx, task); err != nil {
		return "", err
	}
	return task.ID, nil
}

// ListReminders returns pending reminders for a user.
func (s *Service) ListReminders(ctx context.Context, userID string) ([]tools.Reminder, error) {
	rows, _, err := s.tasks.List(ctx, "pending", platform.Page{Limit: 200})
	if err != nil {
		return nil, err
	}
	out := make([]tools.Reminder, 0, 8)
	for _, task := range rows {
		if task.Kind != "reminder" {
			continue
		}
		var payload struct {
			UserID string `json:"user_id"`
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(task.Payload, &payload) != nil || payload.UserID != userID {
			continue
		}
		out = append(out, tools.Reminder{ID: task.ID, Prompt: payload.Prompt, At: task.RunAt})
	}
	return out, nil
}

// Cancel marks a pending task done. privileged callers (admin/operator) may
// cancel any task; regular users only their own.
func (s *Service) Cancel(ctx context.Context, userID, taskID string, privileged bool) error {
	task, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if !privileged {
		var payload struct {
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal(task.Payload, &payload)
		if payload.UserID != userID {
			return platform.Forbidden("schedule.forbidden", "task belongs to another user")
		}
	}
	return s.tasks.Complete(ctx, taskID, time.Now().UTC())
}

// Tasks exposes the durable queue for the admin dashboard.
func (s *Service) Tasks(ctx context.Context, status string, page platform.Page) ([]domain.Task, int, error) {
	return s.tasks.List(ctx, status, page)
}

// Stats returns task counts by status.
func (s *Service) Stats(ctx context.Context) (map[string]int, error) {
	return s.tasks.CountByStatus(ctx)
}

var _ = ports.Job{}
