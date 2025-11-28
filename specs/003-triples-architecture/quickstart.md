# Quickstart: Triples Architecture Evolution

**Feature**: 003-triples-architecture
**Date**: 2025-11-27

## What's Changing

This feature simplifies SemStreams' data model by making **Triples the single source of truth** for entity data. The key changes are:

1. **OUTGOING_INDEX**: Query forward relationships without accessing EntityState.Edges
2. **Stateful Rules**: on_enter/on_exit actions for automatic relationship management
3. **TTL Triples**: Relationships can auto-expire
4. **Simplified EntityState**: Eventually removes redundant Edges and Properties

## Quick Examples

### Before: Edge-Based Traversal

```go
// Old way: Access edges from EntityState
entity, _ := dataManager.GetEntityState(ctx, "acme.telemetry.robotics.gcs1.drone.001")
for _, edge := range entity.Edges {
    fmt.Printf("→ %s (%s)\n", edge.ToEntityID, edge.EdgeType)
}
```

### After: Index-Based Traversal

```go
// New way: Query OUTGOING_INDEX directly
entries, _ := indexManager.GetOutgoing(ctx, "acme.telemetry.robotics.gcs1.drone.001")
for _, entry := range entries {
    fmt.Printf("→ %s (%s)\n", entry.ToEntityID, entry.Predicate)
}
```

### Stateful Rule Example

```yaml
# Old: Rule fires every time condition is true
rules:
  - id: "proximity-alert"
    entity_patterns: ["*.*.robotics.*.drone.*"]
    condition: "distance(entity.position, target.position) < 100"
    actions:
      - type: "publish"
        subject: "alerts.proximity"  # Fires repeatedly!

# New: Rule fires only on transitions
rules:
  - id: "proximity-tracking"
    entity_patterns: ["*.*.robotics.*.drone.*"]
    related_patterns: ["*.*.robotics.*.drone.*"]
    condition: "distance(entity.position, related.position) < 100"

    on_enter:  # Fires ONCE when condition becomes true
      - type: "add_triple"
        predicate: "proximity.near"
        object: "$related.id"
        ttl: "5m"

    on_exit:   # Fires ONCE when condition becomes false
      - type: "remove_triple"
        predicate: "proximity.near"
        object: "$related.id"
```

### TTL Triples

```go
// Create a triple that expires in 5 minutes
expiresAt := time.Now().Add(5 * time.Minute)
triple := message.Triple{
    Subject:    droneID,
    Predicate:  "proximity.near",
    Object:     otherDroneID,
    Source:     "rule:proximity-tracking",
    Timestamp:  time.Now(),
    Confidence: 1.0,
    ExpiresAt:  &expiresAt,  // NEW: Auto-expires
}
```

## Migration Path

### Phase 1-4: Non-Breaking (Current)

Your existing code continues to work. New capabilities are additive:

```go
// Old code still works
for _, edge := range entity.Edges { ... }

// New code available alongside
entries, _ := indexManager.GetOutgoing(ctx, entityID)
```

### Phase 4: Deprecation Warnings

```go
// This will log a deprecation warning:
// "DEPRECATED: Access edges via GetOutgoing() instead of entity.Edges"
for _, edge := range entity.Edges { ... }

// Use the new helper methods:
value := entity.GetPropertyValue("robotics.battery.level")
triple := entity.GetTriple("robotics.battery.level")
```

### Phase 5: Breaking Change (v2.0)

```go
// entity.Edges is removed - this won't compile
// entity.Node.Properties is removed - this won't compile

// Use index queries instead
entries, _ := indexManager.GetOutgoing(ctx, entityID)

// Use triple helpers
value := entity.GetPropertyValue("robotics.battery.level")
```

## Validation Scenarios

### Scenario 1: OUTGOING_INDEX Query Parity

```bash
# Verify OUTGOING_INDEX returns same results as Edge iteration
go test -v ./processor/graph/indexmanager/... -run TestOutgoingIndexParity
```

Expected: All relationship data accessible via both methods during transition period.

### Scenario 2: Stateful Rule Transitions

```bash
# Verify on_enter fires exactly once
go test -v ./processor/rule/... -run TestStatefulRuleEnter

# Verify on_exit fires exactly once
go test -v ./processor/rule/... -run TestStatefulRuleExit

# Verify no duplicate fires on repeated updates
go test -v ./processor/rule/... -run TestStatefulRuleNoDuplicates
```

Expected:

- on_enter: Fires once when condition transitions false→true
- on_exit: Fires once when condition transitions true→false
- No duplicate fires on repeated updates while condition holds

### Scenario 3: TTL Triple Cleanup

```bash
# Verify expired triples are cleaned up
go test -v ./processor/graph/... -run TestExpiredTripleCleanup
```

Expected: Triples with ExpiresAt in the past are removed within 60 seconds.

### Scenario 4: Community Triples (when enabled)

```bash
# Verify community membership creates relationship triples
INTEGRATION_TESTS=1 go test -v ./pkg/graphclustering/... -run TestCommunityTriples
```

Expected: With `create_triples: true`, community membership is queryable via standard triple queries.

## Configuration

### Enable Stateful Rules

No additional configuration needed. Existing rules continue to work. To use stateful features, add `on_enter`/`on_exit` to rule definitions.

### Enable Community Triples

```json
{
  "community_detection": {
    "enabled": true,
    "create_triples": true,
    "triple_predicate": "graph.community.member_of"
  }
}
```

### TTL Cleanup Worker

Enabled by default. Configure interval:

```json
{
  "graph_processor": {
    "cleanup_interval": "30s",
    "cleanup_batch_size": 100
  }
}
```

## Performance Notes

- OUTGOING_INDEX queries should be within 10% of Edge iteration performance
- State tracker uses LRU cache for hot rule states
- TTL cleanup runs in background, minimal impact on throughput

## Troubleshooting

### "Entity has no outgoing relationships"

```go
entries, err := indexManager.GetOutgoing(ctx, entityID)
if errors.Is(err, indexmanager.ErrNotFound) {
    // Entity exists but has no relationship triples
    // This is normal for entities with only property triples
}
```

### "Rule state not found"

First evaluation for a rule/entity pair. State will be created after first evaluation.

### Deprecation warnings in logs

Update code to use new APIs before Phase 5 (breaking change):

```go
// Replace entity.Edges with:
entries, _ := indexManager.GetOutgoing(ctx, entityID)

// Replace entity.Node.Properties["key"] with:
value := entity.GetPropertyValue("key")
```

## Next Steps

1. Review data-model.md for detailed structure changes
2. Review contracts/ for API specifications
3. Run validation scenarios to verify functionality
4. Update code using deprecated APIs before Phase 5
