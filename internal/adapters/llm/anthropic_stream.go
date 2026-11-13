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

type anthropicEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream performs an SSE completion against the Messages API.
func (a *Anthropic) Stream(ctx context.Context, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	body := a.build(req)
	body.Stream = true
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, providerErr(a.name, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", strings.NewReader(string(raw)))
	if err != nil {
		return nil, providerErr(a.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range a.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, providerErr(a.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := httpx.ReadLimit(resp.Body, 64<<10)
		_ = resp.Body.Close()
		return nil, providerErr(a.name, &httpx.StatusError{StatusCode: resp.StatusCode, Body: string(payload), URL: a.baseURL})
	}

	ch := make(chan ports.Chunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		type block struct {
			id   string
			name string
			args strings.Builder
		}
		blocks := map[int]*block{}
		usage := &ports.Usage{}
		stopReason := ""

		send := func(c ports.Chunk) bool {
			select {
			case ch <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}

		err := httpx.ReadSSE(ctx, resp.Body, func(line httpx.SSELine) error {
			var ev anthropicEvent
			if err := json.Unmarshal([]byte(line.Data), &ev); err != nil {
				return nil
			}
			switch ev.Type {
			case "message_start":
				usage.PromptTokens = ev.Message.Usage.InputTokens
			case "content_block_start":
				if ev.ContentBlock.Type == "tool_use" {
					blocks[ev.Index] = &block{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				}
			case "content_block_delta":
				switch ev.Delta.Type {
				case "text_delta":
					if ev.Delta.Text != "" && !send(ports.Chunk{Delta: ev.Delta.Text}) {
						return context.Canceled
					}
				case "input_json_delta":
					if b, ok := blocks[ev.Index]; ok {
						b.args.WriteString(ev.Delta.PartialJSON)
					}
				}
			case "message_delta":
				if ev.Delta.StopReason != "" {
					stopReason = ev.Delta.StopReason
				}
				if ev.Usage.OutputTokens > 0 {
					usage.CompletionTokens = ev.Usage.OutputTokens
				}
			case "error":
				if ev.Error != nil {
					return &httpx.StatusError{StatusCode: http.StatusBadGateway, Body: ev.Error.Message, URL: a.baseURL}
				}
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			send(ports.Chunk{Err: providerErr(a.name, err), Done: true})
			return
		}
		indexes := make([]int, 0, len(blocks))
		for idx := range blocks {
			indexes = append(indexes, idx)
		}
		sort.Ints(indexes)
		calls := make([]domain.ToolCall, 0, len(indexes))
		for _, idx := range indexes {
			b := blocks[idx]
			args := map[string]any{}
			if s := strings.TrimSpace(b.args.String()); s != "" {
				_ = json.Unmarshal([]byte(s), &args)
			}
			calls = append(calls, domain.ToolCall{ID: b.id, Name: b.name, Arguments: args})
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		final := ports.Chunk{Done: true, Usage: usage}
		if len(calls) > 0 {
			final.ToolCalls = calls
		}
		_ = stopReason
		send(final)
	}()
	return ch, nil
}
