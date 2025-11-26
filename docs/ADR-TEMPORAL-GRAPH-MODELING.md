# ADR: Temporal Graph Modeling and Stateful Rules

**Status:** Proposed
**Priority:** High
**Related:** ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md, TODO-GRAPH-INDEXING-ARCHITECTURE.md

---

## Problem Statement

The current Rules Engine is **stateless and event-triggered**. It evaluates conditions per entity update without tracking state transitions. This creates problems for dynamic relationships that should be invalidated when conditions change.

### Example: Proximity Relationships

```
T1: drone.001 position update → distance to drone.002 = 80m
    Rule: "distance < 100m" evaluates TRUE → Creates relationship triple ✓

T2: drone.001 position update → distance to drone.002 = 120m
    Rule: "distance < 100m" evaluates FALSE → Nothing happens
    Relationship triple persists incorrectly!
```

The rule only fires when the condition is TRUE. There's no mechanism to retract the triple when the condition becomes FALSE.

---

## Strategic Context

### Alignment with ADR-TRIPLES-AS-SOURCE-OF-TRUTH

This ADR builds on the proposal to:
1. **Remove `Edges` field from EntityState** - Relationships stored as triples only
2. **Add OUTGOING_INDEX** - Replaces stored edges for traversal
3. **Triples as single source of truth** - All mutations via triples

**Implications for this ADR:**
- Rule actions create/remove **relationship triples**, not edges
- Traversal queries use **OUTGOING_INDEX** and **INCOMING_INDEX**
- TTL/expiration via **Triple.ExpiresAt** field (proposed)
- Cleanup worker scans **triples**, not edges

### Two-Phase Roadmap

| Phase | Capability | Use Case |
|-------|------------|----------|
| **MVP** | Stateful ECA Rules | Graph consistency, dynamic relationships |
| **Post-MVP** | Behavior Trees | Autonomous agent behavior, mission execution |

The MVP stateful rules are a **foundation** for behavior trees - same state tracking abstraction, simpler structure.

---

## Requirements

### MVP (Graph Consistency)
1. **Entry/Exit Detection**: Distinguish "condition became true" vs "condition became false"
2. **Automatic Retraction**: Invalidate relationship triples when conditions no longer hold
3. **Immediate Effect**: No stale window (or minimal TTL fallback)
4. **Incremental Enhancement**: Not a refactor - add to existing Rules Engine

### Post-MVP (Autonomous Agents)
5. **Hierarchical Behaviors**: Selector/Sequence/Decorator patterns
6. **Long-Running Tasks**: RUNNING state with interruption
7. **Tick-Based Evaluation**: Continuous or event-driven

---

## MVP Solution: Stateful ECA Rules

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Rules Engine                            │
│  ┌─────────────┐    ┌──────────────┐    ┌────────────────┐ │
│  │ Entity      │ → │ Rule         │ → │ Action         │ │
│  │ Watcher     │    │ Evaluator    │    │ Executor       │ │
│  └─────────────┘    └──────────────┘    └────────────────┘ │
│                            ↓↑                               │
│                  ┌──────────────────┐  ← NEW COMPONENT     │
│                  │ State Tracker    │                       │
│                  │ (RULE_STATE KV)  │                       │
│                  └──────────────────┘                       │
│                            ↓                                │
│                  ┌──────────────────┐                       │
│                  │ Mutation API     │ → ENTITY_STATES      │
│                  │ (triples)        │ → OUTGOING_INDEX     │
│                  └──────────────────┘ → INCOMING_INDEX     │
└─────────────────────────────────────────────────────────────┘
```

### New KV Bucket: RULE_STATE

```go
// Bucket: RULE_STATE
// Key format: {rule_id}:{entity_key}
//   - Single entity: {rule_id}:{entity_id}
//   - Entity pair:   {rule_id}:{entity1}:{entity2}

type RuleMatchState struct {
    RuleID         string    `json:"rule_id"`
    EntityKey      string    `json:"entity_key"`      // "e1" or "e1:e2"
    IsMatching     bool      `json:"is_matching"`
    LastTransition string    `json:"last_transition"` // "entered"|"exited"|""
    TransitionAt   time.Time `json:"transition_at,omitempty"`
    SourceRevision uint64    `json:"source_revision"` // Entity revision that caused state
    LastChecked    time.Time `json:"last_checked"`
}
```

### Enhanced Rule Schema

```yaml
# Existing ECA syntax (unchanged, still works)
rules:
  - id: "low-battery-alert"
    entity_patterns: ["*.*.robotics.*.drone.*"]
    condition: "entity.triples['robotics.battery.level'] < 20"
    actions:
      - type: "publish"
        subject: "alerts.battery.low"

# NEW: Stateful rules with transition actions
rules:
  - id: "proximity-tracking"
    entity_patterns: ["*.*.robotics.*.drone.*"]
    related_patterns: ["*.*.robotics.*.drone.*"]  # For pair rules
    condition: |
      distance(entity.position, related.position) < 100

    on_enter:   # Fires ONCE when condition becomes true
      - type: "add_triple"
        predicate: "proximity.near"
        object: "$related.id"
        ttl: "5m"  # Optional fallback TTL

    on_exit:    # Fires ONCE when condition becomes false
      - type: "remove_triple"
        predicate: "proximity.near"
        object: "$related.id"

    while_true: # Optional: fires on EVERY update while condition holds
      - type: "update_triple"
        predicate: "proximity.near"
        object: "$related.id"
        properties:
          distance: "$calculated_distance"
```

### State Comparison Logic

```go
// In rule evaluator - ~50 lines addition
func (e *RuleEvaluator) evaluateWithState(
    ctx context.Context,
    rule Rule,
    entity EntityState,
    related *EntityState,  // nil for single-entity rules
) error {
    // 1. Evaluate condition (existing code)
    matches, err := e.evaluateCondition(rule.Condition, entity, related)
    if err != nil {
        return err
    }

    // 2. Build entity key
    entityKey := entity.Node.ID
    if related != nil {
        entityKey = fmt.Sprintf("%s:%s", entity.Node.ID, related.Node.ID)
    }

    // 3. Get previous state (NEW)
    prevState, _ := e.stateTracker.Get(ctx, rule.ID, entityKey)

    // 4. Detect transition (NEW)
    transition := e.detectTransition(prevState.IsMatching, matches)

    // 5. Execute appropriate actions (ENHANCED)
    switch transition {
    case TransitionEntered:
        if err := e.executeActions(ctx, rule.OnEnter, entity, related); err != nil {
            return err
        }
    case TransitionExited:
        if err := e.executeActions(ctx, rule.OnExit, entity, related); err != nil {
            return err
        }
    case TransitionNone:
        if matches && len(rule.WhileTrue) > 0 {
            if err := e.executeActions(ctx, rule.WhileTrue, entity, related); err != nil {
                return err
            }
        }
    }

    // 6. Update state (NEW)
    return e.stateTracker.Set(ctx, rule.ID, entityKey, RuleMatchState{
        RuleID:         rule.ID,
        EntityKey:      entityKey,
        IsMatching:     matches,
        LastTransition: string(transition),
        TransitionAt:   time.Now(),
        SourceRevision: entity.Version,
        LastChecked:    time.Now(),
    })
}

type Transition string

const (
    TransitionNone    Transition = ""
    TransitionEntered Transition = "entered"
    TransitionExited  Transition = "exited"
)

func (e *RuleEvaluator) detectTransition(wasMatching, nowMatching bool) Transition {
    if !wasMatching && nowMatching {
        return TransitionEntered
    }
    if wasMatching && !nowMatching {
        return TransitionExited
    }
    return TransitionNone
}
```

### New Action Types (Aligned with Triples-as-Source-of-Truth)

```go
// processor/rule/actions.go

func (e *ActionExecutor) executeAddTriple(ctx context.Context, action Action, entity, related EntityState) error {
    triple := message.Triple{
        Subject:    entity.Node.ID,
        Predicate:  action.Predicate,
        Object:     e.resolveObject(action.Object, related),  // "$related.id" → actual ID
        Source:     fmt.Sprintf("rule:%s", action.RuleID),
        Timestamp:  time.Now(),
        Confidence: action.Confidence,  // Default 1.0
    }

    // Optional TTL for fallback expiration
    if action.TTL != "" {
        duration, _ := time.ParseDuration(action.TTL)
        expiresAt := time.Now().Add(duration)
        triple.ExpiresAt = &expiresAt
    }

    return e.mutationClient.AddTriple(ctx, triple)
}

func (e *ActionExecutor) executeRemoveTriple(ctx context.Context, action Action, entity, related EntityState) error {
    req := RemoveTripleRequest{
        Subject:   entity.Node.ID,
        Predicate: action.Predicate,
        Object:    e.resolveObject(action.Object, related),
    }
    return e.mutationClient.RemoveTriple(ctx, req)
}
```

### Expression Evaluator Enhancements

```go
// New functions for rule conditions

// Check if relationship triple exists
func hasTriple(entityID, predicate, objectID string) bool

// Query OUTGOING_INDEX for relationships
func getOutgoing(entityID string) []OutgoingRelationship

// Query INCOMING_INDEX for reverse relationships
func getIncoming(entityID string) []IncomingRelationship

// Get entity by ID for cross-entity conditions
func getEntity(entityID string) EntityState

// Calculate distance between positions
func distance(pos1, pos2 Position) float64
```

---

## Post-MVP: Behavior Trees

### Foundation from Stateful ECA

Stateful ECA is a **degenerate behavior tree** (single node, no hierarchy). The abstraction can be extended:

```go
// MVP: StatefulRule implements Evaluator
type Evaluator interface {
    Evaluate(ctx context.Context, entity EntityState) (EvalResult, error)
}

type EvalResult struct {
    Matches      bool
    Transitioned bool
    Transition   Transition
}

// Post-MVP: BTNode extends Evaluator
type BTNode interface {
    Tick(ctx context.Context, blackboard *Blackboard) BTStatus
    Children() []BTNode
}

type BTStatus int

const (
    BTSuccess BTStatus = iota
    BTFailure
    BTRunning
)
```

### Behavior Tree Node Types

```go
// Selector: Try children until one succeeds
type Selector struct {
    children []BTNode
}

func (s *Selector) Tick(ctx context.Context, bb *Blackboard) BTStatus {
    for _, child := range s.children {
        status := child.Tick(ctx, bb)
        if status != BTFailure {
            return status
        }
    }
    return BTFailure
}

// Sequence: Run children until one fails
type Sequence struct {
    children []BTNode
}

func (s *Sequence) Tick(ctx context.Context, bb *Blackboard) BTStatus {
    for _, child := range s.children {
        status := child.Tick(ctx, bb)
        if status != BTSuccess {
            return status
        }
    }
    return BTSuccess
}

// Condition: Wraps ECA rule as BT node
type ConditionNode struct {
    rule StatefulRule
}

func (c *ConditionNode) Tick(ctx context.Context, bb *Blackboard) BTStatus {
    entity := bb.Get("entity").(EntityState)
    result, _ := c.rule.Evaluate(ctx, entity)
    if result.Matches {
        return BTSuccess
    }
    return BTFailure
}
```

### Behavior Tree Definition Format

```yaml
# Post-MVP: Full behavior tree syntax
behavior_trees:
  - id: "drone-proximity-behavior"
    entity_patterns: ["*.*.robotics.*.drone.*"]

    root:
      type: "selector"
      children:
        - type: "sequence"
          name: "handle-proximity"
          children:
            - type: "condition"
              rule: "distance(entity.position, $target.position) < 100"
            - type: "action"
              action: "add_triple"
              predicate: "proximity.near"
              object: "$target.id"

        - type: "sequence"
          name: "handle-separation"
          children:
            - type: "condition"
              rule: "distance(entity.position, $target.position) >= 100"
            - type: "condition"
              rule: "hasTriple(entity.id, 'proximity.near', $target.id)"
            - type: "action"
              action: "remove_triple"
              predicate: "proximity.near"
              object: "$target.id"
```

---

## KV Revision History Integration

### MVP Usage: Debugging and Audit

```go
type RuleMatchState struct {
    // ...existing fields...
    SourceRevision uint64 `json:"source_revision"` // Entity revision that caused state change
}
```

**Use cases:**
- Debug: "Which entity update triggered this rule?"
- Audit: "Trace relationship creation back to source event"
- Verification: "Is rule state consistent with entity at revision N?"

### Post-MVP Usage: Temporal Queries

```go
// Query graph state at specific revision
func (q *QueryEngine) GetGraphAsOfRevision(ctx context.Context, revision uint64) (*GraphSnapshot, error)

// Reconstruct entity state history
func (q *QueryEngine) GetEntityHistory(ctx context.Context, entityID string, from, to time.Time) ([]EntityState, error)
```

**Use cases:**
- Time-travel queries: "What was the graph state at 10:30 AM?"
- Compliance: "Prove this relationship existed during incident"
- Rollback: "Undo all changes after revision X"

---

## Implementation Phases

### Phase 1: MVP Stateful ECA (3-4 days)

| Component | Change | Effort |
|-----------|--------|--------|
| RULE_STATE bucket | New KV bucket + RuleMatchState type | 2-3 hours |
| State tracker | Get/Set with LRU caching | 4-6 hours |
| Transition detection | detectTransition() + evaluateWithState() | 2-3 hours |
| Rule schema | Add on_enter/on_exit/while_true fields | 2-3 hours |
| add_triple action | Create relationship triple via mutation API | 2-3 hours |
| remove_triple action | Remove relationship triple | 2-3 hours |
| Expression functions | hasTriple(), getOutgoing(), distance() | 3-4 hours |
| Tests | Integration tests for state transitions | 4-6 hours |

**Deliverables:**
- Stateful rules with on_enter/on_exit
- Automatic relationship retraction
- TTL fallback for graceful degradation

### Phase 2: TTL Cleanup Worker (1-2 days)

Add background worker to clean expired triples:

```go
func (p *GraphProcessor) cleanupExpiredTriples(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            p.scanAndCleanExpiredTriples(ctx)
        }
    }
}
```

**Note:** Aligns with ADR-TRIPLES-AS-SOURCE-OF-TRUTH proposal for Triple.ExpiresAt field.

### Phase 3: Behavior Trees (Post-MVP, 2-3 weeks)

| Component | Change | Effort |
|-----------|--------|--------|
| BTNode interface | Base interface + status types | 1 day |
| Core nodes | Selector, Sequence, Parallel | 2-3 days |
| Decorator nodes | Inverter, Repeater, Timeout | 2 days |
| Blackboard | Shared state for tree execution | 1 day |
| Tree parser | YAML/JSON → BTNode tree | 2-3 days |
| Tick scheduler | Event-driven or periodic ticking | 2-3 days |
| Integration | Connect to entity watcher | 1-2 days |

### Phase 4: Bi-Temporal Model (Long-term, 4-6 weeks)

Add ValidFrom/ValidTo to Triple for full temporal semantics:

```go
type Triple struct {
    // ... existing fields ...
    ValidFrom  time.Time  `json:"valid_from"`
    ValidTo    *time.Time `json:"valid_to,omitempty"`
}
```

---

## Compatibility Notes

### With ADR-TRIPLES-AS-SOURCE-OF-TRUTH

| This ADR | Triples ADR | Alignment |
|----------|-------------|-----------|
| add_triple action | Triples as source | ✅ Creates relationship triples |
| remove_triple action | No edges field | ✅ Removes triples, not edges |
| hasTriple() function | OUTGOING_INDEX | ✅ Queries index, not stored edges |
| TTL/expiration | Triple.ExpiresAt | ✅ Uses proposed field |

### Migration Path

1. **Phase 1 of Triples ADR**: Add OUTGOING_INDEX
2. **Phase 1 of this ADR**: Stateful ECA using triples
3. **Phase 2 of Triples ADR**: Deprecate Edges field
4. **Both complete**: Clean architecture with triples + indexes + stateful rules

---

## Decision

**Recommended approach:**

1. **Implement MVP stateful ECA** (3-4 days) for immediate graph consistency
2. **Coordinate with OUTGOING_INDEX work** from Triples ADR
3. **Design behavior trees** post-MVP using same state tracking foundation
4. **Leverage KV revision history** for debugging now, temporal queries later

---

## References

- `ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md` - Triples as single source, OUTGOING_INDEX
- `TODO-GRAPH-INDEXING-ARCHITECTURE.md` - Index management issues
- `processor/rule/` - Current Rules Engine
- `message/triple.go` - Triple struct with RDF* metadata
- `graph/types.go` - Edge.ExpiresAt (pattern for Triple.ExpiresAt)
- NATS KV documentation - Revision history API
