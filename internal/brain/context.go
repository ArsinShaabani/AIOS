package brain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/personality"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// ContextInput carries everything needed to assemble a prompt.
type ContextInput struct {
	User        *domain.User
	Session     *domain.Session
	Personality *domain.Personality
	Agent       *domain.Agent
	Query       string
	Intent      Intent
	Channel     domain.Channel
}

// BuiltContext is the assembled prompt plus diagnostics for the prompt viewer.
type BuiltContext struct {
	Messages        []ports.ChatMessage
	System          string
	MemoryIDs       []string
	HistoryCount    int
	EstimatedTokens int
}

// Builder assembles prompts within a strict token budget.
type Builder struct {
	memory     *memory.Service
	messages   ports.MessageRepository
	cfg        config.MemoryConfig
	basePrompt string
}

// NewBuilder builds the context assembler.
func NewBuilder(mem *memory.Service, messages ports.MessageRepository, cfg config.MemoryConfig, basePrompt string) *Builder {
	return &Builder{memory: mem, messages: messages, cfg: cfg, basePrompt: basePrompt}
}

// Build assembles system + memory + history for one turn.
func (b *Builder) Build(ctx context.Context, in ContextInput) (*BuiltContext, error) {
	var memBlock strings.Builder
	memIDs := make([]string, 0, 8)
	if b.memory != nil && in.User != nil {
		memories, err := b.memory.Recall(ctx, in.User.ID, in.Query, b.cfg.RetrievalTopK)
		if err != nil {
			// Memory is an enhancement, never a hard dependency.
			memories = nil
		}
		if len(memories) > 0 {
			memBlock.WriteString("Relevant memories about the user (use them naturally; do not recite them):\n")
			used := 0
			for _, m := range memories {
				if used > b.cfg.MaxContextTokens/3*4 {
					break
				}
				line := fmt.Sprintf("- [%s] %s\n", m.Kind, m.Content)
				memBlock.WriteString(line)
				used += len([]rune(line))
				memIDs = append(memIDs, m.ID)
			}
		}
	}

	runtime := runtimeInfo(in)
	system := personality.SystemPrompt(b.basePrompt, in.Personality, in.Agent.SystemPrompt, memBlock.String(), runtime)
	out := &BuiltContext{System: system, MemoryIDs: memIDs}
	out.Messages = append(out.Messages, ports.ChatMessage{Role: domain.MsgSystem, Content: system})

	if in.Session != nil && b.messages != nil {
		limit := b.cfg.MaxShortTurns * 2
		if limit <= 0 {
			limit = 40
		}
		rows, err := b.messages.ListBySession(ctx, in.Session.ID, limit)
		if err != nil {
			return nil, err
		}
		history := make([]ports.ChatMessage, 0, len(rows))
		for _, m := range rows {
			switch m.Role {
			case domain.MsgUser:
				history = append(history, ports.ChatMessage{Role: domain.MsgUser, Content: m.Content})
			case domain.MsgAssistant:
				if strings.TrimSpace(m.Content) == "" {
					continue // pure tool-call turn; results are not useful later
				}
				history = append(history, ports.ChatMessage{Role: domain.MsgAssistant, Content: m.Content})
			}
		}
		for len(history) > 0 && estimateTokens(system, history) > b.cfg.MaxContextTokens {
			// Drop the oldest exchange (user + assistant) to protect the budget.
			if len(history) > 1 {
				history = history[2:]
			} else {
				history = history[1:]
			}
		}
		out.Messages = append(out.Messages, history...)
		out.HistoryCount = len(history)
	}
	if len(out.Messages) > 1 && out.Messages[len(out.Messages)-1].Role == domain.MsgUser &&
		strings.TrimSpace(out.Messages[len(out.Messages)-1].Content) == "" {
		out.Messages = out.Messages[:len(out.Messages)-1]
	}
	out.EstimatedTokens = estimateTokens(system, out.Messages[1:])
	return out, nil
}

func runtimeInfo(in ContextInput) string {
	now := time.Now().UTC()
	loc := "UTC"
	if in.User != nil && in.User.Timezone != "" {
		loc = in.User.Timezone
	}
	local := now
	if parsed, err := time.LoadLocation(loc); err == nil {
		local = now.In(parsed)
	}
	channel := in.Channel
	if channel == "" {
		channel = domain.ChannelAPI
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Runtime: UTC %s; user timezone %s (local %s); channel %s.",
		now.Format(time.RFC3339), loc, local.Format("2006-01-02 15:04"), channel)
	if in.User != nil && in.User.DisplayName != "" {
		fmt.Fprintf(&b, " The user's display name is %s.", in.User.DisplayName)
	}
	if in.Intent.Name != "" {
		fmt.Fprintf(&b, " Detected intent: %s.", in.Intent.Name)
	}
	b.WriteString(" If the user shares a durable preference or personal fact, call memory_save. " +
		"Use tools for live data; never fabricate tool results.")
	return b.String()
}

func estimateTokens(system string, messages []ports.ChatMessage) int {
	total := len([]rune(system))/4 + 8
	for _, m := range messages {
		total += len([]rune(m.Content))/4 + 6
	}
	return total
}
