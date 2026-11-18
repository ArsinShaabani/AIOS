package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// OpenAI-compatible tunnel. External apps/agents present an AIOS API key bound to
// one of the user's own providers/models; the server forwards with the encrypted
// upstream key and never exposes it.

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content"`
	Name       string       `json:"name,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaRequest struct {
	Model       string      `json:"model"`
	Messages    []oaMessage `json:"messages"`
	Temperature *float64    `json:"temperature"`
	TopP        *float64    `json:"top_p"`
	MaxTokens   int         `json:"max_tokens"`
	Stream      bool        `json:"stream"`
	Tools       []oaTool    `json:"tools"`
	ToolChoice  any         `json:"tool_choice"`
}

func (a *API) tunnelAuth(w http.ResponseWriter, r *http.Request) (*domain.User, *domain.APIKey, bool) {
	principal, user, err := a.authenticate(r)
	if err != nil {
		writeOAError(w, http.StatusUnauthorized, "invalid_api_key", err.Error())
		return nil, nil, false
	}
	if !principal.Can(security.PermChat) {
		writeOAError(w, http.StatusForbidden, "insufficient_scope", "this API key cannot use chat")
		return nil, nil, false
	}
	var key *domain.APIKey
	token := bearer(r)
	if token == "" {
		token = r.Header.Get("X-API-Key")
	}
	if strings.HasPrefix(token, "aios_") {
		key, _ = a.Repos.Tokens.GetAPIKeyByHash(r.Context(), security.HashToken(token))
	}
	return user, key, true
}

func (a *API) handleTunnelModels(w http.ResponseWriter, r *http.Request) {
	user, key, ok := a.tunnelAuth(w, r)
	if !ok {
		return
	}
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	out := make([]model, 0, 8)
	if key != nil && key.Provider != "" {
		_, cfg, err := a.UserAI.Build(r.Context(), user.ID, key.Provider)
		if err == nil {
			if key.Model != "" {
				out = append(out, model{ID: key.Model, Object: "model", OwnedBy: cfg.Name})
			} else {
				for _, m := range cfg.Models {
					out = append(out, model{ID: m.Name, Object: "model", OwnedBy: cfg.Name})
				}
			}
		}
	} else {
		list, err := a.UserAI.List(r.Context(), user.ID)
		if err != nil {
			writeOAError(w, http.StatusInternalServerError, "provider_error", err.Error())
			return
		}
		for _, p := range list {
			for _, m := range p.Models {
				out = append(out, model{ID: m, Object: "model", OwnedBy: p.Name})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": out})
}

func writeOAError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": message, "type": code, "code": code},
	})
}
