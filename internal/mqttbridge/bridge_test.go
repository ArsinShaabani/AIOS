package mqttbridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

func requestJSON(t *testing.T, req Request) []byte {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodeResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response: %v (%s)", err, raw)
	}
	return resp
}

func TestBridgeSuccess(t *testing.T) {
	e := newBridgeEnv(t, nil)
	e.bridge.Handle(context.Background(), requestJSON(t, Request{
		ID: "req-1", APIKey: e.apiKey, Message: "hello", AgentID: "agent_general",
	}))
	pub, ok := e.client.last()
	if !ok {
		t.Fatal("no reply published")
	}
	if pub.topic != "aios/bridge/reply/req-1" {
		t.Fatalf("reply topic: %s", pub.topic)
	}
	resp := decodeResponse(t, pub.payload)
	if resp.Error != "" || resp.Content != "hello from mqtt" || resp.SessionID != "ses_mqtt" {
		t.Fatalf("response: %+v", resp)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage: %+v", resp.Usage)
	}
	if e.chat.last.UserID != e.user.ID || e.chat.last.Channel != domain.ChannelMQTT {
		t.Fatalf("brain input: %+v", e.chat.last)
	}
}

func TestBridgeAuthFailures(t *testing.T) {
	e := newBridgeEnv(t, nil)
	cases := []struct {
		name string
		key  string
		code string
	}{
		{"missing", "", "mqttbridge.api_key"},
		{"invalid", "aios_wrong", "mqttbridge.api_key"},
	}
	for _, c := range cases {
		e.bridge.Handle(context.Background(), requestJSON(t, Request{ID: c.name, APIKey: c.key, Message: "hi"}))
		pub, _ := e.client.last()
		resp := decodeResponse(t, pub.payload)
		if resp.Code != c.code || resp.Error == "" {
			t.Fatalf("%s: %+v", c.name, resp)
		}
	}

	// Revoked key.
	now := time.Now().UTC()
	if err := e.repos.Tokens.RevokeAPIKey(context.Background(), "k1", e.user.ID, now); err != nil {
		t.Fatal(err)
	}
	e.bridge.Handle(context.Background(), requestJSON(t, Request{ID: "revoked", APIKey: e.apiKey, Message: "hi"}))
	pub, _ := e.client.last()
	if resp := decodeResponse(t, pub.payload); resp.Code != "mqttbridge.api_key_expired" {
		t.Fatalf("revoked: %+v", resp)
	}

	// Expired key on an active account.
	expiredRaw, _ := security.NewSecret("aios")
	past := now.Add(-time.Hour)
	expired := &domain.APIKey{ID: "k2", UserID: e.user.ID, Name: "expired",
		KeyHash: security.HashToken(expiredRaw), Prefix: security.TokenPrefix(expiredRaw),
		Scopes: []string{"chat"}, ExpiresAt: &past, CreatedAt: now}
	if err := e.repos.Tokens.CreateAPIKey(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	e.bridge.Handle(context.Background(), requestJSON(t, Request{ID: "expired", APIKey: expiredRaw, Message: "hi"}))
	pub, _ = e.client.last()
	if resp := decodeResponse(t, pub.payload); resp.Code != "mqttbridge.api_key_expired" {
		t.Fatalf("expired: %+v", resp)
	}
}

func TestBridgeScopesAndDisabledUser(t *testing.T) {
	e := newBridgeEnv(t, nil)
	ctx := context.Background()
	// Key without chat scope.
	scopedRaw, _ := security.NewSecret("aios")
	scoped := &domain.APIKey{ID: "k3", UserID: e.user.ID, Name: "memory-only",
		KeyHash: security.HashToken(scopedRaw), Prefix: security.TokenPrefix(scopedRaw),
		Scopes: []string{"memory:read"}, CreatedAt: time.Now().UTC()}
	if err := e.repos.Tokens.CreateAPIKey(ctx, scoped); err != nil {
		t.Fatal(err)
	}
	e.bridge.Handle(ctx, requestJSON(t, Request{ID: "scope", APIKey: scopedRaw, Message: "hi"}))
	pub, _ := e.client.last()
	if resp := decodeResponse(t, pub.payload); resp.Code != "mqttbridge.scope" {
		t.Fatalf("scope: %+v", resp)
	}

	// Disabled account.
	e.user.Status = "disabled"
	if err := e.repos.Users.Update(ctx, e.user); err != nil {
		t.Fatal(err)
	}
	e.bridge.Handle(ctx, requestJSON(t, Request{ID: "disabled", APIKey: e.apiKey, Message: "hi"}))
	pub, _ = e.client.last()
	if resp := decodeResponse(t, pub.payload); resp.Code != "mqttbridge.user" {
		t.Fatalf("disabled: %+v", resp)
	}
}

func TestBridgeRateLimitAndBrainError(t *testing.T) {
	e := newBridgeEnv(t, func(c *config.MQTTConfig) { c.Bridge.RequestsPerMinute = 1 })
	ctx := context.Background()
	// Burst is at least 5; the sixth request in the same minute is limited.
	for i := 0; i < 5; i++ {
		e.bridge.Handle(ctx, requestJSON(t, Request{ID: "ok", APIKey: e.apiKey, Message: "hi"}))
		if resp := decodeResponse(t, e.client.lastPayload()); resp.Error != "" {
			t.Fatalf("request %d failed: %+v", i, resp)
		}
	}
	e.bridge.Handle(ctx, requestJSON(t, Request{ID: "two", APIKey: e.apiKey, Message: "hi"}))
	if resp := decodeResponse(t, e.client.lastPayload()); resp.Code != "mqttbridge.rate_limited" {
		t.Fatalf("rate limit: %+v", resp)
	}

	e2 := newBridgeEnv(t, nil)
	e2.chat.err = platform.Unavailable("llm.down", "model down")
	e2.bridge.Handle(ctx, requestJSON(t, Request{ID: "err", APIKey: e2.apiKey, Message: "hi"}))
	if resp := decodeResponse(t, e2.client.lastPayload()); resp.Code != "llm.down" || resp.Error == "" {
		t.Fatalf("brain error: %+v", resp)
	}
}

func (f *fakeClient) lastPayload() []byte {
	pub, _ := f.last()
	return pub.payload
}
