package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderModelDiscoveryAndOptions(t *testing.T) {
	e := newEnv(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" && r.Header.Get("Authorization") == "Bearer test-key" {
			fmt.Fprint(w, `{"data":[{"id":"mock-a"},{"id":"mock-b"}]}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer mock.Close()

	status, saved := e.do("PUT", "/api/v1/admin/providers/mockx", e.token, map[string]any{
		"kind": "openai", "base_url": mock.URL + "/v1", "api_key": "test-key",
		"models": []string{"mock-a"}, "default_model": "mock-a", "enabled": true,
		"max_tokens": 4096, "context_window": 64000, "temperature": 0.4,
		"top_p": 0.9, "reasoning_effort": "high",
	})
	if status != 200 {
		t.Fatalf("upsert provider: %d %v", status, saved)
	}
	if int(saved["max_tokens"].(float64)) != 4096 || saved["reasoning_effort"] != "high" {
		t.Fatalf("provider knobs not persisted: %v", saved)
	}
	status, list := e.do("GET", "/api/v1/admin/providers", e.token, nil)
	if status != 200 {
		t.Fatalf("list providers: %d", status)
	}
	found := false
	for _, item := range list["items"].([]any) {
		p := item.(map[string]any)
		if p["name"] == "mockx" {
			found = true
			if p["context_window"].(float64) != 64000 || !p["has_api_key"].(bool) {
				t.Fatalf("provider view missing knobs: %v", p)
			}
		}
	}
	if !found {
		t.Fatal("mockx not listed")
	}

	status, discovered := e.do("POST", "/api/v1/admin/providers/discover", e.token, map[string]any{"name": "mockx"})
	if status != 200 || len(discovered["models"].([]any)) != 2 {
		t.Fatalf("discover stored provider: %d %v", status, discovered)
	}
	status, discovered = e.do("POST", "/api/v1/admin/providers/discover?save=true", e.token, map[string]any{
		"name": "mockx", "kind": "openai", "base_url": mock.URL + "/v1", "api_key": "",
	})
	if status != 200 {
		t.Fatalf("discover+save: %d %v", status, discovered)
	}
	status, _ = e.do("DELETE", "/api/v1/admin/providers/mockx", e.token, nil)
	if status != 204 {
		t.Fatalf("delete provider: %d", status)
	}
}

func TestAgentSkillCatalog(t *testing.T) {
	e := newEnv(t)
	status, catalog := e.do("GET", "/api/v1/agents/catalog", e.token, nil)
	if status != 200 || len(catalog["items"].([]any)) < 10 {
		t.Fatalf("catalog: %d %v", status, catalog)
	}
	status, installed := e.do("POST", "/api/v1/agents/install", e.token, map[string]string{"id": "coder"})
	if status != 200 || installed["id"] != "agent_skill_coder" {
		t.Fatalf("install skill: %d %v", status, installed)
	}
	status, agents := e.do("GET", "/api/v1/agents?all=true", e.token, nil)
	if status != 200 {
		t.Fatal("list agents failed")
	}
	found := false
	for _, item := range agents["items"].([]any) {
		if item.(map[string]any)["id"] == "agent_skill_coder" {
			found = true
		}
	}
	if !found {
		t.Fatal("installed skill not listed")
	}
	// Installing again updates instead of duplicating.
	if status, _ := e.do("POST", "/api/v1/agents/install", e.token, map[string]string{"id": "coder"}); status != 200 {
		t.Fatalf("reinstall: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/agents/install", e.token, map[string]string{"id": "nope"}); status != 404 {
		t.Fatalf("unknown skill must 404, got %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/agents/agent_skill_coder", e.token, nil); status != 204 {
		t.Fatalf("delete skill: %d", status)
	}
}

func TestChatOptionsAndAttachments(t *testing.T) {
	e := newEnv(t)
	status, chat := e.do("POST", "/api/v1/chat", e.token, map[string]any{
		"message": "summarise this",
		"options": map[string]any{"max_tokens": 256, "temperature": 0.2, "reasoning_effort": "low", "context_window": 8000},
		"attachments": []map[string]string{
			{"name": "notes.txt", "mime": "text/plain", "text": "attached-secret-content"},
		},
	})
	if status != 200 {
		t.Fatalf("chat with options: %d %v", status, chat)
	}
	if chat["session_id"] == "" || chat["content"] == "" {
		t.Fatalf("empty chat result: %v", chat)
	}
	// Oversized payloads are still rejected, but the old 12k character ceiling is gone.
	status, bigReply := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": strings.Repeat("x", 15000)})
	if status != 200 {
		t.Fatalf("15k message should be accepted, got %d %v", status, bigReply)
	}
	status, _ = e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": strings.Repeat("x", 200001)})
	if status != 400 {
		t.Fatalf("200k+ message must be 400, got %d", status)
	}
}
