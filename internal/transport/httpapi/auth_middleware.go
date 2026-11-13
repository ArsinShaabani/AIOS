package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// authed resolves a bearer token or API key and injects the principal.
func (a *API) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, user, err := a.authenticate(r)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal)
		ctx = context.WithValue(ctx, userKey, user)
		next(w, r.WithContext(ctx))
	}
}

// perm enforces a permission after authentication.
func (a *API) perm(perm security.Permission, next http.HandlerFunc) http.HandlerFunc {
	return a.authed(func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r.Context())
		if p == nil || !p.Can(perm) {
			a.fail(w, r, platform.Forbidden("http.forbidden", "you do not have permission for this action"))
			return
		}
		next(w, r)
	})
}

// deviceAuthed authenticates a device token.
func (a *API) deviceAuthed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			token = r.Header.Get("X-Device-Token")
		}
		device, err := a.Devices.Authenticate(r.Context(), token)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		principal := &security.Principal{
			UserID: device.UserID, Role: domain.RoleDevice, Method: "device", DeviceID: device.ID,
		}
		ctx := context.WithValue(r.Context(), principalKey, principal)
		ctx = context.WithValue(ctx, deviceKey, device)
		next(w, r.WithContext(ctx))
	}
}

func (a *API) authenticate(r *http.Request) (*security.Principal, *domain.User, error) {
	token := bearer(r)
	if token == "" {
		token = r.Header.Get("X-API-Key")
	}
	if token == "" {
		return nil, nil, platform.Unauthorized("auth.missing", "authentication required")
	}
	if strings.HasPrefix(token, "aios_") {
		key, err := a.Repos.Tokens.GetAPIKeyByHash(r.Context(), security.HashToken(token))
		if err != nil {
			return nil, nil, platform.Unauthorized("auth.api_key", "invalid API key")
		}
		now := time.Now().UTC()
		if key.RevokedAt != nil || (key.ExpiresAt != nil && now.After(*key.ExpiresAt)) {
			return nil, nil, platform.Unauthorized("auth.api_key_expired", "API key is revoked or expired")
		}
		user, err := a.Repos.Users.GetByID(r.Context(), key.UserID)
		if err != nil || !user.IsActive() {
			return nil, nil, platform.Unauthorized("auth.user", "account is unavailable")
		}
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = a.Repos.Tokens.TouchAPIKey(ctx, id, time.Now().UTC())
		}(key.ID)
		return &security.Principal{UserID: user.ID, Role: user.Role, Scopes: key.Scopes, Method: "api_key"}, user, nil
	}
	principal, err := a.Auth.ParseAccess(token)
	if err != nil {
		return nil, nil, err
	}
	user, err := a.Repos.Users.GetByID(r.Context(), principal.UserID)
	if err != nil {
		return nil, nil, platform.Unauthorized("auth.user", "account no longer exists")
	}
	if !user.IsActive() {
		return nil, nil, platform.Forbidden("auth.disabled", "account is disabled")
	}
	principal.Role = user.Role
	return principal, user, nil
}

func deviceFrom(ctx context.Context) *domain.Device {
	if v, ok := ctx.Value(deviceKey).(*domain.Device); ok {
		return v
	}
	return nil
}
