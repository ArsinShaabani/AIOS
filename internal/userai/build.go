package userai

import (
	"context"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/llm"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/providers"
)

// Credential returns the decrypted provider config for server-side use.
func (s *Service) Credential(ctx context.Context, userID, name string) (config.ProviderConfig, error) {
	rows, err := s.repo.List(ctx, userID)
	if err != nil {
		return config.ProviderConfig{}, err
	}
	if len(rows) == 0 {
		return config.ProviderConfig{}, platform.Invalid("provider.missing", "add an AI provider on your account first")
	}
	var row *domain.UserProvider
	if name == "" {
		row = &rows[0]
	} else {
		for i := range rows {
			if rows[i].Name == name {
				row = &rows[i]
				break
			}
		}
	}
	if row == nil {
		return config.ProviderConfig{}, platform.NotFound("provider.not_found", "provider not found")
	}
	if !row.Enabled {
		return config.ProviderConfig{}, platform.Invalid("provider.disabled", "provider is disabled")
	}
	key := ""
	if row.APIKeyEnc != "" {
		key, err = s.keyring.Decrypt(row.APIKeyEnc)
		if err != nil {
			return config.ProviderConfig{}, platform.Wrap(platform.KindInternal, "provider.decrypt", "cannot decrypt api key", err)
		}
	}
	cfg := config.ProviderConfig{Name: row.Name, Kind: row.Kind, BaseURL: row.BaseURL,
		APIKey: key, DefaultModel: row.DefaultModel, ContextWindow: row.ContextWindow, MaxTokens: row.MaxTokens}
	for _, m := range row.Models {
		cfg.Models = append(cfg.Models, config.ModelConfig{Name: m, MaxOutputTokens: row.MaxTokens, ContextWindow: row.ContextWindow})
	}
	return cfg, nil
}

// Build returns a live provider for one of the user's credentials.
func (s *Service) Build(ctx context.Context, userID, name string) (ports.Provider, config.ProviderConfig, error) {
	cfg, err := s.Credential(ctx, userID, name)
	if err != nil {
		return nil, config.ProviderConfig{}, err
	}
	p, err := llm.NewProvider(cfg)
	if err != nil {
		return nil, config.ProviderConfig{}, platform.Wrap(platform.KindUnavailable, "provider.build", "cannot initialise provider", err)
	}
	return p, cfg, nil
}

// Discover lists models for a draft (or stored) credential.
func (s *Service) Discover(ctx context.Context, userID string, in Input) ([]string, error) {
	cfg := config.ProviderConfig{Name: in.Name, Kind: in.Kind, BaseURL: in.BaseURL, APIKey: in.APIKey}
	if in.APIKey == "" && in.Name != "" {
		stored, err := s.Credential(ctx, userID, strings.ToLower(in.Name))
		if err == nil {
			cfg.APIKey = stored.APIKey
			if cfg.Kind == "" {
				cfg.Kind = stored.Kind
			}
			if cfg.BaseURL == "" {
				cfg.BaseURL = stored.BaseURL
			}
		}
	}
	return providers.ListModels(ctx, cfg)
}

// Test performs a tiny completion so the UI can verify a credential.
func (s *Service) Test(ctx context.Context, userID, name string) (int64, string, error) {
	p, cfg, err := s.Build(ctx, userID, name)
	if err != nil {
		return 0, "", err
	}
	start := time.Now()
	resp, err := p.Chat(ctx, ports.ChatRequest{
		Model: cfg.DefaultModel, MaxTokens: 16, Temperature: 0,
		Messages: []ports.ChatMessage{{Role: "user", Content: "Reply with the single word OK."}},
	})
	if err != nil {
		return 0, "", platform.Wrap(platform.KindUnavailable, "provider.test", "provider test failed", err)
	}
	return time.Since(start).Milliseconds(), resp.Content, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
