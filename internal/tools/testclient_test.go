package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// clientFor routes every outbound host to the given test server, so tools with
// hard-coded public endpoints (Wikipedia, Nominatim, OSRM) can be exercised
// without network access.
func clientFor(srv *httptest.Server) *WebClient {
	host := strings.TrimPrefix(srv.URL, "http://")
	return &WebClient{
		client: &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = host
			return http.DefaultTransport.RoundTrip(req)
		})},
		userAgent: "aios-test",
		maxBytes:  4 << 20,
		block:     false,
	}
}
