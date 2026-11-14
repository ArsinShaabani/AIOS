package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"

	adapterllm "github.com/ArsinShaabani/AIOS/internal/adapters/llm"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	routerllm "github.com/ArsinShaabani/AIOS/internal/llm"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

const (
	scope        = "config"
	scopeID      = "providers"
	overrideKey  = "override"
	secretKeyFmt = "provider.%s.api_key"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

type View struct {
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	BaseURL         string   `json:"base_url"`
	Models          []string `json:"models"`
	DefaultModel    string   `json:"default_model"`
	Enabled         bool     `json:"enabled"`
	Default         bool     `json:"default"`
	HasAPIKey       bool     `json:"has_api_key"`
	APIKeyHint      string   `json:"api_key_hint,omitempty"`
	Priority        int      `json:"priority"`
	MaxConcurrency  int      `json:"max_concurrency"`
	MaxTokens       int      `json:"max_tokens"`
	ContextWindow   int      `json:"context_window"`
	Temperature     float64  `json:"temperature"`
	TopP            float64  `json:"top_p"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Healthy         bool     `json:"healthy"`
	Breaker         string   `json:"breaker,omitempty"`
	LastError       string   `json:"last_error,omitempty"`
	Runtime         bool     `json:"runtime"`
}

type Input struct {
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	BaseURL         string   `json:"base_url"`
	APIKey          string   `json:"api_key"`
	ClearAPIKey     bool     `json:"clear_api_key"`
	Models          []string `json:"models"`
	DefaultModel    string   `json:"default_model"`
	Enabled         *bool    `json:"enabled"`
	Priority        int      `json:"priority"`
	MaxConcurrency  int      `json:"max_concurrency"`
	MaxTokens       int      `json:"max_tokens"`
	ContextWindow   int      `json:"context_window"`
	Temperature     float64  `json:"temperature"`
	TopP            float64  `json:"top_p"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
}

type TestResult struct {
	OK        bool    `json:"ok"`
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	LatencyMS int64   `json:"latency_ms"`
	TokPerSec float64 `json:"tok_per_sec,omitempty"`
	Reply     string  `json:"reply,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type overrides struct {
	Providers       []config.ProviderConfig `json:"providers"`
	DefaultProvider string                  `json:"default_provider"`
	DefaultModel    string                  `json:"default_model"`
}

type Service struct {
	mu       sync.Mutex
	llm      config.LLMConfig
	holder   *Holder
	settings ports.SettingRepository
	secrets  ports.SecretStore
	cache    ports.Cache
	usage    routerllm.UsageSink
	obs      routerllm.Observer
	log      *slog.Logger
	pulls    map[string]*PullStatus
}

func New(cfg config.LLMConfig, settings ports.SettingRepository, secrets ports.SecretStore,
	cache ports.Cache, usage routerllm.UsageSink, obs routerllm.Observer, log *slog.Logger) (*Service, error) {
	s := &Service{settings: settings, secrets: secrets, cache: cache, usage: usage,
		obs: obs, log: log, pulls: map[string]*PullStatus{}}
	list, defProv, defModel, err := s.loadOverrides(context.Background(), cfg.Providers)
	if err != nil {
		return nil, err
	}
	cfg.Providers = list
	if defProv != "" {
		cfg.DefaultProvider = defProv
	}
	if defModel != "" {
		cfg.DefaultModel = defModel
	}
	router, err := build(cfg, cache, usage, obs, log)
	if err != nil {
		return nil, err
	}
	s.llm = cfg
	s.holder = NewHolder(router)
	return s, nil
}

func (s *Service) Router() ports.ModelRouter { return s.holder }

func (s *Service) LLM() config.LLMConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.llm
}

func (s *Service) loadOverrides(ctx context.Context, base []config.ProviderConfig) ([]config.ProviderConfig, string, string, error) {
	row, err := s.settings.Get(ctx, scope, scopeID, overrideKey)
	if err != nil || row == nil {
		return base, "", "", nil
	}
	var ov overrides
	if err := json.Unmarshal(row.Value, &ov); err != nil {
		return nil, "", "", fmt.Errorf("provider overrides are corrupt: %w", err)
	}
	if len(ov.Providers) == 0 {
		return base, ov.DefaultProvider, ov.DefaultModel, nil
	}
	for i := range ov.Providers {
		if key, err := s.secrets.Get(ctx, fmt.Sprintf(secretKeyFmt, ov.Providers[i].Name)); err == nil {
			ov.Providers[i].APIKey = key
		}
	}
	return ov.Providers, ov.DefaultProvider, ov.DefaultModel, nil
}

func build(cfg config.LLMConfig, cache ports.Cache, usage routerllm.UsageSink, obs routerllm.Observer, log *slog.Logger) (*routerllm.Router, error) {
	built, errs := adapterllm.BuildAll(cfg.Providers)
	if len(errs) > 0 && log != nil {
		for _, e := range errs {
			log.Warn("provider skipped", "error", e)
		}
	}
	if _, ok := built[cfg.DefaultProvider]; !ok {
		msg := "default provider is not available"
		if len(errs) > 0 {
			msg += ": " + errs[0].Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return routerllm.NewRouter(cfg, built, cache, usage, obs, log), nil
}

func (s *Service) applyLocked(ctx context.Context, cfg config.LLMConfig) error {
	router, err := build(cfg, s.cache, s.usage, s.obs, s.log)
	if err != nil {
		return err
	}
	s.llm = cfg
	s.holder.Swap(router)
	clean := make([]config.ProviderConfig, len(cfg.Providers))
	for i, p := range cfg.Providers {
		p.APIKey = ""
		clean[i] = p
	}
	raw, err := json.Marshal(overrides{Providers: clean, DefaultProvider: cfg.DefaultProvider, DefaultModel: cfg.DefaultModel})
	if err != nil {
		return err
	}
	return s.settings.Set(ctx, &domain.Setting{Scope: scope, ScopeID: scopeID, Key: overrideKey, Value: raw})
}
