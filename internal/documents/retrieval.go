package documents

import (
	"context"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Search returns the most relevant chunks for a user's query with citations.
func (s *Service) Search(ctx context.Context, userID, query string, topK int) ([]domain.DocumentMatch, error) {
	if !s.cfg.Enabled {
		return nil, platform.Unavailable("documents.disabled", "document retrieval is disabled")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, platform.Invalid("documents.query", "query is required")
	}
	if topK <= 0 {
		topK = s.cfg.TopK
	}
	if topK > 50 {
		topK = 50
	}
	if s.embedder == nil {
		return nil, platform.Unavailable("documents.embedder", "no embedding provider is configured")
	}
	vecs, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) != s.dim {
		return nil, platform.Internal("documents.query_embed", "invalid query embedding")
	}
	minScore := s.cfg.MinScore
	if minScore <= 0 {
		minScore = 0.2
	}
	matches, err := s.vector.Search(ctx, userID, vecs[0], topK*2, minScore)
	if err != nil {
		return nil, err
	}
	out := make([]domain.DocumentMatch, 0, topK)
	seen := map[string]bool{}
	for _, m := range matches {
		if len(out) >= topK {
			break
		}
		chunk, err := s.repo.GetChunk(ctx, m.ID)
		if err != nil || chunk.UserID != userID {
			continue
		}
		if seen[chunk.ID] {
			continue
		}
		seen[chunk.ID] = true
		title, _ := m.Payload["title"].(string)
		if title == "" {
			if doc, err := s.repo.GetDocument(ctx, chunk.DocumentID); err == nil {
				title = doc.Title
			}
		}
		out = append(out, domain.DocumentMatch{
			DocumentID: chunk.DocumentID, Title: title, ChunkID: chunk.ID,
			ChunkIndex: chunk.Index, Content: chunk.Content, Score: m.Score,
		})
	}
	return out, nil
}

// List returns the user's documents.
func (s *Service) List(ctx context.Context, userID string, page platform.Page) ([]domain.Document, int, error) {
	return s.repo.ListDocuments(ctx, userID, page)
}

// Get loads a document with ownership enforcement.
func (s *Service) Get(ctx context.Context, userID, id string) (*domain.Document, error) {
	doc, err := s.repo.GetDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc.UserID != userID {
		return nil, platform.Forbidden("documents.forbidden", "document belongs to another user")
	}
	return doc, nil
}

// Chunks returns a document's chunks for the citation UI.
func (s *Service) Chunks(ctx context.Context, userID, id string, limit int) ([]domain.DocumentChunk, error) {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return nil, err
	}
	return s.repo.ListChunks(ctx, id, limit)
}

// Delete removes a document, its chunks and its vectors.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return err
	}
	chunks, err := s.repo.ListChunks(ctx, id, 1000)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		ids = append(ids, c.ID)
	}
	if len(ids) > 0 {
		_ = s.vector.Delete(ctx, ids)
	}
	return s.repo.DeleteDocument(ctx, id)
}

// Count returns the total document count (admin metrics).
func (s *Service) Count(ctx context.Context) (int, error) {
	return s.repo.CountDocuments(ctx)
}
