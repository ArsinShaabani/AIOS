package brain

import (
	"log/slog"

	"github.com/ArsinShaabani/AIOS/internal/agents"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/personality"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

// Engine is the conversational decision core.
type Engine struct {
	Users         ports.UserRepository
	Sessions      ports.SessionRepository
	Messages      ports.MessageRepository
	Memory        *memory.Service
	Personalities *personality.Engine
	Agents        *agents.Registry
	Tools         *tools.Registry
	Router        ports.ModelRouter
	Queue         ports.Queue
	Events        ports.Publisher
	Audit         *security.Auditor
	Config        config.Config
	Builder       *Builder
	Log           *slog.Logger
}

// ChatInput is one request to the brain.
type ChatInput struct {
	UserID        string         `json:"-"`
	SessionID     string         `json:"session_id"`
	Message       string         `json:"message"`
	AgentID       string         `json:"agent_id,omitempty"`
	PersonalityID string         `json:"personality_id,omitempty"`
	Channel       domain.Channel `json:"channel,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ToolEvent describes a tool invocation inside a stream.
type ToolEvent struct {
	CallID    string         `json:"call_id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    string         `json:"result,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
	LatencyMS int64          `json:"latency_ms,omitempty"`
}

// Event is streamed to WebSocket/SSE clients.
type Event struct {
	Type      string       `json:"type"` // session | delta | tool_start | tool_result | done | error
	SessionID string       `json:"session_id,omitempty"`
	MessageID string       `json:"message_id,omitempty"`
	Delta     string       `json:"delta,omitempty"`
	Tool      *ToolEvent   `json:"tool,omitempty"`
	Usage     *ports.Usage `json:"usage,omitempty"`
	Agent     string       `json:"agent,omitempty"`
	Provider  string       `json:"provider,omitempty"`
	Model     string       `json:"model,omitempty"`
	Error     string       `json:"error,omitempty"`
	Degraded  bool         `json:"degraded,omitempty"`
	Notice    string       `json:"notice,omitempty"`
}

// EmitFunc receives streaming events. Returning an error aborts generation
// (client disconnect / barge-in).
type EmitFunc func(Event) error

// ChatOutput is the final result of a turn.
type ChatOutput struct {
	SessionID string            `json:"session_id"`
	MessageID string            `json:"message_id"`
	Content   string            `json:"content"`
	AgentID   string            `json:"agent_id"`
	AgentName string            `json:"agent_name"`
	Provider  string            `json:"provider"`
	Model     string            `json:"model"`
	Usage     ports.Usage       `json:"usage"`
	ToolCalls []domain.ToolCall `json:"tool_calls,omitempty"`
	LatencyMS int64             `json:"latency_ms"`
	Intent    string            `json:"intent"`
	// Degraded is true when a fallback provider answered instead of the one
	// the router selected (for example the local template because Ollama failed).
	Degraded bool   `json:"degraded,omitempty"`
	Notice   string `json:"notice,omitempty"`
}
