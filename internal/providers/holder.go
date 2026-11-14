// Package providers owns the live model-provider registry: it merges database
// overrides (managed from the GUI) with the YAML baseline, builds providers,
// swaps the router at runtime and exposes diagnostics.
package providers

import (
	"context"
	"net/http"
	"sort"
	"sync"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Holder is a stable ModelRouter whose backend can be swapped at runtime, so
// services that captured it at boot (brain, memory, documents) follow GUI edits.
type Holder struct {
	mu sync.RWMutex
	r  ports.ModelRouter
}

func NewHolder(r ports.ModelRouter) *Holder { return &Holder{r: r} }

func (h *Holder) Swap(r ports.ModelRouter) {
	h.mu.Lock()
	h.r = r
	h.mu.Unlock()
}

func (h *Holder) Current() ports.ModelRouter {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.r
}

func (h *Holder) Resolve(agentID, intent, text string) ports.RouteDecision {
	return h.Current().Resolve(agentID, intent, text)
}

func (h *Holder) Chat(ctx context.Context, d ports.RouteDecision, req ports.ChatRequest) (*ports.ChatResponse, error) {
	return h.Current().Chat(ctx, d, req)
}

func (h *Holder) Stream(ctx context.Context, d ports.RouteDecision, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	return h.Current().Stream(ctx, d, req)
}

func (h *Holder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	return h.Current().Embed(ctx, inputs)
}

func (h *Holder) Providers() []ports.ProviderSummary {
	list := h.Current().Providers()
	sort.SliceStable(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

var _ ports.ModelRouter = (*Holder)(nil)

// pinger is implemented by HTTP-backed providers.
type pinger interface {
	Ping(context.Context) error
}

func ping(ctx context.Context, p ports.Provider) error {
	if pp, ok := p.(pinger); ok {
		return pp.Ping(ctx)
	}
	return nil
}

var _ = http.StatusOK
