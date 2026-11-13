package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestDeviceMemorySchedulerTools(t *testing.T) {
	devices := &fakeDevices{
		devices: []domain.Device{{ID: "dev1", Name: "Lamp", Kind: domain.DeviceLight, Status: "online",
			Capabilities: []domain.Capability{{Name: "power", Type: "switch"}}}},
		result: ports.CommandResult{OK: true, Detail: "ack", LatencyMS: 5},
	}
	memory := &fakeMemory{items: []domain.Memory{{ID: "m1", Kind: domain.KindFact, Content: "likes tea", Importance: 0.9}}}
	sch := &fakeScheduler{}
	reg := NewRegistry()
	if err := RegisterDeviceTools(reg, devices); err != nil {
		t.Fatal(err)
	}
	if err := RegisterMemoryTools(reg, memory); err != nil {
		t.Fatal(err)
	}
	if err := RegisterSchedulerTools(reg, sch); err != nil {
		t.Fatal(err)
	}

	res, _ := reg.Execute(context.Background(), "device_list", ports.ToolCallContext{UserID: "u1", Arguments: map[string]any{"query": "Lamp"}})
	if res.IsError || !strings.Contains(res.Content, "Lamp") {
		t.Fatalf("device list: %+v", res)
	}
	res, _ = reg.Execute(context.Background(), "device_command", ports.ToolCallContext{UserID: "u1", Arguments: map[string]any{
		"device": "Lamp", "capability": "power", "value": true,
	}})
	if res.IsError || devices.command != "power" {
		t.Fatalf("device command: %+v", res)
	}
	if res, _ := reg.Execute(context.Background(), "device_command", ports.ToolCallContext{}); !res.IsError {
		t.Fatal("missing args must fail")
	}
	devices.err = platform.Unavailable("dev.down", "down")
	if res, _ := reg.Execute(context.Background(), "device_command", ports.ToolCallContext{Arguments: map[string]any{"device": "x", "capability": "y", "value": 1}}); !res.IsError {
		t.Fatal("device error must surface")
	}
	devices.err = nil
	res, _ = reg.Execute(context.Background(), "device_telemetry", ports.ToolCallContext{Arguments: map[string]any{"device": "Lamp"}})
	if res.IsError || !strings.Contains(res.Content, "temp") {
		t.Fatalf("telemetry: %+v", res)
	}
	devices.err = errors.New("boom")
	if res, _ := reg.Execute(context.Background(), "device_telemetry", ports.ToolCallContext{Arguments: map[string]any{"device": "Lamp"}}); !res.IsError {
		t.Fatal("telemetry error must surface")
	}

	res, _ = reg.Execute(context.Background(), "memory_search", ports.ToolCallContext{Arguments: map[string]any{"query": "tea"}})
	if res.IsError || !strings.Contains(res.Content, "likes tea") {
		t.Fatalf("memory search: %+v", res)
	}
	res, _ = reg.Execute(context.Background(), "memory_save", ports.ToolCallContext{Arguments: map[string]any{"content": "likes coffee", "kind": "preference"}})
	if res.IsError || memory.saved != "likes coffee" {
		t.Fatalf("memory save: %+v", res)
	}
	if res, _ := reg.Execute(context.Background(), "memory_save", ports.ToolCallContext{}); !res.IsError {
		t.Fatal("empty memory must fail")
	}

	res, _ = reg.Execute(context.Background(), "schedule_reminder", ports.ToolCallContext{Arguments: map[string]any{"text": "tea", "at": "10m"}})
	if res.IsError || sch.created != "tea" {
		t.Fatalf("reminder: %+v", res)
	}
	if res, _ := reg.Execute(context.Background(), "schedule_reminder", ports.ToolCallContext{Arguments: map[string]any{"text": "tea", "at": "nonsense"}}); !res.IsError {
		t.Fatal("bad time must fail")
	}
	res, _ = reg.Execute(context.Background(), "list_reminders", ports.ToolCallContext{})
	if res.IsError || !strings.Contains(res.Content, "tea") {
		t.Fatalf("list reminders: %+v", res)
	}
	res, _ = reg.Execute(context.Background(), "schedule_prompt", ports.ToolCallContext{Arguments: map[string]any{"prompt": "brief me", "when": "1h"}})
	if res.IsError || !strings.Contains(res.Content, "a1") {
		t.Fatalf("schedule prompt: %+v", res)
	}
	if res, _ := reg.Execute(context.Background(), "device_telemetry", ports.ToolCallContext{}); !res.IsError {
		t.Fatal("missing device arg must fail")
	}
}
