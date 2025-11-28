# Roadmap: Graph Architecture Evolution

**Status:** Proposed
**Created:** 2025-11-27
**Related ADRs:**

- [ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md](./ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md)
- [ADR-TEMPORAL-GRAPH-MODELING.md](./ADR-TEMPORAL-GRAPH-MODELING.md)
- [TODO-GRAPH-INDEXING-ARCHITECTURE.md](./TODO-GRAPH-INDEXING-ARCHITECTURE.md)

---

## Executive Summary

Three architectural improvements have been proposed that together simplify SemStreams' core data model and enable powerful new capabilities. This roadmap unifies them into a single phased implementation plan.

**Core Insight:** These documents are deeply interconnected and should be implemented as a single unified effort with careful ordering to minimize disruption.

### Document Relationships

```text
TODO-GRAPH-INDEXING-ARCHITECTURE.md
├── Problem 1: Mutation API Inconsistency (Community Detection)
├── Problem 2: Index Management Inconsistency
└── Problem 3: Index Entry Provenance
        │
        └──► Feeds into ──►

ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md
├── Remove circular data conversion
├── Eliminate dual representation (Triples + Edges)
├── Add OUTGOING_INDEX
└── Simplify EntityState
        │
        └──► Enables ──►

ADR-TEMPORAL-GRAPH-MODELING.md
├── Stateful ECA rules (on_enter/on_exit)
├── Automatic relationship retraction
├── Triple.ExpiresAt field
└── (Post-MVP) Behavior Trees
```

### Key Dependencies

| Change | Depends On | Enables |
|--------|------------|---------|
| OUTGOING_INDEX | Nothing (additive) | Stateful rules, Edge removal |
| Triple.ExpiresAt | Nothing (additive) | TTL fallback, cleanup worker |
| Stateful ECA rules | OUTGOING_INDEX | Dynamic relationships |
| Community Detection alignment | OUTGOING_INDEX | Unified query model |
| Deprecate Edges | OUTGOING_INDEX + queries migrated | Simplified EntityState |
| Remove deprecated fields | All deprecations complete | Clean architecture |

---

## Phased Implementation Plan

### Phase 1: Foundation (Non-Breaking) - 3-4 days

**Goal:** Add new capabilities without changing existing behavior.

#### 1.1 Add OUTGOING_INDEX

Create symmetric index to INCOMING_INDEX for forward traversal.

```go
// Key: entityID
// Value: array of outgoing relationships
type OutgoingEntry struct {
    Predicate    string `json:"predicate"`
    ToEntityID   string `json:"to_entity_id"`
}
```

**Tasks:**

- [ ] Create OUTGOING_INDEX bucket in IndexManager
- [ ] Populate from relationship triples (same source as current Edges)
- [ ] Add `GetOutgoing(ctx, entityID)` query method
- [ ] Validate parity with existing Edge-based queries
- [ ] Add unit and integration tests

**Effort:** 1-2 days
**Breaking:** No

#### 1.2 Add Triple.ExpiresAt Field

Enable TTL-based triple expiration for dynamic relationships.

```go
type Triple struct {
    // ... existing fields ...
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
```

**Tasks:**

- [ ] Add ExpiresAt field to `message.Triple`
- [ ] Update Triple JSON serialization
- [ ] Add `RemoveExpiredTriples()` helper to EntityState
- [ ] Update relevant tests

**Effort:** 0.5 days
**Breaking:** No (new optional field)

---

### Phase 2: Stateful ECA Rules (Non-Breaking) - 3-4 days

**Goal:** Enable automatic relationship retraction when conditions change.

#### 2.1 RULE_STATE KV Bucket

```go
type RuleMatchState struct {
    RuleID         string    `json:"rule_id"`
    EntityKey      string    `json:"entity_key"`
    IsMatching     bool      `json:"is_matching"`
    LastTransition string    `json:"last_transition"` // "entered"|"exited"|""
    TransitionAt   time.Time `json:"transition_at,omitempty"`
    SourceRevision uint64    `json:"source_revision"`
    LastChecked    time.Time `json:"last_checked"`
}
```

#### 2.2 Enhanced Rule Schema

```yaml
rules:
  - id: "proximity-tracking"
    entity_patterns: ["*.*.robotics.*.drone.*"]
    related_patterns: ["*.*.robotics.*.drone.*"]
    condition: "distance(entity.position, related.position) < 100"

    on_enter:   # Fires ONCE when condition becomes true
      - type: "add_triple"
        predicate: "proximity.near"
        object: "$related.id"
        ttl: "5m"

    on_exit:    # Fires ONCE when condition becomes false
      - type: "remove_triple"
        predicate: "proximity.near"
        object: "$related.id"
```

**Tasks:**

- [ ] Create RULE_STATE KV bucket
- [ ] Implement StateTracker with Get/Set and LRU caching
- [ ] Add transition detection (`detectTransition()`)
- [ ] Enhance RuleEvaluator with `evaluateWithState()`
- [ ] Add on_enter/on_exit/while_true to rule schema
- [ ] Implement add_triple action (via mutation API)
- [ ] Implement remove_triple action
- [ ] Add expression functions: `hasTriple()`, `getOutgoing()`, `distance()`
- [ ] Integration tests for state transitions

**Effort:** 3-4 days
**Breaking:** No (additive)

#### 2.3 TTL Cleanup Worker

Background worker to remove expired triples.

```go
func (p *GraphProcessor) cleanupExpiredTriples(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    // ... scan and clean expired triples
}
```

**Tasks:**

- [ ] Implement cleanup worker in graph processor
- [ ] Add configuration for cleanup interval
- [ ] Add metrics for expired triple cleanup
- [ ] Integration tests

**Effort:** 1-2 days
**Breaking:** No

---

### Phase 3: Community Detection Alignment (Non-Breaking) - 2-3 days

**Goal:** Make community membership queryable as triples.

**Current Problem:** Community Detection writes directly to COMMUNITY_INDEX, not via mutation API. Community membership is invisible to PathRAG and standard triple queries.

**Solution:** Community Detection should create relationship triples via mutation API.

```go
triple := message.Triple{
    Subject:    entityID,
    Predicate:  "graph.community.member_of",
    Object:     communityID,
    Source:     "lpa_detection",
    Timestamp:  time.Now(),
    Confidence: communityScore,
}
```

**Tasks:**

- [ ] Add `create_triples` config option to community detection
- [ ] Modify LPADetector to publish relationship triples
- [ ] Keep COMMUNITY_INDEX for fast community lookups (dual-write)
- [ ] Add tests verifying community membership via triple queries
- [ ] Update PathRAG to traverse community relationships

**Effort:** 2-3 days
**Breaking:** No (config-controlled)

---

### Phase 4: Deprecate Legacy Fields (Non-Breaking) - 2-3 days

**Goal:** Prepare codebase for removal of redundant structures.

#### 4.1 Deprecate EntityState.Edges

```go
type EntityState struct {
    // Deprecated: Use OUTGOING_INDEX for traversal. Will be removed in v2.0.
    Edges []Edge `json:"edges,omitempty"`
    // ...
}
```

**Tasks:**

- [ ] Add deprecation warnings to Edges field access
- [ ] Update all query code to use OUTGOING_INDEX
- [ ] Add helper methods for transition period
- [ ] Update documentation

**Effort:** 1 day

#### 4.2 Deprecate Node.Properties

```go
type NodeProperties struct {
    // Deprecated: Access properties via GetTriple(predicate). Will be removed in v2.0.
    Properties map[string]any `json:"properties,omitempty"`
    // ...
}
```

**Tasks:**

- [ ] Add `GetTriple(predicate)` and `GetPropertyValue(predicate)` helpers
- [ ] Add deprecation warnings to Properties map access
- [ ] Update query code to use triple-based property access
- [ ] Update documentation

**Effort:** 1-2 days

---

### Phase 5: Remove Deprecated Fields (Breaking) - 2-3 days

**Goal:** Simplify EntityState to final form.

**Target Structure:**

```go
type EntityState struct {
    ID        string           // From EntityID() - 6-part federated format
    Type      string           // Derived from ID (5th part)
    Triples   []message.Triple // Single source of truth for ALL semantic facts
    ObjectRef string           // Reference to full message in ObjectStore
    Version   uint64           // For conflict resolution
    UpdatedAt time.Time
}
```

**Tasks:**

- [ ] Remove `Edges` field from EntityState
- [ ] Remove `Properties` from NodeProperties (or NodeProperties entirely)
- [ ] Remove `extractPropertiesAndRelationships()`
- [ ] Remove `buildEdgesFromRelationships()`
- [ ] Simplify MessageManager processor
- [ ] Migration script for existing data
- [ ] Update all tests

**Effort:** 2-3 days
**Breaking:** Yes - major version bump required

---

### Phase 6: Behavior Trees (Post-MVP) - 2-3 weeks

**Goal:** Enable hierarchical autonomous agent behaviors.

**Note:** This phase builds on Phase 2 (Stateful ECA) using the same state tracking abstraction.

**Components:**

| Component | Description | Effort |
|-----------|-------------|--------|
| BTNode interface | Base interface + status types | 1 day |
| Core nodes | Selector, Sequence, Parallel | 2-3 days |
| Decorator nodes | Inverter, Repeater, Timeout | 2 days |
| Blackboard | Shared state for tree execution | 1 day |
| Tree parser | YAML/JSON to BTNode tree | 2-3 days |
| Tick scheduler | Event-driven or periodic ticking | 2-3 days |
| Integration | Connect to entity watcher | 1-2 days |

**Breaking:** No (additive feature)

---

## Summary Timeline

| Phase | Duration | Breaking | Cumulative |
|-------|----------|----------|------------|
| Phase 1: Foundation | 3-4 days | No | 3-4 days |
| Phase 2: Stateful ECA | 3-4 days | No | 6-8 days |
| Phase 3: Community Alignment | 2-3 days | No | 8-11 days |
| Phase 4: Deprecations | 2-3 days | No | 10-14 days |
| Phase 5: Remove Fields | 2-3 days | **Yes** | 12-17 days |
| Phase 6: Behavior Trees | 2-3 weeks | No | Post-MVP |

**MVP Milestone:** After Phase 4 (10-14 days)

- OUTGOING_INDEX operational
- Stateful ECA rules working
- Community membership queryable as triples
- Legacy fields deprecated but still functional

**Clean Architecture Milestone:** After Phase 5 (12-17 days)

- Triples as single source of truth
- Simplified EntityState
- Breaking change release (v2.0)

---

## Risk Mitigation

### Phase 5 Breaking Changes

1. **Backward compatibility period:** Phases 1-4 provide migration time
2. **Deprecation warnings:** Give developers clear guidance
3. **Helper methods:** `GetTriple()`, `GetPropertyValue()` ease transition
4. **Migration scripts:** Automate data transformation
5. **Comprehensive testing:** Verify behavior parity before removal

### Performance Concerns

1. **OUTGOING_INDEX vs Stored Edges:** Benchmark query performance
2. **Triple iteration vs map lookup:** Profile property access patterns
3. **State tracker caching:** LRU cache for hot rule states

### Rollback Strategy

- Each phase is independently deployable
- Feature flags for new capabilities (stateful rules, community triples)
- NATS KV revision history enables point-in-time rollback

---

## Success Criteria

### Technical

- [ ] All existing tests pass after each phase
- [ ] Query performance within 10% of baseline
- [ ] OUTGOING_INDEX returns same results as Edge-based queries
- [ ] Stateful rules correctly detect transitions
- [ ] Community membership queryable via standard triple queries

### Developer Experience

- [ ] Single mental model: "Everything is a Triple"
- [ ] Clear migration documentation
- [ ] Helper methods for common access patterns
- [ ] Deprecation warnings with fix guidance

### Architecture

- [ ] No circular data conversions
- [ ] No dual representation of relationships
- [ ] Unified index management
- [ ] Clean EntityState structure

---

## Specification Recommendation

**Implement as a single unified spec** with phased milestones:

```text
specs/003-triples-architecture/
├── spec.md           # Combined requirements from all three ADRs
├── plan.md           # This roadmap (phases, dependencies, ordering)
├── tasks.md          # Tasks organized by phase
├── research.md       # Performance benchmarks, migration analysis
└── checklists/
    └── requirements.md
```

**Rationale:**

1. Documents are deeply interconnected (circular references)
2. Implementation order is critical for success
3. Single spec prevents scope creep and ensures coordination
4. Clear milestone checkpoints for validation

---

## References

- [ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md](./ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md) - Core data model simplification
- [ADR-TEMPORAL-GRAPH-MODELING.md](./ADR-TEMPORAL-GRAPH-MODELING.md) - Stateful rules and behavior trees
- [TODO-GRAPH-INDEXING-ARCHITECTURE.md](./TODO-GRAPH-INDEXING-ARCHITECTURE.md) - Index management issues
- `processor/graph/messagemanager/processor.go` - Current circular conversion
- `processor/graph/indexmanager/` - Current index implementations
- `processor/rule/` - Current Rules Engine
- `message/triple.go` - Triple structure
