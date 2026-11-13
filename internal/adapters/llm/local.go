// Package llm contains provider adapters (OpenAI-compatible, Anthropic, Gemini,
// Ollama, local) behind the ports.Provider interface. Each adapter is
// independently testable with httptest.
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Local is the always-available fallback provider. It cannot reason, but it
// serves deterministic lexical embeddings (so semantic memory works offline)
// and an honest template reply that explains how to enable a real model.
type Local struct {
	name   string
	dim    int
	models []string
}

var _ ports.Provider = (*Local)(nil)

// NewLocal builds the local provider from configuration.
func NewLocal(cfg config.ProviderConfig) *Local {
	dim := cfg.EmbeddingDim
	if dim <= 0 {
		dim = 384
	}
	models := make([]string, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		models = append(models, m.Name)
	}
	if len(models) == 0 {
		models = []string{"local-template"}
	}
	name := cfg.Name
	if name == "" {
		name = "local"
	}
	return &Local{name: name, dim: dim, models: models}
}

// Name implements ports.Provider.
func (l *Local) Name() string { return l.name }

// Capabilities implements ports.Provider.
func (l *Local) Capabilities() ports.Capabilities {
	return ports.Capabilities{Embeddings: true, Streaming: true, Tools: false}
}

// Models implements ports.Provider.
func (l *Local) Models() []string { return l.models }

// Chat returns a helpful template response.
func (l *Local) Chat(_ context.Context, req ports.ChatRequest) (*ports.ChatResponse, error) {
	content := l.reply(req)
	promptTokens := estimateTokens(req.Messages)
	completionTokens := estimateTokensText(content)
	return &ports.ChatResponse{
		Provider:     l.name,
		Model:        firstOr(req.Model, "local-template"),
		Content:      content,
		FinishReason: "stop",
		Usage: ports.Usage{
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			TotalTokens: promptTokens + completionTokens,
		},
	}, nil
}

// Stream emits the template response in two chunks so clients exercise their
// streaming path without a network model.
func (l *Local) Stream(ctx context.Context, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	content := l.reply(req)
	ch := make(chan ports.Chunk, 4)
	go func() {
		defer close(ch)
		mid := len(content) / 2
		if mid > 0 {
			select {
			case ch <- ports.Chunk{Delta: content[:mid]}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case ch <- ports.Chunk{Delta: content[mid:], Done: true, Usage: &ports.Usage{
			PromptTokens:     estimateTokens(req.Messages),
			CompletionTokens: estimateTokensText(content),
		}}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

// Embed returns deterministic hashed embeddings.
func (l *Local) Embed(_ context.Context, _ string, inputs []string) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i, in := range inputs {
		out[i] = HashEmbedding(in, l.dim)
	}
	return out, nil
}

// Ping always succeeds.
func (l *Local) Ping(context.Context) error { return nil }

func (l *Local) reply(req ports.ChatRequest) string {
	var lastUser, lastTool string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		m := req.Messages[i]
		if m.Role == domain.MsgTool && lastTool == "" {
			lastTool = m.Content
			continue
		}
		if m.Role == domain.MsgUser {
			lastUser = m.Content
			break
		}
	}
	lastUser = strings.TrimSpace(lastUser)
	if lastUser == "" {
		lastUser = "(empty message)"
	}
	var b strings.Builder
	b.WriteString("I'm running AIOS' built-in local template model, so I can't reason about this yet.\n\n")
	fmt.Fprintf(&b, "You said: %q\n\n", platform.Truncate(lastUser, 400))
	if lastTool != "" {
		fmt.Fprintf(&b, "Tool output available: %s\n\n", platform.Truncate(lastTool, 400))
	}
	b.WriteString("To enable real answers, add a provider under Settings → AI (Ollama runs fully offline: `ollama pull qwen2.5:1.5b`).\n")
	b.WriteString("Memory, devices, plugins and voice are already wired; they activate with the model.")
	return b.String()
}
