package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Gemini implements Google's generateContent API.
type Gemini struct {
	name    string
	baseURL string
	apiKey  string
	models  []string
	client  *http.Client
}

var _ ports.Provider = (*Gemini)(nil)

// NewGemini builds a Gemini provider.
func NewGemini(cfg config.ProviderConfig) *Gemini {
	models := make([]string, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		models = append(models, m.Name)
	}
	if len(models) == 0 && cfg.DefaultModel != "" {
		models = []string{cfg.DefaultModel}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &Gemini{name: cfg.Name, baseURL: base, apiKey: cfg.APIKey, models: models, client: httpx.NewClient(cfg.Timeout.Std())}
}

func (g *Gemini) Name() string { return g.name }

func (g *Gemini) Capabilities() ports.Capabilities {
	return ports.Capabilities{Tools: true, Streaming: true, Embeddings: true, Vision: true}
}

func (g *Gemini) Models() []string { return g.models }

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  map[string]any   `json:"generationConfig,omitempty"`
	Tools             []map[string]any `json:"tools,omitempty"`
}

type geminiChunk struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func stripModels(model string) string { return strings.TrimPrefix(model, "models/") }

func (g *Gemini) build(req ports.ChatRequest) geminiRequest {
	out := geminiRequest{}
	var system strings.Builder
	for _, m := range req.Messages {
		switch m.Role {
		case domain.MsgSystem:
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(m.Content)
		case domain.MsgTool:
			name := m.Name
			if name == "" {
				name = m.ToolCallID
			}
			out.Contents = append(out.Contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{FunctionResponse: &geminiFunctionResponse{
					Name:     name,
					Response: map[string]any{"result": m.Content},
				}}},
			})
		case domain.MsgAssistant:
			parts := make([]geminiPart, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				parts = append(parts, geminiPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{Name: tc.Name, Args: tc.Arguments}})
			}
			if len(parts) == 0 {
				parts = append(parts, geminiPart{Text: "(empty)"})
			}
			out.Contents = append(out.Contents, geminiContent{Role: "model", Parts: parts})
		default:
			out.Contents = append(out.Contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: m.Content}}})
		}
	}
	if system.Len() > 0 {
		out.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: system.String()}}}
	}
	gen := map[string]any{}
	if req.Temperature > 0 {
		gen["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		gen["topP"] = req.TopP
	}
	if req.MaxTokens > 0 {
		gen["maxOutputTokens"] = req.MaxTokens
	}
	if len(gen) > 0 {
		out.GenerationConfig = gen
	}
	if len(req.Tools) > 0 {
		decls := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			var params any = map[string]any{"type": "object", "properties": map[string]any{}}
			if len(t.Parameters) > 0 {
				_ = json.Unmarshal(t.Parameters, &params)
			}
			decls = append(decls, map[string]any{"name": t.Name, "description": t.Description, "parameters": params})
		}
		out.Tools = []map[string]any{{"functionDeclarations": decls}}
	}
	return out
}

func (g *Gemini) endpoint(model, method string) string {
	model = stripModels(firstOr(model, g.defaultModel()))
	return fmt.Sprintf("%s/models/%s:%s?key=%s", g.baseURL, model, method, url.QueryEscape(g.apiKey))
}

func (g *Gemini) defaultModel() string {
	if len(g.models) > 0 {
		return g.models[0]
	}
	return "gemini-2.0-flash"
}
