package security

import (
	"sync"
	"time"
)

// RateLimiter is a keyed token-bucket limiter. It is used for per-IP request
// limits and stricter per-account login limits.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	ratePerSec float64
	burst      float64
	lastSweep  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter builds a limiter allowing perMinute sustained with the given
// burst. Non-positive values disable limiting (nil limiter).
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	if perMinute <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = perMinute
	}
	return &RateLimiter{
		buckets:    make(map[string]*bucket, 256),
		ratePerSec: float64(perMinute) / 60.0,
		burst:      float64(burst),
		lastSweep:  time.Now(),
	}
}

// Allow consumes one token for key.
func (l *RateLimiter) Allow(key string) bool { return l.AllowN(key, 1) }

// AllowN consumes n tokens for key.
func (l *RateLimiter) AllowN(key string, n int) bool {
	if l == nil {
		return true
	}
	if n <= 0 {
		n = 1
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = minFloat(l.burst, b.tokens+elapsed*l.ratePerSec)
	b.last = now
	if b.tokens < float64(n) {
		l.sweepLocked(now)
		return false
	}
	b.tokens -= float64(n)
	l.sweepLocked(now)
	return true
}

// RetryAfter reports how long until the key has a token again.
func (l *RateLimiter) RetryAfter(key string) time.Duration {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok || b.tokens >= 1 {
		return 0
	}
	secs := (1 - b.tokens) / l.ratePerSec
	return time.Duration(secs * float64(time.Second))
}

func (l *RateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute || len(l.buckets) < 4096 {
		return
	}
	cutoff := now.Add(-10 * time.Minute)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
	l.lastSweep = now
}

// Len reports tracked keys (metrics/tests).
func (l *RateLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
