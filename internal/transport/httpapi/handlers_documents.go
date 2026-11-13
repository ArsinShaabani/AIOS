package httpapi

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

type ingestDocumentRequest struct {
	Title   string `json:"title"`
	Source  string `json:"source"`
	Mime    string `json:"mime"`
	Content string `json:"content"`
}

func (a *API) handleIngestDocument(w http.ResponseWriter, r *http.Request) {
	if a.Documents == nil {
		a.fail(w, r, platform.Unavailable("documents.disabled", "document ingestion is disabled"))
		return
	}
	user := userFrom(r.Context())
	maxBytes := a.Config.RAG.MaxDocumentBytes
	contentType := r.Header.Get("Content-Type")

	var (
		title, source, mime string
		raw                 []byte
		err                 error
	)
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
			a.fail(w, r, platform.Invalid("documents.form", "invalid multipart upload"))
			return
		}
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			a.fail(w, r, platform.Invalid("documents.file", "file field is required"))
			return
		}
		defer file.Close()
		if header.Size > maxBytes {
			a.fail(w, r, platform.Invalid("documents.too_large", "document exceeds rag.max_document_bytes"))
			return
		}
		raw, err = io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if int64(len(raw)) > maxBytes {
			a.fail(w, r, platform.Invalid("documents.too_large", "document exceeds rag.max_document_bytes"))
			return
		}
		title = r.FormValue("title")
		source = header.Filename
		mime = header.Header.Get("Content-Type")
		if mime == "" || mime == "application/octet-stream" {
			mime = mimeForFile(header.Filename)
		}
	} else {
		var req ingestDocumentRequest
		if err := decodeJSON(w, r, &req); err != nil {
			a.fail(w, r, err)
			return
		}
		title, source, mime = req.Title, req.Source, req.Mime
		raw = []byte(req.Content)
	}
	doc, err := a.Documents.Ingest(r.Context(), user.ID, title, source, mime, raw)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "document.ingest", "document", doc.ID, "success",
		clientIP(r), r.UserAgent(), map[string]any{"title": doc.Title, "chunks": doc.ChunkCount, "bytes": doc.Size})
	writeJSON(w, http.StatusCreated, doc)
}

func (a *API) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	if a.Documents == nil {
		a.fail(w, r, platform.Unavailable("documents.disabled", "documents are disabled"))
		return
	}
	user := userFrom(r.Context())
	page := platform.Page{Limit: queryInt(r, "limit", 25), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Documents.List(r.Context(), user.ID, page)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

func (a *API) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	if a.Documents == nil {
		a.fail(w, r, platform.Unavailable("documents.disabled", "documents are disabled"))
		return
	}
	user := userFrom(r.Context())
	doc, err := a.Documents.Get(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := map[string]any{"document": doc}
	if r.URL.Query().Get("chunks") == "true" {
		chunks, err := a.Documents.Chunks(r.Context(), user.ID, doc.ID, queryInt(r, "chunk_limit", 100))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		out["chunks"] = chunks
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleDocumentChunks(w http.ResponseWriter, r *http.Request) {
	if a.Documents == nil {
		a.fail(w, r, platform.Unavailable("documents.disabled", "documents are disabled"))
		return
	}
	user := userFrom(r.Context())
	chunks, err := a.Documents.Chunks(r.Context(), user.ID, r.PathValue("id"), queryInt(r, "limit", 100))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": chunks})
}

func (a *API) handleSearchDocuments(w http.ResponseWriter, r *http.Request) {
	if a.Documents == nil {
		a.fail(w, r, platform.Unavailable("documents.disabled", "documents are disabled"))
		return
	}
	user := userFrom(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		a.fail(w, r, platform.Invalid("documents.query", "q is required"))
		return
	}
	limit := queryInt(r, "limit", a.Config.RAG.TopK)
	matches, err := a.Documents.Search(r.Context(), user.ID, query, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": matches, "query": query})
}

func (a *API) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if a.Documents == nil {
		a.fail(w, r, platform.Unavailable("documents.disabled", "documents are disabled"))
		return
	}
	user := userFrom(r.Context())
	id := r.PathValue("id")
	if err := a.Documents.Delete(r.Context(), user.ID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "document.delete", "document", id, "success",
		clientIP(r), r.UserAgent(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func mimeForFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return "text/html"
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".txt", ".log":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	default:
		return "text/plain"
	}
}
