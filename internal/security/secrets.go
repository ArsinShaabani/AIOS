package security

import (
	"context"
	"encoding/json"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

const (
	secretScope = "secret"
	secretID    = "global"
)

// Secrets stores provider keys and integration tokens AES-GCM encrypted inside
// the settings table, so a DB backup alone does not leak credentials.
type Secrets struct {
	settings ports.SettingRepository
	keyring  *Keyring
}

var _ ports.SecretStore = (*Secrets)(nil)

// NewSecrets builds an encrypted secret store.
func NewSecrets(settings ports.SettingRepository, keyring *Keyring) *Secrets {
	return &Secrets{settings: settings, keyring: keyring}
}

// Get decrypts a secret.
func (s *Secrets) Get(ctx context.Context, name string) (string, error) {
	row, err := s.settings.Get(ctx, secretScope, secretID, name)
	if err != nil {
		return "", err
	}
	var enc string
	if err := json.Unmarshal(row.Value, &enc); err != nil {
		return "", platform.Wrap(platform.KindInternal, "secret.decode", "secret payload is corrupt", err)
	}
	plain, err := s.keyring.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return plain, nil
}

// Set encrypts and upserts a secret.
func (s *Secrets) Set(ctx context.Context, name, value string) error {
	enc, err := s.keyring.Encrypt(value)
	if err != nil {
		return platform.Wrap(platform.KindInternal, "secret.encrypt", "cannot encrypt secret", err)
	}
	raw, err := json.Marshal(enc)
	if err != nil {
		return err
	}
	return s.settings.Set(ctx, &domain.Setting{
		Scope: secretScope, ScopeID: secretID, Key: name, Value: raw,
	})
}

// Delete removes a secret.
func (s *Secrets) Delete(ctx context.Context, name string) error {
	return s.settings.Delete(ctx, secretScope, secretID, name)
}

// List returns secret names (never values).
func (s *Secrets) List(ctx context.Context) ([]string, error) {
	rows, err := s.settings.List(ctx, secretScope, secretID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Key)
	}
	return out, nil
}
