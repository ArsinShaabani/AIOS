package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestTokenEdgeCases(t *testing.T) {
	keyring := testKeyring(t)
	cfg := config.SecurityConfig{Issuer: "aios-test", AccessTTL: config.Duration(time.Minute),
		RefreshTTL: config.Duration(time.Hour), AllowedClockSkew: config.Duration(0)}
	svc := NewTokenService(cfg, keyring, nil, nil)
	user := &domain.User{ID: "u1", Email: "a@b.c", Role: domain.RoleUser, Status: "active"}

	token, _, err := svc.IssueAccess(user)
	if err != nil {
		t.Fatal(err)
	}
	// Expired access token.
	svc.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	if _, err := svc.ParseAccess(token); err == nil {
		t.Fatal("expired token accepted")
	}
	svc.SetClock(time.Now)

	// A refresh-typed token must not be accepted as an access token.
	claims := Claims{Role: "user", Type: "refresh", RegisteredClaims: jwt.RegisteredClaims{
		Subject: "u1", Issuer: cfg.Issuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	wrongType, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(keyring.SigningKey())
	if _, err := svc.ParseAccess(wrongType); err == nil {
		t.Fatal("refresh token accepted as access")
	}
	// Wrong signing key.
	other, _ := LoadOrCreate(config.SecurityConfig{AutoGenerateKeys: true}, t.TempDir())
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{Role: "admin", Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "u1", Issuer: cfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}).SignedString(other.SigningKey())
	if _, err := svc.ParseAccess(forged); err == nil {
		t.Fatal("foreign signature accepted")
	}
	if _, err := svc.ParseAccess("not.a.jwt"); err == nil {
		t.Fatal("garbage token accepted")
	}
}

func TestSecretHelpersAndLimiter(t *testing.T) {
	if !ConstantTimeEqual("abc", "abc") || ConstantTimeEqual("abc", "abd") {
		t.Fatal("constant time compare broken")
	}
	token, err := NewSecret("pfx")
	if err != nil || len(token) < 8 || token[:4] != "pfx_" {
		t.Fatalf("secret: %q %v", token, err)
	}
	if TokenPrefix("short") != "short" || len(TokenPrefix("0123456789abcdefghij")) != 16 {
		t.Fatal("token prefix rules wrong")
	}
	if len(HashToken("x")) != 64 {
		t.Fatal("hash length wrong")
	}
	l := NewRateLimiter(1, 1)
	if !l.Allow("k") || l.Allow("k") {
		t.Fatal("limiter burst wrong")
	}
	if l.RetryAfter("k") <= 0 {
		t.Fatal("retry after should be positive")
	}
	if l.Len() != 1 {
		t.Fatalf("limiter len: %d", l.Len())
	}
	if !(*RateLimiter)(nil).Allow("x") {
		t.Fatal("nil limiter must allow")
	}
	if (*RateLimiter)(nil).RetryAfter("x") != 0 {
		t.Fatal("nil limiter retry-after")
	}
}
