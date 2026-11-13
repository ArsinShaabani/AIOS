package llm

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/cache"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestRouterResolveRulesAndFallbacks(t *testing.T) {
	enabled := true
	cfg := config.LLMConfig{
		DefaultProvider: "primary", DefaultModel: "p-model", Temperature: 0.7, TopP: 0.9, MaxTokens: 500,
		MaxConcurrency: 2, MaxRetries: 0,
		Providers: []config.ProviderConfig{
			{Name: "primary", Enabled: &enabled, Priority: 10},
			{Name: "fallback", Enabled: &enabled, Priority: 20},
		},
		Routing: []config.RoutingRule{
			{Name: "research", Intents: []string{"research"}, MaxTokens: 2048, Fallback: []string{"fallback"}},
		},
	}
	providers := map[string]ports.Provider{"primary": &fakeProvider{name: "primary"}, "fallback": &fakeProvider{name: "fallback"}}
	r := NewRouter(cfg, providers, cache.NewMemory(100, time.Minute), &usageSink{}, nil, nil)

	d := r.Resolve("agent_general", "research", "compare two approaches")
	if d.Provider != "primary" || d.MaxTokens != 2048 || d.Reason != "routing:research" {
		t.Fatalf("unexpected route: %+v", d)
	}
	if len(d.Fallbacks) != 1 || d.Fallbacks[0].Provider != "fallback" {
		t.Fatalf("fallback missing: %+v", d.Fallbacks)
	}
	if def := r.Resolve("", "greeting", "hello"); def.Provider != "primary" || def.Reason != "default" {
		t.Fatalf("default route: %+v", def)
	}
}

func TestRouterChatFallbackAndUsage(t *testing.T) {
	enabled := true
	cfg := config.LLMConfig{
		DefaultProvider: "down", DefaultModel: "m", MaxTokens: 100, MaxConcurrency: 1, MaxRetries: 0,
		Providers: []config.ProviderConfig{
			{Name: "down", Enabled: &enabled, Priority: 1},
			{Name: "up", Enabled: &enabled, Priority: 2},
		},
	}
	down := &fakeProvider{name: "down", failures: 10}
	up := &fakeProvider{name: "up", content: "from fallback"}
	sink := &usageSink{}
	r := NewRouter(cfg, map[string]ports.Provider{"down": down, "up": up},
		cache.NewMemory(100, time.Minute), sink, nil, nil)

	resp, err := r.Chat(context.Background(), r.Resolve("", "", "hi"), ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: domain.MsgUser, Content: "hi"}},
		Metadata: map[string]string{"user_id": "u1", "session_id": "s1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Provider != "up" || resp.Content != "from fallback" {
		t.Fatalf("fallback not used: %+v", resp)
	}
	if sink.count() != 1 || sink.recs[0].UserID != "u1" {
		t.Fatalf("usage not recorded: %+v", sink.recs)
	}
	// Drive enough failures to open the breaker (threshold is 5).
	for i := 0; i < 5; i++ {
		_, _ = r.Chat(context.Background(), r.Resolve("", "", "hi"), ports.ChatRequest{})
	}
	if b := r.breaker("down"); b == nil || b.State() == "closed" {
		t.Fatalf("breaker should open after repeated failures, state=%v", r.breaker("down").State())
	}
}

func TestRouterStreamFallback(t *testing.T) {
	enabled := true
	cfg := config.LLMConfig{
		DefaultProvider: "bad", DefaultModel: "m", MaxTokens: 100, MaxConcurrency: 1, MaxRetries: 0,
		Providers: []config.ProviderConfig{
			{Name: "bad", Enabled: &enabled, Priority: 1},
			{Name: "good", Enabled: &enabled, Priority: 2},
		},
	}
	bad := &fakeProvider{name: "bad", streamErr: platform.Unavailable("bad.stream", "boom")}
	good := &fakeProvider{name: "good"}
	r := NewRouter(cfg, map[string]ports.Provider{"bad": bad, "good": good},
		cache.NewMemory(10, time.Minute), &usageSink{}, nil, nil)
	stream, err := r.Stream(context.Background(), r.Resolve("", "", "hi"), ports.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var done bool
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		text += chunk.Delta
		done = done || chunk.Done
	}
	if text != "hello" || !done {
		t.Fatalf("stream content=%q done=%v", text, done)
	}
}

func TestRouterEmbedCacheAndFallback(t *testing.T) {
	enabled := true
	cfg := config.LLMConfig{
		DefaultProvider: "chat", DefaultModel: "m", EmbeddingProvider: "chat", EmbeddingModel: "emb",
		MaxConcurrency: 2, MaxTokens: 10,
		Providers: []config.ProviderConfig{
			{Name: "chat", Enabled: &enabled, Priority: 1},
			{Name: "embed", Enabled: &enabled, Priority: 2},
		},
	}
	chat := &fakeProvider{name: "chat"}
	embed := &fakeProvider{name: "embed", embed: []float32{1, 2, 3}}
	r := NewRouter(cfg, map[string]ports.Provider{"chat": chat, "embed": embed},
		cache.NewMemory(100, time.Minute), &usageSink{}, nil, nil)

	vecs, err := r.Embed(context.Background(), []string{"hello", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("embeddings: %+v", vecs)
	}
	if embed.embedCalls != 1 {
		t.Fatalf("expected one embed call (cache+dedupe), got %d", embed.embedCalls)
	}
	if _, err := r.Embed(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestRouterUnknownProvider(t *testing.T) {
	enabled := true
	cfg := config.LLMConfig{
		DefaultProvider: "gone", DefaultModel: "m", MaxConcurrency: 1, MaxTokens: 10,
		Providers: []config.ProviderConfig{{Name: "gone", Enabled: &enabled}},
	}
	r := NewRouter(cfg, map[string]ports.Provider{}, cache.NewMemory(10, time.Minute), nil, nil, nil)
	if _, err := r.Chat(context.Background(), r.Resolve("", "", "hi"), ports.ChatRequest{}); err == nil {
		t.Fatal("chat without any provider must fail")
	}
	if _, err := r.Stream(context.Background(), r.Resolve("", "", "hi"), ports.ChatRequest{}); err == nil {
		t.Fatal("stream without any provider must fail")
	}
	if _, err := r.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("embed without any provider must fail")
	}
}
