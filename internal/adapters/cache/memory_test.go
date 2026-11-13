package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCache(t *testing.T) {
	ctx := context.Background()
	c := NewMemory(2, time.Minute)
	if err := c.Set(ctx, "a", []byte("1"), 0); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := c.Get(ctx, "a"); !ok || string(v) != "1" {
		t.Fatalf("get: %q %v", v, ok)
	}
	if _, ok, _ := c.Get(ctx, "missing"); ok {
		t.Fatal("missing key returned")
	}
	if err := c.Set(ctx, "b", []byte("2"), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, "b"); ok {
		t.Fatal("expired key returned")
	}
	// LRU eviction: inserting a third entry evicts the least recently used.
	_ = c.Set(ctx, "c", []byte("3"), 0)
	_ = c.Set(ctx, "d", []byte("4"), 0)
	if c.Len() > 2 {
		t.Fatalf("cache exceeded max entries: %d", c.Len())
	}
	if _, ok, _ := c.Get(ctx, "a"); ok {
		t.Fatal("oldest entry was not evicted")
	}
	n, err := c.Incr(ctx, "counter", time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("incr: %d %v", n, err)
	}
	if n, _ := c.Incr(ctx, "counter", time.Minute); n != 2 {
		t.Fatalf("incr twice: %d", n)
	}
	if err := c.Delete(ctx, "counter"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, "counter"); ok {
		t.Fatal("delete failed")
	}
	if err := c.Ping(ctx); err != nil || c.Close() != nil {
		t.Fatal("lifecycle failed")
	}
}
