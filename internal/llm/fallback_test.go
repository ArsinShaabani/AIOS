package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// A fallback must be visible: the response has to name the provider that
// actually answered, and it must report where it fell back from.
func boolp(b bool) *bool { return &b }

func TestFallbackIsReported(t *testing.T) {
	broken := &fakeProvider{name: "ollama", failures: 1, err: errors.New("connection refused")}
	working := &fakeProvider{name: "local", content: "template answer"}
	cfg := config.LLMConfig{
		DefaultProvider: "ollama",
		MaxRetries:      0,
		MaxConcurrency:  1,
		Providers: []config.ProviderConfig{
			{Name: "ollama", Kind: "ollama", Enabled: boolp(true), BaseURL: "http://127.0.0.1:1"},
			{Name: "local", Kind: "local", Enabled: boolp(true)},
		},
	}
	r := NewRouter(cfg, map[string]ports.Provider{"ollama": broken, "local": working}, nil, nil, nil, nil)
	decision := r.Resolve("", "", "hi")
	if decision.Provider != "ollama" || len(decision.Fallbacks) == 0 {
		t.Fatalf("expected ollama with fallbacks, got %+v", decision)
	}
	resp, err := r.Chat(context.Background(), decision,
		ports.ChatRequest{Messages: []ports.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("fallback should answer: %v", err)
	}
	if resp.Provider != "local" {
		t.Fatalf("response must name the answering provider, got %q", resp.Provider)
	}
	if resp.FallbackFrom != "ollama" {
		t.Fatalf("fallback origin missing: %+v", resp)
	}
	found := false
	for _, p := range r.Providers() {
		if p.Name == "ollama" {
			found = true
			if p.LastError == "" {
				t.Fatal("provider error should be surfaced for diagnostics")
			}
		}
	}
	if !found {
		t.Fatal("ollama provider missing from summaries")
	}
}

func TestProviderErrorClearedAfterSuccess(t *testing.T) {
	p := &fakeProvider{name: "ollama", content: "ok", failures: 1, err: errors.New("boom")}
	cfg := config.LLMConfig{DefaultProvider: "ollama", MaxRetries: 0, MaxConcurrency: 1,
		Providers: []config.ProviderConfig{{Name: "ollama", Kind: "ollama", Enabled: boolp(true)}}}
	r := NewRouter(cfg, map[string]ports.Provider{"ollama": p}, nil, nil, nil, nil)
	if _, err := r.Chat(context.Background(), ports.RouteDecision{Provider: "ollama", Model: "m"},
		ports.ChatRequest{}); err == nil {
		t.Fatal("expected first call to fail")
	}
	if _, err := r.Chat(context.Background(), ports.RouteDecision{Provider: "ollama", Model: "m"},
		ports.ChatRequest{}); err != nil {
		t.Fatalf("second call should succeed: %v", err)
	}
	for _, summary := range r.Providers() {
		if summary.Name == "ollama" && summary.LastError != "" {
			t.Fatalf("last error should be cleared, got %q", summary.LastError)
		}
	}
}
