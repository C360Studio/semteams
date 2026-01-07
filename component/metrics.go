// Package component provides base types and utilities for SemStreams components.
package component

import (
	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// ProcessorMetrics provides standard Prometheus metrics for processor components.
// Each processor component should create its own instance with a unique subsystem name.
type ProcessorMetrics struct {
	// EventsProcessed counts total events processed, labeled by operation type
	EventsProcessed *prometheus.CounterVec

	// EventsErrors counts total processing errors, labeled by error type
	EventsErrors *prometheus.CounterVec

	// KVOperations counts KV bucket operations, labeled by operation (get/put/delete/watch)
	KVOperations *prometheus.CounterVec

	// ProcessingDuration measures processing latency in seconds
	ProcessingDuration prometheus.Histogram

	// subsystem is stored for registration key generation
	subsystem string
}

// NewProcessorMetrics creates and registers processor metrics with the given subsystem name.
// The subsystem name should be the component name with underscores (e.g., "graph_ingest").
// If registry is nil, metrics are created but not registered (useful for testing).
func NewProcessorMetrics(registry *metric.MetricsRegistry, subsystem string) *ProcessorMetrics {
	m := &ProcessorMetrics{
		subsystem: subsystem,
		EventsProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: subsystem,
				Name:      "events_processed_total",
				Help:      "Total events processed by component",
			},
			[]string{"operation"},
		),
		EventsErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: subsystem,
				Name:      "events_errors_total",
				Help:      "Total processing errors by component",
			},
			[]string{"error_type"},
		),
		KVOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: subsystem,
				Name:      "kv_operations_total",
				Help:      "Total KV bucket operations",
			},
			[]string{"operation"},
		),
		ProcessingDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: subsystem,
				Name:      "processing_duration_seconds",
				Help:      "Processing duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
		),
	}

	// Register with the metrics registry if available
	if registry != nil {
		// Use component name as service name for registration key
		_ = registry.RegisterCounterVec(subsystem, "events_processed_total", m.EventsProcessed)
		_ = registry.RegisterCounterVec(subsystem, "events_errors_total", m.EventsErrors)
		_ = registry.RegisterCounterVec(subsystem, "kv_operations_total", m.KVOperations)
		_ = registry.RegisterHistogram(subsystem, "processing_duration_seconds", m.ProcessingDuration)
	}

	return m
}

// RecordEvent increments the events processed counter for the given operation
func (m *ProcessorMetrics) RecordEvent(operation string) {
	m.EventsProcessed.WithLabelValues(operation).Inc()
}

// RecordError increments the error counter for the given error type
func (m *ProcessorMetrics) RecordError(errorType string) {
	m.EventsErrors.WithLabelValues(errorType).Inc()
}

// RecordKVOperation increments the KV operations counter
func (m *ProcessorMetrics) RecordKVOperation(operation string) {
	m.KVOperations.WithLabelValues(operation).Inc()
}

// ObserveDuration records a processing duration
func (m *ProcessorMetrics) ObserveDuration(seconds float64) {
	m.ProcessingDuration.Observe(seconds)
}
