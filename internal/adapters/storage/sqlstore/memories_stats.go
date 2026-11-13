package sqlstore

import (
	"context"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// RecentByUser returns the newest memories, used to build context.
func (r *MemoryRepo) RecentByUser(ctx context.Context, userID string, limit int) ([]domain.Memory, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+memoryCols+` FROM memories
		WHERE user_id = ? ORDER BY pinned DESC, importance DESC, created_at DESC LIMIT ?`), userID, limit)
	if err != nil {
		return nil, mapErr("memory.recent", err)
	}
	defer rows.Close()
	out := make([]domain.Memory, 0, limit)
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, mapErr("memory.recent_scan", err)
		}
		out = append(out, *m)
	}
	return out, mapErr("memory.recent_iter", rows.Err())
}

// ListWithEmbeddings returns memories that have vectors, newest first. The
// local vector store warms itself from this at boot.
func (r *MemoryRepo) ListWithEmbeddings(ctx context.Context, userID string, limit int) ([]domain.Memory, error) {
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	query := `SELECT ` + memoryCols + ` FROM memories WHERE embedding <> '[]'`
	args := []any{}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, mapErr("memory.embeddings", err)
	}
	defer rows.Close()
	out := make([]domain.Memory, 0, 128)
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, mapErr("memory.embeddings_scan", err)
		}
		if len(m.Embedding) > 0 {
			out = append(out, *m)
		}
	}
	return out, mapErr("memory.embeddings_iter", rows.Err())
}

// IncrementUse bumps retrieval statistics in one statement.
func (r *MemoryRepo) IncrementUse(ctx context.Context, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, ft(at))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE memories SET use_count = use_count + 1, last_used_at = ?
		WHERE id IN (`+strings.Join(placeholders, ",")+`)`), args...)
	return mapErr("memory.touch", err)
}

// PruneExpired deletes expired memories in bounded batches.
func (r *MemoryRepo) PruneExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM memories WHERE id IN (
		SELECT id FROM memories WHERE expires_at IS NOT NULL AND expires_at < ? LIMIT ?)`), ft(now), limit)
	if err != nil {
		return 0, mapErr("memory.prune_expired", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PruneOld enforces retention while preserving pinned memories.
func (r *MemoryRepo) PruneOld(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM memories WHERE id IN (
		SELECT id FROM memories WHERE created_at < ? AND pinned = 0 LIMIT ?)`), ft(before), limit)
	if err != nil {
		return 0, mapErr("memory.prune_old", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CountByUser counts a user's memories.
func (r *MemoryRepo) CountByUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM memories WHERE user_id = ?`), userID).Scan(&n)
	return n, mapErr("memory.count_user", err)
}

// Count counts all memories.
func (r *MemoryRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&n)
	return n, mapErr("memory.count_all", err)
}
