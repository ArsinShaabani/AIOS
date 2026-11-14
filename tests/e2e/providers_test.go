package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderManagement(t *testing.T) {
	e := newEnv(t)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/models":
			fmt.Fprint(w, `{"data":[{"id":"mock-1"}]}`)
		case "/chat/completions":
			fmt.Fprint(w, `{"id":"x","object":"chat.completion","model":"mock-1","choices":[{"index":0,"message":{"role":"assistant","content":"ready"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer llm.Close()

	status, catalog := e.do("GET", "/api/v1/admin/providers/catalog", e.token, nil)
	if status != 200 {
		t.Fatalf("catalog: %d %v", status, catalog)
	}
	found := false
	for _, item := range catalog["items"].([]any) {
		p := item.(map[string]any)
		if p["id"] == "pollinations" && p["key_required"] == false {
			found = true
		}
	}
	if !found {
		t.Fatal("keyless pollinations preset missing")
	}

	status, _ = e.do("PUT", "/api/v1/admin/providers/smoke", e.token, map[string]any{
		"kind": "openai", "base_url": llm.URL, "api_key": "secret-key",
		"models": []string{"mock-1"}, "default_model": "mock-1", "enabled": true,
	})
	if status != 200 {
		t.Fatalf("upsert provider: %d", status)
	}
	status, list := e.do("GET", "/api/v1/admin/providers", e.token, nil)
	if status != 200 {
		t.Fatalf("list: %d", status)
	}
	var smoke map[string]any
	for _, item := range list["items"].([]any) {
		p := item.(map[string]any)
		if p["name"] == "smoke" {
			smoke = p
		}
	}
	if smoke == nil || smoke["has_api_key"] != true || smoke["api_key_hint"] == "" {
		t.Fatalf("provider/key not registered: %v", smoke)
	}
	status, result := e.do("POST", "/api/v1/admin/providers/smoke/test", e.token, map[string]any{})
	if status != 200 || result["ok"] != true {
		t.Fatalf("provider test: %d %v", status, result)
	}
	status, _ = e.do("POST", "/api/v1/admin/providers/default", e.token, map[string]any{"provider": "smoke", "model": "mock-1"})
	if status != 200 {
		t.Fatalf("set default: %d", status)
	}
	// The live router must now use the new provider.
	status, chat := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": "hi"})
	if status != 200 {
		t.Fatalf("chat after provider switch: %d %v", status, chat)
	}
	if status, _ := e.do("DELETE", "/api/v1/admin/providers/smoke", e.token, nil); status != 204 {
		t.Fatalf("delete provider: %d", status)
	}
	status, _ = e.do("PUT", "/api/v1/admin/providers/bad", e.token, map[string]any{"kind": "bogus"})
	if status != 400 {
		t.Fatalf("invalid provider must be 400, got %d", status)
	}
}
