package llm

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Stream streams a completion, falling back to the next provider if the first
// one fails before emitting any content.
func (r *Router) Stream(ctx context.Context, d ports.RouteDecision, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	applyDecision(&req, d)
	routes := r.routes(d)
	if len(routes) == 0 {
		return nil, platform.Unavailable("llm.no_provider", "no model provider is enabled")
	}
	out := make(chan ports.Chunk, 32)
	go func() {
		defer close(out)
		var lastErr error
		send := func(c ports.Chunk) bool {
			select {
			case out <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for i, route := range routes {
			p := r.providers[route.Provider]
			if p == nil {
				continue
			}
			br := r.breaker(route.Provider)
			if br != nil && !br.Allow() {
				lastErr = platform.Unavailable("llm.breaker_open", "provider circuit breaker is open: "+route.Provider)
				continue
			}
			if i > 0 && r.obs != nil {
				r.obs.ObserveFallback(route.Provider, route.Model)
			}
			req.Model = route.Model
			release, err := r.acquire(ctx, route.Provider)
			if err != nil {
				lastErr = err
				continue
			}
			start := time.Now()
			stream, err := p.Stream(ctx, req)
			if err != nil {
				release()
				if br != nil {
					br.Failure()
				}
				lastErr = err
				r.log.Warn("llm stream failed to start", "provider", route.Provider, "error", err)
				continue
			}
			produced := false
			var (
				streamErr error
				usage     ports.Usage
				gotUsage  bool
			)
			for chunk := range stream {
				if chunk.Err != nil {
					streamErr = chunk.Err
					break
				}
				if chunk.Delta != "" || len(chunk.ToolCalls) > 0 {
					produced = true
				}
				if chunk.Usage != nil {
					usage = *chunk.Usage
					gotUsage = true
				}
				if !send(chunk) {
					release()
					return
				}
				if chunk.Done {
					break
				}
			}
			release()
			if streamErr == nil {
				if br != nil {
					br.Success()
				}
				if r.obs != nil {
					r.obs.ObserveLLM(route.Provider, route.Model, time.Since(start), nil)
				}
				if gotUsage {
					r.recordUsage(ctx, req, route.Provider, route.Model, "chat", usage, time.Since(start))
				}
				return
			}
			lastErr = streamErr
			if br != nil {
				br.Failure()
			}
			if produced {
				// Content already reached the client; a silent retry would
				// duplicate text, so surface the error instead.
				send(ports.Chunk{Err: streamErr, Done: true})
				return
			}
			r.log.Warn("llm stream failed, trying fallback", "provider", route.Provider, "error", streamErr)
		}
		if lastErr == nil {
			lastErr = platform.Unavailable("llm.unavailable", "all model providers failed")
		}
		send(ports.Chunk{Err: lastErr, Done: true})
	}()
	return out, nil
}
