// Package agenticmodel provides Prometheus metrics for agentic-model component.
package agenticmodel

import (
	"sync"

	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// modelMetrics holds Prometheus metrics for the agentic-model component.
type modelMetrics struct {
	// Requests
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestsInFlight *prometheus.GaugeVec

	// Errors
	errorsTotal *prometheus.CounterVec

	// Response characteristics
	toolCallsReturned *prometheus.HistogramVec

	// Token usage
	tokensTotal *prometheus.CounterVec
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce sync.Once
	metrics     *modelMetrics
)

// getMetrics returns the singleton metrics instance, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *modelMetrics {
	metricsOnce.Do(func() {
		metrics = &modelMetrics{
			requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_model",
				Name:      "requests_total",
				Help:      "Total model requests by model and status",
			}, []string{"model", "status"}),

			requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_model",
				Name:      "request_duration_seconds",
				Help:      "Model request latency in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
			}, []string{"model"}),

			requestsInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_model",
				Name:      "requests_in_flight",
				Help:      "Number of model requests currently in flight",
			}, []string{"model"}),

			errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_model",
				Name:      "errors_total",
				Help:      "Total model errors by model and error type",
			}, []string{"model", "error_type"}),

			toolCallsReturned: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_model",
				Name:      "tool_calls_returned",
				Help:      "Distribution of tool calls per response",
				Buckets:   []float64{0, 1, 2, 3, 5, 10},
			}, []string{"model"}),

			tokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_model",
				Name:      "tokens_total",
				Help:      "Total tokens used by model and type (prompt/completion)",
			}, []string{"model", "type"}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounterVec("agentic-model", "requests_total", metrics.requestsTotal)
			_ = registry.RegisterHistogramVec("agentic-model", "request_duration_seconds", metrics.requestDuration)
			_ = registry.RegisterGaugeVec("agentic-model", "requests_in_flight", metrics.requestsInFlight)
			_ = registry.RegisterCounterVec("agentic-model", "errors_total", metrics.errorsTotal)
			_ = registry.RegisterHistogramVec("agentic-model", "tool_calls_returned", metrics.toolCallsReturned)
			_ = registry.RegisterCounterVec("agentic-model", "tokens_total", metrics.tokensTotal)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metrics.requestsTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.requestDuration)
			_ = prometheus.DefaultRegisterer.Register(metrics.requestsInFlight)
			_ = prometheus.DefaultRegisterer.Register(metrics.errorsTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.toolCallsReturned)
			_ = prometheus.DefaultRegisterer.Register(metrics.tokensTotal)
		}
	})
	return metrics
}

// recordRequestStart records the start of a model request.
func (m *modelMetrics) recordRequestStart(model string) {
	m.requestsInFlight.WithLabelValues(model).Inc()
}

// recordRequestComplete records a successful model request completion.
func (m *modelMetrics) recordRequestComplete(model string, durationSeconds float64, toolCalls int) {
	m.requestsInFlight.WithLabelValues(model).Dec()
	m.requestsTotal.WithLabelValues(model, "success").Inc()
	m.requestDuration.WithLabelValues(model).Observe(durationSeconds)
	m.toolCallsReturned.WithLabelValues(model).Observe(float64(toolCalls))
}

// recordRequestError records a failed model request.
func (m *modelMetrics) recordRequestError(model, errorType string, durationSeconds float64) {
	m.requestsInFlight.WithLabelValues(model).Dec()
	m.requestsTotal.WithLabelValues(model, "error").Inc()
	m.requestDuration.WithLabelValues(model).Observe(durationSeconds)
	m.errorsTotal.WithLabelValues(model, errorType).Inc()
}

// recordTokenUsage records token usage for a request.
func (m *modelMetrics) recordTokenUsage(model string, promptTokens, completionTokens int) {
	if promptTokens > 0 {
		m.tokensTotal.WithLabelValues(model, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		m.tokensTotal.WithLabelValues(model, "completion").Add(float64(completionTokens))
	}
}
