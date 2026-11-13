package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// providerErr maps transport/HTTP failures onto typed, retry-aware errors.
func providerErr(provider string, err error) error {
	if err == nil {
		return nil
	}
	var se *httpx.StatusError
	if errors.As(err, &se) {
		switch {
		case se.StatusCode == http.StatusUnauthorized:
			return platform.Wrap(platform.KindUnauthorized, provider+".unauthorized", "provider rejected the API key", err)
		case se.StatusCode == http.StatusForbidden:
			return platform.Wrap(platform.KindForbidden, provider+".forbidden", "provider denied access", err)
		case se.StatusCode == http.StatusTooManyRequests, se.StatusCode >= 500,
			se.StatusCode == http.StatusRequestTimeout, se.StatusCode == http.StatusConflict:
			return platform.Wrap(platform.KindUnavailable, provider+".unavailable", "provider temporarily unavailable", err)
		case se.StatusCode >= 400 && se.StatusCode < 500:
			return platform.Wrap(platform.KindInvalid, provider+".invalid_request", "provider rejected the request", err)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return platform.Wrap(platform.KindTimeout, provider+".timeout", "provider timed out", err)
	}
	if errors.Is(err, context.Canceled) {
		return platform.Wrap(platform.KindTimeout, provider+".cancelled", "request cancelled", err)
	}
	return platform.Wrap(platform.KindUnavailable, provider+".network", "provider unreachable", err)
}

// normaliseFinish maps vendor finish reasons to a small stable set.
func normaliseFinish(reason string) string {
	switch strings.ToLower(reason) {
	case "stop", "end_turn", "stop_sequence":
		return "stop"
	case "length", "max_tokens":
		return "length"
	case "tool_calls", "tool_use", "function_call":
		return "tool_calls"
	case "content_filter", "safety":
		return "content_filter"
	case "":
		return "stop"
	default:
		return strings.ToLower(reason)
	}
}

// toProviderTools converts neutral tool specs into OpenAI-style definitions.
func toOpenAITools(specs []ports.ToolSpec) []map[string]any {
	if len(specs) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(specs))
	for _, s := range specs {
		params := s.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

// toolCallsToDomain parses OpenAI tool call arguments.
func toolCallsToDomain(calls []oaToolCall) []domain.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]domain.ToolCall, 0, len(calls))
	for _, c := range calls {
		args := map[string]any{}
		if strings.TrimSpace(c.Function.Arguments) != "" {
			_ = json.Unmarshal([]byte(c.Function.Arguments), &args)
		}
		out = append(out, domain.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: args})
	}
	return out
}

type oaToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function oaToolFunc `json:"function"`
}

type oaToolFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    any          `json:"content,omitempty"`
	Name       string       `json:"name,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaRequest struct {
	Model          string           `json:"model"`
	Messages       []oaMessage      `json:"messages"`
	Temperature    float64          `json:"temperature,omitempty"`
	TopP           float64          `json:"top_p,omitempty"`
	MaxTokens      int              `json:"max_tokens,omitempty"`
	Tools          []map[string]any `json:"tools,omitempty"`
	ToolChoice     string           `json:"tool_choice,omitempty"`
	Stream         bool             `json:"stream,omitempty"`
	Stop           []string         `json:"stop,omitempty"`
	User           string           `json:"user,omitempty"`
	ResponseFormat map[string]any   `json:"response_format,omitempty"`
	StreamOptions  map[string]any   `json:"stream_options,omitempty"`
}

type oaResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string       `json:"role"`
			Content   string       `json:"content"`
			ToolCalls []oaToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func toOAMessages(msgs []ports.ChatMessage) []oaMessage {
	out := make([]oaMessage, 0, len(msgs))
	for _, m := range msgs {
		om := oaMessage{Role: string(m.Role), Name: m.Name, ToolCallID: m.ToolCallID}
		if m.Content != "" {
			om.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			om.ToolCalls = append(om.ToolCalls, oaToolCall{
				ID: tc.ID, Type: "function",
				Function: oaToolFunc{Name: tc.Name, Arguments: string(args)},
			})
			om.Content = nil
		}
		out = append(out, om)
	}
	return out
}
