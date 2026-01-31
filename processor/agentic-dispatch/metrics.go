// Package agenticdispatch provides Prometheus metrics for agentic-dispatch component.
package agenticdispatch

import (
	"sync"

	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// routerMetrics holds Prometheus metrics for the router component.
type routerMetrics struct {
	messagesReceived    *prometheus.CounterVec
	commandsExecuted    *prometheus.CounterVec
	tasksSubmitted      prometheus.Counter
	activeLoops         prometheus.Gauge
	routingDuration     prometheus.Histogram
	completionsReceived *prometheus.CounterVec
}

// Package-level metrics cache (keyed by registry to allow test isolation)
var (
	metricsMu    sync.Mutex
	metricsCache = make(map[*metric.MetricsRegistry]*routerMetrics)
	nilMetrics   *routerMetrics
	nilOnce      sync.Once
)

// getMetrics returns the metrics instance for the given registry, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *routerMetrics {
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
func createAndRegisterMetrics(registry *metric.MetricsRegistry) *routerMetrics {
	m := &routerMetrics{
		messagesReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "router",
			Name:      "messages_received_total",
			Help:      "Total number of user messages received by channel type",
		}, []string{"channel_type"}),

		commandsExecuted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "router",
			Name:      "commands_executed_total",
			Help:      "Total number of commands executed",
		}, []string{"command"}),

		tasksSubmitted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "router",
			Name:      "tasks_submitted_total",
			Help:      "Total number of tasks submitted to agentic loops",
		}),

		activeLoops: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "semstreams",
			Subsystem: "router",
			Name:      "active_loops",
			Help:      "Number of currently active agentic loops",
		}),

		routingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "semstreams",
			Subsystem: "router",
			Name:      "routing_duration_seconds",
			Help:      "Duration of message routing operations in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		}),

		completionsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "router",
			Name:      "completions_received_total",
			Help:      "Total number of agent completions received by status",
		}, []string{"status"}),
	}

	// Register metrics with the metrics registry if available
	if registry != nil {
		_ = registry.RegisterCounterVec("router", "messages_received_total", m.messagesReceived)
		_ = registry.RegisterCounterVec("router", "commands_executed_total", m.commandsExecuted)
		_ = registry.RegisterCounter("router", "tasks_submitted_total", m.tasksSubmitted)
		_ = registry.RegisterGauge("router", "active_loops", m.activeLoops)
		_ = registry.RegisterHistogram("router", "routing_duration_seconds", m.routingDuration)
		_ = registry.RegisterCounterVec("router", "completions_received_total", m.completionsReceived)
	} else {
		// Fallback to default prometheus registry for production
		_ = prometheus.DefaultRegisterer.Register(m.messagesReceived)
		_ = prometheus.DefaultRegisterer.Register(m.commandsExecuted)
		_ = prometheus.DefaultRegisterer.Register(m.tasksSubmitted)
		_ = prometheus.DefaultRegisterer.Register(m.activeLoops)
		_ = prometheus.DefaultRegisterer.Register(m.routingDuration)
		_ = prometheus.DefaultRegisterer.Register(m.completionsReceived)
	}

	return m
}

// recordMessageReceived increments the messages received counter for a channel type.
func (m *routerMetrics) recordMessageReceived(channelType string) {
	m.messagesReceived.WithLabelValues(channelType).Inc()
}

// recordCommandExecuted increments the commands executed counter for a command.
func (m *routerMetrics) recordCommandExecuted(command string) {
	m.commandsExecuted.WithLabelValues(command).Inc()
}

// recordTaskSubmitted increments the tasks submitted counter.
func (m *routerMetrics) recordTaskSubmitted() {
	m.tasksSubmitted.Inc()
}

// recordLoopStarted increments the active loops gauge.
func (m *routerMetrics) recordLoopStarted() {
	m.activeLoops.Inc()
}

// recordLoopEnded decrements the active loops gauge.
func (m *routerMetrics) recordLoopEnded() {
	m.activeLoops.Dec()
}

// recordRoutingDuration records the duration of a routing operation.
func (m *routerMetrics) recordRoutingDuration(seconds float64) {
	m.routingDuration.Observe(seconds)
}

// recordCompletionReceived increments the completions received counter for a status.
func (m *routerMetrics) recordCompletionReceived(status string) {
	m.completionsReceived.WithLabelValues(status).Inc()
}
