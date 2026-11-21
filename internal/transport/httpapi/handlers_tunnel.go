package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
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

// oaContent accepts both shapes OpenAI clients send for a message body: a plain
// string, or an array of typed parts ({"type":"text","text":"…"}). Agent clients
// switch to parts whenever an image, a file or a reasoning block is involved;
// rejecting the array failed the whole turn with an opaque 400.
type oaContent struct{ text string }

func (c *oaContent) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case trimmed == "" || trimmed == "null":
		return nil
	case strings.HasPrefix(trimmed, `"`):
		return json.Unmarshal(data, &c.text)
	case strings.HasPrefix(trimmed, "["):
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(data, &parts); err != nil {
			return err
		}
		texts := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Text == "" {
				continue
			}
			if part.Type == "" || part.Type == "text" || part.Type == "input_text" {
				texts = append(texts, part.Text)
			}
		}
		c.text = strings.Join(texts, "\n")
		return nil
	default:
		return fmt.Errorf("content must be a string or an array of content parts")
	}
}

func (c oaContent) String() string { return c.text }

type oaMessage struct {
	Role       string       `json:"role"`
	Content    oaContent    `json:"content"`
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
	// MaxCompletionTokens is the modern OpenAI name; clients such as Hermes and
	// the current SDKs send only this one. Ignoring it replaced the caller's
	// budget with AIOS' default.
	MaxCompletionTokens int      `json:"max_completion_tokens"`
	Stream              bool     `json:"stream"`
	Tools               []oaTool `json:"tools"`
	ToolChoice          any      `json:"tool_choice"`
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

// writeProviderFailure maps an upstream provider failure onto the OpenAI error
// envelope with a status the caller can act on. Agents distinguish "the request
// is too large" (413), "back off" (429) and "your key is wrong" (401); folding
// all of them into 502 made every one of those look like a broken gateway.
func writeProviderFailure(w http.ResponseWriter, err error) {
	status, code := http.StatusBadGateway, "upstream_error"
	var upstream *httpx.StatusError
	if errors.As(err, &upstream) {
		switch upstream.StatusCode {
		case http.StatusBadRequest:
			status, code = http.StatusBadRequest, "invalid_request_error"
		case http.StatusUnauthorized:
			status, code = http.StatusUnauthorized, "provider_unauthorized"
		case http.StatusForbidden:
			status, code = http.StatusForbidden, "provider_forbidden"
		case http.StatusNotFound:
			status, code = http.StatusNotFound, "model_not_found"
		case http.StatusRequestEntityTooLarge:
			status, code = http.StatusRequestEntityTooLarge, "request_too_large"
		case http.StatusTooManyRequests:
			status, code = http.StatusTooManyRequests, "rate_limit_exceeded"
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			status, code = http.StatusGatewayTimeout, "upstream_timeout"
		}
	} else {
		switch platform.AsError(err).Kind {
		case platform.KindRateLimited:
			status, code = http.StatusTooManyRequests, "rate_limit_exceeded"
		case platform.KindTimeout:
			status, code = http.StatusGatewayTimeout, "upstream_timeout"
		case platform.KindUnauthorized:
			status, code = http.StatusUnauthorized, "provider_unauthorized"
		case platform.KindForbidden:
			status, code = http.StatusForbidden, "provider_forbidden"
		}
	}
	message := strings.TrimSpace(err.Error())
	if status == http.StatusTooManyRequests || status == http.StatusRequestEntityTooLarge {
		w.Header().Set("Retry-After", "20")
		message += " (the provider's per-minute token limit was exceeded; reduce the prompt or tool payload, or upgrade the provider plan)"
	}
	writeOAError(w, status, code, message)
}
