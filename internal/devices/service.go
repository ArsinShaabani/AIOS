// Package devices implements device registration, credential rotation, command
// validation (including robotics safety bounds), telemetry caching and presence.
package devices

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// Service manages devices and routes commands to protocol transports.
type Service struct {
	repo       ports.DeviceRepository
	transports []ports.DeviceTransport
	cache      ports.Cache
	audit      *security.Auditor
	events     ports.Publisher
	bcryptCost int
	log        *slog.Logger
}

// New builds the device service.
func New(repo ports.DeviceRepository, transports []ports.DeviceTransport, cache ports.Cache,
	audit *security.Auditor, events ports.Publisher, bcryptCost int, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, transports: transports, cache: cache, audit: audit, events: events, bcryptCost: bcryptCost, log: log}
}

// Register creates a device and returns its one-time token.
func (s *Service) Register(ctx context.Context, userID string, d *domain.Device) (*domain.Device, string, error) {
	if userID == "" {
		return nil, "", platform.Unauthorized("device.user", "authentication required")
	}
	if strings.TrimSpace(d.Name) == "" {
		return nil, "", platform.Invalid("device.name", "device name is required")
	}
	if len(d.Capabilities) > 32 {
		return nil, "", platform.Invalid("device.capabilities", "at most 32 capabilities are allowed")
	}
	seen := map[string]bool{}
	for _, c := range d.Capabilities {
		if strings.TrimSpace(c.Name) == "" {
			return nil, "", platform.Invalid("device.capability_name", "capability names must not be empty")
		}
		if seen[c.Name] {
			return nil, "", platform.Invalid("device.capability_duplicate", "duplicate capability: "+c.Name)
		}
		seen[c.Name] = true
	}
	cred, err := security.NewDeviceCredential(s.bcryptCost)
	if err != nil {
		return nil, "", err
	}
	d.ID = platform.NewID("dev")
	d.UserID = userID
	d.Status = "offline"
	d.TokenHash = cred.Hash
	d.TokenPrefix = cred.Prefix
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, "", err
	}
	s.record(ctx, userID, "device.register", d.ID, nil)
	return d, cred.Token, nil
}

// Get loads a device owned by the user.
func (s *Service) Get(ctx context.Context, userID, id string) (*domain.Device, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID != "" && d.UserID != userID {
		return nil, platform.Forbidden("device.forbidden", "device belongs to another user")
	}
	return d, nil
}

// Resolve finds a device by id, exact name (case-insensitive) or a partial
// "room name" match.
func (s *Service) Resolve(ctx context.Context, userID, ref string) (*domain.Device, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, platform.Invalid("device.ref", "device reference is required")
	}
	if d, err := s.repo.Get(ctx, ref); err == nil && (userID == "" || d.UserID == userID) {
		return d, nil
	}
	list, _, err := s.repo.ListByUser(ctx, userID, ports.DeviceFilter{}, platform.Page{Limit: 200})
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(ref)
	for i := range list {
		if strings.ToLower(list[i].Name) == lower {
			return &list[i], nil
		}
	}
	for i := range list {
		full := strings.ToLower(strings.TrimSpace(list[i].Room + " " + list[i].Name))
		if full == lower || strings.Contains(full, lower) {
			return &list[i], nil
		}
	}
	return nil, platform.NotFound("device.not_found", "device not found: "+ref)
}

func (s *Service) record(ctx context.Context, userID, action, deviceID string, detail map[string]any) {
	if s.audit == nil {
		return
	}
	s.audit.Log(ctx, userID, "user", action, "device", deviceID, "success", "", "", detail)
}

// Now is exposed for tests.
var Now = time.Now
