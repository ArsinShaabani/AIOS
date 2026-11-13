package llm

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/cache"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestRouterRetryEmbedFallbackPing(t *testing.T) {
	enabled := true
	cfg := config.LLMConfig{
		DefaultProvider: "flaky", DefaultModel: "m", EmbeddingProvider: "broken", EmbeddingModel: "emb",
		MaxConcurrency: 2, MaxTokens: 10, MaxRetries: 1, Timeout: config.Duration(2 * time.Second),
		Providers: []config.ProviderConfig{
			{Name: "flaky", Enabled: &enabled, Priority: 1},
			{Name: "broken", Enabled: &enabled, Priority: 2},
			{Name: "embed", Enabled: &enabled, Priority: 3},
		},
	}
	flaky := &fakeProvider{name: "flaky", failures: 1, content: "recovered"}
	broken := &fakeProvider{name: "broken", embed: []float32{1}, embedErr: platform.Unavailable("embed", "down")}
	embed := &fakeProvider{name: "embed", embed: []float32{1, 2}}
	r := NewRouter(cfg, map[string]ports.Provider{"flaky": flaky, "broken": broken, "embed": embed},
		cache.NewMemory(10, time.Minute), &usageSink{}, nil, nil)

	resp, err := r.Chat(context.Background(), r.Resolve("", "", "hi"), ports.ChatRequest{})
	if err != nil || resp.Content != "recovered" {
		t.Fatalf("retry failed: %v %+v", err, resp)
	}
	if flaky.chatCalls < 2 {
		t.Fatalf("provider not retried: %d", flaky.chatCalls)
	}
	vecs, err := r.Embed(context.Background(), []string{"hello"})
	if err != nil || len(vecs[0]) != 2 {
		t.Fatalf("embed fallback: %v %+v", err, vecs)
	}
	ping := r.Ping(context.Background())
	if len(ping) != 3 || ping["flaky"] != "ok" {
		t.Fatalf("ping: %+v", ping)
	}
	summaries := r.Providers()
	if len(summaries) != 3 {
		t.Fatalf("summaries: %+v", summaries)
	}
	for _, s := range summaries {
		if s.Name == "" || s.Breaker == "" {
			t.Fatalf("summary: %+v", s)
		}
	}
}
