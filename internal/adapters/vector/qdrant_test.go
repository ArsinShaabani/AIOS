package vector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestQdrantAdapter(t *testing.T) {
	var created, upserted, deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections":
			_, _ = w.Write([]byte(`{"result":{"collections":[]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/collections/mem":
			if !created {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"result":{"points_count":3}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/collections/mem":
			created = true
			_, _ = w.Write([]byte(`{"result":true}`))
		case r.Method == http.MethodPut && r.URL.Path == "/collections/mem/points":
			var body struct {
				Points []map[string]any `json:"points"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Points) != 2 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			upserted = true
			_, _ = w.Write([]byte(`{"result":{"operation_id":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/collections/mem/points/search":
			_, _ = w.Write([]byte(`{"result":[{"id":"uuid-1","score":0.91,"payload":{"ref":"mem_1","user_id":"u1"}},{"id":"uuid-2","score":0.12,"payload":{"ref":"mem_2"}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/collections/mem/points/delete":
			deleted = true
			_, _ = w.Write([]byte(`{"result":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	store := NewQdrant(config.VectorConfig{Driver: "qdrant", QdrantURL: srv.URL, Collection: "mem",
		Dim: 3, Timeout: config.Duration(2e9)})
	ctx := t.Context()
	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, []ports.VectorPoint{
		{ID: "mem_1", UserID: "u1", Vector: []float32{1, 0, 0}, Payload: map[string]any{"kind": "fact"}},
		{ID: "mem_2", UserID: "u1", Vector: []float32{0, 1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if !upserted {
		t.Fatal("upsert did not reach qdrant")
	}
	matches, err := store.Search(ctx, "u1", []float32{1, 0, 0}, 2, 0.5)
	if err != nil || len(matches) != 2 || matches[0].ID != "mem_1" || matches[0].Score != 0.91 {
		t.Fatalf("search: %v %+v", err, matches)
	}
	if err := store.Delete(ctx, []string{"mem_1"}); err != nil || !deleted {
		t.Fatalf("delete: %v", err)
	}
	count, err := store.Count(ctx)
	if err != nil || count != 3 {
		t.Fatalf("count: %v %d", err, count)
	}
	if err := store.Upsert(ctx, nil); err != nil {
		t.Fatal("empty upsert should be a no-op")
	}
	if err := store.Delete(ctx, nil); err != nil {
		t.Fatal("empty delete should be a no-op")
	}
}

func TestQdrantErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	store := NewQdrant(config.VectorConfig{QdrantURL: srv.URL, Collection: "mem", Dim: 2, Timeout: config.Duration(time.Second)})
	if err := store.Upsert(t.Context(), []ports.VectorPoint{{ID: "x", Vector: []float32{1, 2}}}); err == nil {
		t.Fatal("upsert error not surfaced")
	}
	if _, err := store.Search(t.Context(), "", []float32{1, 2}, 1, 0); err == nil {
		t.Fatal("search error not surfaced")
	}
	if _, err := store.Count(t.Context()); err == nil {
		t.Fatal("count error not surfaced")
	}
	if err := store.Ping(t.Context()); err == nil {
		t.Fatal("ping error not surfaced")
	}
}
