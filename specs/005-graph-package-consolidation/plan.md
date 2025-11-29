# Implementation Plan: Graph Package Consolidation

**Branch**: `005-graph-package-consolidation` | **Date**: 2025-11-29 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/005-graph-package-consolidation/spec.md`

## Summary

Consolidate graph-related packages to eliminate technical debt: migrate 9 files from `types/graph/` to `graph/`, move Graphable interface to `graph/`, delete redundant federation code, relocate graphclustering, and remove Java-style getter anti-patterns. This implements ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.md following the 004-semantic-refactor.

## Technical Context

**Language/Version**: Go 1.25+
**Primary Dependencies**: NATS JetStream, GraphQL (gqlgen)
**Storage**: NATS KV buckets (ENTITY_STATES, indexes)
**Testing**: `go test -race`, table-driven tests
**Target Platform**: Linux server (containerized)
**Project Type**: Single Go module with multiple packages
**Performance Goals**: Existing performance maintained (refactor only)
**Constraints**: No import cycles, zero build errors
**Scale/Scope**: ~15 files for Graphable migration, ~9 files for types/graph, ~7 files for graphinterfaces

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Spec-First Development | PASS | Spec complete at `specs/005-graph-package-consolidation/spec.md` |
| II. TDD (NON-NEGOTIABLE) | PASS | Existing tests will be migrated; build verification after each phase |
| III. Quality Gate Compliance | PASS | Six gates will be applied per task |
| IV. Code Review Standards | PASS | go-reviewer will review each phase |
| V. Documentation & Traceability | PASS | ADR exists; READMEs will be updated |

**Gate Result**: PASS - No violations. Proceed to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/005-graph-package-consolidation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (minimal - refactor only)
├── quickstart.md        # Phase 1 output (migration guide)
├── contracts/           # Phase 1 output (N/A - no new APIs)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Current Structure (to be consolidated)
graph/                           # Target: authoritative graph types
├── types.go                     # EntityState (already simplified by 004)
├── graphable.go                 # WILL BE MOVED HERE from message/
├── federation.go                # WILL BE DELETED
├── helpers.go
├── events.go
└── query/                       # External query client

types/graph/                     # WILL BE DELETED (stale duplicate)
├── types.go                     # Duplicate of graph/types.go
├── helpers.go
├── events.go
└── ...

message/
├── graphable.go                 # WILL BE MOVED to graph/
├── types.go                     # EntityID (stays - transport primitive)
├── triple.go                    # Triple (stays - transport primitive)
└── federation.go                # FederationMeta (stays - identity)

pkg/graphclustering/             # WILL BE MOVED to processor/graph/clustering/
├── types.go                     # Community struct (getters to be removed)
├── lpa.go
├── storage.go
└── summarizer.go

pkg/graphinterfaces/             # WILL BE DELETED (cycle-breaking hack)
└── community.go                 # Interface with 10 getter methods

pkg/embedding/                   # WILL BE MOVED to processor/graph/embedding/
├── embedder.go                  # Embedder interface
├── bm25_embedder.go             # BM25 implementation
├── http_embedder.go             # HTTP-based embedding service
├── cache.go                     # Embedding cache
├── storage.go                   # Embedding storage
├── vector.go                    # Vector operations
└── worker.go                    # Async worker pool

processor/graph/
├── clustering/                  # NEW LOCATION for graphclustering
├── embedding/                   # NEW LOCATION for embedding
├── querymanager/
├── indexmanager/                # Consumer of embedding (2 files)
├── datamanager/
└── messagemanager/
```

**Structure Decision**: Consolidate to standard Go package layout where graph types live in `graph/`, clustering moves inside `processor/graph/`, and `types/graph/` + `pkg/graphinterfaces/` are deleted.

## Complexity Tracking

> No constitution violations requiring justification. All changes follow established patterns.

| Item | Complexity | Rationale |
|------|------------|-----------|
| Move graphclustering | Medium | ~5000 LOC, but mechanical move with import path updates |
| Move embedding | Low | ~7 files, only 2 consumers in indexmanager |
| Move Graphable | Low | Single interface, ~15 implementers to update |
| Delete federation.go | Low | No active consumers found in current usage |
| Delete types/graph | Low | 9 files with simple import path change |

## Phase Dependencies

```text
Phase 1: Eliminate types/graph     ─┐
Phase 2: Move Graphable            ─┼─→ Can run in parallel
Phase 3: Delete federation.go      ─┘

Phase 4: Move graphclustering      ─┐
                                    ├─→ Must be sequential
Phase 5: Delete graphinterfaces    ─┘   (Phase 4 before Phase 5)

Phase 6: Move embedding            ─→ Independent (can run anytime)

Phase 7: Documentation             ─→ After all code changes
```

## Migration Phases

### Phase 1: Eliminate types/graph/ (9 files)

**Files to Update**:
1. `processor/graph/cleanup.go`
2. `processor/graph/cleanup_test.go`
3. `processor/rule/kv_test_helpers.go`
4. `processor/rule/entity_watcher.go`
5. `processor/rule/rule_integration_test.go`
6. `processor/rule/test_rule_factory.go`
7. `processor/rule/expression/evaluator_test.go`
8. `processor/rule/expression/types.go`
9. `processor/rule/expression/evaluator.go`

**Change Pattern**:
```diff
-import gtypes "github.com/c360/semstreams/types/graph"
+import gtypes "github.com/c360/semstreams/graph"
```

**Verification**: `go build ./...` succeeds

### Phase 2: Move Graphable to graph/ (9 usages)

**Files to Update**:
1. `message/graphable.go` → `graph/graphable.go`
2. `processor/graph/messagemanager/processor.go`
3. `storage/objectstore/stored_message.go`
4. `examples/processors/iot_sensor/processor_test.go`
5. `examples/processors/iot_sensor/payload_test.go`
6. Any other Graphable implementers

**Change Pattern**:
```diff
-import "github.com/c360/semstreams/message"
+import "github.com/c360/semstreams/graph"

-message.Graphable
+graph.Graphable
```

### Phase 3: Delete graph/federation.go

**Analysis Needed**: Check for consumers of:
- `BuildGlobalID()`
- `FederatedEntity`
- `EnrichEntityState()`
- `GetFederationInfo()`

**Action**: Delete file if no consumers, or migrate consumers to use `message.EntityID` fields.

### Phase 4: Move graphclustering to processor/graph/clustering/

**Steps**:
1. `mv pkg/graphclustering processor/graph/clustering`
2. Update package name in all files: `package clustering`
3. Update all import paths
4. Verify no import cycles

### Phase 5: Delete graphinterfaces and Community getters

**Prerequisite**: Phase 4 complete (import cycle eliminated)

**Steps**:
1. Remove all getter methods from Community struct (10 methods)
2. Update callers to use direct field access
3. Delete `pkg/graphinterfaces/` directory

### Phase 6: Move embedding to processor/graph/embedding/

**Files to Move** (7 files):
- `pkg/embedding/embedder.go` - Embedder interface
- `pkg/embedding/bm25_embedder.go` - BM25 implementation
- `pkg/embedding/http_embedder.go` - HTTP-based embedding service
- `pkg/embedding/cache.go` - Embedding cache
- `pkg/embedding/storage.go` - Embedding storage
- `pkg/embedding/vector.go` - Vector operations
- `pkg/embedding/worker.go` - Async worker pool

**Files to Update** (2 consumers):
- `processor/graph/indexmanager/semantic.go`
- `processor/graph/indexmanager/manager.go`

**Steps**:
1. `mv pkg/embedding processor/graph/embedding`
2. Update package name in all files: `package embedding` (no change needed)
3. Update import paths in consumer files
4. Verify `go build ./...` succeeds

### Phase 7: Documentation

**Files to Update**:
- `graph/README.md` - Document ownership scope
- `message/README.md` - Clarify transport-only scope
- `processor/graph/README.md` - Document clustering and embedding locations

---

## Constitution Re-Check (Post-Design)

*GATE: Verify design still complies with constitution principles.*

| Principle | Status | Post-Design Evidence |
|-----------|--------|---------------------|
| I. Spec-First Development | PASS | Spec, plan, research, data-model, quickstart complete |
| II. TDD (NON-NEGOTIABLE) | PASS | Refactor preserves all existing tests; build verification gates each phase |
| III. Quality Gate Compliance | PASS | Six gates will be applied per task; phases have explicit verification steps |
| IV. Code Review Standards | PASS | go-reviewer will review each phase before merge |
| V. Documentation & Traceability | PASS | ADR referenced; quickstart.md provides migration guide |

**Post-Design Gate Result**: PASS - Design complies with all constitution principles.

---

## Generated Artifacts

| Artifact | Path | Purpose |
|----------|------|---------|
| Specification | `specs/005-graph-package-consolidation/spec.md` | Feature requirements and success criteria |
| Implementation Plan | `specs/005-graph-package-consolidation/plan.md` | This file - technical approach |
| Research | `specs/005-graph-package-consolidation/research.md` | Codebase analysis and unknowns resolution |
| Data Model | `specs/005-graph-package-consolidation/data-model.md` | Types being relocated |
| Quickstart | `specs/005-graph-package-consolidation/quickstart.md` | Migration guide for developers |

**Next Step**: Run `/speckit.tasks` to generate actionable task breakdown.
