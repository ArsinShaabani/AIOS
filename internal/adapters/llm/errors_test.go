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

func TestProviderErrorBranches(t *testing.T) {
	cases := []struct {
		status int
		check  func(error) bool
		name   string
	}{
		{http.StatusUnauthorized, func(e error) bool { return strings.Contains(e.Error(), "unauthorized") }, "401"},
		{http.StatusForbidden, func(e error) bool { return strings.Contains(e.Error(), "forbidden") }, "403"},
		{http.StatusBadRequest, func(e error) bool { return strings.Contains(e.Error(), "invalid_request") }, "400"},
		{http.StatusInternalServerError, func(e error) bool { return strings.Contains(e.Error(), "unavailable") }, "500"},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			fmt.Fprint(w, `{"error":{"message":"x"}}`)
		}))
		p := NewOpenAI(config.ProviderConfig{Name: "openai", Kind: "openai", BaseURL: srv.URL,
			APIKey: "k", DefaultModel: "m", Timeout: config.Duration(2e9),
			Models: []config.ModelConfig{{Name: "m"}}})
		_, err := p.Chat(context.Background(), ports.ChatRequest{})
		if err == nil || !c.check(err) {
			t.Fatalf("%s: unexpected error %v", c.name, err)
		}
		srv.Close()
	}
	if normaliseFinish("end_turn") != "stop" || normaliseFinish("max_tokens") != "length" ||
		normaliseFinish("tool_use") != "tool_calls" || normaliseFinish("") != "stop" {
		t.Fatal("finish reason mapping wrong")
	}
}

func TestOpenAIToolMessageMapping(t *testing.T) {
	msgs := toOAMessages([]ports.ChatMessage{
		{Role: domain.MsgUser, Content: "hi"},
		{Role: domain.MsgAssistant, ToolCalls: []domain.ToolCall{{ID: "c1", Name: "x", Arguments: map[string]any{"a": 1}}}},
		{Role: domain.MsgTool, ToolCallID: "c1", Name: "x", Content: "result"},
	})
	if len(msgs) != 3 || msgs[0].Content != "hi" || len(msgs[1].ToolCalls) != 1 ||
		!strings.Contains(msgs[1].ToolCalls[0].Function.Arguments, `"a":1`) {
		t.Fatalf("mapping: %+v", msgs)
	}
	cfg := config.ProviderConfig{Name: "anthropic", Kind: "anthropic", BaseURL: "http://x", APIKey: "k",
		DefaultModel: "claude"}
	a := NewAnthropic(cfg)
	built := a.build(ports.ChatRequest{
		Messages: []ports.ChatMessage{
			{Role: domain.MsgSystem, Content: "sys"},
			{Role: domain.MsgUser, Content: "hi"},
			{Role: domain.MsgAssistant, Content: "ok", ToolCalls: []domain.ToolCall{{ID: "t", Name: "x", Arguments: map[string]any{"v": 1}}}},
			{Role: domain.MsgTool, ToolCallID: "t", Content: "res"},
		},
		Tools:     []ports.ToolSpec{{Name: "x", Description: "d"}},
		MaxTokens: 50,
	})
	if built.System != "sys" || len(built.Messages) != 3 || len(built.Tools) != 1 || built.MaxTokens != 50 {
		t.Fatalf("anthropic build: %+v", built)
	}
}
