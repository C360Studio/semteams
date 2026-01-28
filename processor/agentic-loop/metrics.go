// Package agenticloop provides Prometheus metrics for agentic-loop component.
package agenticloop

import (
	"sync"

	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// loopMetrics holds Prometheus metrics for the agentic-loop component.
type loopMetrics struct {
	// Loop lifecycle
	loopsCreated   prometheus.Counter
	loopsCompleted prometheus.Counter
	loopsFailed    *prometheus.CounterVec
	loopsTimeout   prometheus.Counter
	activeLoops    prometheus.Gauge

	// Iterations
	iterationsTotal   prometheus.Counter
	iterationsPerLoop prometheus.Histogram

	// Duration
	loopDuration *prometheus.HistogramVec

	// Trajectory
	trajectorySteps *prometheus.CounterVec

	// Tool calls
	toolCallsDispatched *prometheus.CounterVec
	toolResultsReceived *prometheus.CounterVec
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce sync.Once
	metrics     *loopMetrics
)

// getMetrics returns the singleton metrics instance, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *loopMetrics {
	metricsOnce.Do(func() {
		metrics = &loopMetrics{
			loopsCreated: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "loops_created_total",
				Help:      "Total number of agentic loops created",
			}),

			loopsCompleted: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "loops_completed_total",
				Help:      "Total number of agentic loops completed successfully",
			}),

			loopsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "loops_failed_total",
				Help:      "Total number of agentic loops that failed",
			}, []string{"reason"}),

			loopsTimeout: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "loops_timeout_total",
				Help:      "Total number of agentic loops that timed out",
			}),

			activeLoops: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "active_loops",
				Help:      "Number of currently active agentic loops",
			}),

			iterationsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "iterations_total",
				Help:      "Total number of iterations across all loops",
			}),

			iterationsPerLoop: prometheus.NewHistogram(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "iterations_per_loop",
				Help:      "Distribution of iterations per loop",
				Buckets:   []float64{1, 2, 3, 5, 10, 15, 20, 50},
			}),

			loopDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "duration_seconds",
				Help:      "Duration of agentic loops in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
			}, []string{"status"}),

			trajectorySteps: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "trajectory_steps_total",
				Help:      "Total trajectory steps by type",
			}, []string{"step_type"}),

			toolCallsDispatched: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "tool_calls_dispatched_total",
				Help:      "Total tool calls dispatched by tool name",
			}, []string{"tool_name"}),

			toolResultsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "agentic_loop",
				Name:      "tool_results_received_total",
				Help:      "Total tool results received by status",
			}, []string{"status"}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounter("agentic-loop", "loops_created_total", metrics.loopsCreated)
			_ = registry.RegisterCounter("agentic-loop", "loops_completed_total", metrics.loopsCompleted)
			_ = registry.RegisterCounterVec("agentic-loop", "loops_failed_total", metrics.loopsFailed)
			_ = registry.RegisterCounter("agentic-loop", "loops_timeout_total", metrics.loopsTimeout)
			_ = registry.RegisterGauge("agentic-loop", "active_loops", metrics.activeLoops)
			_ = registry.RegisterCounter("agentic-loop", "iterations_total", metrics.iterationsTotal)
			_ = registry.RegisterHistogram("agentic-loop", "iterations_per_loop", metrics.iterationsPerLoop)
			_ = registry.RegisterHistogramVec("agentic-loop", "duration_seconds", metrics.loopDuration)
			_ = registry.RegisterCounterVec("agentic-loop", "trajectory_steps_total", metrics.trajectorySteps)
			_ = registry.RegisterCounterVec("agentic-loop", "tool_calls_dispatched_total", metrics.toolCallsDispatched)
			_ = registry.RegisterCounterVec("agentic-loop", "tool_results_received_total", metrics.toolResultsReceived)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metrics.loopsCreated)
			_ = prometheus.DefaultRegisterer.Register(metrics.loopsCompleted)
			_ = prometheus.DefaultRegisterer.Register(metrics.loopsFailed)
			_ = prometheus.DefaultRegisterer.Register(metrics.loopsTimeout)
			_ = prometheus.DefaultRegisterer.Register(metrics.activeLoops)
			_ = prometheus.DefaultRegisterer.Register(metrics.iterationsTotal)
			_ = prometheus.DefaultRegisterer.Register(metrics.iterationsPerLoop)
			_ = prometheus.DefaultRegisterer.Register(metrics.loopDuration)
			_ = prometheus.DefaultRegisterer.Register(metrics.trajectorySteps)
			_ = prometheus.DefaultRegisterer.Register(metrics.toolCallsDispatched)
			_ = prometheus.DefaultRegisterer.Register(metrics.toolResultsReceived)
		}
	})
	return metrics
}

// recordLoopCreated increments the loops created counter and active gauge.
func (m *loopMetrics) recordLoopCreated() {
	m.loopsCreated.Inc()
	m.activeLoops.Inc()
}

// recordLoopCompleted records a successful loop completion.
func (m *loopMetrics) recordLoopCompleted(iterations int, durationSeconds float64) {
	m.loopsCompleted.Inc()
	m.activeLoops.Dec()
	m.iterationsPerLoop.Observe(float64(iterations))
	m.loopDuration.WithLabelValues("completed").Observe(durationSeconds)
}

// recordLoopFailed records a failed loop.
func (m *loopMetrics) recordLoopFailed(reason string, iterations int, durationSeconds float64) {
	m.loopsFailed.WithLabelValues(reason).Inc()
	m.activeLoops.Dec()
	m.iterationsPerLoop.Observe(float64(iterations))
	m.loopDuration.WithLabelValues("failed").Observe(durationSeconds)
}

// recordLoopTimeout records a loop timeout.
func (m *loopMetrics) recordLoopTimeout(iterations int, durationSeconds float64) {
	m.loopsTimeout.Inc()
	m.activeLoops.Dec()
	m.iterationsPerLoop.Observe(float64(iterations))
	m.loopDuration.WithLabelValues("timeout").Observe(durationSeconds)
}

// recordIteration increments the total iterations counter.
func (m *loopMetrics) recordIteration() {
	m.iterationsTotal.Inc()
}

// recordTrajectoryStep records a trajectory step by type.
func (m *loopMetrics) recordTrajectoryStep(stepType string) {
	m.trajectorySteps.WithLabelValues(stepType).Inc()
}

// recordToolCallDispatched records a tool call being dispatched.
func (m *loopMetrics) recordToolCallDispatched(toolName string) {
	m.toolCallsDispatched.WithLabelValues(toolName).Inc()
}

// recordToolResultReceived records a tool result being received.
func (m *loopMetrics) recordToolResultReceived(hasError bool) {
	status := "success"
	if hasError {
		status = "error"
	}
	m.toolResultsReceived.WithLabelValues(status).Inc()
}
