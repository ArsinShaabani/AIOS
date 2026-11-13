// Package ports defines the interfaces the AIOS core depends on. Adapters
// (SQLite, Postgres, Redis, LLM APIs, MQTT, ...) implement these; the core never
// imports an adapter. This is the seam that keeps the deployment a modular
// monolith today and a set of microservices tomorrow.
package ports

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// UserFilter narrows user listings.
type UserFilter struct {
	Role   domain.Role
	Status string
	Query  string
}

// UserRepository persists principals.
type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, u *domain.User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, page platform.Page, filter UserFilter) ([]domain.User, int, error)
	Count(ctx context.Context) (int, error)
}

// SessionRepository persists conversations.
type SessionRepository interface {
	Create(ctx context.Context, s *domain.Session) error
	Get(ctx context.Context, id string) (*domain.Session, error)
	Update(ctx context.Context, s *domain.Session) error
	Delete(ctx context.Context, id string) error
	ListByUser(ctx context.Context, userID string, page platform.Page) ([]domain.Session, int, error)
	Touch(ctx context.Context, id string, messageDelta int, at time.Time) error
	Count(ctx context.Context) (int, error)
}

// MessageRepository persists turns and supports context loading.
type MessageRepository interface {
	Create(ctx context.Context, m *domain.Message) error
	Get(ctx context.Context, id string) (*domain.Message, error)
	ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.Message, error)
	ListRecent(ctx context.Context, sessionID string, limit int) ([]domain.Message, error)
	CountBySession(ctx context.Context, sessionID string) (int, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}

// MemoryFilter narrows memory listings.
type MemoryFilter struct {
	Tier      domain.MemoryTier
	Kind      domain.MemoryKind
	SessionID string
	Query     string
	Pinned    *bool
}

// MemoryRepository is the durable store for all three memory tiers. Semantic
// similarity search lives in the VectorStore port; this repository owns CRUD,
// recency and maintenance.
type MemoryRepository interface {
	Create(ctx context.Context, m *domain.Memory) error
	Update(ctx context.Context, m *domain.Memory) error
	Get(ctx context.Context, id string) (*domain.Memory, error)
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID string) (int, error)
	ListByUser(ctx context.Context, userID string, filter MemoryFilter, page platform.Page) ([]domain.Memory, int, error)
	RecentByUser(ctx context.Context, userID string, limit int) ([]domain.Memory, error)
	ListWithEmbeddings(ctx context.Context, userID string, limit int) ([]domain.Memory, error)
	IncrementUse(ctx context.Context, ids []string, at time.Time) error
	PruneExpired(ctx context.Context, now time.Time, limit int) (int, error)
	PruneOld(ctx context.Context, before time.Time, limit int) (int, error)
	CountByUser(ctx context.Context, userID string) (int, error)
	Count(ctx context.Context) (int, error)
}

// PersonalityRepository stores presets and user-defined personalities.
type PersonalityRepository interface {
	Create(ctx context.Context, p *domain.Personality) error
	Update(ctx context.Context, p *domain.Personality) error
	Get(ctx context.Context, id string) (*domain.Personality, error)
	Delete(ctx context.Context, id string) error
	ListAvailable(ctx context.Context, ownerID string) ([]domain.Personality, error)
	Count(ctx context.Context) (int, error)
}

// AgentRepository stores agent definitions (data-driven, not compiled in).
type AgentRepository interface {
	Upsert(ctx context.Context, a *domain.Agent) error
	Get(ctx context.Context, id string) (*domain.Agent, error)
	List(ctx context.Context, enabledOnly bool) ([]domain.Agent, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}

// DeviceFilter narrows device listings.
type DeviceFilter struct {
	Kind   domain.DeviceKind
	Status string
	Room   string
	Query  string
}

// DeviceRepository persists devices and their hashed credentials.
type DeviceRepository interface {
	Create(ctx context.Context, d *domain.Device) error
	Update(ctx context.Context, d *domain.Device) error
	Get(ctx context.Context, id string) (*domain.Device, error)
	GetByTokenPrefix(ctx context.Context, prefix string) (*domain.Device, error)
	ListByUser(ctx context.Context, userID string, filter DeviceFilter, page platform.Page) ([]domain.Device, int, error)
	ListAll(ctx context.Context, filter DeviceFilter, page platform.Page) ([]domain.Device, int, error)
	SetStatus(ctx context.Context, id, status string, at time.Time) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
	CountByStatus(ctx context.Context) (map[string]int, error)
}

// PluginRepository persists installed plugins.
type PluginRepository interface {
	Upsert(ctx context.Context, p *domain.Plugin) error
	Get(ctx context.Context, id string) (*domain.Plugin, error)
	List(ctx context.Context) ([]domain.Plugin, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}
