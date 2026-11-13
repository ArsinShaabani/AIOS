package security

import (
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// DeviceCredential is a one-time-view device token plus its stored forms.
type DeviceCredential struct {
	Token  string `json:"token"`
	Prefix string `json:"prefix"`
	Hash   string `json:"-"`
}

// NewDeviceCredential mints a device token. Only the bcrypt hash and a lookup
// prefix are persisted, so a database leak does not expose device credentials.
func NewDeviceCredential(bcryptCost int) (*DeviceCredential, error) {
	token, err := NewSecret("dev")
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "device.token", "cannot generate device token", err)
	}
	if bcryptCost < 8 || bcryptCost > 14 {
		bcryptCost = 10
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcryptCost)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "device.hash", "cannot hash device token", err)
	}
	return &DeviceCredential{Token: token, Prefix: TokenPrefix(token), Hash: string(hash)}, nil
}

// VerifyDeviceCredential compares a presented token with the stored bcrypt hash.
func VerifyDeviceCredential(hash, token string) bool {
	if hash == "" || token == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil
}

// APICredential is a programmatic key shown once to its owner.
type APICredential struct {
	Token  string `json:"token"`
	Prefix string `json:"prefix"`
	Hash   string `json:"-"`
}

// NewAPICredential mints a high-entropy API key; only its SHA-256 hash is stored.
func NewAPICredential() (*APICredential, error) {
	token, err := NewSecret("aios")
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "apikey.token", "cannot generate API key", err)
	}
	return &APICredential{Token: token, Prefix: TokenPrefix(token), Hash: HashToken(token)}, nil
}

// VerifyAPICredential compares hashes in constant time.
func VerifyAPICredential(hash, token string) bool {
	if hash == "" || token == "" {
		return false
	}
	return ConstantTimeEqual(hash, HashToken(token))
}

// BearerToken extracts a token from an Authorization header, accepting either a
// "Bearer <token>" or a raw token value.
func BearerToken(header string) string {
	h := strings.TrimSpace(header)
	if h == "" {
		return ""
	}
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return h
}
