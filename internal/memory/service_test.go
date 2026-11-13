package memory

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/cache"
	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/adapters/vector"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type fakeEmbedder struct{ err error }

func (f fakeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{0.5, 0.5, 0.1, 0, 0, 0, 0, 0}
	}
	return out, nil
}

func memoryService(t *testing.T, embedder Embedder, cfg config.MemoryConfig) (*Service, *sqlstore.Repositories) {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "memory.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })
	svc := New(repos.Memories, repos.Sessions, repos.Messages, repos.Users, vector.NewLocal(8),
		cache.NewMemory(100, time.Minute), embedder, cfg, 8, slog.Default())
	return svc, repos
}

func baseMemoryConfig() config.MemoryConfig {
	return config.MemoryConfig{MaxShortTurns: 10, LongTermEnabled: true, SemanticEnabled: true,
		RetrievalTopK: 5, MinSimilarity: 0.1, MaxPerUser: 100, MaxContextTokens: 1000}
}

func TestRememberRecallDelete(t *testing.T) {
	svc, _ := memoryService(t, fakeEmbedder{}, baseMemoryConfig())
	ctx := context.Background()
	m, err := svc.Remember(ctx, &domain.Memory{UserID: "u1", Content: "I like green tea", Kind: domain.KindPreference})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || len(m.Embedding) != 8 || m.Summary == "" {
		t.Fatalf("remember: %+v", m)
	}
	recalled, err := svc.Recall(ctx, "u1", "tea", 5)
	if err != nil || len(recalled) == 0 || !strings.Contains(recalled[0].Content, "tea") {
		t.Fatalf("recall: %v %+v", err, recalled)
	}
	recent, err := svc.Recent(ctx, "u1", 5)
	if err != nil || len(recent) != 1 {
		t.Fatalf("recent: %v %+v", err, recent)
	}
	if got, err := svc.Get(ctx, "u1", m.ID); err != nil || got.ID != m.ID {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Get(ctx, "other", m.ID); !platform.IsForbidden(err) {
		t.Fatalf("get ownership: %v", err)
	}
	if _, _, err := svc.List(ctx, "u1", ports.MemoryFilter{Kind: domain.KindPreference, Query: "tea"}, platform.Page{Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "other", m.ID); !platform.IsForbidden(err) {
		t.Fatalf("delete ownership: %v", err)
	}
	if err := svc.Delete(ctx, "u1", m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, "u1", m.ID); !platform.IsNotFound(err) {
		t.Fatalf("deleted memory readable: %v", err)
	}
	if _, err := svc.Remember(ctx, &domain.Memory{Content: "x"}); !platform.IsInvalid(err) {
		t.Fatalf("missing user accepted: %v", err)
	}
	if _, err := svc.Remember(ctx, &domain.Memory{UserID: "u1", Content: " "}); !platform.IsInvalid(err) {
		t.Fatalf("empty content accepted: %v", err)
	}
}

func TestRememberWithoutEmbeddingsAndErrors(t *testing.T) {
	cfg := baseMemoryConfig()
	svc, _ := memoryService(t, fakeEmbedder{err: errors.New("embed down")}, cfg)
	m, err := svc.Remember(context.Background(), &domain.Memory{UserID: "u1", Content: "survives without vector"})
	if err != nil || m.ID == "" {
		t.Fatalf("remember should tolerate embedding failure: %v", err)
	}
	if len(m.Embedding) != 0 {
		t.Fatal("embedding should be absent")
	}
	cfg.SemanticEnabled = false
	svc2, _ := memoryService(t, fakeEmbedder{}, cfg)
	if _, err := svc2.Remember(context.Background(), &domain.Memory{UserID: "u1", Content: "no semantic"}); err != nil {
		t.Fatal(err)
	}
}

func TestRememberExtractedDedupe(t *testing.T) {
	svc, repos := memoryService(t, fakeEmbedder{}, baseMemoryConfig())
	ctx := context.Background()
	found := []domain.Memory{{Content: "User likes tea", Kind: domain.KindPreference}}
	n, err := svc.RememberExtracted(ctx, "u1", "s1", "conversation", found)
	if err != nil || n != 1 {
		t.Fatalf("first extraction: %v %d", err, n)
	}
	n, err = svc.RememberExtracted(ctx, "u1", "s1", "conversation", found)
	if err != nil || n != 0 {
		t.Fatalf("duplicate not deduped: %v %d", err, n)
	}
	if count, _ := repos.Memories.CountByUser(ctx, "u1"); count != 1 {
		t.Fatalf("stored count: %d", count)
	}
}

func TestWarmPruneAndDeleteAll(t *testing.T) {
	svc, repos := memoryService(t, fakeEmbedder{}, baseMemoryConfig())
	ctx := context.Background()
	m, _ := svc.Remember(ctx, &domain.Memory{UserID: "u1", Content: "warm me"})
	// A fresh service/index must recover the vector from SQL.
	svc2 := New(repos.Memories, repos.Sessions, repos.Messages, repos.Users, vector.NewLocal(8),
		cache.NewMemory(100, time.Minute), fakeEmbedder{}, baseMemoryConfig(), 8, slog.Default())
	if err := svc2.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	recalled, err := svc2.Recall(ctx, "u1", "warm", 3)
	if err != nil || len(recalled) == 0 {
		t.Fatalf("warm recall: %v %+v", err, recalled)
	}
	_ = m

	// Expired memory + retention.
	past := time.Now().Add(-time.Hour)
	if err := repos.Memories.Create(ctx, &domain.Memory{ID: "expired", UserID: "u1", Content: "old",
		ExpiresAt: &past, CreatedAt: past, UpdatedAt: past}); err != nil {
		t.Fatal(err)
	}
	cfg := baseMemoryConfig()
	cfg.RetentionDays = 1
	svc3 := New(repos.Memories, repos.Sessions, repos.Messages, repos.Users, vector.NewLocal(8),
		cache.NewMemory(100, time.Minute), fakeEmbedder{}, cfg, 8, slog.Default())
	expired, old, err := svc3.Prune(ctx)
	if err != nil || expired+old < 1 {
		t.Fatalf("prune: %v expired=%d old=%d", err, expired, old)
	}
	n, err := svc.DeleteAll(ctx, "u1")
	if err != nil || n < 1 {
		t.Fatalf("delete all: %v %d", err, n)
	}
}
