package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

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
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.BaseURL == "" {
		return nil, platform.Invalid("provider.base_url", "base_url is required")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var (
		req  *http.Request
		err  error
		body map[string]any
	)
	switch in.Kind {
	case "ollama":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, in.BaseURL+"/api/tags", nil)
	case "anthropic":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, in.BaseURL+"/v1/models", nil)
		if err == nil {
			req.Header.Set("x-api-key", in.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	case "gemini":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, in.BaseURL+"/models?key="+in.APIKey, nil)
	default: // openai-compatible
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, in.BaseURL+"/models", nil)
		if err == nil && in.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+in.APIKey)
		}
	}
	if err != nil {
		return nil, platform.Invalid("provider.discover", err.Error())
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, platform.Unavailable("provider.discover", "cannot reach provider: "+err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, platform.Unavailable("provider.discover", fmt.Sprintf("provider returned %d: %s", resp.StatusCode, snippet))
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, platform.Invalid("provider.discover", "provider returned non-JSON")
	}
	out := parseModelList(body)
	if len(out) == 0 {
		return nil, platform.NotFound("provider.models", "provider returned no models")
	}
	sort.Strings(out)
	return out, nil
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
