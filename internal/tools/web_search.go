package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func (w *WebTools) searchSearx(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	base := strings.TrimRight(w.cfg.SearxURL, "/")
	if base == "" {
		return nil, platform.Invalid("tools.searx", "searxng URL is not configured")
	}
	endpoint := fmt.Sprintf("%s/search?q=%s&format=json&safesearch=1", base, url.QueryEscape(query))
	body, _, err := w.client.Get(ctx, endpoint, 2<<20)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, platform.Wrap(platform.KindUnavailable, "tools.searx_parse", "invalid searx response", err)
	}
	out := make([]SearchResult, 0, limit)
	for _, r := range payload.Results {
		if len(out) >= limit {
			break
		}
		out = append(out, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}

// searchDuckDuckGo parses the lightweight HTML endpoint, which needs no API key
// and keeps the default install free.
func (w *WebTools) searchDuckDuckGo(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	endpoint := "https://lite.duckduckgo.com/lite/?q=" + url.QueryEscape(query)
	body, _, err := w.client.Get(ctx, endpoint, 1<<20)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, platform.Wrap(platform.KindUnavailable, "tools.ddg_parse", "cannot parse search results", err)
	}
	results := make([]SearchResult, 0, limit)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			text := strings.TrimSpace(nodeText(n))
			if href != "" && text != "" && strings.HasPrefix(href, "http") &&
				!strings.Contains(href, "duckduckgo.com") && len([]rune(text)) > 3 {
				results = append(results, SearchResult{Title: text, URL: href})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(results) == 0 {
		return nil, platform.Unavailable("tools.ddg_empty", "search returned no results (the provider may be rate-limiting)")
	}
	return results, nil
}

func (w *WebTools) searchWikipedia(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	lang := "en"
	if containsPersian(query) {
		lang = "fa"
	}
	endpoint := fmt.Sprintf("https://%s.wikipedia.org/w/api.php?action=opensearch&format=json&limit=%d&search=%s",
		lang, limit, url.QueryEscape(query))
	body, _, err := w.client.Get(ctx, endpoint, 1<<20)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || len(raw) < 4 {
		return nil, platform.Unavailable("tools.wiki", "invalid wikipedia response")
	}
	var titles, snippets, links []string
	_ = json.Unmarshal(raw[1], &titles)
	_ = json.Unmarshal(raw[2], &snippets)
	_ = json.Unmarshal(raw[3], &links)
	out := make([]SearchResult, 0, len(titles))
	for i := range titles {
		if i >= limit {
			break
		}
		r := SearchResult{Title: titles[i]}
		if i < len(links) {
			r.URL = links[i]
		}
		if i < len(snippets) {
			r.Snippet = snippets[i]
		}
		out = append(out, r)
	}
	return out, nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func containsPersian(s string) bool {
	for _, r := range s {
		if r >= 0x0600 && r <= 0x06FF {
			return true
		}
	}
	return false
}

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func intArg(args map[string]any, key string, fallback int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return fallback
}
