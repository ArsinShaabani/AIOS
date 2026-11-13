package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// WebClient performs outbound HTTP with an SSRF guard: private, loopback and
// link-local addresses are refused unless explicitly allowed. This protects a
// LAN-hosted AIOS from being used to scan the local network.
type WebClient struct {
	client    *http.Client
	userAgent string
	maxBytes  int64
	block     bool
}

// NewWebClient builds the guarded client.
func NewWebClient(cfg config.ToolsConfig) *WebClient {
	timeout := cfg.HTTPTimeout.Std()
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	maxBytes := cfg.MaxFetchBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "AIOS/1.0"
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	blockPrivate := cfg.BlockPrivateIPs
	transport := &http.Transport{
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 8 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range ips {
				if blockPrivate && isPrivateIP(candidate.IP) {
					return nil, fmt.Errorf("blocked private address %s", candidate.IP)
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("only http(s) redirects are allowed")
			}
			return nil
		},
	}
	return &WebClient{client: client, userAgent: ua, maxBytes: maxBytes, block: blockPrivate}
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// Get fetches a URL and returns the body plus content type.
func (w *WebClient) Get(ctx context.Context, rawURL string, limit int64) ([]byte, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", platform.Invalid("web.url", "only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return nil, "", platform.Invalid("web.url", "URL host is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", platform.Wrap(platform.KindInvalid, "web.request", "invalid URL", err)
	}
	req.Header.Set("User-Agent", w.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.5")
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, "", platform.Wrap(platform.KindUnavailable, "web.fetch", "request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", platform.Wrap(platform.KindUnavailable, "web.status",
			fmt.Sprintf("upstream returned %d", resp.StatusCode), nil)
	}
	if limit <= 0 || limit > w.maxBytes {
		limit = w.maxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, "", platform.Wrap(platform.KindUnavailable, "web.read", "cannot read response", err)
	}
	return body, resp.Header.Get("Content-Type"), nil
}
