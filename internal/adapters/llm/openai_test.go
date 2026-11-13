package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func openAIConfig(base string) config.ProviderConfig {
	return config.ProviderConfig{
		Name: "openai", Kind: "openai", BaseURL: base, APIKey: "test-key",
		DefaultModel: "gpt-test", Timeout: config.Duration(5e9),
		Models: []config.ModelConfig{{Name: "gpt-test", PromptPer1K: 1, CompletionPer1K: 2}},
	}
}

func TestOpenAIChatToolsAndEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/chat/completions":
			fmt.Fprint(w, `{"model":"gpt-test","choices":[{"message":{"role":"assistant","content":"hello","tool_calls":[{"id":"c1","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"x\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`)
		case "/embeddings":
			fmt.Fprint(w, `{"data":[{"index":0,"embedding":[0.1,0.2]},{"index":1,"embedding":[0.3,0.4]}]}`)
		case "/models":
			fmt.Fprint(w, `{"data":[{"id":"gpt-test"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewOpenAI(openAIConfig(srv.URL))
	if p.Name() != "openai" || !p.Capabilities().Tools || !p.Capabilities().Embeddings {
		t.Fatalf("capabilities: %+v", p.Capabilities())
	}
	resp, err := p.Chat(context.Background(), ports.ChatRequest{
		Model: "gpt-test", Messages: []ports.ChatMessage{{Role: domain.MsgUser, Content: "hi"}},
		Tools: []ports.ToolSpec{{Name: "web_search", Description: "search", Parameters: []byte(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "hello" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "web_search" {
		t.Fatalf("chat response: %+v", resp)
	}
	if resp.ToolCalls[0].Arguments["query"] != "x" || resp.FinishReason != "tool_calls" {
		t.Fatalf("tool args: %+v", resp.ToolCalls[0])
	}
	if resp.Usage.TotalTokens != 16 {
		t.Fatalf("usage: %+v", resp.Usage)
	}
	vecs, err := p.Embed(context.Background(), "text-embedding-3-small", []string{"a", "b"})
	if err != nil || len(vecs) != 2 || vecs[1][1] != 0.4 {
		t.Fatalf("embeddings: %v %+v", err, vecs)
	}
	if err := p.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAIStreamAssemblesToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`{"choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"web_search","arguments":"{\"query\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"tea\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAI(openAIConfig(srv.URL))
	stream, err := p.Stream(context.Background(), ports.ChatRequest{Model: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var calls []domain.ToolCall
	var usage ports.Usage
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		text.WriteString(chunk.Delta)
		if len(chunk.ToolCalls) > 0 {
			calls = chunk.ToolCalls
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
	}
	if text.String() != "Hello" {
		t.Fatalf("text=%q", text.String())
	}
	if len(calls) != 1 || calls[0].Name != "web_search" || calls[0].Arguments["query"] != "tea" {
		t.Fatalf("calls: %+v", calls)
	}
	if usage.PromptTokens != 3 || usage.CompletionTokens != 2 {
		t.Fatalf("usage: %+v", usage)
	}
}

func TestOpenAIErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	p := NewOpenAI(openAIConfig(srv.URL))
	_, err := p.Chat(context.Background(), ports.ChatRequest{})
	if !platform.IsUnavailable(err) {
		t.Fatalf("429 must be retryable, got %v", err)
	}
}

func TestLocalProviderHashEmbeddings(t *testing.T) {
	cfg := config.ProviderConfig{Name: "local", Kind: "local", EmbeddingDim: 64,
		Models: []config.ModelConfig{{Name: "local-template"}}}
	p := NewLocal(cfg)
	vecs, err := p.Embed(context.Background(), "", []string{"the quick brown fox", "the quick brown fox", "quantum physics"})
	if err != nil || len(vecs) != 3 || len(vecs[0]) != 64 {
		t.Fatalf("embeddings: %v", err)
	}
	if vecs[0][0] != vecs[1][0] {
		t.Fatal("hash embeddings must be deterministic")
	}
	var dot float64
	for i := range vecs[0] {
		dot += float64(vecs[0][i] * vecs[2][i])
	}
	if dot > 0.9 {
		t.Fatalf("unrelated texts are too similar: %f", dot)
	}
	resp, err := p.Chat(context.Background(), ports.ChatRequest{Messages: []ports.ChatMessage{{Role: domain.MsgUser, Content: "hi"}}})
	if err != nil || !strings.Contains(resp.Content, "Ollama") {
		t.Fatalf("local chat should explain how to enable models: %v", err)
	}
	stream, err := p.Stream(context.Background(), ports.ChatRequest{Messages: []ports.ChatMessage{{Role: domain.MsgUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var out string
	for chunk := range stream {
		out += chunk.Delta
	}
	if out == "" {
		t.Fatal("local stream emitted nothing")
	}
}
