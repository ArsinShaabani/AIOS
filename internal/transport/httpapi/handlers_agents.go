package httpapi

import (
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/agents"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

func (a *API) handleListAgents(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "true"
	p := principalFrom(r.Context())
	if all && !p.Can(security.PermAgentsWrite) {
		all = false
	}
	items, err := a.Agents.List(r.Context(), !all)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "tools": a.Brain.Tools.Names()})
}

func (a *API) handleUpsertAgent(w http.ResponseWriter, r *http.Request) {
	var ag domain.Agent
	if err := decodeJSON(w, r, &ag); err != nil {
		a.fail(w, r, err)
		return
	}
	if id := r.PathValue("id"); id != "" {
		ag.ID = id
	}
	if ag.ID == "" {
		ag.ID = platform.NewID("agent")
	}
	if ag.Name == "" {
		ag.Name = "Custom agent"
	}
	if err := agents.Validate(&ag); err != nil {
		a.fail(w, r, err)
		return
	}
	if ag.OwnerID == "" {
		ag.OwnerID = userFrom(r.Context()).ID
	}
	ag.Builtin = false
	if err := a.Repos.Agents.Upsert(r.Context(), &ag); err != nil {
		a.fail(w, r, err)
		return
	}
	_ = a.Agents.Reload(r.Context())
	writeJSON(w, http.StatusOK, ag)
}

func (a *API) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	ag, err := a.Repos.Agents.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if ag.Builtin {
		a.fail(w, r, platform.Forbidden("agent.builtin", "built-in agents cannot be deleted; disable them instead"))
		return
	}
	if err := a.Repos.Agents.Delete(r.Context(), ag.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	_ = a.Agents.Reload(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListPersonalities(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	items, err := a.Repos.Personalities.ListAvailable(r.Context(), user.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "default": user.PersonalityID})
}

func (a *API) handleCreatePersonality(w http.ResponseWriter, r *http.Request) {
	var p domain.Personality
	if err := decodeJSON(w, r, &p); err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	p.ID = platform.NewID("per")
	p.OwnerID = user.ID
	p.IsPreset = false
	if strings.TrimSpace(p.Name) == "" {
		a.fail(w, r, platform.Invalid("personality.name", "name is required"))
		return
	}
	p.Clamp()
	if err := a.Repos.Personalities.Create(r.Context(), &p); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (a *API) handleUpdatePersonality(w http.ResponseWriter, r *http.Request) {
	existing, err := a.Repos.Personalities.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	if existing.OwnerID != user.ID && p.Role != domain.RoleAdmin {
		a.fail(w, r, platform.Forbidden("personality.forbidden", "personality belongs to another user"))
		return
	}
	var patch domain.Personality
	if err := decodeJSON(w, r, &patch); err != nil {
		a.fail(w, r, err)
		return
	}
	existing.Name = firstString(patch.Name, existing.Name)
	existing.Description = firstString(patch.Description, existing.Description)
	existing.Tone = firstString(patch.Tone, existing.Tone)
	existing.Style = firstString(patch.Style, existing.Style)
	existing.CustomPrompt = patch.CustomPrompt
	existing.ResponseLength = firstString(patch.ResponseLength, existing.ResponseLength)
	if patch.Humor != 0 {
		existing.Humor = patch.Humor
	}
	if patch.EmotionalLevel != 0 {
		existing.EmotionalLevel = patch.EmotionalLevel
	}
	existing.Clamp()
	if err := a.Repos.Personalities.Update(r.Context(), existing); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (a *API) handleDeletePersonality(w http.ResponseWriter, r *http.Request) {
	existing, err := a.Repos.Personalities.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	if existing.IsPreset && p.Role != domain.RoleAdmin {
		a.fail(w, r, platform.Forbidden("personality.preset", "presets cannot be deleted"))
		return
	}
	if existing.OwnerID != user.ID && p.Role != domain.RoleAdmin {
		a.fail(w, r, platform.Forbidden("personality.forbidden", "personality belongs to another user"))
		return
	}
	if err := a.Repos.Personalities.Delete(r.Context(), existing.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func firstString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
