package tools

import (
	"context"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// DeviceAPI is the narrow device surface the tools need (implemented by the
// devices service). Keeping it here avoids a package cycle.
type DeviceAPI interface {
	List(ctx context.Context, userID string) ([]domain.Device, error)
	Resolve(ctx context.Context, userID, ref string) (*domain.Device, error)
	Command(ctx context.Context, userID, ref, capability string, value any) (ports.CommandResult, error)
	Telemetry(ctx context.Context, userID, ref string) (map[string]any, error)
}

// MemoryAPI is the narrow memory surface.
type MemoryAPI interface {
	Search(ctx context.Context, userID, query string) ([]domain.Memory, error)
	Save(ctx context.Context, userID, content, kind string) (*domain.Memory, error)
}

// Reminder is a scheduled reminder view.
type Reminder struct {
	ID     string    `json:"id"`
	Prompt string    `json:"prompt"`
	At     time.Time `json:"at"`
}

// SchedulerAPI creates and lists reminders/automations.
type SchedulerAPI interface {
	CreateReminder(ctx context.Context, userID, prompt string, at time.Time, agentID string) (string, error)
	ListReminders(ctx context.Context, userID string) ([]Reminder, error)
	SchedulePrompt(ctx context.Context, userID, prompt string, at time.Time, agentID, deliver string) (string, error)
}

// MQTTPublisher publishes arbitrary MQTT messages (IoT escape hatch).
type MQTTPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte, retain bool) error
}

// HomeAssistantAPI proxies calls to Home Assistant.
type HomeAssistantAPI interface {
	Call(ctx context.Context, action, entityID, service string, data map[string]any) (map[string]any, error)
}

// EmailSender sends transactional email.
type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// ParseWhen accepts RFC3339, a Go duration ("15m"), or "2006-01-02 15:04".
func ParseWhen(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, platform.Invalid("when.empty", "time is required")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		if t.Before(time.Now().Add(-time.Minute)) {
			return time.Time{}, platform.Invalid("when.past", "the requested time is in the past")
		}
		return t.UTC(), nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d <= 0 {
			return time.Time{}, platform.Invalid("when.past", "duration must be positive")
		}
		return time.Now().Add(d).UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", raw, time.UTC); err == nil {
		if t.Before(time.Now().Add(-time.Minute)) {
			return time.Time{}, platform.Invalid("when.past", "the requested time is in the past")
		}
		return t.UTC(), nil
	}
	return time.Time{}, platform.Invalid("when.format", "use RFC3339 (2026-01-31T09:00:00Z) or a duration like 15m")
}

func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
