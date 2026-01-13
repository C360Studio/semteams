# ADR-005: Anomaly Approval Workflow

## Status

Proposed

## Context

SemStreams includes structural anomaly detection that identifies potential issues in the knowledge graph:

- **Semantic-Structural Gaps**: High semantic similarity but high structural distance
- **Core Isolation**: High k-core entities with few peer connections
- **Core Demotion**: Entities that dropped k-core level between runs
- **Transitivity Gaps**: Missing transitive relationships

The detection system is implemented and working. However, the **approval workflow** for acting on detected anomalies exists as code but is **not wired** into the runtime:

| Component | Status | Location |
|-----------|--------|----------|
| Anomaly Detection | Working | `graph/inference/core_anomaly.go` |
| ReviewWorker | Implemented, NOT started | `graph/inference/review_worker.go` |
| HTTP Handlers | Implemented, NOT registered | `graph/inference/http_handlers.go` |
| RelationshipApplier | Implemented | `graph/inference/applier.go` |
| ReviewConfig | Defined, NOT used | `graph/inference/config.go` |
| Suggestion Generation | Incomplete | Core anomalies lack suggestions |

## Decision

Wire existing components into the runtime with minimal new code:

### Integration Points

1. **graph-clustering Component**
   - When `Review.Enabled: true`, create and start `ReviewWorker` in `Component.Start()`
   - Pass configured `LLMClient` if available, `nil` otherwise
   - Stop worker in `Component.Stop()`

2. **graph-gateway Component**
   - Register `/inference/*` HTTP handlers in `RegisterHTTPHandlers()`
   - Handlers expose: list pending, get details, submit review

3. **Core Anomaly Detectors**
   - Populate `Suggestion` field with proposed relationship
   - Without suggestion, approval would have nothing to apply

### Operational Modes

| Mode | LLM Available | Behavior |
|------|---------------|----------|
| Disabled | N/A | No review workflow, anomalies stored only |
| Human-Only | No | All anomalies go to StatusHumanReview |
| LLM-Assisted | Yes | Auto-approve high confidence, LLM reviews medium, human reviews uncertain |

**Human-Only is a valid deployment mode.** The workflow should function without LLM integration.

### Configuration

```yaml
graph_clustering:
  review:
    enabled: false              # Opt-in (default disabled)
    workers: 2                  # Concurrent review workers
    auto_approve_threshold: 0.9 # Auto-approve >= 90% confidence
    auto_reject_threshold: 0.3  # Auto-reject <= 30% confidence
    fallback_to_human: true     # Escalate uncertain cases
    batch_size: 10              # Processing batch size
    review_timeout: 30s         # LLM call timeout
```

### Suggestion Generation for Core Anomalies

Currently, `CoreAnomalyDetector` creates anomalies with `StatusPending` but no `Suggestion`. The `ReviewWorker` requires a suggestion to apply approved anomalies.

For each anomaly type, define the suggested relationship:

| Anomaly Type | Suggested Predicate | Rationale |
|--------------|---------------------|-----------|
| CoreIsolation | `inference.suggested.peer` | Connect isolated entity to potential peers |
| CoreDemotion | `inference.suggested.support` | Strengthen connections to maintain core level |
| TransitivityGap | (existing triple) | Apply the missing transitive relationship |
| SemanticStructuralGap | `inference.suggested.related` | Connect semantically similar entities |

### HTTP API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/inference/anomalies/pending` | List anomalies awaiting human review |
| GET | `/inference/anomalies/{id}` | Get anomaly details |
| POST | `/inference/anomalies/{id}/review` | Submit human review decision |
| GET | `/inference/stats` | Review statistics |

## Consequences

### Positive

- **Leverages Existing Code**: Most functionality already implemented
- **Human-Only Mode Valid**: Works without LLM infrastructure
- **Progressive Enhancement**: Add LLM later without architecture change
- **Auditable**: All decisions logged with reviewer identity
- **Safe Defaults**: Disabled by default, explicit opt-in

### Negative

- **Suggestion Generation Needed**: Core anomaly detectors need updates
- **E2E Test Coverage**: Approval flow needs explicit testing
- **API Surface**: New HTTP endpoints to maintain

### Neutral

- **Schema Updates**: Component schema needs review config fields
- **Metrics**: Review metrics (approved/rejected/deferred) need registration

## Implementation Plan

### Phase 1: Suggestion Generation
- Update `CoreAnomalyDetector` to populate `Suggestion` field
- Add appropriate predicates for each anomaly type

### Phase 2: ReviewWorker Integration
- Add ReviewWorker instantiation in graph-clustering `Start()`
- Wire LLM client (optional)
- Add Stop() cleanup

### Phase 3: HTTP Handler Registration
- Register handlers in graph-gateway
- Add authentication/authorization if needed

### Phase 4: E2E Testing
- Add anomaly detection → review → approval test scenario
- Verify relationship application

## Key Files

| File | Change |
|------|--------|
| `graph/inference/core_anomaly.go` | Add suggestion generation |
| `processor/graph-clustering/component.go` | Start ReviewWorker |
| `gateway/graph-gateway/component.go` | Register HTTP handlers |
| `graph/inference/review_worker.go` | Already implemented |
| `graph/inference/http_handlers.go` | Already implemented |

## References

- [Community Detection Concepts](../concepts/05-community-detection.md)
- [Structural Analysis Concepts](../concepts/06-structural-analysis.md)
- [ADR-001: Pragmatic Semantic Web](./adr-001-pragmatic-semantic-web.md)
