package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestWebSearchProvidersAndFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search":
			fmt.Fprint(w, `{"results":[{"title":"AIOS","url":"https://example.com/aios","content":"local AI"}]}`)
		case r.URL.Path == "/lite/":
			fmt.Fprint(w, `<html><body><a href="https://example.com/a">Example A</a><a href="https://duckduckgo.com/x">sponsor</a></body></html>`)
		case r.URL.Path == "/w/api.php":
			fmt.Fprint(w, `["aios",["AIOS"],["AI operating system"],["https://en.wikipedia.org/wiki/AIOS"]]`)
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/summary/"):
			fmt.Fprint(w, `{"title":"AIOS","extract":"An operating system for AI.","content_urls":{"desktop":{"page":"https://en.wikipedia.org/wiki/AIOS"}},"thumbnail":{"source":"https://img/x.png"}}`)
		case strings.HasPrefix(r.URL.Path, "/page"):
			fmt.Fprint(w, `<html><head><title>Docs</title></head><body><script>bad()</script><h1>AIOS</h1><p>Readable text.</p></body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client := clientFor(srv)

	// searxng
	cfg := config.ToolsConfig{InternetEnabled: true, SearchProvider: "searxng", SearxURL: "http://searx.test", MaxSearchResults: 5}
	w := &WebTools{cfg: cfg, client: client}
	results, err := w.Search(context.Background(), "aios", 3)
	if err != nil || len(results) != 1 || results[0].URL != "https://example.com/aios" {
		t.Fatalf("searx: %v %+v", err, results)
	}

	// duckduckgo HTML parsing, including ad filtering
	w.cfg.SearchProvider = "duckduckgo"
	results, err = w.Search(context.Background(), "aios", 5)
	if err != nil || len(results) != 1 || results[0].Title != "Example A" {
		t.Fatalf("ddg: %v %+v", err, results)
	}

	// wikipedia search
	w.cfg.SearchProvider = "wikipedia"
	results, err = w.Search(context.Background(), "aios", 5)
	if err != nil || len(results) != 1 || results[0].Snippet != "AI operating system" {
		t.Fatalf("wiki search: %v %+v", err, results)
	}

	// mock + disabled + empty results
	w.cfg.SearchProvider = "mock"
	if r, err := w.Search(context.Background(), "x", 2); err != nil || len(r) != 1 {
		t.Fatalf("mock search: %v", err)
	}
	w.cfg.InternetEnabled = false
	if _, err := w.Search(context.Background(), "x", 2); err == nil {
		t.Fatal("disabled internet must fail")
	}

	// fetch + extraction
	page, err := client.Fetch(context.Background(), "http://public.test/page")
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Docs" || !strings.Contains(page.Content, "Readable text.") || strings.Contains(page.Content, "bad()") {
		t.Fatalf("fetch: %+v", page)
	}
	if _, err := client.Fetch(context.Background(), "ftp://x"); err == nil {
		t.Fatal("non-http scheme accepted")
	}

	// tool registration and execution
	reg := NewRegistry()
	if err := w.Register(reg); err != nil {
		t.Fatal(err)
	}
	w.cfg.InternetEnabled = true
	w.cfg.SearchProvider = "searxng"
	res, err := reg.Execute(context.Background(), "web_search", ports.ToolCallContext{Arguments: map[string]any{"query": "aios", "limit": 1}})
	if err != nil || !strings.Contains(res.Content, "example.com") {
		t.Fatalf("web_search tool: %v %s", err, res.Content)
	}
	if res, err := reg.Execute(context.Background(), "web_search", ports.ToolCallContext{}); err != nil || !res.IsError {
		t.Fatalf("missing query must be a tool error: %+v %v", res, err)
	}
	res, err = reg.Execute(context.Background(), "web_fetch", ports.ToolCallContext{Arguments: map[string]any{"url": "http://public.test/page"}})
	if err != nil || res.IsError || !strings.Contains(res.Content, "Readable") {
		t.Fatalf("web_fetch tool: %v %+v", err, res)
	}
}

func TestKnowledgeAndNavigationTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/summary/"):
			fmt.Fprint(w, `{"title":"Tea","extract":"Tea is a drink.","content_urls":{"desktop":{"page":"https://en.wikipedia.org/wiki/Tea"}}}`)
		case r.URL.Path == "/w/api.php":
			fmt.Fprint(w, `["tea",["Tea"],["drink"],["https://en.wikipedia.org/wiki/Tea"]]`)
		case r.URL.Path == "/search" && r.URL.Query().Get("format") == "json":
			fmt.Fprint(w, `[{"lat":"35.7","lon":"51.4","display_name":"Tehran, Iran"}]`)
		case strings.HasPrefix(r.URL.Path, "/route/v1/"):
			fmt.Fprint(w, `{"routes":[{"distance":12500.0,"duration":900.0}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client := clientFor(srv)

	reg := NewRegistry()
	if err := RegisterKnowledgeTool(reg, client); err != nil {
		t.Fatal(err)
	}
	res, err := reg.Execute(context.Background(), "knowledge_lookup", ports.ToolCallContext{Arguments: map[string]any{"topic": "Tea"}})
	if err != nil || res.IsError || !strings.Contains(res.Content, "Tea is a drink") {
		t.Fatalf("knowledge: %v %s", err, res.Content)
	}
	if res, _ := reg.Execute(context.Background(), "knowledge_lookup", ports.ToolCallContext{}); !res.IsError {
		t.Fatal("missing topic must be a tool error")
	}

	if err := RegisterNavigationTool(reg, client); err != nil {
		t.Fatal(err)
	}
	res, err = reg.Execute(context.Background(), "navigate_route", ports.ToolCallContext{Arguments: map[string]any{
		"origin": "Tehran", "destination": "Tehran", "mode": "driving",
	}})
	if err != nil || res.IsError {
		t.Fatalf("navigation: %v %+v", err, res)
	}
	var data map[string]any
	_ = json.Unmarshal([]byte(res.Content), &data)
	if data["distance_km"].(float64) != 12.5 {
		t.Fatalf("distance: %+v", data)
	}
	if res, _ := reg.Execute(context.Background(), "navigate_route", ports.ToolCallContext{Arguments: map[string]any{"origin": "x"}}); !res.IsError {
		t.Fatal("missing destination must fail")
	}
}
