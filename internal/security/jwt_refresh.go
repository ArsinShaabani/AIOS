package security

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// IssuePair creates an access token and a rotating refresh token.
func (s *TokenService) IssuePair(ctx context.Context, u *domain.User, userAgent, ip string) (*TokenPair, error) {
	access, exp, err := s.IssueAccess(u)
	if err != nil {
		return nil, err
	}
	refresh, refreshExp, err := s.newRefresh(ctx, u.ID, "", userAgent, ip)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:      access,
		RefreshToken:     refresh,
		TokenType:        "Bearer",
		ExpiresIn:        int(time.Until(exp).Seconds()),
		RefreshExpiresAt: refreshExp,
		User:             u.PublicPtr(),
	}, nil
}

func (s *TokenService) newRefresh(ctx context.Context, userID, family, userAgent, ip string) (string, time.Time, error) {
	raw, err := NewSecret("rt")
	if err != nil {
		return "", time.Time{}, platform.Wrap(platform.KindInternal, "token.random", "cannot generate refresh token", err)
	}
	if family == "" {
		family = platform.NewID("fam")
	}
	exp := s.now().UTC().Add(s.refreshTTL)
	row := &domain.RefreshToken{
		ID:        platform.NewID("rft"),
		UserID:    userID,
		TokenHash: HashToken(raw),
		FamilyID:  family,
		UserAgent: platform.Truncate(userAgent, 200),
		IP:        ip,
		ExpiresAt: exp,
		CreatedAt: s.now().UTC(),
	}
	if err := s.tokens.CreateRefresh(ctx, row); err != nil {
		return "", time.Time{}, err
	}
	return raw, exp, nil
}

// Refresh rotates a refresh token. Reuse of a rotated token revokes the family.
func (s *TokenService) Refresh(ctx context.Context, raw, userAgent, ip string) (*TokenPair, error) {
	if raw == "" {
		return nil, platform.Unauthorized("token.missing_refresh", "refresh token is required")
	}
	row, err := s.tokens.GetRefreshByHash(ctx, HashToken(raw))
	if err != nil {
		return nil, platform.Unauthorized("token.invalid_refresh", "invalid refresh token")
	}
	now := s.now().UTC()
	if row.RevokedAt != nil {
		return nil, platform.Unauthorized("token.revoked", "refresh token has been revoked")
	}
	if row.ReplacedBy != "" {
		// Reuse of an already-rotated token: assume theft and burn the family.
		_ = s.tokens.RevokeFamily(ctx, row.FamilyID, now)
		return nil, platform.Unauthorized("token.reuse", "refresh token reuse detected; sessions revoked")
	}
	if now.After(row.ExpiresAt) {
		return nil, platform.Unauthorized("token.expired_refresh", "refresh token expired")
	}
	user, err := s.users.GetByID(ctx, row.UserID)
	if err != nil {
		return nil, platform.Unauthorized("token.user_missing", "account no longer exists")
	}
	if !user.IsActive() {
		return nil, platform.Forbidden("token.disabled", "account is disabled")
	}
	access, exp, err := s.IssueAccess(user)
	if err != nil {
		return nil, err
	}
	refresh, refreshExp, err := s.newRefresh(ctx, user.ID, row.FamilyID, userAgent, ip)
	if err != nil {
		return nil, err
	}
	newRow, err := s.tokens.GetRefreshByHash(ctx, HashToken(refresh))
	if err != nil {
		return nil, err
	}
	if err := s.tokens.RotateRefresh(ctx, row.ID, newRow.ID, now); err != nil {
		// Lost a race: revoke the family to be safe.
		_ = s.tokens.RevokeFamily(ctx, row.FamilyID, now)
		return nil, platform.Unauthorized("token.race", "refresh token already used")
	}
	return &TokenPair{
		AccessToken:      access,
		RefreshToken:     refresh,
		TokenType:        "Bearer",
		ExpiresIn:        int(time.Until(exp).Seconds()),
		RefreshExpiresAt: refreshExp,
		User:             user.PublicPtr(),
	}, nil
}

// Revoke logs out a refresh token (idempotent).
func (s *TokenService) Revoke(ctx context.Context, raw string) error {
	row, err := s.tokens.GetRefreshByHash(ctx, HashToken(raw))
	if err != nil {
		if platform.IsNotFound(err) {
			return nil
		}
		return err
	}
	return s.tokens.RevokeRefresh(ctx, row.ID, s.now().UTC())
}

// RevokeAll revokes every refresh token for a user (admin action).
func (s *TokenService) RevokeAll(ctx context.Context, userID string) error {
	return s.tokens.RevokeAllForUser(ctx, userID, s.now().UTC())
}
