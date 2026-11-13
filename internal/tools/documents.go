package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// DocumentAPI is the narrow RAG retrieval surface.
type DocumentAPI interface {
	Search(ctx context.Context, userID, query string, topK int) ([]domain.DocumentMatch, error)
}

// RegisterDocumentTool adds document_search backed by the RAG index.
func RegisterDocumentTool(reg *Registry, api DocumentAPI) error {
	if api == nil {
		return nil
	}
	return reg.Register(ports.Tool{
		Name:        "document_search",
		Description: "Search the user's uploaded documents and return relevant passages with citations (title, document id, chunk index, score).",
		Parameters: Schema(map[string]any{
			"query": map[string]any{"type": "string", "description": "What to look for in the documents"},
			"limit": map[string]any{"type": "integer", "description": "Max passages (1-10)", "default": 5},
		}, "query"),
		Permissions: []string{"memory:read"},
		Timeout:     20,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			query := stringArg(call.Arguments, "query")
			if query == "" {
				return ports.ToolResult{IsError: true, Content: "query is required"}, nil
			}
			limit := intArg(call.Arguments, "limit", 5)
			matches, err := api.Search(ctx, call.UserID, query, limit)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			type item struct {
				Title      string  `json:"title"`
				DocumentID string  `json:"document_id"`
				ChunkIndex int     `json:"chunk_index"`
				Score      float64 `json:"score"`
				Content    string  `json:"content"`
			}
			out := make([]item, 0, len(matches))
			var b strings.Builder
			for i, m := range matches {
				out = append(out, item{Title: m.Title, DocumentID: m.DocumentID,
					ChunkIndex: m.ChunkIndex, Score: m.Score, Content: m.Content})
				fmt.Fprintf(&b, "\n[%d] %s (chunk %d, score %.2f)\n%s\n", i+1, m.Title, m.ChunkIndex, m.Score, m.Content)
			}
			if len(out) == 0 {
				return ports.ToolResult{Content: "no matching document passages"}, nil
			}
			return ports.ToolResult{Content: strings.TrimSpace(b.String()), Data: map[string]any{"matches": out}}, nil
		},
	})
}
