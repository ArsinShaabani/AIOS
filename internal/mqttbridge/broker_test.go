package mqttbridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"

	aiosmqtt "github.com/ArsinShaabani/AIOS/internal/adapters/mqtt"
	"github.com/ArsinShaabani/AIOS/internal/config"
)

// startBroker runs an in-process MQTT broker on a random port. This exercises
// the real paho client, subscriptions and QoS1 handshakes in CI without Docker.
func startBroker(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	server := mqtt.New(&mqtt.Options{
		Capabilities: mqtt.NewDefaultServerCapabilities(),
		Logger:       slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatal(err)
	}
	if err := server.AddListener(listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr})); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return addr, func() { _ = server.Close() }
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestBridgeOverRealBroker(t *testing.T) {
	addr, stop := startBroker(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	env := newBridgeEnv(t, nil)
	mqttCfg := config.MQTTConfig{
		Enabled: true, Broker: "tcp://" + addr, ClientID: "aios-bridge-test",
		TopicPrefix: "aios", QoS: 1, KeepAlive: 15,
		Bridge: env.cfg.Bridge,
	}
	client, err := aiosmqtt.Connect(ctx, mqttCfg, slog.Default())
	if err != nil {
		t.Fatalf("connect broker: %v", err)
	}
	defer client.Close()
	bridge := New(mqttCfg, client, env.repos.Tokens, env.repos.Users, env.chat, nil, slog.Default())
	if err := bridge.Start(ctx); err != nil {
		t.Fatalf("start bridge: %v", err)
	}

	replies := make(chan Response, 1)
	if err := client.Subscribe(ctx, "aios/bridge/reply/integration", func(_ string, payload []byte) {
		var resp Response
		if json.Unmarshal(payload, &resp) == nil {
			replies <- resp
		}
	}); err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(Request{
		ID: "integration", APIKey: env.apiKey, Message: "hello over mqtt",
		ReplyTo: "aios/bridge/reply/integration",
	})
	if err := client.Publish(ctx, "aios/bridge/chat", req, false); err != nil {
		t.Fatal(err)
	}
	select {
	case resp := <-replies:
		if resp.Error != "" || resp.Content != "hello from mqtt" || resp.SessionID == "" {
			t.Fatalf("response: %+v", resp)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("no MQTT reply received")
	}
}

func TestBridgeOverBrokerRejectsBadKey(t *testing.T) {
	addr, stop := startBroker(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	env := newBridgeEnv(t, nil)
	mqttCfg := config.MQTTConfig{Enabled: true, Broker: "tcp://" + addr, ClientID: "aios-bridge-badkey",
		TopicPrefix: "aios", QoS: 1, KeepAlive: 15, Bridge: env.cfg.Bridge}
	client, err := aiosmqtt.Connect(ctx, mqttCfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	bridge := New(mqttCfg, client, env.repos.Tokens, env.repos.Users, env.chat, nil, slog.Default())
	if err := bridge.Start(ctx); err != nil {
		t.Fatal(err)
	}
	replies := make(chan Response, 1)
	_ = client.Subscribe(ctx, "aios/bridge/reply/bad", func(_ string, payload []byte) {
		var resp Response
		if json.Unmarshal(payload, &resp) == nil {
			replies <- resp
		}
	})
	req, _ := json.Marshal(Request{ID: "bad", APIKey: "aios_wrong", Message: "hi", ReplyTo: "aios/bridge/reply/bad"})
	if err := client.Publish(ctx, "aios/bridge/chat", req, false); err != nil {
		t.Fatal(err)
	}
	select {
	case resp := <-replies:
		if resp.Code != "mqttbridge.api_key" {
			t.Fatalf("bad key response: %+v", resp)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("no rejection received")
	}
}
