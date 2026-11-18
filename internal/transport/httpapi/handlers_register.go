package httpapi

import (
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// handleRegister creates a normal user account and signs it in immediately.
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Security.AllowRegistration {
		a.fail(w, r, platform.Forbidden("auth.registration_disabled", "registration is disabled on this server"))
		return
	}
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	email := domain.NormalizeEmail(req.Email)
	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, "@") {
		a.fail(w, r, platform.Invalid("auth.email", "enter a valid email address"))
		return
	}
	if len(req.Password) < 8 {
		a.fail(w, r, platform.Invalid("auth.password", "password must be at least 8 characters"))
		return
	}
	if a.LoginLimiter != nil && !a.LoginLimiter.Allow(clientIP(r)+"|register") {
		a.fail(w, r, platform.RateLimited("auth.rate_limited", "too many attempts; try again shortly"))
		return
	}
	if _, err := a.Repos.Users.GetByEmail(r.Context(), email); err == nil {
		a.fail(w, r, platform.Conflict("auth.email_taken", "an account with this email already exists"))
		return
	}
	hash, err := security.HashPassword(req.Password, a.Config.Security.BcryptCost)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	now := time.Now().UTC()
	user := &domain.User{ID: platform.NewID("usr"), Email: email, DisplayName: platform.Truncate(name, 80),
		PasswordHash: hash, Role: domain.RoleUser, Status: "active", Locale: "fa", Timezone: "UTC",
		CreatedAt: now, UpdatedAt: now}
	if err := a.Repos.Users.Create(r.Context(), user); err != nil {
		a.fail(w, r, err)
		return
	}
	pair, err := a.Auth.IssuePair(r.Context(), user, r.UserAgent(), clientIP(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "auth.register", "user", user.ID, "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusCreated, map[string]any{
		"tokens": pair, "user": user.Public(), "permissions": security.Permissions(user.Role),
	})
}
