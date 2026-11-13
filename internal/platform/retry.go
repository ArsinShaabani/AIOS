package platform

import (
	"context"
	"math/rand/v2"
	"time"
)

// Backoff describes an exponential backoff with full jitter.
type Backoff struct {
	Base   time.Duration
	Max    time.Duration
	Jitter bool
}

// DefaultBackoff is tuned for remote LLM/HTTP providers: aggressive enough to
// recover from a transient blip, short enough to keep p95 latency sane.
var DefaultBackoff = Backoff{Base: 250 * time.Millisecond, Max: 8 * time.Second, Jitter: true}

func (b Backoff) delay(attempt int) time.Duration {
	if b.Base <= 0 {
		b.Base = 100 * time.Millisecond
	}
	if b.Max <= 0 {
		b.Max = 5 * time.Second
	}
	d := b.Base << min(attempt, 10)
	if d > b.Max {
		d = b.Max
	}
	if b.Jitter {
		// Full jitter avoids thundering herds after a provider outage.
		d = time.Duration(rand.Int64N(int64(d)) + int64(d)/2)
		if d > b.Max {
			d = b.Max
		}
	}
	return d
}

// Retry runs fn until it succeeds, the context is cancelled, or attempts are
// exhausted. The last error is returned.
func (b Backoff) Retry(ctx context.Context, attempts int, fn func(ctx context.Context, attempt int) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if err != nil {
				return err
			}
			return ctxErr
		}
		err = fn(ctx, i)
		if err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		t := time.NewTimer(b.delay(i))
		select {
		case <-ctx.Done():
			t.Stop()
			return err
		case <-t.C:
		}
	}
	return err
}

// Retryable reports whether an error class is worth retrying.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	k := kindOf(err)
	return k == KindUnavailable || k == KindTimeout
}
