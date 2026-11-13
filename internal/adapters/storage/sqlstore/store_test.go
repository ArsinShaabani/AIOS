package sqlstore

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func newTestStore(t *testing.T) *Repositories {
	t.Helper()
	cfg := config.DatabaseConfig{Driver: "sqlite", DSN: "test.db", AutoMigrate: true, MaxOpenConns: 1, MaxIdleConns: 1}
	db, err := Open(context.Background(), cfg, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepositories(db)
}

func TestUserCrud(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	u := &domain.User{ID: "u1", Email: "A@Example.com", PasswordHash: "x", Role: domain.RoleAdmin, Status: "active"}
	if err := r.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, err := r.Users.GetByEmail(ctx, "a@example.com")
	if err != nil || got.ID != "u1" {
		t.Fatalf("get by email: %v", err)
	}
	if _, err := r.Users.GetByID(ctx, "missing"); !platform.IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
	if err := r.Users.Create(ctx, &domain.User{ID: "u2", Email: "a@example.com", PasswordHash: "x", Role: domain.RoleUser}); !platform.IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}
	u.DisplayName = "Admin"
	if err := r.Users.Update(ctx, u); err != nil {
		t.Fatal(err)
	}
	list, total, err := r.Users.List(ctx, platform.Page{Limit: 10}, portsFilter())
	if err != nil || total != 1 || list[0].DisplayName != "Admin" {
		t.Fatalf("list: %v total=%d", err, total)
	}
	if err := r.Users.Delete(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionsMessagesCascade(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	u := &domain.User{ID: "u1", Email: "a@b.c", PasswordHash: "x", Role: domain.RoleUser}
	if err := r.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	s := &domain.Session{ID: "s1", UserID: "u1", Title: "Test", Channel: domain.ChannelAPI}
	if err := r.Sessions.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		m := &domain.Message{ID: "m" + string(rune('a'+i)), SessionID: "s1", UserID: "u1", Role: domain.MsgUser, Content: "hello"}
		if err := r.Messages.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := r.Messages.ListBySession(ctx, "s1", 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("messages: %v %d", err, len(rows))
	}
	if err := r.Sessions.Touch(ctx, "s1", 3, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Sessions.Get(ctx, "s1")
	if got.MessageCount != 3 {
		t.Fatalf("message count=%d", got.MessageCount)
	}
	if err := r.Sessions.Delete(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.Messages.CountBySession(ctx, "s1"); n != 0 {
		t.Fatalf("cascade failed: %d", n)
	}
}

func TestMemoriesAndTasks(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	m := &domain.Memory{ID: "m1", UserID: "u1", Content: "likes tea", Kind: domain.KindPreference,
		Tags: []string{"tea"}, Embedding: []float32{0.1, 0.2}}
	if err := r.Memories.Create(ctx, m); err != nil {
		t.Fatal(err)
	}
	rows, err := r.Memories.ListWithEmbeddings(ctx, "u1", 10)
	if err != nil || len(rows) != 1 || len(rows[0].Embedding) != 2 {
		t.Fatalf("embeddings: %v %+v", err, rows)
	}
	if _, total, err := r.Memories.ListByUser(ctx, "u1", portsMemoryFilter(), platform.Page{}); err != nil || total != 1 {
		t.Fatalf("list memories: %v %d", err, total)
	}

	task := &domain.Task{ID: "t1", Kind: "reminder", Payload: []byte(`{"user_id":"u1"}`), RunAt: time.Now().Add(-time.Second)}
	if err := r.Tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	claimed, err := r.Tasks.ClaimDue(ctx, time.Now(), 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %d", err, len(claimed))
	}
	if again, _ := r.Tasks.ClaimDue(ctx, time.Now(), 10); len(again) != 0 {
		t.Fatal("task claimed twice")
	}
	if err := r.Tasks.Complete(ctx, "t1", time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestTokensAndSettings(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	rt := &domain.RefreshToken{ID: "r1", UserID: "u1", TokenHash: "h1", FamilyID: "f1", ExpiresAt: time.Now().Add(time.Hour)}
	if err := r.Tokens.CreateRefresh(ctx, rt); err != nil {
		t.Fatal(err)
	}
	got, err := r.Tokens.GetRefreshByHash(ctx, "h1")
	if err != nil || got.ID != "r1" {
		t.Fatalf("refresh: %v", err)
	}
	if err := r.Tokens.RevokeFamily(ctx, "f1", time.Now()); err != nil {
		t.Fatal(err)
	}
	revoked, _ := r.Tokens.GetRefreshByHash(ctx, "h1")
	if revoked.RevokedAt == nil {
		t.Fatal("family not revoked")
	}
	key := &domain.APIKey{ID: "k1", UserID: "u1", Name: "cli", KeyHash: "kh", Prefix: "aios_abc", Scopes: []string{"chat"}}
	if err := r.Tokens.CreateAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if keys, _ := r.Tokens.ListAPIKeys(ctx, "u1"); len(keys) != 1 {
		t.Fatal("api keys list")
	}
	if err := r.Settings.Set(ctx, &domain.Setting{Scope: "global", Key: "smtp", Value: []byte(`{"host":"x"}`)}); err != nil {
		t.Fatal(err)
	}
	s, err := r.Settings.Get(ctx, "global", "", "smtp")
	if err != nil || string(s.Value) != `{"host":"x"}` {
		t.Fatalf("setting: %v %s", err, s.Value)
	}
}
