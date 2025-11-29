# Tasks: Semantic System Refactor

**Feature**: 004-semantic-refactor
**Generated**: 2025-01-29
**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

## Task Overview

| Priority | Story | Tasks | Description |
|----------|-------|-------|-------------|
| P1 | US1 | 5 | Simplified EntityState structure |
| P1 | US2 | 3 | Domain-owned status (remove hardcoded) |
| P2 | US3 | 2 | Full StorageReference support |
| P2 | US4 | 3 | Delete unused abstractions |
| P3 | INT | 8 | Integration and validation |

## Phase 1: Simplified EntityState (P1 - US1)

**User Story**: Domain Developer Uses Simplified EntityState
**Dependencies**: None (foundational)

- [x] [T001] [P1] [US1] Update EntityState struct in `graph/types.go`: promote ID to top-level, change ObjectRef to StorageRef, change MessageType to message.Type, delete NodeProperties/Position/EntityStatus
- [x] [T002] [P1] [US1] Update duplicate EntityState in `types/graph/types.go` with same changes as T001 (duplicate maintained for package import isolation)
- [x] [T003] [P1] [US1] Update all `state.Node.ID` references to `state.ID` across codebase (58 occurrences)
- [x] [T004] [P1] [US1] Update all `state.Node.Type` references to use `message.ParseEntityID(state.ID).Type` (14 occurrences)
- [x] [T005] [P1] [US1] Update tests in `graph/` and `types/graph/` packages to use new EntityState structure

## Phase 2: Domain-Owned Status (P1 - US2)

**User Story**: Domain Developer Defines Custom Status
**Dependencies**: T001-T002 (EntityState structure)

- [x] [T006] [P1] [US2] Remove hardcoded StatusActive assignment in `processor/graph/messagemanager/processor.go`
- [x] [T007] [P1] [US2] Remove all `state.Node.Status` references from codebase
- [x] [T008] [P1] [US2] Update IoT sensor pipeline (`test/e2e/scenarios/iot_sensor_pipeline.go`) to demonstrate domain-specific status triples pattern

## Phase 3: Full StorageReference (P2 - US3)

**User Story**: Domain Developer Uses Full StorageReference
**Dependencies**: T001-T002 (EntityState structure)

- [x] [T009] [P2] [US3] Update all `ObjectRef` references to use `StorageRef` with nil checks (10 occurrences)
- [x] [T010] [P2] [US3] Update `processor/graph/datamanager/manager.go` to populate StorageRef from Storable interface

## Phase 4: Delete Unused Abstractions (P2 - US4)

**User Story**: Codebase Has No Unused Abstractions
**Dependencies**: T001-T010 (all consumer updates complete)

- [x] [T011] [P2] [US4] Delete `message/entity_types.go` (EntityClass, EntityRole definitions)
- [x] [T012] [P2] [US4] Delete `message/entity_types_test.go` (tests for deleted types)
- [x] [T013] [P2] [US4] Delete `message/entity_payload.go` (unused payload types)

## Phase 5: Integration & Validation (P3 - INT)

**User Story**: Cross-cutting integration tasks after all P1/P2 work complete
**Dependencies**: All P1 and P2 tasks complete (T001-T013)

- [x] [T014] [P3] [INT] Update GraphQL resolvers in `gateway/graphql/base_resolver.go` to use new EntityState structure
- [x] [T015] [P3] [INT] Update `pkg/graphclustering/summarizer.go` for new entity structure
- [x] [T016] [P3] [INT] Update query client in `graph/query/` for state.ID access pattern
- [x] [T017] [P3] [INT] Update index managers in `processor/graph/indexmanager/` for new structure
- [x] [T018] [P3] [INT] Update query manager in `processor/graph/querymanager/` for new structure
- [x] [T019] [P3] [INT] Run full test suite: `go test -race ./...`
- [x] [T020] [P3] [INT] Run linting: `go fmt ./... && revive ./...`
- [x] [T021] [P3] [INT] Verify success criteria SC-001 through SC-007

## Verification Checklist

After all tasks complete:

```bash
# SC-002: No old Node references
grep -r "\.Node\.ID\|\.Node\.Type\|\.Node\.Status\|\.Node\.Position" --include="*.go" | wc -l
# Expected: 0

# SC-003: Deleted files don't exist
ls message/entity_types.go message/entity_types_test.go message/entity_payload.go 2>&1 | grep -c "No such file"
# Expected: 3

# SC-004: Deleted types don't exist
grep -r "EntityClass\|EntityRole\|EntityStatus\|NodeProperties\|Position struct" --include="*.go" | wc -l
# Expected: 0

# SC-001: All tests pass
go test -race ./...
# Expected: PASS
```

## Task Dependencies Graph

```text
        ┌─────────────────────────────────────────────────────────────┐
        │                     PHASE 1 (P1-US1)                        │
        │                                                             │
T001 ───┼──┬──────────────────────────────────────────────────────────┤
        │  │                                                          │
T002 ───┼──┴─→ T003 ─┬─→ T005                                         │
        │      T004 ─┘                                                │
        └─────────────────────────────────────────────────────────────┘
                │
                ▼
        ┌─────────────────────────────────────────────────────────────┐
        │                     PHASE 2 (P1-US2)                        │
        │                                                             │
        │      T006 ─→ T007 ─→ T008                                   │
        └─────────────────────────────────────────────────────────────┘
                │
                ▼
        ┌─────────────────────────────────────────────────────────────┐
        │                     PHASE 3 (P2-US3)                        │
        │                                                             │
        │      T009 ─→ T010                                           │
        └─────────────────────────────────────────────────────────────┘
                │
                ▼
        ┌─────────────────────────────────────────────────────────────┐
        │                     PHASE 4 (P2-US4)                        │
        │                                                             │
        │      T011 ─→ T012 ─→ T013                                   │
        └─────────────────────────────────────────────────────────────┘
                │
                ▼
        ┌─────────────────────────────────────────────────────────────┐
        │                     PHASE 5 (P3-INT)                        │
        │                                                             │
        │      T014 ─┬─→ T019 ─→ T020 ─→ T021                         │
        │      T015 ─┤                                                │
        │      T016 ─┤                                                │
        │      T017 ─┤                                                │
        │      T018 ─┘                                                │
        └─────────────────────────────────────────────────────────────┘
```

## Notes

- **Greenfield**: No backward compatibility required (D7 from ADR)
- **TDD**: Write tests first for new EntityState structure before implementation
- **Critical Path**: T001/T002 → T003/T004 → T019 (EntityState changes → consumer updates → validation)
- **Parallel Work**:
  - T003 and T004 can proceed in parallel after T001+T002 complete
  - T014-T018 can proceed in parallel before T019
- **File Count**: ~50 files affected (per plan.md)
- **Duplicate Types File**: `types/graph/types.go` exists for package import isolation - both files must stay synchronized
