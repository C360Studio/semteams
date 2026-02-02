// Package cli provides Prometheus metrics for CLI input component.
package cli

import (
	"sync"

	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// cliMetrics holds Prometheus metrics for the CLI input component.
type cliMetrics struct {
	messagesPublished prometheus.Counter
	signalsSent       *prometheus.CounterVec
	responsesReceived *prometheus.CounterVec
}

// Package-level metrics cache (keyed by registry to allow test isolation)
var (
	metricsMu    sync.Mutex
	metricsCache = make(map[*metric.MetricsRegistry]*cliMetrics)
	nilMetrics   *cliMetrics
	nilOnce      sync.Once
)

// getMetrics returns the metrics instance for the given registry, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *cliMetrics {
	// Special handling for nil registry (production use with default Prometheus registry)
	if registry == nil {
		nilOnce.Do(func() {
			nilMetrics = createAndRegisterMetrics(nil)
		})
		return nilMetrics
	}

	// For non-nil registries, create per-registry instances (test isolation)
	metricsMu.Lock()
	defer metricsMu.Unlock()

	if m, exists := metricsCache[registry]; exists {
		return m
	}

	m := createAndRegisterMetrics(registry)
	metricsCache[registry] = m
	return m
}

// createAndRegisterMetrics creates a new metrics instance and registers it.
func createAndRegisterMetrics(registry *metric.MetricsRegistry) *cliMetrics {
	m := &cliMetrics{
		messagesPublished: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "cli_input",
			Name:      "messages_published_total",
			Help:      "Total number of user messages published from CLI",
		}),

		signalsSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "cli_input",
			Name:      "signals_sent_total",
			Help:      "Total number of signals sent by type",
		}, []string{"signal_type"}),

		responsesReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "cli_input",
			Name:      "responses_received_total",
			Help:      "Total number of responses received by type",
		}, []string{"type"}),
	}

	// Register metrics with the metrics registry if available
	if registry != nil {
		_ = registry.RegisterCounter("cli-input", "messages_published_total", m.messagesPublished)
		_ = registry.RegisterCounterVec("cli-input", "signals_sent_total", m.signalsSent)
		_ = registry.RegisterCounterVec("cli-input", "responses_received_total", m.responsesReceived)
	} else {
		// Fallback to default prometheus registry for production
		_ = prometheus.DefaultRegisterer.Register(m.messagesPublished)
		_ = prometheus.DefaultRegisterer.Register(m.signalsSent)
		_ = prometheus.DefaultRegisterer.Register(m.responsesReceived)
	}

	return m
}

// recordMessagePublished increments the messages published counter.
func (m *cliMetrics) recordMessagePublished() {
	m.messagesPublished.Inc()
}

// recordSignalSent increments the signals sent counter for a signal type.
func (m *cliMetrics) recordSignalSent(signalType string) {
	m.signalsSent.WithLabelValues(signalType).Inc()
}

// recordResponseReceived increments the responses received counter for a response type.
func (m *cliMetrics) recordResponseReceived(responseType string) {
	m.responsesReceived.WithLabelValues(responseType).Inc()
}
