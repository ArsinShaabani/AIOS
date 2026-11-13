package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// RegisterSchedulerTools adds reminder and automation tools.
func RegisterSchedulerTools(reg *Registry, api SchedulerAPI) error {
	if api == nil {
		return nil
	}
	if err := reg.Register(ports.Tool{
		Name:        "schedule_reminder",
		Description: "Create a reminder that notifies the user at a specific time. Provide `at` as RFC3339 or a duration like 10m.",
		Parameters: Schema(map[string]any{
			"text": map[string]any{"type": "string", "description": "What to remind about"},
			"at":   map[string]any{"type": "string", "description": "RFC3339 timestamp or duration (e.g. 15m, 2h)"},
		}, "text", "at"),
		Permissions: []string{"chat"},
		Timeout:     10,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			text := stringArg(call.Arguments, "text")
			atRaw := stringArg(call.Arguments, "at")
			if text == "" || atRaw == "" {
				return ports.ToolResult{IsError: true, Content: "text and at are required"}, nil
			}
			at, err := ParseWhen(atRaw)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			id, err := api.CreateReminder(ctx, call.UserID, text, at, call.AgentID)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			return ports.ToolResult{
				Content: "reminder scheduled for " + at.Format(time.RFC3339) + " (id " + id + ")",
				Data:    map[string]any{"id": id, "at": at},
			}, nil
		},
	}); err != nil {
		return err
	}
	if err := reg.Register(ports.Tool{
		Name:        "list_reminders",
		Description: "List pending reminders for the user.",
		Parameters:  Schema(map[string]any{}),
		Permissions: []string{"chat"},
		Timeout:     10,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			items, err := api.ListReminders(ctx, call.UserID)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			raw, _ := json.Marshal(items)
			return ports.ToolResult{Content: string(raw), Data: map[string]any{"count": len(items)}}, nil
		},
	}); err != nil {
		return err
	}
	return reg.Register(ports.Tool{
		Name:        "schedule_prompt",
		Description: "Schedule a prompt to run automatically later (an automation), e.g. a daily briefing.",
		Parameters: Schema(map[string]any{
			"prompt":  map[string]any{"type": "string", "description": "Instruction to run at that time"},
			"when":    map[string]any{"type": "string", "description": "RFC3339 timestamp or duration (e.g. 1h)"},
			"deliver": map[string]any{"type": "string", "enum": []string{"memory", "mqtt"}, "default": "memory"},
		}, "prompt", "when"),
		Permissions: []string{"chat"},
		Timeout:     10,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			prompt := stringArg(call.Arguments, "prompt")
			whenRaw := stringArg(call.Arguments, "when")
			if prompt == "" || whenRaw == "" {
				return ports.ToolResult{IsError: true, Content: "prompt and when are required"}, nil
			}
			at, err := ParseWhen(whenRaw)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			deliver := stringArg(call.Arguments, "deliver")
			if deliver == "" {
				deliver = "memory"
			}
			id, err := api.SchedulePrompt(ctx, call.UserID, prompt, at, call.AgentID, deliver)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			return ports.ToolResult{Content: "automation scheduled (id " + id + ")", Data: map[string]any{"id": id, "at": at}}, nil
		},
	})
}
