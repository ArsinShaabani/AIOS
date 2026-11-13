package ports

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// ErrNotSupported is returned by providers that cannot perform an operation
// (e.g. a chat-only model asked for embeddings).
var ErrNotSupported = errors.New("operation not supported by provider")

// ChatMessage is the provider-neutral chat turn.
type ChatMessage struct {
	Role       domain.MessageRole `json:"role"`
	Content    string             `json:"content"`
	Name       string             `json:"name,omitempty"`
	ToolCalls  []domain.ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

// ToolSpec is a JSON-Schema tool definition sent to the model.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatRequest is the provider-neutral completion request.
type ChatRequest struct {
	Model          string
	Messages       []ChatMessage
	Temperature    float64
	TopP           float64
	MaxTokens      int
	Tools          []ToolSpec
	ToolChoice     string // auto | none | required
	Stream         bool
	Stop           []string
	User           string
	ResponseFormat string // "" | "json"
	Metadata       map[string]string
}

// ChatResponse is a completed, non-streamed turn.
type ChatResponse struct {
	Provider     string            `json:"provider"`
	Model        string            `json:"model"`
	Content      string            `json:"content"`
	ToolCalls    []domain.ToolCall `json:"tool_calls,omitempty"`
	FinishReason string            `json:"finish_reason"`
	Usage        Usage             `json:"usage"`
	// FallbackFrom names the preferred provider when a fallback answered.
	FallbackFrom string `json:"fallback_from,omitempty"`
}

// Chunk is one streamed delta. Exactly one of Delta/ToolCalls/Err is expected.
type Chunk struct {
	Delta     string            `json:"delta,omitempty"`
	ToolCalls []domain.ToolCall `json:"tool_calls,omitempty"`
	Done      bool              `json:"done"`
	Usage     *Usage            `json:"usage,omitempty"`
	Provider  string            `json:"provider,omitempty"`
	Err       error             `json:"-"`
}

// Capabilities describes what a provider/model supports. The brain uses this to
// decide whether to attach tools, stream, or fall back.
type Capabilities struct {
	Tools      bool
	Streaming  bool
	Embeddings bool
	Vision     bool
}

// Provider is implemented by each LLM backend adapter.
type Provider interface {
	Name() string
	Capabilities() Capabilities
	Models() []string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
	Embed(ctx context.Context, model string, inputs []string) ([][]float32, error)
	Ping(ctx context.Context) error
}

// Fallback is one alternative route.
type Fallback struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// RouteDecision is the router's answer for one request. The brain passes it to
// ModelRouter.Chat/Stream so retries/fallbacks stay inside the model layer.
type RouteDecision struct {
	Provider    string     `json:"provider"`
	Model       string     `json:"model"`
	Temperature float64    `json:"temperature"`
	TopP        float64    `json:"top_p"`
	MaxTokens   int        `json:"max_tokens"`
	Fallbacks   []Fallback `json:"fallbacks,omitempty"`
	Reason      string     `json:"reason,omitempty"`
}

// ModelRouter resolves routes and executes completions with fallback, retry,
// circuit breaking and usage accounting.
type ModelRouter interface {
	Resolve(agentID, intent, text string) RouteDecision
	Chat(ctx context.Context, decision RouteDecision, req ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, decision RouteDecision, req ChatRequest) (<-chan Chunk, error)
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
	Providers() []ProviderSummary
}

// ProviderSummary is a redacted provider view for the admin dashboard.
type ProviderSummary struct {
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Enabled      bool         `json:"enabled"`
	Models       []string     `json:"models"`
	Capabilities Capabilities `json:"capabilities"`
	Healthy      bool         `json:"healthy"`
	Breaker      string       `json:"breaker"`
	LastError    string       `json:"last_error,omitempty"`
}
