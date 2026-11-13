package security

import (
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func testKeyring(t *testing.T) *Keyring {
	t.Helper()
	cfg := config.SecurityConfig{AutoGenerateKeys: true}
	k, err := LoadOrCreate(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryptRoundTrip(t *testing.T) {
	k := testKeyring(t)
	plain := "sk-secret-provider-key"
	enc, err := k.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, plain) {
		t.Fatal("ciphertext leaks plaintext")
	}
	dec, err := k.Decrypt(enc)
	if err != nil || dec != plain {
		t.Fatalf("decrypt: %q %v", dec, err)
	}
	if _, err := k.Decrypt("not-base64"); err == nil {
		t.Fatal("invalid ciphertext accepted")
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery", 8)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse battery") {
		t.Fatal("valid password rejected")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if CheckPassword("", "anything") {
		t.Fatal("empty hash accepted")
	}
	if _, err := HashPassword("short", 8); err == nil {
		t.Fatal("weak password should be rejected")
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	k := testKeyring(t)
	cfg := config.SecurityConfig{Issuer: "aios-test", AccessTTL: config.Duration(time.Minute), RefreshTTL: config.Duration(time.Hour), AllowedClockSkew: config.Duration(0)}
	svc := NewTokenService(cfg, k, nil, nil)
	user := &domain.User{ID: "usr_1", Email: "a@b.c", Role: domain.RoleAdmin, Status: "active"}
	token, _, err := svc.IssueAccess(user)
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.ParseAccess(token)
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != user.ID || p.Role != domain.RoleAdmin || !p.Can(PermUsersWrite) {
		t.Fatalf("bad principal: %+v", p)
	}
	if _, err := svc.ParseAccess(token + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestRBAC(t *testing.T) {
	if !Can(domain.RoleAdmin, PermPluginsWrite) || !Can(domain.RoleAdmin, PermAnything()) {
		t.Fatal("admin must hold every permission")
	}
	if Can(domain.RoleUser, PermUsersWrite) {
		t.Fatal("regular users must not manage users")
	}
	if !Can(domain.RoleOperator, PermQueueWrite) || Can(domain.RoleViewer, PermDevicesWrite) {
		t.Fatal("role matrix is wrong")
	}
	p := &Principal{Role: domain.RoleUser, Scopes: []string{string(PermChat)}}
	if !p.Can(PermChat) || p.Can(PermMemoryRead) {
		t.Fatal("API key scopes must narrow permissions")
	}
	d := &Principal{Role: domain.RoleUser, Method: "device"}
	if !d.Can(PermDevicesRead) || d.Can(PermUsersRead) {
		t.Fatal("device principals must be limited")
	}
}

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter(60, 2)
	if !l.Allow("ip") || !l.Allow("ip") {
		t.Fatal("burst should pass")
	}
	if l.Allow("ip") {
		t.Fatal("limiter did not block")
	}
	if l.RetryAfter("ip") <= 0 {
		t.Fatal("retry-after should be positive")
	}
	if !NewRateLimiter(0, 0).Allow("x") {
		t.Fatal("nil limiter must allow")
	}
}

func TestCredentials(t *testing.T) {
	cred, err := NewDeviceCredential(8)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cred.Token, "dev_") || cred.Prefix == "" || cred.Hash == cred.Token {
		t.Fatal("device credential is malformed")
	}
	if !VerifyDeviceCredential(cred.Hash, cred.Token) || VerifyDeviceCredential(cred.Hash, "dev_wrong") {
		t.Fatal("device verification is wrong")
	}
	key, err := NewAPICredential()
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyAPICredential(key.Hash, key.Token) || VerifyAPICredential(key.Hash, key.Token+"x") {
		t.Fatal("api key verification is wrong")
	}
	if BearerToken("Bearer abc") != "abc" || BearerToken("abc") != "abc" || BearerToken("") != "" {
		t.Fatal("bearer parsing is wrong")
	}
}
