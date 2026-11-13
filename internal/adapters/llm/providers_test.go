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
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestAnthropicChatAndEmbeddings(t *testing.T) {
	var sawKey, sawVersion bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = r.Header.Get("x-api-key") == "ak"
		sawVersion = r.Header.Get("anthropic-version") != ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/messages"):
			fmt.Fprint(w, `{"content":[{"type":"text","text":"Hello"},{"type":"tool_use","id":"tu1","name":"device_command","input":{"device":"lamp","capability":"power","value":true}}],"stop_reason":"tool_use","usage":{"input_tokens":20,"output_tokens":5}}`)
		case strings.HasSuffix(r.URL.Path, "/v1/models"):
			fmt.Fprint(w, `{"data":[{"id":"claude-test"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	cfg := config.ProviderConfig{Name: "anthropic", Kind: "anthropic", BaseURL: srv.URL, APIKey: "ak",
		DefaultModel: "claude-test", Timeout: config.Duration(5e9),
		Models: []config.ModelConfig{{Name: "claude-test"}}}
	p := NewAnthropic(cfg)
	resp, err := p.Chat(context.Background(), ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: domain.MsgSystem, Content: "sys"}, {Role: domain.MsgUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawKey || !sawVersion {
		t.Fatal("auth/version headers missing")
	}
	if resp.Content != "Hello" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Arguments["value"] != true {
		t.Fatalf("response: %+v", resp)
	}
	if resp.FinishReason != "tool_calls" || resp.Usage.TotalTokens != 25 {
		t.Fatalf("finish/usage: %+v", resp)
	}
	if _, err := p.Embed(context.Background(), "", []string{"x"}); err == nil {
		t.Fatal("anthropic has no embeddings; expected ErrNotSupported")
	}
	if err := p.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiChatAndEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, ":generateContent"):
			fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi"},{"functionCall":{"name":"schedule_reminder","args":{"text":"tea","at":"10m"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3,"totalTokenCount":11}}`)
		case strings.Contains(r.URL.Path, ":batchEmbedContents"):
			fmt.Fprint(w, `{"embeddings":[{"values":[1,2,3]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	cfg := config.ProviderConfig{Name: "gemini", Kind: "gemini", BaseURL: srv.URL, APIKey: "gk",
		DefaultModel: "gemini-test", Timeout: config.Duration(5e9),
		Models: []config.ModelConfig{{Name: "gemini-test"}}}
	p := NewGemini(cfg)
	resp, err := p.Chat(context.Background(), ports.ChatRequest{
		Model: "gemini-test", Messages: []ports.ChatMessage{{Role: domain.MsgUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hi" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "schedule_reminder" {
		t.Fatalf("response: %+v", resp)
	}
	if resp.Usage.TotalTokens != 11 || resp.FinishReason != "tool_calls" {
		t.Fatalf("usage/finish: %+v", resp)
	}
	vecs, err := p.Embed(context.Background(), "text-embedding-004", []string{"hello"})
	if err != nil || len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("embeddings: %v %+v", err, vecs)
	}
}
