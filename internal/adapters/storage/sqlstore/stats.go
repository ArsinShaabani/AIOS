package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// StatsRepo implements ports.StatsRepository with cheap aggregate counts.
type StatsRepo struct {
	db    *DB
	usage *UsageRepo
}

func NewStatsRepo(db *DB, usage *UsageRepo) *StatsRepo { return &StatsRepo{db: db, usage: usage} }

// Overview builds the dashboard summary.
func (r *StatsRepo) Overview(ctx context.Context) (ports.Overview, error) {
	var o ports.Overview
	counts := map[string]*int{
		"SELECT COUNT(*) FROM users":        &o.Users,
		"SELECT COUNT(*) FROM sessions":     &o.Sessions,
		"SELECT COUNT(*) FROM messages":     &o.Messages,
		"SELECT COUNT(*) FROM devices":      &o.Devices,
		"SELECT COUNT(*) FROM agents":       &o.Agents,
		"SELECT COUNT(*) FROM plugins":      &o.Plugins,
		"SELECT COUNT(*) FROM memories":     &o.Memories,
		"SELECT COUNT(*) FROM audit_events": &o.AuditEvents,
	}
	for query, dest := range counts {
		if err := r.db.QueryRowContext(ctx, query).Scan(dest); err != nil {
			return o, mapErr("stats.overview", err)
		}
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status = 'pending'`).Scan(&o.PendingTasks); err != nil {
		return o, mapErr("stats.tasks", err)
	}
	statuses, err := (&DeviceRepo{db: r.db}).CountByStatus(ctx)
	if err != nil {
		return o, err
	}
	o.DevicesByStatus = statuses

	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	totals, err := r.usage.Totals(ctx, "", start, now.Add(time.Minute))
	if err != nil {
		return o, err
	}
	o.UsageToday = totals
	return o, nil
}
