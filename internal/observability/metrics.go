// Package observability provides Prometheus metrics, host statistics and health
// checks. It is intentionally dependency-light so it runs on small devices.
package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the AIOS Prometheus registry.
type Metrics struct {
	Registry     *prometheus.Registry
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
	LLMRequests  *prometheus.CounterVec
	LLMDuration  *prometheus.HistogramVec
	LLMTokens    *prometheus.CounterVec
	LLMCost      *prometheus.CounterVec
	Fallbacks    *prometheus.CounterVec
	Chats        *prometheus.CounterVec
	ToolCalls    *prometheus.CounterVec
	QueueDepth   prometheus.Gauge
	QueueRunning prometheus.Gauge
	Devices      *prometheus.GaugeVec
	Vectors      prometheus.Gauge
	Memories     prometheus.Gauge
	ActiveVoices prometheus.Gauge
	BuildInfo    *prometheus.GaugeVec
	started      time.Time
}

func cv(name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "aios", Name: name, Help: help}, labels)
}

func hv(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "aios", Name: name, Help: help, Buckets: buckets,
	}, labels)
}

func g(name, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "aios", Name: name, Help: help})
}

// NewMetrics builds and registers all collectors.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg, started: time.Now(),
		HTTPRequests: cv("http_requests_total", "HTTP requests by route and status.", "method", "route", "status"),
		HTTPDuration: hv("http_request_duration_seconds", "HTTP request latency.",
			[]float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}, "method", "route"),
		LLMRequests: cv("llm_requests_total", "Model requests by provider/model/result.", "provider", "model", "status"),
		LLMDuration: hv("llm_request_duration_seconds", "Model latency.",
			[]float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 60, 120}, "provider", "model"),
		LLMTokens:    cv("llm_tokens_total", "Tokens processed.", "provider", "model", "kind"),
		LLMCost:      cv("llm_cost_usd_total", "Estimated model spend in USD.", "provider", "model"),
		Fallbacks:    cv("llm_fallbacks_total", "Provider fallbacks.", "provider", "model"),
		Chats:        cv("chats_total", "Chat turns by agent and channel.", "agent", "channel"),
		ToolCalls:    cv("tool_calls_total", "Tool invocations by tool and result.", "tool", "status"),
		QueueDepth:   g("queue_depth", "Pending jobs."),
		QueueRunning: g("queue_running", "Jobs in flight."),
		Devices:      prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "aios", Name: "devices", Help: "Registered devices by status."}, []string{"status"}),
		Vectors:      g("memory_vectors", "Vectors in the semantic index."),
		Memories:     g("memory_records_total", "Stored memory records."),
		ActiveVoices: g("voice_active_sessions", "Open voice sessions."),
		BuildInfo:    prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "aios", Name: "build_info", Help: "Build metadata."}, []string{"version", "go_version"}),
	}
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(m.HTTPRequests, m.HTTPDuration, m.LLMRequests, m.LLMDuration, m.LLMTokens,
		m.LLMCost, m.Fallbacks, m.Chats, m.ToolCalls, m.QueueDepth, m.QueueRunning, m.Devices,
		m.Vectors, m.Memories, m.ActiveVoices, m.BuildInfo)
	return m
}

// Handler returns the /metrics handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{MaxRequestsInFlight: 4})
}

// SetBuild records version metadata once at boot.
func (m *Metrics) SetBuild(version, goVersion string) {
	m.BuildInfo.WithLabelValues(version, goVersion).Set(1)
}

// Uptime returns how long the process has been running.
func (m *Metrics) Uptime() time.Duration { return time.Since(m.started) }
