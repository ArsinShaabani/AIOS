package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// SettingRepo implements ports.SettingRepository.
type SettingRepo struct{ db *DB }

func NewSettingRepo(db *DB) *SettingRepo { return &SettingRepo{db: db} }

// Get loads one setting.
func (r *SettingRepo) Get(ctx context.Context, scope, scopeID, key string) (*domain.Setting, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT scope, scope_id, key, value, updated_by, updated_at
		FROM settings WHERE scope = ? AND scope_id = ? AND key = ?`), scope, scopeID, key)
	var (
		s       domain.Setting
		value   string
		updated string
	)
	if err := row.Scan(&s.Scope, &s.ScopeID, &s.Key, &value, &s.UpdatedBy, &updated); err != nil {
		return nil, mapErr("setting.get", err)
	}
	s.Value = []byte(value)
	s.UpdatedAt = parseTime(updated)
	return &s, nil
}

// Set upserts a setting.
func (r *SettingRepo) Set(ctx context.Context, s *domain.Setting) error {
	if s.Scope == "" {
		s.Scope = "global"
	}
	s.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO settings (scope, scope_id, key, value, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope, scope_id, key) DO UPDATE SET value = excluded.value,
			updated_by = excluded.updated_by, updated_at = excluded.updated_at`),
		s.Scope, s.ScopeID, s.Key, string(s.Value), s.UpdatedBy, ft(s.UpdatedAt))
	return mapErr("setting.set", err)
}

// List returns all settings at a scope.
func (r *SettingRepo) List(ctx context.Context, scope, scopeID string) ([]domain.Setting, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT scope, scope_id, key, value, updated_by, updated_at
		FROM settings WHERE scope = ? AND scope_id = ? ORDER BY key ASC`), scope, scopeID)
	if err != nil {
		return nil, mapErr("setting.list", err)
	}
	defer rows.Close()
	out := make([]domain.Setting, 0, 16)
	for rows.Next() {
		var (
			s       domain.Setting
			value   string
			updated string
		)
		if err := rows.Scan(&s.Scope, &s.ScopeID, &s.Key, &value, &s.UpdatedBy, &updated); err != nil {
			return nil, mapErr("setting.list_scan", err)
		}
		s.Value = []byte(value)
		s.UpdatedAt = parseTime(updated)
		out = append(out, s)
	}
	return out, mapErr("setting.list_iter", rows.Err())
}

// Delete removes a setting.
func (r *SettingRepo) Delete(ctx context.Context, scope, scopeID, key string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM settings WHERE scope = ? AND scope_id = ? AND key = ?`),
		scope, scopeID, key)
	return mapErr("setting.delete", err)
}
