# ADR: Consolidate Graph Type Definitions into Single Package

**Status:** Proposed
**Author:** Architecture Review
**Date:** 2025-11-29
**Related:** ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md, TODO-GRAPH-INDEXING-ARCHITECTURE.md

---

## Context

The SemStreams codebase has accumulated duplicate graph-related packages that create confusion, maintenance burden, and potential type incompatibility:

### Current State

| Package | Purpose | Importers | Status |
|---------|---------|-----------|--------|
| `/graph/` | Core types, federation, events, mutations | 46 files | Canonical |
| `/types/graph/` | Near-duplicate types | 9 files | Stale duplicate |
| `/processor/graph/` | Runtime processor | N/A | Keep as-is |

### Problem 1: Type Duplication and Drift

Two packages define the same types with subtle differences:

```go
// /graph/types.go - CANONICAL
type EntityState struct {
    Node        NodeProperties
    Triples     []message.Triple
    ObjectRef   string
    MessageType string    // EXISTS
    Version     uint64
    UpdatedAt   time.Time
}

// /types/graph/types.go - STALE
type EntityState struct {
    Node      NodeProperties
    Triples   []message.Triple
    ObjectRef string
    // MessageType MISSING - DRIFT
    Version   uint64
    UpdatedAt time.Time
}
```

### Problem 2: Inconsistent Import Patterns

```go
// Rule processor uses stale package
import gtypes "github.com/c360/semstreams/types/graph"

// Graph processor uses canonical package
import gtypes "github.com/c360/semstreams/graph"

// Same alias hides the difference!
```

### Problem 3: Missing Features in Stale Package

`/types/graph/` lacks critical functionality present in `/graph/`:
- `incoming.go` - Incoming edge index types
- `query_types.go` - Query direction enums
- `federation.go` - Federation utilities (BuildGlobalID, ParseGlobalID)

### Problem 4: Query Package Confusion

Two query-related packages with overlapping purposes:

| Location | Purpose | When to Use |
|----------|---------|-------------|
| `/graph/query/` | External client library, direct KV access | Components outside GraphProcessor |
| `/processor/graph/querymanager/` | Internal service, multi-tier caching | Inside GraphProcessor only |

### Historical Context

`/types/graph/` is a historical artifact from pre-greenfield migration. The README still references "Edge structure" which was removed. Last updated: 2024-08-29.

---

## Decision

**Consolidate all graph type definitions into `/graph/` package and delete `/types/graph/`.**

### Actions

1. Migrate all `/types/graph/` imports to `/graph/` (9 files)
2. Add linter rule preventing new `/types/graph/` imports
3. Delete `/types/graph/` directory
4. Document clear boundaries between `/graph/query/` and `/processor/graph/querymanager/`

---

## Consequences

### Positive

- **Single source of truth** for graph types
- **All components access federation utilities** (currently blocked for rule processor)
- **Reduced maintenance burden** (no duplicate changes)
- **Eliminated type drift risk** (MessageType field difference)
- **Clearer package structure** for new developers

### Negative

- Requires migration of 9 files (low effort, ~4 hours)
- Temporary deprecation period needed
- Must update documentation

### Neutral

- No breaking changes expected (canonical EntityState is superset)
- Test patterns remain the same

---

## Migration Plan

### Phase 1: Migrate Imports (Week 1)

**Files to migrate** (9 total):

```
processor/rule/entity_watcher.go
processor/rule/expression/evaluator.go
processor/rule/expression/types.go
processor/rule/kv_test_helpers.go
processor/rule/test_rule_factory.go
processor/graph/cleanup.go
processor/graph/cleanup_test.go
```

**Change pattern**:
```diff
-import gtypes "github.com/c360/semstreams/types/graph"
+import gtypes "github.com/c360/semstreams/graph"
```

**Verification**:
```bash
go test -race ./processor/rule/...
go test -race ./processor/graph/...
```

### Phase 2: Deprecate Package (Week 1)

Add linter rule to prevent new imports:

```yaml
# .golangci.yml
linters-settings:
  depguard:
    rules:
      main:
        deny:
          - pkg: "github.com/c360/semstreams/types/graph"
            desc: "DEPRECATED: Use github.com/c360/semstreams/graph instead"
```

### Phase 3: Delete Package (Week 2)

```bash
# Verify zero imports
grep -r "types/graph" --include="*.go" . | grep -v "types/graph/"

# Delete
rm -rf types/graph/

# Clean module
go mod tidy
```

### Phase 4: Document Query Boundaries (Week 2)

Update READMEs to clarify:

**`/graph/query/`** - Use when:
- External component (outside GraphProcessor)
- Read-only graph access needed
- Direct NATS KV bucket access
- Don't need GraphProcessor's multi-tier caching

**`/processor/graph/querymanager/`** - Use when:
- Inside GraphProcessor implementation
- Need multi-tier caching (L1/L2/L3)
- Need KV Watch for cache invalidation
- Need GraphRAG search integration

---

## Alternatives Considered

### Alternative 1: Keep Both Packages

**Rejected**: Perpetuates confusion and maintenance burden. Type drift will continue.

### Alternative 2: Make /types/graph/ Canonical

**Rejected**: Would require migrating 46 files vs 9 files. Also lacks federation utilities.

### Alternative 3: Create New /pkg/graphtypes

**Rejected**: Adds third package, increases confusion. `/graph/` is already well-established.

### Alternative 4: Type Aliases for Compatibility

```go
// types/graph/compat.go
type EntityState = graph.EntityState
```

**Rejected**: Adds complexity, doesn't solve the root problem. Better to migrate cleanly.

---

## Success Criteria

- [ ] All 9 files migrated to `/graph/` imports
- [ ] All tests passing with new imports
- [ ] Linter rule preventing new `/types/graph/` imports
- [ ] Zero imports of `/types/graph/` verified
- [ ] `/types/graph/` directory deleted
- [ ] Query package documentation updated
- [ ] `go mod tidy` completed successfully

---

## Risk Analysis

### Low Risk
- Import path changes (mechanical, easily reverted)
- Test updates (straightforward)

### Medium Risk
- Rule processor type compatibility (mitigated: canonical is superset)
- Integration tests (mitigated: comprehensive test suite exists)

### Mitigation
- Run full test suite before and after
- Migrate one component at a time
- Keep revert commits prepared

---

## References

- Architect analysis: 2025-11-29 code review session
- Feature 003-triples-architecture: Greenfield migration that removed Edges/Properties
- Related issue: Graph processor and rule processor type incompatibility

---

**Version**: 1.0.0 | **Proposed**: 2025-11-29
