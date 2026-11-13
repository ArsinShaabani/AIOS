package mqttbridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
)

func TestRequestTopicConsumerGroup(t *testing.T) {
	base := newBridgeEnv(t, nil)
	cases := []struct{ group, want string }{
		{"", "aios/bridge/chat"},
		{"aios-core", "$share/aios-core/aios/bridge/chat"},
	}
	for _, c := range cases {
		env := newBridgeEnv(t, func(cfg *config.MQTTConfig) { cfg.Bridge.ConsumerGroup = c.group })
		if got := env.bridge.requestTopic(); got != c.want {
			t.Fatalf("group %q: %s want %s", c.group, got, c.want)
		}
	}
	if got := base.bridge.requestTopic(); got != "aios/bridge/chat" {
		t.Fatalf("default: %s", got)
	}
	// Start must subscribe to the shared filter.
	env := newBridgeEnv(t, func(cfg *config.MQTTConfig) { cfg.Bridge.ConsumerGroup = "grp" })
	if err := env.bridge.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if env.client.topic != "$share/grp/aios/bridge/chat" {
		t.Fatalf("subscribed: %s", env.client.topic)
	}
}

func TestReplyTopicRules(t *testing.T) {
	e := newBridgeEnv(t, nil)
	cases := []struct {
		req  Request
		want string
	}{
		{Request{ID: "abc"}, "aios/bridge/reply/abc"},
		{Request{ID: "abc", ReplyTo: "aios/bridge/reply/custom"}, "aios/bridge/reply/custom"},
		{Request{ID: "abc", ReplyTo: "other/topic"}, "aios/bridge/reply/abc"},
		{Request{ID: "abc", ReplyTo: "aios/bridge/reply/+/x"}, "aios/bridge/reply/abc"},
		{Request{ID: "bad/id with spaces#"}, "aios/bridge/reply/badidwithspaces"},
		{Request{ID: ""}, "aios/bridge/reply/"},
	}
	for _, c := range cases {
		got := e.bridge.replyTopic(c.req)
		if c.req.ID == "" {
			if !strings.HasPrefix(got, "aios/bridge/reply/req_") {
				t.Fatalf("generated topic: %s", got)
			}
			continue
		}
		if got != c.want {
			t.Fatalf("replyTopic(%+v)=%s want %s", c.req, got, c.want)
		}
	}
}

func TestPayloadValidation(t *testing.T) {
	small := newBridgeEnv(t, func(c *config.MQTTConfig) { c.Bridge.MaxMessageBytes = 128 })
	ctx := context.Background()
	// Oversized payload is dropped without a reply.
	big, _ := json.Marshal(Request{ID: "big", APIKey: small.apiKey, Message: strings.Repeat("x", 500)})
	small.bridge.Handle(ctx, big)
	if small.client.count() != 0 {
		t.Fatal("oversized payload replied")
	}
	e := newBridgeEnv(t, func(c *config.MQTTConfig) { c.Bridge.MaxMessageBytes = 20000 })
	// Invalid JSON is dropped.
	e.bridge.Handle(ctx, []byte("{not-json"))
	if e.client.count() != 0 {
		t.Fatal("invalid JSON replied")
	}
	// Empty and oversized messages produce explicit errors.
	e.bridge.Handle(ctx, requestJSON(t, Request{ID: "empty", APIKey: e.apiKey}))
	if resp := decodeResponse(t, e.client.lastPayload()); resp.Code != "chat.empty" {
		t.Fatalf("empty: %+v", resp)
	}
	e.bridge.Handle(ctx, requestJSON(t, Request{ID: "long", APIKey: e.apiKey, Message: strings.Repeat("y", 12001)}))
	if resp := decodeResponse(t, e.client.lastPayload()); resp.Code != "chat.too_long" {
		t.Fatalf("long: %+v", resp)
	}
}

func TestStartSubscriptionAndBusy(t *testing.T) {
	e := newBridgeEnv(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.bridge.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if e.client.topic != "aios/bridge/chat" || e.client.handler == nil {
		t.Fatalf("subscription: %+v", e.client.topic)
	}
	// Fill the worker pool; the next message must get a busy reply, not block.
	e.bridge.sem <- struct{}{}
	e.bridge.sem <- struct{}{}
	e.client.handler("aios/bridge/chat", requestJSON(t, Request{ID: "busy", APIKey: e.apiKey, Message: "hi"}))
	if resp := decodeResponse(t, e.client.lastPayload()); resp.Code != "mqttbridge.busy" {
		t.Fatalf("busy: %+v", resp)
	}
	<-e.bridge.sem
	<-e.bridge.sem

	// Subscription errors propagate.
	e2 := newBridgeEnv(t, nil)
	e2.client.subErr = platformUnavailable()
	if err := e2.bridge.Start(ctx); err == nil {
		t.Fatal("subscribe error not propagated")
	}
	// Disabled bridge is a no-op.
	e3 := newBridgeEnv(t, func(c *config.MQTTConfig) { c.Bridge.Enabled = false })
	if err := e3.bridge.Start(ctx); err != nil {
		t.Fatal(err)
	}
}

func platformUnavailable() error {
	return &unavailableError{}
}

type unavailableError struct{}

func (e *unavailableError) Error() string { return "unavailable" }
