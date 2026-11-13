package tools

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type fakeDevices struct {
	devices []domain.Device
	result  ports.CommandResult
	err     error
	command string
}

func (f *fakeDevices) List(context.Context, string) ([]domain.Device, error) { return f.devices, f.err }
func (f *fakeDevices) Resolve(_ context.Context, _, ref string) (*domain.Device, error) {
	for i := range f.devices {
		if f.devices[i].Name == ref {
			return &f.devices[i], nil
		}
	}
	return nil, platform.NotFound("device.missing", "missing")
}
func (f *fakeDevices) Command(_ context.Context, _, _, capability string, _ any) (ports.CommandResult, error) {
	f.command = capability
	if f.err != nil {
		return ports.CommandResult{}, f.err
	}
	return f.result, nil
}
func (f *fakeDevices) Telemetry(context.Context, string, string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return map[string]any{"temp": 21}, nil
}

type fakeMemory struct {
	items []domain.Memory
	saved string
}

func (f *fakeMemory) Search(context.Context, string, string) ([]domain.Memory, error) {
	return f.items, nil
}
func (f *fakeMemory) Save(_ context.Context, userID, content, kind string) (*domain.Memory, error) {
	f.saved = content
	return &domain.Memory{ID: "mem_1", UserID: userID, Content: content, Kind: domain.MemoryKind(kind)}, nil
}

type fakeScheduler struct {
	created  string
	reminder []Reminder
	err      error
}

func (f *fakeScheduler) CreateReminder(_ context.Context, _, prompt string, at time.Time, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.created = prompt
	f.reminder = append(f.reminder, Reminder{ID: "r1", Prompt: prompt, At: at})
	return "r1", nil
}
func (f *fakeScheduler) ListReminders(context.Context, string) ([]Reminder, error) {
	return f.reminder, f.err
}
func (f *fakeScheduler) SchedulePrompt(_ context.Context, _, _ string, at time.Time, _, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "a1", nil
}

type fakePublisher struct {
	topic   string
	payload string
	err     error
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload []byte, _ bool) error {
	f.topic, f.payload = topic, string(payload)
	return f.err
}

type fakeEmail struct{ sent string }

func (f *fakeEmail) Send(_ context.Context, to, subject, body string) error {
	f.sent = to + "|" + subject + "|" + body
	return nil
}
