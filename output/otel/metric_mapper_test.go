package otel

import (
	"testing"
)

func TestNewMetricMapper(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	if mm == nil {
		t.Fatal("expected metric mapper, got nil")
	}

	if mm.serviceName != "test-service" {
		t.Errorf("expected service name 'test-service', got %q", mm.serviceName)
	}

	if mm.serviceVersion != "1.0.0" {
		t.Errorf("expected service version '1.0.0', got %q", mm.serviceVersion)
	}
}

func TestMetricMapperRecordCounter(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	attrs := map[string]any{"agent": "test-agent"}
	mm.RecordCounter("requests_total", "Total requests", "1", 100, attrs)

	stats := mm.Stats()
	if stats["metrics_processed"] != 1 {
		t.Errorf("expected 1 metric processed, got %d", stats["metrics_processed"])
	}

	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "requests_total" {
		t.Errorf("expected name 'requests_total', got %q", metric.Name)
	}
	if metric.Type != MetricTypeCounter {
		t.Errorf("expected type counter, got %v", metric.Type)
	}
	if len(metric.DataPoints) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(metric.DataPoints))
	}
	if metric.DataPoints[0].Value != 100 {
		t.Errorf("expected value 100, got %f", metric.DataPoints[0].Value)
	}
}

func TestMetricMapperRecordGauge(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	attrs := map[string]any{"host": "localhost"}
	mm.RecordGauge("memory_usage", "Memory usage", "bytes", 1024*1024, attrs)

	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "memory_usage" {
		t.Errorf("expected name 'memory_usage', got %q", metric.Name)
	}
	if metric.Type != MetricTypeGauge {
		t.Errorf("expected type gauge, got %v", metric.Type)
	}
	if metric.Unit != "bytes" {
		t.Errorf("expected unit 'bytes', got %q", metric.Unit)
	}
}

func TestMetricMapperRecordHistogram(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	buckets := []BucketCount{
		{UpperBound: 0.1, Count: 10},
		{UpperBound: 0.5, Count: 50},
		{UpperBound: 1.0, Count: 90},
		{UpperBound: 5.0, Count: 100},
	}
	attrs := map[string]any{"endpoint": "/api/tasks"}
	mm.RecordHistogram("request_duration", "Request duration", "seconds", 100, 25.5, buckets, attrs)

	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "request_duration" {
		t.Errorf("expected name 'request_duration', got %q", metric.Name)
	}
	if metric.Type != MetricTypeHistogram {
		t.Errorf("expected type histogram, got %v", metric.Type)
	}

	dp := metric.DataPoints[0]
	if dp.Count != 100 {
		t.Errorf("expected count 100, got %d", dp.Count)
	}
	if dp.Sum != 25.5 {
		t.Errorf("expected sum 25.5, got %f", dp.Sum)
	}
	if len(dp.Buckets) != 4 {
		t.Errorf("expected 4 buckets, got %d", len(dp.Buckets))
	}
}

func TestMetricMapperRecordSummary(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	quantiles := []QuantileValue{
		{Quantile: 0.5, Value: 0.1},
		{Quantile: 0.9, Value: 0.5},
		{Quantile: 0.99, Value: 1.0},
	}
	attrs := map[string]any{"method": "POST"}
	mm.RecordSummary("response_time", "Response time", "seconds", 1000, 150.0, quantiles, attrs)

	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "response_time" {
		t.Errorf("expected name 'response_time', got %q", metric.Name)
	}
	if metric.Type != MetricTypeSummary {
		t.Errorf("expected type summary, got %v", metric.Type)
	}

	dp := metric.DataPoints[0]
	if dp.Count != 1000 {
		t.Errorf("expected count 1000, got %d", dp.Count)
	}
	if len(dp.Quantiles) != 3 {
		t.Errorf("expected 3 quantiles, got %d", len(dp.Quantiles))
	}
}

func TestMetricMapperMultipleDataPoints(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	// Record multiple values for same metric
	for i := 0; i < 5; i++ {
		mm.RecordGauge("cpu_usage", "CPU usage", "percent", float64(i*10), nil)
	}

	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if len(metric.DataPoints) != 5 {
		t.Errorf("expected 5 data points, got %d", len(metric.DataPoints))
	}
}

func TestMetricMapperFlushClearsDataPoints(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	mm.RecordCounter("test_counter", "Test counter", "1", 100, nil)

	// First flush
	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	// Second flush should be empty
	metrics = mm.FlushMetrics()
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics after second flush, got %d", len(metrics))
	}

	// Record another value
	mm.RecordCounter("test_counter", "Test counter", "1", 200, nil)

	// Third flush should have new data
	metrics = mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if len(metrics[0].DataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(metrics[0].DataPoints))
	}
	if metrics[0].DataPoints[0].Value != 200 {
		t.Errorf("expected value 200, got %f", metrics[0].DataPoints[0].Value)
	}
}

func TestMetricMapperRecordAgentMetrics(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	stats := map[string]int64{
		"loops_completed": 10,
		"tasks_executed":  50,
		"tools_invoked":   100,
	}
	mm.RecordAgentMetrics("loop-001", "architect", stats)

	metrics := mm.FlushMetrics()
	if len(metrics) != 3 {
		t.Errorf("expected 3 metrics, got %d", len(metrics))
	}

	// Verify metric names
	names := make(map[string]bool)
	for _, m := range metrics {
		names[m.Name] = true
	}

	expectedNames := []string{"agent.loops_completed", "agent.tasks_executed", "agent.tools_invoked"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected metric %q not found", name)
		}
	}
}

func TestMetricMapperMapFromPrometheus(t *testing.T) {
	tests := []struct {
		name       string
		prom       *PrometheusMetric
		wantType   MetricType
		wantValue  float64
		wantCount  uint64
		wantSum    float64
		checkValue bool
	}{
		{
			name: "counter",
			prom: &PrometheusMetric{
				Name:   "http_requests_total",
				Help:   "Total HTTP requests",
				Type:   "counter",
				Labels: map[string]string{"method": "GET"},
				Value:  1000,
			},
			wantType:   MetricTypeCounter,
			wantValue:  1000,
			checkValue: true,
		},
		{
			name: "gauge",
			prom: &PrometheusMetric{
				Name:   "active_connections",
				Help:   "Active connections",
				Type:   "gauge",
				Labels: map[string]string{"server": "web1"},
				Value:  42,
			},
			wantType:   MetricTypeGauge,
			wantValue:  42,
			checkValue: true,
		},
		{
			name: "histogram",
			prom: &PrometheusMetric{
				Name: "request_duration_seconds",
				Help: "Request duration",
				Type: "histogram",
				Buckets: map[float64]uint64{
					0.1: 10,
					0.5: 50,
					1.0: 100,
				},
				Count: 100,
				Sum:   25.5,
			},
			wantType:  MetricTypeHistogram,
			wantCount: 100,
			wantSum:   25.5,
		},
		{
			name: "summary",
			prom: &PrometheusMetric{
				Name: "response_latency_seconds",
				Help: "Response latency",
				Type: "summary",
				Quantiles: map[float64]float64{
					0.5:  0.1,
					0.99: 0.5,
				},
				Count: 500,
				Sum:   50.0,
			},
			wantType:  MetricTypeSummary,
			wantCount: 500,
			wantSum:   50.0,
		},
		{
			name: "unknown defaults to gauge",
			prom: &PrometheusMetric{
				Name:  "unknown_metric",
				Help:  "Unknown metric type",
				Type:  "unknown",
				Value: 99,
			},
			wantType:   MetricTypeGauge,
			wantValue:  99,
			checkValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mm := NewMetricMapper("test-service", "1.0.0")
			mm.MapFromPrometheus(tt.prom)

			metrics := mm.FlushMetrics()
			if len(metrics) != 1 {
				t.Fatalf("expected 1 metric, got %d", len(metrics))
			}

			metric := metrics[0]
			if metric.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, metric.Type)
			}

			if len(metric.DataPoints) != 1 {
				t.Fatalf("expected 1 data point, got %d", len(metric.DataPoints))
			}

			dp := metric.DataPoints[0]
			if tt.checkValue && dp.Value != tt.wantValue {
				t.Errorf("expected value %f, got %f", tt.wantValue, dp.Value)
			}
			if tt.wantCount > 0 && dp.Count != tt.wantCount {
				t.Errorf("expected count %d, got %d", tt.wantCount, dp.Count)
			}
			if tt.wantSum > 0 && dp.Sum != tt.wantSum {
				t.Errorf("expected sum %f, got %f", tt.wantSum, dp.Sum)
			}
		})
	}
}

func TestMetricMapperStats(t *testing.T) {
	mm := NewMetricMapper("test-service", "1.0.0")

	mm.RecordCounter("counter1", "", "1", 1, nil)
	mm.RecordGauge("gauge1", "", "1", 1, nil)
	mm.RecordCounter("counter2", "", "1", 1, nil)

	stats := mm.Stats()

	if stats["metrics_processed"] != 3 {
		t.Errorf("expected 3 metrics processed, got %d", stats["metrics_processed"])
	}

	// 2 unique metric types (counter1, counter2 are same type)
	if stats["metric_types"] != 3 {
		t.Errorf("expected 3 metric types, got %d", stats["metric_types"])
	}

	// Flush and check export count
	_ = mm.FlushMetrics()
	stats = mm.Stats()
	if stats["metrics_exported"] != 3 {
		t.Errorf("expected 3 metrics exported, got %d", stats["metrics_exported"])
	}
}

func TestMetricMapperServiceAttributes(t *testing.T) {
	mm := NewMetricMapper("my-service", "2.0.0")

	mm.RecordGauge("test_metric", "Test", "1", 1, nil)

	metrics := mm.FlushMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Attributes["service.name"] != "my-service" {
		t.Errorf("expected service.name 'my-service', got %v", metric.Attributes["service.name"])
	}
	if metric.Attributes["service.version"] != "2.0.0" {
		t.Errorf("expected service.version '2.0.0', got %v", metric.Attributes["service.version"])
	}
}
