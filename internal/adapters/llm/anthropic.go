package llm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Anthropic implements the Messages API (Claude).
type Anthropic struct {
	name    string
	baseURL string
	headers map[string]string
	models  []string
	client  *http.Client
}

var _ ports.Provider = (*Anthropic)(nil)

// NewAnthropic builds a Claude provider.
func NewAnthropic(cfg config.ProviderConfig) *Anthropic {
	models := make([]string, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		models = append(models, m.Name)
	}
	if len(models) == 0 && cfg.DefaultModel != "" {
		models = []string{cfg.DefaultModel}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &Anthropic{
		name:    cfg.Name,
		baseURL: base,
		headers: map[string]string{
			"x-api-key":         cfg.APIKey,
			"anthropic-version": "2023-06-01",
			"User-Agent":        "AIOS/1.0",
		},
		models: models,
		client: httpx.NewClient(cfg.Timeout.Std()),
	}
}

func (a *Anthropic) Name() string { return a.name }

func (a *Anthropic) Capabilities() ports.Capabilities {
	return ports.Capabilities{Tools: true, Streaming: true}
}

func (a *Anthropic) Models() []string { return a.models }

type anthropicBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	TopP        float64            `json:"top_p,omitempty"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Anthropic) build(req ports.ChatRequest) anthropicRequest {
	system := strings.Builder{}
	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case domain.MsgSystem:
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(m.Content)
		case domain.MsgTool:
			msgs = append(msgs, anthropicMessage{
				Role:    "user",
				Content: []anthropicBlock{{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}},
			})
		case domain.MsgAssistant:
			blocks := make([]anthropicBlock, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Arguments})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: "(empty)"})
			}
			msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
		default:
			msgs = append(msgs, anthropicMessage{Role: "user", Content: []anthropicBlock{{Type: "text", Text: m.Content}}})
		}
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	out := anthropicRequest{
		Model:       firstOr(req.Model, a.defaultModel()),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		System:      system.String(),
		Messages:    msgs,
	}
	for _, t := range req.Tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: params})
	}
	return out
}

func (a *Anthropic) defaultModel() string {
	if len(a.models) > 0 {
		return a.models[0]
	}
	return "claude-3-5-haiku-latest"
}
