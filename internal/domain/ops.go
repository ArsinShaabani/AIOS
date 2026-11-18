package domain

import (
	"encoding/json"
	"time"
)

// AuditEvent is an immutable security-relevant record.
type AuditEvent struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorType  string         `json:"actor_type"` // user | device | system | api_key
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID string         `json:"resource_id,omitempty"`
	Status     string         `json:"status"` // success | failure
	IP         string         `json:"ip,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// UsageRecord captures billable token/audio usage for analytics and budgets.
type UsageRecord struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	SessionID        string    `json:"session_id,omitempty"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Kind             string    `json:"kind"` // chat | embedding | stt | tts
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	LatencyMS        int64     `json:"latency_ms"`
	CreatedAt        time.Time `json:"created_at"`
}

// UsageTotals is an aggregate used by the dashboard.
type UsageTotals struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	AvgLatencyMS     float64 `json:"avg_latency_ms"`
}

// Setting is a namespaced JSON configuration value (global or per user).
type Setting struct {
	Scope     string          `json:"scope"` // global | user
	ScopeID   string          `json:"scope_id,omitempty"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedBy string          `json:"updated_by,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Task is a durable background job (memory consolidation, reminders, webhooks).
type Task struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"` // pending | running | done | failed | dead
	Priority    int             `json:"priority"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	RunAt       time.Time       `json:"run_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// RefreshToken stores a hashed rotation token. Reuse of a rotated token is
// treated as theft and revokes the whole family.
type RefreshToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TokenHash  string     `json:"-"`
	FamilyID   string     `json:"family_id"`
	ReplacedBy string     `json:"replaced_by,omitempty"`
	UserAgent  string     `json:"user_agent,omitempty"`
	IP         string     `json:"ip,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// APIKey is a programmatic credential with a permission subset.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	Provider   string     `json:"provider,omitempty"`
	Model      string     `json:"model,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
