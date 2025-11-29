# Implementation Plan: Triples Architecture Evolution

**Branch**: `003-triples-architecture` | **Date**: 2025-11-27 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-triples-architecture/spec.md`
**Updated**: 2025-11-28 (spec update for index sync and relationship detection)

## Summary

Establish semantic Triples as the single source of truth for entity data, replacing the dual Triples/Edges/Properties representation. Add OUTGOING_INDEX for forward relationship traversal, implement stateful ECA rules with automatic relationship retraction, and align community detection with the triple-based model.

**Key Changes (2025-11-28 Update)**:
- Added FR-005a/b/c: INCOMING_INDEX orphan cleanup on entity deletion
- Added FR-006a/b: Relationship detection via Triple.IsRelationship() (no hardcoded predicates)
- Added SC-011/SC-012: Success criteria for cleanup and relationship detection

## Technical Context

**Language/Version**: Go 1.25+
**Primary Dependencies**: NATS JetStream (KV buckets), existing IndexManager, RuleProcessor
**Storage**: NATS KV (ENTITY_STATES, PREDICATE_INDEX, INCOMING_INDEX, OUTGOING_INDEX, RULE_STATE)
**Testing**: `go test -race`, table-driven tests, integration tests with real NATS
**Target Platform**: Linux server (containerized)
**Project Type**: Single backend service (graph processor)
**Performance Goals**: OUTGOING_INDEX queries within 10% of Edge-based traversal
**Constraints**: Atomic index updates, no orphaned references after deletion
**Scale/Scope**: Thousands of entities, hundreds of relationships per entity

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Spec-First Development | ✅ PASS | spec.md defines all requirements, success criteria |
| II. TDD (NON-NEGOTIABLE) | ✅ PASS | Tests written first per tasks.md, verified RED before implementation |
| III. Quality Gate Compliance | ✅ PASS | Six gates enforced in tasks.md workflow |
| IV. Code Review Standards | ✅ PASS | go-reviewer agent validates all implementations |
| V. Documentation & Traceability | ✅ PASS | research.md, data-model.md, ADRs linked |

**Go Standards Check**:
- [x] `go fmt` - enforced at Gate 3/5
- [x] `revive` linting - enforced at Gate 3/5
- [x] `-race` flag on all tests - enforced at Gate 5
- [x] Context as first parameter - followed in all I/O operations
- [x] Error wrapping with context - using errors package

## Project Structure

### Documentation (this feature)

```text
specs/003-triples-architecture/
├── plan.md              # This file
├── spec.md              # Feature specification (updated 2025-11-28)
├── research.md          # Phase 0 research findings
├── data-model.md        # Entity definitions and relationships
├── quickstart.md        # Developer quick start guide
├── contracts/           # API contracts (internal Go interfaces)
├── checklists/          # Quality checklists
│   └── requirements.md  # Spec validation checklist
└── tasks.md             # Implementation tasks with TDD workflow
```

### Source Code (repository root)

```text
message/
└── triple.go            # Triple struct (ExpiresAt field added)

graph/
└── types.go             # EntityState, NodeProperties (simplified)

processor/graph/indexmanager/
├── indexes.go           # Index implementations (OUTGOING_INDEX, cleanup methods)
├── manager.go           # IndexManager (orchestrates cleanup)
└── *_test.go            # Unit and integration tests

processor/graph/datamanager/
├── edge_ops.go          # Triple operations (relationship detection fix)
└── edge_ops_test.go     # Tests for relationship detection

processor/rule/
├── config.go            # Rule Definition (OnEnter/OnExit/WhileTrue)
├── state_tracker.go     # RuleMatchState, StateTracker (NEW)
├── actions.go           # add_triple, remove_triple actions (NEW)
└── *_test.go            # Rule processor tests

pkg/graphclustering/
├── storage.go           # Community triple creation
└── config.go            # CreateTriples config option
```

**Structure Decision**: Existing monorepo structure maintained. New files added to existing packages following established patterns.

## Implementation Phases

### Phase 1: Foundation (US1 - Forward Traversal)

**Status**: ✅ COMPLETE

- OUTGOING_INDEX implemented with HandleCreate/Update/Delete
- GetOutgoing and GetOutgoingByPredicate query methods
- Triple.ExpiresAt field added
- Unit and integration tests passing

### Phase 2: Stateful Rules (US2 - Auto Retraction)

**Status**: ✅ COMPLETE (2025-11-28)

- RULE_STATE KV bucket - StateTracker persists match state
- StateTracker with transition detection - DetectTransition() function
- OnEnter/OnExit/WhileTrue rule actions - StatefulEvaluator fires actions on transitions
- add_triple/remove_triple action types - ActionExecutor supports both
- Expression functions: hasTriple(), getOutgoing(), distance()
- Expired triple cleanup worker - exists in processor/graph/cleanup.go

### Phase 3: Community Alignment (US3 - Unified Queries)

**Status**: ⏳ PENDING

- CreateTriples config option
- Community membership as relationship triples
- Dual-write (COMMUNITY_INDEX + triples)

### Phase 4: Simplified Data Model (US4 - Deprecation/Removal)

**Status**: ✅ COMPLETE (Greenfield)

- Edges field removed from EntityState
- Properties field removed from NodeProperties
- Triples as single source of truth
- Helper methods GetTriple(), GetPropertyValue() available

### Phase 6b: Index Synchronization Fixes (Post-US4)

**Status**: 🔄 IN PROGRESS

**Gap Identified**: Entity deletion leaves orphaned INCOMING_INDEX entries. The `isRelationshipPredicate()` function uses hardcoded predicate list instead of vocabulary system.

**Requirements**:
- FR-005a: Read OUTGOING_INDEX on delete to identify targets
- FR-005b: Remove deleted entity from each target's INCOMING_INDEX
- FR-005c: Cleanup sequence (INCOMING first, then OUTGOING)
- FR-006a: Use Triple.IsRelationship() for relationship detection
- FR-006b: No hardcoded predicate lists

**Implementation**:
1. IncomingIndex.RemoveIncomingReference() - already exists
2. Manager.CleanupOrphanedIncomingReferences() - orchestrates cleanup
3. Manager.updateIndex() - calls cleanup before outgoing delete
4. Replace isRelationshipPredicate() with Triple.IsRelationship()

### Phase 5: Polish

**Status**: ⏳ PENDING

- Full test suite validation
- Documentation updates
- Performance benchmarks

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |

## Key Design Decisions

### D1: Orphan Cleanup Sequence (NEW - 2025-11-28)

**Decision**: When deleting an entity, cleanup INCOMING_INDEX references BEFORE deleting OUTGOING_INDEX entry.

**Rationale**: OUTGOING_INDEX contains the list of target entities that need their INCOMING_INDEX entries updated. If we delete OUTGOING first, we lose that information.

**Sequence**:
1. Read deleted entity's OUTGOING_INDEX to get target entity IDs
2. For each target, call IncomingIndex.RemoveIncomingReference(targetID, deletedID)
3. Delete OUTGOING_INDEX entry for deleted entity
4. Delete INCOMING_INDEX entry for deleted entity itself

### D2: Relationship Detection (NEW - 2025-11-28)

**Decision**: Use Triple.IsRelationship() which validates Object as 6-part EntityID.

**Rationale**: The vocabulary system and EntityID format are the authoritative sources for what constitutes a relationship. Hardcoded predicate lists are unmaintainable and miss new predicates.

**Implementation**: Replace all calls to `isRelationshipPredicate(predicate)` with `triple.IsRelationship()`.

### D3: OUTGOING_INDEX as Primary Traversal

**Decision**: Use OUTGOING_INDEX for forward relationship queries instead of iterating entity.Edges.

**Rationale**: Edges field is being deprecated. OUTGOING_INDEX provides the same data with direct KV lookup.

### D4: Stateful Rules with Persistent State

**Decision**: Store rule match state in RULE_STATE KV bucket, not in-memory.

**Rationale**: In-memory state lost on restart causes rules to re-fire on_enter incorrectly.

## References

- [spec.md](./spec.md) - Feature specification
- [research.md](./research.md) - Research findings
- [data-model.md](./data-model.md) - Entity definitions
- [tasks.md](./tasks.md) - Implementation tasks
- `docs/ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md` - Architecture decision
- `message/triple.go` - Triple.IsRelationship() implementation
