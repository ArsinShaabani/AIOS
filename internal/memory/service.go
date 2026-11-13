// Package memory implements AIOS' three-tier memory: the short-term working set
// (session messages), long-term facts/preferences and semantic vectors.
package memory

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

// Embedder produces vectors (implemented by the model router).
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

// Service coordinates durable memory, the vector index and extraction.
type Service struct {
	memories ports.MemoryRepository
	sessions ports.SessionRepository
	messages ports.MessageRepository
	users    ports.UserRepository
	vector   ports.VectorStore
	cache    ports.Cache
	embedder Embedder
	cfg      config.MemoryConfig
	dim      int
	log      *slog.Logger
}

// New builds the memory service.
func New(memories ports.MemoryRepository, sessions ports.SessionRepository, messages ports.MessageRepository,
	users ports.UserRepository, vector ports.VectorStore, cache ports.Cache, embedder Embedder,
	cfg config.MemoryConfig, dim int, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{memories: memories, sessions: sessions, messages: messages, users: users,
		vector: vector, cache: cache, embedder: embedder, cfg: cfg, dim: dim, log: log}
}

// Remember persists a memory and indexes its vector.
func (s *Service) Remember(ctx context.Context, m *domain.Memory) (*domain.Memory, error) {
	if strings.TrimSpace(m.Content) == "" {
		return nil, platform.Invalid("memory.empty", "memory content is required")
	}
	if m.UserID == "" {
		return nil, platform.Invalid("memory.user", "memory user is required")
	}
	if m.ID == "" {
		m.ID = platform.NewID("mem")
	}
	m.Content = strings.TrimSpace(m.Content)
	if m.Summary == "" {
		m.Summary = platform.Truncate(m.Content, 160)
	}
	if m.Tier == "" {
		m.Tier = domain.TierLong
	}
	if m.Kind == "" {
		m.Kind = domain.KindFact
	}
	if m.Importance <= 0 {
		m.Importance = 0.5
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now

	if s.cfg.SemanticEnabled && s.embedder != nil && m.Tier != domain.TierShort {
		vec, err := s.embedOne(ctx, m.Content)
		if err != nil {
			s.log.Warn("memory embedding failed; storing without vector", "error", err)
		} else if vec != nil {
			m.Embedding = vec
			_ = s.vector.Upsert(ctx, []ports.VectorPoint{{
				ID: m.ID, UserID: m.UserID, Vector: vec,
				Payload: map[string]any{"kind": string(m.Kind), "tier": string(m.Tier), "preview": m.Summary},
			}})
		}
	}
	if err := s.memories.Create(ctx, m); err != nil {
		_ = s.vector.Delete(ctx, []string{m.ID})
		return nil, err
	}
	return m, nil
}

// RememberExtracted stores memories produced by Extract, de-duplicating within
// a 24h window.
func (s *Service) RememberExtracted(ctx context.Context, userID, sessionID, source string, found []domain.Memory) (int, error) {
	stored := 0
	for i := range found {
		m := found[i]
		m.UserID = userID
		m.SessionID = sessionID
		m.Source = source
		if s.isDuplicate(ctx, userID, m.Content) {
			continue
		}
		if _, err := s.Remember(ctx, &m); err != nil {
			s.log.Warn("cannot store memory", "error", err)
			continue
		}
		if s.cache != nil {
			_ = s.cache.Set(ctx, dupKey(userID, m.Content), []byte("1"), 24*time.Hour)
		}
		stored++
	}
	return stored, nil
}

func (s *Service) isDuplicate(ctx context.Context, userID, content string) bool {
	if s.cache == nil {
		return false
	}
	_, ok, err := s.cache.Get(ctx, dupKey(userID, content))
	return ok && err == nil
}

// Recall returns the most relevant memories for a query: semantic matches first,
// then recent durable memories (preferences/facts).
func (s *Service) Recall(ctx context.Context, userID, query string, topK int) ([]domain.Memory, error) {
	if userID == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = s.cfg.RetrievalTopK
	}
	out := make([]domain.Memory, 0, topK*2)
	seen := map[string]struct{}{}

	if s.cfg.SemanticEnabled && s.embedder != nil && strings.TrimSpace(query) != "" {
		vec, err := s.embedOne(ctx, query)
		if err == nil && vec != nil {
			matches, searchErr := s.vector.Search(ctx, userID, vec, topK, s.cfg.MinSimilarity)
			if searchErr != nil {
				s.log.Warn("vector search failed; falling back to recency", "error", searchErr)
			}
			for _, match := range matches {
				if _, ok := seen[match.ID]; ok {
					continue
				}
				m, err := s.memories.Get(ctx, match.ID)
				if err != nil || m.UserID != userID || m.Expired(time.Now()) {
					continue
				}
				seen[m.ID] = struct{}{}
				out = append(out, *m)
			}
		}
	}
	recent, err := s.memories.RecentByUser(ctx, userID, max(topK, 8))
	if err != nil {
		return out, err
	}
	for _, m := range recent {
		if len(out) >= topK*2 {
			break
		}
		if _, ok := seen[m.ID]; ok {
			continue
		}
		if m.Expired(time.Now()) {
			continue
		}
		seen[m.ID] = struct{}{}
		out = append(out, m)
	}
	if len(out) > topK*2 {
		out = out[:topK*2]
	}
	ids := make([]string, 0, len(out))
	for _, m := range out {
		ids = append(ids, m.ID)
	}
	if len(ids) > 0 {
		_ = s.memories.IncrementUse(ctx, ids, time.Now().UTC())
	}
	return out, nil
}
