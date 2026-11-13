// Package queue implements the Queue port: an in-process worker pool and a
// Redis-backed queue for multi-instance deployments. Both provide at-least-once
// delivery with bounded retries and exponential backoff.
package queue

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Memory is a bounded, in-process queue. Jobs are lost on restart by design;
// durable work (reminders, consolidation) also lives in the tasks table and is
// re-enqueued by the scheduler on boot.
type Memory struct {
	workers     int
	maxAttempts int
	handlers    map[string]ports.JobHandler
	mu          sync.RWMutex
	ch          chan ports.Job
	processed   atomic.Int64
	failed      atomic.Int64
	running     atomic.Int64
	closed      atomic.Bool
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	log         *slog.Logger
}

var _ ports.Queue = (*Memory)(nil)

// NewMemory builds an in-memory queue.
func NewMemory(workers, maxAttempts, buffer int, log *slog.Logger) *Memory {
	if workers < 1 {
		workers = 1
	}
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	if buffer <= 0 {
		buffer = workers * 64
	}
	if log == nil {
		log = slog.Default()
	}
	return &Memory{
		workers:     workers,
		maxAttempts: maxAttempts,
		handlers:    make(map[string]ports.JobHandler),
		ch:          make(chan ports.Job, buffer),
		log:         log,
	}
}

// Register binds a handler to a job kind.
func (q *Memory) Register(kind string, handler ports.JobHandler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[kind] = handler
}

// Enqueue schedules a job.
func (q *Memory) Enqueue(ctx context.Context, job ports.Job, delay time.Duration) error {
	if q.closed.Load() {
		return platform.Unavailable("queue.closed", "queue is shutting down")
	}
	if delay > 0 {
		time.AfterFunc(delay, func() {
			q.push(job)
		})
		return nil
	}
	select {
	case q.ch <- job:
		return nil
	case <-ctx.Done():
		return platform.Wrap(platform.KindTimeout, "queue.enqueue", "enqueue cancelled", ctx.Err())
	}
}

func (q *Memory) push(job ports.Job) {
	if q.closed.Load() {
		return
	}
	defer func() { _ = recover() }() // channel closed during shutdown
	select {
	case q.ch <- job:
	default:
		q.failed.Add(1)
		q.log.Warn("queue full, dropping job", "kind", job.Kind, "job_id", job.ID)
	}
}

// Start launches workers.
func (q *Memory) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	q.cancel = cancel
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx)
	}
}

func (q *Memory) worker(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.ch:
			q.run(ctx, job)
		}
	}
}

func (q *Memory) run(ctx context.Context, job ports.Job) {
	q.mu.RLock()
	handler := q.handlers[job.Kind]
	q.mu.RUnlock()
	if handler == nil {
		q.failed.Add(1)
		q.log.Error("no handler for job", "kind", job.Kind, "job_id", job.ID)
		return
	}
	q.running.Add(1)
	defer q.running.Add(-1)

	err := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				q.log.Error("job panicked", "kind", job.Kind, "panic", p)
				err = platform.Internal("queue.panic", "handler panicked")
			}
		}()
		return handler(ctx, job)
	}()

	if err == nil {
		q.processed.Add(1)
		return
	}
	job.Attempts++
	if job.Attempts >= q.maxAttempts {
		q.failed.Add(1)
		q.log.Error("job exhausted retries", "kind", job.Kind, "job_id", job.ID, "error", err)
		return
	}
	delay := min(time.Duration(1<<min(job.Attempts, 6))*time.Second, 2*time.Minute)
	q.log.Warn("job failed, retrying", "kind", job.Kind, "job_id", job.ID, "attempt", job.Attempts, "in", delay.String(), "error", err)
	time.AfterFunc(delay, func() { q.push(job) })
}

// Stop drains workers and waits for in-flight jobs.
func (q *Memory) Stop(ctx context.Context) {
	if q.closed.Swap(true) {
		return
	}
	if q.cancel != nil {
		q.cancel()
	}
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Stats reports queue health.
func (q *Memory) Stats(context.Context) (ports.QueueStats, error) {
	return ports.QueueStats{
		Depth:     len(q.ch),
		Running:   int(q.running.Load()),
		Processed: q.processed.Load(),
		Failed:    q.failed.Load(),
	}, nil
}
