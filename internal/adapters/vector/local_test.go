package vector

import (
	"context"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestLocalUpsertSearchDelete(t *testing.T) {
	idx := NewLocal(16)
	ctx := context.Background()
	vecA := make([]float32, 16)
	vecB := make([]float32, 16)
	vecA[0] = 1
	vecB[0] = 0.6
	vecB[1] = 0.8
	err := idx.Upsert(ctx, []ports.VectorPoint{
		{ID: "a", UserID: "u1", Vector: vecA, Payload: map[string]any{"kind": "fact"}},
		{ID: "b", UserID: "u1", Vector: vecB},
		{ID: "c", UserID: "u2", Vector: vecA},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := idx.Count(ctx); n != 3 {
		t.Fatalf("count=%d", n)
	}
	matches, err := idx.Search(ctx, "u1", vecA, 5, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].ID != "a" || matches[0].Score < 0.99 {
		t.Fatalf("unexpected matches: %+v", matches)
	}
	if len(matches) > 1 && matches[1].ID == "c" {
		t.Fatal("user filtering failed")
	}
	if _, err := idx.Search(ctx, "u1", make([]float32, 8), 5, 0); err == nil {
		t.Fatal("dimension mismatch must fail")
	}
	if err := idx.Upsert(ctx, []ports.VectorPoint{{ID: "x", Vector: make([]float32, 4)}}); err == nil {
		t.Fatal("bad dimension accepted")
	}
	if err := idx.Delete(ctx, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := idx.Count(ctx); n != 2 {
		t.Fatalf("delete failed: %d", n)
	}
	if err := idx.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
