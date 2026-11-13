package devices

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// List returns the user's devices.
func (s *Service) List(ctx context.Context, userID string, filter ports.DeviceFilter, page platform.Page) ([]domain.Device, int, error) {
	return s.repo.ListByUser(ctx, userID, filter, page)
}

// ListAll returns devices across users (admin).
func (s *Service) ListAll(ctx context.Context, filter ports.DeviceFilter, page platform.Page) ([]domain.Device, int, error) {
	return s.repo.ListAll(ctx, filter, page)
}

// Update applies a partial patch to a device. Omitted fields keep their current
// values, which is what PATCH semantics promise and prevents an accidental
// zeroing of capabilities or protocol.
func (s *Service) Update(ctx context.Context, userID string, d *domain.Device) error {
	existing, err := s.Get(ctx, userID, d.ID)
	if err != nil {
		return err
	}
	if d.Name != "" {
		existing.Name = d.Name
	}
	if d.Kind != "" {
		existing.Kind = d.Kind
	}
	if d.Protocol != "" {
		existing.Protocol = d.Protocol
	}
	if d.Hardware != "" {
		existing.Hardware = d.Hardware
	}
	if d.Room != "" {
		existing.Room = d.Room
	}
	if d.Firmware != "" {
		existing.Firmware = d.Firmware
	}
	if d.Metadata != nil {
		existing.Metadata = d.Metadata
	}
	if len(d.Capabilities) > 0 {
		existing.Capabilities = d.Capabilities
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return err
	}
	s.record(ctx, userID, "device.update", existing.ID, nil)
	return nil
}

// Delete removes a device.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.record(ctx, userID, "device.delete", id, nil)
	return nil
}

// RotateToken mints a new device token (the old one stops working).
func (s *Service) RotateToken(ctx context.Context, userID, id string) (string, error) {
	d, err := s.Get(ctx, userID, id)
	if err != nil {
		return "", err
	}
	cred, err := security.NewDeviceCredential(s.bcryptCost)
	if err != nil {
		return "", err
	}
	d.TokenHash = cred.Hash
	d.TokenPrefix = cred.Prefix
	if err := s.repo.Update(ctx, d); err != nil {
		return "", err
	}
	s.record(ctx, userID, "device.rotate_token", id, nil)
	return cred.Token, nil
}

// Authenticate resolves a device from a bearer token.
func (s *Service) Authenticate(ctx context.Context, token string) (*domain.Device, error) {
	if token == "" {
		return nil, platform.Unauthorized("device.token_missing", "device token is required")
	}
	prefix := security.TokenPrefix(token)
	d, err := s.repo.GetByTokenPrefix(ctx, prefix)
	if err != nil {
		return nil, platform.Unauthorized("device.token_invalid", "invalid device token")
	}
	if !security.VerifyDeviceCredential(d.TokenHash, token) {
		return nil, platform.Unauthorized("device.token_invalid", "invalid device token")
	}
	return d, nil
}

// Touch marks a device online.
func (s *Service) Touch(ctx context.Context, id string) {
	now := Now().UTC()
	if err := s.repo.SetStatus(ctx, id, "online", now); err != nil {
		s.log.Debug("cannot update device presence", "device_id", id, "error", err)
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, "device:seen:"+id, []byte(now.Format("2006-01-02T15:04:05Z")), 0)
	}
}
