package platform

import (
	"sync"
	"time"
)

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// Breaker is a tiny three-state circuit breaker. It protects the brain from
// hammering a failing LLM provider (and from paying for timeouts) while
// recovering automatically.
type Breaker struct {
	mu           sync.Mutex
	state        breakerState
	failures     int
	successes    int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
	now          func() time.Time
}

// NewBreaker creates a breaker that opens after `threshold` consecutive
// failures and probes again after resetTimeout.
func NewBreaker(threshold int, resetTimeout time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = 5
	}
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	return &Breaker{threshold: threshold, resetTimeout: resetTimeout, now: time.Now}
}

// Allow reports whether a call may proceed.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case breakerOpen:
		if b.now().Sub(b.lastFailure) >= b.resetTimeout {
			b.state = breakerHalfOpen
			b.successes = 0
			return true
		}
		return false
	case breakerHalfOpen:
		// Allow one probe at a time.
		return b.successes == 0
	default:
		return true
	}
}

// Success records a successful call, closing the breaker after two probes.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	if b.state == breakerHalfOpen {
		b.successes++
		if b.successes >= 2 {
			b.state = breakerClosed
			b.successes = 0
		}
	}
}

// Failure records a failed call, opening the breaker at the threshold.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.lastFailure = b.now()
	if b.state == breakerHalfOpen || b.failures >= b.threshold {
		b.state = breakerOpen
	}
}

// State exposes a human readable state for metrics/dashboards.
func (b *Breaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case breakerOpen:
		return "open"
	case breakerHalfOpen:
		return "half_open"
	default:
		return "closed"
	}
}
