package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// RegisterKnowledgeTool adds knowledge_lookup backed by Wikipedia.
func RegisterKnowledgeTool(reg *Registry, client *WebClient) error {
	if client == nil {
		return nil
	}
	return reg.Register(ports.Tool{
		Name:        "knowledge_lookup",
		Description: "Look up an encyclopaedic summary of a topic on Wikipedia. Good for definitions, history and background.",
		Parameters: Schema(map[string]any{
			"topic": map[string]any{"type": "string", "description": "Topic or article title"},
			"lang":  map[string]any{"type": "string", "description": "ISO language code, default en"},
		}, "topic"),
		Permissions: []string{PermInternet},
		Timeout:     20,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			topic := stringArg(call.Arguments, "topic")
			if topic == "" {
				return ports.ToolResult{IsError: true, Content: "topic is required"}, nil
			}
			lang := stringArg(call.Arguments, "lang")
			if lang == "" {
				if containsPersian(topic) {
					lang = "fa"
				} else {
					lang = "en"
				}
			}
			summary, err := wikipediaSummary(ctx, client, lang, topic)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: "lookup failed: " + err.Error()}, nil
			}
			raw, _ := json.Marshal(summary)
			return ports.ToolResult{Content: string(raw), Data: map[string]any{"title": summary.Title, "url": summary.URL}}, nil
		},
	})
}

type wikiSummary struct {
	Title     string `json:"title"`
	Extract   string `json:"extract"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

func wikipediaSummary(ctx context.Context, client *WebClient, lang, topic string) (wikiSummary, error) {
	if len(lang) > 8 || lang == "" {
		lang = "en"
	}
	title := strings.ReplaceAll(strings.TrimSpace(topic), " ", "_")
	summary, err := wikiGetSummary(ctx, client, lang, title)
	if err == nil {
		return summary, nil
	}
	// The exact title may not exist; search for the closest article.
	results, searchErr := wikiSearch(ctx, client, lang, topic, 1)
	if searchErr != nil || len(results) == 0 {
		return wikiSummary{}, err
	}
	title = strings.ReplaceAll(results[0].Title, " ", "_")
	return wikiGetSummary(ctx, client, lang, title)
}

func wikiGetSummary(ctx context.Context, client *WebClient, lang, title string) (wikiSummary, error) {
	endpoint := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s", lang, url.PathEscape(title))
	body, _, err := client.Get(ctx, endpoint, 1<<20)
	if err != nil {
		return wikiSummary{}, err
	}
	var payload struct {
		Title     string `json:"title"`
		Extract   string `json:"extract"`
		Thumbnail struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return wikiSummary{}, platform.Wrap(platform.KindUnavailable, "knowledge.parse", "invalid wikipedia response", err)
	}
	return wikiSummary{
		Title:     payload.Title,
		Extract:   platform.Truncate(payload.Extract, 4000),
		URL:       payload.ContentURLs.Desktop.Page,
		Thumbnail: payload.Thumbnail.Source,
	}, nil
}

func wikiSearch(ctx context.Context, client *WebClient, lang, query string, limit int) ([]SearchResult, error) {
	endpoint := fmt.Sprintf("https://%s.wikipedia.org/w/api.php?action=opensearch&format=json&limit=%d&search=%s",
		lang, limit, url.QueryEscape(query))
	body, _, err := client.Get(ctx, endpoint, 1<<20)
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
