package ports

import (
	"context"
	"time"
)

// Event is an in-process domain event. The bus is the CQRS seam: today
// subscribers run in-process, later the same events can be published to
// NATS/Redis Streams without touching producers.
type Event struct {
	Name       string         `json:"name"`
	OccurredAt time.Time      `json:"occurred_at"`
	UserID     string         `json:"user_id,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// Publisher emits events.
type Publisher interface {
	Publish(ctx context.Context, event Event)
}

// Subscriber consumes events.
type Subscriber interface {
	Subscribe(name string, handler func(ctx context.Context, event Event))
}

// EventBus combines both directions.
type EventBus interface {
	Publisher
	Subscriber
}

// Clock exists so tests can freeze time without sleeping.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
