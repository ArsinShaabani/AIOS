package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Redis is a multi-instance queue built on lists + a delayed sorted set.
// Workers use BRPOPLPUSH into a processing list so a crash does not lose jobs;
// Start re-queues leftover processing entries (single-consumer-group model).
type Redis struct {
	client      *redis.Client
	prefix      string
	workers     int
	maxAttempts int
	pollEvery   time.Duration
	handlers    map[string]ports.JobHandler
	mu          sync.RWMutex
	processed   atomic.Int64
	failed      atomic.Int64
	running     atomic.Int64
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	log         *slog.Logger
}

var _ ports.Queue = (*Redis)(nil)

// NewRedis builds a Redis queue.
func NewRedis(cfg config.QueueConfig, cacheCfg config.RedisConfig, prefix string, log *slog.Logger) (*Redis, error) {
	if log == nil {
		log = slog.Default()
	}
	client := redis.NewClient(&redis.Options{
		Addr:         cacheCfg.Addr,
		Password:     cacheCfg.Password,
		DB:           cacheCfg.DB,
		PoolSize:     max(cacheCfg.PoolSize, 2),
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, platform.Wrap(platform.KindUnavailable, "queue.redis", "redis unreachable", err)
	}
	if prefix == "" {
		prefix = "aios"
	}
	return &Redis{
		client:      client,
		prefix:      prefix + ":queue:",
		workers:     max(cfg.Workers, 1),
		maxAttempts: max(cfg.MaxAttempts, 3),
		pollEvery:   maxDuration(cfg.PollInterval.Std(), 200*time.Millisecond),
		handlers:    make(map[string]ports.JobHandler),
		log:         log,
	}, nil
}

func (q *Redis) jobsKey() string       { return q.prefix + "jobs" }
func (q *Redis) processingKey() string { return q.prefix + "processing" }
func (q *Redis) delayedKey() string    { return q.prefix + "delayed" }

// Register binds a handler.
func (q *Redis) Register(kind string, handler ports.JobHandler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[kind] = handler
}

// Enqueue pushes a job now or into the delayed set.
func (q *Redis) Enqueue(ctx context.Context, job ports.Job, delay time.Duration) error {
	raw, err := json.Marshal(job)
	if err != nil {
		return platform.Wrap(platform.KindInvalid, "queue.encode", "cannot encode job", err)
	}
	if delay > 0 {
		return q.client.ZAdd(ctx, q.delayedKey(), redis.Z{Score: float64(time.Now().Add(delay).UnixMilli()), Member: raw}).Err()
	}
	return q.client.LPush(ctx, q.jobsKey(), raw).Err()
}

// Start recovers in-flight jobs and launches workers + promoter.
func (q *Redis) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	q.cancel = cancel

	// Recover jobs left in the processing list by a previous crash.
	if items, err := q.client.LRange(ctx, q.processingKey(), 0, 999).Result(); err == nil && len(items) > 0 {
		for _, item := range items {
			_ = q.client.LPush(ctx, q.jobsKey(), item).Err()
		}
		_ = q.client.Del(ctx, q.processingKey()).Err()
		q.log.Warn("requeued orphaned redis jobs", "count", len(items))
	}

	q.wg.Add(1)
	go q.promote(ctx)
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx)
	}
}

func (q *Redis) promote(ctx context.Context) {
	defer q.wg.Done()
	ticker := time.NewTicker(q.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UnixMilli()
			items, err := q.client.ZRangeByScore(ctx, q.delayedKey(), &redis.ZRangeBy{
				Min: "-inf", Max: itoa(now), Count: 50,
			}).Result()
			if err != nil || len(items) == 0 {
				continue
			}
			for _, item := range items {
				if err := q.client.LPush(ctx, q.jobsKey(), item).Err(); err == nil {
					q.client.ZRem(ctx, q.delayedKey(), item)
				}
			}
		}
	}
}
