# Tasks: Remove Legacy RDF Predicates

**Input**: Design documents from `/specs/001-predicate-notation/`
**Scope**: Remove `rdf:type` and `rdf:class` triple generation, update tests and docs

---

## Phase 1: Setup

- [x] T001 Verify baseline: `go test -race ./message/... ./processor/json_to_entity/...`

**Checkpoint**: All existing tests pass before modification

---

## Phase 2: Update Tests (RED phase)

> **TDD**: Update test expectations FIRST - tests should FAIL until implementation changes

- [x] T002 [P] Update `processor/json_to_entity/json_to_entity_test.go` - remove `rdf:type` and `rdf:class` expectations
- [x] T003 [P] Update `processor/json_to_entity/json_to_entity_integration_test.go` - remove `rdf:type` and `rdf:class` expectations

**Checkpoint**: Tests fail because code still generates rdf:type/rdf:class triples

---

## Phase 3: Implementation (GREEN phase)

- [x] T004 Remove `rdf:type` triple generation from `message/entity_payload.go` (lines 124-132)
- [x] T005 Remove `rdf:class` triple generation from `message/entity_payload.go` (lines 134-144)
- [x] T006 Run tests: `go test -race ./message/... ./processor/json_to_entity/...`

**Checkpoint**: Tests pass - no more rdf:type/rdf:class triples generated

---

## Phase 4: Documentation

- [x] T007 Update doc example in `message/graphable.go` (line 45) to use `geo.location.latitude`
- [x] T008 Verify no remaining `rdf:type` or `rdf:class` in Go files

---

## Phase 5: Validation

- [x] T009 Run `go fmt ./...`
- [x] T010 Run `revive ./...`
- [x] T011 Run full test suite: `go test -race ./...`

---

## Parallel Opportunities

- T002 and T003 can run in parallel (different test files)
- T009, T010, T011 can run in parallel (different tools)

## Execution Order

1. T001 (baseline)
2. T002, T003 in parallel (tests)
3. T004, T005 sequential (same file)
4. T006 (verify)
5. T007, T008 in parallel (docs)
6. T009, T010, T011 in parallel (validation)

**Estimated time**: ~15 minutes
