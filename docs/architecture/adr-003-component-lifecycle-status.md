# ADR-003: Component Lifecycle Status Pattern

## Status

Accepted

## Context

Long-running async components in SemStreams (graph-clustering, graph-embedding) perform multi-phase processing cycles. For example, graph-clustering runs through:

1. Community Detection (LPA)
2. Structural Computation (k-core, pivots)
3. Anomaly Detection

Currently there's no standard way to observe:
- What stage a component is currently in
- When it last completed a cycle
- Whether the last cycle succeeded or failed

Prometheus metrics show counts and durations but not current state. This makes operational monitoring and debugging difficult, especially when cycles take significant time.

## Decision

Adopt a **lifecycle status pattern** using a dedicated `COMPONENT_STATUS` KV bucket where components publish their current processing stage.

### Data Model

**Key**: `COMPONENT_STATUS/<component-name>`

**Value**:
```json
{
  "component": "graph-clustering",
  "stage": "structural_computation",
  "cycle_id": "abc123",
  "cycle_started_at": "2024-01-15T10:30:00Z",
  "stage_started_at": "2024-01-15T10:30:05Z",
  "last_completed_at": "2024-01-15T10:29:30Z",
  "last_result": "success",
  "last_error": ""
}
```

### Stage Values

Components define their own stage names. Example for graph-clustering:

| Stage | Description |
|-------|-------------|
| `idle` | Waiting for trigger (timer or batch threshold) |
| `community_detection` | Running LPA algorithm |
| `structural_computation` | Computing k-core and pivot distances |
| `anomaly_detection` | Detecting anomalies within communities |
| `llm_enhancement` | Generating LLM summaries (if enabled) |

### Interface

```go
type LifecycleReporter interface {
    // ReportStage updates the component's current stage
    ReportStage(ctx context.Context, stage string) error

    // ReportCycleStart marks the beginning of a new processing cycle
    ReportCycleStart(ctx context.Context) error

    // ReportCycleComplete marks successful cycle completion
    ReportCycleComplete(ctx context.Context) error

    // ReportCycleError marks cycle failure with error details
    ReportCycleError(ctx context.Context, err error) error
}
```

## Consequences

### Positive

- **Real-time visibility** - Know what a component is doing NOW, not just aggregate metrics
- **Watchable** - External systems can watch `COMPONENT_STATUS.>` for coordination
- **Complements metrics** - State information that Prometheus counters/histograms can't capture
- **Debugging aid** - "Component stuck in stage X for 5 minutes" is actionable
- **Dashboard-friendly** - Easy to build operational status boards

### Negative

- **Additional KV writes** - One write per stage transition (typically 3-5 per cycle)
- **Implementation burden** - Components must call stage reporting methods
- **Consistency window** - Status may briefly lag actual state

### Neutral

- **Optional adoption** - Components can implement incrementally
- **No cross-component coordination** - This is observability, not orchestration

## Implementation Scope

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `component/lifecycle.go` | LifecycleReporter interface definition |
| `component/lifecycle_reporter.go` | Default implementation using KV |
| `natsclient/buckets.go` | Add COMPONENT_STATUS bucket creation |

### Adoption Path

1. Define interface in `component/lifecycle.go`
2. Implement default reporter in `component/lifecycle_reporter.go`
3. Add bucket creation to natsclient
4. Adopt in graph-clustering as proof-of-concept
5. Extend to graph-embedding and other async components

## Use Cases

### Operational Dashboard

```bash
# Watch all component status changes
nats kv watch COMPONENT_STATUS

# Query current status
nats kv get COMPONENT_STATUS graph-clustering
```

### Alerting

```yaml
# Alert if component stuck in non-idle stage for too long
alert: ComponentStuck
expr: time() - component_stage_started_at > 300
for: 1m
labels:
  severity: warning
```

### Coordination

External systems can watch for cycle completions:

```go
watcher, _ := kv.Watch("COMPONENT_STATUS.graph-clustering")
for entry := range watcher.Updates() {
    var status ComponentStatus
    json.Unmarshal(entry.Value(), &status)
    if status.Stage == "idle" && status.LastResult == "success" {
        // Trigger dependent processing
    }
}
```

## Related

- [ADR-002: Query Capability Discovery](./adr-002-query-capability-discovery.md) - Similar pattern for query discovery
- `processor/graph-clustering/` - Primary candidate for initial adoption
- Prometheus metrics - Complementary observability (numbers vs state)
