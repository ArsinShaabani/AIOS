package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/llm"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type chatAttachment struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Text string `json:"text"`
}

type chatRequest struct {
	Message       string             `json:"message"`
	SessionID     string             `json:"session_id"`
	AgentID       string             `json:"agent_id"`
	PersonalityID string             `json:"personality_id"`
	Channel       string             `json:"channel"`
	Stream        bool               `json:"stream"`
	Provider      string             `json:"provider,omitempty"`
	Model         string             `json:"model,omitempty"`
	Options       *brain.ChatOptions `json:"options"`
	Attachments   []chatAttachment   `json:"attachments"`
}

func (a *API) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	message := composeMessage(req.Message, req.Attachments)
	if len([]rune(message)) > 200000 {
		a.fail(w, r, platform.Invalid("chat.too_long", "message exceeds 200000 characters"))
		return
	}
	user := userFrom(r.Context())
	channel := domain.ChannelAPI
	switch req.Channel {
	case string(domain.ChannelWeb), string(domain.ChannelAPI), string(domain.ChannelVoice),
		string(domain.ChannelMQTT), string(domain.ChannelDevice), string(domain.ChannelCLI):
		channel = domain.Channel(req.Channel)
	}
	stream := req.Stream || wantStream(r)
	input := brain.ChatInput{
		UserID: user.ID, SessionID: req.SessionID, Message: message, Options: req.Options,
		AgentID: req.AgentID, PersonalityID: req.PersonalityID, Channel: channel, Stream: stream,
		Provider: req.Provider, Model: req.Model,
	}
	providerName := req.Provider
	modelName := req.Model
	if providerName == "" && len(a.Router.Providers()) == 0 {
		if list, err := a.UserAI.List(r.Context(), user.ID); err == nil && len(list) > 0 {
			providerName = list[0].Name
			modelName = list[0].DefaultModel
		}
	}
	if providerName != "" {
		p, pcfg, err := a.UserAI.Build(r.Context(), user.ID, providerName)
		if err != nil {
			a.fail(w, r, platform.Invalid("chat.provider", "provider is not available on your account"))
			return
		}
		model := modelName
		if model == "" {
			model = pcfg.DefaultModel
		}
		llmCfg := config.LLMConfig{DefaultProvider: pcfg.Name, DefaultModel: model,
			Temperature: a.Config.LLM.Temperature, TopP: a.Config.LLM.TopP,
			MaxTokens: pcfg.MaxTokens, Timeout: a.Config.LLM.Timeout,
			MaxRetries: 1, MaxToolIterations: a.Config.LLM.MaxToolIterations, Providers: []config.ProviderConfig{pcfg}}
		if llmCfg.MaxTokens <= 0 {
			llmCfg.MaxTokens = 4096
		}
		input.Router = llm.NewRouter(llmCfg, map[string]ports.Provider{pcfg.Name: p}, nil, nil, nil, a.Log)
		input.Model = model
	}
	if !stream {
		out, err := a.Brain.Chat(r.Context(), input, nil)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if a.Metrics != nil {
			a.Metrics.ObserveChat(out.AgentID, string(channel))
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	a.streamChat(w, r, input)
}

func (a *API) streamChat(w http.ResponseWriter, r *http.Request, input brain.ChatInput) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		a.fail(w, r, platform.Internal("http.sse", "streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	send := func(ev brain.Event) error {
		raw, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	out, err := a.Brain.Chat(r.Context(), input, send)
	if err != nil {
		_ = send(brain.Event{Type: "error", Error: platform.AsError(err).Message})
		return
	}
	if a.Metrics != nil {
		a.Metrics.ObserveChat(out.AgentID, string(input.Channel))
	}
	_ = send(brain.Event{Type: "done", SessionID: out.SessionID, MessageID: out.MessageID,
		Usage: &out.Usage, Agent: out.AgentID, Provider: out.Provider, Model: out.Model})
}

func (a *API) handleListSessions(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	page := platform.Page{Limit: queryInt(r, "limit", 25), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Repos.Sessions.ListByUser(r.Context(), user.ID, page)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

func (a *API) handleGetSession(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	session, err := a.Repos.Sessions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if session.UserID != user.ID {
		a.fail(w, r, platform.Forbidden("session.forbidden", "session belongs to another user"))
		return
	}
	writeJSON(w, http.StatusOK, session)
}

type patchSessionRequest struct {
	Title         *string `json:"title"`
	PersonalityID *string `json:"personality_id"`
	AgentID       *string `json:"agent_id"`
}

func (a *API) handlePatchSession(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	session, err := a.Repos.Sessions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if session.UserID != user.ID {
		a.fail(w, r, platform.Forbidden("session.forbidden", "session belongs to another user"))
		return
	}
	var req patchSessionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.Title != nil {
		session.Title = platform.Truncate(*req.Title, 120)
	}
	if req.PersonalityID != nil {
		session.PersonalityID = *req.PersonalityID
	}
	if req.AgentID != nil {
		session.AgentID = *req.AgentID
	}
	if err := a.Repos.Sessions.Update(r.Context(), session); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (a *API) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	session, err := a.Repos.Sessions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if session.UserID != user.ID {
		a.fail(w, r, platform.Forbidden("session.forbidden", "session belongs to another user"))
		return
	}
	if err := a.Repos.Sessions.Delete(r.Context(), session.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleSessionMessages(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	session, err := a.Repos.Sessions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if session.UserID != user.ID {
		a.fail(w, r, platform.Forbidden("session.forbidden", "session belongs to another user"))
		return
	}
	limit := queryInt(r, "limit", 100)
	items, err := a.Repos.Messages.ListRecent(r.Context(), session.ID, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "session": session, "server_time": time.Now().UTC()})
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

// composeMessage folds text attachments into the prompt. Binary/vision inputs are
// referenced by name only until a multimodal provider is configured.
func composeMessage(text string, files []chatAttachment) string {
	var b strings.Builder
	b.WriteString(text)
	for _, f := range files {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		t := f.Text
		if len(t) > 60000 {
			t = t[:60000] + "\n...(truncated)"
		}
		b.WriteString("\n\n--- attached file: ")
		b.WriteString(f.Name)
		b.WriteString(" (")
		b.WriteString(f.MIME)
		b.WriteString(") ---\n")
		b.WriteString(t)
	}
	return b.String()
}
