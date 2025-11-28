# Research: Triples Architecture Evolution

**Feature**: 003-triples-architecture
**Date**: 2025-11-27
**Status**: Complete

## Research Questions

### RQ-1: Existing Index Pattern Analysis

**Question**: How do existing indexes (PREDICATE_INDEX, INCOMING_INDEX) handle entity changes?

**Findings**:

From `processor/graph/indexmanager/indexes.go`:

```go
type Index interface {
    HandleCreate(ctx context.Context, entityID string, entityState interface{}) error
    HandleUpdate(ctx context.Context, entityID string, entityState interface{}) error
    HandleDelete(ctx context.Context, entityID string) error
}
```

- Each index implements the `Index` interface with Create/Update/Delete handlers
- Updates are handled by diffing old vs new state and updating index entries
- Key format varies by index type (e.g., `{predicate}:{value}` for PREDICATE_INDEX)
- INCOMING_INDEX is keyed by target entity ID, storing source entity references

**Decision**: OUTGOING_INDEX will follow the same pattern, keyed by source entity ID

---

### RQ-2: Current Triple Structure

**Question**: What fields does Triple currently have? What needs to be added?

**Findings**:

From `message/triple.go`:

```go
type Triple struct {
    Subject    string    `json:"subject"`
    Predicate  string    `json:"predicate"`
    Object     any       `json:"object"`
    Source     string    `json:"source"`
    Timestamp  time.Time `json:"timestamp"`
    Confidence float64   `json:"confidence"`
    Context    string    `json:"context,omitempty"`
    Datatype   string    `json:"datatype,omitempty"`
}
```

**Missing for this feature**: `ExpiresAt *time.Time` for TTL-based expiration

**Decision**: Add `ExpiresAt *time.Time` field with `json:"expires_at,omitempty"` tag

---

### RQ-3: Current EntityState Structure

**Question**: What is the current EntityState structure and what will change?

**Findings**:

From `graph/types.go`:

```go
type EntityState struct {
    Node        NodeProperties   `json:"node"`
    Edges       []Edge           `json:"edges"`        // To be deprecated/removed
    Triples     []message.Triple `json:"triples"`
    ObjectRef   string           `json:"object_ref"`
    MessageType string           `json:"message_type"`
    Version     uint64           `json:"version"`
    UpdatedAt   time.Time        `json:"updated_at"`
}

type NodeProperties struct {
    ID         string         `json:"id"`
    Type       string         `json:"type"`
    Properties map[string]any `json:"properties"` // To be deprecated/removed
    Position   *Position      `json:"position,omitempty"`
    Status     EntityStatus   `json:"status"`
}
```

**Dual representation problem confirmed**:

- `Edges` duplicates relationship triples
- `Properties` duplicates property triples
- Both are derived from `Triples` in `extractPropertiesAndRelationships()`

**Decision**:

- Phase 4: Add deprecation warnings, helper methods
- Phase 5: Remove Edges, Properties; simplify to target structure

---

### RQ-4: Current Rule Processor Architecture

**Question**: How does the current rule processor work? Where does state tracking fit?

**Findings**:

From `processor/rule/`:

- `config.go`: Rule definitions with `Definition` struct
- `processor.go`: Main processor that watches ENTITY_STATES
- `entity_watcher.go`: Watches KV for entity changes
- `expression/evaluator.go`: Evaluates rule conditions

Current rule schema supports:

- `entity_patterns`: Glob patterns for matching entities
- `condition`: Expression to evaluate
- `actions`: List of actions to execute when condition is true

**Missing for stateful rules**:

- `on_enter`: Actions when condition transitions false→true
- `on_exit`: Actions when condition transitions true→false
- `while_true`: Actions on every update while condition holds
- State tracking to detect transitions

**Decision**:

- Add `state_tracker.go` with RULE_STATE KV bucket
- Extend `Definition` struct with `OnEnter`, `OnExit`, `WhileTrue` fields
- Add transition detection in rule evaluation

---

### RQ-5: NATS KV Best Practices

**Question**: What are NATS KV best practices for the new buckets?

**Findings**:

From existing codebase patterns:

- Use `natsclient.KVStore` wrapper for consistent error handling
- Key sanitization via `sanitizeNATSKey()` for security
- CAS (Compare-And-Swap) operations for atomic updates
- History/revision tracking available but not currently used extensively

**For OUTGOING_INDEX**:

- Key: `{sourceEntityID}` (sanitized)
- Value: `[]OutgoingEntry` (JSON array)
- Update atomically with INCOMING_INDEX

**For RULE_STATE**:

- Key: `{ruleID}:{entityKey}` where entityKey is `{entityID}` or `{entityID1}:{entityID2}`
- Value: `RuleMatchState` struct
- Use revision for optimistic concurrency

**Decision**: Follow existing KV patterns, add bucket initialization to Manager.Start()

---

### RQ-6: Community Detection Integration

**Question**: How does community detection currently store results?

**Findings**:

From `pkg/graphclustering/storage.go`:

- `NATSCommunityStorage` manages COMMUNITY_INDEX directly
- Key format: `graph.community.{level}.{communityID}`
- Entity membership: `graph.community.entity.{level}.{entityID}`
- Does NOT use mutation API for graph writes

**Integration approach**:

1. Add `CreateTriples bool` config option
2. When enabled, also publish relationship triples via mutation API
3. Keep COMMUNITY_INDEX for fast community-specific queries (dual-write)

**Decision**: Implement dual-write with configurable triple creation

---

### RQ-7: Performance Baseline

**Question**: What is the current performance for Edge-based traversal?

**Findings**:

Current pattern (from QueryManager):

```go
// Get entity, iterate Edges
entity, _ := dm.GetEntityState(ctx, entityID)
for _, edge := range entity.Edges {
    // edge.ToEntityID, edge.EdgeType
}
```

Expected OUTGOING_INDEX pattern:

```go
// Query index directly
outgoing, _ := im.GetOutgoing(ctx, entityID)
for _, entry := range outgoing {
    // entry.ToEntityID, entry.Predicate
}
```

**Benchmark approach**:

- Create benchmark test with 100 entities, 10 relationships each
- Compare Edge iteration vs OUTGOING_INDEX query
- Target: within 10% of Edge-based performance

**Decision**: Add benchmark tests in Phase 1, validate before Phase 5

---

## Data Model Decisions

### OutgoingEntry Structure

```go
type OutgoingEntry struct {
    Predicate  string `json:"predicate"`
    ToEntityID string `json:"to_entity_id"`
}
```

Rationale: Minimal structure matching relationship triple data. No timestamp/confidence as those live on the source Triple.

### RuleMatchState Structure

```go
type RuleMatchState struct {
    RuleID         string    `json:"rule_id"`
    EntityKey      string    `json:"entity_key"`      // "entityID" or "entityID1:entityID2"
    IsMatching     bool      `json:"is_matching"`
    LastTransition string    `json:"last_transition"` // "entered"|"exited"|""
    TransitionAt   time.Time `json:"transition_at,omitempty"`
    SourceRevision uint64    `json:"source_revision"`
    LastChecked    time.Time `json:"last_checked"`
}
```

Rationale: Tracks current match state plus last transition for debugging/audit.

### Triple.ExpiresAt Addition

```go
type Triple struct {
    // ... existing fields ...
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
```

Rationale: Optional field, nil means no expiration. Pointer for omitempty JSON behavior.

---

## Alternatives Considered

### Alternative 1: Keep Edges, Add OUTGOING_INDEX

**Rejected**: Maintains dual representation problem. Complexity of keeping both in sync outweighs benefits.

### Alternative 2: In-Memory Rule State

**Rejected**: State lost on restart. Rules would re-fire on_enter after restart even if condition was already true.

### Alternative 3: Separate Community Membership Index Only

**Rejected**: PathRAG cannot traverse community relationships. Inconsistent query model.

### Alternative 4: Break Compatibility Immediately

**Rejected**: Existing deployments would break. Deprecation period allows migration.

---

---

## Update: 2025-11-28 - Index Synchronization Research

### RQ-8: INCOMING_INDEX Orphan Cleanup

**Question**: How should orphaned INCOMING_INDEX entries be cleaned up when an entity is deleted?

**Problem Identified**:
When entity A has a relationship to entity B, both indexes are updated:
- OUTGOING_INDEX[A] contains reference to B
- INCOMING_INDEX[B] contains reference from A

When A is deleted:
- OUTGOING_INDEX[A] is deleted ✅
- INCOMING_INDEX[A] is deleted ✅
- INCOMING_INDEX[B] still contains orphaned reference to A ❌

**Findings**:

From `processor/graph/indexmanager/indexes.go`:
- `IncomingIndex.RemoveIncomingReference(ctx, toEntityID, fromEntityID)` already exists
- `OutgoingIndex.GetOutgoing(ctx, entityID)` can retrieve targets before deletion
- `Manager.updateIndex()` handles delete events per index type

**Decision**: Implement cleanup in Manager.updateIndex() for outgoing index delete:
1. Read OUTGOING_INDEX for entity being deleted
2. For each target entity, call RemoveIncomingReference
3. Then delete OUTGOING_INDEX entry
4. INCOMING_INDEX delete happens separately (deletes entity's own incoming entry)

**Implementation Location**: `processor/graph/indexmanager/manager.go`

---

### RQ-9: Relationship Detection Method

**Question**: How should relationship triples be identified?

**Problem Identified**:
`processor/graph/datamanager/edge_ops.go` contains `isRelationshipPredicate()`:
```go
func isRelationshipPredicate(predicate string) bool {
    relationshipPredicates := map[string]bool{
        "POWERED_BY":   true,
        "NEAR":         true,
        // ... hardcoded list
    }
    return relationshipPredicates[predicate]
}
```

This approach:
- Misses new predicates registered in vocabulary
- Requires code changes for new relationship types
- Ignores the authoritative EntityID format

**Findings**:

From `message/triple.go`:
```go
func (t Triple) IsRelationship() bool {
    if str, ok := t.Object.(string); ok {
        return IsValidEntityID(str)
    }
    return false
}

func IsValidEntityID(s string) bool {
    // Validates 6-part format: org.platform.domain.system.type.instance
    parts := strings.Split(s, ".")
    return len(parts) == 6
}
```

This approach:
- Uses structural validation (6-part EntityID)
- Works with any predicate
- Aligns with vocabulary system design

**Decision**: Replace all `isRelationshipPredicate(predicate)` calls with `triple.IsRelationship()`.

**Files to Update**:
- `processor/graph/datamanager/edge_ops.go` - Remove isRelationshipPredicate function
- Any other files using hardcoded predicate checks

---

## References

- `processor/graph/indexmanager/indexes.go` - Existing index patterns
- `processor/graph/indexmanager/manager.go` - Index orchestration
- `message/triple.go` - Current Triple structure, IsRelationship() method
- `graph/types.go` - Current EntityState/Edge structures
- `processor/rule/config.go` - Current rule configuration
- `processor/graph/datamanager/edge_ops.go` - Relationship detection (to be fixed)
- `pkg/graphclustering/storage.go` - Community detection storage
- `docs/ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md` - Architectural decision record
- `docs/ADR-TEMPORAL-GRAPH-MODELING.md` - Stateful rules proposal
- `docs/TODO-GRAPH-INDEXING-ARCHITECTURE.md` - Index issues documentation
