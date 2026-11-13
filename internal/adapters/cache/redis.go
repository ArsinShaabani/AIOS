package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Redis is the shared cache implementation for multi-instance deployments.
type Redis struct {
	client *redis.Client
	prefix string
}

var _ ports.Cache = (*Redis)(nil)

// NewRedis connects to Redis and verifies connectivity.
func NewRedis(ctx context.Context, cfg config.RedisConfig, prefix string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     max(cfg.PoolSize, 2),
		MinIdleConns: 1,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, platform.Wrap(platform.KindUnavailable, "cache.redis", "redis unreachable", err)
	}
	return &Redis{client: client, prefix: prefix + ":"}, nil
}

func (r *Redis) key(k string) string { return r.prefix + k }

// Get reads a key.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	raw, err := r.client.Get(ctx, r.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, platform.Wrap(platform.KindUnavailable, "cache.get", "redis get failed", err)
	}
	return raw, true, nil
}

// Set writes a key with TTL.
func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := r.client.Set(ctx, r.key(key), value, ttl).Err(); err != nil {
		return platform.Wrap(platform.KindUnavailable, "cache.set", "redis set failed", err)
	}
	return nil
}

// Delete removes keys.
func (r *Redis) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = r.key(k)
	}
	if err := r.client.Del(ctx, prefixed...).Err(); err != nil {
		return platform.Wrap(platform.KindUnavailable, "cache.delete", "redis delete failed", err)
	}
	return nil
}

// Incr increments a counter, setting the TTL when the key is created.
func (r *Redis) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	k := r.key(key)
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.ExpireNX(ctx, k, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, platform.Wrap(platform.KindUnavailable, "cache.incr", "redis incr failed", err)
	}
	return incr.Val(), nil
}

// Ping checks connectivity.
func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Close closes the pool.
func (r *Redis) Close() error { return r.client.Close() }

// New builds the configured cache implementation.
func New(ctx context.Context, cfg config.CacheConfig) (ports.Cache, error) {
	switch cfg.Driver {
	case "redis":
		return NewRedis(ctx, cfg.Redis, cfg.Prefix)
	default:
		return NewMemory(cfg.MaxEntries, cfg.DefaultTTL.Std()), nil
	}
}
