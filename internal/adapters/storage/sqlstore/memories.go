package sqlstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// MemoryRepo implements ports.MemoryRepository.
type MemoryRepo struct{ db *DB }

func NewMemoryRepo(db *DB) *MemoryRepo { return &MemoryRepo{db: db} }

const memoryCols = `id, user_id, session_id, tier, kind, content, summary, tags, importance, pinned,
	source, embedding, use_count, expires_at, last_used_at, created_at, updated_at`

func scanMemory(row rowScanner) (*domain.Memory, error) {
	var (
		m        domain.Memory
		tags     string
		embed    string
		pinned   int
		expires  sql.NullString
		lastUsed sql.NullString
		created  string
		updated  string
	)
	if err := row.Scan(&m.ID, &m.UserID, &m.SessionID, &m.Tier, &m.Kind, &m.Content, &m.Summary, &tags,
		&m.Importance, &pinned, &m.Source, &embed, &m.UseCount, &expires, &lastUsed, &created, &updated); err != nil {
		return nil, err
	}
	m.Tags = decodeStrings(tags)
	m.Embedding = decodeFloats(embed)
	m.Pinned = pinned == 1
	m.ExpiresAt = ntp(expires)
	m.LastUsedAt = ntp(lastUsed)
	m.CreatedAt = parseTime(created)
	m.UpdatedAt = parseTime(updated)
	return &m, nil
}

// Create inserts a memory.
func (r *MemoryRepo) Create(ctx context.Context, m *domain.Memory) error {
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.Tier == "" {
		m.Tier = domain.TierLong
	}
	if m.Kind == "" {
		m.Kind = domain.KindFact
	}
	if m.Importance <= 0 {
		m.Importance = 0.5
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO memories (`+memoryCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		m.ID, m.UserID, m.SessionID, string(m.Tier), string(m.Kind), m.Content, m.Summary, jsonSliceText(m.Tags),
		m.Importance, b2i(m.Pinned), m.Source, jsonText(floatsToJSON(m.Embedding)), m.UseCount,
		ftp(m.ExpiresAt), ftp(m.LastUsedAt), ft(m.CreatedAt), ft(m.UpdatedAt))
	return mapErr("memory.create", err)
}

// Update rewrites a memory.
func (r *MemoryRepo) Update(ctx context.Context, m *domain.Memory) error {
	m.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE memories SET session_id = ?, tier = ?, kind = ?, content = ?,
		summary = ?, tags = ?, importance = ?, pinned = ?, source = ?, embedding = ?, expires_at = ?, updated_at = ? WHERE id = ?`),
		m.SessionID, string(m.Tier), string(m.Kind), m.Content, m.Summary, jsonSliceText(m.Tags), m.Importance,
		b2i(m.Pinned), m.Source, jsonText(floatsToJSON(m.Embedding)), ftp(m.ExpiresAt), ft(m.UpdatedAt), m.ID)
	if err != nil {
		return mapErr("memory.update", err)
	}
	return ensureAffected(res, "memory.update")
}

// Get loads one memory.
func (r *MemoryRepo) Get(ctx context.Context, id string) (*domain.Memory, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+memoryCols+` FROM memories WHERE id = ?`), id)
	m, err := scanMemory(row)
	if err != nil {
		return nil, mapErr("memory.get", err)
	}
	return m, nil
}

// Delete removes one memory.
func (r *MemoryRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM memories WHERE id = ?`), id)
	if err != nil {
		return mapErr("memory.delete", err)
	}
	return ensureAffected(res, "memory.delete")
}

// DeleteByUser wipes every memory for a user (GDPR-style deletion).
func (r *MemoryRepo) DeleteByUser(ctx context.Context, userID string) (int, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM memories WHERE user_id = ?`), userID)
	if err != nil {
		return 0, mapErr("memory.delete_user", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListByUser returns a filtered memory page.
func (r *MemoryRepo) ListByUser(ctx context.Context, userID string, filter ports.MemoryFilter, page platform.Page) ([]domain.Memory, int, error) {
	page = page.Normalize()
	where := []string{"user_id = ?"}
	args := []any{userID}
	if filter.Tier != "" {
		where = append(where, "tier = ?")
		args = append(args, string(filter.Tier))
	}
	if filter.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(filter.Kind))
	}
	if filter.SessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, filter.SessionID)
	}
	if filter.Pinned != nil {
		where = append(where, "pinned = ?")
		args = append(args, b2i(*filter.Pinned))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, "(LOWER(content) LIKE ? OR LOWER(summary) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM memories WHERE `+clause), args...).Scan(&total); err != nil {
		return nil, 0, mapErr("memory.count", err)
	}
	queryArgs := append(append([]any{}, args...), page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+memoryCols+` FROM memories WHERE `+clause+`
		ORDER BY pinned DESC, importance DESC, created_at DESC LIMIT ? OFFSET ?`), queryArgs...)
	if err != nil {
		return nil, 0, mapErr("memory.list", err)
	}
	defer rows.Close()
	out := make([]domain.Memory, 0, page.Limit)
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, 0, mapErr("memory.list_scan", err)
		}
		out = append(out, *m)
	}
	return out, total, mapErr("memory.list_iter", rows.Err())
}
