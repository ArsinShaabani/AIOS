package sqlstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// TestPostgresIntegration runs the same repository contracts against a real
// PostgreSQL instance. It is skipped unless AIOS_TEST_POSTGRES_DSN is set; CI
// provides a postgres service container and exports the variable.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("AIOS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AIOS_TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{
		Driver: "postgres", DSN: dsn, AutoMigrate: true,
		MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: config.Duration(time.Minute),
	}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	if db.Dialect != DialectPostgres {
		t.Fatalf("dialect: %s", db.Dialect)
	}
	if got := db.Rebind("SELECT ? , ?"); got != "SELECT $1 , $2" {
		t.Fatalf("rebind: %q", got)
	}
	r := NewRepositories(db)
	cleanup := []string{"messages", "sessions", "memories", "devices", "plugins", "audit_events",
		"usage_records", "tasks", "refresh_tokens", "api_keys", "settings", "agents", "personalities", "users"}
	for _, table := range cleanup {
		if _, err := db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("clean %s: %v", table, err)
		}
	}

	user := &domain.User{ID: "pg-user", Email: "pg@test.local", PasswordHash: "x", Role: domain.RoleAdmin}
	if err := r.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Users.GetByEmail(ctx, "pg@test.local"); err != nil || got.ID != user.ID {
		t.Fatalf("get user: %v", err)
	}
	if err := r.Users.Create(ctx, &domain.User{ID: "dup", Email: "pg@test.local", PasswordHash: "x", Role: domain.RoleUser}); !platform.IsConflict(err) {
		t.Fatalf("expected unique violation, got %v", err)
	}

	session := &domain.Session{ID: "pg-session", UserID: user.ID, Title: "Postgres", Channel: domain.ChannelAPI}
	if err := r.Sessions.Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := r.Messages.Create(ctx, &domain.Message{ID: "pg-msg-" + string(rune('a'+i)),
			SessionID: session.ID, UserID: user.ID, Role: domain.MsgUser, Content: "hello"}); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := r.Messages.ListBySession(ctx, session.ID, 10)
	if err != nil || len(msgs) != 3 {
		t.Fatalf("messages: %v %d", err, len(msgs))
	}

	mem := &domain.Memory{ID: "pg-mem", UserID: user.ID, Content: "likes postgres", Kind: domain.KindPreference,
		Tags: []string{"db"}, Embedding: []float32{0.1, 0.2}}
	if err := r.Memories.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}
	if list, total, err := r.Memories.ListByUser(ctx, user.ID, ports.MemoryFilter{Query: "postgres"}, platform.Page{Limit: 5}); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("memories: %v %d", err, total)
	}

	device := &domain.Device{ID: "pg-dev", UserID: user.ID, Name: "Sensor", TokenPrefix: "pgprefix", TokenHash: "h"}
	if err := r.Devices.Create(ctx, device); err != nil {
		t.Fatal(err)
	}
	if err := r.Devices.SetStatus(ctx, device.ID, "online", time.Now()); err != nil {
		t.Fatal(err)
	}
	if statuses, err := r.Devices.CountByStatus(ctx); err != nil || statuses["online"] != 1 {
		t.Fatalf("device status: %v %+v", err, statuses)
	}

	if err := r.Usage.Create(ctx, &domain.UsageRecord{ID: "pg-usage", UserID: user.ID, Provider: "local",
		Model: "m", Kind: "chat", PromptTokens: 5, CompletionTokens: 3, CostUSD: 0.001}); err != nil {
		t.Fatal(err)
	}
	totals, err := r.Usage.Totals(ctx, user.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || totals.Requests != 1 || totals.PromptTokens != 5 {
		t.Fatalf("usage: %v %+v", err, totals)
	}

	if err := r.Audit.Create(ctx, &domain.AuditEvent{ID: "pg-audit", Action: "test", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Audit.Count(ctx); err != nil || n != 1 {
		t.Fatalf("audit: %v %d", err, n)
	}

	task := &domain.Task{ID: "pg-task", Kind: "reminder", Payload: []byte(`{}`), RunAt: time.Now().Add(-time.Second)}
	if err := r.Tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	claimed, err := r.Tasks.ClaimDue(ctx, time.Now(), 5)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %d", err, len(claimed))
	}
	if n, err := r.Tasks.ReclaimStale(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("reclaim: %v %d", err, n)
	}

	if err := r.Settings.Set(ctx, &domain.Setting{Scope: "global", Key: "pg", Value: []byte(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Settings.Get(ctx, "global", "", "pg"); err != nil || string(got.Value) != `{"ok":true}` {
		t.Fatalf("setting: %v", err)
	}

	refresh := &domain.RefreshToken{ID: "pg-refresh", UserID: user.ID, TokenHash: "pghash",
		FamilyID: "pgfam", ExpiresAt: time.Now().Add(time.Hour)}
	if err := r.Tokens.CreateRefresh(ctx, refresh); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Tokens.GetRefreshByHash(ctx, "pghash"); err != nil {
		t.Fatal(err)
	}

	overview, err := r.Stats.Overview(ctx)
	if err != nil || overview.Users != 1 || overview.Sessions != 1 || overview.Messages != 3 {
		t.Fatalf("overview: %v %+v", err, overview)
	}
}
