package brain

import (
	"context"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// streamCompletion consumes a router stream, forwarding deltas to the client and
// collecting tool calls and usage.
func (e *Engine) streamCompletion(ctx context.Context, router ports.ModelRouter, route ports.RouteDecision, req ports.ChatRequest, emit EmitFunc) (string, []domain.ToolCall, ports.Usage, string, error) {
	var (
		b        strings.Builder
		calls    []domain.ToolCall
		usage    ports.Usage
		provider string
	)
	stream, err := router.Stream(ctx, route, req)
	if err != nil {
		return "", nil, usage, "", err
	}
	for chunk := range stream {
		if chunk.Err != nil {
			return b.String(), calls, usage, provider, chunk.Err
		}
		if chunk.Delta != "" {
			b.WriteString(chunk.Delta)
			if err := emit(Event{Type: "delta", Delta: chunk.Delta}); err != nil {
				return b.String(), calls, usage, provider, context.Canceled
			}
		}
		if len(chunk.ToolCalls) > 0 {
			calls = chunk.ToolCalls
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if chunk.Provider != "" {
			provider = chunk.Provider
		}
	}
	return b.String(), calls, usage, provider, nil
}
