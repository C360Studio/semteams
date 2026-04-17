// Package teamtools provides Prometheus metrics for agentic-tools component.
package teamtools

import (
	"sync"

	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// toolsMetrics holds Prometheus metrics for the agentic-tools component.
type toolsMetrics struct {
	// Executions
	executionsTotal   *prometheus.CounterVec
	executionDuration *prometheus.HistogramVec

	// Errors
	errorsTotal  *prometheus.CounterVec
	timeoutTotal *prometheus.CounterVec

	// Filtering
	filteredTotal *prometheus.CounterVec

	// Registry
	toolsRegistered prometheus.Gauge
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce sync.Once
	metrics     *toolsMetrics
)

// getMetrics returns the singleton metrics instance, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *toolsMetrics {
	metricsOnce.Do(func() {
		metrics = &toolsMetrics{
			executionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_tools",
				Name:      "executions_total",
				Help:      "Total tool executions by tool name and status",
			}, []string{"tool_name", "status"}),

			executionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_tools",
				Name:      "execution_duration_seconds",
				Help:      "Tool execution latency in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
			}, []string{"tool_name"}),

			errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_tools",
				Name:      "errors_total",
				Help:      "Total tool errors by tool name and error type",
			}, []string{"tool_name", "error_type"}),

			timeoutTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_tools",
				Name:      "timeout_total",
				Help:      "Total tool execution timeouts by tool name",
			}, []string{"tool_name"}),

			filteredTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_tools",
				Name:      "filtered_total",
				Help:      "Total tool calls filtered by reason",
			}, []string{"tool_name", "reason"}),

			toolsRegistered: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_tools",
				Name:      "registered",
				Help:      "Number of registered tools",
			}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounterVec("teams-tools", "executions_total", metrics.executionsTotal)
			_ = registry.RegisterHistogramVec("teams-tools", "execution_duration_seconds", metrics.executionDuration)
			_ = registry.RegisterCounterVec("teams-tools", "errors_total", metrics.errorsTotal)
			_ = registry.RegisterCounterVec("teams-tools", "timeout_total", metrics.timeoutTotal)
			_ = registry.RegisterCounterVec("teams-tools", "filtered_total", metrics.filteredTotal)
			_ = registry.RegisterGauge("teams-tools", "registered", metrics.toolsRegistered)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metrics.executionsTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.executionDuration)
			_ = prometheus.DefaultRegisterer.Register(metrics.errorsTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.timeoutTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.filteredTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.toolsRegistered)
		}
	})
	return metrics
}

// recordToolsRegistered sets the number of registered tools.
func (m *toolsMetrics) recordToolsRegistered(count int) {
	m.toolsRegistered.Set(float64(count))
}

// recordExecutionStart is called when a tool execution starts.
// Returns a function to call when the execution completes.
func (m *toolsMetrics) recordExecutionStart(toolName string) func(success bool) {
	start := prometheus.NewTimer(m.executionDuration.WithLabelValues(toolName))
	return func(success bool) {
		start.ObserveDuration()
		status := "success"
		if !success {
			status = "error"
		}
		m.executionsTotal.WithLabelValues(toolName, status).Inc()
	}
}

// recordExecutionSuccess records a successful tool execution.
func (m *toolsMetrics) recordExecutionSuccess(toolName string, durationSeconds float64) {
	m.executionsTotal.WithLabelValues(toolName, "success").Inc()
	m.executionDuration.WithLabelValues(toolName).Observe(durationSeconds)
}

// recordExecutionError records a failed tool execution.
func (m *toolsMetrics) recordExecutionError(toolName, errorType string, durationSeconds float64) {
	m.executionsTotal.WithLabelValues(toolName, "error").Inc()
	m.executionDuration.WithLabelValues(toolName).Observe(durationSeconds)
	m.errorsTotal.WithLabelValues(toolName, errorType).Inc()
}

// recordExecutionTimeout records a tool execution timeout.
func (m *toolsMetrics) recordExecutionTimeout(toolName string, durationSeconds float64) {
	m.executionsTotal.WithLabelValues(toolName, "timeout").Inc()
	m.executionDuration.WithLabelValues(toolName).Observe(durationSeconds)
	m.timeoutTotal.WithLabelValues(toolName).Inc()
}

// recordToolFiltered records a filtered tool call.
func (m *toolsMetrics) recordToolFiltered(toolName, reason string) {
	m.filteredTotal.WithLabelValues(toolName, reason).Inc()
}
