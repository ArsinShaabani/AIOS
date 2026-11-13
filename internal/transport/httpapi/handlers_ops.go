package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

func (a *API) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	items, err := a.Plugins.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		domain.Plugin
		WasmModule string `json:"wasm_module"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	var wasm []byte
	if req.WasmModule != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.WasmModule)
		if err != nil {
			a.fail(w, r, platform.Invalid("plugin.wasm_base64", "wasm_module must be base64-encoded"))
			return
		}
		wasm = decoded
	}
	installed, err := a.Plugins.InstallModule(r.Context(), &req.Plugin, wasm)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), userFrom(r.Context()).ID, "user", "plugin.install", "plugin", installed.ID,
		"success", clientIP(r), r.UserAgent(), map[string]any{"name": installed.Name, "kind": installed.Kind,
			"verified": installed.Verified})
	writeJSON(w, http.StatusCreated, installed)
}

func (a *API) handleTogglePlugin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Plugins.SetEnabled(r.Context(), r.PathValue("id"), req.Enabled); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "enabled": req.Enabled})
}

func (a *API) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	if err := a.Plugins.Uninstall(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListSettings(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}
	if scope == "user" {
		scopeID := user.ID
		items, err := a.Repos.Settings.List(r.Context(), "user", scopeID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "user", "items": items})
		return
	}
	items, err := a.Repos.Settings.List(r.Context(), "global", "")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scope": "global", "items": items})
}

func (a *API) handleSetSetting(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" || strings.HasPrefix(key, "secret.") {
		a.fail(w, r, platform.Invalid("settings.key", "invalid setting key"))
		return
	}
	var body json.RawMessage
	if err := decodeJSON(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	scope, scopeID := "global", ""
	if r.URL.Query().Get("scope") == "user" {
		scope, scopeID = "user", user.ID
	}
	setting := &domain.Setting{
		Scope: scope, ScopeID: scopeID, Key: key, Value: body, UpdatedBy: user.ID,
	}
	if err := a.Repos.Settings.Set(r.Context(), setting); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "settings.set", "setting", key, "success", clientIP(r), r.UserAgent(), map[string]any{"scope": scope})
	writeJSON(w, http.StatusOK, setting)
}

func (a *API) handleDeleteSetting(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	scope, scopeID := "global", ""
	if r.URL.Query().Get("scope") == "user" {
		scope, scopeID = "user", user.ID
	}
	if err := a.Repos.Settings.Delete(r.Context(), scope, scopeID, r.PathValue("key")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": a.Router.Providers(),
		"default":   map[string]string{"provider": a.Config.LLM.DefaultProvider, "model": a.Config.LLM.DefaultModel},
		"routing":   a.Config.LLM.Routing,
		"embedding": map[string]string{"provider": a.Config.LLM.EmbeddingProvider, "model": a.Config.LLM.EmbeddingModel},
	})
}

func parseRange(r *http.Request) (time.Time, time.Time) {
	to := time.Now().UTC()
	from := to.Add(-30 * 24 * time.Hour)
	if raw := r.URL.Query().Get("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			from = t
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			to = t
		}
	}
	return from, to
}

func (a *API) handleUsage(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	userID := user.ID
	if r.URL.Query().Get("all") == "true" && p.Can(security.PermUsersRead) {
		userID = ""
	}
	from, to := parseRange(r)
	totals, err := a.Repos.Usage.Totals(r.Context(), userID, from, to)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"totals": totals, "from": from, "to": to, "budget_usd": a.Config.LLM.DailyBudgetUSD})
}

func (a *API) handleUsageDaily(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	userID := user.ID
	if r.URL.Query().Get("all") == "true" && p.Can(security.PermUsersRead) {
		userID = ""
	}
	from, to := parseRange(r)
	points, err := a.Repos.Usage.DailySeries(r.Context(), userID, from, to)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": points, "from": from, "to": to})
}

func (a *API) handleUsageProviders(w http.ResponseWriter, r *http.Request) {
	from, to := parseRange(r)
	items, err := a.Repos.Usage.ByProvider(r.Context(), from, to)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
