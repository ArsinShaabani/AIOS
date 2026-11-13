package platform

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoggerRedactionAndContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger("debug", "text", &buf)
	ctx := WithLogger(context.Background(), logger)
	LoggerFrom(ctx).Info("login", "password", "hunter2", "api_key", "sk-123", "user", "u1")
	out := buf.String()
	if strings.Contains(out, "hunter2") || strings.Contains(out, "sk-123") {
		t.Fatalf("secrets leaked: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") || !strings.Contains(out, "u1") {
		t.Fatalf("unexpected log: %s", out)
	}
	if LoggerFrom(context.Background()) == nil {
		t.Fatal("default logger missing")
	}
	jsonLogger := NewLogger("info", "json", &buf)
	jsonLogger.Debug("hidden")
	if strings.Contains(buf.String(), "hidden") {
		t.Fatal("debug logged at info level")
	}
}

func TestErrorHelpers(t *testing.T) {
	base := errors.New("root")
	wrapped := Wrap(KindUnavailable, "code", "message", base)
	if !errors.Is(wrapped, base) || wrapped.Unwrap() != base {
		t.Fatal("wrapping broken")
	}
	formatted := Errorf(KindInvalid, "code", "bad %s", "value")
	if formatted.Message != "bad value" || formatted.HTTPStatus() != 400 {
		t.Fatalf("formatted: %+v", formatted)
	}
	if AsError(nil) != nil {
		t.Fatal("nil should map to nil")
	}
}

func TestRetrySucceedsAfterFailures(t *testing.T) {
	attempts := 0
	err := (Backoff{Base: time.Millisecond, Max: 5 * time.Millisecond}).Retry(context.Background(), 4, func(ctx context.Context, attempt int) error {
		attempts++
		if attempts < 3 {
			return Unavailable("x", "retry")
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("retry: %v attempts=%d", err, attempts)
	}
	if Retryable(nil) || !Retryable(Unavailable("x", "y")) || Retryable(Invalid("x", "y")) {
		t.Fatal("Retryable classification wrong")
	}
}

func TestLoggerFormatFallbacks(t *testing.T) {
	l := NewLogger("warn", "", nil)
	if l == nil {
		t.Fatal("nil logger")
	}
	if slog.Default() == nil {
		t.Fatal("default slog missing")
	}
}
