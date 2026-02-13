// Package otel provides an OpenTelemetry exporter for SemStreams agent telemetry.
//
// The OTEL exporter subscribes to agent lifecycle events from NATS JetStream
// and converts them to OpenTelemetry spans and metrics for export to OTEL collectors.
//
// # Architecture
//
// The exporter follows the AGNTCY integration pattern for observability:
//
//	                                 ┌─────────────────────┐
//	Agent Events                     │   OTEL Exporter     │
//	─────────────────────────────────┤                     │
//	agent.loop.created ─────────────►│  SpanCollector     │──────► OTEL Collector
//	agent.loop.completed ───────────►│                     │
//	agent.task.* ───────────────────►│  MetricMapper      │──────► (Traces + Metrics)
//	agent.tool.* ───────────────────►│                     │
//	                                 └─────────────────────┘
//
// # Span Collection
//
// The SpanCollector converts agent lifecycle events to OTEL spans:
//
//   - loop.created → Root span start
//   - loop.completed/failed → Root span end
//   - task.started/completed/failed → Child span for task
//   - tool.started/completed/failed → Child span for tool execution
//
// Spans are automatically linked via trace ID derived from the loop ID,
// creating a complete trace hierarchy for each agent execution.
//
// # Metric Mapping
//
// The MetricMapper converts internal metrics to OTEL format:
//
//   - Counters (cumulative values)
//   - Gauges (instantaneous values)
//   - Histograms (distribution buckets)
//   - Summaries (quantile values)
//
// # Configuration
//
// Key configuration options:
//
//	{
//	  "endpoint": "localhost:4317",      // OTEL collector endpoint
//	  "protocol": "grpc",                // "grpc" or "http"
//	  "service_name": "semstreams",      // Service name for traces
//	  "export_traces": true,             // Enable trace export
//	  "export_metrics": true,            // Enable metric export
//	  "batch_timeout": "5s",             // Batch export interval
//	  "sampling_rate": 1.0               // Trace sampling rate (0.0-1.0)
//	}
//
// # NATS Subjects
//
// The exporter subscribes to agent events from JetStream:
//
//	| Stream         | Subject   | Purpose                    |
//	|----------------|-----------|----------------------------|
//	| AGENT_EVENTS   | agent.>   | All agent lifecycle events |
//
// # Integration with OTEL SDK
//
// This package provides the data collection and transformation layer.
// For actual export to OTEL collectors, integrate with the OTEL Go SDK:
//
//	import (
//	    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
//	    "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
//	)
//
// The Exporter interface allows plugging in real OTEL exporters or using
// the stub implementation for testing.
//
// # Example Flow Configuration
//
//	components:
//	  - type: output
//	    name: otel-exporter
//	    config:
//	      endpoint: "localhost:4317"
//	      protocol: "grpc"
//	      service_name: "my-agent-system"
//	      export_traces: true
//	      export_metrics: true
//	      sampling_rate: 0.1  # Sample 10% of traces
//
// # Trace Correlation
//
// Traces are correlated using the agent loop ID as the trace seed:
//
//	TraceID = hash(loop_id)  → 32-character hex
//	SpanID = hash(span_key)  → 16-character hex
//
// This ensures consistent trace IDs across distributed agent executions
// and allows correlating spans from multiple components.
//
// # References
//
//   - ADR-019: AGNTCY Integration
//   - OpenTelemetry Specification: https://opentelemetry.io/docs/specs/
//   - OTEL Go SDK: https://pkg.go.dev/go.opentelemetry.io/otel
package otel
