# OpenTelemetry Export

OpenTelemetry (OTEL) integration enables observability for agent execution by exporting traces and metrics
to standard OTEL collectors. This makes agent behavior visible across distributed systems.

## What is OpenTelemetry?

OpenTelemetry is an observability framework that provides a vendor-neutral standard for collecting telemetry
data from applications. It defines how to instrument code, collect data, and export it to observability
backends like Jaeger, Prometheus, or cloud monitoring services.

### The Three Pillars of Observability

```text
┌───────────────────────────────────────────────────────────────┐
│                  OpenTelemetry Data Types                     │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                    │
│  │  TRACES  │  │ METRICS  │  │   LOGS   │                    │
│  ├──────────┤  ├──────────┤  ├──────────┤                    │
│  │ What      │  │ How much │  │ Context  │                    │
│  │ happened  │  │ happened │  │ and      │                    │
│  │ and when  │  │          │  │ details  │                    │
│  └──────────┘  └──────────┘  └──────────┘                    │
│                                                               │
│   Spans +       Counters +      Structured                    │
│   Context       Gauges +        Events                        │
│                 Histograms                                    │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

**Traces** capture the execution path through a system. Each operation is a span with start time, duration,
and relationships to other spans. This shows "what happened" and "in what order."

**Metrics** aggregate measurements over time. Counters track totals, gauges track instantaneous values,
histograms track distributions. This shows "how much happened" and "how it performed."

**Logs** provide detailed event records with structured context. This shows "exactly what was happening"
when something went wrong or when detailed forensics are needed.

SemStreams currently exports traces and metrics. Log export is planned for future releases.

## Why OTEL for Agent Systems?

Agent systems are complex distributed workflows. Traditional logging doesn't capture the execution flow
across multiple components, model calls, and tool executions. OTEL provides:

**End-to-end visibility**: See the complete agent execution from user request through model calls, tool
executions, and final response — all correlated in a single trace.

**Performance analysis**: Track latency for each phase (model calls, tool execution, context management)
and identify bottlenecks.

**Cost tracking**: Measure token usage per agent execution for accurate billing and optimization.

**Failure diagnosis**: When an agent fails, the trace shows exactly which step failed and what led to it.

**Multi-agent coordination**: When agents spawn sub-agents or coordinate work, traces maintain parent-child
relationships showing the complete orchestration.

## Traces and Spans Overview

A trace is the complete record of an agent execution. It consists of spans arranged in a hierarchy.

### Span Hierarchy

```text
┌──────────────────────────────────────────────────────────────┐
│ Trace: 7a3b4c9e8f2d1a5b                                      │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ agent.loop (root span)                                  │ │
│  │ SpanID: a1b2c3d4                                        │ │
│  │ Start: 10:30:00  Duration: 45s                          │ │
│  │ Attributes: loop_id, entity_id, role                    │ │
│  └─────┬──────────────────────────────────────────────────┘ │
│        │                                                     │
│        ├──► ┌────────────────────────────────────────────┐  │
│        │    │ agent.task                                  │  │
│        │    │ SpanID: e5f6g7h8  Parent: a1b2c3d4         │  │
│        │    │ Start: 10:30:02  Duration: 8s               │  │
│        │    │ Attributes: task_id, model                  │  │
│        │    └─────┬────────────────────────────────────┘  │
│        │          │                                        │
│        │          ├──► ┌─────────────────────────────┐    │
│        │          │    │ agent.tool.read_file        │    │
│        │          │    │ SpanID: i9j0k1l2            │    │
│        │          │    │ Duration: 25ms              │    │
│        │          │    └─────────────────────────────┘    │
│        │          │                                        │
│        │          └──► ┌─────────────────────────────┐    │
│        │               │ agent.tool.graph_query      │    │
│        │               │ SpanID: m3n4o5p6            │    │
│        │               │ Duration: 180ms             │    │
│        │               └─────────────────────────────┘    │
│        │                                                   │
│        └──► ┌────────────────────────────────────────────┐ │
│             │ agent.task                                  │ │
│             │ SpanID: q7r8s9t0  Parent: a1b2c3d4         │ │
│             │ Start: 10:30:12  Duration: 12s              │ │
│             └────────────────────────────────────────────┘ │
│                                                            │
└──────────────────────────────────────────────────────────────┘
```

### Span Structure

Each span contains:

| Field | Description | Example |
|-------|-------------|---------|
| **TraceID** | Unique identifier for entire trace | `7a3b4c9e8f2d1a5b` (32 hex chars) |
| **SpanID** | Unique identifier for this span | `a1b2c3d4` (16 hex chars) |
| **ParentSpanID** | SpanID of parent span | `e5f6g7h8` |
| **Name** | Operation name | `agent.loop`, `agent.tool.read_file` |
| **Kind** | Span type | `server`, `client`, `internal` |
| **StartTime** | When span started | `2024-01-15T10:30:00Z` |
| **EndTime** | When span ended | `2024-01-15T10:30:45Z` |
| **Status** | Outcome | `ok`, `error`, `unset` |
| **Attributes** | Key-value metadata | `loop_id=loop_123`, `role=architect` |
| **Events** | Timestamped events within span | State transitions, checkpoints |

## Agent Event to Span Mapping

SemStreams agent lifecycle events automatically convert to OTEL spans.

### Loop Events

```text
Agent Event: loop.created
────────────────────────────────────────
loop_id:    "loop_123"
entity_id:  "acme.ops.agent.general.001"
role:       "architect"
timestamp:  2024-01-15T10:30:00Z

Maps To:
────────────────────────────────────────
OTEL Span (root):
  trace_id:     7a3b4c9e8f2d1a5b  (derived from loop_id)
  span_id:      a1b2c3d4          (derived from loop_id)
  parent_id:    (none - root span)
  name:         "agent.loop"
  kind:         "server"
  start_time:   2024-01-15T10:30:00Z
  status:       "unset"
  attributes:
    agent.loop_id:    "loop_123"
    agent.entity_id:  "acme.ops.agent.general.001"
    agent.role:       "architect"
    service.name:     "semstreams"
```

```text
Agent Event: loop.completed
────────────────────────────────────────
loop_id:    "loop_123"
timestamp:  2024-01-15T10:30:45Z
duration:   45000ms

Updates Span:
────────────────────────────────────────
  end_time:   2024-01-15T10:30:45Z
  status:     "ok"
  attributes:
    agent.duration_ms: 45000
```

### Task Events

```text
Agent Event: task.started
────────────────────────────────────────
loop_id:    "loop_123"
task_id:    "task_456"
timestamp:  2024-01-15T10:30:02Z

Creates Child Span:
────────────────────────────────────────
OTEL Span:
  trace_id:     7a3b4c9e8f2d1a5b  (inherited from parent)
  span_id:      e5f6g7h8          (derived from loop_id:task_id)
  parent_id:    a1b2c3d4          (loop span)
  name:         "agent.task"
  kind:         "internal"
  start_time:   2024-01-15T10:30:02Z
  attributes:
    agent.loop_id: "loop_123"
    agent.task_id: "task_456"
```

### Tool Events

```text
Agent Event: tool.started
────────────────────────────────────────
loop_id:    "loop_123"
task_id:    "task_456"
tool_name:  "read_file"
timestamp:  2024-01-15T10:30:03Z

Creates Child Span:
────────────────────────────────────────
OTEL Span:
  trace_id:     7a3b4c9e8f2d1a5b
  span_id:      i9j0k1l2
  parent_id:    e5f6g7h8          (task span)
  name:         "agent.tool.read_file"
  kind:         "client"
  start_time:   2024-01-15T10:30:03Z
  attributes:
    agent.loop_id:  "loop_123"
    agent.task_id:  "task_456"
    tool.name:      "read_file"
```

### Event Mapping Summary

| Agent Event | OTEL Span | Relationship | Kind |
|-------------|-----------|--------------|------|
| `loop.created` | Root span start | Trace root | `server` |
| `loop.completed` | Root span end | Completes trace | - |
| `loop.failed` | Root span end with error | Marks failure | - |
| `task.started` | Child span start | Child of loop | `internal` |
| `task.completed` | Child span end | - | - |
| `task.failed` | Child span end with error | - | - |
| `tool.started` | Child span start | Child of task or loop | `client` |
| `tool.completed` | Child span end | - | - |
| `tool.failed` | Child span end with error | - | - |

## Span Hierarchy in Detail

The three-level hierarchy captures the natural structure of agent execution.

### Level 1: Loop Span (Root)

The loop span is the trace root, representing the entire agent execution lifecycle:

```text
agent.loop
├─ Start: When loop orchestrator begins
├─ End: When loop reaches terminal state
├─ Attributes: loop_id, entity_id, role, model
└─ Duration: Total execution time (all iterations)
```

**Use cases:**
- Track complete agent execution time
- Measure cost per agent invocation
- Correlate agent behavior with user requests
- Identify slow agent executions

### Level 2: Task Spans (Children of Loop)

Task spans represent individual iterations or steps within the loop:

```text
agent.task
├─ Parent: Loop span
├─ Start: When model receives context
├─ End: When model response processed
├─ Attributes: task_id, model, tokens_in, tokens_out
└─ Duration: Single iteration time
```

**Use cases:**
- Track iteration count and timing
- Measure token usage per iteration
- Identify expensive iterations
- Debug iteration loops

### Level 3: Tool Spans (Children of Task)

Tool spans represent individual tool executions:

```text
agent.tool.{name}
├─ Parent: Task span (or loop span if no task context)
├─ Start: When tool executor receives call
├─ End: When tool returns result or error
├─ Attributes: tool.name, tool parameters
└─ Duration: Tool execution time
```

**Use cases:**
- Measure tool performance
- Track tool usage patterns
- Identify slow tools
- Debug tool failures

### Parallel Tool Execution

When an agent requests multiple tools simultaneously, they appear as parallel spans:

```text
                ┌─────────────┐
                │ agent.task  │
                └──────┬──────┘
                       │
       ┌───────────────┼───────────────┐
       ▼               ▼               ▼
  ┌────────┐      ┌────────┐      ┌────────┐
  │tool A  │      │tool B  │      │tool C  │
  │25ms    │      │180ms   │      │40ms    │
  └────────┘      └────────┘      └────────┘

  Overlapping time ranges show concurrent execution.
```

The parent task span duration includes all tool execution time. Individual tool spans show their
specific execution time, enabling identification of the slowest tool in a parallel batch.

## Metric Collection and Mapping

Metrics provide aggregate views of agent performance over time.

### Agent Metrics

Standard agent metrics automatically tracked:

| Metric Name | Type | Description | Unit |
|-------------|------|-------------|------|
| `agent.loop.created` | Counter | Total loops started | `1` |
| `agent.loop.completed` | Counter | Loops completed successfully | `1` |
| `agent.loop.failed` | Counter | Loops that failed | `1` |
| `agent.loop.duration` | Histogram | Loop execution time distribution | `ms` |
| `agent.task.count` | Counter | Total tasks executed | `1` |
| `agent.task.duration` | Histogram | Task duration distribution | `ms` |
| `agent.tool.calls` | Counter | Total tool invocations | `1` |
| `agent.tool.duration` | Histogram | Tool execution time distribution | `ms` |
| `agent.tokens.input` | Counter | Total input tokens | `tokens` |
| `agent.tokens.output` | Counter | Total output tokens | `tokens` |
| `agent.iteration.count` | Histogram | Iterations per loop | `1` |

### Metric Types

**Counters** accumulate monotonically. Use for tracking totals:
- Loop completions
- Tool invocations
- Token consumption

**Gauges** track instantaneous values. Use for current state:
- Active loops
- Queue depth
- Memory usage

**Histograms** track value distributions with buckets. Use for latencies:
- Loop duration (buckets: 1s, 5s, 10s, 30s, 60s)
- Tool execution time (buckets: 10ms, 50ms, 100ms, 500ms, 1s)

**Summaries** track quantiles (p50, p90, p99). Use for percentile analysis:
- Response time percentiles
- Token usage percentiles

### Metric Attributes

Metrics include dimensional attributes for filtering and grouping:

```text
agent.loop.duration{
  agent.role="architect",
  agent.model="gpt-4",
  agent.outcome="success"
}
```

| Attribute | Purpose | Example Values |
|-----------|---------|----------------|
| `agent.role` | Agent type | `architect`, `editor`, `reviewer` |
| `agent.model` | LLM model used | `gpt-4`, `claude-3-5-sonnet` |
| `agent.outcome` | Execution result | `success`, `failed`, `cancelled` |
| `tool.name` | Tool identifier | `read_file`, `graph_query` |
| `service.name` | Service identifier | `semstreams`, `agent-system` |

These attributes enable queries like "average duration for architect agents using GPT-4" or
"p99 latency for graph_query tool."

## Integration with OTEL Collectors

The OTEL exporter sends data to standard OTEL collectors that route telemetry to backends.

### Architecture

```text
┌─────────────────────────────────────────────────────────────────┐
│                    SemStreams OTEL Export                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────────┐                                             │
│  │ agentic-loop   │ publishes                                   │
│  │ agentic-tools  │────────────┐                                │
│  │ agentic-model  │            │                                │
│  └────────────────┘            ▼                                │
│                         ┌─────────────┐                         │
│                         │ AGENT_EVENTS│                         │
│                         │  JetStream  │                         │
│                         └──────┬──────┘                         │
│                                │ subscribes                     │
│                                ▼                                │
│                         ┌─────────────┐                         │
│                         │ OTEL Export │                         │
│                         │  Component  │                         │
│                         ├─────────────┤                         │
│                         │ SpanCollect │ converts events         │
│                         │ MetricMapper│ to OTEL format          │
│                         └──────┬──────┘                         │
│                                │                                │
│                                ▼                                │
│                         ┌─────────────┐                         │
│                         │OTEL Protocol│                         │
│                         │ (gRPC/HTTP) │                         │
│                         └──────┬──────┘                         │
└────────────────────────────────┼──────────────────────────────┘
                                 │
                                 ▼
                         ┌─────────────┐
                         │    OTEL     │
                         │  Collector  │
                         ├─────────────┤
                         │ Receives    │
                         │ Processes   │
                         │ Routes      │
                         └──────┬──────┘
                                │
              ┌─────────────────┼─────────────────┐
              ▼                 ▼                 ▼
        ┌──────────┐      ┌──────────┐     ┌──────────┐
        │  Jaeger  │      │Prometheus│     │DataDog   │
        │ (traces) │      │(metrics) │     │(all)     │
        └──────────┘      └──────────┘     └──────────┘
```

### Collector Configuration

Example OTEL collector config for receiving SemStreams telemetry:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 10s
    send_batch_size: 512

  attributes:
    actions:
      - key: deployment.environment
        value: production
        action: insert

exporters:
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true

  prometheus:
    endpoint: 0.0.0.0:8889

  otlp/datadog:
    endpoint: api.datadoghq.com:443
    headers:
      DD-API-KEY: ${DD_API_KEY}

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, attributes]
      exporters: [jaeger, otlp/datadog]

    metrics:
      receivers: [otlp]
      processors: [batch, attributes]
      exporters: [prometheus, otlp/datadog]
```

### Protocol Options

SemStreams supports both gRPC and HTTP/protobuf:

| Protocol | Endpoint | Performance | Use Case |
|----------|----------|-------------|----------|
| **gRPC** | `localhost:4317` | Higher throughput, lower latency | Production deployments |
| **HTTP** | `localhost:4318` | Easier debugging, wider compatibility | Development, legacy systems |

## Sampling Strategies

High-volume agent systems generate significant telemetry. Sampling reduces overhead while maintaining
observability.

### Head Sampling (Configured in Exporter)

Decision made when trace starts. SemStreams exporter supports configurable sampling rate:

```json
{
  "sampling_rate": 0.1
}
```

This samples 10% of traces — every trace has a 10% chance of being collected. Simple and performant,
but can miss rare failures.

### Always Sample Strategy

```json
{
  "sampling_rate": 1.0
}
```

Collect every trace. Appropriate for:
- Development environments
- Low-volume production systems
- Initial debugging of issues

### Tail Sampling (Configured in Collector)

Decision made after trace completes. OTEL collector evaluates full traces and decides what to keep:

```yaml
processors:
  tail_sampling:
    policies:
      - name: error-traces
        type: status_code
        status_code: {status_codes: [ERROR]}

      - name: long-duration
        type: latency
        latency: {threshold_ms: 30000}

      - name: sample-normal
        type: probabilistic
        probabilistic: {sampling_percentage: 1}
```

This keeps:
- All traces with errors (100%)
- All traces over 30 seconds (100%)
- 1% of normal traces (sampled)

Tail sampling requires the collector to buffer traces, increasing memory usage but providing
intelligent sampling based on trace content.

### Sampling Recommendations

| Environment | Strategy | Rate | Rationale |
|-------------|----------|------|-----------|
| Development | Head sampling | 1.0 (100%) | Full visibility for debugging |
| Staging | Head sampling | 0.5 (50%) | High visibility, moderate cost |
| Production (low volume) | Head sampling | 1.0 (100%) | <1000 traces/min, affordable |
| Production (high volume) | Tail sampling | Error: 100%, Normal: 1% | Capture failures, sample successes |

## Correlation with Agent Execution

Traces maintain correlation across the agent execution lifecycle.

### Trace ID Generation

TraceIDs are deterministically generated from loop IDs:

```text
loop_id:  "loop_abc123xyz"
          ↓ hash function
trace_id: "7a3b4c9e8f2d1a5b" (32 hex characters)
```

This determinism means:
- Same loop ID always produces same trace ID
- Traces can be looked up by loop ID
- Multiple components exporting for same loop share trace ID

### Span ID Generation

SpanIDs are generated from span keys:

```text
Loop span:       hash("loop_abc123xyz") → a1b2c3d4
Task span:       hash("loop_abc123xyz:task_456") → e5f6g7h8
Tool span:       hash("loop_abc123xyz:tool:read_file") → i9j0k1l2
```

### Parent-Child Relationships

Spans automatically link to parents:

```text
agent.loop
  ├─ span_id: a1b2c3d4
  ├─ parent_id: (none)
  └─ trace_id: 7a3b4c9e8f2d1a5b

  agent.task
    ├─ span_id: e5f6g7h8
    ├─ parent_id: a1b2c3d4 (loop span)
    └─ trace_id: 7a3b4c9e8f2d1a5b (inherited)

    agent.tool.read_file
      ├─ span_id: i9j0k1l2
      ├─ parent_id: e5f6g7h8 (task span)
      └─ trace_id: 7a3b4c9e8f2d1a5b (inherited)
```

This linking enables:
- Trace visualization showing execution hierarchy
- Drill-down from loop → task → tool
- Root cause analysis following span relationships

### Correlation with NATS Messages

Agent events on NATS carry loop and task IDs that correlate with spans:

| NATS Subject | Message Field | OTEL Span Attribute |
|--------------|---------------|---------------------|
| `agent.task.*` | `loop_id` | `agent.loop_id` |
| `agent.task.*` | `task_id` | `agent.task_id` |
| `tool.execute.*` | `tool_name` | `tool.name` |
| `agent.complete.*` | `outcome` | `agent.outcome` |

This enables linking between NATS-based debugging and OTEL-based observability.

## Observability Best Practices

### Structured Attributes

Use consistent attribute naming and values for effective filtering:

```text
Good:
  agent.role = "architect"
  agent.model = "gpt-4"
  tool.name = "read_file"

Bad:
  role = "Architect Agent"    (inconsistent casing, extra words)
  model = "GPT-4-turbo-0125"  (version details unnecessary)
  tool = "readFile"           (inconsistent naming)
```

### Meaningful Span Names

Span names should be:
- **Consistent**: Same operation = same name
- **Hierarchical**: Use dots for structure (`agent.loop`, `agent.tool.read_file`)
- **Bounded cardinality**: Don't include IDs or values that create infinite variations

```text
Good:
  agent.loop
  agent.task
  agent.tool.read_file

Bad:
  loop_abc123xyz               (includes ID)
  task_for_user_42             (includes user)
  agent.tool.read_file./path   (includes variable path)
```

### Error Handling in Spans

When operations fail, capture error details:

```text
span.status = "error"
span.status.message = "Permission denied: /etc/passwd"
span.attributes["error.type"] = "permission_error"
span.attributes["error.file"] = "/etc/passwd"
```

This enables:
- Filtering by error type
- Grouping similar failures
- Understanding failure patterns

### Trace Context Propagation

When agents spawn sub-agents or call external services, propagate trace context:

```text
Parent Agent:
  trace_id: 7a3b4c9e8f2d1a5b
  span_id:  a1b2c3d4

  ↓ spawns child with context

Child Agent:
  trace_id: 7a3b4c9e8f2d1a5b  (inherited)
  span_id:  q7r8s9t0           (new)
  parent_id: a1b2c3d4           (links to parent)
```

This creates distributed traces across agent hierarchies.

### Metric Dimensionality

Balance between detail and cardinality:

```text
Good dimensions (bounded):
  agent.role (5-10 values)
  agent.model (3-5 values)
  tool.name (10-50 values)

Bad dimensions (unbounded):
  agent.loop_id (infinite)
  user.name (many thousands)
  file.path (infinite)
```

High-cardinality dimensions explode storage costs and slow queries. Use attributes for details,
dimensions for aggregation.

## Configuration Example

Example component configuration in a flow:

```json
{
  "components": [
    {
      "type": "output",
      "name": "otel-exporter",
      "config": {
        "endpoint": "localhost:4317",
        "protocol": "grpc",
        "service_name": "semstreams-agents",
        "service_version": "1.0.0",
        "export_traces": true,
        "export_metrics": true,
        "sampling_rate": 0.1,
        "batch_timeout": "5s",
        "export_timeout": "30s",
        "insecure": true,
        "resource_attributes": {
          "deployment.environment": "production",
          "deployment.region": "us-west-2"
        },
        "ports": {
          "inputs": [
            {
              "name": "agent_events",
              "subject": "agent.>",
              "type": "jetstream",
              "stream_name": "AGENT_EVENTS",
              "required": true,
              "description": "Agent lifecycle events"
            }
          ],
          "outputs": []
        }
      }
    }
  ]
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `endpoint` | string | `localhost:4317` | OTEL collector endpoint |
| `protocol` | string | `grpc` | Export protocol (`grpc` or `http`) |
| `service_name` | string | `semstreams` | Service name for traces |
| `service_version` | string | `1.0.0` | Service version |
| `export_traces` | bool | `true` | Enable trace export |
| `export_metrics` | bool | `true` | Enable metric export |
| `sampling_rate` | float | `1.0` | Trace sampling rate (0.0-1.0) |
| `batch_timeout` | string | `5s` | Batch export interval |
| `export_timeout` | string | `30s` | Export operation timeout |
| `insecure` | bool | `true` | Allow insecure connections |
| `resource_attributes` | object | `{}` | Additional resource attributes |

## Debugging OTEL Integration

### Verify Event Flow

Check that agent events are reaching the exporter:

```bash
# Subscribe to agent events
nats sub "agent.>"

# Trigger an agent execution
# Verify events appear: loop.created, task.started, tool.*, task.completed, loop.completed
```

### Inspect Span Collection

The exporter logs span collection activity:

```text
INFO OTEL exporter started endpoint=localhost:4317 export_traces=true
DEBUG Exported spans count=3
DEBUG Would export spans (no exporter configured) count=5
```

### Check Collector Connectivity

Verify the collector is reachable:

```bash
# Test gRPC endpoint
grpcurl -plaintext localhost:4317 list

# Test HTTP endpoint
curl http://localhost:4318/v1/traces
```

### Trace Visualization

View traces in Jaeger UI:

```text
1. Navigate to Jaeger UI (usually http://localhost:16686)
2. Select service "semstreams" from dropdown
3. Click "Find Traces"
4. Click on a trace to see the span hierarchy
```

Look for:
- Complete span hierarchy (loop → task → tool)
- Correct parent-child relationships
- Accurate timestamps and durations
- Expected attributes on spans

### Common Issues

| Symptom | Cause | Solution |
|---------|-------|----------|
| No traces in backend | Collector not receiving data | Check endpoint, protocol, network |
| Incomplete traces | Events not all published | Check agent event publication |
| Wrong service name | Config mismatch | Verify `service_name` config |
| High memory usage | No sampling | Enable sampling or tail sampling |
| Missing attributes | Event metadata incomplete | Check agent event payload structure |

## Integration with AGNTCY

The OTEL exporter is part of the AGNTCY integration architecture, enabling observability across
the Internet of Agents ecosystem.

```text
┌─────────────────────────────────────────────────────────────┐
│               AGNTCY Observability Stack                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  SemStreams Agent                                           │
│    ├─ Publishes: agent.* events to NATS                    │
│    ├─ Exports: OTEL traces/metrics                         │
│    └─ Correlates: loop_id → trace_id                       │
│                                                             │
│  SLIM Transport (optional)                                  │
│    └─ OTEL data can flow over SLIM for secure cross-org    │
│                                                             │
│  OTEL Collector                                             │
│    ├─ Receives: traces/metrics from multiple agents        │
│    ├─ Processes: sampling, filtering, enrichment           │
│    └─ Routes: to backend systems                           │
│                                                             │
│  Observability Backends                                     │
│    ├─ Jaeger: trace visualization                          │
│    ├─ Prometheus: metric storage and queries               │
│    └─ DataDog/NewRelic/etc: cloud observability            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

When participating in multi-organization agent workflows, OTEL provides visibility into the complete
execution path even when agents cross organizational boundaries. Trace context propagates through
SLIM messages, maintaining correlation across the distributed system.

## Performance Considerations

### Export Overhead

OTEL export adds minimal overhead when properly configured:

| Operation | Overhead | Impact |
|-----------|----------|--------|
| Event collection | <1ms | Negligible |
| Span creation | <100μs | Negligible |
| Batch export | 5-50ms | Batched, async |
| Network transmission | Variable | Depends on collector latency |

The exporter uses batching and async export to minimize impact on agent execution.

### Memory Usage

Span collector maintains active spans in memory until completion:

```text
Memory per span: ~500 bytes
Active spans: ~10-100 per agent loop
Peak memory: ~50KB per active loop
```

For systems with hundreds of concurrent loops, this is minimal compared to agent memory usage.

### Sampling for Scale

At high volume, sampling is essential:

| Traces/min | Sampling | Kept/min | Storage/day (est) |
|------------|----------|----------|-------------------|
| 1,000 | 100% | 1,000 | ~100MB |
| 10,000 | 10% | 1,000 | ~100MB |
| 100,000 | 1% | 1,000 | ~100MB |

Sampling maintains constant storage cost while scaling throughput.

## Related Documentation

- [ADR-019: AGNTCY Integration](../architecture/adr-019-agntcy-integration.md) — AGNTCY architecture decision
- [Agentic Systems](./11-agentic-systems.md) — Agent execution model
- [Orchestration Layers](./12-orchestration-layers.md) — How agents, rules, and workflows coordinate

## References

- [OpenTelemetry Specification](https://opentelemetry.io/docs/specs/)
- [OTEL Go SDK](https://pkg.go.dev/go.opentelemetry.io/otel)
- [OTEL Collector Configuration](https://opentelemetry.io/docs/collector/configuration/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Prometheus Documentation](https://prometheus.io/docs/)
