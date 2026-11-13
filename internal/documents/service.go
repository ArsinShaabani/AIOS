// Package documents implements document RAG: ingestion, extraction, chunking,
// embedding and citation-aware retrieval over the shared vector store.
package documents

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Embedder produces vectors (the model router implements this).
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

// Service coordinates documents, chunks and the vector index.
type Service struct {
	repo     ports.DocumentRepository
	vector   ports.VectorStore
	embedder Embedder
	cfg      config.RAGConfig
	dim      int
	log      *slog.Logger
}

// New builds the service.
func New(repo ports.DocumentRepository, vector ports.VectorStore, embedder Embedder,
	cfg config.RAGConfig, dim int, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if dim <= 0 {
		dim = 384
	}
	return &Service{repo: repo, vector: vector, embedder: embedder, cfg: cfg, dim: dim, log: log}
}

// Ingest extracts text, chunks it, embeds the chunks and indexes the document.
func (s *Service) Ingest(ctx context.Context, userID, title, source, mime string, raw []byte) (*domain.Document, error) {
	if !s.cfg.Enabled {
		return nil, platform.Unavailable("documents.disabled", "document ingestion is disabled")
	}
	if userID == "" {
		return nil, platform.Unauthorized("documents.user", "authentication required")
	}
	if len(raw) == 0 {
		return nil, platform.Invalid("documents.empty", "document is empty")
	}
	if limit := s.cfg.MaxDocumentBytes; limit > 0 && int64(len(raw)) > limit {
		return nil, platform.Invalid("documents.too_large", "document exceeds rag.max_document_bytes")
	}
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime == "" {
		mime = "text/plain"
	}
	text := ExtractText(string(raw), mime)
	if strings.TrimSpace(text) == "" {
		return nil, platform.Invalid("documents.no_text", "no extractable text found in the document")
	}
	chunks := ChunkText(text, s.cfg.ChunkSize, s.cfg.ChunkOverlap, s.cfg.MaxChunks)
	if len(chunks) == 0 {
		return nil, platform.Invalid("documents.no_chunks", "document produced no chunks")
	}
	vectors, err := s.embed(ctx, chunks)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if strings.TrimSpace(title) == "" {
		title = defaultTitle(source, text)
	}
	doc := &domain.Document{
		ID: platform.NewID("doc"), UserID: userID, Title: platform.Truncate(title, 200),
		Source: platform.Truncate(source, 500), Mime: mime, Size: int64(len(raw)),
		Status: domain.DocumentIndexed, ChunkCount: len(chunks), CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, err
	}
	rows := make([]domain.DocumentChunk, len(chunks))
	points := make([]ports.VectorPoint, len(chunks))
	for i, content := range chunks {
		rows[i] = domain.DocumentChunk{
			ID: platform.NewID("chk"), DocumentID: doc.ID, UserID: userID, Index: i,
			Content: content, Tokens: estimateTokens(content), CreatedAt: now,
		}
		points[i] = ports.VectorPoint{
			ID: rows[i].ID, UserID: userID, Vector: vectors[i],
			Payload: map[string]any{
				"document_id": doc.ID, "title": doc.Title, "chunk_index": i,
				"preview": platform.Truncate(content, 160), "kind": "document",
			},
		}
	}
	if err := s.repo.CreateChunks(ctx, rows); err != nil {
		_ = s.repo.DeleteDocument(ctx, doc.ID)
		return nil, err
	}
	if err := s.vector.Upsert(ctx, points); err != nil {
		doc.Status = domain.DocumentError
		doc.Error = "vector index unavailable"
		_ = s.repo.UpdateDocument(ctx, doc)
		s.log.Warn("document stored without vectors", "document_id", doc.ID, "error", err)
		return doc, nil
	}
	return doc, nil
}

func (s *Service) embed(ctx context.Context, chunks []string) ([][]float32, error) {
	if s.embedder == nil {
		return nil, platform.Unavailable("documents.embedder", "no embedding provider is configured")
	}
	batch := s.cfg.EmbedBatch
	if batch <= 0 {
		batch = 16
	}
	out := make([][]float32, len(chunks))
	for start := 0; start < len(chunks); start += batch {
		end := min(start+batch, len(chunks))
		vectors, err := s.embedder.Embed(ctx, chunks[start:end])
		if err != nil {
			return nil, err
		}
		if len(vectors) != end-start {
			return nil, platform.Internal("documents.embed_count", "embedding provider returned a mismatched count")
		}
		for i, vec := range vectors {
			if len(vec) != s.dim {
				return nil, platform.Invalid("documents.embed_dim", "embedding dimension does not match vector.dim")
			}
			out[start+i] = vec
		}
	}
	return out, nil
}

func defaultTitle(source, text string) string {
	if strings.TrimSpace(source) != "" {
		return source
	}
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	title := platform.Truncate(line, 80)
	if title == "" {
		return "Untitled document"
	}
	return title
}

func estimateTokens(text string) int {
	return max(len([]rune(text))/4, 1)
}
