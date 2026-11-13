package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Stream performs an SSE generateContent call.
func (g *Gemini) Stream(ctx context.Context, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	body := g.build(req)
	body.GenerationConfig = mergeGenerationConfig(body.GenerationConfig)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, providerErr(g.name, err)
	}
	url := g.baseURL + "/models/" + stripModels(firstOr(req.Model, g.defaultModel())) +
		":streamGenerateContent?alt=sse&key=" + g.apiKey
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		return nil, providerErr(g.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, providerErr(g.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := httpx.ReadLimit(resp.Body, 64<<10)
		_ = resp.Body.Close()
		return nil, providerErr(g.name, &httpx.StatusError{StatusCode: resp.StatusCode, Body: string(payload), URL: url})
	}

	ch := make(chan ports.Chunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		usage := &ports.Usage{}
		calls := make([]domain.ToolCall, 0, 2)
		pending := map[string]domain.ToolCall{}

		send := func(c ports.Chunk) bool {
			select {
			case ch <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}

		err := httpx.ReadSSE(ctx, resp.Body, func(line httpx.SSELine) error {
			var chunk geminiChunk
			if err := json.Unmarshal([]byte(line.Data), &chunk); err != nil {
				return nil
			}
			if chunk.Error != nil {
				return &httpx.StatusError{StatusCode: http.StatusBadGateway, Body: chunk.Error.Message, URL: url}
			}
			if chunk.UsageMetadata.TotalTokenCount > 0 {
				usage.PromptTokens = chunk.UsageMetadata.PromptTokenCount
				usage.CompletionTokens = chunk.UsageMetadata.CandidatesTokenCount
				usage.TotalTokens = chunk.UsageMetadata.TotalTokenCount
			}
			for _, cand := range chunk.Candidates {
				for i, part := range cand.Content.Parts {
					if part.Text != "" {
						if !send(ports.Chunk{Delta: part.Text}) {
							return context.Canceled
						}
					}
					if part.FunctionCall != nil {
						call := domain.ToolCall{
							ID:        fmt.Sprintf("%s_%d", part.FunctionCall.Name, i),
							Name:      part.FunctionCall.Name,
							Arguments: part.FunctionCall.Args,
						}
						pending[call.ID] = call
					}
				}
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			send(ports.Chunk{Err: providerErr(g.name, err), Done: true})
			return
		}
		for _, c := range pending {
			calls = append(calls, c)
		}
		final := ports.Chunk{Done: true, Usage: usage}
		if len(calls) > 0 {
			final.ToolCalls = calls
		}
		send(final)
	}()
	return ch, nil
}

// mergeGenerationConfig ensures maxOutputTokens is always present; some Gemini
// versions require it for streaming responses.
func mergeGenerationConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		cfg = map[string]any{}
	}
	if _, ok := cfg["maxOutputTokens"]; !ok {
		cfg["maxOutputTokens"] = 1024
	}
	return cfg
}
