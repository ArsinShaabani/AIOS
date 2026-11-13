package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// SearchResult is one web search hit.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// WebTools exposes web_search and web_fetch.
type WebTools struct {
	cfg    config.ToolsConfig
	client *WebClient
	log    *slog.Logger
}

// NewWebTools builds the web tool set.
func NewWebTools(cfg config.ToolsConfig, log *slog.Logger) *WebTools {
	if log == nil {
		log = slog.Default()
	}
	return &WebTools{cfg: cfg, client: NewWebClient(cfg), log: log}
}

// Register wires the tools into a registry.
func (w *WebTools) Register(reg *Registry) error {
	err := reg.Register(ports.Tool{
		Name:        "web_search",
		Description: "Search the public web. Returns titles, URLs and snippets. Use for news, facts, comparisons and anything not in memory.",
		Parameters: Schema(map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
			"limit": map[string]any{"type": "integer", "description": "Max results (1-10)", "default": 6},
		}, "query"),
		Permissions: []string{PermInternet},
		Timeout:     25,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			query := stringArg(call.Arguments, "query")
			if query == "" {
				return ports.ToolResult{IsError: true, Content: "query is required"}, nil
			}
			limit := intArg(call.Arguments, "limit", w.cfg.MaxSearchResults)
			results, err := w.Search(ctx, query, limit)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: "search failed: " + err.Error()}, nil
			}
			raw, _ := json.Marshal(results)
			return ports.ToolResult{Content: string(raw), Data: map[string]any{"results": results}}, nil
		},
	})
	if err != nil {
		return err
	}
	return reg.Register(ports.Tool{
		Name:        "web_fetch",
		Description: "Fetch a URL and return readable text (scripts and markup removed). Use after web_search to read a promising page.",
		Parameters: Schema(map[string]any{
			"url": map[string]any{"type": "string", "description": "Absolute http(s) URL"},
		}, "url"),
		Permissions: []string{PermInternet},
		Timeout:     30,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			target := stringArg(call.Arguments, "url")
			if target == "" {
				return ports.ToolResult{IsError: true, Content: "url is required"}, nil
			}
			page, err := w.client.Fetch(ctx, target)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: "fetch failed: " + err.Error()}, nil
			}
			body, _ := json.Marshal(page)
			return ports.ToolResult{Content: string(body), Data: map[string]any{"title": page.Title, "url": page.URL}}, nil
		},
	})
}

// PermInternet is the permission required for outbound web access.
const PermInternet = "internet"

// Search dispatches to the configured provider.
func (w *WebTools) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if !w.cfg.InternetEnabled {
		return nil, platform.Unavailable("tools.internet_disabled", "internet access is disabled in settings")
	}
	if limit <= 0 {
		limit = w.cfg.MaxSearchResults
	}
	if limit > 10 {
		limit = 10
	}
	switch w.cfg.SearchProvider {
	case "searxng":
		return w.searchSearx(ctx, query, limit)
	case "wikipedia":
		return w.searchWikipedia(ctx, query, limit)
	case "mock":
		return []SearchResult{
			{Title: "Mock result for " + query, URL: "https://example.com/mock", Snippet: "Deterministic mock search result used in tests."},
		}, nil
	default:
		return w.searchDuckDuckGo(ctx, query, limit)
	}
}
