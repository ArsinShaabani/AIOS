// Package llm hosts the model router: it resolves a provider/model for each
// request (intent + agent aware), applies retries/fallbacks/circuit breaking,
// enforces token budgets, caches embeddings and records usage/cost.
package llm

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// UsageSink persists usage records. Implementations should be non-blocking.
type UsageSink interface {
	Record(ctx context.Context, rec domain.UsageRecord)
}

// Observer receives metrics callbacks. It is optional (nil-safe).
type Observer interface {
	ObserveLLM(provider, model string, duration time.Duration, err error)
	ObserveTokens(provider, model string, prompt, completion int, cost float64)
	ObserveFallback(provider, model string)
}

// Router implements ports.ModelRouter.
type Router struct {
	cfg       config.LLMConfig
	providers map[string]ports.Provider
	pcfg      map[string]config.ProviderConfig
	breakers  map[string]*platform.Breaker
	sem       map[string]chan struct{}
	cache     ports.Cache
	usage     UsageSink
	obs       Observer
	log       *slog.Logger
	mu        sync.RWMutex
	lastErr   map[string]string
}

var _ ports.ModelRouter = (*Router)(nil)

// NewRouter builds the router from configuration and constructed providers.
func NewRouter(cfg config.LLMConfig, providers map[string]ports.Provider, cache ports.Cache, usage UsageSink, obs Observer, log *slog.Logger) *Router {
	if log == nil {
		log = slog.Default()
	}
	r := &Router{
		cfg:       cfg,
		providers: providers,
		pcfg:      make(map[string]config.ProviderConfig, len(cfg.Providers)),
		breakers:  make(map[string]*platform.Breaker, len(providers)),
		sem:       make(map[string]chan struct{}, len(providers)),
		cache:     cache,
		usage:     usage,
		obs:       obs,
		log:       log,
		lastErr:   make(map[string]string),
	}
	for _, pc := range cfg.Providers {
		r.pcfg[pc.Name] = pc
		limit := pc.MaxConcurrency
		if limit <= 0 {
			limit = cfg.MaxConcurrency
		}
		if limit <= 0 {
			limit = 2
		}
		r.sem[pc.Name] = make(chan struct{}, limit)
	}
	for name := range providers {
		r.breakers[name] = platform.NewBreaker(5, 30*time.Second)
	}
	return r
}

// Resolve picks a route using the rules table, then appends other enabled
// providers as fallbacks ordered by priority.
func (r *Router) Resolve(agentID, intent, text string) ports.RouteDecision {
	d := ports.RouteDecision{
		Provider:    r.cfg.DefaultProvider,
		Model:       r.cfg.DefaultModel,
		Temperature: r.cfg.Temperature,
		TopP:        r.cfg.TopP,
		MaxTokens:   r.cfg.MaxTokens,
		Reason:      "default",
	}
	// Provider/model defaults: context window, output cap and effort.
	if pc, ok := r.pcfg[d.Provider]; ok {
		if pc.Temperature > 0 {
			d.Temperature = pc.Temperature
		}
		if pc.TopP > 0 {
			d.TopP = pc.TopP
		}
		if pc.MaxTokens > 0 {
			d.MaxTokens = pc.MaxTokens
		}
		if pc.ContextWindow > 0 {
			d.ContextWindow = pc.ContextWindow
		}
		d.ReasoningEffort = pc.ReasoningEffort
		for _, m := range pc.Models {
			if m.Name != d.Model {
				continue
			}
			if m.ContextWindow > 0 {
				d.ContextWindow = m.ContextWindow
			}
			if m.MaxOutputTokens > 0 {
				d.MaxTokens = m.MaxOutputTokens
			}
			if m.ReasoningEffort != "" {
				d.ReasoningEffort = m.ReasoningEffort
			}
		}
	}
	lower := strings.ToLower(text)
	for _, rule := range r.cfg.Routing {
		if !r.ruleMatches(rule, agentID, intent, lower) {
			continue
		}
		if rule.Provider != "" && r.available(rule.Provider) {
			d.Provider = rule.Provider
			d.Model = firstOr(rule.Model, r.defaultModel(rule.Provider))
		} else if rule.Model != "" {
			d.Model = rule.Model
		}
		if rule.MaxTokens > 0 {
			d.MaxTokens = rule.MaxTokens
		}
		if rule.Temperature != nil {
			d.Temperature = *rule.Temperature
		}
		d.Reason = "routing:" + rule.Name
		for _, fb := range rule.Fallback {
			provider, model := splitRoute(fb)
			if provider != "" && r.available(provider) {
				d.Fallbacks = append(d.Fallbacks, ports.Fallback{Provider: provider, Model: firstOr(model, r.defaultModel(provider))})
			}
		}
		break
	}
	for _, name := range r.sortedProviders() {
		if name == d.Provider {
			continue
		}
		dup := false
		for _, fb := range d.Fallbacks {
			if fb.Provider == name {
				dup = true
				break
			}
		}
		if !dup {
			d.Fallbacks = append(d.Fallbacks, ports.Fallback{Provider: name, Model: r.defaultModel(name)})
		}
	}
	return d
}

func (r *Router) ruleMatches(rule config.RoutingRule, agentID, intent, lowerText string) bool {
	if rule.Agent != "" && agentID != "" && strings.EqualFold(rule.Agent, agentID) {
		return true
	}
	for _, in := range rule.Intents {
		if intent != "" && strings.EqualFold(in, intent) {
			return true
		}
	}
	for _, kw := range rule.Keywords {
		if kw != "" && strings.Contains(lowerText, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func (r *Router) available(name string) bool {
	_, ok := r.providers[name]
	return ok
}

func (r *Router) defaultModel(provider string) string {
	if pc, ok := r.pcfg[provider]; ok {
		if pc.DefaultModel != "" {
			return pc.DefaultModel
		}
		if len(pc.Models) > 0 {
			return pc.Models[0].Name
		}
	}
	return ""
}

func (r *Router) sortedProviders() []string {
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		pi, pj := r.pcfg[names[i]].Priority, r.pcfg[names[j]].Priority
		if pi == pj {
			return names[i] < names[j]
		}
		return pi < pj
	})
	return names
}

func splitRoute(s string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(s), "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
