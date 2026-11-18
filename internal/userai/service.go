// Package userai manages per-user BYOK provider credentials and turns them into
// live LLM providers for the OpenAI-compatible tunnel.
package userai

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Service is the per-user AI provider registry.
type Service struct {
	repo    ports.UserProviderRepository
	keyring *security.Keyring
}

// New builds the service.
func New(repo ports.UserProviderRepository, keyring *security.Keyring) *Service {
	return &Service{repo: repo, keyring: keyring}
}

// Input is the create/update payload.
type Input struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	BaseURL       string   `json:"base_url"`
	APIKey        string   `json:"api_key"`
	DefaultModel  string   `json:"default_model"`
	Models        []string `json:"models"`
	Enabled       *bool    `json:"enabled"`
	MaxTokens     int      `json:"max_tokens"`
	ContextWindow int      `json:"context_window"`
}

// View is safe to return to the browser: it never contains the API key.
type View struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	BaseURL       string    `json:"base_url"`
	DefaultModel  string    `json:"default_model"`
	Models        []string  `json:"models"`
	Enabled       bool      `json:"enabled"`
	MaxTokens     int       `json:"max_tokens"`
	ContextWindow int       `json:"context_window"`
	HasKey        bool      `json:"has_api_key"`
	KeyHint       string    `json:"api_key_hint,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func view(p *domain.UserProvider) View {
	v := View{ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL,
		DefaultModel: p.DefaultModel, Models: p.Models, Enabled: p.Enabled,
		HasKey: p.APIKeyEnc != "", MaxTokens: p.MaxTokens, ContextWindow: p.ContextWindow,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	if v.HasKey {
		v.KeyHint = "•••• stored"
	}
	return v
}

// List returns the caller's providers.
func (s *Service) List(ctx context.Context, userID string) ([]View, error) {
	rows, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for i := range rows {
		out = append(out, view(&rows[i]))
	}
	return out, nil
}

// Upsert creates or replaces a provider, encrypting the API key at rest.
func (s *Service) Upsert(ctx context.Context, userID string, in Input) (View, error) {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if !namePattern.MatchString(in.Name) {
		return View{}, platform.Invalid("provider.name", "name must be 1-32 chars: a-z, 0-9, - or _")
	}
	switch in.Kind {
	case "openai", "anthropic", "gemini", "ollama":
	default:
		return View{}, platform.Invalid("provider.kind", "kind must be openai, anthropic, gemini or ollama")
	}
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.BaseURL == "" {
		if in.Kind == "ollama" {
			in.BaseURL = "http://127.0.0.1:11434"
		} else {
			return View{}, platform.Invalid("provider.base_url", "base_url is required")
		}
	}
	if in.Kind != "ollama" && in.APIKey != "" && len(in.APIKey) < 8 {
		return View{}, platform.Invalid("provider.api_key", "api key looks too short")
	}
	models := dedupe(in.Models)
	if in.DefaultModel == "" && len(models) > 0 {
		in.DefaultModel = models[0]
	}
	if in.MaxTokens == 0 {
		in.MaxTokens = 8192
	}
	if in.ContextWindow == 0 {
		in.ContextWindow = 32768
	}
	if in.MaxTokens < 256 || in.MaxTokens > 200000 || in.ContextWindow < 1024 || in.ContextWindow > 2000000 {
		return View{}, platform.Invalid("provider.limits", "invalid token limits")
	}
	if in.DefaultModel == "" && in.Kind != "ollama" {
		return View{}, platform.Invalid("provider.model", "default_model is required")
	}
	if in.DefaultModel != "" && !contains(models, in.DefaultModel) {
		models = append([]string{in.DefaultModel}, models...)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	existing, err := s.repo.Get(ctx, userID, in.Name)
	enc := ""
	if err == nil {
		enc = existing.APIKeyEnc
	} else if !platform.IsNotFound(err) {
		return View{}, err
	}
	if in.APIKey != "" {
		enc, err = s.keyring.Encrypt(in.APIKey)
		if err != nil {
			return View{}, platform.Wrap(platform.KindInternal, "provider.encrypt", "cannot encrypt api key", err)
		}
	}
	now := time.Now().UTC()
	p := &domain.UserProvider{ID: platform.NewID("uprv"), UserID: userID, Name: in.Name,
		Kind: in.Kind, BaseURL: in.BaseURL, DefaultModel: in.DefaultModel, Models: models,
		APIKeyEnc: enc, Enabled: enabled, MaxTokens: in.MaxTokens, ContextWindow: in.ContextWindow, CreatedAt: now, UpdatedAt: now}
	if existing != nil {
		p.ID, p.CreatedAt = existing.ID, existing.CreatedAt
	}
	if err := s.repo.Upsert(ctx, p); err != nil {
		return View{}, err
	}
	return view(p), nil
}

// Delete removes a provider.
func (s *Service) Delete(ctx context.Context, userID, name string) error {
	return s.repo.Delete(ctx, userID, name)
}
