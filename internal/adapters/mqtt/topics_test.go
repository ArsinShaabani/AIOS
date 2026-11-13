package mqtt

import (
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestTopicContracts(t *testing.T) {
	c := &Client{cfg: config.MQTTConfig{TopicPrefix: "aios"}}
	if got := c.topic("", "devices", "dev1", "cmd"); got != "aios/devices/dev1/cmd" {
		t.Fatalf("topic: %q", got)
	}
	if got := c.topic("home", "x"); got != "home/aios/x" {
		t.Fatalf("prefixed topic: %q", got)
	}
	empty := &Client{cfg: config.MQTTConfig{}}
	if got := empty.topic("", "a"); got != "aios/a" {
		t.Fatalf("default prefix: %q", got)
	}
	tr := NewTransport(c, "")
	if tr.Name() != "mqtt" || !tr.Supports(domain.ProtoMQTT) || !tr.Supports("") || tr.Supports(domain.ProtoHTTP) {
		t.Fatal("transport capabilities wrong")
	}
	device := domain.Device{ID: "dev1", Metadata: map[string]any{"command_topic": "custom/cmd", "telemetry_topic": "custom/telemetry"}}
	if got := tr.commandTopic(device); got != "custom/cmd" {
		t.Fatalf("custom command topic: %q", got)
	}
	if got := tr.telemetryTopic(device); got != "custom/telemetry" {
		t.Fatalf("custom telemetry topic: %q", got)
	}
	plain := domain.Device{ID: "dev1"}
	if got := tr.commandTopic(plain); got != "aios/devices/dev1/cmd" {
		t.Fatalf("default command topic: %q", got)
	}
	if got := tr.telemetryTopic(plain); got != "aios/devices/dev1/telemetry" {
		t.Fatalf("default telemetry topic: %q", got)
	}
	if topicSegment("aios/devices/dev1/status", 2) != "dev1" || topicSegment("x", 9) != "" {
		t.Fatal("topic segment parsing")
	}
	if c.Connected() {
		t.Fatal("nil client must not report connected")
	}
	if tr.Ping(t.Context()) == nil {
		t.Fatal("disconnected transport should fail ping")
	}
}
