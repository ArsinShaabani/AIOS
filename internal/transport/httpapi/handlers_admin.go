package httpapi

import (
	"net/http"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := a.Repos.Stats.Overview(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"overview": overview, "system": a.System.Snapshot(),
		"uptime_sec": int64(a.Metrics.Uptime().Seconds()), "version": a.Version,
	})
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	filter := ports.UserFilter{
		Role:   domain.Role(r.URL.Query().Get("role")),
		Status: r.URL.Query().Get("status"),
		Query:  r.URL.Query().Get("q"),
	}
	page := platform.Page{Limit: queryInt(r, "limit", 25), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Repos.Users.List(r.Context(), page, filter)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]domain.User, len(items))
	for i := range items {
		out[i] = items[i].Public()
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(out, total, page))
}

type createUserRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	DisplayName   string `json:"display_name"`
	Role          string `json:"role"`
	PersonalityID string `json:"personality_id"`
	Locale        string `json:"locale"`
	Timezone      string `json:"timezone"`
}

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	role := domain.Role(req.Role)
	if role == "" {
		role = domain.RoleUser
	}
	if !role.Valid() || role == domain.RoleDevice {
		a.fail(w, r, platform.Invalid("user.role", "invalid role"))
		return
	}
	hash, err := security.HashPassword(req.Password, a.Config.Security.BcryptCost)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	u := &domain.User{
		ID: platform.NewID("usr"), Email: domain.NormalizeEmail(req.Email), DisplayName: req.DisplayName,
		PasswordHash: hash, Role: role, Status: "active", PersonalityID: req.PersonalityID,
		Locale: req.Locale, Timezone: req.Timezone, CreatedAt: time.Now().UTC(),
	}
	if err := u.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Repos.Users.Create(r.Context(), u); err != nil {
		a.fail(w, r, err)
		return
	}
	actor := userFrom(r.Context())
	a.Auditor.Log(r.Context(), actor.ID, "user", "user.create", "user", u.ID, "success", clientIP(r), r.UserAgent(), map[string]any{"role": string(role)})
	writeJSON(w, http.StatusCreated, u.Public())
}
