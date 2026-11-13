package platform

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type loggerKey struct{}

// NewLogger returns a structured logger. Secrets are redacted centrally so a
// careless log statement cannot leak credentials or bearer tokens.
func NewLogger(level, format string, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{
		Level:       lvl,
		ReplaceAttr: redactAttr,
	}
	var h slog.Handler
	if strings.EqualFold(format, "text") {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h)
}

var sensitiveKeys = map[string]struct{}{
	"password": {}, "passwd": {}, "secret": {}, "token": {}, "access_token": {},
	"refresh_token": {}, "authorization": {}, "api_key": {}, "apikey": {},
	"private_key": {}, "client_secret": {}, "cookie": {}, "set-cookie": {},
	"device_token": {}, "encryption_key": {},
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	// Handles nested forms such as "provider.api_key".
	if idx := strings.LastIndex(key, "."); idx >= 0 {
		key = key[idx+1:]
	}
	if _, ok := sensitiveKeys[key]; ok {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

// WithLogger stores a logger on the context. Request-scoped loggers carry
// request_id / user_id and are retrieved with LoggerFrom.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFrom returns the context logger or the default logger.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
