package llm

import (
	"fmt"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// NewProvider builds the provider adapter for a configuration entry. Adding a
// new backend means adding a case here — the core never changes.
func NewProvider(cfg config.ProviderConfig) (ports.Provider, error) {
	switch cfg.Kind {
	case "openai", "ollama":
		return NewOpenAI(cfg), nil
	case "anthropic":
		return NewAnthropic(cfg), nil
	case "gemini":
		return NewGemini(cfg), nil
	case "local":
		return NewLocal(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", cfg.Kind)
	}
}

// BuildAll constructs every enabled provider plus a registry keyed by name.
func BuildAll(cfgs []config.ProviderConfig) (map[string]ports.Provider, []error) {
	providers := make(map[string]ports.Provider, len(cfgs))
	var errs []error
	for _, cfg := range cfgs {
		if !cfg.IsEnabled() {
			continue
		}
		p, err := NewProvider(cfg)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %s: %w", cfg.Name, err))
			continue
		}
		providers[cfg.Name] = p
	}
	return providers, errs
}
