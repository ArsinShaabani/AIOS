package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

type ctxKey string

const (
	principalKey ctxKey = "principal"
	userKey      ctxKey = "user"
	requestIDKey ctxKey = "request_id"
	deviceKey    ctxKey = "device"
)

// writeJSON serialises a response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

type apiError struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Meta    map[string]string `json:"meta,omitempty"`
		TraceID string            `json:"trace_id,omitempty"`
	} `json:"error"`
}

// fail maps a typed error onto HTTP without leaking internals.
func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	appErr := platform.AsError(err)
	status := appErr.HTTPStatus()
	code, message := appErr.Code, appErr.Message
	if appErr.Kind == platform.KindInternal {
		message = "internal error"
		a.Log.Error("request failed", "path", r.URL.Path, "error", err, "request_id", requestID(r.Context()))
	}
	var payload apiError
	payload.Error.Code = code
	payload.Error.Message = message
	payload.Error.Meta = appErr.Meta
	payload.Error.TraceID = requestID(r.Context())
	writeJSON(w, status, payload)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return platform.Invalid("body.missing", "request body is required")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return platform.Invalid("body.empty", "request body is empty")
		}
		return platform.Invalid("body.invalid", "request body is not valid JSON")
	}
	return nil
}

func requestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func principalFrom(ctx context.Context) *security.Principal {
	if v, ok := ctx.Value(principalKey).(*security.Principal); ok {
		return v
	}
	return nil
}

// clientIP extracts the caller address, honouring X-Forwarded-For only when the
// immediate peer is loopback (a local reverse proxy).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "127.0.0.1" || host == "::1" {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if idx := strings.IndexByte(fwd, ','); idx >= 0 {
				return strings.TrimSpace(fwd[:idx])
			}
			return strings.TrimSpace(fwd)
		}
	}
	return host
}

func bearer(r *http.Request) string { return security.BearerToken(r.Header.Get("Authorization")) }

func wantStream(r *http.Request) bool {
	if r.URL.Query().Get("stream") == "true" {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}
