package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// HomeAssistant is a minimal REST client for Home Assistant.
type HomeAssistant struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewHomeAssistant builds the client from configuration.
func NewHomeAssistant(cfg config.HomeAssistantConfig) *HomeAssistant {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HomeAssistant{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
		client:  httpx.NewClient(timeout),
	}
}

func (h *HomeAssistant) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + h.token}
}

// Call performs one of list_states / get_state / call_service.
func (h *HomeAssistant) Call(ctx context.Context, action, entityID, service string, data map[string]any) (map[string]any, error) {
	if h.baseURL == "" || h.token == "" {
		return nil, platform.Unavailable("homeassistant.config", "Home Assistant is not configured (base URL and token are required)")
	}
	switch action {
	case "", "get_state":
		if entityID == "" {
			return nil, platform.Invalid("homeassistant.entity", "entity_id is required")
		}
		var out map[string]any
		if err := httpx.DoJSON(ctx, h.client, http.MethodGet, h.baseURL+"/api/states/"+entityID, h.headers(), nil, &out, 1<<20); err != nil {
			return nil, platform.Wrap(platform.KindUnavailable, "homeassistant.state", "cannot read entity state", err)
		}
		return out, nil
	case "list_states":
		var out []map[string]any
		if err := httpx.DoJSON(ctx, h.client, http.MethodGet, h.baseURL+"/api/states", h.headers(), nil, &out, 4<<20); err != nil {
			return nil, platform.Wrap(platform.KindUnavailable, "homeassistant.states", "cannot list states", err)
		}
		// Trim attributes to keep the model context small.
		trimmed := make([]map[string]any, 0, len(out))
		limit := min(len(out), 100)
		for _, entity := range out[:limit] {
			trimmed = append(trimmed, map[string]any{
				"entity_id":   entity["entity_id"],
				"state":       entity["state"],
				"name":        nested(entity, "attributes", "friendly_name"),
				"unit":        nested(entity, "attributes", "unit_of_measurement"),
				"last_change": entity["last_changed"],
			})
		}
		return map[string]any{"count": len(out), "entities": trimmed}, nil
	case "call_service":
		if service == "" {
			return nil, platform.Invalid("homeassistant.service", "service is required, e.g. light.turn_on")
		}
		parts := strings.SplitN(service, ".", 2)
		if len(parts) != 2 {
			return nil, platform.Invalid("homeassistant.service", "service must look like domain.action")
		}
		body := map[string]any{}
		for k, v := range data {
			body[k] = v
		}
		if entityID != "" {
			body["entity_id"] = entityID
		}
		var out []map[string]any
		endpoint := h.baseURL + "/api/services/" + parts[0] + "/" + parts[1]
		if err := httpx.DoJSON(ctx, h.client, http.MethodPost, endpoint, h.headers(), body, &out, 2<<20); err != nil {
			return nil, platform.Wrap(platform.KindUnavailable, "homeassistant.call", "service call failed", err)
		}
		return map[string]any{"ok": true, "changed": len(out)}, nil
	default:
		return nil, platform.Invalid("homeassistant.action", "unknown action: "+action)
	}
}

func nested(m map[string]any, keys ...string) any {
	var current any = m
	for _, k := range keys {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[k]
	}
	return current
}

// RegisterHomeAssistantTool exposes the smart-home integration.
func RegisterHomeAssistantTool(reg *Registry, api HomeAssistantAPI) error {
	if api == nil {
		return nil
	}
	return reg.Register(ports.Tool{
		Name:        "home_assistant",
		Description: "Read or control Home Assistant entities: list_states, get_state, call_service (e.g. light.turn_on).",
		Parameters: Schema(map[string]any{
			"action":    map[string]any{"type": "string", "enum": []string{"list_states", "get_state", "call_service"}, "default": "get_state"},
			"entity_id": map[string]any{"type": "string", "description": "Entity id, e.g. light.kitchen"},
			"service":   map[string]any{"type": "string", "description": "Domain.action, e.g. light.turn_on"},
			"data":      map[string]any{"type": "object", "description": "Extra service data, e.g. {brightness: 128}"},
		}, "action"),
		Permissions: []string{"devices:write"},
		Timeout:     20,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			action := stringArg(call.Arguments, "action")
			if action == "" {
				action = "get_state"
			}
			data := map[string]any{}
			if raw, ok := call.Arguments["data"].(map[string]any); ok {
				data = raw
			}
			result, err := api.Call(ctx, action, stringArg(call.Arguments, "entity_id"), stringArg(call.Arguments, "service"), data)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			raw, _ := json.Marshal(result)
			return ports.ToolResult{Content: string(raw), Data: result}, nil
		},
	})
}
