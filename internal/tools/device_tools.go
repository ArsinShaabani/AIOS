package tools

import (
	"context"
	"encoding/json"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// RegisterDeviceTools adds device_list, device_command and device_telemetry.
func RegisterDeviceTools(reg *Registry, api DeviceAPI) error {
	if api == nil {
		return nil
	}
	if err := reg.Register(ports.Tool{
		Name:        "device_list",
		Description: "List the user's registered devices with kind, room, status and capabilities.",
		Parameters:  Schema(map[string]any{"query": map[string]any{"type": "string", "description": "Optional filter"}}),
		Permissions: []string{"devices:read"},
		Timeout:     10,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			devices, err := api.List(ctx, call.UserID)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			query := stringArg(call.Arguments, "query")
			type brief struct {
				ID           string              `json:"id"`
				Name         string              `json:"name"`
				Kind         string              `json:"kind"`
				Room         string              `json:"room,omitempty"`
				Status       string              `json:"status"`
				Capabilities []domain.Capability `json:"capabilities"`
			}
			out := make([]brief, 0, len(devices))
			for _, d := range devices {
				if !containsFold(d.Name+" "+d.Room+" "+d.Hardware, query) {
					continue
				}
				out = append(out, brief{ID: d.ID, Name: d.Name, Kind: string(d.Kind), Room: d.Room, Status: d.Status, Capabilities: d.Capabilities})
			}
			raw, _ := json.Marshal(out)
			return ports.ToolResult{Content: string(raw), Data: map[string]any{"count": len(out)}}, nil
		},
	}); err != nil {
		return err
	}

	if err := reg.Register(ports.Tool{
		Name:        "device_command",
		Description: "Send a command to a device capability (e.g. set brightness, move a robot). Values are validated against declared safety limits.",
		Parameters: Schema(map[string]any{
			"device":     map[string]any{"type": "string", "description": "Device id or name"},
			"capability": map[string]any{"type": "string", "description": "Capability name, e.g. power or brightness"},
			"value":      map[string]any{"description": "New value (boolean, number, string)"},
		}, "device", "capability", "value"),
		Permissions: []string{"devices:write"},
		Timeout:     20,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			device := stringArg(call.Arguments, "device")
			capability := stringArg(call.Arguments, "capability")
			value, hasValue := call.Arguments["value"]
			if device == "" || capability == "" || !hasValue {
				return ports.ToolResult{IsError: true, Content: "device, capability and value are required"}, nil
			}
			result, err := api.Command(ctx, call.UserID, device, capability, value)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: "command failed: " + err.Error()}, nil
			}
			detail := result.Detail
			if detail == "" {
				detail = "ok"
			}
			return ports.ToolResult{Content: detail, Data: map[string]any{"ok": result.OK, "latency_ms": result.LatencyMS}}, nil
		},
	}); err != nil {
		return err
	}

	return reg.Register(ports.Tool{
		Name:        "device_telemetry",
		Description: "Read the latest cached telemetry (sensor values, battery, position) for a device.",
		Parameters: Schema(map[string]any{
			"device": map[string]any{"type": "string", "description": "Device id or name"},
		}, "device"),
		Permissions: []string{"devices:read"},
		Timeout:     10,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			device := stringArg(call.Arguments, "device")
			if device == "" {
				return ports.ToolResult{IsError: true, Content: "device is required"}, nil
			}
			state, err := api.Telemetry(ctx, call.UserID, device)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			if len(state) == 0 {
				return ports.ToolResult{Content: "no telemetry has been received for this device yet"}, nil
			}
			raw, _ := json.Marshal(state)
			return ports.ToolResult{Content: string(raw), Data: state}, nil
		},
	})
}
