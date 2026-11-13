package security

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// dummyHash is compared against when an account does not exist so login timing
// does not reveal which emails are registered.
var dummyHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0T6c9m7oQaTQZmFh4S8F0pLr0Yq"

// HashPassword produces a bcrypt hash with the configured cost.
func HashPassword(password string, cost int) (string, error) {
	if len(password) < 8 {
		return "", platform.Invalid("password.weak", "password must be at least 8 characters")
	}
	if len(password) > 256 {
		return "", platform.Invalid("password.long", "password must be at most 256 characters")
	}
	if cost < 8 || cost > 14 {
		cost = 10
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", platform.Wrap(platform.KindInternal, "password.hash", "cannot hash password", err)
	}
	return string(hash), nil
}

// CheckPassword compares a password against a bcrypt hash. When hash is empty it
// still performs a comparison to keep timing uniform.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CheckPasswordHash is an alias kept for readability in tests.
func CheckPasswordHash(hash, password string) bool { return CheckPassword(hash, password) }

// ErrInvalidCredentials is returned to clients for every login failure mode.
var ErrInvalidCredentials = errors.New("invalid credentials")
