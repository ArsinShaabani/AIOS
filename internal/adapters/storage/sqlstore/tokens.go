package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// TokenRepo implements ports.TokenRepository.
type TokenRepo struct{ db *DB }

func NewTokenRepo(db *DB) *TokenRepo { return &TokenRepo{db: db} }

const refreshCols = `id, user_id, token_hash, family_id, replaced_by, user_agent, ip, expires_at, revoked_at, created_at`

func scanRefresh(row rowScanner) (*domain.RefreshToken, error) {
	var (
		t       domain.RefreshToken
		expires string
		revoked sqlNullString
		created string
	)
	if err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID, &t.ReplacedBy, &t.UserAgent, &t.IP,
		&expires, &revoked, &created); err != nil {
		return nil, err
	}
	t.ExpiresAt = parseTime(expires)
	t.RevokedAt = revoked.Ptr()
	t.CreatedAt = parseTime(created)
	return &t, nil
}

// CreateRefresh stores a refresh token hash.
func (r *TokenRepo) CreateRefresh(ctx context.Context, t *domain.RefreshToken) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO refresh_tokens (`+refreshCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		t.ID, t.UserID, t.TokenHash, t.FamilyID, t.ReplacedBy, t.UserAgent, t.IP,
		ft(t.ExpiresAt), ftp(t.RevokedAt), ft(t.CreatedAt))
	return mapErr("token.create_refresh", err)
}

// GetRefreshByHash looks up a refresh token without revealing it.
func (r *TokenRepo) GetRefreshByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+refreshCols+` FROM refresh_tokens WHERE token_hash = ?`), hash)
	t, err := scanRefresh(row)
	if err != nil {
		return nil, mapErr("token.get_refresh", err)
	}
	return t, nil
}

// RotateRefresh links an old refresh token to its replacement.
func (r *TokenRepo) RotateRefresh(ctx context.Context, oldID, newID string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE refresh_tokens SET replaced_by = ? WHERE id = ? AND replaced_by = ''`),
		newID, oldID)
	if err != nil {
		return mapErr("token.rotate", err)
	}
	return ensureAffected(res, "token.rotate")
}

// RevokeFamily revokes every token in a rotation family (reuse detected).
func (r *TokenRepo) RevokeFamily(ctx context.Context, familyID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE refresh_tokens SET revoked_at = ?
		WHERE family_id = ? AND revoked_at IS NULL`), ft(at), familyID)
	return mapErr("token.revoke_family", err)
}

// RevokeRefresh revokes a single refresh token.
func (r *TokenRepo) RevokeRefresh(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE refresh_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`),
		ft(at), id)
	return mapErr("token.revoke", err)
}

// RevokeAllForUser revokes every refresh token belonging to a user.
func (r *TokenRepo) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE refresh_tokens SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL`), ft(at), userID)
	return mapErr("token.revoke_user", err)
}

// DeleteExpiredRefresh trims expired tokens.
func (r *TokenRepo) DeleteExpiredRefresh(ctx context.Context, before time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM refresh_tokens WHERE expires_at < ?`), ft(before))
	if err != nil {
		return 0, mapErr("token.prune", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

const apiKeyCols = `id, user_id, name, key_hash, prefix, scopes, provider, model, last_used_at, expires_at, revoked_at, created_at`

func scanAPIKey(row rowScanner) (*domain.APIKey, error) {
	var (
		k        domain.APIKey
		scopes   string
		lastUsed sqlNullString
		expires  sqlNullString
		revoked  sqlNullString
		created  string
	)
	if err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.Prefix, &scopes, &k.Provider, &k.Model, &lastUsed, &expires, &revoked, &created); err != nil {
		return nil, err
	}
	k.Scopes = decodeStrings(scopes)
	k.LastUsedAt = lastUsed.Ptr()
	k.ExpiresAt = expires.Ptr()
	k.RevokedAt = revoked.Ptr()
	k.CreatedAt = parseTime(created)
	return &k, nil
}

// CreateAPIKey stores a hashed API key.
func (r *TokenRepo) CreateAPIKey(ctx context.Context, k *domain.APIKey) error {
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO api_keys (`+apiKeyCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		k.ID, k.UserID, k.Name, k.KeyHash, k.Prefix, jsonSliceText(k.Scopes), k.Provider, k.Model,
		ftp(k.LastUsedAt), ftp(k.ExpiresAt), ftp(k.RevokedAt), ft(k.CreatedAt))
	return mapErr("apikey.create", err)
}

// GetAPIKeyByHash loads an API key by its hash.
func (r *TokenRepo) GetAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+apiKeyCols+` FROM api_keys WHERE key_hash = ?`), hash)
	k, err := scanAPIKey(row)
	if err != nil {
		return nil, mapErr("apikey.get", err)
	}
	return k, nil
}

// ListAPIKeys lists a user's API keys.
func (r *TokenRepo) ListAPIKeys(ctx context.Context, userID string) ([]domain.APIKey, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+apiKeyCols+` FROM api_keys
		WHERE user_id = ? ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, mapErr("apikey.list", err)
	}
	defer rows.Close()
	out := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, mapErr("apikey.list_scan", err)
		}
		out = append(out, *k)
	}
	return out, mapErr("apikey.list_iter", rows.Err())
}

// RevokeAPIKey revokes a key owned by userID.
func (r *TokenRepo) RevokeAPIKey(ctx context.Context, id, userID string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE api_keys SET revoked_at = ? WHERE id = ? AND user_id = ?`),
		ft(at), id, userID)
	if err != nil {
		return mapErr("apikey.revoke", err)
	}
	return ensureAffected(res, "apikey.revoke")
}

// TouchAPIKey records last usage without blocking the request path.
func (r *TokenRepo) TouchAPIKey(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`), ft(at), id)
	return mapErr("apikey.touch", err)
}
