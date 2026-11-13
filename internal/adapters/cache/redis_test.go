package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/ArsinShaabani/AIOS/internal/config"
)

func TestRedisCache(t *testing.T) {
	mr := miniredis.RunT(t)
	ctx := context.Background()
	c, err := NewRedis(ctx, config.RedisConfig{Addr: mr.Addr(), PoolSize: 2}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := c.Get(ctx, "k")
	if err != nil || !ok || string(raw) != "v" {
		t.Fatalf("get: %v %v %q", ok, err, raw)
	}
	if _, ok, _ := c.Get(ctx, "missing"); ok {
		t.Fatal("missing key returned")
	}
	if n, err := c.Incr(ctx, "counter", time.Minute); err != nil || n != 1 {
		t.Fatalf("incr: %d %v", n, err)
	}
	if n, _ := c.Incr(ctx, "counter", time.Minute); n != 2 {
		t.Fatalf("incr twice: %d", n)
	}
	if err := c.Delete(ctx, "k", "counter"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx); err != nil {
		t.Fatal("empty delete should be a no-op")
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	built, err := New(ctx, config.CacheConfig{Driver: "redis", Prefix: "p", Redis: config.RedisConfig{Addr: mr.Addr()}})
	if err != nil || built == nil {
		t.Fatalf("factory: %v", err)
	}
	_ = built.Close()
	if _, err := NewRedis(ctx, config.RedisConfig{Addr: "127.0.0.1:1"}, "x"); err == nil {
		t.Fatal("unreachable redis accepted")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
