package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (a *API) handleTunnelChat(w http.ResponseWriter, r *http.Request) {
	user, key, ok := a.tunnelAuth(w, r)
	if !ok {
		return
	}
	var req oaRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeOAError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeOAError(w, http.StatusBadRequest, "invalid_request", "messages are required")
		return
	}
	providerName, model := "", ""
	if key != nil {
		providerName, model = key.Provider, key.Model
	}
	if providerName == "" && strings.Contains(req.Model, "/") {
		parts := strings.SplitN(req.Model, "/", 2)
		providerName, model = parts[0], parts[1]
	}
	provider, cfg, err := a.UserAI.Build(r.Context(), user.ID, providerName)
	if err != nil {
		writeOAError(w, http.StatusBadGateway, "provider_unavailable", err.Error())
		return
	}
	if model == "" {
		model = req.Model
	}
	if model == "" {
		model = cfg.DefaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = req.MaxCompletionTokens
	}
	modelCap := 0
	for _, m := range cfg.Models {
		if m.Name == model && m.MaxOutputTokens > 0 {
			modelCap = m.MaxOutputTokens
			break
		}
	}
	if maxTokens <= 0 {
		maxTokens = modelCap
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	if modelCap > 0 && maxTokens > modelCap {
		maxTokens = modelCap
	}
	temperature := 0.7
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	topP := 0.9
	if req.TopP != nil {
		topP = *req.TopP
	}
	chatReq := ports.ChatRequest{
		Model: model, Messages: oaMessages(req.Messages), Temperature: temperature,
		TopP: topP, MaxTokens: maxTokens, Stream: req.Stream,
		Metadata: map[string]string{"user_id": user.ID, "via": "tunnel"},
	}
	for _, t := range req.Tools {
		if t.Function.Name == "" {
			continue
		}
		chatReq.Tools = append(chatReq.Tools, ports.ToolSpec{
			Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters,
		})
	}
	if len(chatReq.Tools) > 0 {
		chatReq.ToolChoice = "auto"
	}
	if req.Stream {
		a.tunnelStream(w, r, provider, chatReq)
		return
	}
	resp, err := provider.Chat(r.Context(), chatReq)
	if err != nil {
		a.Log.Warn("tunnel upstream failure", "provider", providerName, "model", model, "error", err)
		writeProviderFailure(w, err)
		return
	}
	msg := map[string]any{"role": "assistant", "content": resp.Content}
	if len(resp.ToolCalls) > 0 {
		msg["tool_calls"] = oaToolCalls(resp.ToolCalls)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "chatcmpl-" + platform.NewID(""), "object": "chat.completion",
		"created": time.Now().Unix(), "model": model,
		"choices": []map[string]any{{
			"index": 0, "message": msg, "finish_reason": firstNonEmpty(resp.FinishReason, "stop"),
		}},
		"usage": map[string]any{
			"prompt_tokens": resp.Usage.PromptTokens, "completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens": resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	})
}

func (a *API) tunnelStream(w http.ResponseWriter, r *http.Request, provider ports.Provider, req ports.ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOAError(w, http.StatusInternalServerError, "stream_unsupported", "streaming is unavailable")
		return
	}
	ch, err := provider.Stream(r.Context(), req)
	if err != nil {
		a.Log.Warn("tunnel upstream failure", "model", req.Model, "error", err)
		writeProviderFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	id := "chatcmpl-" + platform.NewID("")
	send := func(delta map[string]any, finish string, usage *ports.Usage) {
		payload := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": req.Model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		if usage != nil {
			payload["usage"] = map[string]any{"prompt_tokens": usage.PromptTokens,
				"completion_tokens": usage.CompletionTokens, "total_tokens": usage.PromptTokens + usage.CompletionTokens}
		}
		raw, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", raw)
		flusher.Flush()
	}
	send(map[string]any{"role": "assistant"}, "", nil)
	sawTools := false
	for chunk := range ch {
		if chunk.Err != nil {
			raw, _ := json.Marshal(map[string]any{"error": map[string]any{"message": chunk.Err.Error(), "type": "upstream_error"}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
			break
		}
		if chunk.Delta != "" {
			send(map[string]any{"content": chunk.Delta}, "", nil)
		}
		if len(chunk.ToolCalls) > 0 {
			sawTools = true
			send(map[string]any{"tool_calls": oaToolCalls(chunk.ToolCalls)}, "", nil)
		}
		if chunk.Done {
			// OpenAI reports "tool_calls" when the turn ends on a tool call;
			// agents use it to decide whether to run the call or finish.
			finish := "stop"
			if sawTools {
				finish = "tool_calls"
			}
			send(map[string]any{}, finish, chunk.Usage)
			break
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func oaMessages(in []oaMessage) []ports.ChatMessage {
	out := make([]ports.ChatMessage, 0, len(in))
	for _, m := range in {
		msg := ports.ChatMessage{Role: domain.MessageRole(m.Role), Content: m.Content.String(),
			Name: m.Name, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			args := map[string]any{}
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			msg.ToolCalls = append(msg.ToolCalls, domain.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
		}
		out = append(out, msg)
	}
	return out
}

func oaToolCalls(in []domain.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for i, tc := range in {
		args, _ := json.Marshal(tc.Arguments)
		out = append(out, map[string]any{
			"index": i, "id": firstNonEmpty(tc.ID, platform.NewID("call")), "type": "function",
			"function": map[string]any{"name": tc.Name, "arguments": string(args)},
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
