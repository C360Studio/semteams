# Feature Specification: Remove Legacy RDF Predicates

**Feature Branch**: `001-predicate-notation`
**Created**: 2025-11-26
**Status**: Draft
**Input**: User description: "Fix predicate Notation - see docs/TODO-PREDICATE-NOTATION-CONSISTENCY.md"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Remove Unnecessary Type/Class Triples (Priority: P1)

As a developer, I want to remove the legacy `rdf:type` and `rdf:class` predicates from EntityPayload because they are redundant (the entity's type and class are already stored in the EntityPayload struct fields) and they use colon notation which breaks the vocabulary registry pattern.

**Why this priority**: The `rdf:type` and `rdf:class` triples add no value - the entity's Type and Class are already available as struct fields. Removing them eliminates confusion and simplifies the triple output.

**Independent Test**: Create an EntityPayload, call Triples(), and verify no `rdf:type` or `rdf:class` triples are present.

**Acceptance Scenarios**:

1. **Given** an EntityPayload with Type="robotics.sensor" and properties, **When** Triples() is called, **Then** the result contains only property triples like `robotics.sensor.temperature`, not `rdf:type` triples.
2. **Given** the migration is complete, **When** searching for `rdf:type` or `rdf:class` in the codebase, **Then** zero occurrences are found.

---

### User Story 2 - Documentation Consistency (Priority: P2)

As a developer reading documentation, I want code examples to use real predicates like `sensor.temperature.celsius` so I understand the correct convention without confusion.

**Why this priority**: Documentation currently shows RDF-style colon notation which doesn't match the actual vocabulary system.

**Independent Test**: Review graphable.go and verify examples use dotted notation predicates.

**Acceptance Scenarios**:

1. **Given** documentation examples exist in graphable.go, **When** a developer reads them, **Then** all predicate examples use dotted notation like `sensor.temperature.celsius`.
2. **Given** the migration is complete, **When** searching for colon-style predicates in Go files, **Then** no occurrences are found.

---

### Edge Cases

- Tests that expect `rdf:type` triples will need to be updated or removed.
- No data migration needed - predicates are code literals only.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST remove `rdf:type` triple generation from EntityPayload.Triples().
- **FR-002**: System MUST remove `rdf:class` triple generation from EntityPayload.Triples().
- **FR-003**: System MUST update all tests to not expect `rdf:type` or `rdf:class` triples.
- **FR-004**: System MUST update documentation examples to use real predicates like `sensor.temperature.celsius`.
- **FR-005**: System MUST have zero colon-notation predicates in Go files after migration.

### Key Entities

- **Predicate**: A relationship identifier in triples. Must follow `domain.category.property` pattern (e.g., `sensor.temperature.celsius`, `geo.location.latitude`).
- **Triple**: A subject-predicate-object statement. Property triples use predicates derived from entity type + property key.

### Assumptions

- Entity type and class are already stored in EntityPayload.Type and EntityPayload.Class fields - separate triples are redundant.
- No external systems depend on `rdf:type` or `rdf:class` predicates.
- This is a code-only change - no persistent data uses these predicates.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero occurrences of `rdf:type` or `rdf:class` in Go code, tests, or documentation.
- **SC-002**: EntityPayload.Triples() does not generate `rdf:type` or `rdf:class` triples.
- **SC-003**: All existing tests pass after removing type/class triple expectations.
- **SC-004**: Documentation examples use real predicates like `sensor.temperature.celsius`.
