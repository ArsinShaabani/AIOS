package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

func (a *API) upgrader() websocket.Upgrader {
	allowed := originsSet(a.Config.Server.CORSOrigins)
	return websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser client
			}
			if allowed["*"] {
				return true
			}
			if allowed[origin] {
				return true
			}
			// Same-origin dashboard.
			return strings.HasSuffix(origin, "://"+r.Host) || origin == "http://"+r.Host || origin == "https://"+r.Host
		},
	}
}

// authenticateWS accepts a bearer header or a ?token= query (browsers cannot set
// headers on WebSocket handshakes).
func (a *API) authenticateWS(r *http.Request) (*security.Principal, *domain.User, error) {
	token := bearer(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return nil, nil, platform.Unauthorized("ws.token", "a token is required")
	}
	if strings.HasPrefix(token, "aios_") {
		key, err := a.Repos.Tokens.GetAPIKeyByHash(r.Context(), security.HashToken(token))
		if err != nil {
			return nil, nil, platform.Unauthorized("ws.api_key", "invalid API key")
		}
		if key.RevokedAt != nil {
			return nil, nil, platform.Unauthorized("ws.api_key", "API key revoked")
		}
		user, err := a.Repos.Users.GetByID(r.Context(), key.UserID)
		if err != nil || !user.IsActive() {
			return nil, nil, platform.Unauthorized("ws.user", "account unavailable")
		}
		return &security.Principal{UserID: user.ID, Role: user.Role, Scopes: key.Scopes, Method: "api_key"}, user, nil
	}
	p, err := a.Auth.ParseAccess(token)
	if err != nil {
		return nil, nil, err
	}
	user, err := a.Repos.Users.GetByID(r.Context(), p.UserID)
	if err != nil || !user.IsActive() {
		return nil, nil, platform.Unauthorized("ws.user", "account unavailable")
	}
	p.Role = user.Role
	return p, user, nil
}

type wsEnvelope struct {
	Type          string `json:"type"`
	Message       string `json:"message"`
	SessionID     string `json:"session_id"`
	AgentID       string `json:"agent_id"`
	PersonalityID string `json:"personality_id"`
}

type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsWriter) json(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return w.conn.WriteJSON(v)
}

func (w *wsWriter) binary(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return w.conn.WriteMessage(websocket.BinaryMessage, data)
}

// handleWSChat is the realtime text channel. One turn runs at a time; a new
// message cancels the previous turn (barge-in).
func (a *API) handleWSChat(w http.ResponseWriter, r *http.Request) {
	principal, user, err := a.authenticateWS(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	upgrader := a.upgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	writer := &wsWriter{conn: conn}
	_ = writer.json(map[string]any{"type": "ready", "user_id": principal.UserID, "version": a.Version})

	var (
		turnMu sync.Mutex
		cancel context.CancelFunc
		active bool
	)
	conn.SetReadLimit(64 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })

	for {
		var env wsEnvelope
		if err := conn.ReadJSON(&env); err != nil {
			turnMu.Lock()
			if cancel != nil {
				cancel()
			}
			turnMu.Unlock()
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		switch env.Type {
		case "cancel", "interrupt":
			turnMu.Lock()
			if cancel != nil {
				cancel()
			}
			turnMu.Unlock()
			_ = writer.json(map[string]any{"type": "cancelled"})
		case "ping":
			_ = writer.json(map[string]any{"type": "pong", "time": time.Now().UTC()})
		case "message", "":
			if strings.TrimSpace(env.Message) == "" {
				continue
			}
			turnMu.Lock()
			if active && cancel != nil {
				cancel() // barge-in
			}
			ctx, turnCancel := context.WithCancel(context.Background())
			cancel = turnCancel
			active = true
			turnMu.Unlock()

			go func(env wsEnvelope, ctx context.Context) {
				defer func() {
					turnMu.Lock()
					active = false
					turnMu.Unlock()
					turnCancel()
				}()
				input := brain.ChatInput{
					UserID: user.ID, SessionID: env.SessionID, Message: env.Message,
					AgentID: env.AgentID, PersonalityID: env.PersonalityID,
					Channel: domain.ChannelWeb, Stream: true,
				}
				_, err := a.Brain.Chat(ctx, input, func(ev brain.Event) error {
					return writer.json(ev)
				})
				if err != nil {
					_ = writer.json(map[string]any{"type": "error", "error": platform.AsError(err).Message})
				}
			}(env, ctx)
		default:
			_ = writer.json(map[string]any{"type": "error", "error": "unknown message type"})
		}
	}
}
