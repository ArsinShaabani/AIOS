package security

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Auditor writes audit events asynchronously so a slow disk never blocks a
// request. Events that cannot be buffered are logged instead of dropped
// silently.
type Auditor struct {
	repo ports.AuditRepository
	ch   chan domain.AuditEvent
	log  *slog.Logger
	wg   sync.WaitGroup
	stop chan struct{}
	once sync.Once
}

// NewAuditor builds an auditor with a bounded buffer.
func NewAuditor(repo ports.AuditRepository, log *slog.Logger) *Auditor {
	if log == nil {
		log = slog.Default()
	}
	return &Auditor{repo: repo, ch: make(chan domain.AuditEvent, 512), log: log, stop: make(chan struct{})}
}

// Start launches the writer.
func (a *Auditor) Start(ctx context.Context) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-ctx.Done():
				a.drain()
				return
			case <-a.stop:
				a.drain()
				return
			case evt := <-a.ch:
				a.write(ctx, evt)
			}
		}
	}()
}

func (a *Auditor) drain() {
	for {
		select {
		case evt := <-a.ch:
			a.write(context.Background(), evt)
		default:
			return
		}
	}
}

func (a *Auditor) write(ctx context.Context, evt domain.AuditEvent) {
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := a.repo.Create(writeCtx, &evt); err != nil && !platform.IsNotFound(err) {
		a.log.Warn("cannot persist audit event", "action", evt.Action, "error", err)
	}
}

// Stop flushes buffered events.
func (a *Auditor) Stop(context.Context) {
	a.once.Do(func() {
		close(a.stop)
		a.wg.Wait()
	})
}

// Record enqueues an event. It never blocks the caller.
func (a *Auditor) Record(_ context.Context, evt domain.AuditEvent) {
	if evt.ID == "" {
		evt.ID = platform.NewID("aud")
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	if evt.ActorType == "" {
		evt.ActorType = "system"
	}
	if evt.Status == "" {
		evt.Status = "success"
	}
	select {
	case a.ch <- evt:
	default:
		a.log.Warn("audit buffer full; logging event inline", "action", evt.Action, "actor", evt.ActorID, "status", evt.Status)
	}
}

// Log is a convenience helper for common audit events.
func (a *Auditor) Log(ctx context.Context, actorID, actorType, action, resource, resourceID, status, ip, ua string, detail map[string]any) {
	a.Record(ctx, domain.AuditEvent{
		ActorID: actorID, ActorType: actorType, Action: action, Resource: resource,
		ResourceID: resourceID, Status: status, IP: ip, UserAgent: ua, Detail: detail,
	})
}
