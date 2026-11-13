// Package platform contains small, dependency-light building blocks shared by
// every layer of AIOS: identifiers, typed errors, logging, retries and
// circuit breaking. It intentionally has no knowledge of the domain.
package platform

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/google/uuid"
)

// NewID returns a random RFC-4122 identifier with an optional readable prefix.
// Prefixes make logs and the admin dashboard far easier to scan.
func NewID(prefix string) string {
	id := uuid.NewString()
	if prefix == "" {
		return id
	}
	return prefix + "_" + id
}

// RandomToken returns a cryptographically random, URL-safe token carrying n
// bytes of entropy (n should be >= 16 for secrets).
func RandomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// MustRandomToken is RandomToken for initialisation paths where a failure to
// read the OS CSPRNG is unrecoverable.
func MustRandomToken(n int) string {
	tok, err := RandomToken(n)
	if err != nil {
		panic("platform: crypto/rand unavailable: " + err.Error())
	}
	return tok
}

// Truncate shortens s to at most n runes, appending an ellipsis when cut.
// It is rune aware so that non-Latin text (Farsi, Arabic, CJK) is not split.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// Slug converts a human label into a stable, lowercase, dash-separated key.
func Slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
