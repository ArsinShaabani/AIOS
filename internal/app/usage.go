package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// UsageRecorder batches usage records off the request path.
type UsageRecorder struct {
	repo   ports.UsageRepository
	log    *slog.Logger
	ch     chan domain.UsageRecord
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewUsageRecorder builds the recorder.
func NewUsageRecorder(repo ports.UsageRepository, log *slog.Logger) *UsageRecorder {
	return &UsageRecorder{repo: repo, log: log, ch: make(chan domain.UsageRecord, 512)}
}

// Start launches the writer.
func (u *UsageRecorder) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	u.cancel = cancel
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		for {
			select {
			case <-ctx.Done():
				u.drain()
				return
			case rec := <-u.ch:
				u.write(ctx, rec)
			}
		}
	}()
}

// Record queues a usage record without blocking.
func (u *UsageRecorder) Record(_ context.Context, rec domain.UsageRecord) {
	select {
	case u.ch <- rec:
	default:
		u.log.Warn("usage buffer full; dropping record", "provider", rec.Provider)
	}
}

func (u *UsageRecorder) write(ctx context.Context, rec domain.UsageRecord) {
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := u.repo.Create(writeCtx, &rec); err != nil {
		u.log.Warn("cannot persist usage record", "error", err)
	}
}

func (u *UsageRecorder) drain() {
	for {
		select {
		case rec := <-u.ch:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			u.write(ctx, rec)
			cancel()
		default:
			return
		}
	}
}

// Stop flushes and exits.
func (u *UsageRecorder) Stop() {
	if u.cancel != nil {
		u.cancel()
	}
	u.wg.Wait()
}
