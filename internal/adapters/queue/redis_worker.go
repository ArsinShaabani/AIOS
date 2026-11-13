package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (q *Redis) worker(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		raw, err := q.client.BRPopLPush(ctx, q.jobsKey(), q.processingKey(), 2*time.Second).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		// Track the in-flight deadline so a crashed worker's job is reclaimed.
		_ = q.client.ZAdd(ctx, q.inflightKey(), redis.Z{
			Score: float64(time.Now().UnixMilli()), Member: raw,
		}).Err()
		var job ports.Job
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			q.failed.Add(1)
			q.client.LRem(ctx, q.processingKey(), 1, raw)
			continue
		}
		q.mu.RLock()
		handler := q.handlers[job.Kind]
		q.mu.RUnlock()
		if handler == nil {
			q.failed.Add(1)
			q.client.LRem(ctx, q.processingKey(), 1, raw)
			q.log.Error("no handler for job", "kind", job.Kind)
			continue
		}
		q.running.Add(1)
		runErr := safeRun(ctx, handler, job)
		q.running.Add(-1)
		q.client.LRem(ctx, q.processingKey(), 1, raw)
		q.client.ZRem(ctx, q.inflightKey(), raw)
		if runErr == nil {
			q.processed.Add(1)
			continue
		}
		job.Attempts++
		if job.Attempts >= q.maxAttempts {
			q.failed.Add(1)
			q.log.Error("job exhausted retries", "kind", job.Kind, "error", runErr)
			continue
		}
		delay := min(time.Duration(1<<min(job.Attempts, 6))*time.Second, 2*time.Minute)
		if err := q.Enqueue(ctx, job, delay); err != nil {
			q.log.Warn("cannot requeue job", "kind", job.Kind, "error", err)
		}
	}
}

func safeRun(ctx context.Context, handler ports.JobHandler, job ports.Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = platform.Internal("queue.panic", "handler panicked")
		}
	}()
	return handler(ctx, job)
}

// Stop cancels workers.
func (q *Redis) Stop(ctx context.Context) {
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
	_ = q.client.Close()
}

// Stats reports queue depth.
func (q *Redis) Stats(ctx context.Context) (ports.QueueStats, error) {
	depth, err := q.client.LLen(ctx, q.jobsKey()).Result()
	if err != nil {
		return ports.QueueStats{}, err
	}
	processing, _ := q.client.LLen(ctx, q.processingKey()).Result()
	return ports.QueueStats{
		Depth:     int(depth),
		Running:   int(processing) + int(q.running.Load()),
		Processed: q.processed.Load(),
		Failed:    q.failed.Load(),
	}, nil
}

// New builds the configured queue implementation.
func New(cfg config.QueueConfig, cacheCfg config.CacheConfig, log *slog.Logger) (ports.Queue, error) {
	switch cfg.Driver {
	case "redis":
		return NewRedis(cfg, cacheCfg.Redis, cacheCfg.Prefix, log)
	default:
		return NewMemory(cfg.Workers, cfg.MaxAttempts, cfg.Workers*64, log), nil
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func itoa(n int64) string {
	var buf [20]byte
	if n == 0 {
		return "0"
	}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
