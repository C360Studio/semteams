# Research: Semantic System Refactor

**Feature**: 004-semantic-refactor
**Date**: 2025-01-29
**Status**: Complete (ADR-driven)

## Overview

This research phase is minimal because all architectural decisions have been made and documented in the approved ADR:

- **Reference**: [ADR-SEMANTIC-TYPE-STATUS-ONTOLOGY.md](../../docs/ADR-SEMANTIC-TYPE-STATUS-ONTOLOGY.md)

The ADR documents extensive investigation including:
- Framework vs domain responsibility analysis
- Usage analysis of EntityClass, EntityRole, EntityStatus (all unused)
- NodeProperties field-by-field analysis (redundant/dead/buggy)
- ObjectRef vs StorageReference comparison
- RDF/Semantic Web boundary clarification

## Decisions from ADR

### D1: Framework vs Domain Responsibilities

**Decision**: Framework is infrastructure only. No `system.*` predicate namespace.

**Rationale**: Framework cannot determine EntityClass (is SensorReading an Object or Event?), EntityRole (per-message context), or operational status (thresholds are domain-specific).

**Alternatives Considered**: System-level predicates for cross-domain queries. Rejected because framework has no domain knowledge.

### D2: Delete EntityClass and EntityRole

**Decision**: Delete `message/entity_types.go` and `message/entity_payload.go`.

**Rationale**: Zero external consumers. Only used in fallback path with hardcoded values.

**Alternatives Considered**: Make mandatory on Graphable interface. Rejected because framework cannot determine correct values.

### D3: Delete EntityStatus Enum

**Decision**: Delete EntityStatus from `graph/types.go`. Domains emit status as triples.

**Rationale**: Status semantics are domain-specific. 10% battery is critical for drones but acceptable for warehouse sensors.

**Alternatives Considered**: Keep as vocabulary suggestion. Rejected - just delete, domains define their own.

### D4: Replace ObjectRef with StorageRef

**Decision**: Use `*message.StorageReference` instead of `ObjectRef string`.

**Rationale**: Bare string loses StorageInstance, ContentType, and Size metadata needed for store-once-reference-anywhere pattern.

**Alternatives Considered**: Keep ObjectRef with separate metadata fields. Rejected - StorageReference already exists and is correct.

### D5: Eliminate NodeProperties

**Decision**: Promote ID to EntityState.ID, delete NodeProperties struct entirely.

**Rationale**:
- ID: Essential but should be top-level
- Type: Redundant (just parses segment 5 from ID)
- Position: Dead code (never set, spatial index reads from triples)
- Status: Bug (hardcoded StatusActive)

**Alternatives Considered**: Keep minimal NodeProperties with just ID and Type. Rejected - maintains false impression of separate concerns.

### D6: No Type() Helper on EntityState

**Decision**: Use `message.ParseEntityID(state.ID).Type` instead of helper method.

**Rationale**: ParseEntityID already exists, returns full struct with all 6 components. ID parsing belongs in message package.

**Alternatives Considered**: Add Type() helper to EntityState. Rejected - duplicates existing functionality.

### D7: Greenfield Migration

**Decision**: Break and fix. No backward compatibility shims.

**Rationale**: This is a greenfield project with no production data.

**Alternatives Considered**: Deprecation period, migration scripts. Rejected - unnecessary complexity.

## Impact Analysis

### Files to Delete (3 files)

| File | Lines | Reason |
|------|-------|--------|
| `message/entity_types.go` | 143 | EntityClass/EntityRole unused |
| `message/entity_types_test.go` | 148 | Tests for deleted types |
| `message/entity_payload.go` | ~100 | Zero external consumers |

### Types to Delete (5 types)

| Type | Location | Reason |
|------|----------|--------|
| EntityClass | message/entity_types.go | Unused |
| EntityRole | message/entity_types.go | Unused |
| EntityStatus | graph/types.go | Framework can't know status |
| NodeProperties | graph/types.go | Redundant/dead/buggy |
| Position | graph/types.go | Dead code |

### Consumer Updates Required

Based on grep analysis from ADR investigation:

| Pattern | Count | Migration |
|---------|-------|-----------|
| `state.Node.ID` | ~40 | → `state.ID` |
| `state.Node.Type` | ~16 | → `message.ParseEntityID(state.ID).Type` |
| `state.Node.Status` | ~1 | Delete (was hardcoded anyway) |
| `state.Node.Position` | ~2 | Delete (always nil) |
| `ObjectRef` | ~10 | → `StorageRef` |

## No Outstanding Unknowns

All technical decisions documented in ADR. No NEEDS CLARIFICATION items.
