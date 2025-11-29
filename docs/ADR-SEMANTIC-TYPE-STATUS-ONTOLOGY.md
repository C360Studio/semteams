# ADR: Semantic Type and Status Ontology Clarification

- **Status**: Implemented
- **Created**: 2025-01-29
- **Updated**: 2025-01-29
- **Implemented**: 2025-01-29 (via 004-semantic-refactor)
- **Context**: 003-triples-architecture migration revealed ontology confusion
- **Related**: ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.md, specs/004-semantic-refactor/

## Problem Statement

During the triples-as-single-source-of-truth migration, we discovered overlapping concepts, unused abstractions, and architectural inconsistencies that need resolution. The core question is: **What should the framework provide vs what belongs to domain implementations?**

Key issues discovered:

1. Multiple overlapping concepts for "type" and "status"
2. EntityClass and EntityRole exist but are unused technical debt
3. NodeProperties contradicts "triples as single source of truth"
4. ObjectRef in EntityState is a bare string losing StorageReference metadata
5. Hardcoded status is a clear bug

## Decisions

### D1: Framework vs Domain Responsibilities

The framework is **infrastructure** - it moves messages, stores triples, indexes entities. It has **no domain knowledge** about what entities mean, what "healthy" means, or how to classify things semantically.

**Framework responsibilities:**

- Message transport and routing
- Triple storage and retrieval
- Entity identity management (6-part EntityID)
- Indexing for efficient queries
- Storage references for full message access

**Domain responsibilities:**

- Semantic classification of entities
- Status definitions and thresholds
- Business rules and logic
- What triples to emit for any given message

**There will be no `system.*` predicate namespace managed by the framework.**

### D2: Delete EntityClass and EntityRole

These were added early based on Schema.org/RDF patterns but never integrated:

| Location | Usage |
|----------|-------|
| `message/entity_payload.go` | Part of EntityPayload struct (ZERO external consumers) |
| `processor/graph/messagemanager/processor.go:293-294` | Fallback path hardcodes `ClassThing` and `RolePrimary` |

**No queries use these. No indexes use these. No domain implementations use these.**

The framework cannot determine these values - only domain knowledge can answer "Is a SensorReading an Object or Event?" EntityRole is per-message context, not per-entity state.

**Action:** Delete `entity_types.go` and `entity_payload.go`.

### D3: Delete EntityStatus Enum

The `EntityStatus` enum (active/warning/critical/emergency/inactive) implies the framework knows what these states mean. It doesn't and can't:

- Thresholds are domain-specific (10% battery: critical for drones, acceptable for sensors)
- Status is computed from multiple signals (connectivity + battery + error rate)
- Business rules define severity

**Action:** Delete `EntityStatus` from `graph/types.go`. Domains define their own status vocabularies and emit status as domain triples.

### D4: Replace ObjectRef with StorageRef

Current `ObjectRef string` loses critical metadata. Replace with proper type:

```go
// Optional storage reference - supports "store once, reference anywhere" pattern
StorageRef *message.StorageReference `json:"storage_ref,omitempty"`
```

This preserves `StorageInstance`, `ContentType`, and `Size` when available.

### D5: Eliminate NodeProperties

Current NodeProperties is redundant, dead, or buggy:

| Field | Verdict | Reasoning |
|-------|---------|-----------|
| `ID` | Essential | But should be `EntityState.ID` directly |
| `Type` | Redundant | Just parses segment 5 from EntityID |
| `Position` | Dead code | Never set; spatial index reads from triples |
| `Status` | Bug | Hardcoded `StatusActive`; deleted per D3 |

**Action:** Promote `ID` to `EntityState.ID`, delete `NodeProperties` struct entirely.

### D6: No Type() Helper on EntityState

The question was raised whether EntityState needs a `Type()` method. Analysis shows:

- `message.ParseEntityID(id)` already exists and returns full struct
- Provides access to ALL parts: Org, Platform, Domain, System, Type, Instance
- ID parsing belongs in `message` package, not `graph` package

Consumers needing the type segment use:

```go
eid, _ := message.ParseEntityID(state.ID)
entityType := eid.Type
```

**Action:** No `Type()` helper on EntityState. Use existing `message.ParseEntityID()`.

### D7: Greenfield Migration

This is a greenfield project. No backward compatibility shims, no deprecation periods.

**Action:** Break and fix. Update all `state.Node.ID` usages to `state.ID` directly.

## Proposed EntityState

```go
// EntityState represents complete local graph state for an entity.
// Triples are the single source of truth for all semantic properties.
type EntityState struct {
    // ID is the 6-part entity identifier: org.platform.domain.system.type.instance
    // Used as NATS KV key for storage and retrieval.
    ID string `json:"id"`

    // Triples contains all semantic facts about this entity.
    // Properties, relationships, and domain-specific data are all stored as triples.
    Triples []message.Triple `json:"triples"`

    // StorageRef optionally points to where the full original message is stored.
    // Supports "store once, reference anywhere" pattern for large payloads.
    // Nil if message was not stored or storage reference not available.
    StorageRef *message.StorageReference `json:"storage_ref,omitempty"`

    // MessageType records the original message type that created/updated this entity.
    // Provides provenance and enables filtering by message source.
    MessageType message.Type `json:"message_type"`

    // Version is incremented on each update for optimistic concurrency control.
    Version uint64 `json:"version"`

    // UpdatedAt records when this entity state was last modified.
    UpdatedAt time.Time `json:"updated_at"`
}
```

Note: `message.Type` is a proper struct with `Domain`, `Category`, and `Version` fields, providing type-safe message classification rather than a bare string.

**Removed:**

- `Node NodeProperties` - eliminated, ID promoted to top level
- `ObjectRef string` - replaced with proper `*StorageReference`

**Deleted from codebase:**

- `NodeProperties` struct
- `Position` struct (spatial data comes from triples)
- `EntityStatus` enum
- `EntityClass` enum
- `EntityRole` enum
- `entity_types.go`
- `entity_payload.go`

## RDF/Semantic Web Clarification

### What We Borrowed from RDF

The Triple structure follows RDF's Subject-Predicate-Object pattern:

```go
type Triple struct {
    Subject    string    // Entity ID
    Predicate  string    // Property name (dotted notation)
    Object     any       // Value or entity reference
    // ... metadata
}
```

This is useful for:

- Flexible property storage
- Relationship representation
- NATS wildcard queries on predicates

### What We Should NOT Borrow from RDF

RDF/OWL traditions include rich ontological classification:

- `rdf:type` for class membership
- `rdfs:Class` hierarchies
- `owl:Thing` as universal superclass
- Inference engines reasoning over class hierarchies

**This is not our use case.** SemStreams is:

- Domain-specific message processing (not universal knowledge graphs)
- Operational systems (not inference engines)
- Known schemas at compile time (not arbitrary RDF discovery)

Domains that want Schema.org-style classification can emit those triples themselves using domain predicates. The framework doesn't provide or require them.

## Summary of Changes

| Item | Action | Rationale |
|------|--------|-----------|
| `EntityClass` | DELETE | Unused, framework can't determine, over-engineering |
| `EntityRole` | DELETE | Unused, per-message not per-entity, over-engineering |
| `EntityStatus` | DELETE | Framework has no domain knowledge of status semantics |
| `entity_types.go` | DELETE | Contains only deleted types |
| `entity_payload.go` | DELETE | Zero external consumers |
| `NodeProperties` | DELETE | Redundant/dead/buggy fields |
| `Position` struct | DELETE | Spatial data comes from triples |
| `ObjectRef` | REPLACE | Use `*StorageReference` with full metadata |
| `system.*` namespace | DO NOT CREATE | Framework has no domain knowledge |
| 6-part EntityID | KEEP | Well-designed identity structure |
| `message.ParseEntityID()` | KEEP | Proper way to extract ID components |

## Implementation Steps

**All steps completed via 004-semantic-refactor:**

1. [x] Delete `message/entity_types.go` and `message/entity_types_test.go`
2. [x] Delete `message/entity_payload.go`
3. [x] Update `graph/types.go`:
   - Delete `NodeProperties` struct
   - Delete `Position` struct
   - Delete `EntityStatus` enum and methods
   - Add `ID string` field to EntityState
   - Replace `ObjectRef string` with `StorageRef *message.StorageReference`
4. [x] Update `types/graph/types.go` (duplicate) with same changes
5. [x] Update `processor/graph/messagemanager/processor.go`:
   - Remove hardcoded `StatusActive`
   - Remove `entity_class` and `entity_role` triple generation
   - Populate `StorageRef` instead of `ObjectRef`
   - Use `state.ID` instead of `state.Node.ID`
6. [x] Update all consumers of `state.Node.*` to use `state.*` or `message.ParseEntityID()`
7. [x] Update GraphQL resolvers to use new EntityState structure
8. [x] Update tests

**Documentation updated:**

- [x] Updated `types/graph/README.md` with new EntityState structure
- [x] Updated `specs/004-semantic-refactor/` spec and tasks
- [x] Migration guide available in `specs/004-semantic-refactor/quickstart.md`

## References

- `message/graphable.go` - Core Graphable interface (unchanged)
- `message/storable.go` - Storable interface with StorageReference
- `message/types.go` - EntityID struct and ParseEntityID function
- `message/triple.go` - Triple structure
- `graph/types.go` - EntityState (to be simplified)
- `processor/graph/messagemanager/processor.go` - Message processing (to be updated)
