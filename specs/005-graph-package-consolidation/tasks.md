# Tasks: Graph Package Consolidation

**Input**: Design documents from `/specs/005-graph-package-consolidation/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: This is a refactoring feature - existing tests must pass after each phase. No new test creation required unless existing tests break. Per constitution, TDD applies to new implementation; refactoring preserves existing test coverage as the safety net.

**Organization**: Tasks are grouped by user story from spec.md. Each story is a migration phase that can be independently verified. Each story ends with a **[→go-reviewer]** handoff task per Gate 3→4 requirements.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Go module root**: Repository root (`/Users/coby/Code/c360/semstreams/`)
- **Target consolidations**:
  - `types/graph/` → DELETED (use `graph/`)
  - `message/graphable.go` → `graph/graphable.go`
  - `graph/federation.go` → DELETED
  - `pkg/graphclustering/` → `processor/graph/clustering/`
  - `pkg/graphinterfaces/` → DELETED
  - `pkg/embedding/` → `processor/graph/embedding/`

---

## Phase 1: Setup (Verification)

**Purpose**: Verify current state matches research.md assumptions before making changes

- [x] T001 Verify `types/graph/` directory exists and count files (expect 9 imports)
- [x] T002 Verify `message/graphable.go` exists with Graphable interface
- [x] T003 Verify `graph/federation.go` exists with no external consumers
- [x] T004 Verify `pkg/graphclustering/` exists and count external consumers (expect 1)
- [x] T005 Verify `pkg/graphinterfaces/` exists and count Community getter methods (expect 10)
- [x] T006 Verify `pkg/embedding/` exists and count external consumers (expect 2 in indexmanager)
- [x] T007 Run `go build ./...` to establish baseline (must pass)

**Checkpoint**: Current state verified - migration can begin

---

## Phase 2: User Story 1 - Eliminate types/graph Duplicate (Priority: P1) 🎯 MVP

**Goal**: Migrate 9 files from `types/graph/` imports to `graph/` package, then delete `types/graph/`

**Independent Test**: `grep -r "types/graph" --include="*.go" .` returns no results AND `go build ./...` succeeds

### Implementation for User Story 1

- [x] T008 [P] [US1] Update import in `processor/graph/cleanup.go:11` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T009 [P] [US1] Update import in `processor/graph/cleanup_test.go:10` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T010 [P] [US1] Update import in `processor/rule/kv_test_helpers.go:10` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T011 [P] [US1] Update import in `processor/rule/entity_watcher.go:12` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T012 [P] [US1] Update import in `processor/rule/rule_integration_test.go:23` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T013 [P] [US1] Update import in `processor/rule/test_rule_factory.go:10` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T014 [P] [US1] Update import in `processor/rule/expression/evaluator_test.go:11` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T015 [P] [US1] Update import in `processor/rule/expression/types.go:7` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T016 [P] [US1] Update import in `processor/rule/expression/evaluator.go:9` from `gtypes "github.com/c360/semstreams/types/graph"` to `gtypes "github.com/c360/semstreams/graph"`
- [x] T017 [US1] Run `go build ./...` to verify no compilation errors
- [x] T018 [US1] Delete `types/graph/` directory entirely
- [x] T019 [US1] Run `go build ./...` to confirm deletion successful
- [x] T020 [US1] Run `go test -race ./processor/graph/... ./processor/rule/...` to verify tests pass
- [x] T020r [US1] **[→go-reviewer]** Code review for User Story 1 changes

**Checkpoint**: User Story 1 complete - single authoritative graph types package

---

## Phase 3: User Story 2 - Move Graphable Interface (Priority: P1)

**Goal**: Move `message/graphable.go` to `graph/graphable.go` and update all references

**Independent Test**: `grep -r "message.Graphable" --include="*.go" .` returns no results (except test data strings) AND `go build ./...` succeeds

### Implementation for User Story 2

- [x] T021 [US2] Copy `message/graphable.go` to `graph/graphable.go` with package declaration change
- [x] T022 [US2] Update package declaration in `graph/graphable.go` - change `package message` to `package graph`
- [x] T022a [US2] Verify `graph/graphable.go` imports `message.Triple` correctly (Graphable interface returns `[]Triple`)
- [x] T023 [P] [US2] Update `processor/graph/messagemanager/processor.go:175` - change type assertion from `msg.(message.Graphable)` to `msg.(graph.Graphable)`
- [x] T024 [P] [US2] Update `processor/graph/messagemanager/processor.go:186` - change parameter type from `message.Graphable` to `graph.Graphable`
- [x] T025 [P] [US2] Update `storage/objectstore/stored_message.go:47` - change parameter type from `message.Graphable` to `graph.Graphable`
- [x] T026 [P] [US2] Update `storage/objectstore/stored_message.go` - update interface implementation comments (lines 57, 62)
- [x] T027 [P] [US2] Update `examples/processors/iot_sensor/processor_test.go:98` - change interface assertion to `graph.Graphable`
- [x] T028 [P] [US2] Update `examples/processors/iot_sensor/payload_test.go:224-225` - change interface assertions to `graph.Graphable`
- [x] T029 [US2] Run `go build ./...` to verify no compilation errors
- [x] T030 [US2] Delete `message/graphable.go` after all references updated
- [x] T031 [US2] Run `go test -race ./processor/graph/messagemanager/... ./storage/objectstore/... ./examples/...` to verify tests pass
- [x] T031r [US2] **[→go-reviewer]** Code review for User Story 2 changes

**Checkpoint**: User Story 2 complete - Graphable interface in graph/ package

---

## Phase 4: User Story 3 - Delete Federation Redundancy (Priority: P2)

**Goal**: Delete `graph/federation.go` and `graph/federation_iri_test.go` - federation info already in EntityID

**Independent Test**: `graph/federation.go` does not exist AND `go build ./...` succeeds

### Implementation for User Story 3

- [x] T032 [US3] Verify no production code references `BuildGlobalID`, `FederatedEntity`, `EnrichEntityState`, `GetFederationInfo` (only test file)
- [x] T033 [US3] Delete `graph/federation_iri_test.go` (test file for deleted code)
- [x] T034 [US3] Delete `graph/federation.go`
- [x] T035 [US3] Run `go build ./...` to verify no compilation errors
- [x] T036 [US3] Run `go test -race ./graph/...` to verify tests pass
- [x] T036r [US3] **[→go-reviewer]** Code review for User Story 3 changes

**Checkpoint**: User Story 3 complete - no duplicate federation code

---

## Phase 5: User Story 4 - Relocate graphclustering (Priority: P2)

**Goal**: Move `pkg/graphclustering/` to `processor/graph/clustering/`

**Independent Test**: `grep -r "pkg/graphclustering" --include="*.go" .` returns no results AND `go build ./...` succeeds

### Implementation for User Story 4

- [x] T037 [US4] Create `processor/graph/clustering/` directory
- [x] T038 [US4] Move all files from `pkg/graphclustering/` to `processor/graph/clustering/`
- [x] T039 [US4] Update package declaration in all moved files: `package graphclustering` → `package clustering`
- [x] T040 [P] [US4] Update import in `processor/graph/graphrag_integration_test.go` from `pkg/graphclustering` to `processor/graph/clustering`
- [x] T041 [US4] Update any internal imports within clustering package files
- [x] T042 [US4] Run `go build ./...` to verify no import cycles
- [x] T043 [US4] Delete `pkg/graphclustering/` directory
- [x] T044 [US4] Run `go test -race ./processor/graph/...` to verify tests pass
- [x] T044r [US4] **[→go-reviewer]** Code review for User Story 4 changes

**Checkpoint**: User Story 4 complete - graphclustering inside processor/graph/

---

## Phase 6: User Story 5 - Delete graphinterfaces Hack (Priority: P2)

**Goal**: Delete `pkg/graphinterfaces/` and remove Community getter methods - use direct field access

**Independent Test**: `grep -r "pkg/graphinterfaces" --include="*.go" .` returns no results AND `grep -r "\.Get[A-Z].*\(\)" --include="*.go" . | grep -i community` returns no results AND `go build ./...` succeeds

**Dependency**: MUST complete User Story 4 first (import cycle resolution)

### Implementation for User Story 5

#### Remove getter method calls (update to direct field access):

- [x] T045 [P] [US5] Update `processor/graph/querymanager/graphrag_search.go` - replace ~12 getter calls with direct field access (GetID→ID, GetLevel→Level, etc.)
- [x] T046 [P] [US5] Update `processor/graph/querymanager/graphrag_search_test.go` - replace ~8 getter calls with direct field access
- [x] T047 [P] [US5] Update `gateway/graphql/base_resolver.go` - replace ~9 getter calls with direct field access

#### Update interface usage to concrete type:

- [x] T048 [P] [US5] Update `processor/graph/querymanager/interface.go` - change `graphinterfaces.Community` to `*clustering.Community`
- [x] T049 [P] [US5] Update `gateway/graphql/graphql_test.go` - change `graphinterfaces.Community` to `*clustering.Community`

#### Remove getters from Community struct:

- [x] T050 [US5] Remove 10 getter methods from `processor/graph/clustering/types.go`: GetID, GetLevel, GetMembers, GetParentID, GetKeywords, GetRepEntities, GetStatisticalSummary, GetLLMSummary, GetSummaryStatus, GetMetadata
- [x] T051 [US5] Run `go build ./...` to verify no compilation errors
- [x] T052 [US5] Delete `pkg/graphinterfaces/` directory
- [x] T053 [US5] Run `go test -race ./processor/graph/... ./gateway/graphql/...` to verify tests pass
- [x] T053r [US5] **[→go-reviewer]** Code review for User Story 5 changes

**Checkpoint**: User Story 5 complete - no Java-style getter anti-patterns

---

## Phase 7: User Story 6 - Relocate embedding (Priority: P2)

**Goal**: Move `pkg/embedding/` to `processor/graph/embedding/`

**Independent Test**: `grep -r "pkg/embedding" --include="*.go" .` returns no results AND `go build ./...` succeeds

### Implementation for User Story 6

- [x] T054 [US6] Create `processor/graph/embedding/` directory
- [x] T055 [US6] Move all 7 files from `pkg/embedding/` to `processor/graph/embedding/` (embedder.go, bm25_embedder.go, http_embedder.go, cache.go, storage.go, vector.go, worker.go)
- [x] T056 [P] [US6] Update import in `processor/graph/indexmanager/semantic.go` from `pkg/embedding` to `processor/graph/embedding`
- [x] T057 [P] [US6] Update import in `processor/graph/indexmanager/manager.go` from `pkg/embedding` to `processor/graph/embedding`
- [x] T058 [US6] Run `go build ./...` to verify no compilation errors
- [x] T059 [US6] Delete `pkg/embedding/` directory
- [x] T060 [US6] Run `go test -race ./processor/graph/indexmanager/...` to verify tests pass
- [x] T060r [US6] **[→go-reviewer]** Code review for User Story 6 changes

**Checkpoint**: User Story 6 complete - embedding inside processor/graph/

---

## Phase 8: User Story 7 - Document Package Ownership (Priority: P3)

**Goal**: Update package READMEs with clear ownership documentation

**Independent Test**: README files exist in graph/, message/, processor/graph/ with ownership scope documentation

### Implementation for User Story 7

- [x] T061 [P] [US7] Create/update `graph/README.md` - document ownership: Graphable interface, EntityState, graph helpers
- [x] T062 [P] [US7] Create/update `message/README.md` - document ownership: EntityID, Triple, FederationMeta (transport primitives)
- [x] T063 [P] [US7] Create/update `processor/graph/README.md` - document ownership: clustering/, embedding/, querymanager/, indexmanager/, datamanager/, messagemanager/

**Checkpoint**: User Story 7 complete - clear package ownership documentation

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Final verification and cleanup

- [x] T064 Run `go build ./...` for full compilation check
- [x] T065 Run `go test -race ./...` for full test suite
- [x] T066 Verify success criteria from spec.md:
  - SC-001: Zero files import from `types/graph/`
  - SC-002: `types/graph/` directory does not exist
  - SC-003: `message/graphable.go` does not exist; `graph/graphable.go` exists
  - SC-004: Zero files reference `message.Graphable`
  - SC-005: `graph/federation.go` does not exist
  - SC-006: `pkg/graphclustering/` does not exist; `processor/graph/clustering/` exists
  - SC-007: `pkg/graphinterfaces/` does not exist
  - SC-008: Community struct has zero getter methods
  - SC-009: `pkg/embedding/` does not exist; `processor/graph/embedding/` exists
  - SC-010: `go build ./...` exit code 0
  - SC-011: `go test -race ./...` passes
  - SC-012: Package READMEs document ownership
- [x] T067 Update quickstart.md if any migration patterns changed (no changes needed - patterns still valid)
- [x] T068 Final code review with go-reviewer agent (APPROVED after 2 fix cycles)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - verification only
- **User Story 1 (Phase 2)**: Can start after Setup
- **User Story 2 (Phase 3)**: Can start after Setup (parallel with US1)
- **User Story 3 (Phase 4)**: Can start after Setup (parallel with US1, US2)
- **User Story 4 (Phase 5)**: Can start after Setup (parallel with US1-3)
- **User Story 5 (Phase 6)**: **MUST wait for User Story 4** (import cycle resolution)
- **User Story 6 (Phase 7)**: Can start after Setup (parallel with US1-4)
- **User Story 7 (Phase 8)**: Can start after all code changes (US1-6)
- **Polish (Phase 9)**: Depends on all user stories being complete

### Critical Ordering

```text
Phase 1 (Setup) ──┬──→ Phase 2 (US1: types/graph)     ─┐
                  ├──→ Phase 3 (US2: Graphable)        ├──→ Phase 8 (US7: Docs) ──→ Phase 9 (Polish)
                  ├──→ Phase 4 (US3: federation)       │
                  ├──→ Phase 5 (US4: graphclustering) ─┴──→ Phase 6 (US5: graphinterfaces)
                  └──→ Phase 7 (US6: embedding)        ─────────────────────┘
```

**CRITICAL**: User Story 5 (graphinterfaces) CANNOT start until User Story 4 (graphclustering) is complete.

### Within Each User Story

1. Make code changes (import updates, file moves)
2. Run `go build ./...` to verify compilation
3. Delete old files/directories
4. Run `go test -race` for affected packages

### Parallel Opportunities

- T008-T016 (US1 import updates) can all run in parallel
- T023-T028 (US2 reference updates) can all run in parallel
- T045-T049 (US5 getter→field updates) can all run in parallel
- T056-T057 (US6 import updates) can all run in parallel
- T061-T063 (US7 README updates) can all run in parallel
- US1, US2, US3, US4, US6 can all run in parallel after Setup

---

## Parallel Example: User Story 1

```bash
# Launch all import updates together (9 files, no dependencies):
Task T008: "Update import in processor/graph/cleanup.go:11"
Task T009: "Update import in processor/graph/cleanup_test.go:10"
Task T010: "Update import in processor/rule/kv_test_helpers.go:10"
Task T011: "Update import in processor/rule/entity_watcher.go:12"
Task T012: "Update import in processor/rule/rule_integration_test.go:23"
Task T013: "Update import in processor/rule/test_rule_factory.go:10"
Task T014: "Update import in processor/rule/expression/evaluator_test.go:11"
Task T015: "Update import in processor/rule/expression/types.go:7"
Task T016: "Update import in processor/rule/expression/evaluator.go:9"

# Then sequentially:
Task T017: "Run go build ./..."
Task T018: "Delete types/graph/ directory"
Task T019: "Run go build ./..."
Task T020: "Run go test -race"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (verify current state)
2. Complete Phase 2: User Story 1 (eliminate types/graph)
3. **STOP and VALIDATE**: `go build ./...` and `go test -race ./...`
4. Commit with clear message

### Incremental Delivery

1. Complete Setup → State verified
2. Complete US1 → types/graph eliminated → Commit
3. Complete US2 → Graphable moved → Commit
4. Complete US3 → federation deleted → Commit
5. Complete US4 → graphclustering moved → Commit
6. Complete US5 → graphinterfaces deleted (depends on US4) → Commit
7. Complete US6 → embedding moved → Commit
8. Complete US7 → Documentation → Commit
9. Complete Polish → Final verification → Ready for merge

### Risk Mitigation

- Run `go build ./...` after EVERY change
- Run tests after EVERY phase
- Commit after EVERY user story
- If any phase fails, rollback to last commit

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and verifiable
- Verify `go build ./...` passes before deleting any files
- Commit after each user story completion
- Stop at any checkpoint to validate independently
- **CRITICAL**: US5 depends on US4 - do not parallelize these two
