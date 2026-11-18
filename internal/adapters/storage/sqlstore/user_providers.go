package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

const userProviderCols = `id, user_id, name, kind, base_url, default_model, models, api_key_enc, enabled, max_tokens, context_window, created_at, updated_at`

// UserProviderRepo persists per-user AI provider credentials.
type UserProviderRepo struct{ db *DB }

func NewUserProviderRepo(db *DB) *UserProviderRepo { return &UserProviderRepo{db: db} }

func scanUserProvider(row rowScanner) (*domain.UserProvider, error) {
	var (
		p                 domain.UserProvider
		models            string
		enabled           int
		created, modified string
	)
	if err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Kind, &p.BaseURL, &p.DefaultModel,
		&models, &p.APIKeyEnc, &enabled, &p.MaxTokens, &p.ContextWindow, &created, &modified); err != nil {
		return nil, err
	}
	p.Models = decodeStrings(models)
	p.Enabled = enabled == 1
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(modified)
	return &p, nil
}

func (r *UserProviderRepo) List(ctx context.Context, userID string) ([]domain.UserProvider, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+userProviderCols+` FROM user_providers WHERE user_id = ? ORDER BY name`), userID)
	if err != nil {
		return nil, mapErr("provider.list", err)
	}
	defer rows.Close()
	out := make([]domain.UserProvider, 0, 4)
	for rows.Next() {
		p, err := scanUserProvider(rows)
		if err != nil {
			return nil, mapErr("provider.scan", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *UserProviderRepo) Get(ctx context.Context, userID, name string) (*domain.UserProvider, error) {
	p, err := scanUserProvider(r.db.QueryRowContext(ctx,
		r.db.Rebind(`SELECT `+userProviderCols+` FROM user_providers WHERE user_id = ? AND name = ?`), userID, name))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NotFound("provider.not_found", "provider not found")
		}
		return nil, mapErr("provider.get", err)
	}
	return p, nil
}

func (r *UserProviderRepo) Upsert(ctx context.Context, p *domain.UserProvider) error {
	raw, err := json.Marshal(p.Models)
	if err != nil {
		return err
	}
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO user_providers (`+userProviderCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, name) DO UPDATE SET
			kind = excluded.kind, base_url = excluded.base_url, default_model = excluded.default_model,
			models = excluded.models, api_key_enc = excluded.api_key_enc, enabled = excluded.enabled,
			max_tokens = excluded.max_tokens, context_window = excluded.context_window,
			updated_at = excluded.updated_at`),
		p.ID, p.UserID, p.Name, p.Kind, p.BaseURL, p.DefaultModel, string(raw), p.APIKeyEnc,
		enabled, p.MaxTokens, p.ContextWindow, ft(p.CreatedAt), ft(p.UpdatedAt))
	return mapErr("provider.upsert", err)
}

func (r *UserProviderRepo) Delete(ctx context.Context, userID, name string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM user_providers WHERE user_id = ? AND name = ?`), userID, name)
	if err != nil {
		return mapErr("provider.delete", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return platform.NotFound("provider.not_found", "provider not found")
	}
	return nil
}
