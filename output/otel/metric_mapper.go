package otel

import (
	"sync"
	"time"
)

// MetricType represents the type of metric.
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// MetricData represents a metric ready for export.
type MetricData struct {
	// Name is the metric name.
	Name string `json:"name"`

	// Description describes the metric.
	Description string `json:"description,omitempty"`

	// Unit is the metric unit.
	Unit string `json:"unit,omitempty"`

	// Type is the metric type.
	Type MetricType `json:"type"`

	// DataPoints contains the metric values.
	DataPoints []DataPoint `json:"data_points"`

	// Attributes are metric-level attributes.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// DataPoint represents a single metric data point.
type DataPoint struct {
	// Timestamp is when the data point was recorded.
	Timestamp time.Time `json:"timestamp"`

	// Value is the metric value (for counter/gauge).
	Value float64 `json:"value,omitempty"`

	// Count is the count (for histogram/summary).
	Count uint64 `json:"count,omitempty"`

	// Sum is the sum (for histogram/summary).
	Sum float64 `json:"sum,omitempty"`

	// Buckets are histogram bucket counts.
	Buckets []BucketCount `json:"buckets,omitempty"`

	// Quantiles are summary quantile values.
	Quantiles []QuantileValue `json:"quantiles,omitempty"`

	// Attributes are data point attributes.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// BucketCount represents a histogram bucket.
type BucketCount struct {
	// UpperBound is the bucket upper bound.
	UpperBound float64 `json:"upper_bound"`

	// Count is the cumulative count.
	Count uint64 `json:"count"`
}

// QuantileValue represents a summary quantile.
type QuantileValue struct {
	// Quantile is the quantile (e.g., 0.5, 0.9, 0.99).
	Quantile float64 `json:"quantile"`

	// Value is the quantile value.
	Value float64 `json:"value"`
}

// MetricMapper maps internal metrics to OTEL format.
type MetricMapper struct {
	mu sync.RWMutex

	// Service information
	serviceName    string
	serviceVersion string

	// Metric registry
	metrics map[string]*MetricData

	// Counters
	metricsProcessed int64
	metricsExported  int64
}

// NewMetricMapper creates a new metric mapper.
func NewMetricMapper(serviceName, serviceVersion string) *MetricMapper {
	return &MetricMapper{
		serviceName:    serviceName,
		serviceVersion: serviceVersion,
		metrics:        make(map[string]*MetricData),
	}
}

// RecordCounter records a counter metric.
func (mm *MetricMapper) RecordCounter(name, description, unit string, value float64, attrs map[string]any) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	metric := mm.getOrCreateMetric(name, description, unit, MetricTypeCounter)
	metric.DataPoints = append(metric.DataPoints, DataPoint{
		Timestamp:  time.Now(),
		Value:      value,
		Attributes: attrs,
	})
	mm.metricsProcessed++
}

// RecordGauge records a gauge metric.
func (mm *MetricMapper) RecordGauge(name, description, unit string, value float64, attrs map[string]any) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	metric := mm.getOrCreateMetric(name, description, unit, MetricTypeGauge)
	metric.DataPoints = append(metric.DataPoints, DataPoint{
		Timestamp:  time.Now(),
		Value:      value,
		Attributes: attrs,
	})
	mm.metricsProcessed++
}

// RecordHistogram records a histogram metric.
func (mm *MetricMapper) RecordHistogram(name, description, unit string, count uint64, sum float64, buckets []BucketCount, attrs map[string]any) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	metric := mm.getOrCreateMetric(name, description, unit, MetricTypeHistogram)
	metric.DataPoints = append(metric.DataPoints, DataPoint{
		Timestamp:  time.Now(),
		Count:      count,
		Sum:        sum,
		Buckets:    buckets,
		Attributes: attrs,
	})
	mm.metricsProcessed++
}

// RecordSummary records a summary metric.
func (mm *MetricMapper) RecordSummary(name, description, unit string, count uint64, sum float64, quantiles []QuantileValue, attrs map[string]any) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	metric := mm.getOrCreateMetric(name, description, unit, MetricTypeSummary)
	metric.DataPoints = append(metric.DataPoints, DataPoint{
		Timestamp:  time.Now(),
		Count:      count,
		Sum:        sum,
		Quantiles:  quantiles,
		Attributes: attrs,
	})
	mm.metricsProcessed++
}

// getOrCreateMetric gets or creates a metric entry.
func (mm *MetricMapper) getOrCreateMetric(name, description, unit string, metricType MetricType) *MetricData {
	metric, ok := mm.metrics[name]
	if !ok {
		metric = &MetricData{
			Name:        name,
			Description: description,
			Unit:        unit,
			Type:        metricType,
			DataPoints:  make([]DataPoint, 0),
			Attributes: map[string]any{
				"service.name":    mm.serviceName,
				"service.version": mm.serviceVersion,
			},
		}
		mm.metrics[name] = metric
	}
	return metric
}

// FlushMetrics returns and clears all collected metrics.
func (mm *MetricMapper) FlushMetrics() []*MetricData {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Collect all metrics with data points
	metrics := make([]*MetricData, 0, len(mm.metrics))
	for name, metric := range mm.metrics {
		if len(metric.DataPoints) > 0 {
			// Make a copy for export
			metricCopy := *metric
			metricCopy.DataPoints = make([]DataPoint, len(metric.DataPoints))
			copy(metricCopy.DataPoints, metric.DataPoints)
			metrics = append(metrics, &metricCopy)

			// Clear data points
			mm.metrics[name].DataPoints = mm.metrics[name].DataPoints[:0]
			mm.metricsExported++
		}
	}

	return metrics
}

// RecordAgentMetrics records standard agent metrics.
func (mm *MetricMapper) RecordAgentMetrics(loopID, role string, stats map[string]int64) {
	attrs := map[string]any{
		"agent.loop_id": loopID,
		"agent.role":    role,
	}

	for name, value := range stats {
		mm.RecordGauge(
			"agent."+name,
			"Agent metric: "+name,
			"1",
			float64(value),
			attrs,
		)
	}
}

// Stats returns mapper statistics.
func (mm *MetricMapper) Stats() map[string]int64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return map[string]int64{
		"metrics_processed": mm.metricsProcessed,
		"metrics_exported":  mm.metricsExported,
		"metric_types":      int64(len(mm.metrics)),
	}
}

// MapPrometheusMetric maps a Prometheus-style metric to OTEL format.
type PrometheusMetric struct {
	Name   string
	Help   string
	Type   string
	Labels map[string]string
	Value  float64
	// For histograms
	Buckets map[float64]uint64
	Count   uint64
	Sum     float64
	// For summaries
	Quantiles map[float64]float64
}

// MapFromPrometheus converts Prometheus metrics to OTEL format.
func (mm *MetricMapper) MapFromPrometheus(prom *PrometheusMetric) {
	attrs := make(map[string]any, len(prom.Labels))
	for k, v := range prom.Labels {
		attrs[k] = v
	}

	switch prom.Type {
	case "counter":
		mm.RecordCounter(prom.Name, prom.Help, "1", prom.Value, attrs)
	case "gauge":
		mm.RecordGauge(prom.Name, prom.Help, "1", prom.Value, attrs)
	case "histogram":
		buckets := make([]BucketCount, 0, len(prom.Buckets))
		for bound, count := range prom.Buckets {
			buckets = append(buckets, BucketCount{
				UpperBound: bound,
				Count:      count,
			})
		}
		mm.RecordHistogram(prom.Name, prom.Help, "1", prom.Count, prom.Sum, buckets, attrs)
	case "summary":
		quantiles := make([]QuantileValue, 0, len(prom.Quantiles))
		for q, v := range prom.Quantiles {
			quantiles = append(quantiles, QuantileValue{
				Quantile: q,
				Value:    v,
			})
		}
		mm.RecordSummary(prom.Name, prom.Help, "1", prom.Count, prom.Sum, quantiles, attrs)
	default:
		// Default to gauge
		mm.RecordGauge(prom.Name, prom.Help, "1", prom.Value, attrs)
	}
}
