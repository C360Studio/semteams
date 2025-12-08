# Architecture Research Findings

Technical discoveries from architecture exploration. Reference document for future development.

*Research conducted: December 2024*

## 1. GraphProvider Interface and Edge Weights

**File:** `processor/graph/clustering/provider.go`

The `GraphProvider` interface is what LPA clustering uses to traverse the graph:

```go
type GraphProvider interface {
    GetAllEntityIDs(ctx context.Context) ([]string, error)
    GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error)
    GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error)
}
```

**Finding:** Base providers always return `1.0` for edge weights:

```go
// From PredicateGraphProvider.GetEdgeWeight():
func (p *PredicateGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
    // ... lookup logic ...
    for _, rel := range rels {
        if rel.ToEntityID == toID {
            return 1.0, nil  // <-- ALWAYS 1.0
        }
    }
    return 0.0, nil
}
```

**Implication:** LPA treats all explicit edges equally. Future enhancement: weight by relationship type or frequency.

## 2. SemanticGraphProvider Pattern (Virtual Edges)

**File:** `processor/graph/clustering/semantic_provider.go`

Wraps base provider, adds virtual edges from embedding similarity:

```go
type SemanticGraphProvider struct {
    base                GraphProvider
    embeddingStore      EmbeddingStore
    entityStore         EntityStore
    similarityThreshold float64
    maxVirtualNeighbors int
    virtualEdges        map[string][]virtualEdge  // Cached
}
```

**Key pattern - GetNeighbors combines explicit + virtual:**

```go
func (p *SemanticGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
    // 1. Get explicit neighbors from base provider
    neighbors, err := p.base.GetNeighbors(ctx, entityID, direction)

    // 2. Add virtual neighbors from embedding similarity
    if virtualNeighbors, ok := p.virtualEdges[entityID]; ok {
        for _, vn := range virtualNeighbors {
            neighbors = append(neighbors, vn.targetID)
        }
    }
    return neighbors, nil
}
```

**Key pattern - GetEdgeWeight returns actual similarity:**

```go
func (p *SemanticGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
    // Explicit edge takes precedence
    weight, err := p.base.GetEdgeWeight(ctx, fromID, toID)
    if weight > 0 {
        return weight, nil
    }
    // Virtual edge returns actual similarity score
    return cachedSimilarity, nil  // e.g., 0.85
}
```

**Architecture insight:** This is the pattern for Spatial/Temporal providers.

## 3. Index Usage in Clustering (Actual vs Theoretical)

**Investigation Results:**

| Index | Used in Clustering? | How? | Evidence |
|-------|---------------------|------|----------|
| `PREDICATE_INDEX` | YES | Entity filtering | `PredicateGraphProvider.GetNeighbors()` |
| `INCOMING_INDEX` | YES | Edge traversal | `IncomingGraphProvider` exists |
| `OUTGOING_INDEX` | YES | Edge traversal | `OutgoingGraphProvider` exists |
| `EMBEDDING_INDEX` | YES | Virtual edges | `SemanticGraphProvider` uses it |
| `SPATIAL_INDEX` | **NO** | Not used | No `SpatialGraphProvider` |
| `TEMPORAL_INDEX` | **NO** | Not used | No `TemporalGraphProvider` |
| `ALIAS_INDEX` | NO | ID resolution only | Not clustering-relevant |

**Assessment:** Spatial/Temporal gaps are NOT bad design - architecture supports it via the wrapper pattern.

## 4. Rules Engine Actions

**File:** `processor/rule/actions.go`

```go
const (
    ActionTypePublish      = "publish"
    ActionTypeAddTriple    = "add_triple"
    ActionTypeRemoveTriple = "remove_triple"
    ActionTypeUpdateTriple = "update_triple"
)
```

**Rules build the graph:**

1. Rule condition evaluates to true (OnEnter)
2. `add_triple` creates: `sensor.fleet_membership → fleet-123`
3. Triple stored in ENTITY_STATES
4. IndexManager updates OUTGOING_INDEX
5. LPA `GetNeighbors()` returns fleet-123
6. Sensor clusters with fleet-123's community

## 5. LPA Community Detection Flow

**Files:** `processor/graph/clustering/lpa.go`, `enhancement_worker.go`

```
1. Entity changes accumulate (entityChangeCount > threshold)
2. DetectCommunities() called
3. EnhancementWorker.Pause()
4. LPA runs:
   - For each level (0 → N):
     - Initialize labels (each entity = own label)
     - Iterate until convergence
     - Group by label → Communities
5. Jaccard matching - preserve summaries if membership ≥ 80%
6. Statistical summarization (immediate, ~1ms)
7. Save to COMMUNITY_INDEX with status="statistical"
8. EnhancementWorker.Resume()
9. EnhancementWorker picks up (async, 1-5s/community)
```

## 6. Known Gaps and Enhancement Paths

### Gap 1: Spatial Clustering

**Current:** SPATIAL_INDEX exists, populated from `geo.*` predicates
**Missing:** `SpatialGraphProvider` to add virtual edges by proximity
**Pattern:** Same as SemanticGraphProvider

### Gap 2: Temporal Clustering

**Current:** TEMPORAL_INDEX exists, populated from timestamp predicates
**Missing:** `TemporalGraphProvider` to add virtual edges by time correlation
**Pattern:** Same as SemanticGraphProvider

### Gap 3: Edge Weighting

**Current:** All explicit edges return weight 1.0
**Enhancement:** Weight by relationship type, frequency, or explicit weight predicate

### Gap 4: LLM Spatial/Temporal Context

**Current:** Community summaries don't include geo/time context
**Enhancement:** Include spatial extent and temporal range in LLM prompts

## 7. File Reference

| File | Contents |
|------|----------|
| `processor/graph/clustering/provider.go` | All GraphProvider implementations |
| `processor/graph/clustering/semantic_provider.go` | Virtual edge pattern |
| `processor/graph/clustering/lpa.go` | Label Propagation Algorithm |
| `processor/graph/clustering/enhancement_worker.go` | Async LLM enhancement |
| `processor/graph/indexmanager/README.md` | All 7 indexes documented |
| `processor/rule/README.md` | Rules engine documentation |
| `processor/rule/actions.go` | Action type constants |
| `docs/architecture/graph-processor.md` | Architecture diagram |
| `docs/e2e/tiers.md` | Three-tier architecture |

## 8. Three-Tier Architecture

| Tier | Components | Graph Building |
|------|------------|----------------|
| **Tier 0** | Rules only | Explicit edges from rules + triples |
| **Tier 1** | + BM25 + Statistical | + Statistical community summaries |
| **Tier 2** | + Neural + LLM | + Virtual edges + LLM summaries |

**Key insight:** Start with Tier 0 (just NATS), add tiers progressively.

## 9. Future Enhancement Template

Adding a new GraphProvider wrapper (e.g., Spatial):

```go
type SpatialGraphProvider struct {
    base      GraphProvider
    spatial   SpatialIndex
    proximity float64
}

func (p *SpatialGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
    neighbors, _ := p.base.GetNeighbors(ctx, entityID, direction)
    nearby, _ := p.spatial.GetNearby(ctx, entityID, p.proximity)
    return append(neighbors, nearby...), nil
}

func (p *SpatialGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
    weight, _ := p.base.GetEdgeWeight(ctx, fromID, toID)
    if weight > 0 {
        return weight, nil
    }
    // Return proximity score as weight
    return p.spatial.GetProximityScore(ctx, fromID, toID)
}
```

---

*Files examined: ~15 source files across processor/graph/clustering/, processor/rule/, processor/graph/indexmanager/*
