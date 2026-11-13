package memory

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Recent returns recent memories without a vector search.
func (s *Service) Recent(ctx context.Context, userID string, limit int) ([]domain.Memory, error) {
	return s.memories.RecentByUser(ctx, userID, limit)
}

// List exposes memory browsing for the dashboard/admin API.
func (s *Service) List(ctx context.Context, userID string, filter ports.MemoryFilter, page platform.Page) ([]domain.Memory, int, error) {
	return s.memories.ListByUser(ctx, userID, filter, page)
}

// Get loads one memory with ownership enforcement.
func (s *Service) Get(ctx context.Context, userID, id string) (*domain.Memory, error) {
	m, err := s.memories.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID != "" && m.UserID != userID {
		return nil, platform.Forbidden("memory.forbidden", "memory belongs to another user")
	}
	return m, nil
}

// Delete removes a memory from SQL and the vector index.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	m, err := s.memories.Get(ctx, id)
	if err != nil {
		return err
	}
	if userID != "" && m.UserID != userID {
		return platform.Forbidden("memory.forbidden", "memory belongs to another user")
	}
	if err := s.memories.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.vector.Delete(ctx, []string{id})
	return nil
}

// DeleteAll wipes a user's memories (GDPR endpoint).
func (s *Service) DeleteAll(ctx context.Context, userID string) (int, error) {
	all, err := s.memories.ListWithEmbeddings(ctx, userID, 20000)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(all))
	for _, m := range all {
		ids = append(ids, m.ID)
	}
	if len(ids) > 0 {
		_ = s.vector.Delete(ctx, ids)
	}
	return s.memories.DeleteByUser(ctx, userID)
}

// Warm reindexes stored embeddings at boot so semantic recall works immediately.
func (s *Service) Warm(ctx context.Context) error {
	if !s.cfg.SemanticEnabled {
		return nil
	}
	rows, err := s.memories.ListWithEmbeddings(ctx, "", 20000)
	if err != nil {
		return err
	}
	points := make([]ports.VectorPoint, 0, len(rows))
	for _, m := range rows {
		if len(m.Embedding) != s.dim {
			continue
		}
		points = append(points, ports.VectorPoint{ID: m.ID, UserID: m.UserID, Vector: m.Embedding,
			Payload: map[string]any{"kind": string(m.Kind), "tier": string(m.Tier), "preview": m.Summary}})
	}
	if len(points) == 0 {
		return nil
	}
	if err := s.vector.Upsert(ctx, points); err != nil {
		return err
	}
	s.log.Info("semantic memory index warmed", "vectors", len(points))
	return nil
}

// Prune enforces TTL and retention policies.
func (s *Service) Prune(ctx context.Context) (int, int, error) {
	expired, err := s.memories.PruneExpired(ctx, time.Now().UTC(), 500)
	if err != nil {
		return 0, 0, err
	}
	old := 0
	if s.cfg.RetentionDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -s.cfg.RetentionDays)
		old, err = s.memories.PruneOld(ctx, cutoff, 500)
		if err != nil {
			return expired, 0, err
		}
	}
	return expired, old, nil
}

func (s *Service) embedOne(ctx context.Context, text string) ([]float32, error) {
	vecs, err := s.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	if len(vecs[0]) != s.dim {
		return nil, platform.Invalid("memory.dim", "embedding dimension does not match vector dimension")
	}
	return vecs[0], nil
}
