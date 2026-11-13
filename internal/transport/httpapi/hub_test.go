package httpapi

import (
	"context"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// fakeBus records subscriptions so hub behaviour can be tested deterministically.
type fakeBus struct {
	subs map[string]func(context.Context, ports.Event)
}

func (f *fakeBus) Publish(context.Context, ports.Event) {}
func (f *fakeBus) Subscribe(name string, h func(context.Context, ports.Event)) {
	if f.subs == nil {
		f.subs = map[string]func(context.Context, ports.Event){}
	}
	f.subs[name] = h
}
func (f *fakeBus) Start(context.Context) {}
func (f *fakeBus) Stop()                 {}

func TestHubRemoteEvents(t *testing.T) {
	hub := NewHub()
	bus := &fakeBus{}
	hub.Start(context.Background(), bus)
	handler, ok := bus.subs["chat.completed"]
	if !ok {
		t.Fatal("hub did not subscribe to chat.completed")
	}
	// A locally produced event is ignored (origin == this process).
	handler(context.Background(), ports.Event{Name: "chat.completed", SessionID: "s1",
		Payload: map[string]any{"origin": platform.InstanceID(), "agent": "a"}})
	// A remote event for an unknown local session is harmless.
	handler(context.Background(), ports.Event{Name: "chat.completed", SessionID: "s2",
		Payload: map[string]any{"origin": "other-instance", "agent": "agent_home"}})
	// Missing session id is ignored.
	handler(context.Background(), ports.Event{Name: "chat.completed", Payload: map[string]any{"origin": "x"}})
	// Nil handlers are safe.
	var nilHub *Hub
	nilHub.Start(context.Background(), bus)
	nilHub.Start(context.Background(), nil)
	hub.Start(context.Background(), nil)
}

func TestHubRegisterBindBroadcast(t *testing.T) {
	hub := NewHub()
	if hub.Count("s1") != 0 {
		t.Fatal("fresh hub should be empty")
	}
	var got int
	w := &wsWriter{}
	hub.Register(nil, "s1") // nil-safe
	hub.Register(w, "")     // unbound
	hub.Bind(w, "s1")
	if hub.Count("s1") != 1 {
		t.Fatalf("bound count: %d", hub.Count("s1"))
	}
	hub.Unregister(w)
	if hub.Count("s1") != 0 {
		t.Fatalf("after unregister: %d", hub.Count("s1"))
	}
	hub.Unregister(nil)
	var nilHub *Hub
	nilHub.Register(w, "s")
	nilHub.Broadcast("s", map[string]any{})
	if nilHub.Count("s") != 0 {
		t.Fatal("nil hub count")
	}
	_ = got
}
