package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// ensureAffected converts a zero-row update/delete into a NotFound error.
func ensureAffected(res sql.Result, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return mapErr(op, err)
	}
	if n == 0 {
		return platform.NotFound(op+".not_found", "resource not found")
	}
	return nil
}

// SessionRepo implements ports.SessionRepository.
type SessionRepo struct{ db *DB }

func NewSessionRepo(db *DB) *SessionRepo { return &SessionRepo{db: db} }

const sessionCols = `id, user_id, title, personality_id, agent_id, channel, metadata, message_count, created_at, updated_at`

func scanSession(row rowScanner) (*domain.Session, error) {
	var (
		s       domain.Session
		meta    string
		created string
		updated string
	)
	if err := row.Scan(&s.ID, &s.UserID, &s.Title, &s.PersonalityID, &s.AgentID, &s.Channel, &meta, &s.MessageCount, &created, &updated); err != nil {
		return nil, err
	}
	decodeJSON(meta, &s.Metadata)
	s.CreatedAt = parseTime(created)
	s.UpdatedAt = parseTime(updated)
	return &s, nil
}

// Create inserts a session.
func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	now := time.Now().UTC()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.Channel == "" {
		s.Channel = domain.ChannelAPI
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO sessions (`+sessionCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		s.ID, s.UserID, s.Title, s.PersonalityID, s.AgentID, string(s.Channel), jsonText(s.Metadata),
		s.MessageCount, ft(s.CreatedAt), ft(s.UpdatedAt))
	return mapErr("session.create", err)
}

// Get loads a session.
func (r *SessionRepo) Get(ctx context.Context, id string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+sessionCols+` FROM sessions WHERE id = ?`), id)
	s, err := scanSession(row)
	if err != nil {
		return nil, mapErr("session.get", err)
	}
	return s, nil
}

// Update rewrites mutable session fields.
func (r *SessionRepo) Update(ctx context.Context, s *domain.Session) error {
	s.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE sessions SET title = ?, personality_id = ?, agent_id = ?,
		metadata = ?, updated_at = ? WHERE id = ?`),
		s.Title, s.PersonalityID, s.AgentID, jsonText(s.Metadata), ft(s.UpdatedAt), s.ID)
	if err != nil {
		return mapErr("session.update", err)
	}
	return ensureAffected(res, "session.update")
}

// Delete removes a session and its messages.
func (r *SessionRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM sessions WHERE id = ?`), id)
	if err != nil {
		return mapErr("session.delete", err)
	}
	return ensureAffected(res, "session.delete")
}

// ListByUser returns a page of sessions ordered by recency.
func (r *SessionRepo) ListByUser(ctx context.Context, userID string, page platform.Page) ([]domain.Session, int, error) {
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`), userID).Scan(&total); err != nil {
		return nil, 0, mapErr("session.count", err)
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+sessionCols+` FROM sessions WHERE user_id = ?
		ORDER BY updated_at DESC LIMIT ? OFFSET ?`), userID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, mapErr("session.list", err)
	}
	defer rows.Close()
	out := make([]domain.Session, 0, page.Limit)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, 0, mapErr("session.list_scan", err)
		}
		out = append(out, *s)
	}
	return out, total, mapErr("session.list_iter", rows.Err())
}

// Touch updates recency and the message counter after a turn.
func (r *SessionRepo) Touch(ctx context.Context, id string, messageDelta int, at time.Time) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE sessions SET message_count = message_count + ?, updated_at = ? WHERE id = ?`),
		messageDelta, ft(at), id)
	if err != nil {
		return mapErr("session.touch", err)
	}
	return ensureAffected(res, "session.touch")
}

// Count returns the total number of sessions.
func (r *SessionRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n, mapErr("session.count_all", err)
}
