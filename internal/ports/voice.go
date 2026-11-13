package ports

import (
	"context"
	"encoding/json"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// Audio is a chunk of encoded audio plus its container format.
type Audio struct {
	Data       []byte `json:"-"`
	Format     string `json:"format"` // wav | mp3 | opus | pcm16
	SampleRate int    `json:"sample_rate"`
	DurationMS int64  `json:"duration_ms"`
}

// Transcript is the result of speech-to-text.
type Transcript struct {
	Text       string  `json:"text"`
	Language   string  `json:"language,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	IsFinal    bool    `json:"is_final"`
}

// VoiceOptions tunes synthesis.
type VoiceOptions struct {
	Voice  string
	Format string
	Speed  float64
	Style  string
}

// STT converts audio to text. Implementations must honour ctx cancellation so
// barge-in can abort an in-flight transcription.
type STT interface {
	Name() string
	Transcribe(ctx context.Context, audio Audio, language string) (Transcript, error)
}

// TTS converts text to audio.
type TTS interface {
	Name() string
	Synthesize(ctx context.Context, text string, opts VoiceOptions) (Audio, error)
}

// Telemetry is a device state update.
type Telemetry struct {
	DeviceID string         `json:"device_id"`
	Metric   string         `json:"metric"`
	Value    any            `json:"value"`
	Unit     string         `json:"unit,omitempty"`
	At       int64          `json:"at"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// Command is a capability invocation targeted at a device.
type Command struct {
	RequestID  string         `json:"request_id"`
	DeviceID   string         `json:"device_id"`
	Capability string         `json:"capability"`
	Value      any            `json:"value"`
	Args       map[string]any `json:"args,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
	ExpiresAt  int64          `json:"expires_at,omitempty"`
}

// CommandResult reports the outcome of a device command.
type CommandResult struct {
	OK        bool           `json:"ok"`
	Detail    string         `json:"detail,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	LatencyMS int64          `json:"latency_ms"`
}

// DeviceTransport delivers commands and telemetry for one protocol family
// (MQTT, WebSocket, HTTP, TCP, rosbridge).
type DeviceTransport interface {
	Name() string
	Supports(protocol domain.DeviceProtocol) bool
	Send(ctx context.Context, device domain.Device, cmd Command) (CommandResult, error)
	Subscribe(ctx context.Context, handler func(Telemetry)) error
	Ping(ctx context.Context) error
	Close() error
}

// Tool is a capability the brain can call through the model's function-calling
// loop. Tools are the extension point used by agents and plugins.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Permissions []string        `json:"permissions,omitempty"`
	Timeout     int             `json:"timeout_seconds,omitempty"`
	Executor    ToolExecutor    `json:"-"`
}

// ToolExecutor runs a tool invocation.
type ToolExecutor func(ctx context.Context, call ToolCallContext) (ToolResult, error)

// ToolCallContext carries identity and arguments into a tool.
type ToolCallContext struct {
	UserID    string
	SessionID string
	AgentID   string
	Arguments map[string]any
}

// ToolResult is returned to the model as a tool message.
type ToolResult struct {
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
	IsError bool           `json:"is_error"`
}

// ToolRegistry resolves tools by name, honouring per-agent allow-lists.
type ToolRegistry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	List(allowed []string) []Tool
	Specs(allowed []string) []ToolSpec
	Names() []string
}
