package platform

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestErrorsKindsAndStatus(t *testing.T) {
	cases := []struct {
		err    *Error
		kind   Kind
		status int
	}{
		{Invalid("c", "m"), KindInvalid, 400},
		{NotFound("c", "m"), KindNotFound, 404},
		{Conflict("c", "m"), KindConflict, 409},
		{Unauthorized("c", "m"), KindUnauthorized, 401},
		{Forbidden("c", "m"), KindForbidden, 403},
		{RateLimited("c", "m"), KindRateLimited, 429},
		{Unavailable("c", "m"), KindUnavailable, 503},
		{Internal("c", "m"), KindInternal, 500},
	}
	for _, c := range cases {
		if c.err.Kind != c.kind {
			t.Errorf("kind mismatch: %s", c.err.Kind)
		}
		if c.err.HTTPStatus() != c.status {
			t.Errorf("%s: got status %d want %d", c.kind, c.err.HTTPStatus(), c.status)
		}
		if c.err.Error() == "" {
			t.Error("error string must not be empty")
		}
	}
	wrapped := WrapInternal(errors.New("boom"))
	if !errors.Is(AsError(wrapped), wrapped) {
		t.Error("AsError should preserve typed errors")
	}
	if AsError(errors.New("plain")).Kind != KindInternal {
		t.Error("plain errors should map to internal")
	}
	if !IsNotFound(NotFound("x", "y")) || IsInvalid(NotFound("x", "y")) {
		t.Error("predicate helpers are wrong")
	}
}

func TestBreakerLifecycle(t *testing.T) {
	b := NewBreaker(2, 10*time.Millisecond)
	if !b.Allow() {
		t.Fatal("closed breaker should allow")
	}
	b.Failure()
	b.Failure()
	if b.Allow() {
		t.Fatal("open breaker must reject")
	}
	if b.State() != "open" {
		t.Fatalf("state=%s", b.State())
	}
	time.Sleep(15 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("half-open breaker should probe")
	}
	b.Success()
	b.Success()
	if b.State() != "closed" || !b.Allow() {
		t.Fatalf("breaker should close after two probes, state=%s", b.State())
	}
}

func TestRetryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := (Backoff{Base: time.Millisecond, Max: 5 * time.Millisecond}).Retry(ctx, 5, func(ctx context.Context, attempt int) error {
		attempts++
		cancel()
		return Unavailable("x", "try again")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts > 2 {
		t.Fatalf("retry ignored cancellation: %d attempts", attempts)
	}
}

func TestPageNormalize(t *testing.T) {
	p := Page{}.Normalize()
	if p.Limit != 25 || p.Offset != 0 {
		t.Fatalf("unexpected defaults: %+v", p)
	}
	if (Page{Limit: 1000}).Normalize().Limit != 200 {
		t.Fatal("limit must clamp")
	}
	res := NewPageResult[int](nil, 0, p)
	if res.Items == nil {
		t.Fatal("items must be non-nil")
	}
}

func TestIDsAndSlug(t *testing.T) {
	id := NewID("dev")
	if len(id) < 10 || id[:4] != "dev_" {
		t.Fatalf("bad id %q", id)
	}
	if Slug("  Hello, World! / Test_One ") != "hello-world-test-one" {
		t.Fatalf("slug: %q", Slug("  Hello, World! / Test_One "))
	}
	if Truncate("سلام دنیا", 4) != "سلا…" {
		t.Fatalf("rune truncate: %q", Truncate("سلام دنیا", 4))
	}
	tok, err := RandomToken(16)
	if err != nil || len(tok) < 16 {
		t.Fatalf("token: %v %q", err, tok)
	}
}
