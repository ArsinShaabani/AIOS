package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	email := domain.NormalizeEmail(req.Email)
	key := clientIP(r) + "|" + email
	if a.LoginLimiter != nil && !a.LoginLimiter.Allow(key) {
		w.Header().Set("Retry-After", "60")
		a.Auditor.Log(r.Context(), "", "system", "auth.login_rate_limited", "user", "", "failure", clientIP(r), r.UserAgent(), map[string]any{"email": email})
		a.fail(w, r, platform.RateLimited("auth.rate_limited", "too many login attempts; try again shortly"))
		return
	}
	user, err := a.Repos.Users.GetByEmail(r.Context(), email)
	if err != nil || !security.CheckPassword(user.PasswordHash, req.Password) || !user.IsActive() {
		a.Auditor.Log(r.Context(), "", "system", "auth.login", "user", "", "failure", clientIP(r), r.UserAgent(), map[string]any{"email": email})
		a.fail(w, r, platform.Unauthorized("auth.invalid_credentials", "invalid email or password"))
		return
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	if err := a.Repos.Users.Update(r.Context(), user); err != nil {
		a.Log.Warn("cannot update last login", "error", err)
	}
	pair, err := a.Auth.IssuePair(r.Context(), user, r.UserAgent(), clientIP(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "auth.login", "user", user.ID, "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"tokens":      pair,
		"user":        user.Public(),
		"permissions": security.Permissions(user.Role),
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (a *API) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	pair, err := a.Auth.Refresh(r.Context(), req.RefreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		a.Auditor.Log(r.Context(), "", "system", "auth.refresh", "session", "", "failure", clientIP(r), r.UserAgent(), nil)
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": pair})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err == nil && req.RefreshToken != "" {
		_ = a.Auth.Revoke(r.Context(), req.RefreshToken)
	}
	p := principalFrom(r.Context())
	a.Auditor.Log(r.Context(), p.UserID, "user", "auth.logout", "session", "", "success", clientIP(r), r.UserAgent(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"user":        user.Public(),
		"principal":   p,
		"permissions": security.Permissions(user.Role),
	})
}

func (a *API) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	keys, err := a.Repos.Tokens.ListAPIKeys(r.Context(), user.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": keys})
}

type createKeyRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
}

func (a *API) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		a.fail(w, r, platform.Invalid("apikey.name", "a name is required"))
		return
	}
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	if len(req.Scopes) == 0 {
		req.Scopes = []string{string(security.PermChat), string(security.PermMemoryRead)}
	}
	for _, scope := range req.Scopes {
		if !p.Can(security.Permission(scope)) {
			a.fail(w, r, platform.Forbidden("apikey.scope", "you cannot grant a permission you do not hold: "+scope))
			return
		}
	}
	cred, err := security.NewAPICredential()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	key := &domain.APIKey{
		ID: platform.NewID("key"), UserID: user.ID, Name: name, KeyHash: cred.Hash,
		Prefix: cred.Prefix, Scopes: req.Scopes, CreatedAt: time.Now().UTC(),
	}
	if req.ExpiresInDays > 0 {
		exp := time.Now().UTC().AddDate(0, 0, req.ExpiresInDays)
		key.ExpiresAt = &exp
	}
	if err := a.Repos.Tokens.CreateAPIKey(r.Context(), key); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "apikey.create", "api_key", key.ID, "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusCreated, map[string]any{"api_key": key, "token": cred.Token})
}

func (a *API) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user := userFrom(r.Context())
	if err := a.Repos.Tokens.RevokeAPIKey(r.Context(), id, user.ID, time.Now().UTC()); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "apikey.revoke", "api_key", id, "success", clientIP(r), r.UserAgent(), nil)
	w.WriteHeader(http.StatusNoContent)
}
