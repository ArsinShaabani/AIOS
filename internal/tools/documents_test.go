package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type fakeDocuments struct{ hits []domain.DocumentMatch }

func (f *fakeDocuments) Search(context.Context, string, string, int) ([]domain.DocumentMatch, error) {
	return f.hits, nil
}

func TestDocumentSearchTool(t *testing.T) {
	reg := NewRegistry()
	api := &fakeDocuments{hits: []domain.DocumentMatch{
		{DocumentID: "doc1", Title: "Policy", ChunkIndex: 2, Score: 0.82, Content: "Refunds take five days."},
	}}
	if err := RegisterDocumentTool(reg, api); err != nil {
		t.Fatal(err)
	}
	res, err := reg.Execute(context.Background(), "document_search", ports.ToolCallContext{
		UserID: "u1", Arguments: map[string]any{"query": "refund", "limit": 3},
	})
	if err != nil || res.IsError {
		t.Fatalf("execute: %v %+v", err, res)
	}
	if !containsSubstring(res.Content, "Policy") || !containsSubstring(res.Content, "Refunds take five days") {
		t.Fatalf("content: %s", res.Content)
	}
	raw, _ := json.Marshal(res.Data)
	if !containsSubstring(string(raw), "doc1") || !containsSubstring(string(raw), "Policy") {
		t.Fatalf("data: %s", raw)
	}
	if res, _ := reg.Execute(context.Background(), "document_search", ports.ToolCallContext{}); !res.IsError {
		t.Fatal("missing query must be a tool error")
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOfString(haystack, needle) >= 0)
}

func indexOfString(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
