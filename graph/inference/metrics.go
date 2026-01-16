// Package inference provides structural anomaly detection for missing relationships.
package inference

import (
	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// ReviewMetrics provides Prometheus metrics for anomaly review processing.
type ReviewMetrics struct {
	// Timing
	reviewLatency prometheus.Histogram // Per-review processing latency

	// Counters
	approvedTotal prometheus.Counter // Total approved anomalies
	rejectedTotal prometheus.Counter // Total rejected anomalies
	deferredTotal prometheus.Counter // Total deferred to human review
	failedTotal   prometheus.Counter // Total failed processing attempts

	// Gauges
	pendingCount  prometheus.Gauge // Anomalies pending review
	workersActive prometheus.Gauge // Currently processing workers
}

// NewReviewMetrics creates a new ReviewMetrics instance using MetricsRegistry.
func NewReviewMetrics(component string, registry *metric.MetricsRegistry) *ReviewMetrics {
	if registry == nil {
		return nil
	}

	const (
		namespace = "semstreams"
		subsystem = "inference"
	)

	// Review latency - buckets tuned for review processing (typically 0.1-30s for LLM)
	reviewLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "review_latency_seconds",
		Help:        "Latency of anomaly review processing in seconds",
		ConstLabels: prometheus.Labels{"component": component},
		Buckets:     []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
	})
	registry.RegisterHistogram(subsystem, "review_latency_seconds", reviewLatency)

	approvedTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "approved_total",
		Help:        "Total number of anomalies approved (auto or LLM)",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter(subsystem, "approved_total", approvedTotal)

	rejectedTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "rejected_total",
		Help:        "Total number of anomalies rejected (auto or LLM)",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter(subsystem, "rejected_total", rejectedTotal)

	deferredTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "deferred_total",
		Help:        "Total number of anomalies deferred to human review",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter(subsystem, "deferred_total", deferredTotal)

	failedTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "failed_total",
		Help:        "Total number of failed anomaly processing attempts",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter(subsystem, "failed_total", failedTotal)

	pendingCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "pending_count",
		Help:        "Current number of anomalies pending review",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterGauge(subsystem, "pending_count", pendingCount)

	workersActive := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "workers_active",
		Help:        "Number of review workers currently processing",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterGauge(subsystem, "workers_active", workersActive)

	return &ReviewMetrics{
		reviewLatency: reviewLatency,
		approvedTotal: approvedTotal,
		rejectedTotal: rejectedTotal,
		deferredTotal: deferredTotal,
		failedTotal:   failedTotal,
		pendingCount:  pendingCount,
		workersActive: workersActive,
	}
}

// RecordApproved records an approved anomaly with latency.
func (m *ReviewMetrics) RecordApproved(latencySeconds float64) {
	if m == nil {
		return
	}
	m.approvedTotal.Inc()
	m.reviewLatency.Observe(latencySeconds)
}

// RecordRejected records a rejected anomaly with latency.
func (m *ReviewMetrics) RecordRejected(latencySeconds float64) {
	if m == nil {
		return
	}
	m.rejectedTotal.Inc()
	m.reviewLatency.Observe(latencySeconds)
}

// RecordDeferred records an anomaly deferred to human review with latency.
func (m *ReviewMetrics) RecordDeferred(latencySeconds float64) {
	if m == nil {
		return
	}
	m.deferredTotal.Inc()
	m.reviewLatency.Observe(latencySeconds)
}

// RecordFailed records a failed processing attempt with latency.
func (m *ReviewMetrics) RecordFailed(latencySeconds float64) {
	if m == nil {
		return
	}
	m.failedTotal.Inc()
	m.reviewLatency.Observe(latencySeconds)
}

// SetPendingCount sets the current pending anomaly count.
func (m *ReviewMetrics) SetPendingCount(count int) {
	if m == nil {
		return
	}
	m.pendingCount.Set(float64(count))
}

// IncWorkersActive increments the active workers count.
func (m *ReviewMetrics) IncWorkersActive() {
	if m == nil {
		return
	}
	m.workersActive.Inc()
}

// DecWorkersActive decrements the active workers count.
func (m *ReviewMetrics) DecWorkersActive() {
	if m == nil {
		return
	}
	m.workersActive.Dec()
}
