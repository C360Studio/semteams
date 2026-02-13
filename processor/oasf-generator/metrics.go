package oasfgenerator

import (
	"sync"

	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the Prometheus metrics for the OASF generator.
type Metrics struct {
	recordsGenerated   prometheus.Counter
	recordsFailed      prometheus.Counter
	generationDuration prometheus.Histogram
	skillsGenerated    prometheus.Counter
	domainsGenerated   prometheus.Counter
	entityChanges      prometheus.Counter
	kvWatchErrors      prometheus.Counter
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce sync.Once
	metrics     *Metrics
)

// newMetrics creates new metrics for the OASF generator.
func newMetrics(registry *metric.MetricsRegistry) *Metrics {
	metricsOnce.Do(func() {
		metrics = &Metrics{
			recordsGenerated: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "records_generated_total",
				Help:      "Total number of OASF records successfully generated",
			}),
			recordsFailed: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "records_failed_total",
				Help:      "Total number of OASF record generation failures",
			}),
			generationDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "generation_duration_seconds",
				Help:      "Duration of OASF record generation in seconds",
				Buckets:   prometheus.DefBuckets,
			}),
			skillsGenerated: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "skills_generated_total",
				Help:      "Total number of skills generated across all OASF records",
			}),
			domainsGenerated: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "domains_generated_total",
				Help:      "Total number of domains generated across all OASF records",
			}),
			entityChanges: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "entity_changes_total",
				Help:      "Total number of entity changes processed",
			}),
			kvWatchErrors: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "oasf_generator",
				Name:      "kv_watch_errors_total",
				Help:      "Total number of KV watch errors",
			}),
		}

		// Register metrics with the registry if available
		if registry != nil {
			_ = registry.RegisterCounter("oasf-generator", "records_generated_total", metrics.recordsGenerated)
			_ = registry.RegisterCounter("oasf-generator", "records_failed_total", metrics.recordsFailed)
			_ = registry.RegisterHistogram("oasf-generator", "generation_duration_seconds", metrics.generationDuration)
			_ = registry.RegisterCounter("oasf-generator", "skills_generated_total", metrics.skillsGenerated)
			_ = registry.RegisterCounter("oasf-generator", "domains_generated_total", metrics.domainsGenerated)
			_ = registry.RegisterCounter("oasf-generator", "entity_changes_total", metrics.entityChanges)
			_ = registry.RegisterCounter("oasf-generator", "kv_watch_errors_total", metrics.kvWatchErrors)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metrics.recordsGenerated)
			_ = prometheus.DefaultRegisterer.Register(metrics.recordsFailed)
			_ = prometheus.DefaultRegisterer.Register(metrics.generationDuration)
			_ = prometheus.DefaultRegisterer.Register(metrics.skillsGenerated)
			_ = prometheus.DefaultRegisterer.Register(metrics.domainsGenerated)
			_ = prometheus.DefaultRegisterer.Register(metrics.entityChanges)
			_ = prometheus.DefaultRegisterer.Register(metrics.kvWatchErrors)
		}
	})
	return metrics
}

// RecordGenerated records a successful OASF record generation.
func (m *Metrics) RecordGenerated(skillCount, domainCount int, duration float64) {
	if m == nil {
		return
	}
	m.recordsGenerated.Inc()
	m.generationDuration.Observe(duration)
	m.skillsGenerated.Add(float64(skillCount))
	m.domainsGenerated.Add(float64(domainCount))
}

// RecordFailed records a failed OASF record generation.
func (m *Metrics) RecordFailed() {
	if m == nil {
		return
	}
	m.recordsFailed.Inc()
}

// EntityChanged records an entity change event.
func (m *Metrics) EntityChanged() {
	if m == nil {
		return
	}
	m.entityChanges.Inc()
}

// KVWatchError records a KV watch error.
func (m *Metrics) KVWatchError() {
	if m == nil {
		return
	}
	m.kvWatchErrors.Inc()
}
