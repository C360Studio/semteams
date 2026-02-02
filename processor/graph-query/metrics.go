// Package graphquery provides Prometheus metrics for graph-query component.
package graphquery

import (
	"sync"

	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// queryMetrics holds Prometheus metrics for the graph-query component.
type queryMetrics struct {
	communityCacheHits     prometheus.Counter
	communityCacheMisses   prometheus.Counter
	communityStorageHits   prometheus.Counter
	communityStorageMisses prometheus.Counter
	communityLookups       *prometheus.CounterVec
	localSearchRequests    prometheus.Counter
	globalSearchRequests   prometheus.Counter
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce sync.Once
	metrics     *queryMetrics
)

// getMetrics returns the singleton metrics instance, creating and registering it if needed.
func getMetrics(registry *metric.MetricsRegistry) *queryMetrics {
	metricsOnce.Do(func() {
		metrics = &queryMetrics{
			communityCacheHits: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "community_cache_hits_total",
				Help:      "Total community cache hits (entity found in cache)",
			}),

			communityCacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "community_cache_misses_total",
				Help:      "Total community cache misses (entity not in cache, falling back to storage)",
			}),

			communityStorageHits: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "community_storage_hits_total",
				Help:      "Total community storage fallback hits (entity found via NATS query)",
			}),

			communityStorageMisses: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "community_storage_misses_total",
				Help:      "Total community storage fallback misses (entity not in any community)",
			}),

			communityLookups: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "community_lookups_total",
				Help:      "Total community lookups by result type",
			}, []string{"result"}), // result: cache_hit, storage_hit, not_found

			localSearchRequests: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "local_search_requests_total",
				Help:      "Total GraphRAG local search requests",
			}),

			globalSearchRequests: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "graph_query",
				Name:      "global_search_requests_total",
				Help:      "Total GraphRAG global search requests",
			}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounter("graph-query", "community_cache_hits_total", metrics.communityCacheHits)
			_ = registry.RegisterCounter("graph-query", "community_cache_misses_total", metrics.communityCacheMisses)
			_ = registry.RegisterCounter("graph-query", "community_storage_hits_total", metrics.communityStorageHits)
			_ = registry.RegisterCounter("graph-query", "community_storage_misses_total", metrics.communityStorageMisses)
			_ = registry.RegisterCounterVec("graph-query", "community_lookups_total", metrics.communityLookups)
			_ = registry.RegisterCounter("graph-query", "local_search_requests_total", metrics.localSearchRequests)
			_ = registry.RegisterCounter("graph-query", "global_search_requests_total", metrics.globalSearchRequests)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metrics.communityCacheHits)
			_ = prometheus.DefaultRegisterer.Register(metrics.communityCacheMisses)
			_ = prometheus.DefaultRegisterer.Register(metrics.communityStorageHits)
			_ = prometheus.DefaultRegisterer.Register(metrics.communityStorageMisses)
			_ = prometheus.DefaultRegisterer.Register(metrics.communityLookups)
			_ = prometheus.DefaultRegisterer.Register(metrics.localSearchRequests)
			_ = prometheus.DefaultRegisterer.Register(metrics.globalSearchRequests)
		}
	})
	return metrics
}

// recordCacheHit records a community cache hit.
func (m *queryMetrics) recordCacheHit() {
	m.communityCacheHits.Inc()
	m.communityLookups.WithLabelValues("cache_hit").Inc()
}

// recordCacheMiss records a community cache miss (triggering storage fallback).
func (m *queryMetrics) recordCacheMiss() {
	m.communityCacheMisses.Inc()
}

// recordStorageHit records a successful storage fallback lookup.
func (m *queryMetrics) recordStorageHit() {
	m.communityStorageHits.Inc()
	m.communityLookups.WithLabelValues("storage_hit").Inc()
}

// recordStorageMiss records a failed storage fallback (entity not in any community).
func (m *queryMetrics) recordStorageMiss() {
	m.communityStorageMisses.Inc()
	m.communityLookups.WithLabelValues("not_found").Inc()
}

// recordLocalSearch records a local search request.
func (m *queryMetrics) recordLocalSearch() {
	m.localSearchRequests.Inc()
}

// recordGlobalSearch records a global search request.
func (m *queryMetrics) recordGlobalSearch() {
	m.globalSearchRequests.Inc()
}
