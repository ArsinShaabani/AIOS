package providers

import (
	"context"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Discover lists models from a provider using the supplied draft config (or the
// stored key when the draft leaves it blank). It never persists anything.
func (s *Service) Discover(ctx context.Context, in Input) ([]string, error) {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	s.mu.Lock()
	var eBase, eKind, eKey string
	for i := range s.llm.Providers {
		if s.llm.Providers[i].Name == in.Name {
			eBase, eKind, eKey = s.llm.Providers[i].BaseURL, s.llm.Providers[i].Kind, s.llm.Providers[i].APIKey
		}
	}
	s.mu.Unlock()
	if in.BaseURL == "" {
		in.BaseURL = eBase
	}
	if in.Kind == "" {
		in.Kind = eKind
	}
	if in.APIKey == "" {
		in.APIKey = eKey
	}
	if in.Kind == "" {
		return nil, platform.Invalid("provider.kind", "kind is required")
	}
	return ListModels(ctx, config.ProviderConfig{Name: in.Name, Kind: in.Kind, BaseURL: in.BaseURL, APIKey: in.APIKey})
}

func parseModelList(body map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(strings.TrimPrefix(name, "models/"))
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if data, ok := body["data"].([]any); ok { // OpenAI
		for _, item := range data {
			if m, ok := item.(map[string]any); ok {
				if id, ok := m["id"].(string); ok {
					add(id)
				}
			}
		}
	}
	if models, ok := body["models"].([]any); ok { // Ollama + Gemini
		for _, item := range models {
			switch m := item.(type) {
			case string:
				add(m)
			case map[string]any:
				for _, k := range []string{"name", "model", "id"} {
					if v, ok := m[k].(string); ok && v != "" {
						add(v)
						break
					}
				}
			}
		}
	}
	return out
}
