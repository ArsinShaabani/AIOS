// Package security implements AIOS' security kernel: key management, password
// hashing, JWT/refresh rotation, RBAC, rate limiting, encrypted secrets and the
// audit trail. It has no transport dependencies so REST, WebSocket, MQTT and
// voice share exactly the same rules.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Keyring holds the token signing key and the at-rest encryption key.
type Keyring struct {
	signingKey    []byte
	encryptionKey []byte
	aead          cipher.AEAD
}

// LoadOrCreate builds the keyring from configuration or from 0600 files in the
// data directory. Missing keys are generated once and persisted; this keeps the
// zero-config experience while never shipping default secrets.
func LoadOrCreate(cfg config.SecurityConfig, dataDir string) (*Keyring, error) {
	signing := []byte(cfg.JWTSecret)
	if len(signing) == 0 {
		if !cfg.AutoGenerateKeys {
			return nil, platform.Invalid("security.jwt_missing", "security.jwt_secret is required when auto_generate_keys is false")
		}
		key, err := loadOrCreateFile(filepath.Join(dataDir, "keys", "jwt.key"), 48)
		if err != nil {
			return nil, err
		}
		signing = key
	}
	var encKey []byte
	if cfg.EncryptionKey != "" {
		key, err := config.DecodeKey(cfg.EncryptionKey)
		if err != nil {
			return nil, platform.Wrap(platform.KindInvalid, "security.enc_key", "invalid encryption key", err)
		}
		encKey = key
	} else {
		if !cfg.AutoGenerateKeys {
			return nil, platform.Invalid("security.enc_missing", "security.encryption_key is required when auto_generate_keys is false")
		}
		key, err := loadOrCreateFile(filepath.Join(dataDir, "keys", "encryption.key"), 32)
		if err != nil {
			return nil, err
		}
		encKey = key
	}
	if len(encKey) != 32 {
		return nil, platform.Invalid("security.enc_length", "encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "security.aes", "cannot initialise AES", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "security.gcm", "cannot initialise GCM", err)
	}
	return &Keyring{signingKey: signing, encryptionKey: encKey, aead: aead}, nil
}

func loadOrCreateFile(path string, size int) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		decoded, decErr := hex.DecodeString(string(trimSpace(raw)))
		if decErr == nil && len(decoded) == size {
			return decoded, nil
		}
		if len(raw) == size {
			return raw, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	encoded := []byte(hex.EncodeToString(buf))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, fmt.Errorf("persist key: %w", err)
	}
	return buf, nil
}

func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\n' || b[start] == '\r' || b[start] == '\t') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

// SigningKey returns the HMAC key used for JWTs.
func (k *Keyring) SigningKey() []byte { return k.signingKey }

// Encrypt seals plaintext with AES-256-GCM and returns base64(nonce||ciphertext).
func (k *Keyring) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := k.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a value produced by Encrypt.
func (k *Keyring) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", platform.Wrap(platform.KindInvalid, "security.decrypt", "invalid ciphertext", err)
	}
	ns := k.aead.NonceSize()
	if len(raw) < ns {
		return "", platform.Invalid("security.decrypt", "ciphertext too short")
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plain, err := k.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", platform.Wrap(platform.KindInternal, "security.decrypt", "decryption failed", err)
	}
	return string(plain), nil
}

// HashToken returns a stable hex SHA-256 hash for bearer-like secrets. Tokens
// are high-entropy, so a fast hash is appropriate (bcrypt is reserved for
// low-entropy passwords).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two strings without leaking length/timing.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// NewSecret returns a prefixed, URL-safe random secret with 32 bytes of entropy.
func NewSecret(prefix string) (string, error) {
	tok, err := platform.RandomToken(32)
	if err != nil {
		return "", err
	}
	if prefix == "" {
		return tok, nil
	}
	return prefix + "_" + tok, nil
}

// TokenPrefix returns the lookup prefix stored alongside a hashed secret.
func TokenPrefix(token string) string {
	const n = 16
	if len(token) <= n {
		return token
	}
	return token[:n]
}
