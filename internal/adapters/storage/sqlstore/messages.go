package sqlstore

import (
	"context"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// MessageRepo implements ports.MessageRepository.
type MessageRepo struct{ db *DB }

func NewMessageRepo(db *DB) *MessageRepo { return &MessageRepo{db: db} }

const messageCols = `id, session_id, user_id, role, content, name, tool_calls, tool_call_id, agent_id,
	provider, model, tokens_in, tokens_out, latency_ms, cached, created_at`

func scanMessage(row rowScanner) (*domain.Message, error) {
	var (
		m       domain.Message
		tools   string
		cached  int
		created string
	)
	if err := row.Scan(&m.ID, &m.SessionID, &m.UserID, &m.Role, &m.Content, &m.Name, &tools, &m.ToolCallID,
		&m.AgentID, &m.Provider, &m.Model, &m.TokensIn, &m.TokensOut, &m.LatencyMS, &cached, &created); err != nil {
		return nil, err
	}
	decodeJSON(tools, &m.ToolCalls)
	m.Cached = cached == 1
	m.CreatedAt = parseTime(created)
	return &m, nil
}

// Create inserts a message.
func (r *MessageRepo) Create(ctx context.Context, m *domain.Message) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO messages (`+messageCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		m.ID, m.SessionID, m.UserID, string(m.Role), m.Content, m.Name, jsonText(m.ToolCalls), m.ToolCallID,
		m.AgentID, m.Provider, m.Model, m.TokensIn, m.TokensOut, m.LatencyMS, b2i(m.Cached), ft(m.CreatedAt))
	return mapErr("message.create", err)
}

// Get loads one message.
func (r *MessageRepo) Get(ctx context.Context, id string) (*domain.Message, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+messageCols+` FROM messages WHERE id = ?`), id)
	m, err := scanMessage(row)
	if err != nil {
		return nil, mapErr("message.get", err)
	}
	return m, nil
}

// ListBySession returns the last `limit` messages in chronological order.
func (r *MessageRepo) ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+messageCols+` FROM messages
		WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`), sessionID, limit)
	if err != nil {
		return nil, mapErr("message.list", err)
	}
	defer rows.Close()
	out := make([]domain.Message, 0, limit)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, mapErr("message.list_scan", err)
		}
		out = append(out, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, mapErr("message.list_iter", err)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ListRecent returns messages newest-first (dashboard/API use).
func (r *MessageRepo) ListRecent(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+messageCols+` FROM messages
		WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`), sessionID, limit)
	if err != nil {
		return nil, mapErr("message.list_recent", err)
	}
	defer rows.Close()
	out := make([]domain.Message, 0, limit)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, mapErr("message.list_recent_scan", err)
		}
		out = append(out, *m)
	}
	return out, mapErr("message.list_recent_iter", rows.Err())
}

// CountBySession counts messages in a session.
func (r *MessageRepo) CountBySession(ctx context.Context, sessionID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM messages WHERE session_id = ?`), sessionID).Scan(&n)
	return n, mapErr("message.count", err)
}

// DeleteBySession removes all messages in a session.
func (r *MessageRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM messages WHERE session_id = ?`), sessionID)
	return mapErr("message.delete", err)
}

// CountMessages returns the global message count.
func (r *MessageRepo) CountMessages(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&n)
	return n, mapErr("message.count_all", err)
}

// searchLike is shared by repositories that support free-text filtering.
func searchLike(q string) string {
	return "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
}
