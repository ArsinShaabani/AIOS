package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	adapterllm "github.com/ArsinShaabani/AIOS/internal/adapters/llm"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Delete removes a provider and its stored key.
func (s *Service) Delete(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.llm
	out := make([]config.ProviderConfig, 0, len(cfg.Providers))
	found := false
	for _, p := range cfg.Providers {
		if p.Name == name {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return platform.NotFound("provider.not_found", "provider does not exist")
	}
	if len(out) == 0 {
		return fmt.Errorf("cannot delete the only provider")
	}
	cfg.Providers = out
	if !enabledProvider(cfg.Providers, cfg.DefaultProvider) {
		cfg.DefaultProvider, cfg.DefaultModel = "", ""
		for _, p := range cfg.Providers {
			if p.IsEnabled() {
				cfg.DefaultProvider, cfg.DefaultModel = p.Name, p.DefaultModel
				break
			}
		}
	}
	if err := s.applyLocked(ctx, cfg); err != nil {
		return err
	}
	_ = s.secrets.Delete(ctx, fmt.Sprintf(secretKeyFmt, name))
	return nil
}

// SetDefault changes the default route.
func (s *Service) SetDefault(ctx context.Context, provider, model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.llm
	found := false
	for _, p := range cfg.Providers {
		if p.Name != provider {
			continue
		}
		found = true
		if !p.IsEnabled() {
			return fmt.Errorf("provider %q is disabled", provider)
		}
		if model == "" {
			model = p.DefaultModel
		}
	}
	if !found {
		return platform.NotFound("provider.not_found", "provider does not exist")
	}
	if model == "" {
		return fmt.Errorf("model is required")
	}
	cfg.DefaultProvider, cfg.DefaultModel = provider, model
	return s.applyLocked(ctx, cfg)
}

// Test performs a real completion against one provider.
func (s *Service) Test(ctx context.Context, name, model string) TestResult {
	s.mu.Lock()
	var target *config.ProviderConfig
	for i := range s.llm.Providers {
		if s.llm.Providers[i].Name == name {
			p := s.llm.Providers[i]
			target = &p
			break
		}
	}
	s.mu.Unlock()
	if target == nil {
		return TestResult{Provider: name, Error: "provider does not exist"}
	}
	built, errs := adapterllm.BuildAll([]config.ProviderConfig{*target})
	p := built[name]
	if p == nil {
		msg := "provider could not be built"
		if len(errs) > 0 {
			msg = errs[0].Error()
		}
		return TestResult{Provider: name, Model: model, Error: msg}
	}
	if model == "" {
		model = target.DefaultModel
	}
	start := time.Now()
	if err := ping(ctx, p); err != nil {
		return TestResult{Provider: name, Model: model, LatencyMS: time.Since(start).Milliseconds(), Error: err.Error()}
	}
	resp, err := p.Chat(ctx, ports.ChatRequest{Model: model,
		Messages:  []ports.ChatMessage{{Role: "user", Content: "Reply with the single word: ready"}},
		MaxTokens: 16, Temperature: 0})
	elapsed := time.Since(start)
	if err != nil {
		return TestResult{Provider: name, Model: model, LatencyMS: elapsed.Milliseconds(), Error: err.Error()}
	}
	out := TestResult{OK: true, Provider: name, Model: model, LatencyMS: elapsed.Milliseconds(),
		Reply: strings.TrimSpace(resp.Content)}
	if resp.Usage.CompletionTokens > 0 && elapsed > 0 {
		out.TokPerSec = float64(resp.Usage.CompletionTokens) / elapsed.Seconds()
	}
	return out
}

func enabledProvider(list []config.ProviderConfig, name string) bool {
	for _, p := range list {
		if p.Name == name && p.IsEnabled() {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func maskKey(k string) string {
	if len(k) <= 4 {
		return "••••"
	}
	return "••••" + k[len(k)-4:]
}
