package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Embed returns embeddings for inputs, using the configured embedding provider,
// a provider-level cache and graceful fallback to any embeddings-capable
// provider.
func (r *Router) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	providerName := r.cfg.EmbeddingProvider
	p := r.providers[providerName]
	if p == nil || !p.Capabilities().Embeddings {
		p, providerName = r.embeddingFallback()
	}
	if p == nil {
		return nil, platform.Unavailable("llm.no_embedder", "no embeddings-capable provider is enabled")
	}
	model := r.cfg.EmbeddingModel
	if model == "" {
		model = "text-embedding-3-small"
	}

	vectors := make([][]float32, len(inputs))
	missing := make([]int, 0, len(inputs))
	for i, in := range inputs {
		if v, ok := r.cachedEmbedding(ctx, providerName, model, in); ok {
			vectors[i] = v
			continue
		}
		missing = append(missing, i)
	}
	if len(missing) == 0 {
		return vectors, nil
	}

	texts := make([]string, len(missing))
	for i, idx := range missing {
		texts[i] = inputs[idx]
	}
	release, err := r.acquire(ctx, providerName)
	if err != nil {
		return nil, err
	}
	callCtx := ctx
	var cancel context.CancelFunc
	if r.cfg.Timeout.Std() > 0 {
		callCtx, cancel = context.WithTimeout(ctx, r.cfg.Timeout.Std())
	}
	start := time.Now()
	embedded, err := p.Embed(callCtx, model, texts)
	if cancel != nil {
		cancel()
	}
	release()
	if err != nil {
		if p2, name2 := r.embeddingFallbackExcept(providerName); p2 != nil {
			r.log.Warn("embedding provider failed, falling back", "provider", providerName, "fallback", name2, "error", err)
			return r.embedWith(ctx, p2, name2, model, inputs)
		}
		return nil, err
	}
	if len(embedded) != len(missing) {
		return nil, platform.Internal("llm.embed_count", "embedding provider returned a mismatched vector count")
	}
	for i, idx := range missing {
		vectors[idx] = embedded[i]
		r.storeEmbedding(ctx, providerName, model, inputs[idx], embedded[i])
	}
	prompt := 0
	for _, t := range texts {
		prompt += estimateTokensText(t)
	}
	r.recordUsage(ctx, ports.ChatRequest{}, providerName, model, "embedding", ports.Usage{PromptTokens: prompt}, time.Since(start))
	return vectors, nil
}

func (r *Router) embedWith(ctx context.Context, p ports.Provider, name, model string, inputs []string) ([][]float32, error) {
	release, err := r.acquire(ctx, name)
	if err != nil {
		return nil, err
	}
	defer release()
	vecs, err := p.Embed(ctx, model, inputs)
	if err != nil {
		return nil, err
	}
	for i := range vecs {
		if i < len(inputs) {
			r.storeEmbedding(ctx, name, model, inputs[i], vecs[i])
		}
	}
	return vecs, nil
}

func (r *Router) embeddingFallback() (ports.Provider, string) {
	return r.embeddingFallbackExcept("")
}

// embeddingFallbackExcept picks an embeddings-capable provider, skipping the one
// that just failed.
func (r *Router) embeddingFallbackExcept(exclude string) (ports.Provider, string) {
	for _, name := range r.sortedProviders() {
		if name == exclude {
			continue
		}
		p := r.providers[name]
		if p != nil && p.Capabilities().Embeddings {
			return p, name
		}
	}
	return nil, ""
}

func (r *Router) embeddingKey(provider, model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return "emb:" + provider + ":" + hex.EncodeToString(sum[:16])
}

func (r *Router) cachedEmbedding(ctx context.Context, provider, model, text string) ([]float32, bool) {
	if r.cache == nil {
		return nil, false
	}
	raw, ok, err := r.cache.Get(ctx, r.embeddingKey(provider, model, text))
	if err != nil || !ok {
		return nil, false
	}
	var vec []float32
	if err := json.Unmarshal(raw, &vec); err != nil || len(vec) == 0 {
		return nil, false
	}
	return vec, true
}

func (r *Router) storeEmbedding(ctx context.Context, provider, model, text string, vec []float32) {
	if r.cache == nil || len(vec) == 0 {
		return
	}
	raw, err := json.Marshal(vec)
	if err != nil {
		return
	}
	_ = r.cache.Set(ctx, r.embeddingKey(provider, model, text), raw, 7*24*time.Hour)
}

// Providers returns redacted provider summaries for the dashboard.
func (r *Router) Providers() []ports.ProviderSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ports.ProviderSummary, 0, len(r.providers))
	for _, name := range r.sortedProviders() {
		p := r.providers[name]
		if p == nil {
			continue
		}
		brState := "closed"
		healthy := true
		if b, ok := r.breakers[name]; ok {
			brState = b.State()
			healthy = brState != "open"
		}
		out = append(out, ports.ProviderSummary{
			Name:         name,
			Kind:         r.pcfg[name].Kind,
			Enabled:      true,
			Models:       p.Models(),
			Capabilities: p.Capabilities(),
			Healthy:      healthy,
			Breaker:      brState,
		})
	}
	return out
}

// Ping checks every provider's connectivity (used by /readyz and the doctor).
func (r *Router) Ping(ctx context.Context) map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.providers))
	for name, p := range r.providers {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := p.Ping(cctx)
		cancel()
		if err != nil {
			out[name] = err.Error()
		} else {
			out[name] = "ok"
		}
	}
	return out
}
