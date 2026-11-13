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

func TestAnthropicStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":7}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi "}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"there"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu1","name":"device_command"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"device\":\"lamp\","}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"value\":true}"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
			`{"type":"message_stop"}`,
		}
		for _, e := range events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e, e)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()
	p := NewAnthropic(config.ProviderConfig{Name: "anthropic", Kind: "anthropic", BaseURL: srv.URL,
		APIKey: "k", DefaultModel: "claude", Timeout: config.Duration(5e9),
		Models: []config.ModelConfig{{Name: "claude"}}})
	stream, err := p.Stream(context.Background(), ports.ChatRequest{})
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
	if text.String() != "Hi there" {
		t.Fatalf("text=%q", text.String())
	}
	if len(calls) != 1 || calls[0].Name != "device_command" || calls[0].Arguments["value"] != true {
		t.Fatalf("calls: %+v", calls)
	}
	if usage.PromptTokens != 7 || usage.CompletionTokens != 4 || usage.TotalTokens != 11 {
		t.Fatalf("usage: %+v", usage)
	}
}

func TestGeminiStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`{"candidates":[{"content":{"parts":[{"text":"Hel"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`,
			`{"candidates":[{"content":{"parts":[{"text":"lo"},{"functionCall":{"name":"web_search","args":{"query":"x"}}}]}}]}`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()
	p := NewGemini(config.ProviderConfig{Name: "gemini", Kind: "gemini", BaseURL: srv.URL,
		APIKey: "k", DefaultModel: "gemini-test", Timeout: config.Duration(5e9),
		Models: []config.ModelConfig{{Name: "gemini-test"}}})
	stream, err := p.Stream(context.Background(), ports.ChatRequest{Model: "gemini-test"})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var calls []domain.ToolCall
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		text += chunk.Delta
		if len(chunk.ToolCalls) > 0 {
			calls = chunk.ToolCalls
		}
	}
	if text != "Hello" || len(calls) != 1 || calls[0].Name != "web_search" {
		t.Fatalf("text=%q calls=%+v", text, calls)
	}
}
