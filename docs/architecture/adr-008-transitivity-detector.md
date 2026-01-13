# ADR-008: Transitivity Detector Wiring

## Status

Proposed

## Context

The transitivity gap detector identifies missing edges implied by transitive predicates. For example, if:
- Alice `member_of` Engineering
- Engineering `part_of` Company

Then Alice should have a short path to Company. If the actual graph distance is unexpectedly high, this suggests a missing relationship.

The detector code exists at `graph/inference/transitivity.go` with full implementation:
- `TransitivityDetector` struct
- Configuration for transitive predicates (`member_of`, `part_of`, `located_in`, `belongs_to`)
- Path length analysis logic

However, the detector is **intentionally skipped at runtime** due to a missing dependency:

```go
// processor/graph-clustering/anomaly.go:132-135
if cfg.Transitivity.Enabled {
    c.logger.Warn("transitivity detector enabled but RelationshipQuerier not yet wired - skipping")
}
```

The detector requires a `RelationshipQuerier` interface to traverse relationship chains, which is not currently passed to `initAnomalyDetection()`.

## Decision

Wire the `RelationshipQuerier` dependency to enable the transitivity detector:

1. Create or adapt a `RelationshipQuerier` implementation using the existing graph provider
2. Pass the querier to `initAnomalyDetection()` in the component startup
3. Register the transitivity detector with the anomaly orchestrator
4. Remove the skip warning

## Implementation

### 1. Adapt GraphProvider to RelationshipQuerier

The `graphProviderAdapter` in `anomaly.go` already wraps the provider. Extend it to satisfy `RelationshipQuerier`:

```go
type graphProviderAdapter struct {
    provider graph.Provider
}

func (a *graphProviderAdapter) GetRelationships(entityID string, predicates []string) ([]Relationship, error) {
    // Use provider.GetOutgoing/GetIncoming filtered by predicates
}
```

### 2. Register Transitivity Detector

```go
if cfg.Transitivity.Enabled {
    querier := &graphProviderAdapter{provider: c.graphProvider}
    transitivityDetector := inference.NewTransitivityDetector(cfg.Transitivity, querier)
    c.anomalyOrchestrator.RegisterDetector(transitivityDetector)
}
```

### 3. Update Configuration

Ensure transitivity config is properly exposed:
- `transitivity.enabled` (default: false)
- `transitivity.transitive_predicates` (default: ["member_of", "part_of", "located_in", "belongs_to"])
- `transitivity.max_intermediate_hops` (default: 2)

## Consequences

### Benefits

- Completes the anomaly detection suite (4/4 detectors active)
- Detects missing edges in hierarchical relationships
- Useful for organizational graphs, document hierarchies, spatial containment

### Costs

- Additional traversal queries during anomaly detection
- May produce false positives in graphs without clear transitive relationships
- Requires careful predicate configuration per domain

### Neutral

- Only runs when explicitly enabled
- Respects existing anomaly detection limits (max_anomalies_per_run, detection_timeout)

## Alternatives Considered

1. **Keep disabled**: Current state. Simple but incomplete feature set.

2. **Inline in existing detector**: Could merge transitivity logic into semantic-gap detector. Rejected because they detect fundamentally different patterns (semantic similarity vs structural transitivity).

3. **Separate process**: Run transitivity detection outside the clustering cycle. Rejected because it benefits from the same structural indices computed during clustering.

## References

- `graph/inference/transitivity.go` - Detector implementation
- `processor/graph-clustering/anomaly.go:132-135` - Current skip logic
- [Anomaly Detection Concepts](../concepts/06-anomaly-detection.md) - User-facing documentation
