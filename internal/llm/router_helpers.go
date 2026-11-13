package llm

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// cost returns the USD cost for a usage record based on configured pricing.
func (r *Router) cost(provider, model string, usage ports.Usage) float64 {
	for _, m := range r.pcfg[provider].Models {
		if m.Name == model {
			return float64(usage.PromptTokens)/1000*m.PromptPer1K + float64(usage.CompletionTokens)/1000*m.CompletionPer1K
		}
	}
	return 0
}

func (r *Router) recordUsage(ctx context.Context, req ports.ChatRequest, provider, model, kind string, usage ports.Usage, latency time.Duration) {
	cost := r.cost(provider, model, usage)
	if r.obs != nil {
		r.obs.ObserveTokens(provider, model, usage.PromptTokens, usage.CompletionTokens, cost)
	}
	if r.usage == nil {
		return
	}
	userID, sessionID := "", ""
	if req.Metadata != nil {
		userID = req.Metadata["user_id"]
		sessionID = req.Metadata["session_id"]
	}
	r.usage.Record(ctx, domain.UsageRecord{
		ID:               platform.NewID("use"),
		UserID:           userID,
		SessionID:        sessionID,
		Provider:         provider,
		Model:            model,
		Kind:             kind,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CostUSD:          cost,
		LatencyMS:        latency.Milliseconds(),
		CreatedAt:        time.Now().UTC(),
	})
}

func (r *Router) acquire(ctx context.Context, provider string) (func(), error) {
	sem, ok := r.sem[provider]
	if !ok {
		return func() {}, nil
	}
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, platform.Wrap(platform.KindTimeout, "llm.semaphore", "model concurrency limit reached", ctx.Err())
	}
}

// routes flattens the decision into primary + fallbacks.
func (r *Router) routes(d ports.RouteDecision) []ports.Fallback {
	out := make([]ports.Fallback, 0, 1+len(d.Fallbacks))
	primary := ports.Fallback{Provider: d.Provider, Model: d.Model}
	if r.available(d.Provider) {
		out = append(out, primary)
	}
	for _, fb := range d.Fallbacks {
		if r.available(fb.Provider) && fb.Provider != d.Provider {
			out = append(out, fb)
		}
	}
	return out
}

func (r *Router) breaker(provider string) *platform.Breaker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if b, ok := r.breakers[provider]; ok {
		return b
	}
	return nil
}
