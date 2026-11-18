package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserProvidersAndOpenAITunnel(t *testing.T) {
	e := newEnv(t)
	var seenModel, seenAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer up-secret-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/chat/completions":
			seenAuth = r.Header.Get("Authorization")
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			seenModel, _ = body["model"].(string)
			fmt.Fprint(w, `{"id":"x","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"سلام از تانل"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	status, created := e.do("PUT", "/api/v1/me/providers/my-openai", e.token, map[string]any{
		"kind": "openai", "base_url": upstream.URL + "/v1", "api_key": "up-secret-123",
		"default_model": "test-model", "models": []string{"test-model"}, "max_tokens": 2048,
	})
	if status != 200 || created["has_api_key"] != true {
		t.Fatalf("provider upsert: %d %v", status, created)
	}
	status, list := e.do("GET", "/api/v1/me/providers", e.token, nil)
	if status != 200 || len(list["items"].([]any)) != 1 {
		t.Fatalf("provider list: %d %v", status, list)
	}
	status, discovered := e.do("POST", "/api/v1/me/providers/discover", e.token, map[string]any{
		"name": "my-openai", "kind": "openai", "base_url": upstream.URL + "/v1",
	})
	if status != 200 || len(discovered["models"].([]any)) != 1 {
		t.Fatalf("discover: %d %v", status, discovered)
	}
	status, tested := e.do("POST", "/api/v1/me/providers/my-openai/test", e.token, nil)
	if status != 200 || tested["ok"] != true {
		t.Fatalf("provider test: %d %v", status, tested)
	}

	// The built-in chat also uses the caller's own provider.
	status, chat := e.do("POST", "/api/v1/chat", e.token, map[string]any{
		"message": "سلام", "provider": "my-openai", "model": "test-model",
	})
	if status != 200 || !strings.Contains(fmt.Sprint(chat["content"]), "تانل") {
		t.Fatalf("personal chat: %d %v", status, chat)
	}

	status, keyResp := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{
		"name": "for-opencode", "provider": "my-openai", "model": "test-model",
	})
	if status != 201 {
		t.Fatalf("api key: %d %v", status, keyResp)
	}
	tunnelKey := keyResp["token"].(string)

	payload := map[string]any{
		"model": "anything-client-sends", "max_tokens": 2048,
		"messages": []map[string]string{{"role": "user", "content": "سلام"}},
	}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", e.server.URL+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+tunnelKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("tunnel: %d %s", resp.StatusCode, body)
	}
	var completion map[string]any
	_ = json.Unmarshal(body, &completion)
	choice := completion["choices"].([]any)[0].(map[string]any)
	if choice["message"].(map[string]any)["content"] != "سلام از تانل" {
		t.Fatalf("tunnel content: %s", body)
	}
	if seenAuth != "Bearer up-secret-123" || seenModel != "test-model" {
		t.Fatalf("upstream saw auth=%q model=%q", seenAuth, seenModel)
	}

	req, _ = http.NewRequest("GET", e.server.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tunnelKey)
	resp, err = e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "test-model") {
		t.Fatalf("models: %d %s", resp.StatusCode, body)
	}
	if status, _ := e.do("POST", "/v1/chat/completions", "", payload); status != 401 {
		t.Fatalf("unauthenticated tunnel must be 401, got %d", status)
	}

	status, reg := e.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "second@test.local", "password": "SecondPass123!", "display_name": "Second",
	})
	if status != 201 {
		t.Fatalf("register second: %d %v", status, reg)
	}
	secondToken := reg["tokens"].(map[string]any)["access_token"].(string)
	status, list = e.do("GET", "/api/v1/me/providers", secondToken, nil)
	if status != 200 || len(list["items"].([]any)) != 0 {
		t.Fatalf("provider isolation: %d %v", status, list)
	}
}

func TestTunnelStreamingAndValidation(t *testing.T) {
	e := newEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"سلام \"}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"دنیا\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2}}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"sse-model"}]}`)
	}))
	defer upstream.Close()

	if status, _ := e.do("PUT", "/api/v1/me/providers/bad", e.token, map[string]any{"kind": "nope"}); status != 400 {
		t.Fatalf("invalid provider must be 400, got %d", status)
	}
	if status, _ := e.do("PUT", "/api/v1/me/providers/bad", e.token, map[string]any{
		"kind": "openai", "base_url": upstream.URL + "/v1", "default_model": "x", "max_tokens": 1,
	}); status != 400 {
		t.Fatalf("invalid limits must be 400, got %d", status)
	}
	status, pbody := e.do("PUT", "/api/v1/me/providers/sse", e.token, map[string]any{
		"kind": "openai", "base_url": upstream.URL + "/v1", "api_key": "sk-test-1234567890",
		"default_model": "sse-model", "models": []string{"sse-model"},
	})
	if status != 200 {
		t.Fatalf("provider: %d %v", status, pbody)
	}
	if status, _ := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{"name": "bad", "provider": "ghost"}); status != 400 {
		t.Fatalf("unknown provider key must be 400, got %d", status)
	}
	status, key := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{"name": "unbound"})
	if status != 201 {
		t.Fatalf("unbound key: %d %v", status, key)
	}
	token := key["token"].(string)

	payload := map[string]any{"model": "sse-model", "stream": true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}}}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", e.server.URL+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "دنیا") || !strings.Contains(string(body), "[DONE]") {
		t.Fatalf("stream: %d %s", resp.StatusCode, body)
	}

	if status, _ := e.do("DELETE", "/api/v1/me/providers/sse", e.token, nil); status != 204 {
		t.Fatalf("delete provider: %d", status)
	}
	req, _ = http.NewRequest("POST", e.server.URL+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = e.client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("deleted provider must yield 502, got %d", resp.StatusCode)
	}
}

func TestUserProviderValidationAndScopes(t *testing.T) {
	e := newEnv(t)
	for _, body := range []map[string]any{
		{"kind": "evil", "base_url": "http://x", "default_model": "m"},
		{"name": "limits", "kind": "openai", "base_url": "http://x", "default_model": "m", "max_tokens": 999999},
	} {
		if status, _ := e.do("PUT", "/api/v1/me/providers/x", e.token, body); status != 400 {
			t.Fatalf("invalid provider must be 400: %v", body)
		}
	}
	if status, _ := e.do("PUT", "/api/v1/me/providers/localollama", e.token, map[string]any{"kind": "ollama", "models": []string{"llama3.2:1b"}}); status != 200 {
		t.Fatalf("ollama defaults: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": "hi", "provider": "ghost"}); status != 400 {
		t.Fatalf("unknown chat provider must be 400, got %d", status)
	}
	status, key := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{"name": "narrow", "scopes": []string{"memory:read"}})
	if status != 201 {
		t.Fatalf("key: %d", status)
	}
	token := key["token"].(string)
	raw, _ := json.Marshal(map[string]any{"model": "x", "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	req, _ := http.NewRequest("POST", e.server.URL+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("scope without chat must be 403, got %d", resp.StatusCode)
	}
	if status, _ := e.do("POST", "/api/v1/admin/restore", e.token, map[string]any{"format": "aios-backup", "version": 9}); status != 400 {
		t.Fatalf("bad backup version must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/admin/restore", e.token, map[string]any{"format": "aios-backup", "version": 1, "tables": map[string]any{"evil": []any{}}}); status != 400 {
		t.Fatalf("unknown backup table must be 400, got %d", status)
	}
}

func TestTunnelToolsAndErrors(t *testing.T) {
	e := newEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			fmt.Fprint(w, `{"data":[{"id":"tool-model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		last, _ := msgs[len(msgs)-1].(map[string]any)
		if last["content"] == "fail" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"boom"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"tea\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`)
	}))
	defer upstream.Close()
	if status, _ := e.do("PUT", "/api/v1/me/providers/tools", e.token, map[string]any{
		"kind": "openai", "base_url": upstream.URL + "/v1", "api_key": "sk-tools-123456",
		"default_model": "tool-model", "models": []string{"tool-model"},
	}); status != 200 {
		t.Fatalf("provider: %d", status)
	}
	status, key := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{
		"name": "tools", "provider": "tools", "model": "tool-model",
	})
	if status != 201 {
		t.Fatalf("key: %d", status)
	}
	token := key["token"].(string)
	call := func(body string) *http.Response {
		req, _ := http.NewRequest("POST", e.server.URL+"/v1/chat/completions", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := call(`{bad json`)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("bad json: %d", resp.StatusCode)
	}
	resp = call(`{"messages":[]}`)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("empty messages: %d", resp.StatusCode)
	}
	resp = call(`{"model":"tool-model","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","description":"d","parameters":{"type":"object"}}}]}`)
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	choices, _ := out["choices"].([]any)
	if resp.StatusCode != 200 || len(choices) == 0 {
		t.Fatalf("tools: %d %v", resp.StatusCode, out)
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if _, ok := msg["tool_calls"]; !ok {
		t.Fatalf("tool calls missing: %v", msg)
	}
	resp = call(`{"messages":[{"role":"user","content":"fail"}]}`)
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("upstream error: %d", resp.StatusCode)
	}
}
