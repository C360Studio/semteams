# Claude Code Instructions: Structural Indexing and Inference

## Quick Reference

**Spec Document**: `SPEC-structural-indexing-inference.md`

**Feature**: Add structural graph indexing (k-core, pivot distances) with inference detection and LLM-assisted review.

## Implementation Phases

### Phase 1: Structural Index Core ← START HERE

Create `processor/graph/structuralindex/` package:

```bash
mkdir -p processor/graph/structuralindex
```

**Files to create (in order)**:
1. `types.go` - KCoreIndex, PivotIndex, StructuralIndices structs
2. `kcore.go` - KCoreComputer with peeling algorithm
3. `kcore_test.go` - Unit tests for k-core
4. `pivot.go` - PivotComputer with PageRank pivot selection + BFS
5. `pivot_test.go` - Unit tests for pivot index
6. `provider.go` - StructuralGraphProvider wrapper
7. `storage.go` - NATSStructuralIndexStorage
8. `README.md`

**Key interfaces to implement**:
```go
// KCoreIndex methods
func (idx *KCoreIndex) GetCore(entityID string) int
func (idx *KCoreIndex) FilterByMinCore(entityIDs []string, minCore int) []string
func (idx *KCoreIndex) GetEntitiesAboveCore(minCore int) []string

// PivotIndex methods  
func (idx *PivotIndex) EstimateDistance(entityA, entityB string) (lower, upper int)
func (idx *PivotIndex) IsWithinHops(entityA, entityB string, maxHops int) bool
func (idx *PivotIndex) GetReachableCandidates(source string, maxHops int) []string
```

**Use existing patterns from**:
- `processor/graph/clustering/lpa.go` - Algorithm structure
- `processor/graph/clustering/types.go` - GraphProvider interface
- `processor/graph/clustering/storage.go` - NATS KV patterns

### Phase 2: Query Integration

Modify existing files:
- `processor/graph/indexmanager/semantic.go` - Add k-core filtering to SearchSemantic
- `processor/graph/querymanager/query.go` - Add pivot pruning to ExecutePath
- `processor/graph/processor.go` - Add config and computation calls

### Phase 3: Inference Detection

Create `processor/graph/inference/` package:

```bash
mkdir -p processor/graph/inference
```

**Files to create**:
1. `types.go` - AnomalyType, AnomalyStatus, StructuralAnomaly, RelationshipSuggestion
2. `detector.go` - DetectorOrchestrator
3. `semantic_gap.go` - SemanticStructuralGapDetector
4. `core_anomaly.go` - CoreAnomalyDetector
5. `transitivity.go` - TransitivityGapDetector
6. `storage.go` - NATSAnomalyStorage (ANOMALY_INDEX bucket)
7. `applier.go` - RelationshipApplier implementation
8. `detector_test.go`
9. `README.md`

**Use existing patterns from**:
- `processor/graph/clustering/lpa.go` - InferredTriple, InferRelationshipsFromCommunities
- `processor/graph/indexmanager/semantic.go` - SimilarityFinder interface

### Phase 4: LLM Review Worker

Add to `processor/graph/inference/`:

1. `review_worker.go` - ReviewWorker (follow EnhancementWorker pattern)
2. `http_handlers.go` - Human review API endpoints
3. `review_worker_test.go`

**Use existing patterns from**:
- `processor/graph/clustering/enhancement_worker.go` - KV watcher, pause/resume
- `processor/graph/llm/client.go` - LLM client interface

## Key Algorithms

### K-Core (Peeling Algorithm)
```
1. Compute degree for all vertices
2. Sort by degree ascending
3. For each vertex v in order:
   - core[v] = current degree[v]
   - Mark v removed
   - Decrease degree of remaining neighbors
```

### Pivot Distance (Triangle Inequality)
```
Lower bound: max over all pivots of |d(A,pivot) - d(B,pivot)|
Upper bound: min over all pivots of d(A,pivot) + d(B,pivot)
```

### Semantic-Structural Gap Detection
```
For similar entities (embedding similarity > 0.7):
  If structural distance (pivot estimate) > 3 hops:
    → Anomaly: should probably be connected
```

## Configuration Addition

Add to ClusteringConfig in `processor/graph/processor.go`:

```go
type StructuralIndexConfig struct {
    Enabled bool        `json:"enabled"`
    KCore   KCoreConfig `json:"kcore"`
    Pivot   PivotConfig `json:"pivot"`
}

type StructuralInferenceConfig struct {
    Enabled                   bool                `json:"enabled"`
    RunWithCommunityDetection bool                `json:"run_with_community_detection"`
    MaxAnomaliesPerRun        int                 `json:"max_anomalies_per_run"`
    SemanticStructuralGap     SemanticGapConfig   `json:"semantic_structural_gap"`
    CoreInference             CoreInferenceConfig `json:"core_inference"`
    TransitivityGap           TransitivityConfig  `json:"transitivity_gap"`
    Review                    ReviewConfig        `json:"review"`
}
```

## KV Buckets

**STRUCTURAL_INDEX** (new):
- `kcore._meta` - Metadata
- `kcore.entity.{id}` - Per-entity core number
- `pivot._meta` - Metadata  
- `pivot.entity.{id}` - Distance vector

**ANOMALY_INDEX** (new):
- `anomaly.{uuid}` - Full anomaly document

## Testing Strategy

1. **Unit tests**: Test algorithms on small known graphs
2. **Integration tests**: Use testcontainers with real NATS
3. **Test fixtures**: Create `testdata/` with known graph structures

Example test graph for k-core:
```
A--B--C--D--E
|  |  |
F--G--H
```
Expected: A,B,C,G,H are core-2; D,E,F are core-1

## Dependencies

No new external dependencies. Uses:
- Existing `clustering.GraphProvider` interface
- Existing `llm.Client` interface  
- Existing NATS KV patterns

## Error Handling

Follow existing patterns:
```go
return errs.WrapTransient(err, "KCoreComputer", "Compute", "failed to get neighbors")
return errs.WrapInvalid(err, "PivotIndex", "EstimateDistance", "entity not in index")
```

## Metrics

Add Prometheus metrics following existing patterns in `processor/graph/indexmanager/metrics.go`:
- `structural_index_computation_duration_seconds`
- `inference_anomalies_detected_total`
- `inference_review_worker_*`

## Review Flow Summary

```
Detection → ANOMALY_INDEX (pending) → ReviewWorker → LLM Decision
                                                    ↓
                            ┌───────────────────────┼───────────────────────┐
                            ↓                       ↓                       ↓
                     Auto-Apply              Human Review              Auto-Reject
                   (conf ≥ 0.9)             (0.3 < conf < 0.9)        (conf ≤ 0.3)
```

## Commands for Development

```bash
# Run tests for new packages
go test ./processor/graph/structuralindex/... -v
go test ./processor/graph/inference/... -v

# Run with race detector
go test ./processor/graph/structuralindex/... -race

# Integration tests
go test ./processor/graph/... -tags=integration -v

# Lint
revive ./processor/graph/structuralindex/...
revive ./processor/graph/inference/...
```

## Checklist

### Phase 1
- [ ] `structuralindex/types.go` with KCoreIndex, PivotIndex
- [ ] `structuralindex/kcore.go` with KCoreComputer
- [ ] `structuralindex/kcore_test.go` passing
- [ ] `structuralindex/pivot.go` with PivotComputer  
- [ ] `structuralindex/pivot_test.go` passing
- [ ] `structuralindex/provider.go` with StructuralGraphProvider
- [ ] `structuralindex/storage.go` with NATS KV persistence
- [ ] `structuralindex/README.md`

### Phase 2
- [ ] K-core filtering in IndexManager.SearchSemantic
- [ ] Pivot pruning in QueryManager.ExecutePath
- [ ] Config structs added to processor.go
- [ ] Computation integrated into detectCommunities flow

### Phase 3
- [ ] `inference/types.go` with anomaly types
- [ ] `inference/detector.go` with orchestrator
- [ ] `inference/semantic_gap.go` detector
- [ ] `inference/core_anomaly.go` detector
- [ ] `inference/transitivity.go` detector
- [ ] `inference/storage.go` with NATS KV
- [ ] `inference/applier.go` for relationship creation
- [ ] `inference/detector_test.go` passing
- [ ] `inference/README.md`

### Phase 4
- [ ] `inference/review_worker.go` following EnhancementWorker pattern
- [ ] `inference/http_handlers.go` for human review API
- [ ] `inference/review_worker_test.go` passing
- [ ] Integration with processor.go
- [ ] Metrics added

### Final
- [ ] All tests passing
- [ ] Integration tests passing
- [ ] No race conditions (`go test -race`)
- [ ] Lint clean
- [ ] Documentation complete
