package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/providers"
)

// handleListProviders returns configured providers with live health.
func (a *API) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if a.Providers == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	llm := a.Providers.LLM()
	writeJSON(w, http.StatusOK, map[string]any{
		"items":            a.Providers.List(r.Context()),
		"default_provider": llm.DefaultProvider,
		"default_model":    llm.DefaultModel,
		"ollama_url":       a.Providers.OllamaBaseURL(),
		"pulls":            a.Providers.PullStatuses(),
	})
}

func (a *API) handleProviderCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": providers.Catalog()})
}

func (a *API) handleUpsertProvider(w http.ResponseWriter, r *http.Request) {
	var in providers.Input
	if err := decodeJSON(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	in.Name = r.PathValue("name")
	view, err := a.Providers.Upsert(r.Context(), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.auditProvider(r, "provider.upsert", "provider", view.Name, map[string]any{"kind": view.Kind, "default": view.Default})
	writeJSON(w, http.StatusOK, view)
}

func (a *API) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := a.Providers.Delete(r.Context(), name); err != nil {
		a.fail(w, r, err)
		return
	}
	a.auditProvider(r, "provider.delete", "provider", name, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	_ = decodeJSON(w, r, &body)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res := a.Providers.Test(ctx, r.PathValue("name"), body.Model)
	writeJSON(w, http.StatusOK, res)
}

func (a *API) handleSetDefaultProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Providers.SetDefault(r.Context(), body.Provider, body.Model); err != nil {
		a.fail(w, r, err)
		return
	}
	a.auditProvider(r, "provider.set_default", "provider", body.Provider, map[string]any{"model": body.Model})
	llm := a.Providers.LLM()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": llm.DefaultProvider, "model": llm.DefaultModel})
}

func (a *API) handleOllamaModels(w http.ResponseWriter, r *http.Request) {
	models, err := a.Providers.OllamaModels(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": models, "base_url": a.Providers.OllamaBaseURL(), "pulls": a.Providers.PullStatuses(),
	})
}

func (a *API) handleOllamaPull(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	st, err := a.Providers.PullModel(body.Model)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.auditProvider(r, "model.pull", "model", body.Model, nil)
	writeJSON(w, http.StatusAccepted, st)
}

func (a *API) handleOllamaDelete(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("name")
	if model == "" {
		a.fail(w, r, platform.Invalid("provider.model_required", "model name is required"))
		return
	}
	if err := a.Providers.DeleteOllamaModel(r.Context(), model); err != nil {
		a.fail(w, r, err)
		return
	}
	a.auditProvider(r, "model.delete", "model", model, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) auditProvider(r *http.Request, action, target, id string, meta map[string]any) {
	actor := ""
	if p := principalFrom(r.Context()); p != nil {
		actor = p.UserID
	}
	a.Auditor.Log(r.Context(), actor, "admin", action, target, id, "success", clientIP(r), r.UserAgent(), meta)
}
