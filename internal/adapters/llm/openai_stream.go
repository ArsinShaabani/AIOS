package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type oaStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type partialCall struct {
	id   string
	name string
	args strings.Builder
}

// Stream performs a server-sent-event completion.
func (o *OpenAI) Stream(ctx context.Context, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	body := o.baseReq(req)
	body.Stream = true
	body.StreamOptions = map[string]any{"include_usage": true}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, providerErr(o.name, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", strings.NewReader(string(raw)))
	if err != nil {
		return nil, providerErr(o.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range o.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, providerErr(o.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := httpx.ReadLimit(resp.Body, 64<<10)
		_ = resp.Body.Close()
		return nil, providerErr(o.name, &httpx.StatusError{StatusCode: resp.StatusCode, Body: string(payload), URL: o.baseURL})
	}

	ch := make(chan ports.Chunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		parts := map[int]*partialCall{}
		var usage *ports.Usage
		var streamErr error

		send := func(c ports.Chunk) bool {
			select {
			case ch <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}

		err := httpx.ReadSSE(ctx, resp.Body, func(line httpx.SSELine) error {
			var chunk oaStreamChunk
			if err := json.Unmarshal([]byte(line.Data), &chunk); err != nil {
				return nil // ignore keep-alives / non-JSON frames
			}
			if chunk.Error != nil {
				return &httpx.StatusError{StatusCode: http.StatusBadGateway, Body: chunk.Error.Message, URL: o.baseURL}
			}
			if chunk.Usage != nil {
				usage = &ports.Usage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				}
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					if !send(ports.Chunk{Delta: choice.Delta.Content}) {
						return context.Canceled
					}
				}
				for _, tc := range choice.Delta.ToolCalls {
					p, ok := parts[tc.Index]
					if !ok {
						p = &partialCall{}
						parts[tc.Index] = p
					}
					if tc.ID != "" {
						p.id = tc.ID
					}
					if tc.Function.Name != "" {
						p.name += tc.Function.Name
					}
					p.args.WriteString(tc.Function.Arguments)
				}
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			streamErr = providerErr(o.name, err)
			send(ports.Chunk{Err: streamErr, Done: true})
			return
		}

		calls := make([]domain.ToolCall, 0, len(parts))
		indexes := make([]int, 0, len(parts))
		for idx := range parts {
			indexes = append(indexes, idx)
		}
		sort.Ints(indexes)
		for _, idx := range indexes {
			p := parts[idx]
			args := map[string]any{}
			if s := strings.TrimSpace(p.args.String()); s != "" {
				_ = json.Unmarshal([]byte(s), &args)
			}
			calls = append(calls, domain.ToolCall{ID: p.id, Name: p.name, Arguments: args})
		}
		final := ports.Chunk{Done: true, Usage: usage}
		if len(calls) > 0 {
			final.ToolCalls = calls
		}
		send(final)
	}()
	return ch, nil
}
