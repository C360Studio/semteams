// Package graphindex provides Prometheus metrics for graph-index component.
package graphindex

import (
	"sync"

	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// indexMetrics holds Prometheus metrics for the graph-index component.
// Includes backward-compatible indexengine_* metrics for E2E test compatibility.
type indexMetrics struct {
	// Backward-compatible metrics (indexengine_* prefix for E2E tests)
	eventsProcessed prometheus.Counter
	indexUpdates    *prometheus.CounterVec

	// Component-specific metrics
	kvOperations *prometheus.CounterVec
	watchEvents  *prometheus.CounterVec
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce sync.Once
	metrics     *indexMetrics
)

// getMetrics returns the singleton metrics instance, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *indexMetrics {
	metricsOnce.Do(func() {
		metrics = &indexMetrics{
			// Backward-compatible: indexengine_events_processed_total
			// This metric is checked by E2E test executeValidateMetrics
			eventsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "indexengine",
				Name:      "events_processed_total",
				Help:      "Total events processed by index engine",
			}),

			// Backward-compatible: indexengine_index_updates_total
			// This metric is checked by E2E test executeValidateMetrics
			indexUpdates: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "indexengine",
				Name:      "index_updates_total",
				Help:      "Total index update operations by index type",
			}, []string{"index_type"}),

			// Component-specific: semstreams_graph_index_kv_operations_total
			kvOperations: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_index",
				Name:      "kv_operations_total",
				Help:      "Total KV bucket operations",
			}, []string{"operation", "bucket"}),

			// Component-specific: semstreams_graph_index_watch_events_total
			watchEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_index",
				Name:      "watch_events_total",
				Help:      "Total watch events received",
			}, []string{"event_type"}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			// Backward-compatible metrics
			_ = registry.RegisterCounter("graph-index", "events_processed_total", metrics.eventsProcessed)
			_ = registry.RegisterCounterVec("graph-index", "index_updates_total", metrics.indexUpdates)

			// Component-specific metrics
			_ = registry.RegisterCounterVec("graph-index", "kv_operations_total", metrics.kvOperations)
			_ = registry.RegisterCounterVec("graph-index", "watch_events_total", metrics.watchEvents)
		} else {
			// Fallback to default prometheus registry for testing
			// Ignore errors if already registered (can happen across tests)
			_ = prometheus.DefaultRegisterer.Register(metrics.eventsProcessed)
			_ = prometheus.DefaultRegisterer.Register(metrics.indexUpdates)
			_ = prometheus.DefaultRegisterer.Register(metrics.kvOperations)
			_ = prometheus.DefaultRegisterer.Register(metrics.watchEvents)
		}
	})
	return metrics
}

// recordEventProcessed increments the events processed counter.
func (m *indexMetrics) recordEventProcessed() {
	m.eventsProcessed.Inc()
}

// recordIndexUpdate increments the index update counter for the given index type.
func (m *indexMetrics) recordIndexUpdate(indexType string) {
	m.indexUpdates.WithLabelValues(indexType).Inc()
}

// recordKVOperation records a KV operation for the given operation type and bucket.
func (m *indexMetrics) recordKVOperation(operation, bucket string) {
	m.kvOperations.WithLabelValues(operation, bucket).Inc()
}

// recordWatchEvent records a watch event of the given type.
func (m *indexMetrics) recordWatchEvent(eventType string) {
	m.watchEvents.WithLabelValues(eventType).Inc()
}
