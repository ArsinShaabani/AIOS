package documents

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/adapters/vector"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type fakeEmbedder struct{ err error }

func (f fakeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(inputs))
	for i, text := range inputs {
		vec := make([]float32, 8)
		for _, word := range strings.Fields(strings.ToLower(text)) {
			var h uint32
			for _, r := range word {
				h = h*31 + uint32(r)
			}
			vec[h%8]++
		}
		var norm float32
		for _, v := range vec {
			norm += v * v
		}
		if norm > 0 {
			for j := range vec {
				vec[j] /= norm
			}
		}
		out[i] = vec
	}
	return out, nil
}

type failingVector struct{}

func (failingVector) Upsert(context.Context, []ports.VectorPoint) error {
	return errors.New("vector down")
}
func (failingVector) Delete(context.Context, []string) error { return nil }
func (failingVector) Search(context.Context, string, []float32, int, float64) ([]ports.VectorMatch, error) {
	return nil, nil
}
func (failingVector) Count(context.Context) (int, error) { return 0, nil }
func (failingVector) Ping(context.Context) error         { return nil }

func docService(t *testing.T, embedder Embedder, vec ports.VectorStore) (*Service, *sqlstore.Repositories) {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "documents.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.RAGConfig{Enabled: true, MaxDocumentBytes: 1 << 20, ChunkSize: 300,
		ChunkOverlap: 50, MaxChunks: 50, TopK: 5, EmbedBatch: 2, MinScore: 0.1}
	if vec == nil {
		vec = vector.NewLocal(8)
	}
	return New(repos.Documents, vec, embedder, cfg, 8, slog.Default()), repos
}

func TestIngestSearchDelete(t *testing.T) {
	svc, repos := docService(t, fakeEmbedder{}, nil)
	ctx := context.Background()
	content := "# Refund Policy\n\nRefunds are processed within five business days.\n\n" +
		"Shipping to Europe takes ten days. Contact support@example.com for help."
	doc, err := svc.Ingest(ctx, "u1", "", "policy.md", "text/markdown", []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != "indexed" || doc.ChunkCount == 0 || doc.Title == "" {
		t.Fatalf("doc: %+v", doc)
	}
	if n, _ := svc.Count(ctx); n != 1 {
		t.Fatalf("count: %d", n)
	}
	matches, err := svc.Search(ctx, "u1", "refund policy five days", 3)
	if err != nil || len(matches) == 0 {
		t.Fatalf("search: %v %+v", err, matches)
	}
	if matches[0].DocumentID != doc.ID || matches[0].Title != doc.Title || matches[0].Content == "" {
		t.Fatalf("match: %+v", matches[0])
	}
	if list, total, err := svc.List(ctx, "u1", platform.Page{Limit: 5}); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %v %d", err, total)
	}
	if _, err := svc.Get(ctx, "u2", doc.ID); !platform.IsForbidden(err) {
		t.Fatalf("ownership: %v", err)
	}
	chunks, err := svc.Chunks(ctx, "u1", doc.ID, 10)
	if err != nil || len(chunks) != doc.ChunkCount {
		t.Fatalf("chunks: %v %d", err, len(chunks))
	}
	if err := svc.Delete(ctx, "u2", doc.ID); !platform.IsForbidden(err) {
		t.Fatalf("delete ownership: %v", err)
	}
	if err := svc.Delete(ctx, "u1", doc.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := repos.Documents.CountDocuments(ctx); n != 0 {
		t.Fatalf("document not deleted: %d", n)
	}
	if matches, _ := svc.Search(ctx, "u1", "refund", 2); len(matches) != 0 {
		t.Fatalf("vectors not deleted: %+v", matches)
	}
}

func TestIngestValidation(t *testing.T) {
	svc, _ := docService(t, fakeEmbedder{}, nil)
	ctx := context.Background()
	if _, err := svc.Ingest(ctx, "", "t", "", "text/plain", []byte("x")); !platform.IsUnauthorized(err) {
		t.Fatalf("missing user: %v", err)
	}
	if _, err := svc.Ingest(ctx, "u1", "t", "", "text/plain", nil); !platform.IsInvalid(err) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := svc.Ingest(ctx, "u1", "t", "", "text/html", []byte("<html><script>x</script></html>")); !platform.IsInvalid(err) {
		t.Fatalf("no text: %v", err)
	}
	disabled := New(nil, nil, nil, config.RAGConfig{Enabled: false}, 8, slog.Default())
	if _, err := disabled.Ingest(ctx, "u1", "t", "", "text/plain", []byte("x")); !platform.IsUnavailable(err) {
		t.Fatalf("disabled: %v", err)
	}
	if _, err := disabled.Search(ctx, "u1", "x", 1); !platform.IsUnavailable(err) {
		t.Fatalf("disabled search: %v", err)
	}
	if _, err := svc.Search(ctx, "u1", " ", 3); !platform.IsInvalid(err) {
		t.Fatalf("empty query: %v", err)
	}
}

func TestIngestEmbeddingAndVectorFailures(t *testing.T) {
	svc, repos := docService(t, fakeEmbedder{err: errors.New("embed down")}, nil)
	if _, err := svc.Ingest(context.Background(), "u1", "t", "", "text/plain", []byte("hello")); err == nil {
		t.Fatal("embedding error not surfaced")
	}
	if n, _ := repos.Documents.CountDocuments(context.Background()); n != 0 {
		t.Fatalf("document persisted after embedding failure: %d", n)
	}
	svc2, repos2 := docService(t, fakeEmbedder{}, failingVector{})
	doc, err := svc2.Ingest(context.Background(), "u1", "t", "", "text/plain", []byte(strings.Repeat("text ", 100)))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != "error" || doc.Error == "" {
		t.Fatalf("vector failure status: %+v", doc)
	}
	if n, _ := repos2.Documents.CountDocuments(context.Background()); n != 1 {
		t.Fatalf("document should remain for cleanup: %d", n)
	}
}
