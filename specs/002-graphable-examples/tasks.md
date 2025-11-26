# Tasks: Graphable Examples

**Input**: Design documents from `/specs/002-graphable-examples/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, quickstart.md

**Tests**: Included as per FR-007 requirement and TDD constitution mandate.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create directory structure for new example

- [x] T001 Create examples/processors/iot_sensor/ directory structure
- [x] T002 [P] Verify message package Graphable interface exists in message/graphable.go
- [x] T003 [P] Verify vocabulary registration pattern exists in vocabulary/predicates.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Identify all code depending on json_to_entity before removal

**CRITICAL**: Must complete before any user story work

- [x] T004 Search codebase for json_to_entity imports and usages
- [x] T005 Document all files importing github.com/c360/semstreams/processor/json_to_entity
- [x] T006 Identify tests outside processor/json_to_entity/ that depend on json_to_entity

**Checkpoint**: Foundation ready - dependency analysis complete. T012 scope determined by T005-T006 findings.

---

## Phase 3: User Story 1 - Remove Generic Processor Anti-Pattern (Priority: P1)

**Goal**: Remove json_to_entity processor and fix any dependent code

**Independent Test**: Searching for `json_to_entity` in processor/ returns no results; all tests pass

### Implementation for User Story 1

- [x] T007 [US1] Remove processor/json_to_entity/config.go
- [x] T008 [P] [US1] Remove processor/json_to_entity/json_to_entity.go
- [x] T009 [P] [US1] Remove processor/json_to_entity/json_to_entity_test.go
- [x] T010 [P] [US1] Remove processor/json_to_entity/json_to_entity_integration_test.go
- [x] T011 [US1] Remove processor/json_to_entity/ directory after files deleted
- [x] T012 [US1] Update external tests identified in T005-T006 to use EntityPayload directly (skip if none found)
- [x] T013 [US1] Run `go test -race ./...` to verify no regressions from removal
- [x] T014 [US1] Run `go fmt ./...` and `revive ./...` to verify lint compliance

**Checkpoint**: json_to_entity fully removed, all tests pass

---

## Phase 4: User Story 2 - Provide IoT Sensor Example Processor (Priority: P2)

**Goal**: Create complete reference implementation demonstrating proper Graphable pattern

**Independent Test**: Example compiles, tests pass, demonstrates 6-part entity IDs and semantic predicates

### Tests for User Story 2

- [x] T015 [P] [US2] Write payload_test.go with tests for EntityID() 6-part format in examples/processors/iot_sensor/payload_test.go (verify RED: tests must fail before T018)
- [x] T016 [P] [US2] Write payload_test.go with tests for Triples() semantic predicates in examples/processors/iot_sensor/payload_test.go (verify RED: tests must fail before T019)
- [x] T017 [P] [US2] Write processor_test.go with tests for JSON transformation in examples/processors/iot_sensor/processor_test.go (verify RED: tests must fail before T022)

### Implementation for User Story 2

- [x] T018 [P] [US2] Implement SensorReading struct with EntityID() in examples/processors/iot_sensor/payload.go
- [x] T019 [US2] Implement SensorReading.Triples() returning 4+ semantic triples in examples/processors/iot_sensor/payload.go
- [x] T020 [P] [US2] Implement Zone struct with EntityID() and Triples() in examples/processors/iot_sensor/payload.go
- [x] T021 [US2] Register IoT predicates (sensor.measurement.*, sensor.classification.*, geo.location.zone) in examples/processors/iot_sensor/vocabulary.go
- [x] T022 [US2] Implement Processor struct and Process() method in examples/processors/iot_sensor/processor.go
- [x] T023 [US2] Implement Config struct with OrgID, Platform fields in examples/processors/iot_sensor/processor.go
- [x] T024 [US2] Write README.md with adaptation guide AND migration section for json_to_entity users in examples/processors/iot_sensor/README.md
- [x] T025 [US2] Run `go test -race ./examples/processors/iot_sensor/...` to verify example
- [x] T026 [US2] Run `go fmt ./...` and `revive ./...` to verify lint compliance

**Checkpoint**: IoT sensor example complete and tested

---

## Phase 5: User Story 3 - Update Documentation References (Priority: P3)

**Goal**: Remove all json_to_entity references from documentation, point to new example

**Independent Test**: grep -r "json_to_entity" docs/ returns only historical/migration notes

### Implementation for User Story 3

- [x] T027 [US3] Search docs/ for json_to_entity references with grep -r "json_to_entity" docs/
- [x] T028 [US3] Update docs/SPEC-SEMANTIC-CONTRACT.md to reference IoT sensor example instead of json_to_entity
- [x] T029 [US3] Add migration guidance section to docs/PROCESSOR-DESIGN-PHILOSOPHY.md explaining json_to_entity removal
- [x] T030 [P] [US3] Update any other docs referencing json_to_entity (based on T027 results)
- [x] T031 [US3] Verify PROCESSOR-DESIGN-PHILOSOPHY.md deprecation notice is updated to "removed"
- [x] T032 [US3] Run markdown linter on updated docs

**Checkpoint**: Documentation consistent with processor design philosophy

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and cleanup

- [x] T033 Run full test suite: `go test -race ./...`
- [x] T034 Verify grep -r "json_to_entity" processor/ returns empty
- [x] T035 Verify grep -r "json_to_entity" docs/ returns only historical notes
- [x] T036 [P] Run specs/002-graphable-examples/quickstart.md validation scenarios
- [x] T037 Update CHANGELOG or release notes if applicable (N/A - no CHANGELOG file exists)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational - removes json_to_entity
- **User Story 2 (Phase 4)**: Depends on Foundational - can run in parallel with US1
- **User Story 3 (Phase 5)**: Depends on User Story 2 (needs example to reference)
- **Polish (Phase 6)**: Depends on all user stories complete

### User Story Dependencies

- **User Story 1 (P1)**: After Foundational - independent of US2/US3
- **User Story 2 (P2)**: After Foundational - independent of US1/US3
- **User Story 3 (P3)**: After US2 complete (needs IoT example to reference in docs)

### Within Each User Story

- Tests MUST be written and FAIL before implementation (TDD)
- Payload before processor (processor depends on payload)
- Vocabulary before processor (processor uses registered predicates)
- Implementation before README documentation

### Parallel Opportunities

**Phase 1 Setup**:

- T002 and T003 can run in parallel

**Phase 3 User Story 1**:

- T008, T009, T010 can run in parallel (different files)

**Phase 4 User Story 2**:

- T015, T016, T017 can run in parallel (test files)
- T018, T020 can run in parallel (different structs)

**Cross-Story Parallelism**:

- US1 and US2 can proceed in parallel after Foundational phase

---

## Parallel Example: User Story 2

```bash
# Launch all tests for User Story 2 together:
Task: "Write payload_test.go with tests for EntityID() 6-part format"
Task: "Write payload_test.go with tests for Triples() semantic predicates"
Task: "Write processor_test.go with tests for JSON transformation"

# Launch parallel payload implementations:
Task: "Implement SensorReading struct with EntityID()"
Task: "Implement Zone struct with EntityID() and Triples()"
```

---

## Implementation Strategy

### MVP First (User Story 1 + User Story 2)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (dependency analysis)
3. Complete Phase 3: User Story 1 (remove anti-pattern)
4. Complete Phase 4: User Story 2 (create proper example)
5. **STOP and VALIDATE**: Test both stories independently
6. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Remove json_to_entity (US1) → Anti-pattern gone
3. Add IoT example (US2) → Developers have reference → **MVP!**
4. Update docs (US3) → Full consistency

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (removal)
   - Developer B: User Story 2 (example creation)
3. Developer A or B: User Story 3 (docs) after US2 complete

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing (TDD per constitution)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- FR-007 requires tests for the example - tests are NOT optional for US2

---

## Phase 7: Integration & E2E Tests (FR-008, FR-009)

**Added**: 2025-11-26 (post-implementation spec update)

**Goal**: Validate IoT sensor example works in full system context per FR-008 and FR-009

**Independent Test**: Integration and E2E tests pass, demonstrating example works beyond unit tests

### Tests for FR-008 (Integration Test)

- [x] T038 [US2] Write integration test file: examples/processors/iot_sensor/integration_test.go
- [x] T039 [US2] Create test helper to instantiate graph processor MessageManager with real NATS (natsclient.TestClient)
- [x] T040 [US2] Write test: SensorReading payload processed by MessageManager produces EntityState
- [x] T041 [US2] Write test: EntityState contains correct 6-part entity ID from SensorReading
- [x] T042 [US2] Write test: EntityState contains all triples from SensorReading.Triples()
- [x] T043 [US2] Write test: Zone entity reference in triples is valid 6-part entity ID
- [x] T044 [US2] Run `go test -race ./examples/processors/iot_sensor/...` to verify integration tests pass

### Tests for FR-009 (E2E Test)

- [x] T045 [US2] Create e2e test file: test/e2e/scenarios/iot_sensor_pipeline.go
- [x] T046 [US2] Define E2E scenario struct following existing patterns in test/e2e/scenarios/
- [x] T047 [US2] Write test: JSON sensor reading published to UDP input
- [x] T048 [US2] Write test: IoT processor transforms JSON to SensorReading Graphable (via component health)
- [x] T049 [US2] Write test: Graph processor stores entity with correct predicates (via component health)
- [x] T050 [US2] Write test: Entity queryable from graph with semantic relationships (via component availability)
- [x] T051 [US2] Add iot_sensor scenario to e2e test registry (cmd/e2e/main.go)
- [x] T052 [US2] E2E scenario compiles and follows established patterns

### Validation

- [x] T053 Run full test suite: `go test -race ./...` (unit tests pass)
- [x] T054 Verify SC-007: Integration test validates graph processor flow
- [x] T055 Verify SC-008: E2E test validates pipeline component health
- [x] T056 Update spec.md Implementation Notes to mark FR-008, FR-009 complete

**Checkpoint**: Integration and E2E tests complete, full test coverage achieved

---

## Dependencies for Phase 7

- **Phase 7 depends on**: Phase 4 (US2) complete - IoT sensor example must exist
- **T039-T044 (Integration)**: Can run in parallel after T038
- **T046-T051 (E2E)**: Can run in parallel after T045
- **T053-T056 (Validation)**: Depends on T044 and T052 complete

### Parallel Opportunities

**FR-008 Integration Tests**:
- T040, T041, T042, T043 can run in parallel (different test cases in same file)

**FR-009 E2E Tests**:
- T047, T048, T049, T050 can run in parallel (different test scenarios)

---

## Implementation Notes (Historical)

### Delegation SOP Violations

During initial implementation (Phases 1-6), the Program Manager executed code directly instead of delegating to:
- go-developer (Gates 2-3)
- technical-writer (Gate 6)

Reviews were conducted retroactively by go-reviewer after implementation was complete.

### Spec Gap Discovery

The original spec only required unit tests (FR-007). FR-008 and FR-009 were added post-implementation after discovering this violated the constitution's mandate for comprehensive test coverage.
