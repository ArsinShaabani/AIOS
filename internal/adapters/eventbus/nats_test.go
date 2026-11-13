package eventbus

import (
	"context"
	"log/slog"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func startNATS(t *testing.T) string {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}
	t.Cleanup(srv.Shutdown)
	return srv.ClientURL()
}

func TestNATSPublishSubscribe(t *testing.T) {
	url := startNATS(t)
	bus, err := NewNATS(config.EventsConfig{Driver: "nats", URL: url,
		SubjectPrefix: "test.events", Timeout: config.Duration(2 * time.Second)}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Stop()
	if !bus.Connected() {
		t.Fatal("not connected")
	}
	exact := make(chan ports.Event, 1)
	wildcard := make(chan ports.Event, 1)
	bus.Subscribe("chat.completed", func(ctx context.Context, event ports.Event) { exact <- event })
	bus.Subscribe("*", func(ctx context.Context, event ports.Event) { wildcard <- event })
	bus.Start(context.Background())

	bus.Publish(context.Background(), ports.Event{Name: "chat.completed", UserID: "u1",
		Payload: map[string]any{"tokens": 12}})
	select {
	case event := <-exact:
		if event.UserID != "u1" || event.OccurredAt.IsZero() || event.Payload["tokens"].(float64) != 12 {
			t.Fatalf("event: %+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exact subscriber did not receive event")
	}
	select {
	case event := <-wildcard:
		if event.Name != "chat.completed" {
			t.Fatalf("wildcard: %+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("wildcard subscriber did not receive event")
	}
	// A malformed message on the wire is ignored, not fatal.
	bus.conn.Publish(bus.subject("chat.completed"), []byte("{bad"))
	_ = bus.conn.FlushTimeout(time.Second)
}

func TestNATSSubjectAndErrors(t *testing.T) {
	bus := &NATS{prefix: "aios.events", log: slog.Default()}
	if got := bus.subject("*"); got != "aios.events.>" {
		t.Fatalf("wildcard subject: %s", got)
	}
	if got := bus.subject("Chat Completed!"); got != "aios.events.chat_completed_" {
		t.Fatalf("sanitized subject: %s", got)
	}
	if got := bus.subject(""); got != "aios.events.>" {
		t.Fatalf("empty subject: %s", got)
	}
	if _, err := NewNATS(config.EventsConfig{URL: "nats://127.0.0.1:1",
		SubjectPrefix: "x", Timeout: config.Duration(time.Second)}, slog.Default()); err == nil {
		t.Fatal("unreachable NATS accepted")
	}
	// Nil/closed buses are safe no-ops.
	var nilBus *NATS
	nilBus.Publish(context.Background(), ports.Event{Name: "x"})
	nilBus.Subscribe("x", func(context.Context, ports.Event) {})
	nilBus.Stop()
	if nilBus.Connected() {
		t.Fatal("nil bus connected")
	}
	bus.Stop()
}
