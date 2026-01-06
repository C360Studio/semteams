package clustering

import (
	"github.com/c360/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// EnhancementMetrics provides Prometheus metrics for LLM community enhancement
type EnhancementMetrics struct {
	// Timing
	enhancementLatency prometheus.Histogram // Per-request LLM latency

	// Counters
	requestsTotal   prometheus.Counter // Total enhancement attempts
	requestsSuccess prometheus.Counter // Successful enhancements
	requestsFailed  prometheus.Counter // Failed enhancements

	// Gauges
	queueDepth    prometheus.Gauge // Pending communities awaiting enhancement
	workersActive prometheus.Gauge // Currently processing workers
}

// NewEnhancementMetrics creates a new EnhancementMetrics instance using MetricsRegistry
func NewEnhancementMetrics(component string, registry *metric.MetricsRegistry) *EnhancementMetrics {
	if registry == nil {
		return nil
	}

	// LLM enhancement latency - buckets tuned for LLM inference (typically 1-60s)
	enhancementLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "semstreams_llm_enhancement_latency_seconds",
		Help:        "Latency of LLM community enhancement requests in seconds",
		ConstLabels: prometheus.Labels{"component": component},
		Buckets:     []float64{0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0},
	})
	registry.RegisterHistogram("clustering", "llm_enhancement_latency_seconds", enhancementLatency)

	requestsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "semstreams_llm_enhancement_requests_total",
		Help:        "Total number of LLM enhancement requests attempted",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter("clustering", "llm_enhancement_requests_total", requestsTotal)

	requestsSuccess := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "semstreams_llm_enhancement_success_total",
		Help:        "Total number of successful LLM enhancements",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter("clustering", "llm_enhancement_success_total", requestsSuccess)

	requestsFailed := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "semstreams_llm_enhancement_failed_total",
		Help:        "Total number of failed LLM enhancements",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterCounter("clustering", "llm_enhancement_failed_total", requestsFailed)

	queueDepth := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "semstreams_llm_enhancement_queue_depth",
		Help:        "Current number of communities queued for LLM enhancement",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterGauge("clustering", "llm_enhancement_queue_depth", queueDepth)

	workersActive := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "semstreams_llm_enhancement_workers_active",
		Help:        "Number of enhancement workers currently processing",
		ConstLabels: prometheus.Labels{"component": component},
	})
	registry.RegisterGauge("clustering", "llm_enhancement_workers_active", workersActive)

	return &EnhancementMetrics{
		enhancementLatency: enhancementLatency,
		requestsTotal:      requestsTotal,
		requestsSuccess:    requestsSuccess,
		requestsFailed:     requestsFailed,
		queueDepth:         queueDepth,
		workersActive:      workersActive,
	}
}

// RecordEnhancementStart records the start of an enhancement attempt
func (m *EnhancementMetrics) RecordEnhancementStart() {
	if m == nil {
		return
	}
	m.requestsTotal.Inc()
	m.workersActive.Inc()
}

// RecordEnhancementSuccess records a successful enhancement with latency
func (m *EnhancementMetrics) RecordEnhancementSuccess(latencySeconds float64) {
	if m == nil {
		return
	}
	m.requestsSuccess.Inc()
	m.enhancementLatency.Observe(latencySeconds)
	m.workersActive.Dec()
}

// RecordEnhancementFailed records a failed enhancement with latency
func (m *EnhancementMetrics) RecordEnhancementFailed(latencySeconds float64) {
	if m == nil {
		return
	}
	m.requestsFailed.Inc()
	m.enhancementLatency.Observe(latencySeconds)
	m.workersActive.Dec()
}

// SetQueueDepth sets the current queue depth
func (m *EnhancementMetrics) SetQueueDepth(depth int) {
	if m == nil {
		return
	}
	m.queueDepth.Set(float64(depth))
}

// IncQueueDepth increments the queue depth by 1
func (m *EnhancementMetrics) IncQueueDepth() {
	if m == nil {
		return
	}
	m.queueDepth.Inc()
}

// DecQueueDepth decrements the queue depth by 1
func (m *EnhancementMetrics) DecQueueDepth() {
	if m == nil {
		return
	}
	m.queueDepth.Dec()
}
