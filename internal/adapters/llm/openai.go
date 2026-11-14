package llm

import (
	"context"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// OpenAI implements the OpenAI chat/embeddings wire format. It also serves every
// OpenAI-compatible backend: DeepSeek, Mistral, LM Studio, vLLM, llama.cpp
// server and Ollama's /v1 compatibility layer.
type OpenAI struct {
	name    string
	baseURL string
	apiKey  string
	headers map[string]string
	models  []string
	client  *http.Client
}

var _ ports.Provider = (*OpenAI)(nil)

// NewOpenAI builds an OpenAI-compatible provider.
func NewOpenAI(cfg config.ProviderConfig) *OpenAI {
	timeout := cfg.Timeout.Std()
	models := make([]string, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		models = append(models, m.Name)
	}
	if len(models) == 0 && cfg.DefaultModel != "" {
		models = []string{cfg.DefaultModel}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.APIKey,
		"User-Agent":    "AIOS/1.0",
	}
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	return &OpenAI{
		name:    cfg.Name,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		headers: headers,
		models:  models,
		client:  httpx.NewClient(timeout),
	}
}

func (o *OpenAI) Name() string { return o.name }

func (o *OpenAI) Capabilities() ports.Capabilities {
	return ports.Capabilities{Tools: true, Streaming: true, Embeddings: true}
}

func (o *OpenAI) Models() []string { return o.models }

func (o *OpenAI) baseReq(req ports.ChatRequest) oaRequest {
	body := oaRequest{
		Model:           firstOr(req.Model, firstOr(o.firstModel(), "gpt-4o-mini")),
		Messages:        toOAMessages(req.Messages),
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxTokens:       req.MaxTokens,
		Tools:           toOpenAITools(req.Tools),
		Stop:            req.Stop,
		User:            req.User,
		ReasoningEffort: req.ReasoningEffort,
	}
	if req.ToolChoice != "" && len(body.Tools) > 0 {
		body.ToolChoice = req.ToolChoice
	}
	if req.ResponseFormat == "json" {
		body.ResponseFormat = map[string]any{"type": "json_object"}
	}
	return body
}

func (o *OpenAI) firstModel() string {
	if len(o.models) > 0 {
		return o.models[0]
	}
	return ""
}

// Chat performs a blocking completion.
func (o *OpenAI) Chat(ctx context.Context, req ports.ChatRequest) (*ports.ChatResponse, error) {
	body := o.baseReq(req)
	var out oaResponse
	if err := httpx.DoJSON(ctx, o.client, http.MethodPost, o.baseURL+"/chat/completions", o.headers, body, &out, 8<<20); err != nil {
		return nil, providerErr(o.name, err)
	}
	if out.Error != nil {
		return nil, providerErr(o.name, &httpx.StatusError{StatusCode: http.StatusBadRequest, Body: out.Error.Message, URL: o.baseURL})
	}
	if len(out.Choices) == 0 {
		return nil, providerErr(o.name, &httpx.StatusError{StatusCode: http.StatusBadGateway, Body: "empty completion", URL: o.baseURL})
	}
	choice := out.Choices[0]
	resp := &ports.ChatResponse{
		Provider:     o.name,
		Model:        firstOr(out.Model, body.Model),
		Content:      choice.Message.Content,
		ToolCalls:    toolCallsToDomain(choice.Message.ToolCalls),
		FinishReason: normaliseFinish(choice.FinishReason),
		Usage: ports.Usage{
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			TotalTokens:      out.Usage.TotalTokens,
		},
	}
	if resp.ToolCalls != nil && resp.FinishReason == "stop" {
		resp.FinishReason = "tool_calls"
	}
	return resp, nil
}

// Embed creates embeddings via /embeddings.
func (o *OpenAI) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	body := map[string]any{"model": model, "input": inputs}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := httpx.DoJSON(ctx, o.client, http.MethodPost, o.baseURL+"/embeddings", o.headers, body, &out, 32<<20); err != nil {
		return nil, providerErr(o.name, err)
	}
	vectors := make([][]float32, len(inputs))
	for _, d := range out.Data {
		if d.Index >= 0 && d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}
	return vectors, nil
}

// Ping verifies connectivity and credentials.
func (o *OpenAI) Ping(ctx context.Context) error {
	err := httpx.DoJSON(ctx, o.client, http.MethodGet, o.baseURL+"/models", o.headers, nil, nil, 1<<20)
	if err != nil {
		if se, ok := err.(*httpx.StatusError); ok && se.StatusCode == http.StatusNotFound {
			// Some compatible servers do not expose /models; the API is still up.
			return nil
		}
		return providerErr(o.name, err)
	}
	return nil
}
