# Research: Graph Package Consolidation

**Feature**: 005-graph-package-consolidation
**Date**: 2025-11-29

## Executive Summary

This research validates the ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.md claims and identifies the exact files requiring changes. All unknowns have been resolved through codebase analysis.

## Research Findings

### 1. types/graph/ Import Analysis

**Decision**: Migrate 9 files to use `graph/` package
**Rationale**: Verified 9 files still import `types/graph/`
**Alternatives Considered**: None - this is a required consolidation

**Files Requiring Migration**:
| File | Line | Current Import |
|------|------|----------------|
| `processor/graph/cleanup.go` | 11 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/graph/cleanup_test.go` | 10 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/kv_test_helpers.go` | 10 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/entity_watcher.go` | 12 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/rule_integration_test.go` | 23 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/test_rule_factory.go` | 10 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/expression/evaluator_test.go` | 11 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/expression/types.go` | 7 | `gtypes "github.com/c360/semstreams/types/graph"` |
| `processor/rule/expression/evaluator.go` | 9 | `gtypes "github.com/c360/semstreams/types/graph"` |

### 2. message.Graphable Usage Analysis

**Decision**: Move `message/graphable.go` to `graph/graphable.go`
**Rationale**: 9 usages found; Graphable is a graph contract, not a transport primitive
**Alternatives Considered**: Keep in message/ - rejected per ADR principle that graph contracts belong in graph/

**Files Requiring Update**:
| File | Usage Type |
|------|------------|
| `processor/graph/messagemanager/processor.go:175` | Type assertion `msg.(message.Graphable)` |
| `processor/graph/messagemanager/processor.go:186` | Parameter type `message.Graphable` |
| `storage/objectstore/stored_message.go:47` | Parameter type `message.Graphable` |
| `storage/objectstore/stored_message.go:57` | Interface implementation comment |
| `storage/objectstore/stored_message.go:62` | Interface implementation comment |
| `examples/processors/iot_sensor/processor_test.go:98` | Interface assertion |
| `examples/processors/iot_sensor/payload_test.go:224` | Interface assertion |
| `examples/processors/iot_sensor/payload_test.go:225` | Interface assertion |
| `component/flowgraph/flowgraph_validation_test.go:393` | String reference (test data) |

### 3. graph/federation.go Consumer Analysis

**Decision**: Delete `graph/federation.go` and `graph/federation_iri_test.go`
**Rationale**: No non-test consumers found; federation identity is in `message.EntityID`
**Alternatives Considered**: Keep for potential future use - rejected per greenfield principle

**Findings**:
- `BuildGlobalID()`: Used only in test file `federation_iri_test.go`
- `FederatedEntity`: Used only in test files
- `EnrichEntityState()`: No external consumers
- `GetFederationInfo()`: No external consumers

**Note**: `message/federation.go:90` has a comment referencing `BuildGlobalID` but it's documentation only.

### 4. pkg/graphinterfaces Consumer Analysis

**Decision**: Delete after graphclustering move
**Rationale**: 7 files import it; interface exists only to break import cycle
**Alternatives Considered**: Keep interface for abstraction - rejected per Go idioms

**Files Importing graphinterfaces**:
| File | Usage |
|------|-------|
| `processor/graph/graphrag_integration_test.go` | Test |
| `processor/graph/querymanager/graphrag_search_test.go` | Test |
| `processor/graph/querymanager/graphrag_search.go` | Production |
| `processor/graph/querymanager/interface.go` | Interface definition |
| `gateway/graphql/graphql_test.go` | Test |
| `gateway/graphql/base_resolver.go` | Production |
| `pkg/graphclustering/types.go` | Implementation |

### 5. Community Getter Method Usage Analysis

**Decision**: Remove 10 getter methods, update ~30 call sites to direct field access
**Rationale**: Go idiomatic style uses direct field access, not getters
**Alternatives Considered**: Keep getters for interface compatibility - rejected since interface will be deleted

**Getter Methods to Remove**:
1. `GetID()` → `.ID`
2. `GetLevel()` → `.Level`
3. `GetMembers()` → `.Members`
4. `GetKeywords()` → `.Keywords`
5. `GetRepEntities()` → `.RepEntities`
6. `GetStatisticalSummary()` → `.StatisticalSummary`
7. `GetLLMSummary()` → `.LLMSummary`
8. `GetSummaryStatus()` → `.SummaryStatus`
9. `GetMetadata()` → `.Metadata`
10. `GetParentID()` → `.ParentID`

**Files Requiring Getter→Field Updates**:
- `processor/graph/querymanager/graphrag_search.go` (~12 usages)
- `processor/graph/querymanager/graphrag_search_test.go` (~8 usages)
- `gateway/graphql/base_resolver.go` (~9 usages)

### 6. pkg/graphclustering External Consumer Analysis

**Decision**: Move to `processor/graph/clustering/`
**Rationale**: Only 1 external consumer (a test file)
**Alternatives Considered**: Keep in pkg/ - rejected per ADR analysis showing pkg/ is for reusable external libraries

**External Consumers**:
| File | Type |
|------|------|
| `processor/graph/graphrag_integration_test.go` | Test only |

### 7. pkg/embedding External Consumer Analysis

**Decision**: Move to `processor/graph/embedding/`
**Rationale**: Only 2 consumers, both in `processor/graph/indexmanager/`
**Alternatives Considered**: Keep in pkg/ - rejected per same principle as graphclustering

**Package Contents** (7 files):
| File | Purpose |
|------|---------|
| `embedder.go` | Embedder interface and factory |
| `bm25_embedder.go` | BM25 sparse embedding implementation |
| `http_embedder.go` | HTTP-based external embedding service |
| `cache.go` | Embedding result cache |
| `storage.go` | Embedding persistence |
| `vector.go` | Vector math operations |
| `worker.go` | Async worker pool for batch embeddings |

**External Consumers** (2 files):
| File | Usage |
|------|-------|
| `processor/graph/indexmanager/semantic.go` | Embedding interface usage |
| `processor/graph/indexmanager/manager.go` | Embedding initialization |

### 8. Import Cycle Analysis

**Decision**: Move graphclustering BEFORE deleting graphinterfaces
**Rationale**: Import cycle exists between graphclustering and querymanager; moving graphclustering inside processor/graph/ eliminates the cycle

**Current Cycle**:
```
pkg/graphclustering → processor/graph/querymanager (for Querier)
processor/graph/querymanager → pkg/graphinterfaces (for Community interface)
pkg/graphclustering → pkg/graphinterfaces (implements interface)
```

**After Move**:
```
processor/graph/clustering → processor/graph/querymanager (same parent, no cycle)
processor/graph/querymanager → processor/graph/clustering (same parent, no cycle)
```

**Note**: The embedding package move (Phase 6) is independent and introduces no import cycles since its only consumers are already in `processor/graph/indexmanager/`.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Import cycle after graphclustering move | Low | High | Run `go build ./...` immediately after move |
| Missing Graphable implementer | Low | Medium | Grep verified all implementers are internal |
| Breaking test expectations | Medium | Low | Run full test suite after each phase |
| Documentation references stale paths | Medium | Low | Update READMEs in Phase 6 |

## Resolved Unknowns

| Unknown | Resolution |
|---------|------------|
| How many types/graph importers? | 9 files (verified) |
| Any external Graphable implementers? | No - all internal |
| Federation.go consumers? | Test files only - safe to delete |
| Import cycle resolution order? | Phase 4 (move graphclustering) before Phase 5 (delete graphinterfaces) |
| Getter method call sites? | ~30 call sites across 3 production files + tests |
| Embedding package consumers? | 2 files in processor/graph/indexmanager/ |
| Embedding package files? | 7 files (embedder, bm25, http, cache, storage, vector, worker) |

## Recommendations

1. **Execute phases sequentially** - Build verification between each phase catches issues early
2. **Phase 4 before Phase 5** - Critical ordering to avoid broken imports
3. **Run `go test -race ./...`** - Verify no behavior changes after refactoring
4. **Delete test files with deleted code** - `federation_iri_test.go` must be deleted with `federation.go`
