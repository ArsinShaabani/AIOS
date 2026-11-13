package ports

import (
	"context"
	"encoding/json"
	"time"
)

// Cache is a byte-oriented cache with TTL. Implementations: in-process LRU and
// Redis (for horizontal scaling).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	// Incr atomically increments a counter, setting the TTL on creation.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Ping(ctx context.Context) error
	Close() error
}

// GetJSON is a convenience wrapper around Cache.Get.
func GetJSON[T any](ctx context.Context, c Cache, key string) (T, bool) {
	var zero T
	raw, ok, err := c.Get(ctx, key)
	if err != nil || !ok {
		return zero, false
	}
	if err := json.Unmarshal(raw, &zero); err != nil {
		return zero, false
	}
	return zero, true
}

// SetJSON is a convenience wrapper around Cache.Set.
func SetJSON(ctx context.Context, c Cache, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, raw, ttl)
}

// Job is a unit of background work.
type Job struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Payload  json.RawMessage `json:"payload"`
	Attempts int             `json:"attempts"`
}

// JobHandler executes a job. Returning an error schedules a retry (with
// exponential backoff) until MaxAttempts is reached.
type JobHandler func(ctx context.Context, job Job) error

// QueueStats is exposed on the dashboard and via Prometheus.
type QueueStats struct {
	Depth     int            `json:"depth"`
	Running   int            `json:"running"`
	Processed int64          `json:"processed"`
	Failed    int64          `json:"failed"`
	ByKind    map[string]int `json:"by_kind,omitempty"`
}

// Queue is a durable-ish work queue with at-least-once delivery. The in-memory
// implementation is used on single-node edge boxes; Redis is used when multiple
// AIOS instances share work.
type Queue interface {
	Enqueue(ctx context.Context, job Job, delay time.Duration) error
	Register(kind string, handler JobHandler)
	Start(ctx context.Context)
	Stop(ctx context.Context)
	Stats(ctx context.Context) (QueueStats, error)
}

// VectorPoint is a stored embedding plus filterable payload.
type VectorPoint struct {
	ID      string         `json:"id"`
	UserID  string         `json:"user_id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

// VectorMatch is a similarity hit.
type VectorMatch struct {
	ID      string         `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload,omitempty"`
}

// VectorStore abstracts Qdrant vs. the built-in brute-force index.
type VectorStore interface {
	Upsert(ctx context.Context, points []VectorPoint) error
	Delete(ctx context.Context, ids []string) error
	Search(ctx context.Context, userID string, vector []float32, topK int, minScore float64) ([]VectorMatch, error)
	Count(ctx context.Context) (int, error)
	Ping(ctx context.Context) error
}

// SecretStore abstracts encrypted-at-rest secrets (provider keys, HA tokens).
type SecretStore interface {
	Get(ctx context.Context, name string) (string, error)
	Set(ctx context.Context, name, value string) error
	Delete(ctx context.Context, name string) error
	List(ctx context.Context) ([]string, error)
}
