// Package eventbus provides the in-memory and NATS-backed EventBus adapters.
// NATS is the horizontal-scaling seam: gateway/brain/voice processes exchange
// the same typed domain events and can be deployed independently.
package eventbus

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// NATS is a reconnecting event bus backed by NATS core subjects.
type NATS struct {
	conn   *nats.Conn
	prefix string
	log    *slog.Logger
	mu     sync.Mutex
	subs   []*nats.Subscription
}

var _ ports.EventBus = (*NATS)(nil)

// NewNATS connects to the broker and verifies connectivity.
func NewNATS(cfg config.EventsConfig, log *slog.Logger) (*NATS, error) {
	if log == nil {
		log = slog.Default()
	}
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	opts := []nats.Option{
		nats.Name("aios-event-bus"),
		nats.Timeout(timeout),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warn("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("NATS reconnected", "url", c.ConnectedUrl())
		}),
	}
	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, platform.Wrap(platform.KindUnavailable, "events.nats_connect", "cannot connect to NATS", err)
	}
	if err := conn.FlushTimeout(timeout); err != nil {
		conn.Close()
		return nil, platform.Wrap(platform.KindUnavailable, "events.nats_flush", "NATS handshake failed", err)
	}
	prefix := strings.Trim(cfg.SubjectPrefix, ".")
	if prefix == "" {
		prefix = "aios.events"
	}
	return &NATS{conn: conn, prefix: prefix, log: log}, nil
}

func (n *NATS) subject(name string) string {
	if name == "" || name == "*" {
		return n.prefix + ".>"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	clean := strings.Trim(b.String(), ".")
	if clean == "" {
		clean = "event"
	}
	return n.prefix + "." + clean
}

// Publish emits an event asynchronously. Failures are logged so the request path
// never blocks on the bus.
func (n *NATS) Publish(_ context.Context, event ports.Event) {
	if n == nil || n.conn == nil {
		return
	}
	if event.Name == "" {
		event.Name = "event"
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	raw, err := json.Marshal(event)
	if err != nil {
		n.log.Warn("cannot encode event", "name", event.Name, "error", err)
		return
	}
	if err := n.conn.Publish(n.subject(event.Name), raw); err != nil {
		n.log.Warn("cannot publish event", "name", event.Name, "error", err)
	}
}

// Subscribe registers a handler. "*" subscribes to all events.
func (n *NATS) Subscribe(name string, handler func(ctx context.Context, event ports.Event)) {
	if n == nil || n.conn == nil || handler == nil {
		return
	}
	subject := n.subject(name)
	sub, err := n.conn.Subscribe(subject, func(msg *nats.Msg) {
		var event ports.Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		handler(ctx, event)
	})
	if err != nil {
		n.log.Warn("cannot subscribe to event", "subject", subject, "error", err)
		return
	}
	n.mu.Lock()
	n.subs = append(n.subs, sub)
	n.mu.Unlock()
}

// Start is a no-op: the connection is established in NewNATS.
func (n *NATS) Start(context.Context) {}

// Stop unsubscribes, drains in-flight messages and closes the connection.
func (n *NATS) Stop() {
	if n == nil || n.conn == nil {
		return
	}
	n.mu.Lock()
	subs := append([]*nats.Subscription(nil), n.subs...)
	n.subs = nil
	n.mu.Unlock()
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	_ = n.conn.FlushTimeout(2 * time.Second)
	n.conn.Close()
}

// Connected reports broker connectivity (health checks/tests).
func (n *NATS) Connected() bool { return n != nil && n.conn != nil && n.conn.IsConnected() }
