package sqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func TestTaskRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	now := time.Now().UTC()
	task := &domain.Task{ID: "t1", Kind: "reminder", Payload: []byte(`{"user_id":"u1"}`),
		RunAt: now.Add(-time.Minute), MaxAttempts: 2, Priority: 5}
	if err := r.Tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Tasks.Get(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	claimed, err := r.Tasks.ClaimDue(ctx, now, 10)
	if err != nil || len(claimed) != 1 || claimed[0].Attempts != 1 {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if err := r.Tasks.Fail(ctx, "t1", "boom", now.Add(time.Minute), false, now); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Tasks.Get(ctx, "t1")
	if got.Status != "pending" || got.LastError != "boom" {
		t.Fatalf("retry state: %+v", got)
	}
	if err := r.Tasks.Fail(ctx, "t1", "dead", now, true, now); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Tasks.Get(ctx, "t1")
	if got.Status != "dead" {
		t.Fatalf("dead state: %+v", got)
	}
	list, total, err := r.Tasks.List(ctx, "dead", platform.Page{Limit: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %v %d", err, total)
	}
	if counts, err := r.Tasks.CountByStatus(ctx); err != nil || counts["dead"] != 1 {
		t.Fatalf("counts: %v %+v", err, counts)
	}
	if n, err := r.Tasks.DeleteBefore(ctx, now.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune: %v %d", err, n)
	}
	if _, err := r.Tasks.Get(ctx, "t1"); !platform.IsNotFound(err) {
		t.Fatalf("deleted task readable: %v", err)
	}
}

func TestTokenRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	now := time.Now().UTC()
	old := &domain.RefreshToken{ID: "r1", UserID: "u1", TokenHash: "h1", FamilyID: "f1", ExpiresAt: now.Add(time.Hour)}
	newer := &domain.RefreshToken{ID: "r2", UserID: "u1", TokenHash: "h2", FamilyID: "f1", ExpiresAt: now.Add(time.Hour)}
	if err := r.Tokens.CreateRefresh(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := r.Tokens.CreateRefresh(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if err := r.Tokens.RotateRefresh(ctx, "r1", "r2", now); err != nil {
		t.Fatal(err)
	}
	rotated, _ := r.Tokens.GetRefreshByHash(ctx, "h1")
	if rotated.ReplacedBy != "r2" {
		t.Fatalf("rotation: %+v", rotated)
	}
	if err := r.Tokens.RevokeAllForUser(ctx, "u1", now); err != nil {
		t.Fatal(err)
	}
	revoked, _ := r.Tokens.GetRefreshByHash(ctx, "h1")
	if revoked.RevokedAt == nil {
		t.Fatal("refresh not revoked")
	}
	if n, err := r.Tokens.DeleteExpiredRefresh(ctx, now.Add(2*time.Hour)); err != nil || n != 2 {
		t.Fatalf("delete expired: %v %d", err, n)
	}

	key := &domain.APIKey{ID: "k1", UserID: "u1", Name: "cli", KeyHash: "kh", Prefix: "aios_abc", Scopes: []string{"chat"}}
	if err := r.Tokens.CreateAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := r.Tokens.TouchAPIKey(ctx, "k1", now); err != nil {
		t.Fatal(err)
	}
	loaded, err := r.Tokens.GetAPIKeyByHash(ctx, "kh")
	if err != nil || loaded.LastUsedAt == nil {
		t.Fatalf("api key: %v %+v", err, loaded)
	}
	if err := r.Tokens.RevokeAPIKey(ctx, "k1", "u1", now); err != nil {
		t.Fatal(err)
	}
	if err := r.Tokens.RevokeAPIKey(ctx, "k1", "other", now); !platform.IsNotFound(err) {
		t.Fatalf("wrong owner revoke: %v", err)
	}
}
