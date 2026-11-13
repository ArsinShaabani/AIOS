package mqttbridge

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// Handle processes one request payload. It is exported so tests and other
// transports can reuse the exact authentication and reply semantics.
func (b *Bridge) Handle(ctx context.Context, payload []byte) {
	start := time.Now()
	if maxBytes := b.cfg.Bridge.MaxMessageBytes; maxBytes > 0 && len(payload) > maxBytes {
		b.log.Warn("MQTT bridge payload too large", "bytes", len(payload), "limit", maxBytes)
		return
	}
	var req Request
	if err := json.Unmarshal(payload, &req); err != nil {
		b.log.Warn("invalid MQTT bridge payload", "error", err)
		return
	}
	if req.ID == "" {
		req.ID = platform.NewID("mqtt")
	}
	if strings.TrimSpace(req.Message) == "" {
		b.reply(ctx, b.replyTopic(req), Response{ID: req.ID, Error: "message is required", Code: "chat.empty", At: time.Now().UTC()})
		return
	}
	if len([]rune(req.Message)) > 12000 {
		b.reply(ctx, b.replyTopic(req), Response{ID: req.ID, Error: "message exceeds 12000 characters", Code: "chat.too_long", At: time.Now().UTC()})
		return
	}

	key, user, err := b.authenticate(ctx, req.APIKey)
	if err != nil {
		appErr := platform.AsError(err)
		b.auditResult(ctx, "", "", req.ID, "failure", start, map[string]any{"reason": appErr.Code})
		b.reply(ctx, b.replyTopic(req), Response{ID: req.ID, Error: appErr.Message, Code: appErr.Code, At: time.Now().UTC()})
		return
	}
	if b.limiter != nil && !b.limiter.Allow(key.ID) {
		b.auditResult(ctx, user.ID, key.ID, req.ID, "failure", start, map[string]any{"reason": "rate_limited"})
		b.reply(ctx, b.replyTopic(req), Response{ID: req.ID, Error: "rate limit exceeded", Code: "mqttbridge.rate_limited", At: time.Now().UTC()})
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := b.chat.Chat(callCtx, brain.ChatInput{
		UserID: user.ID, SessionID: req.SessionID, Message: req.Message,
		AgentID: req.AgentID, PersonalityID: req.PersonalityID, Channel: domain.ChannelMQTT,
	}, nil)
	if err != nil {
		appErr := platform.AsError(err)
		b.auditResult(ctx, user.ID, key.ID, req.ID, "failure", start, map[string]any{"reason": appErr.Code})
		b.reply(ctx, b.replyTopic(req), Response{ID: req.ID, Error: appErr.Message, Code: appErr.Code, At: time.Now().UTC()})
		return
	}
	resp := Response{
		ID: req.ID, SessionID: out.SessionID, MessageID: out.MessageID, Content: out.Content,
		AgentID: out.AgentID, Provider: out.Provider, Model: out.Model, Usage: out.Usage,
		LatencyMS: time.Since(start).Milliseconds(), At: time.Now().UTC(),
	}
	b.reply(ctx, b.replyTopic(req), resp)
	b.auditResult(ctx, user.ID, key.ID, req.ID, "success", start, map[string]any{
		"session_id": out.SessionID, "agent": out.AgentID, "tokens": out.Usage.TotalTokens,
	})
}

func (b *Bridge) authenticate(ctx context.Context, apiKey string) (*domain.APIKey, *domain.User, error) {
	if apiKey == "" {
		return nil, nil, platform.Unauthorized("mqttbridge.api_key", "api_key is required")
	}
	row, err := b.tokens.GetAPIKeyByHash(ctx, security.HashToken(apiKey))
	if err != nil {
		return nil, nil, platform.Unauthorized("mqttbridge.api_key", "invalid API key")
	}
	now := time.Now().UTC()
	if row.RevokedAt != nil || (row.ExpiresAt != nil && now.After(*row.ExpiresAt)) {
		return nil, nil, platform.Unauthorized("mqttbridge.api_key_expired", "API key is revoked or expired")
	}
	user, err := b.users.GetByID(ctx, row.UserID)
	if err != nil || !user.IsActive() {
		return nil, nil, platform.Unauthorized("mqttbridge.user", "account is unavailable")
	}
	if !scopeAllows(row.Scopes, user.Role) {
		return nil, nil, platform.Forbidden("mqttbridge.scope", "API key is not allowed to chat")
	}
	return row, user, nil
}

func scopeAllows(scopes []string, role domain.Role) bool {
	if len(scopes) == 0 {
		return security.Can(role, security.PermChat)
	}
	for _, scope := range scopes {
		if scope == string(security.PermChat) || scope == string(security.PermAdmin) {
			return security.Can(role, security.PermChat)
		}
	}
	return false
}

func (b *Bridge) auditResult(ctx context.Context, userID, keyID, requestID, status string, start time.Time, detail map[string]any) {
	if b.audit == nil {
		return
	}
	if detail == nil {
		detail = map[string]any{}
	}
	detail["request_id"] = requestID
	detail["key_id"] = keyID
	detail["latency_ms"] = time.Since(start).Milliseconds()
	b.audit.Log(ctx, userID, "api_key", "mqtt.chat", "bridge", requestID, status, "", "", detail)
}
