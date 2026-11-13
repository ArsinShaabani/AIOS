package httpapi

import (
	"net/http"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (a *API) handleAudit(w http.ResponseWriter, r *http.Request) {
	filter := ports.AuditFilter{
		ActorID: r.URL.Query().Get("actor_id"),
		Action:  r.URL.Query().Get("action"),
		Status:  r.URL.Query().Get("status"),
	}
	from, to := parseRange(r)
	if r.URL.Query().Get("from") != "" || r.URL.Query().Get("to") != "" {
		filter.From, filter.To = &from, &to
	}
	page := platform.Page{Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Repos.Audit.List(r.Context(), filter, page)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

func (a *API) handleSystem(w http.ResponseWriter, r *http.Request) {
	stats := a.System.Snapshot()
	byStatus, _ := a.Repos.Devices.CountByStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"system": stats, "devices": byStatus, "uptime_sec": int64(a.Metrics.Uptime().Seconds()),
		"version": a.Version,
	})
}

func (a *API) handleQueue(w http.ResponseWriter, r *http.Request) {
	stats := ports.QueueStats{}
	if a.Queue != nil {
		if s, err := a.Queue.Stats(r.Context()); err == nil {
			stats = s
		}
	}
	tasks, err := a.Scheduler.Stats(r.Context())
	if err != nil {
		a.Log.Warn("cannot read task stats", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": stats, "tasks": tasks})
}

func (a *API) handlePromptPreview(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	session, err := a.Repos.Sessions.Get(r.Context(), sessionID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	preview, err := a.Brain.PreviewPrompt(r.Context(), session.UserID, sessionID,
		r.URL.Query().Get("agent_id"), r.URL.Query().Get("personality_id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (a *API) handleAdminMemories(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		a.fail(w, r, platform.Invalid("memory.user", "user_id is required"))
		return
	}
	filter := ports.MemoryFilter{Tier: domain.MemoryTier(r.URL.Query().Get("tier")), Query: r.URL.Query().Get("q")}
	page := platform.Page{Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Memory.List(r.Context(), userID, filter, page)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

func (a *API) handlePrune(w http.ResponseWriter, r *http.Request) {
	expired, old, err := a.Memory.Prune(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	auditDeleted, _ := a.Repos.Audit.DeleteBefore(r.Context(), time.Now().UTC().AddDate(0, 0, -90))
	tasksDeleted, _ := a.Repos.Tasks.DeleteBefore(r.Context(), time.Now().UTC().AddDate(0, 0, -30))
	tokensDeleted, _ := a.Repos.Tokens.DeleteExpiredRefresh(r.Context(), time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{
		"memories_expired": expired, "memories_old": old,
		"audit_deleted": auditDeleted, "tasks_deleted": tasksDeleted, "tokens_deleted": tokensDeleted,
	})
}
