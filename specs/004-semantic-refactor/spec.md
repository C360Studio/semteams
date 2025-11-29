# Feature Specification: Semantic System Refactor

**Feature Branch**: `004-semantic-refactor`
**Created**: 2025-01-29
**Status**: Complete
**Completed**: 2025-01-29
**Input**: User description: "Semantic System Refactor - review adr here docs/ADR-SEMANTIC-TYPE-STATUS-ONTOLOGY.md"
**Reference**: [ADR-SEMANTIC-TYPE-STATUS-ONTOLOGY.md](../../docs/ADR-SEMANTIC-TYPE-STATUS-ONTOLOGY.md)

## Overview

This refactor simplifies the entity state model by removing unused abstractions and clarifying the boundary between framework infrastructure and domain-specific semantics. The core principle is that the framework provides infrastructure (message transport, triple storage, entity identity, indexing) while domains own semantic meaning (classification, status, business rules).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Domain Developer Uses Simplified EntityState (Priority: P1)

A domain developer implementing a new message processor works with a cleaner EntityState structure. They access entity ID directly (`state.ID`) instead of through a nested struct (`state.Node.ID`). When they need to parse the entity ID into components, they use the existing `message.ParseEntityID()` function.

**Why this priority**: This is the foundational change that all other work depends on. EntityState is the core data structure for entity storage and retrieval.

**Independent Test**: Can be fully tested by creating, storing, and retrieving an EntityState with the new structure, verifying ID access works correctly.

**Acceptance Scenarios**:

1. **Given** a new EntityState, **When** I set the ID field directly, **Then** the entity can be stored and retrieved using that ID as the NATS KV key
2. **Given** a stored EntityState, **When** I access `state.ID`, **Then** I get the 6-part entity identifier
3. **Given** an entity ID string, **When** I call `message.ParseEntityID(state.ID)`, **Then** I receive a struct with Org, Platform, Domain, System, Type, and Instance fields

---

### User Story 2 - Domain Developer Defines Custom Status (Priority: P1)

A domain developer defines status semantics specific to their domain by emitting status as triples. They are not constrained by a framework-defined `EntityStatus` enum. For example, a robotics domain can emit `{Predicate: "robotics.drone.status", Object: "armed"}` while an IoT domain emits `{Predicate: "sensor.connection.status", Object: "online"}`.

**Why this priority**: Status was hardcoded incorrectly in the framework. Domains must own their status semantics for the system to function correctly.

**Independent Test**: Can be tested by creating domain-specific status triples and verifying they are stored, indexed, and queryable without framework interference.

**Acceptance Scenarios**:

1. **Given** a domain processor, **When** I emit a status triple with a domain-specific predicate, **Then** the triple is stored in the entity's Triples collection
2. **Given** an entity with domain status triples, **When** I query by that predicate, **Then** I can retrieve entities matching the status
3. **Given** message processing, **When** an entity is created or updated, **Then** no hardcoded status is automatically applied

---

### User Story 3 - Domain Developer Uses Full StorageReference (Priority: P2)

A domain developer working with stored messages can access the complete StorageReference metadata when available. Instead of just a bare key string, they have access to StorageInstance (which storage holds the data), ContentType (how to parse it), and Size (for informed fetch decisions).

**Why this priority**: The store-once-reference-anywhere pattern requires full metadata to function properly across distributed storage.

**Independent Test**: Can be tested by storing a message via ObjectStore, verifying the EntityState contains a complete StorageReference, and using that reference to retrieve the original message.

**Acceptance Scenarios**:

1. **Given** a Storable message processed into EntityState, **When** I access `state.StorageRef`, **Then** I receive a pointer to StorageReference with StorageInstance, Key, ContentType, and Size fields
2. **Given** an EntityState created from a non-Storable message, **When** I access `state.StorageRef`, **Then** it is nil
3. **Given** a StorageReference, **When** I use it to fetch the original message, **Then** I can retrieve the complete original payload

---

### User Story 4 - Codebase Has No Unused Abstractions (Priority: P2)

The codebase no longer contains EntityClass, EntityRole, EntityStatus, NodeProperties, or Position types. Developers are not confused by unused code when learning the system. The framework is smaller and easier to understand.

**Why this priority**: Technical debt removal improves maintainability and reduces cognitive load for all developers.

**Independent Test**: Can be tested by verifying the specified files and types no longer exist, and all code compiles and tests pass.

**Acceptance Scenarios**:

1. **Given** the refactored codebase, **When** I search for `EntityClass`, `EntityRole`, or `NodeProperties`, **Then** no definitions are found
2. **Given** the refactored codebase, **When** I look for `entity_types.go` or `entity_payload.go`, **Then** these files do not exist
3. **Given** any existing test suite, **When** I run all tests, **Then** they pass with the new structure

---

### Edge Cases

- What happens when an entity ID cannot be parsed? `message.ParseEntityID()` returns an error that callers must handle.
- How does the system handle entities created before the refactor? This is greenfield - no migration needed, break and fix.
- What if a domain doesn't emit any status triples? The entity simply has no status triples, which is valid. Domains decide what triples to emit.
- What if StorageRef is nil when accessing storage? Callers must check for nil before using. The field is optional.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: EntityState MUST have ID as a direct top-level field (not nested in NodeProperties)
- **FR-002**: EntityState MUST use `*message.StorageReference` instead of `ObjectRef string` for storage references
- **FR-003**: EntityState MUST use `message.Type` struct for MessageType (not a bare string)
- **FR-004**: System MUST NOT define or use `EntityClass`, `EntityRole`, or `EntityStatus` types
- **FR-005**: System MUST NOT generate `entity_class` or `entity_role` triples automatically
- **FR-006**: System MUST NOT hardcode any status value when creating entities
- **FR-007**: System MUST delete `message/entity_types.go`, `message/entity_types_test.go`, and `message/entity_payload.go`
- **FR-008**: System MUST delete `NodeProperties` and `Position` structs from `graph/types.go`
- **FR-009**: All code referencing `state.Node.ID` MUST be updated to use `state.ID`
- **FR-010**: All code referencing `state.Node.Type` MUST use `message.ParseEntityID(state.ID).Type`
- **FR-011**: `types/graph/types.go` (duplicate) MUST receive the same updates as `graph/types.go`
- **FR-012**: GraphQL resolvers MUST be updated to use the new EntityState structure

### Key Entities

- **EntityState**: The core entity storage structure containing ID, Triples, StorageRef, MessageType, Version, and UpdatedAt
- **StorageReference**: Points to where original message data is stored (StorageInstance, Key, ContentType, Size)
- **message.Type**: Typed message classification with Domain, Category, and Version fields
- **Triple**: Subject-Predicate-Object semantic fact (unchanged by this refactor)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All existing tests pass after refactor (100% test pass rate)
- **SC-002**: No code references `state.Node.ID`, `state.Node.Type`, `state.Node.Status`, or `state.Node.Position`
- **SC-003**: Files `entity_types.go`, `entity_types_test.go`, and `entity_payload.go` do not exist
- **SC-004**: Types `EntityClass`, `EntityRole`, `EntityStatus`, `NodeProperties`, and `Position` are not defined anywhere
- **SC-005**: No entity creation code sets a hardcoded status value
- **SC-006**: Entities with StorageRef can be used to retrieve original message data
- **SC-007**: Domain developers can emit and query custom status triples without framework interference

## Assumptions

- This is a greenfield project; no backward compatibility or data migration is required
- All consumers of EntityState can be updated in a single refactor pass
- The existing `message.ParseEntityID()` function is sufficient for extracting ID components
- The `types/graph/types.go` file is a duplicate that must stay synchronized with `graph/types.go`
