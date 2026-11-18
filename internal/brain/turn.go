package brain

import (
	"context"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type turnState struct {
	input       ChatInput
	user        *domain.User
	session     *domain.Session
	personality *domain.Personality
	agent       *domain.Agent
	intent      Intent
	route       ports.RouteDecision
	router      ports.ModelRouter
	history     []ports.ChatMessage
	started     time.Time
	emit        EmitFunc
	degraded    bool
	notice      string
}

// runTurn executes the model/tool loop and persists every step.
func (e *Engine) runTurn(ctx context.Context, st turnState) (*ChatOutput, error) {
	var (
		finalContent  strings.Builder
		allCalls      []domain.ToolCall
		usage         ports.Usage
		lastMessageID string
	)
	specs := e.Tools.Specs(st.agent.Tools)
	maxIterations := e.Config.LLM.MaxToolIterations
	if maxIterations < 1 {
		maxIterations = 1
	}
	history := st.history

	for iteration := 0; iteration < maxIterations; iteration++ {
		req := ports.ChatRequest{
			Messages:        history,
			Temperature:     st.route.Temperature,
			TopP:            st.route.TopP,
			MaxTokens:       st.route.MaxTokens,
			ReasoningEffort: st.route.ReasoningEffort,
			Metadata:        map[string]string{"user_id": st.user.ID, "session_id": st.session.ID},
		}
		if iteration == 0 && len(specs) > 0 {
			req.Tools = specs
			req.ToolChoice = "auto"
		}

		var (
			text  string
			calls []domain.ToolCall
			u     ports.Usage
			err   error
		)
		if st.input.Stream && st.emit != nil {
			var used string
			text, calls, u, used, err = e.streamCompletion(ctx, st.router, st.route, req, st.emit)
			if used != "" {
				if used != st.route.Provider {
					st.degraded = true
					st.notice = "preferred provider " + st.route.Provider + " failed; answered with " + used
				}
				st.route.Provider = used
			}
		} else {
			var resp *ports.ChatResponse
			resp, err = st.router.Chat(ctx, st.route, req)
			if resp != nil {
				text, calls, u = resp.Content, resp.ToolCalls, resp.Usage
				// The router may have answered from a fallback provider; report what
				// actually ran instead of what was requested.
				if resp.Provider != "" {
					if resp.Provider != st.route.Provider {
						st.degraded = true
						st.notice = "preferred provider " + st.route.Provider + " failed; answered with " + resp.Provider
					}
					st.route.Provider = resp.Provider
				}
				if resp.Model != "" {
					st.route.Model = resp.Model
				}
			}
		}
		usage.PromptTokens += u.PromptTokens
		usage.CompletionTokens += u.CompletionTokens
		usage.TotalTokens += u.TotalTokens
		if u.TotalTokens == 0 {
			usage.TotalTokens += u.PromptTokens + u.CompletionTokens
		}

		if err != nil && strings.TrimSpace(text) == "" && len(calls) == 0 {
			if st.emit != nil {
				_ = st.emit(Event{Type: "error", SessionID: st.session.ID, Error: err.Error()})
			}
			return nil, err
		}

		assistantID := platform.NewID("msg")
		if text != "" || len(calls) > 0 {
			msg := &domain.Message{
				ID: assistantID, SessionID: st.session.ID, UserID: st.user.ID, Role: domain.MsgAssistant,
				Content: text, ToolCalls: calls, AgentID: st.agent.ID, Provider: st.route.Provider,
				Model: st.route.Model, TokensIn: u.PromptTokens, TokensOut: u.CompletionTokens,
				LatencyMS: time.Since(st.started).Milliseconds(), CreatedAt: time.Now().UTC(),
			}
			if err := e.Messages.Create(ctx, msg); err != nil {
				e.Log.Warn("cannot persist assistant message", "error", err)
			}
			lastMessageID = assistantID
		}
		finalContent.WriteString(text)
		if len(calls) == 0 {
			break
		}
		history = append(history, ports.ChatMessage{Role: domain.MsgAssistant, Content: text, ToolCalls: calls})
		allCalls = append(allCalls, calls...)
		for i := range calls {
			content, ok := e.executeTool(ctx, st, calls[i])
			if !ok {
				return nil, context.Canceled
			}
			history = append(history, ports.ChatMessage{
				Role: domain.MsgTool, Content: content, Name: calls[i].Name, ToolCallID: calls[i].ID,
			})
		}
	}

	if strings.TrimSpace(finalContent.String()) == "" {
		fallback := "I could not produce a complete answer. Please try rephrasing the request."
		finalContent.WriteString(fallback)
		lastMessageID = platform.NewID("msg")
		_ = e.Messages.Create(ctx, &domain.Message{
			ID: lastMessageID, SessionID: st.session.ID, UserID: st.user.ID, Role: domain.MsgAssistant,
			Content: fallback, AgentID: st.agent.ID, CreatedAt: time.Now().UTC(),
		})
		if st.emit != nil {
			_ = st.emit(Event{Type: "delta", Delta: fallback})
		}
	}

	_ = e.Sessions.Touch(ctx, st.session.ID, 1, time.Now().UTC())
	out := &ChatOutput{
		SessionID: st.session.ID, MessageID: lastMessageID, Content: finalContent.String(),
		AgentID: st.agent.ID, AgentName: st.agent.Name, Provider: st.route.Provider, Model: st.route.Model,
		Usage: usage, ToolCalls: allCalls, LatencyMS: time.Since(st.started).Milliseconds(), Intent: st.intent.Name,
		Degraded: st.degraded, Notice: st.notice,
	}
	if st.emit != nil {
		_ = st.emit(Event{Type: "done", SessionID: st.session.ID, MessageID: lastMessageID, Usage: &usage,
			Agent: st.agent.ID, Provider: st.route.Provider, Model: st.route.Model,
			Degraded: st.degraded, Notice: st.notice})
	}
	if e.Events != nil {
		e.Events.Publish(ctx, ports.Event{Name: "chat.completed", UserID: st.user.ID, SessionID: st.session.ID,
			Payload: map[string]any{"agent": st.agent.ID, "intent": st.intent.Name,
				"tokens": usage.TotalTokens, "origin": platform.InstanceID()}})
	}
	e.enqueueExtraction(ctx, st.user.ID, st.session.ID, st.input.Message, finalContent.String())
	return out, nil
}
