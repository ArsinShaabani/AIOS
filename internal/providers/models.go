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

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// ListModels asks an OpenAI-compatible, Anthropic, Gemini or Ollama endpoint for
// the models it exposes. It is shared by the admin router and per-user BYOK.
func ListModels(ctx context.Context, cfg config.ProviderConfig) ([]string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, platform.Invalid("provider.base_url", "base_url is required")
	}
	if cfg.Kind != "ollama" {
		// OpenAI-compatible base URLs end in /v1; keep a single slash.
		baseURL = strings.TrimRight(baseURL, "/")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var (
		req  *http.Request
		err  error
		body map[string]any
	)
	switch cfg.Kind {
	case "ollama":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	case "anthropic":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
		if err == nil {
			req.Header.Set("x-api-key", cfg.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	case "gemini":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models?key="+cfg.APIKey, nil)
	default:
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
		if err == nil && cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
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
