package observability

import (
	"context"
	"sort"
	"time"
)

// Check is one dependency probe.
type Check struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

// Report is returned by /healthz and /readyz.
type Report struct {
	Status  string  `json:"status"` // ok | degraded | down
	Version string  `json:"version"`
	Uptime  int64   `json:"uptime_sec"`
	Checks  []Check `json:"checks,omitempty"`
}

type namedCheck struct {
	name string
	fn   func(ctx context.Context) error
}

// Health aggregates dependency checks.
type Health struct {
	version string
	started time.Time
	checks  []namedCheck
}

// NewHealth builds the health service.
func NewHealth(version string) *Health {
	return &Health{version: version, started: time.Now()}
}

// Add registers a probe.
func (h *Health) Add(name string, fn func(ctx context.Context) error) {
	h.checks = append(h.checks, namedCheck{name: name, fn: fn})
}

// Live returns a liveness report (never touches dependencies).
func (h *Health) Live() Report {
	return Report{Status: "ok", Version: h.version, Uptime: int64(time.Since(h.started).Seconds())}
}

// Ready probes dependencies with a short timeout.
func (h *Health) Ready(ctx context.Context) Report {
	report := Report{Status: "ok", Version: h.version, Uptime: int64(time.Since(h.started).Seconds())}
	for _, check := range h.checks {
		start := time.Now()
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := check.fn(cctx)
		cancel()
		c := Check{Name: check.name, Status: "ok", LatencyMS: time.Since(start).Milliseconds()}
		if err != nil {
			c.Status = "error"
			c.Detail = err.Error()
			report.Status = "degraded"
		}
		report.Checks = append(report.Checks, c)
	}
	sort.SliceStable(report.Checks, func(i, j int) bool { return report.Checks[i].Name < report.Checks[j].Name })
	return report
}
