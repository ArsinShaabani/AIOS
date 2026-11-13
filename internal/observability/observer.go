package observability

import "time"

// ObserveHTTP records one request.
func (m *Metrics) ObserveHTTP(method, route string, status int, d time.Duration) {
	m.HTTPRequests.WithLabelValues(method, route, statusLabel(status)).Inc()
	m.HTTPDuration.WithLabelValues(method, route).Observe(d.Seconds())
}

// ObserveLLM implements the model router observer.
func (m *Metrics) ObserveLLM(provider, model string, d time.Duration, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	m.LLMRequests.WithLabelValues(provider, model, status).Inc()
	m.LLMDuration.WithLabelValues(provider, model).Observe(d.Seconds())
}

// ObserveTokens records token usage and cost.
func (m *Metrics) ObserveTokens(provider, model string, prompt, completion int, cost float64) {
	m.LLMTokens.WithLabelValues(provider, model, "prompt").Add(float64(prompt))
	m.LLMTokens.WithLabelValues(provider, model, "completion").Add(float64(completion))
	if cost > 0 {
		m.LLMCost.WithLabelValues(provider, model).Add(cost)
	}
}

// ObserveFallback records a provider fallback.
func (m *Metrics) ObserveFallback(provider, model string) {
	m.Fallbacks.WithLabelValues(provider, model).Inc()
}

// ObserveChat counts a completed chat turn.
func (m *Metrics) ObserveChat(agent, channel string) { m.Chats.WithLabelValues(agent, channel).Inc() }

// ObserveTool counts a tool call.
func (m *Metrics) ObserveTool(tool string, isError bool) {
	status := "ok"
	if isError {
		status = "error"
	}
	m.ToolCalls.WithLabelValues(tool, status).Inc()
}

// SetQueue updates queue gauges.
func (m *Metrics) SetQueue(depth, running int) {
	m.QueueDepth.Set(float64(depth))
	m.QueueRunning.Set(float64(running))
}

// SetDevices updates device counts by status.
func (m *Metrics) SetDevices(byStatus map[string]int) {
	for _, s := range []string{"online", "offline", "unknown"} {
		m.Devices.WithLabelValues(s).Set(float64(byStatus[s]))
	}
}

// SetMemory updates memory/vector gauges.
func (m *Metrics) SetMemory(records, vectors int) {
	m.Memories.Set(float64(records))
	m.Vectors.Set(float64(vectors))
}

func statusLabel(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}
