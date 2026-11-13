package brain

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// PromptPreview is returned by the admin prompt viewer: the exact system prompt
// and message list that would be sent for the next turn.
type PromptPreview struct {
	SessionID   string           `json:"session_id"`
	AgentID     string           `json:"agent_id"`
	Personality string           `json:"personality_id"`
	Intent      string           `json:"intent"`
	System      string           `json:"system"`
	Messages    []PreviewMessage `json:"messages"`
	MemoryIDs   []string         `json:"memory_ids,omitempty"`
	Tokens      int              `json:"estimated_tokens"`
}

// PreviewMessage exposes one prompt turn without provider-specific fields.
type PreviewMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PreviewPrompt rebuilds the prompt for a session without calling a model.
func (e *Engine) PreviewPrompt(ctx context.Context, userID, sessionID, agentID, personalityID string) (*PromptPreview, error) {
	user, err := e.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	session, err := e.Sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.UserID != userID {
		return nil, platform.Forbidden("prompt.forbidden", "session belongs to another user")
	}
	rows, err := e.Messages.ListBySession(ctx, sessionID, 1)
	if err != nil {
		return nil, err
	}
	query := ""
	if len(rows) > 0 {
		query = rows[len(rows)-1].Content
	}
	intent := Detect(query)
	agent := e.Agents.Select(ctx, firstNonEmpty(agentID, session.AgentID), intent.Name, query)
	p := e.Personalities.Resolve(ctx, firstNonEmpty(personalityID, session.PersonalityID, user.PersonalityID))
	built, err := e.Builder.Build(ctx, ContextInput{
		User: user, Session: session, Personality: p, Agent: agent,
		Query: query, Intent: intent, Channel: session.Channel,
	})
	if err != nil {
		return nil, err
	}
	preview := &PromptPreview{
		SessionID: sessionID, AgentID: agent.ID, Personality: p.ID, Intent: intent.Name,
		System: built.System, MemoryIDs: built.MemoryIDs, Tokens: built.EstimatedTokens,
	}
	for _, m := range built.Messages {
		if m.Role == domain.MsgSystem {
			continue
		}
		preview.Messages = append(preview.Messages, PreviewMessage{Role: string(m.Role), Content: m.Content})
	}
	return preview, nil
}
