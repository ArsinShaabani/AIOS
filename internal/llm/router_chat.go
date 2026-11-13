package llm

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Chat executes a completion with per-provider retries and route fallbacks.
func (r *Router) Chat(ctx context.Context, d ports.RouteDecision, req ports.ChatRequest) (*ports.ChatResponse, error) {
	applyDecision(&req, d)
	routes := r.routes(d)
	if len(routes) == 0 {
		return nil, platform.Unavailable("llm.no_provider", "no model provider is enabled")
	}
	var lastErr error
	for i, route := range routes {
		br := r.breaker(route.Provider)
		if br != nil && !br.Allow() {
			lastErr = platform.Unavailable("llm.breaker_open", "provider circuit breaker is open: "+route.Provider)
			continue
		}
		if i > 0 && r.obs != nil {
			r.obs.ObserveFallback(route.Provider, route.Model)
		}
		start := time.Now()
		resp, err := r.chatWithRetry(ctx, route, req)
		if err == nil {
			if br != nil {
				br.Success()
			}
			if r.obs != nil {
				r.obs.ObserveLLM(route.Provider, route.Model, time.Since(start), nil)
			}
			r.recordUsage(ctx, req, resp.Provider, resp.Model, "chat", resp.Usage, time.Since(start))
			return resp, nil
		}
		if br != nil {
			br.Failure()
		}
		if r.obs != nil {
			r.obs.ObserveLLM(route.Provider, route.Model, time.Since(start), err)
		}
		lastErr = err
		r.log.Warn("llm route failed", "provider", route.Provider, "model", route.Model, "error", err)
	}
	if lastErr == nil {
		lastErr = platform.Unavailable("llm.unavailable", "all model providers failed")
	}
	return nil, lastErr
}

func (r *Router) chatWithRetry(ctx context.Context, route ports.Fallback, req ports.ChatRequest) (*ports.ChatResponse, error) {
	p := r.providers[route.Provider]
	if p == nil {
		return nil, platform.Unavailable("llm.missing_provider", "provider not loaded: "+route.Provider)
	}
	req.Model = route.Model
	attempts := r.cfg.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		callCtx := ctx
		var cancel context.CancelFunc
		if r.cfg.Timeout.Std() > 0 {
			callCtx, cancel = context.WithTimeout(ctx, r.cfg.Timeout.Std())
		}
		release, err := r.acquire(callCtx, route.Provider)
		if err != nil {
			if cancel != nil {
				cancel()
			}
			return nil, err
		}
		resp, err := p.Chat(callCtx, req)
		release()
		if cancel != nil {
			cancel()
		}
		if err == nil {
			if resp.Provider == "" {
				resp.Provider = route.Provider
			}
			return resp, nil
		}
		lastErr = err
		if !platform.Retryable(err) {
			return nil, err
		}
		if attempt < attempts-1 {
			if err := sleepCtx(ctx, r.retryDelay(attempt)); err != nil {
				return nil, lastErr
			}
		}
	}
	return nil, lastErr
}

func (r *Router) retryDelay(attempt int) time.Duration {
	return min(time.Duration(300<<min(attempt, 5))*time.Millisecond, 5*time.Second)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func applyDecision(req *ports.ChatRequest, d ports.RouteDecision) {
	if req.Temperature == 0 {
		req.Temperature = d.Temperature
	}
	if req.TopP == 0 {
		req.TopP = d.TopP
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = d.MaxTokens
	}
	if req.Model == "" {
		req.Model = d.Model
	}
}
