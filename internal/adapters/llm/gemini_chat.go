package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Chat performs a blocking generateContent call.
func (g *Gemini) Chat(ctx context.Context, req ports.ChatRequest) (*ports.ChatResponse, error) {
	body := g.build(req)
	var out geminiChunk
	if err := httpx.DoJSON(ctx, g.client, http.MethodPost, g.endpoint(req.Model, "generateContent"), nil, body, &out, 8<<20); err != nil {
		return nil, providerErr(g.name, err)
	}
	if out.Error != nil {
		return nil, providerErr(g.name, &httpx.StatusError{StatusCode: http.StatusBadRequest, Body: out.Error.Message, URL: g.baseURL})
	}
	if len(out.Candidates) == 0 {
		return nil, providerErr(g.name, &httpx.StatusError{StatusCode: http.StatusBadGateway, Body: "empty candidates", URL: g.baseURL})
	}
	cand := out.Candidates[0]
	resp := &ports.ChatResponse{
		Provider:     g.name,
		Model:        stripModels(firstOr(req.Model, g.defaultModel())),
		FinishReason: normaliseFinish(strings.ToLower(cand.FinishReason)),
		Usage: ports.Usage{
			PromptTokens:     out.UsageMetadata.PromptTokenCount,
			CompletionTokens: out.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      out.UsageMetadata.TotalTokenCount,
		},
	}
	var text strings.Builder
	for i, part := range cand.Content.Parts {
		if part.Text != "" {
			text.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			resp.ToolCalls = append(resp.ToolCalls, domain.ToolCall{
				ID:        fmt.Sprintf("%s_%d", part.FunctionCall.Name, i),
				Name:      part.FunctionCall.Name,
				Arguments: part.FunctionCall.Args,
			})
		}
	}
	resp.Content = text.String()
	if len(resp.ToolCalls) > 0 {
		resp.FinishReason = "tool_calls"
	}
	return resp, nil
}

// Embed calls batchEmbedContents.
func (g *Gemini) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	model = stripModels(firstOr(model, "text-embedding-004"))
	reqs := make([]map[string]any, 0, len(inputs))
	for _, in := range inputs {
		reqs = append(reqs, map[string]any{
			"model":   "models/" + model,
			"content": map[string]any{"parts": []map[string]any{{"text": in}}},
		})
	}
	var out struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := httpx.DoJSON(ctx, g.client, http.MethodPost, g.endpoint(model, "batchEmbedContents"), nil, map[string]any{"requests": reqs}, &out, 32<<20); err != nil {
		return nil, providerErr(g.name, err)
	}
	vectors := make([][]float32, len(out.Embeddings))
	for i, e := range out.Embeddings {
		vectors[i] = e.Values
	}
	return vectors, nil
}

// Ping lists models to verify the key.
func (g *Gemini) Ping(ctx context.Context) error {
	return providerErr(g.name, httpx.DoJSON(ctx, g.client, http.MethodGet, g.baseURL+"/models?key="+url.QueryEscape(g.apiKey), nil, nil, nil, 1<<20))
}
