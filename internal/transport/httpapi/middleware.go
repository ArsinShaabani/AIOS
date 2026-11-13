package httpapi

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// statusWriter captures the response status for logging/metrics.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, platform.Internal("http.hijack", "connection cannot be hijacked")
	}
	return h.Hijack()
}

func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				a.Log.Error("panic recovered", "panic", p, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, apiError{})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *API) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" || len(rid) > 64 {
			rid = platform.NewID("req")
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		ctx = platform.WithLogger(ctx, a.Log.With("request_id", rid))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=()")
		if !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/graphql" {
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) cors(next http.Handler) http.Handler {
	allowed := originsSet(a.Config.Server.CORSOrigins)
	wildcard := allowed["*"]
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (wildcard || allowed[origin]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Device-Token, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Limiter == nil {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/healthz", "/readyz", "/metrics":
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if !a.Limiter.Allow(ip) {
			w.Header().Set("Retry-After", "30")
			a.fail(w, r, platform.RateLimited("http.rate_limited", "too many requests; slow down"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) bodyLimit(next http.Handler) http.Handler {
	max := a.Config.Server.MaxBodyBytes
	if max <= 0 {
		max = 1 << 20
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			limit := max
			if strings.Contains(r.URL.Path, "/voice/transcribe") {
				limit = 32 << 20 // audio uploads
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/documents") {
				limit = a.Config.RAG.MaxDocumentBytes + (1 << 20)
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		if a.Metrics != nil {
			route := routeLabel(r.URL.Path)
			a.Metrics.ObserveHTTP(r.Method, route, status, duration)
		}
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/static") {
			return
		}
		userID := ""
		if p := principalFrom(r.Context()); p != nil {
			userID = p.UserID
		}
		platform.LoggerFrom(r.Context()).Info("http",
			"method", r.Method, "path", r.URL.Path, "status", status,
			"duration_ms", duration.Milliseconds(), "ip", clientIP(r), "user_id", userID)
	})
}

// routeLabel collapses ids so metrics have bounded cardinality.
func routeLabel(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if len(p) > 24 || strings.Contains(p, "_") {
			parts[i] = ":id"
		}
	}
	return "/" + strings.Join(parts, "/")
}
