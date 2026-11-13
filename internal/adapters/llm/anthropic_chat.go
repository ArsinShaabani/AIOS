package llm

import (
	"context"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Chat performs a blocking completion.
func (a *Anthropic) Chat(ctx context.Context, req ports.ChatRequest) (*ports.ChatResponse, error) {
	body := a.build(req)
	var out anthropicResponse
	if err := httpx.DoJSON(ctx, a.client, http.MethodPost, a.baseURL+"/v1/messages", a.headers, body, &out, 8<<20); err != nil {
		return nil, providerErr(a.name, err)
	}
	if out.Error != nil {
		return nil, providerErr(a.name, &httpx.StatusError{StatusCode: http.StatusBadRequest, Body: out.Error.Message, URL: a.baseURL})
	}
	resp := &ports.ChatResponse{
		Provider:     a.name,
		Model:        body.Model,
		FinishReason: normaliseFinish(out.StopReason),
		Usage: ports.Usage{
			PromptTokens:     out.Usage.InputTokens,
			CompletionTokens: out.Usage.OutputTokens,
			TotalTokens:      out.Usage.InputTokens + out.Usage.OutputTokens,
		},
	}
	var text strings.Builder
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			resp.ToolCalls = append(resp.ToolCalls, domain.ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input})
		}
	}
	resp.Content = text.String()
	if len(resp.ToolCalls) > 0 {
		resp.FinishReason = "tool_calls"
	}
	return resp, nil
}

// Embed is not offered by Anthropic; the router falls back to another provider.
func (a *Anthropic) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, ports.ErrNotSupported
}

// Ping verifies connectivity via the models endpoint.
func (a *Anthropic) Ping(ctx context.Context) error {
	return providerErr(a.name, httpx.DoJSON(ctx, a.client, http.MethodGet, a.baseURL+"/v1/models", a.headers, nil, nil, 1<<20))
}
