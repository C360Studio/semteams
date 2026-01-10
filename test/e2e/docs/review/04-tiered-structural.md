# Tiered-Structural Scenario Review

**Reviewed**: 2025-12-20  
**Files**: `test/e2e/scenarios/tiered.go`, `test/e2e/scenarios/tiered_structural.go`  
**Status**: Good Coverage with Notable Gaps

---

## Scenario Overview

The `tiered --variant structural` scenario validates rules-only processing with ZERO ML inference. This is Tier 0 - the foundational tier that works with just NATS, no embedding services or LLMs required.

**Duration**: ~30 seconds  
**Tier**: Structural (Tier 0)  
**Dependencies**: NATS only

---

## What's Tested

### Common Stages (All Variants)

| Stage | Purpose | Assertion Strength |
|-------|---------|-------------------|
| `verify-components` | Check required components exist | Strong |
| `send-mixed-data` | Send telemetry + regular messages | Medium |
| `validate-processing` | Verify graph processor running | Medium |
| `verify-entity-count` | Count entities in ENTITY_STATES | Medium |
| `verify-entity-retrieval` | Retrieve specific known entities | Medium |
| `validate-entity-structure` | Validate entity data structure | Medium |
| `verify-index-population` | Check 7 indexes populated | Strong |
| `validate-rules` | Verify rules evaluated/triggered | Medium |
| `validate-metrics` | Check Prometheus metrics exposed | Strong |
| `verify-outputs` | Verify output components exist | Medium |

### Structural-Only Stages

| Stage | Purpose | Assertion Strength |
|-------|---------|-------------------|
| `validate-zero-embeddings` | Assert embedding count = 0 | Strong |
| `validate-zero-clusters` | Assert clustering runs = 0 | Strong |
| `validate-rule-transitions` | Validate OnEnter/OnExit fired | Medium |

---

## Correctness Assessment

### Correct

1. **Zero-embeddings constraint** (`tiered_structural.go:12-30`)
   - Correctly validates `semstreams_graph_embedding_embeddings_generated_total = 0`
   - Warns if structural tier constraint violated

2. **Zero-clusters constraint** (`tiered_structural.go:33-51`)
   - Correctly validates `semstreams_clustering_runs_total = 0`
   - Ensures no community detection ran

3. **Rule state transitions** (`tiered_structural.go:54-95`)
   - Validates OnEnter/OnExit metrics
   - Checks minimum thresholds (2 OnEnter, 1 OnExit)

4. **Index population** (`tiered.go:1456-1521`)
   - Validates all 7 core indexes exist and are populated
   - Samples keys for debugging

5. **Entity structure validation** (`tiered.go:1349-1453`)
   - Validates entity ID format
   - Checks triples have subject/predicate
   - Validates version and timestamp

### Issues Found

| Issue | Severity | Description |
|-------|----------|-------------|
| Zero constraint is warning, not failure | **Medium** | Embedding/cluster violations log warnings but don't fail the test |
| PathRAG not tested | **High** | PathRAG is documented as Tier 0 feature but has no e2e test |
| Rule add_triple/remove_triple not validated | **Medium** | Rules can modify graph but effect not verified |
| No predicate index queries | **Medium** | PREDICATE_INDEX populated but not queried |
| State transition thresholds may be too low | **Low** | MinOnEnterFired=2, MinOnExitFired=1 - may pass with minimal rule activity |

---

## Gap Analysis

### Critical Gap: PathRAG Not Tested

The PathRAG pattern is documented as a Tier 0 (structural) feature:

From `docs/concepts/08-pathrag-pattern.md`:
> "PathRAG works at Tier 0 (no ML required)"
> "Given a starting entity, PathRAG answers: 'What's connected to this, and how?'"

**No e2e test validates PathRAG queries.** This means:
- Bounded DFS traversal is untested
- Decay-factor scoring is untested  
- Predicate filtering is untested
- Direction control (incoming/outgoing/both) is untested
- Resource bounds (max_depth, max_nodes, max_time) are untested

### Features Documented but Not Tested

| Feature | Documentation Source | Gap |
|---------|---------------------|-----|
| **PathRAG queries** | `docs/concepts/08-pathrag-pattern.md` | No test exists |
| Rule add_triple effect | Rules can add triples to graph | Not verified in indexes |
| Rule remove_triple effect | Rules can remove triples | Not verified |
| Rule publish action | Rules can publish to subjects | Not verified |
| Predicate index queries | PREDICATE_INDEX enables pattern queries | Not queried |
| Incoming/Outgoing relationship queries | INCOMING/OUTGOING_INDEX for traversal | Not queried |
| Alias resolution | ALIAS_INDEX for friendly names | Not tested |

### Zero-Constraint Assertions Are Weak

The structural variant constraints are only warnings:

```go
// From tiered_structural.go:23-26
if int(embeddingCount) > s.config.ExpectedEmbeddings {
    result.Warnings = append(result.Warnings,
        fmt.Sprintf("Structural tier constraint violated..."))
}
```

**Problem**: Test passes even if embeddings are generated, just logs a warning.

**Recommendation**: Make this a hard failure for structural variant - the whole point is ZERO ML inference.

---

## Recommendations

### Priority: High (Must Fix)

1. **Add PathRAG e2e test**
   
   Create test that:
   - Picks a known entity from test data
   - Executes PathRAG query via GraphQL/MCP gateway
   - Validates returned entities are connected in graph
   - Checks decay scores decrease with depth
   - Validates predicate filtering works

   ```go
   // Example test case structure
   pathRAGTests := []struct {
       startEntity     string
       predicateFilter []string
       maxDepth        int
       expectedEntities []string
   }{
       {
           startEntity:     "c360.logistics.sensor.document.temperature.sensor-temp-001",
           predicateFilter: []string{"sensor.measurement.*"},
           maxDepth:        2,
           expectedEntities: []string{"..."},
       },
   }
   ```

2. **Make zero-constraints hard failures**
   
   For structural variant, embeddings > 0 or clusters > 0 should fail the test, not just warn.

### Priority: Medium (Should Fix)

3. **Validate rule graph modifications**
   - After rules fire, query indexes for added triples
   - Verify add_triple creates entries in PREDICATE_INDEX
   - Verify remove_triple removes entries

4. **Test predicate index queries**
   - Query PREDICATE_INDEX with known predicates
   - Verify returns expected entity IDs

5. **Test relationship traversal**
   - Query INCOMING_INDEX for an entity
   - Query OUTGOING_INDEX for an entity
   - Verify bidirectional relationships

### Priority: Low (Nice to Have)

6. **Increase state transition thresholds**
   - MinOnEnterFired=2 is very low
   - Consider increasing based on test data complexity

7. **Add alias resolution test**
   - Create entity with alias
   - Resolve alias via ALIAS_INDEX
   - Verify returns correct entity ID

---

## Feature Coverage Matrix

| Documented Feature | Tested? | Notes |
|--------------------|---------|-------|
| Zero embeddings constraint | Yes | Warning only, should be hard failure |
| Zero clusters constraint | Yes | Warning only, should be hard failure |
| Rule evaluation | Yes | Checks metrics |
| OnEnter state transitions | Yes | Minimum threshold check |
| OnExit state transitions | Yes | Minimum threshold check |
| Rule add_triple action | **No** | Not verified in indexes |
| Rule remove_triple action | **No** | Not verified |
| Rule publish action | **No** | Not verified |
| **PathRAG queries** | **No** | Critical gap - Tier 0 feature |
| Predicate index queries | **No** | Index populated but not queried |
| Incoming/Outgoing queries | **No** | Indexes populated but not queried |
| Alias resolution | **No** | Not tested |
| Entity storage | Yes | Count and retrieval validated |
| 7 core indexes populated | Yes | Strong validation |
| Prometheus metrics | Yes | Required metrics checked |

---

## Test Configuration

```go
// Structural tier defaults
ExpectedEmbeddings: 0,  // ZERO embeddings
ExpectedClusters:   0,  // ZERO clustering
MinRulesEvaluated:  5,
MinOnEnterFired:    2,
MinOnExitFired:     1,
```

---

## Conclusion

**Overall Assessment**: The `tiered --variant structural` scenario provides **good baseline validation** of the structural tier, correctly enforcing zero-ML constraints and validating rule state transitions. However, it has a **critical gap**: PathRAG is not tested despite being a documented Tier 0 feature.

**Risk**: PathRAG could be completely broken and this test would still pass.

**Recommendations**:
1. **High priority**: Add PathRAG e2e test
2. **High priority**: Make zero-constraints hard failures (not warnings)
3. **Medium priority**: Validate rule graph modifications in indexes
4. **Medium priority**: Test predicate/relationship index queries
