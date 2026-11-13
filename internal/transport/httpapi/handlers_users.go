package httpapi

import (
	"net/http"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

type updateUserRequest struct {
	DisplayName   *string `json:"display_name"`
	Role          *string `json:"role"`
	Status        *string `json:"status"`
	PersonalityID *string `json:"personality_id"`
	Locale        *string `json:"locale"`
	Timezone      *string `json:"timezone"`
}

func (a *API) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	u, err := a.Repos.Users.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	actor := userFrom(r.Context())
	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.DisplayName != nil {
		u.DisplayName = *req.DisplayName
	}
	if req.Role != nil {
		role := domain.Role(*req.Role)
		if !role.Valid() || role == domain.RoleDevice {
			a.fail(w, r, platform.Invalid("user.role", "invalid role"))
			return
		}
		if u.ID == actor.ID && role != domain.RoleAdmin {
			a.fail(w, r, platform.Forbidden("user.self_demote", "you cannot remove your own admin role"))
			return
		}
		u.Role = role
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "disabled" {
			a.fail(w, r, platform.Invalid("user.status", "status must be active or disabled"))
			return
		}
		if u.ID == actor.ID && *req.Status != "active" {
			a.fail(w, r, platform.Forbidden("user.self_disable", "you cannot disable your own account"))
			return
		}
		u.Status = *req.Status
	}
	if req.PersonalityID != nil {
		u.PersonalityID = *req.PersonalityID
	}
	if req.Locale != nil {
		u.Locale = *req.Locale
	}
	if req.Timezone != nil {
		u.Timezone = *req.Timezone
	}
	if err := a.Repos.Users.Update(r.Context(), u); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), actor.ID, "user", "user.update", "user", u.ID, "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusOK, u.Public())
}

func (a *API) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := userFrom(r.Context())
	if id == actor.ID {
		a.fail(w, r, platform.Forbidden("user.self_delete", "you cannot delete your own account"))
		return
	}
	if err := a.Repos.Users.Delete(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), actor.ID, "user", "user.delete", "user", id, "success", clientIP(r), r.UserAgent(), nil)
	w.WriteHeader(http.StatusNoContent)
}

type passwordRequest struct {
	Password string `json:"password"`
}

func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	u, err := a.Repos.Users.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	hash, err := security.HashPassword(req.Password, a.Config.Security.BcryptCost)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	u.PasswordHash = hash
	if err := a.Repos.Users.Update(r.Context(), u); err != nil {
		a.fail(w, r, err)
		return
	}
	_ = a.Auth.RevokeAll(r.Context(), u.ID)
	actor := userFrom(r.Context())
	a.Auditor.Log(r.Context(), actor.ID, "user", "user.reset_password", "user", u.ID, "success", clientIP(r), r.UserAgent(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleRevokeUserTokens(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Auth.RevokeAll(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	actor := userFrom(r.Context())
	a.Auditor.Log(r.Context(), actor.ID, "user", "user.revoke_tokens", "user", id, "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}
