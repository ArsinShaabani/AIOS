package httpapi

import (
	"net/http"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (a *API) handleListMemories(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	filter := ports.MemoryFilter{
		Tier:  domain.MemoryTier(r.URL.Query().Get("tier")),
		Kind:  domain.MemoryKind(r.URL.Query().Get("kind")),
		Query: r.URL.Query().Get("q"),
	}
	page := platform.Page{Limit: queryInt(r, "limit", 25), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Memory.List(r.Context(), user.ID, filter, page)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

type createMemoryRequest struct {
	Content    string   `json:"content"`
	Kind       string   `json:"kind"`
	Importance float64  `json:"importance"`
	Tags       []string `json:"tags"`
	Pinned     bool     `json:"pinned"`
}

func (a *API) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	var req createMemoryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	m := &domain.Memory{
		UserID: user.ID, Content: req.Content, Kind: domain.MemoryKind(req.Kind),
		Importance: req.Importance, Tags: req.Tags, Pinned: req.Pinned, Source: "api",
	}
	stored, err := a.Memory.Remember(r.Context(), m)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "memory.create", "memory", stored.ID, "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusCreated, stored)
}

func (a *API) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if err := a.Memory.Delete(r.Context(), user.ID, r.PathValue("id")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleWipeMemories(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	n, err := a.Memory.DeleteAll(r.Context(), user.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "memory.wipe", "memory", "", "success", clientIP(r), r.UserAgent(), map[string]any{"deleted": n})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}
