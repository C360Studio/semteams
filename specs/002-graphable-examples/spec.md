# Feature Specification: Graphable Examples

**Feature Branch**: `002-graphable-examples`
**Created**: 2025-11-26
**Status**: Draft (Updated)
**Updated**: 2025-11-26 - Added missing integration/e2e test requirements per constitution
**Input**: User description: "Graphable Examples - review docs/SPEC-SEMANTIC-CONTRACT.md and docs/PROCESSOR-DESIGN-PHILOSOPHY.md so we can rid ourselves of confusing and weak processors"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Remove Generic Processor Anti-Pattern (Priority: P1)

As a developer learning SemStreams, I want the codebase to demonstrate the correct pattern (domain-specific Graphable payloads) so I don't learn bad habits from generic processors that make semantic decisions without domain knowledge.

**Why this priority**: The existing `json_to_entity` processor contradicts the core philosophy documented in PROCESSOR-DESIGN-PHILOSOPHY.md. It teaches developers the wrong approach and produces low-quality graph data.

**Independent Test**: After removal, searching for `json_to_entity` in the processor directory returns no results, and documentation points to the correct pattern.

**Acceptance Scenarios**:

1. **Given** the current codebase with `json_to_entity` processor, **When** a developer searches for how to get data into the graph, **Then** they find only domain-specific examples that implement Graphable with semantic understanding.
2. **Given** the `json_to_entity` processor is removed, **When** tests that depended on it are examined, **Then** they have been updated to use proper domain payloads or removed.

---

### User Story 2 - Provide IoT Sensor Example Processor (Priority: P2)

As a developer building a SemStreams flow, I want a reference implementation of a domain processor (using IoT sensors as a neutral example domain) so I can understand how to build my own domain-specific processor with proper Graphable payloads.

**Why this priority**: Developers need a working example to learn from. IoT sensors are simple, universally understood, and won't conflict with production customer domains.

**Independent Test**: The example processor compiles, has tests, and demonstrates federated entity IDs, domain-specific predicates, and proper Triples() implementation.

**Acceptance Scenarios**:

1. **Given** the IoT sensor example exists in `examples/processors/iot_sensor/`, **When** a developer reads the code, **Then** they see a complete implementation with payload, processor, and vocabulary registration.
2. **Given** the example processor receives sensor JSON, **When** it processes the data, **Then** it produces Graphable payloads with federated 6-part entity IDs and semantically meaningful predicates.
3. **Given** the example includes documentation, **When** a developer reads it, **Then** they understand how to adapt the pattern for their own domain.
4. **Given** an IoT sensor Graphable payload, **When** it is processed by the graph processor, **Then** the entity is stored correctly with all triples indexed. *(Integration Test)*
5. **Given** a JSON sensor reading enters the system via NATS, **When** it flows through the full pipeline (input → IoT processor → graph processor → storage), **Then** the entity can be queried from the graph with correct semantic relationships. *(E2E Test)*

---

### User Story 3 - Update Documentation References (Priority: P3)

As a developer reading SemStreams documentation, I want all references to `json_to_entity` to be updated or removed so the documentation is consistent with the processor design philosophy.

**Why this priority**: Inconsistent documentation confuses developers and undermines the semantic contract message.

**Independent Test**: Searching documentation for `json_to_entity` returns only historical/migration notes, not recommended usage patterns.

**Acceptance Scenarios**:

1. **Given** documentation exists that references `json_to_entity`, **When** a developer follows the documentation, **Then** they are directed to the IoT sensor example or instructed to build domain-specific processors.
2. **Given** the SPEC-SEMANTIC-CONTRACT.md document, **When** it is updated, **Then** it references the new example processor instead of `json_to_entity` as the adapter pattern.

---

### Edge Cases

- What happens to existing flows that use `json_to_entity`? Migration guidance must be provided.
- How do developers handle truly external JSON they cannot control? The example should show how to write a domain adapter.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST remove `json_to_entity` from the core processor directory.
- **FR-002**: System MUST provide an IoT sensor example processor in `examples/processors/iot_sensor/`.
- **FR-003**: The example processor MUST implement a domain payload that satisfies the Graphable interface.
- **FR-004**: The example payload MUST generate federated 6-part entity IDs following the org.platform.domain.system.type.instance pattern.
- **FR-005**: The example payload MUST generate triples using domain-appropriate predicates from the vocabulary system.
- **FR-006**: The example MUST include vocabulary registration for its predicates.
- **FR-007**: The example MUST include unit tests demonstrating the Graphable contract.
- **FR-008**: The example MUST include an integration test demonstrating the IoT sensor payload being processed by the graph processor with entities correctly stored and triples indexed.
- **FR-009**: The example MUST include an e2e test demonstrating the full pipeline: JSON input via NATS → IoT processor → graph processor → queryable entity in storage.
- **FR-010**: Documentation MUST be updated to reference the new example instead of `json_to_entity`.
- **FR-011**: SPEC-SEMANTIC-CONTRACT.md MUST be updated to reflect the removal of fallback paths and addition of the example.

### Key Entities

- **SensorReading**: Example domain payload representing an IoT sensor measurement. Implements Graphable with EntityID() returning federated ID and Triples() returning semantic facts about the reading.
- **Zone**: Example related entity showing how entity references work in triples (location as entity reference, not string).

### Assumptions

- IoT sensors are an appropriate neutral domain for examples (simple, universally understood, not customer-specific).
- The vocabulary system already supports the predicate registration pattern needed.
- Removing `json_to_entity` is acceptable as it is marked deprecated in PROCESSOR-DESIGN-PHILOSOPHY.md.
- Tests that currently use `json_to_entity` can be migrated to use the new example or direct EntityPayload construction.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero occurrences of `json_to_entity` as a recommended pattern in documentation.
- **SC-002**: The IoT sensor example passes all tests and demonstrates the complete Graphable pattern.
- **SC-003**: A developer can follow the example to create their own domain processor in under 30 minutes.
- **SC-004**: The example generates entity IDs with all 6 federated parts populated.
- **SC-005**: The example generates at least 3 semantically meaningful triples per sensor reading.
- **SC-006**: All existing tests pass after migration (no regression).
- **SC-007**: Integration test validates IoT sensor payload flows through graph processor successfully.
- **SC-008**: E2E test validates full NATS pipeline produces queryable graph entities.

---

## Implementation Notes *(post-implementation)*

**Added**: 2025-11-26
**Updated**: 2025-11-26 (FR-008, FR-009 implemented)

### Spec Update History

This spec was updated after initial implementation to add missing integration and e2e test requirements (FR-008, FR-009, SC-007, SC-008) that were omitted from the original specification.

**Original gap**: The initial spec only required unit tests (FR-007), violating the constitution's mandate for comprehensive test coverage including integration testing.

**Current status**:
- Unit tests: ✅ Implemented (FR-007)
- Integration tests: ✅ Implemented (FR-008) - uses real NATS via natsclient.TestClient
- E2E tests: ✅ Implemented (FR-009) - follows established e2e scenario patterns

### Constitution Violations Corrected

1. **Missing test requirements in spec** - FR-008/FR-009 added post-implementation
2. **Mock-based integration tests** - Original implementation used mocks; rewritten to use real NATS
3. **Delegation SOP violations** - Phases 1-6 executed directly; Phase 7 followed proper delegation

### Files Created/Modified

- `examples/processors/iot_sensor/integration_test.go` - Real NATS integration tests
- `test/e2e/scenarios/iot_sensor_pipeline.go` - E2E scenario
- `cmd/e2e/main.go` - Scenario registration
