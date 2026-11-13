package brain

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Chat runs one conversational turn, optionally streaming.
func (e *Engine) Chat(ctx context.Context, in ChatInput, emit EmitFunc) (*ChatOutput, error) {
	start := time.Now()
	in.Message = normalizeSpace(in.Message)
	if in.Message == "" {
		return nil, platform.Invalid("chat.empty", "message must not be empty")
	}
	if len([]rune(in.Message)) > 12000 {
		return nil, platform.Invalid("chat.too_long", "message exceeds 12000 characters")
	}
	user, err := e.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, platform.Unauthorized("chat.user", "unknown user")
	}
	if !user.IsActive() {
		return nil, platform.Forbidden("chat.disabled", "account is disabled")
	}
	session, err := e.resolveSession(ctx, user, in)
	if err != nil {
		return nil, err
	}

	personalityID := firstNonEmpty(in.PersonalityID, session.PersonalityID, user.PersonalityID)
	p := e.Personalities.Resolve(ctx, personalityID)
	intent := Detect(in.Message)
	agent := e.Agents.Select(ctx, in.AgentID, intent.Name, in.Message)
	channel := in.Channel
	if channel == "" {
		channel = session.Channel
	}

	now := time.Now().UTC()
	userMsg := &domain.Message{
		ID: platform.NewID("msg"), SessionID: session.ID, UserID: user.ID,
		Role: domain.MsgUser, Content: in.Message, CreatedAt: now,
	}
	if err := e.Messages.Create(ctx, userMsg); err != nil {
		return nil, err
	}
	_ = e.Sessions.Touch(ctx, session.ID, 1, now)

	built, err := e.Builder.Build(ctx, ContextInput{
		User: user, Session: session, Personality: p, Agent: agent,
		Query: in.Message, Intent: intent, Channel: channel,
	})
	if err != nil {
		return nil, err
	}

	route := e.Router.Resolve(agent.ID, intent.Name, in.Message)
	if emit != nil {
		if err := emit(Event{Type: "session", SessionID: session.ID, Agent: agent.ID, Provider: route.Provider, Model: route.Model}); err != nil {
			return nil, err
		}
	}
	return e.runTurn(ctx, turnState{
		input: in, user: user, session: session, personality: p, agent: agent,
		intent: intent, route: route, history: built.Messages, started: start, emit: emit,
	})
}

func (e *Engine) resolveSession(ctx context.Context, user *domain.User, in ChatInput) (*domain.Session, error) {
	if in.SessionID != "" {
		session, err := e.Sessions.Get(ctx, in.SessionID)
		if err != nil {
			return nil, err
		}
		if session.UserID != user.ID {
			return nil, platform.Forbidden("chat.session_forbidden", "session belongs to another user")
		}
		if in.PersonalityID != "" && session.PersonalityID != in.PersonalityID {
			session.PersonalityID = in.PersonalityID
			_ = e.Sessions.Update(ctx, session)
		}
		return session, nil
	}
	session := &domain.Session{
		ID: platform.NewID("ses"), UserID: user.ID, Channel: in.Channel,
		PersonalityID: firstNonEmpty(in.PersonalityID, user.PersonalityID),
		AgentID:       in.AgentID, Title: platform.Truncate(in.Message, 60), CreatedAt: time.Now().UTC(),
	}
	if session.Channel == "" {
		session.Channel = domain.ChannelAPI
	}
	if err := e.Sessions.Create(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (e *Engine) enqueueExtraction(ctx context.Context, userID, sessionID, userText, assistantText string) {
	if e.Queue == nil || !e.Config.Memory.LongTermEnabled {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"user_id": userID, "session_id": sessionID,
		"user_text": platform.Truncate(userText, 4000), "assistant_text": platform.Truncate(assistantText, 4000),
	})
	if err != nil {
		return
	}
	if err := e.Queue.Enqueue(ctx, ports.Job{ID: platform.NewID("job"), Kind: "memory.extract", Payload: payload}, 0); err != nil {
		e.Log.Debug("cannot enqueue memory extraction", "error", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
