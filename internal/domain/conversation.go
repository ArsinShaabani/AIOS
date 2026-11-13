package domain

import (
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// MessageRole is the author of a chat message.
type MessageRole string

const (
	MsgSystem    MessageRole = "system"
	MsgUser      MessageRole = "user"
	MsgAssistant MessageRole = "assistant"
	MsgTool      MessageRole = "tool"
)

// ToolCall is an LLM-requested function invocation. Arguments is raw JSON so
// providers can round-trip them untouched.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    string         `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	LatencyMS int64          `json:"latency_ms,omitempty"`
}

// Message is a single turn in a session.
type Message struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"session_id"`
	UserID     string      `json:"user_id"`
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	AgentID    string      `json:"agent_id,omitempty"`
	Provider   string      `json:"provider,omitempty"`
	Model      string      `json:"model,omitempty"`
	TokensIn   int         `json:"tokens_in"`
	TokensOut  int         `json:"tokens_out"`
	LatencyMS  int64       `json:"latency_ms"`
	Cached     bool        `json:"cached,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// Validate prevents empty/garbage turns from reaching storage.
func (m *Message) Validate() error {
	if m.SessionID == "" {
		return platform.Invalid("message.session_required", "session_id is required")
	}
	switch m.Role {
	case MsgSystem, MsgUser, MsgAssistant, MsgTool:
	default:
		return platform.Invalid("message.role_invalid", "unknown role: "+string(m.Role))
	}
	if m.Content == "" && len(m.ToolCalls) == 0 && m.Role != MsgTool {
		return platform.Invalid("message.empty", "content must not be empty")
	}
	return nil
}

// MemoryTier selects the storage strategy for a memory record.
type MemoryTier string

const (
	TierShort    MemoryTier = "short"    // session-scoped working set
	TierLong     MemoryTier = "long"     // durable facts/preferences
	TierSemantic MemoryTier = "semantic" // embedded, searchable
)

// MemoryKind gives retrieval strategies a hint about usefulness.
type MemoryKind string

const (
	KindFact       MemoryKind = "fact"
	KindPreference MemoryKind = "preference"
	KindEpisode    MemoryKind = "episode"
	KindSummary    MemoryKind = "summary"
)

// Memory is a durable agent memory. Embeddings are kept as float32 slices and
// persisted as JSON so the schema stays portable across SQLite/Postgres.
type Memory struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	SessionID  string     `json:"session_id,omitempty"`
	Tier       MemoryTier `json:"tier"`
	Kind       MemoryKind `json:"kind"`
	Content    string     `json:"content"`
	Summary    string     `json:"summary,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Importance float64    `json:"importance"`
	Pinned     bool       `json:"pinned"`
	Source     string     `json:"source,omitempty"`
	Embedding  []float32  `json:"-"`
	UseCount   int        `json:"use_count"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Expired reports whether the memory is past its TTL at time now.
func (m *Memory) Expired(now time.Time) bool {
	return m.ExpiresAt != nil && now.After(*m.ExpiresAt)
}

// Personality tunes how the assistant speaks. Presets are seeded on first boot
// and can be cloned/edited per user.
type Personality struct {
	ID             string    `json:"id"`
	OwnerID        string    `json:"owner_id,omitempty"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Tone           string    `json:"tone"`            // friendly | professional | concise | playful
	Style          string    `json:"style"`           // sentences | bullets | mixed
	Humor          int       `json:"humor"`           // 0..10
	EmotionalLevel int       `json:"emotional_level"` // 0..10
	ResponseLength string    `json:"response_length"` // short | medium | long
	CustomPrompt   string    `json:"custom_prompt,omitempty"`
	IsPreset       bool      `json:"is_preset"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Clamp enforces ranges so a malformed API payload cannot produce a bizarre
// prompt.
func (p *Personality) Clamp() {
	if p.Humor < 0 {
		p.Humor = 0
	}
	if p.Humor > 10 {
		p.Humor = 10
	}
	if p.EmotionalLevel < 0 {
		p.EmotionalLevel = 0
	}
	if p.EmotionalLevel > 10 {
		p.EmotionalLevel = 10
	}
	switch p.ResponseLength {
	case "short", "medium", "long":
	default:
		p.ResponseLength = "medium"
	}
}
