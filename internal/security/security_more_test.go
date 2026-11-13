package security

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func test1Repos(t *testing.T) *sqlstore.Repositories {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "security.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlstore.NewRepositories(db)
}

func TestKeysAndSecrets(t *testing.T) {
	repos := test1Repos(t)
	keyring, err := LoadOrCreate(config.SecurityConfig{AutoGenerateKeys: true}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(keyring.SigningKey()) == 0 {
		t.Fatal("signing key missing")
	}
	secrets := NewSecrets(repos.Settings, keyring)
	ctx := context.Background()
	if err := secrets.Set(ctx, "provider.key", "super-secret"); err != nil {
		t.Fatal(err)
	}
	got, err := secrets.Get(ctx, "provider.key")
	if err != nil || got != "super-secret" {
		t.Fatalf("secret get: %v %q", err, got)
	}
	names, err := secrets.List(ctx)
	if err != nil || len(names) != 1 || names[0] != "provider.key" {
		t.Fatalf("secret list: %v %v", err, names)
	}
	if err := secrets.Delete(ctx, "provider.key"); err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.Get(ctx, "provider.key"); err == nil {
		t.Fatal("deleted secret readable")
	}
	if _, err := keyring.Decrypt("bm90LWEtdmFsaWQtY2lwaGVydGV4dA"); err == nil {
		t.Fatal("corrupt ciphertext decrypted")
	}
}

func TestKeyFilePersistence(t *testing.T) {
	dir := t.TempDir()
	cfg := config.SecurityConfig{AutoGenerateKeys: true}
	k1, err := LoadOrCreate(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := LoadOrCreate(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(k1.SigningKey()) != string(k2.SigningKey()) {
		t.Fatal("signing key not persisted")
	}
	cfg2 := config.SecurityConfig{AutoGenerateKeys: false}
	if _, err := LoadOrCreate(cfg2, t.TempDir()); err == nil {
		t.Fatal("missing keys accepted when auto-generation is off")
	}
}

func TestAuditorFlushes(t *testing.T) {
	repos := test1Repos(t)
	auditor := NewAuditor(repos.Audit, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	auditor.Start(ctx)
	auditor.Log(ctx, "u1", "user", "test.action", "resource", "r1", "success", "127.0.0.1", "ua", map[string]any{"x": 1})
	auditor.Record(ctx, domain.AuditEvent{Action: "test.second"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := repos.Audit.Count(ctx); n >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	auditor.Stop(ctx)
	cancel()
	if n, _ := repos.Audit.Count(context.Background()); n < 2 {
		t.Fatalf("audit events not flushed: %d", n)
	}
}

func TestTokenServiceRefreshRotationWithSQL(t *testing.T) {
	repos := test1Repos(t)
	keyring, _ := LoadOrCreate(config.SecurityConfig{AutoGenerateKeys: true}, t.TempDir())
	cfg := config.SecurityConfig{Issuer: "aios-test", AccessTTL: config.Duration(15 * time.Minute),
		RefreshTTL: config.Duration(24 * time.Hour), AllowedClockSkew: config.Duration(0)}
	svc := NewTokenService(cfg, keyring, repos.Tokens, repos.Users)
	user := &domain.User{ID: "u1", Email: "a@b.c", PasswordHash: "x", Role: domain.RoleAdmin, Status: "active"}
	if err := repos.Users.Create(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pair, err := svc.IssuePair(ctx, user, "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.User == nil {
		t.Fatalf("pair: %+v", pair)
	}
	refreshed, err := svc.Refresh(ctx, pair.RefreshToken, "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken == pair.RefreshToken {
		t.Fatal("refresh token not rotated")
	}
	if _, err := svc.Refresh(ctx, pair.RefreshToken, "ua", "ip"); err == nil {
		t.Fatal("reused refresh accepted")
	}
	if _, err := svc.Refresh(ctx, refreshed.RefreshToken, "ua", "ip"); err == nil {
		t.Fatal("family should be revoked after reuse")
	}
	// Revoke is idempotent, including unknown tokens.
	if err := svc.Revoke(ctx, "rt_unknown"); err != nil {
		t.Fatalf("revoke unknown: %v", err)
	}
	if err := svc.RevokeAll(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, "", "", ""); err == nil {
		t.Fatal("empty refresh accepted")
	}
}

func TestTokenServiceExpiryAndDisabled(t *testing.T) {
	repos := test1Repos(t)
	keyring, _ := LoadOrCreate(config.SecurityConfig{AutoGenerateKeys: true}, t.TempDir())
	cfg := config.SecurityConfig{Issuer: "aios-test", AccessTTL: config.Duration(time.Minute),
		RefreshTTL: config.Duration(time.Hour)}
	svc := NewTokenService(cfg, keyring, repos.Tokens, repos.Users)
	user := &domain.User{ID: "u1", Email: "a@b.c", PasswordHash: "x", Role: domain.RoleUser, Status: "active"}
	_ = repos.Users.Create(context.Background(), user)
	pair, err := svc.IssuePair(context.Background(), user, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Move time beyond refresh expiry.
	svc.SetClock(func() time.Time { return time.Now().Add(2 * time.Hour) })
	if _, err := svc.Refresh(context.Background(), pair.RefreshToken, "", ""); err == nil {
		t.Fatal("expired refresh accepted")
	}
	svc.SetClock(time.Now)
	user.Status = "disabled"
	_ = repos.Users.Update(context.Background(), user)
	pair2, _ := svc.IssuePair(context.Background(), user, "", "")
	if _, err := svc.Refresh(context.Background(), pair2.RefreshToken, "", ""); err == nil {
		t.Fatal("disabled user refreshed")
	}
}
