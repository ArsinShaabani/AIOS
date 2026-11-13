package e2e

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
)

func TestDocumentRAGLifecycle(t *testing.T) {
	e := newEnv(t)
	content := "# Refund policy\n\nRefunds are processed within five business days.\n\n" +
		"Shipping to Europe takes ten days."
	status, doc := e.do("POST", "/api/v1/documents", e.token, map[string]any{
		"title": "Policy", "mime": "text/markdown", "content": content,
	})
	if status != 201 {
		t.Fatalf("ingest: %d %v", status, doc)
	}
	id := doc["id"].(string)
	if doc["chunk_count"].(float64) < 1 || doc["status"] != "indexed" {
		t.Fatalf("document: %v", doc)
	}
	if status, list := e.do("GET", "/api/v1/documents?limit=10", e.token, nil); status != 200 || list["total"].(float64) != 1 {
		t.Fatalf("list: %d %v", status, list)
	}
	status, fetched := e.do("GET", "/api/v1/documents/"+id+"?chunks=true", e.token, nil)
	if status != 200 || len(fetched["chunks"].([]any)) < 1 {
		t.Fatalf("get with chunks: %d %v", status, fetched)
	}
	if status, chunks := e.do("GET", "/api/v1/documents/"+id+"/chunks", e.token, nil); status != 200 || len(chunks["items"].([]any)) < 1 {
		t.Fatalf("chunks: %d %v", status, chunks)
	}
	status, results := e.do("GET", "/api/v1/documents/search?q=refund%20policy&limit=3", e.token, nil)
	if status != 200 || len(results["items"].([]any)) == 0 {
		t.Fatalf("search: %d %v", status, results)
	}
	hit := results["items"].([]any)[0].(map[string]any)
	if hit["document_id"] != id || hit["title"] != "Policy" || hit["content"] == "" {
		t.Fatalf("citation: %v", hit)
	}
	if status, _ := e.do("GET", "/api/v1/documents/search", e.token, nil); status != 400 {
		t.Fatalf("missing query: %d", status)
	}

	// Multipart upload path.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "notes.md")
	_, _ = part.Write([]byte("# Notes\n\nWarranty lasts two years."))
	_ = mw.Close()
	req := newUploadRequest(t, e, "/api/v1/documents", mw.FormDataContentType(), &body)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("multipart ingest: %d", resp.StatusCode)
	}

	// RBAC and isolation.
	_, created := e.do("POST", "/api/v1/admin/users", e.token, map[string]any{
		"email": "docs-user@test.local", "password": "DocsPassword123!", "role": "user",
	})
	_ = created
	other := e.login("docs-user@test.local", "DocsPassword123!")
	if status, _ := e.do("GET", "/api/v1/documents/"+id, other, nil); status != 403 {
		t.Fatalf("document isolation: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/documents", other, map[string]any{"content": "mine"}); status != 201 {
		t.Fatalf("user ingest: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/documents/"+id, other, nil); status != 403 {
		t.Fatalf("foreign delete: %d", status)
	}

	if status, _ := e.do("DELETE", "/api/v1/documents/"+id, e.token, nil); status != 204 {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/documents/"+id, e.token, nil); status != 404 {
		t.Fatalf("deleted document: %d", status)
	}
}

func TestDocumentIngestLimits(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.RAG.MaxDocumentBytes = 1024 })
	if status, _ := e.do("POST", "/api/v1/documents", e.token, map[string]any{"content": "short"}); status != 201 {
		t.Fatalf("small doc: %d", status)
	}
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'a'
	}
	if status, _ := e.do("POST", "/api/v1/documents", e.token, map[string]any{"content": string(big)}); status != 400 && status != 413 {
		t.Fatalf("oversized doc accepted: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/documents", e.token, map[string]any{"content": "   "}); status != 400 {
		t.Fatalf("blank doc accepted: %d", status)
	}
}
