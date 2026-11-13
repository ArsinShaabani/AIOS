package ports

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// AuditFilter narrows audit queries.
type AuditFilter struct {
	ActorID string
	Action  string
	Status  string
	From    *time.Time
	To      *time.Time
}

// AuditRepository is append-only by design.
type AuditRepository interface {
	Create(ctx context.Context, e *domain.AuditEvent) error
	List(ctx context.Context, filter AuditFilter, page platform.Page) ([]domain.AuditEvent, int, error)
	DeleteBefore(ctx context.Context, before time.Time) (int, error)
	Count(ctx context.Context) (int, error)
}

// UsageRepository powers cost/analytics dashboards.
type UsageRepository interface {
	Create(ctx context.Context, u *domain.UsageRecord) error
	Totals(ctx context.Context, userID string, from, to time.Time) (domain.UsageTotals, error)
	DailySeries(ctx context.Context, userID string, from, to time.Time) ([]UsagePoint, error)
	ByProvider(ctx context.Context, from, to time.Time) ([]ProviderUsage, error)
	Count(ctx context.Context) (int, error)
}

// UsagePoint is one day of aggregated usage.
type UsagePoint struct {
	Day      string  `json:"day"`
	Requests int     `json:"requests"`
	Tokens   int     `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

// ProviderUsage aggregates usage per provider.
type ProviderUsage struct {
	Provider string  `json:"provider"`
	Requests int     `json:"requests"`
	Tokens   int     `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

// SettingRepository stores JSON settings at global or user scope.
type SettingRepository interface {
	Get(ctx context.Context, scope, scopeID, key string) (*domain.Setting, error)
	Set(ctx context.Context, s *domain.Setting) error
	List(ctx context.Context, scope, scopeID string) ([]domain.Setting, error)
	Delete(ctx context.Context, scope, scopeID, key string) error
}

// TaskRepository is the durable side of the worker queue. Claim must be atomic
// so two workers never execute the same job.
type TaskRepository interface {
	Create(ctx context.Context, t *domain.Task) error
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.Task, error)
	Complete(ctx context.Context, id string, at time.Time) error
	Fail(ctx context.Context, id string, errMsg string, retryAt time.Time, dead bool, at time.Time) error
	Get(ctx context.Context, id string) (*domain.Task, error)
	List(ctx context.Context, status string, page platform.Page) ([]domain.Task, int, error)
	CountByStatus(ctx context.Context) (map[string]int, error)
	DeleteBefore(ctx context.Context, before time.Time) (int, error)
}

// TokenRepository persists refresh tokens and API keys. Only hashes are stored.
type TokenRepository interface {
	CreateRefresh(ctx context.Context, t *domain.RefreshToken) error
	GetRefreshByHash(ctx context.Context, hash string) (*domain.RefreshToken, error)
	RotateRefresh(ctx context.Context, oldID, newID string, at time.Time) error
	RevokeFamily(ctx context.Context, familyID string, at time.Time) error
	RevokeRefresh(ctx context.Context, id string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
	DeleteExpiredRefresh(ctx context.Context, before time.Time) (int, error)

	CreateAPIKey(ctx context.Context, k *domain.APIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error)
	ListAPIKeys(ctx context.Context, userID string) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id, userID string, at time.Time) error
	TouchAPIKey(ctx context.Context, id string, at time.Time) error
}

// StatsRepository provides cheap aggregate counters for the dashboard.
type StatsRepository interface {
	Overview(ctx context.Context) (Overview, error)
}

// Overview is the dashboard summary payload.
type Overview struct {
	Users           int                `json:"users"`
	Sessions        int                `json:"sessions"`
	Messages        int                `json:"messages"`
	Devices         int                `json:"devices"`
	DevicesByStatus map[string]int     `json:"devices_by_status"`
	Agents          int                `json:"agents"`
	Plugins         int                `json:"plugins"`
	Memories        int                `json:"memories"`
	PendingTasks    int                `json:"pending_tasks"`
	AuditEvents     int                `json:"audit_events"`
	UsageToday      domain.UsageTotals `json:"usage_today"`
}
