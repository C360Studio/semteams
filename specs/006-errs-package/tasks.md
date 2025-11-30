# Tasks: Move and Rename Errors Package

**Input**: Design documents from `/specs/006-errs-package/`
**Prerequisites**: plan.md (complete), spec.md (complete), research.md, data-model.md, quickstart.md

**Tests**: Existing tests are preserved; no new tests required (this is a pure refactoring task).

**Organization**: Tasks are organized sequentially since this is a migration that must be atomic.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Source**: `errors/` (current location)
- **Target**: `pkg/errs/` (new location)
- **Consumers**: 99 files across the codebase

---

## Phase 1: Setup (Package Migration Infrastructure)

**Purpose**: Create the new package structure and copy files

- [x] T001 Create new package directory at pkg/errs/
- [x] T002 [P] Copy errors/doc.go to pkg/errs/doc.go
- [x] T003 [P] Copy errors/errors.go to pkg/errs/errs.go
- [x] T004 [P] Copy errors/errors_test.go to pkg/errs/errs_test.go

---

## Phase 2: Package Declaration Updates

**Purpose**: Update package name from `errors` to `errs` in all package files

**⚠️ CRITICAL**: Must complete before consumer updates begin

- [x] T005 [P] Update package declaration from `package errors` to `package errs` in pkg/errs/doc.go
- [x] T006 [P] Update package declaration from `package errors` to `package errs` in pkg/errs/errs.go
- [x] T007 [P] Update package declaration from `package errors` to `package errs` in pkg/errs/errs_test.go
- [x] T008 Update doc.go documentation to reference new package name and import path pkg/errs/doc.go

**Checkpoint**: Package compiles with `go build ./pkg/errs/...`

---

## Phase 3: User Story 1 & 3 - No Naming Conflicts & Zero Breaking Changes (Priority: P1)

**Goal**: Update all 99 consumer files to use new import path and package name

**Independent Test**: `go build ./...` compiles successfully; `go test -race ./...` passes

### Consumer Import Updates

Note: Tasks are organized by directory/subsystem for efficient batch processing.

#### Core Packages

- [x] T009 [P] [US1] Update imports in component/*.go files (component/registry.go, component/validation.go, component/schema_tags.go, component/schema_tags_test.go, component/port.go, component/payload_registry.go)
- [x] T010 [P] [US1] Update imports in componentregistry/register.go
- [x] T011 [P] [US1] Update imports in config/validator.go
- [x] T012 [P] [US1] Update imports in engine/*.go files (engine/engine.go, engine/validator.go, engine/engine_integration_test.go)

#### Error Package Internal

- [x] T013 [US1] Update imports in pkg/errs/doc.go (example code references)

#### Flow and Service

- [x] T014 [P] [US1] Update imports in flowstore/*.go files (flowstore/store.go, flowstore/flow.go, flowstore/flow_test.go)
- [x] T015 [P] [US1] Update imports in service/*.go files (service/doc.go, service/flow_runtime_health.go, service/flow_runtime_metrics.go, service/flow_runtime_messages.go)

#### Gateway

- [x] T016 [P] [US1] Update imports in gateway/*.go files (gateway/types.go, gateway/types_test.go)
- [x] T017 [P] [US1] Update imports in gateway/graphql/*.go files (gateway/graphql/graphql.go, gateway/graphql/server.go, gateway/graphql/nats_client.go, gateway/graphql/errors.go, gateway/graphql/config.go)
- [x] T018 [P] [US1] Update imports in gateway/http/*.go files (gateway/http/http.go, gateway/http/http_test.go)

#### Graph Package

- [x] T019 [P] [US1] Update imports in graph/events.go
- [x] T020 [P] [US1] Update imports in processor/graph/*.go files (processor/graph/processor.go, processor/graph/mutations.go, processor/graph/queries.go)
- [x] T021 [P] [US1] Update imports in processor/graph/clustering/*.go files (processor/graph/clustering/provider.go, processor/graph/clustering/storage.go, processor/graph/clustering/summarizer.go, processor/graph/clustering/lpa.go, processor/graph/clustering/enhancement_worker.go)
- [x] T022 [P] [US1] Update imports in processor/graph/datamanager/*.go files (processor/graph/datamanager/manager.go, processor/graph/datamanager/batch_ops.go, processor/graph/datamanager/edge_ops.go, processor/graph/datamanager/errors.go)
- [x] T023 [P] [US1] Update imports in processor/graph/embedding/*.go files (processor/graph/embedding/storage.go, processor/graph/embedding/worker.go)
- [x] T024 [P] [US1] Update imports in processor/graph/indexmanager/*.go files (processor/graph/indexmanager/manager.go, processor/graph/indexmanager/semantic.go, processor/graph/indexmanager/watcher.go, processor/graph/indexmanager/indexes.go, processor/graph/indexmanager/config.go, processor/graph/indexmanager/errors.go, processor/graph/indexmanager/predicate_integration_test.go)
- [x] T025 [P] [US1] Update imports in processor/graph/messagemanager/processor.go
- [x] T026 [P] [US1] Update imports in processor/graph/querymanager/*.go files (processor/graph/querymanager/manager.go, processor/graph/querymanager/query.go, processor/graph/querymanager/config.go, processor/graph/querymanager/errors.go, processor/graph/querymanager/graphrag_search.go)

#### Input/Output

- [x] T027 [P] [US1] Update imports in input/udp/*.go files (input/udp/udp.go, input/udp/udp_test.go)
- [x] T028 [P] [US1] Update imports in input/websocket/*.go files (input/websocket/websocket_input.go, input/websocket/register.go)
- [x] T029 [P] [US1] Update imports in output/file/file.go
- [x] T030 [P] [US1] Update imports in output/httppost/httppost.go
- [x] T031 [P] [US1] Update imports in output/websocket/websocket.go

#### Message and Metrics

- [x] T032 [P] [US1] Update imports in message/*.go files (message/types.go, message/payload_test.go, message/generic_json.go, message/base_message.go)
- [x] T033 [P] [US1] Update imports in metric/*.go files (metric/registry.go, metric/handler.go)

#### NATS and Storage

- [x] T034 [P] [US1] Update imports in natsclient/client.go
- [x] T035 [P] [US1] Update imports in storage/objectstore/stored_message.go

#### Processors

- [x] T036 [P] [US1] Update imports in processor/json_filter/json_filter.go
- [x] T037 [P] [US1] Update imports in processor/json_generic/json_generic.go
- [x] T038 [P] [US1] Update imports in processor/json_map/json_map.go
- [x] T039 [P] [US1] Update imports in processor/parser/json.go
- [x] T040 [P] [US1] Update imports in processor/rule/*.go files (processor/rule/processor.go, processor/rule/config_validation.go, processor/rule/publisher.go, processor/rule/factory.go, processor/rule/runtime_config.go, processor/rule/entity_watcher.go)

#### PKG Utilities

- [x] T041 [P] [US1] Update imports in pkg/acme/client.go
- [x] T042 [P] [US1] Update imports in pkg/buffer/*.go files (pkg/buffer/circular.go, pkg/buffer/buffer_test.go)
- [x] T043 [P] [US1] Update imports in pkg/cache/*.go files (pkg/cache/cache.go, pkg/cache/config.go, pkg/cache/hybrid.go, pkg/cache/lru.go, pkg/cache/simple.go, pkg/cache/ttl.go)
- [x] T044 [P] [US1] Update imports in pkg/tlsutil/tlsutil.go

#### Types

- [x] T045 [P] [US1] Update imports in types/*.go files (types/component.go, types/component_test.go, types/service.go, types/service_test.go)

#### Command Line Tools

- [x] T046 [P] [US1] Update imports in cmd/semstreams-gqlgen/*.go files (cmd/semstreams-gqlgen/config.go, cmd/semstreams-gqlgen/generate.go, cmd/semstreams-gqlgen/templates.go, cmd/semstreams-gqlgen/schema.go)

**Checkpoint**: `go build ./...` succeeds with all import updates

---

## Phase 4: User Story 2 - Consistent Package Organization (Priority: P2)

**Goal**: Remove old package directory and verify organizational consistency

**Independent Test**: `ls errors/` returns "no such file or directory"; `ls pkg/errs/` shows all files

### Cleanup

- [x] T047 [US2] Verify package tests pass with `go test -race ./pkg/errs/...`
- [x] T048 [US2] Remove old errors/ directory from repository root
- [x] T049 [US2] Verify no remaining references to old import path with `grep -r '"github.com/c360/semstreams/errors"' --include="*.go" .`

**Checkpoint**: Old directory removed, no stale references

---

## Phase 5: Polish & Validation

**Purpose**: Final validation and linting

- [x] T050 Run go fmt on all modified files with `go fmt ./...`
- [x] T051 Run revive linter with `revive ./...`
- [x] T052 Run full test suite with `go test -race ./...`
- [x] T053 Verify go.mod has no stale references with `go mod tidy`
- [x] T054 Run quickstart.md validation (manual review of migration steps)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Package Declaration (Phase 2)**: Depends on Phase 1 - creates compilable package
- **Consumer Updates (Phase 3)**: Depends on Phase 2 - all updates in parallel
- **Cleanup (Phase 4)**: Depends on Phase 3 - only after all consumers updated
- **Polish (Phase 5)**: Depends on Phase 4 - final validation

### User Story Mapping

| User Story | Phase | Tasks | Description |
|------------|-------|-------|-------------|
| US1 (P1) | Phase 3 | T009-T046 | No naming conflicts - import updates |
| US2 (P2) | Phase 4 | T047-T049 | Consistent organization - cleanup |
| US3 (P1) | Phase 3 | T009-T046 | Zero breaking changes (same tasks as US1) |

### Parallel Opportunities

**Phase 1** (all parallel):
- T002, T003, T004 - file copies

**Phase 2** (all parallel):
- T005, T006, T007 - package declaration updates

**Phase 3** (all parallel):
- T009-T046 - all consumer import updates (different files)

---

## Parallel Example: Consumer Updates

```bash
# All consumer update tasks can run in parallel since they modify different files:
Task: "Update imports in component/*.go files"
Task: "Update imports in gateway/graphql/*.go files"
Task: "Update imports in processor/graph/indexmanager/*.go files"
Task: "Update imports in pkg/cache/*.go files"
# ... all other T009-T046 tasks
```

---

## Implementation Strategy

### Atomic Migration (Recommended)

This migration should be atomic - all tasks completed in a single commit to avoid broken intermediate states:

1. Complete Phase 1: Setup (copy files)
2. Complete Phase 2: Package declarations (update package name)
3. Complete Phase 3: Consumer updates (all 99 files)
4. Complete Phase 4: Cleanup (remove old directory)
5. Complete Phase 5: Polish (validate everything)
6. Single commit with conventional format: `refactor(errors): move and rename to pkg/errs`

### Validation at Each Checkpoint

- After Phase 2: `go build ./pkg/errs/...` must succeed
- After Phase 3: `go build ./...` must succeed
- After Phase 4: `grep` for old path returns nothing
- After Phase 5: All tests pass, linters clean

---

## Notes

- [P] tasks = different files, no dependencies - can run in parallel
- [US1] and [US3] share tasks because they're both satisfied by the same import updates
- [US2] has dedicated cleanup tasks
- All consumer updates in Phase 3 are parallel since they modify different files
- Total files modified: ~102 (3 new + 99 updated)
- Recommend: Use automated find-and-replace per quickstart.md for efficiency
