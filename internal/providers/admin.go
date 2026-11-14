package providers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// List returns every provider with runtime health.
func (s *Service) List(ctx context.Context) []View {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.llm
	summaries := map[string]ports.ProviderSummary{}
	for _, sm := range s.holder.Providers() {
		summaries[sm.Name] = sm
	}
	out := make([]View, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		v := View{Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, DefaultModel: p.DefaultModel,
			Enabled: p.IsEnabled(), Default: p.Name == cfg.DefaultProvider,
			Priority: p.Priority, MaxConcurrency: p.MaxConcurrency,
			MaxTokens: p.MaxTokens, ContextWindow: p.ContextWindow,
			Temperature: p.Temperature, TopP: p.TopP, ReasoningEffort: p.ReasoningEffort}
		for _, m := range p.Models {
			v.Models = append(v.Models, m.Name)
		}
		if p.APIKey != "" {
			v.HasAPIKey, v.APIKeyHint = true, maskKey(p.APIKey)
		}
		if sm, ok := summaries[p.Name]; ok {
			v.Healthy, v.Breaker, v.LastError, v.Runtime = sm.Healthy, sm.Breaker, sm.LastError, true
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Upsert creates or replaces a provider and applies it without a restart.
func (s *Service) Upsert(ctx context.Context, in Input) (View, error) {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if !namePattern.MatchString(in.Name) {
		return View{}, platform.Invalid("provider.name", "name must match "+namePattern.String())
	}
	switch in.Kind {
	case "openai", "anthropic", "gemini", "ollama":
	default:
		return View{}, platform.Invalid("provider.kind", "kind must be openai, anthropic, gemini or ollama")
	}
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.Kind != "ollama" && in.BaseURL == "" {
		return View{}, platform.Invalid("provider.base_url", "base_url is required for "+in.Kind+" providers")
	}
	if in.MaxTokens < 0 || in.ContextWindow < 0 || in.MaxTokens > 2_000_000 || in.ContextWindow > 10_000_000 {
		return View{}, platform.Invalid("provider.limits", "invalid token limits")
	}
	if in.Temperature < 0 || in.Temperature > 2 || in.TopP < 0 || in.TopP > 1 {
		return View{}, platform.Invalid("provider.sampling", "temperature must be 0..2 and top_p 0..1")
	}
	switch in.ReasoningEffort {
	case "", "low", "medium", "high":
	default:
		return View{}, platform.Invalid("provider.effort", "reasoning_effort must be low, medium or high")
	}
	models := dedupe(in.Models)
	if in.DefaultModel == "" && len(models) > 0 {
		in.DefaultModel = models[0]
	}
	if in.DefaultModel == "" {
		return View{}, platform.Invalid("provider.model", "a model name is required")
	}
	if !contains(models, in.DefaultModel) {
		models = append([]string{in.DefaultModel}, models...)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	pc := config.ProviderConfig{Name: in.Name, Kind: in.Kind, BaseURL: in.BaseURL,
		DefaultModel: in.DefaultModel, Enabled: &enabled, Priority: in.Priority,
		MaxConcurrency: in.MaxConcurrency, MaxTokens: in.MaxTokens,
		ContextWindow: in.ContextWindow, Temperature: in.Temperature, TopP: in.TopP,
		ReasoningEffort: in.ReasoningEffort}
	for _, m := range models {
		pc.Models = append(pc.Models, config.ModelConfig{Name: m})
	}

	cfg := s.llm
	replaced := false
	for i := range cfg.Providers {
		if cfg.Providers[i].Name != in.Name {
			continue
		}
		if in.APIKey == "" && !in.ClearAPIKey {
			pc.APIKey = cfg.Providers[i].APIKey
		}
		cfg.Providers[i] = pc
		replaced = true
		break
	}
	if !replaced {
		cfg.Providers = append(cfg.Providers, pc)
	}

	switch {
	case in.ClearAPIKey:
		_ = s.secrets.Delete(ctx, fmt.Sprintf(secretKeyFmt, in.Name))
	case in.APIKey != "":
		if err := s.secrets.Set(ctx, fmt.Sprintf(secretKeyFmt, in.Name), in.APIKey); err != nil {
			return View{}, fmt.Errorf("store api key: %w", err)
		}
		pc.APIKey = in.APIKey
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == in.Name {
			cfg.Providers[i].APIKey = pc.APIKey
		}
	}
	if cfg.DefaultProvider == in.Name || !enabledProvider(cfg.Providers, cfg.DefaultProvider) {
		cfg.DefaultProvider, cfg.DefaultModel = in.Name, in.DefaultModel
	}
	if err := s.applyLocked(ctx, cfg); err != nil {
		return View{}, err
	}
	// Built inline: List() takes the same lock.
	view := View{Name: pc.Name, Kind: pc.Kind, BaseURL: pc.BaseURL, DefaultModel: pc.DefaultModel,
		Enabled: enabled, Default: cfg.DefaultProvider == pc.Name, Priority: pc.Priority,
		MaxConcurrency: pc.MaxConcurrency, HasAPIKey: pc.APIKey != "",
		MaxTokens: pc.MaxTokens, ContextWindow: pc.ContextWindow, Temperature: pc.Temperature,
		TopP: pc.TopP, ReasoningEffort: pc.ReasoningEffort}
	for _, m := range pc.Models {
		view.Models = append(view.Models, m.Name)
	}
	if pc.APIKey != "" {
		view.APIKeyHint = maskKey(pc.APIKey)
	}
	return view, nil
}
