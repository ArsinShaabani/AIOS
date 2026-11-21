package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// upstreamStub records what the tunnel forwarded and returns a canned reply.
type upstreamStub struct {
	mu      sync.Mutex
	status  int
	body    string
	lastReq map[string]any
	lastRaw []byte
}

func (u *upstreamStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		u.mu.Lock()
		u.lastRaw = raw
		u.lastReq = decoded
		status, body := u.status, u.body
		u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
		}
		fmt.Fprint(w, body)
	}
}

// newTunnelEnv wires one user provider pointing at a stub upstream plus an API
// key bound to it, and returns the one-time key token.
func newTunnelEnv(t *testing.T, upstream http.HandlerFunc) (*env, *upstreamStub, string) {
	t.Helper()
	stub := &upstreamStub{body: `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`}
	handler := upstream
	if handler == nil {
		handler = stub.handler()
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	e := newEnv(t)
	if status, body := e.do("PUT", "/api/v1/me/providers/tunnel", e.token, map[string]any{
		"kind": "openai", "base_url": srv.URL + "/v1", "api_key": "sk-tunnel-123456",
		"default_model": "stub-model", "models": []string{"stub-model"}, "max_tokens": 4096,
	}); status != 200 {
		t.Fatalf("provider upsert: %d %v", status, body)
	}
	status, key := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{
		"name": "tunnel", "provider": "tunnel", "model": "stub-model",
	})
	if status != 201 {
		t.Fatalf("api key: %d %v", status, key)
	}
	return e, stub, key["token"].(string)
}

func tunnelCall(t *testing.T, e *env, token, body string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.server.URL+"/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, raw
}

// Agent clients send content as an array of typed parts; the tunnel must accept
// that shape instead of failing the whole turn with 400 body.invalid.
func TestTunnelAcceptsContentParts(t *testing.T) {
	e, stub, token := newTunnelEnv(t, nil)
	resp, body := tunnelCall(t, e, token, `{"messages":[{"role":"user","content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}]}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("content parts: %d %s", resp.StatusCode, body)
	}
	stub.mu.Lock()
	msgs, _ := stub.lastReq["messages"].([]any)
	stub.mu.Unlock()
	if len(msgs) != 1 {
		t.Fatalf("upstream messages: %v", stub.lastReq["messages"])
	}
	if got := msgs[0].(map[string]any)["content"]; got != "hello \nworld" {
		t.Fatalf("content not flattened: %q", got)
	}
}

// max_completion_tokens is the modern OpenAI field; it must not be replaced by
// the 8192 default.
func TestTunnelHonoursMaxCompletionTokens(t *testing.T) {
	e, stub, token := newTunnelEnv(t, nil)
	resp, body := tunnelCall(t, e, token, `{"messages":[{"role":"user","content":"hi"}],"max_completion_tokens":1234}`)
	if resp.StatusCode != 200 {
		t.Fatalf("max_completion_tokens: %d %s", resp.StatusCode, body)
	}
	stub.mu.Lock()
	seen, _ := stub.lastReq["max_tokens"].(float64)
	stub.mu.Unlock()
	if int(seen) != 1234 {
		t.Fatalf("upstream max_tokens = %v, want 1234", stub.lastReq["max_tokens"])
	}
}

// Upstream throttling must surface as 429 (agents back off) rather than 502.
func TestTunnelMapsRateLimit(t *testing.T) {
	e, stub, token := newTunnelEnv(t, nil)
	stub.mu.Lock()
	stub.status = http.StatusTooManyRequests
	stub.body = `{"error":{"message":"Rate limit reached on tokens per minute"}}`
	stub.mu.Unlock()
	resp, body := tunnelCall(t, e, token, `{"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("rate limit response is missing Retry-After")
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.Error.Code != "rate_limit_exceeded" || !strings.Contains(envelope.Error.Message, "tokens per minute") {
		t.Fatalf("rate limit envelope: %s", body)
	}
}

// A provider 413 means the prompt is too large; report it as such.
func TestTunnelMapsRequestTooLarge(t *testing.T) {
	e, stub, token := newTunnelEnv(t, nil)
	stub.mu.Lock()
	stub.status = http.StatusRequestEntityTooLarge
	stub.body = `{"error":{"message":"Request too large for model"}}`
	stub.mu.Unlock()
	resp, body := tunnelCall(t, e, token, `{"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d %s", resp.StatusCode, body)
	}
}

// A streamed tool call must finish with finish_reason "tool_calls".
func TestTunnelStreamToolCallsFinishReason(t *testing.T) {
	upstream := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\":\\\"tea\\\"}\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
	e, _, token := newTunnelEnv(t, upstream)
	resp, body := tunnelCall(t, e, token, `{"stream":true,"messages":[{"role":"user","content":"tea?"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("stream: %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"finish_reason":"tool_calls"`) {
		t.Fatalf("stream finish_reason: %s", body)
	}
	if !strings.Contains(string(body), `"name":"lookup"`) {
		t.Fatalf("stream tool call missing: %s", body)
	}
}

