package httpapi

import (
	"context"
	"sync"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Hub fans chat events out to WebSocket clients attached to this replica. Because
// it listens on the shared event bus, a client connected to replica A also sees
// turns completed on replica B — the piece that makes multi-instance chat
// coherent without sticky sessions.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]map[*wsWriter]struct{} // session id -> writers
	any   map[*wsWriter]struct{}            // writers not yet bound to a session
	log   interface{ Warn(string, ...any) }
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{
		conns: map[string]map[*wsWriter]struct{}{},
		any:   map[*wsWriter]struct{}{},
		log:   discardLogger{},
	}
}

type discardLogger struct{}

func (discardLogger) Warn(string, ...any) {}

// Register attaches a connection, optionally to a session.
func (h *Hub) Register(w *wsWriter, sessionID string) {
	if h == nil || w == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if sessionID == "" {
		h.any[w] = struct{}{}
		return
	}
	h.bindLocked(w, sessionID)
}

// Bind moves a connection to a session (called once the brain creates it).
func (h *Hub) Bind(w *wsWriter, sessionID string) {
	if h == nil || w == nil || sessionID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.any, w)
	h.bindLocked(w, sessionID)
}

func (h *Hub) bindLocked(w *wsWriter, sessionID string) {
	set, ok := h.conns[sessionID]
	if !ok {
		set = map[*wsWriter]struct{}{}
		h.conns[sessionID] = set
	}
	set[w] = struct{}{}
}

// Unregister removes a connection from every session list.
func (h *Hub) Unregister(w *wsWriter) {
	if h == nil || w == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.any, w)
	for id, set := range h.conns {
		delete(set, w)
		if len(set) == 0 {
			delete(h.conns, id)
		}
	}
}

// Count reports the subscribers of a session (tests/metrics).
func (h *Hub) Count(sessionID string) int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns[sessionID])
}

// Broadcast sends an event to every local subscriber of a session plus clients
// that are not bound yet, so a freshly connected client never misses the first
// turn.
func (h *Hub) Broadcast(sessionID string, event any) {
	if h == nil {
		return
	}
	h.mu.RLock()
	targets := make([]*wsWriter, 0, len(h.conns[sessionID])+len(h.any))
	for w := range h.conns[sessionID] {
		targets = append(targets, w)
	}
	for w := range h.any {
		targets = append(targets, w)
	}
	h.mu.RUnlock()
	for _, w := range targets {
		if err := w.json(event); err != nil {
			h.log.Warn("cannot deliver hub event", "error", err)
		}
	}
}

// Start subscribes to bus events. Remote-only events are forwarded; events
// produced locally are skipped to avoid double delivery.
func (h *Hub) Start(ctx context.Context, bus ports.EventBus) {
	if h == nil || bus == nil {
		return
	}
	bus.Subscribe("chat.completed", func(_ context.Context, event ports.Event) {
		if origin, _ := event.Payload["origin"].(string); origin == platform.InstanceID() {
			return
		}
		if event.SessionID == "" {
			return
		}
		h.Broadcast(event.SessionID, map[string]any{
			"type":       "remote_message",
			"session_id": event.SessionID,
			"agent":      event.Payload["agent"],
			"intent":     event.Payload["intent"],
			"origin":     event.Payload["origin"],
		})
	})
}

// HubStart begins forwarding bus chat events to local WebSocket clients.
func (a *API) HubStart(ctx context.Context) {
	if a != nil && a.Hub != nil {
		a.Hub.Start(ctx, a.Bus)
	}
}
