package httpapi

import (
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/userai"
)

func (a *API) handleListUserProviders(w http.ResponseWriter, r *http.Request) {
	views, err := a.UserAI.List(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (a *API) handleUpsertUserProvider(w http.ResponseWriter, r *http.Request) {
	var in userai.Input
	if err := decodeJSON(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	in.Name = r.PathValue("name")
	user := userFrom(r.Context())
	v, err := a.UserAI.Upsert(r.Context(), user.ID, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "user_provider.upsert", "provider", v.Name, "success", clientIP(r), r.UserAgent(), map[string]any{"kind": v.Kind})
	writeJSON(w, http.StatusOK, v)
}

func (a *API) handleDeleteUserProvider(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if err := a.UserAI.Delete(r.Context(), user.ID, name); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "user_provider.delete", "provider", name, "success", clientIP(r), r.UserAgent(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleDiscoverUserProvider(w http.ResponseWriter, r *http.Request) {
	var in userai.Input
	if err := decodeJSON(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	models, err := a.UserAI.Discover(r.Context(), userFrom(r.Context()).ID, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (a *API) handleTestUserProvider(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	ms, reply, err := a.UserAI.Test(r.Context(), userFrom(r.Context()).ID, name)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": ms, "reply": platform.Truncate(reply, 120)})
}

// handleUserModels returns every provider/model pair the caller can use. The
// browser uses it for provider pickers and API-key creation.
func (a *API) handleUserModels(w http.ResponseWriter, r *http.Request) {
	providers, err := a.UserAI.List(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	type item struct {
		Provider string   `json:"provider"`
		Models   []string `json:"models"`
		Default  string   `json:"default_model"`
	}
	out := make([]item, 0, len(providers))
	for _, p := range providers {
		if p.Enabled {
			out = append(out, item{Provider: p.Name, Models: p.Models, Default: p.DefaultModel})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
