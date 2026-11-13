package tools

import (
	"context"
	"encoding/json"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// RegisterMemoryTools adds memory_search and memory_save.
func RegisterMemoryTools(reg *Registry, api MemoryAPI) error {
	if api == nil {
		return nil
	}
	if err := reg.Register(ports.Tool{
		Name:        "memory_search",
		Description: "Search the user's long-term memory (preferences, facts, past notes).",
		Parameters: Schema(map[string]any{
			"query": map[string]any{"type": "string", "description": "What to look for"},
		}, "query"),
		Permissions: []string{"memory:read"},
		Timeout:     15,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			query := stringArg(call.Arguments, "query")
			if query == "" {
				return ports.ToolResult{IsError: true, Content: "query is required"}, nil
			}
			memories, err := api.Search(ctx, call.UserID, query)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			type item struct {
				ID         string  `json:"id"`
				Kind       string  `json:"kind"`
				Text       string  `json:"text"`
				Importance float64 `json:"importance"`
			}
			out := make([]item, 0, len(memories))
			for _, m := range memories {
				out = append(out, item{ID: m.ID, Kind: string(m.Kind), Text: m.Content, Importance: m.Importance})
			}
			raw, _ := json.Marshal(out)
			return ports.ToolResult{Content: string(raw), Data: map[string]any{"count": len(out)}}, nil
		},
	}); err != nil {
		return err
	}
	return reg.Register(ports.Tool{
		Name:        "memory_save",
		Description: "Persist a durable fact or preference about the user. Use only for information worth remembering later.",
		Parameters: Schema(map[string]any{
			"content": map[string]any{"type": "string", "description": "The fact or preference"},
			"kind":    map[string]any{"type": "string", "enum": []string{"fact", "preference", "note"}, "default": "fact"},
		}, "content"),
		Permissions: []string{"memory:write"},
		Timeout:     15,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			content := stringArg(call.Arguments, "content")
			if content == "" {
				return ports.ToolResult{IsError: true, Content: "content is required"}, nil
			}
			m, err := api.Save(ctx, call.UserID, content, stringArg(call.Arguments, "kind"))
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			return ports.ToolResult{Content: "saved memory " + m.ID, Data: map[string]any{"id": m.ID}}, nil
		},
	})
}
