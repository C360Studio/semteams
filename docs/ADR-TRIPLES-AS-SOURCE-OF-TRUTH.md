# ADR: Triples as Single Source of Truth

**Status:** Implemented
**Author:** Documentation Audit Team
**Date:** 2024-11-25
**Supersedes:** None
**Related:** SPEC-SEMANTIC-CONTRACT.md, ADR-TEMPORAL-GRAPH-MODELING.md, TODO-GRAPH-INDEXING-ARCHITECTURE.md

---

## Context

During documentation audit, we discovered a circular data flow in the graph processor that creates unnecessary complexity and cognitive overhead.

### Current Architecture

```
User's Message (properties)
        │
        ▼
Graphable.Triples()  ──────────────────────────────────┐
        │                                              │
        │  User converts properties → triples         │
        │                                              │
        ▼                                              │
extractPropertiesAndRelationships()                    │
        │                                              │
        ├──► Node.Properties map[string]any  ← CIRCULAR: back to properties
        │                                              │
        └──► Edges []Edge  ← DERIVED: from relationship triples
                                                       │
                                                       ▼
                                              state.Triples (stored)
```

### Current EntityState Structure

```go
type EntityState struct {
    Node        NodeProperties   // Contains Properties map (duplicated from triples)
    Edges       []Edge           // Derived from relationship triples
    Triples     []message.Triple // Original semantic facts
    ObjectRef   string
    MessageType string
    Version     uint64
    UpdatedAt   time.Time
}

type NodeProperties struct {
    ID         string
    Type       string
    Properties map[string]any  // ← DUPLICATION: Same data as property triples
    Position   *Position
    Status     EntityStatus
}
```

### Current Index Usage

| Index | Source Field | Issue |
|-------|--------------|-------|
| PREDICATE_INDEX | `state.Triples` | Correct |
| INCOMING_INDEX | `state.Edges` | Uses derived data, not source |
| ALIAS_INDEX | `state.Triples` | Correct |
| SPATIAL_INDEX | `state.Triples` | Correct |
| TEMPORAL_INDEX | `state.UpdatedAt` | Correct |

**Missing:** OUTGOING_INDEX - currently we store Edges on EntityState to enable outgoing traversal.

---

## Problem Statement

### 1. Circular Data Conversion

```
Properties → Triples() → extractPropertiesAndRelationships() → Properties
```

Users convert their message properties to triples via `Graphable.Triples()`. The system then converts those triples back to a properties map for `NodeProperties.Properties`. This is wasteful and confusing.

### 2. Dual Representation of Relationships

Relationship triples exist in TWO places:

```go
// As Triple (in state.Triples)
{Subject: droneID, Predicate: "ops.fleet.member_of", Object: fleetID}

// As Edge (in state.Edges)
{ToEntityID: fleetID, EdgeType: "ops.fleet.member_of", Weight: 1.0, ...}
```

Same information, different formats, both stored.

### 3. Asymmetric Index Design

- INCOMING_INDEX uses `state.Edges` (derived data)
- No OUTGOING_INDEX exists
- Outgoing traversal requires stored Edges on EntityState

### 4. Cognitive Overhead

Users must understand:
- Triples (RDF concept)
- Edges (graph concept)
- How triples become edges (implicit `IsRelationship()` check)
- Why some indexes use Triples, others use Edges

---

## Decision

**Triples are the single source of truth for entity data.**

### Proposed EntityState Structure

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

**Removed:**
- `Node.Properties` - Redundant, properties ARE triples
- `Edges` - Redundant, relationships ARE triples, indexed via OUTGOING_INDEX

### Proposed Index Architecture

| Index | Source | Key Format | Value |
|-------|--------|------------|-------|
| ENTITY_STATES | - | `{entityID}` | EntityState JSON |
| PREDICATE_INDEX | All triples | `{predicate}:{value}` | `[entityID, ...]` |
| **OUTGOING_INDEX** | Relationship triples | `{fromEntityID}` | `[{predicate, toEntityID}, ...]` |
| INCOMING_INDEX | Relationship triples | `{toEntityID}` | `[{predicate, fromEntityID}, ...]` |
| ALIAS_INDEX | Alias triples | `{alias}` | `entityID` |
| SPATIAL_INDEX | Geo triples | `{geohash}` | `[entityID, ...]` |
| TEMPORAL_INDEX | UpdatedAt | `{timeBucket}` | `[entityID, ...]` |

### OUTGOING_INDEX Structure

```go
// Key: entityID
// Value: array of outgoing relationships
type OutgoingEntry struct {
    Predicate    string `json:"predicate"`
    ToEntityID   string `json:"to_entity_id"`
}

// Example:
// Key: "acme.telemetry.robotics.gcs1.drone.001"
// Value: [
//   {"predicate": "ops.fleet.member_of", "to_entity_id": "acme.ops.logistics.hq.fleet.rescue"},
//   {"predicate": "robotics.operator.controlled_by", "to_entity_id": "acme.platform.auth.main.user.alice"}
// ]
```

### Data Flow (Proposed)

```
Graphable.Triples()
        │
        ▼
   Validate triples
        │
        ▼
   Store in ENTITY_STATES
        │
        ├──► PREDICATE_INDEX (all triples)
        ├──► OUTGOING_INDEX (relationship triples, forward)
        ├──► INCOMING_INDEX (relationship triples, reverse)
        ├──► ALIAS_INDEX (alias predicate triples)
        └──► SPATIAL_INDEX (geo predicate triples)
```

**No conversion. No duplication. Triples flow directly to storage and indexes.**

---

## Consequences

### Positive

1. **Single source of truth** - Triples only, no derived structures
2. **No circular conversion** - Properties stay as triples
3. **Symmetric traversal** - OUTGOING_INDEX mirrors INCOMING_INDEX
4. **Simpler EntityState** - Fewer fields, clearer purpose
5. **Reduced storage** - No duplicate property/edge data
6. **Cleaner mental model** - Users learn one concept (Triples)

### Negative

1. **Breaking change** - EntityState structure changes
2. **Migration required** - Existing data needs transformation
3. **Query changes** - Code accessing `entity.Edges` or `entity.Node.Properties` must change
4. **Index rebuild** - OUTGOING_INDEX must be built, existing indexes may need rebuild

### Neutral

1. **Property access pattern changes** - Instead of `entity.Node.Properties["battery"]`, query by predicate or iterate triples
2. **Edge weight/confidence** - Must be stored as triple metadata or separate triples

---

## Implementation Plan

### Phase 1: Add OUTGOING_INDEX (Non-Breaking)

1. Create OUTGOING_INDEX alongside existing indexes
2. Populate from relationship triples (same source as Edges)
3. Add query methods that use OUTGOING_INDEX
4. Validate parity with existing Edge-based queries

### Phase 2: Deprecate Edges Field

1. Mark `EntityState.Edges` as deprecated
2. Update query code to use OUTGOING_INDEX
3. Log warnings when Edges field is accessed
4. Update documentation

### Phase 3: Deprecate Node.Properties

1. Mark `NodeProperties.Properties` as deprecated
2. Add helper methods: `GetPropertyTriple(predicate)`, `GetPropertyValue(predicate)`
3. Update query code to use triple-based property access
4. Log warnings when Properties map is accessed

### Phase 4: Remove Deprecated Fields (Breaking)

1. Remove `Edges` from EntityState
2. Remove `Properties` from NodeProperties (or remove NodeProperties entirely)
3. Simplify EntityState to proposed structure
4. Migration script for existing data

### Phase 5: Cleanup

1. Remove `extractPropertiesAndRelationships()`
2. Remove `buildEdgesFromRelationships()`
3. Simplify MessageManager processor
4. Update all tests

---

## Migration

### Data Migration

```go
// For each entity in ENTITY_STATES:
// 1. Read current EntityState
// 2. Triples field already contains all data
// 3. Build OUTGOING_INDEX entries from relationship triples
// 4. Write simplified EntityState (or just rebuild indexes)
```

### Code Migration

```go
// Before: Access properties via Node.Properties
value := entity.Node.Properties["robotics.battery.level"]

// After: Access via triple lookup
triple := entity.GetTriple("robotics.battery.level")
value := triple.Object

// Or helper method:
value := entity.GetPropertyValue("robotics.battery.level")
```

```go
// Before: Traverse via Edges
for _, edge := range entity.Edges {
    target := edge.ToEntityID
}

// After: Query OUTGOING_INDEX
outgoing, _ := indexManager.GetOutgoing(ctx, entityID)
for _, rel := range outgoing {
    target := rel.ToEntityID
}
```

---

## Alternatives Considered

### Alternative 1: Keep Edges, Add OUTGOING_INDEX

Keep both Edges on EntityState AND add OUTGOING_INDEX.

**Rejected:** Still has dual representation problem. Indexes should be derived from stored data, not duplicated.

### Alternative 2: Rename Only

Rename Triples to "Facts", Edges to "Links", improve documentation.

**Rejected:** Doesn't solve the circular conversion or dual storage problems. Lipstick on a pig.

### Alternative 3: Separate Properties and Relationships in Interface

```go
type Graphable interface {
    EntityID() string
    Properties() []PropertyFact
    Relationships() []RelationshipFact
}
```

**Rejected:** More verbose for users. Current `Triples()` interface is simpler. The system can determine property vs relationship via `IsRelationship()`.

---

## Triple Already Has RDF*-Like Metadata

The current Triple struct (`message/triple.go`) already supports rich metadata:

```go
type Triple struct {
    Subject    string    `json:"subject"`              // Entity ID (6-part format)
    Predicate  string    `json:"predicate"`            // Dotted notation property
    Object     any       `json:"object"`               // Value or entity reference

    // RDF*-like metadata (already implemented):
    Source     string    `json:"source"`               // Provenance ("mavlink", "operator", "ai_inference")
    Timestamp  time.Time `json:"timestamp"`            // When assertion was made
    Confidence float64   `json:"confidence"`           // 0.0 to 1.0 reliability
    Context    string    `json:"context,omitempty"`    // Correlation/batch ID
    Datatype   string    `json:"datatype,omitempty"`   // RDF datatype hint ("xsd:float", "geo:point")
}
```

### Comparison: Edge Fields vs Triple Fields

| Edge Field | Triple Equivalent | Status |
|------------|-------------------|--------|
| `ToEntityID` | `Object` (when IsRelationship) | ✅ Already there |
| `EdgeType` | `Predicate` | ✅ Already there |
| `Confidence` | `Confidence` | ✅ Already there |
| `CreatedAt` | `Timestamp` | ✅ Already there |
| `Weight` | `Confidence` (or add field) | ⚠️ Semantic overlap |
| `Properties` | Additional triples | ✅ Pattern exists |
| `ExpiresAt` | - | ❌ **Needs to be added** |

### Proposed: Add ExpiresAt to Triple

```go
// ExpiresAt indicates when this assertion should be considered stale.
// Used for temporary relationships like proximity that change over time.
// nil means the assertion does not expire.
ExpiresAt *time.Time `json:"expires_at,omitempty"`
```

This enables:
- **Dynamic proximity relationships**: USV near buoy expires after 5 minutes if not refreshed
- **Temporary operational states**: Mission assignment expires at end of shift
- **Computed relationships**: AI-inferred relationships with TTL

### Cleanup Implementation

With ExpiresAt on Triple, cleanup becomes straightforward:

```go
// During entity processing or query time:
func (es *EntityState) RemoveExpiredTriples() {
    now := time.Now()
    filtered := es.Triples[:0]
    for _, triple := range es.Triples {
        if triple.ExpiresAt == nil || triple.ExpiresAt.After(now) {
            filtered = append(filtered, triple)
        }
    }
    es.Triples = filtered
}
```

This replaces the current `RemoveExpiredEdges()` method which exists but is never called.

---

## Remaining Open Questions

1. **Weight vs Confidence** - Edge has both Weight and Confidence. Options:
   - Use Confidence for both (semantic similarity)
   - Add Weight field to Triple for graph algorithms that need numeric distance
   - Store weight as a separate triple: `{Subject: relationshipID, Predicate: "graph.edge.weight", Object: 0.95}`

2. **NodeProperties.Position** - Currently separate field. Should become geo triples only?
   - Recommendation: Yes, use `geo.location.latitude` and `geo.location.longitude` predicates

3. **NodeProperties.Status** - Entity operational status. Should become a triple?
   - Recommendation: Yes, use `entity.status` predicate

4. **Performance** - Is triple iteration slower than map lookup for property access? Need benchmarks.
   - Mitigation: Helper methods like `GetTriple(predicate)` can use predicate-keyed internal map

---

## References

- `processor/graph/messagemanager/processor.go` - Current circular conversion
- `processor/graph/indexmanager/indexes.go` - Current index implementations
- `graph/types.go` - Current EntityState structure
- `message/triple.go` - Triple structure and IsRelationship()
- SPEC-SEMANTIC-CONTRACT.md - Graphable interface requirement

---

## Decision Outcome

**Pending team review.**

This ADR proposes a significant but necessary simplification of SemStreams' core data model. The circular conversion pattern and dual storage were discovered during documentation audit and represent accidental complexity that should be eliminated.

The key insight: **Triples are already the semantic contract (via Graphable interface). They should also be the storage contract.**
