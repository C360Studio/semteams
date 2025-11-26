# TODO: Graph Indexing Architectural Issues

**Status:** Open
**Priority:** Medium
**Related:** ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md, ADR-TEMPORAL-GRAPH-MODELING.md, TODO-PREDICATE-NOTATION-CONSISTENCY.md

This document captures architectural inconsistencies and improvement opportunities discovered during documentation audit.

---

## Problem 1: Mutation API Inconsistency

There's an architectural inconsistency between Rules Engine and Community Detection:

| Component | Computed Data | Storage Method |
|-----------|--------------|----------------|
| **Rules Engine** | Inferred relationships, alerts | Uses `graph.mutation.*` API → writes to ENTITY_STATES |
| **Community Detection** | Community membership | Writes directly to COMMUNITY_INDEX → separate from entities |

### Rules Engine Pattern

When a rule triggers, it can mutate the graph via NATS:

```go
// Rules Engine publishes to:
"graph.mutation.entity.create"
"graph.mutation.entity.update"
"graph.mutation.edge.add"

// Graph Processor subscribes and writes to ENTITY_STATES
```

This means rule-inferred relationships become queryable triples/edges on entities.

### Community Detection Pattern

When LPA detects communities, it writes to a separate index:

```go
// Community Detection writes directly:
COMMUNITY_INDEX: "graph.community.0.comm-0-auth" → Community JSON
COMMUNITY_INDEX: "graph.community.entity.0.drone.001" → "comm-0-auth"
```

Community membership is NOT queryable as a triple or edge on the entity.

---

## Why This Matters

1. **Query Inconsistency**
   - Rule-inferred relationships: queryable via PREDICATE_INDEX, INCOMING_INDEX
   - Community membership: requires separate COMMUNITY_INDEX query

2. **Mental Model**
   - Users expect "drone.001 is member of community-auth" to be a relationship
   - But it's not - it's a separate index lookup

3. **PathRAG vs GraphRAG Gap**
   - PathRAG traverses relationships (triples/edges)
   - Community membership is invisible to PathRAG

---

## Proposed Fix

Community Detection should use the mutation API to create relationship triples:

```go
// After LPA detects community membership:
mutation := gtypes.AddEdgeRequest{
    FromEntityID: entityID,
    ToEntityID:   communityID,  // Community as pseudo-entity
    EdgeType:     "graph.community.member_of",
    Properties: map[string]any{
        "level":      level,
        "confidence": communityScore,
    },
}

// Publish via mutation API
nc.Publish("graph.mutation.edge.add", mutationJSON)
```

Or create triples:

```go
triple := message.Triple{
    Subject:    entityID,
    Predicate:  "graph.community.member_of",
    Object:     communityID,
    Source:     "lpa_detection",
    Timestamp:  time.Now(),
    Confidence: communityScore,
}
```

---

## Considerations

### Arguments For

1. **Consistency** - All relationships in one model
2. **PathRAG integration** - Community membership becomes traversable
3. **Unified queries** - No separate index query needed
4. **ADR alignment** - Triples as single source of truth

### Arguments Against

1. **Volume** - Community membership changes frequently, lots of writes
2. **Transience** - Communities are recomputed, not user-declared facts
3. **Separation of concerns** - Computed vs declared data
4. **Performance** - COMMUNITY_INDEX is optimized for this use case

### Compromise Option

Keep COMMUNITY_INDEX for fast community lookups, but ALSO create triples for entities that want PathRAG integration:

```json
{
  "community_detection": {
    "create_triples": false,  // Default: just use COMMUNITY_INDEX
    "triple_predicate": "graph.community.member_of"  // If enabled
  }
}
```

---

## Decision

**Pending architectural review.**

This inconsistency was discovered during documentation audit. The current design works but creates a split in how inferred/computed relationships are stored and queried.

---

## Problem 2: Index Management Inconsistency

COMMUNITY_INDEX is managed differently from all other indexes:

| Index | Managed By | Package |
|-------|-----------|---------|
| PREDICATE_INDEX | IndexManager | `processor/graph/indexmanager` |
| INCOMING_INDEX | IndexManager | `processor/graph/indexmanager` |
| ALIAS_INDEX | IndexManager | `processor/graph/indexmanager` |
| SPATIAL_INDEX | IndexManager | `processor/graph/indexmanager` |
| TEMPORAL_INDEX | IndexManager | `processor/graph/indexmanager` |
| **COMMUNITY_INDEX** | **NATSCommunityStorage** | **`pkg/graphclustering`** |

### Current Architecture

```
processor/graph/indexmanager/
├── manager.go          # Manages PREDICATE, INCOMING, ALIAS, SPATIAL, TEMPORAL
├── indexes.go          # Index implementations
└── ...

pkg/graphclustering/
├── storage.go          # NATSCommunityStorage - manages COMMUNITY_INDEX directly
├── lpa.go              # LPADetector - writes to storage
├── enhancement_worker.go # Watches COMMUNITY_INDEX for LLM enhancement
└── ...
```

### Why This Matters

1. **Inconsistent lifecycle** - IndexManager has unified start/stop/health, graphclustering is separate
2. **Configuration fragmentation** - Index config in one place, community config elsewhere
3. **Developer confusion** - "Where do I look for index X?"
4. **Testing complexity** - Different patterns for testing different indexes

---

## Proposed Options

### Option A: Move COMMUNITY_INDEX to IndexManager

IndexManager owns all index buckets, delegates computation to graphclustering:

```go
// indexmanager/manager.go
type Manager struct {
    // ... existing indexes
    communityIndex *CommunityIndex  // New
}

// indexmanager/community_index.go
type CommunityIndex struct {
    storage  *graphclustering.NATSCommunityStorage
    detector *graphclustering.LPADetector
}
```

**Pros:** Unified index management, consistent patterns
**Cons:** IndexManager grows larger, tighter coupling

### Option B: Keep Separate, Unify Interface

Define a common `Index` interface that both IndexManager indexes and graphclustering implement:

```go
type Index interface {
    Name() string
    Start(ctx context.Context) error
    Stop() error
    Health() HealthStatus
    Rebuild(ctx context.Context) error
}
```

**Pros:** Loose coupling, separation of concerns
**Cons:** Still two places to look, coordination overhead

### Option C: Promote graphclustering to processor

Make graphclustering a processor like `processor/rule`:

```
processor/
├── graph/
│   └── indexmanager/   # Core indexes
├── rule/               # Rules Engine processor
└── graphrag/           # Community detection processor (renamed from pkg/graphclustering)
```

**Pros:** Clear processor boundary, consistent with Rules Engine
**Cons:** More restructuring, may be overkill

---

## Recommendation

**Option A (Move to IndexManager)** seems cleanest because:

1. All indexes in one place
2. Unified configuration
3. Consistent lifecycle management
4. graphclustering becomes a library (algorithms only), not a storage owner

The key insight: `pkg/graphclustering` should provide **algorithms** (LPA, PageRank, summarization), not **storage management**. Storage is IndexManager's job.

---

## Problem 3: Index Entry Provenance

Index entries don't track which entity state revision caused them to be created/updated.

### Current State

```go
// PREDICATE_INDEX stores just entity ID lists:
Key: "ops.fleet.member_of:acme.ops.logistics.hq.fleet.rescue"
Value: ["acme.telemetry.robotics.gcs1.drone.001", "acme.telemetry.robotics.gcs1.drone.002"]
```

- **EntityChange event**: Tracks source entity revision (for dedup during processing)
- **Index entries**: Just `[]string` (entity IDs only)
- **CAS operations**: Uses index key revision (for optimistic concurrency), not source entity revision

No way to answer: "Which entity state revision caused this index entry?"

### Why This Matters

1. **Consistency verification**: Can't verify if index is in sync with entity state revision N
2. **Debugging**: Can't trace index entries back to source mutations
3. **Stale detection**: Can't identify index entries from outdated entity states
4. **Audit trail**: No provenance linking index to source

### Proposed Options

#### Option A: Store Entity + Revision Pairs

```go
type IndexedEntity struct {
    EntityID       string    `json:"entity_id"`
    SourceRevision uint64    `json:"source_revision"`  // EntityState revision
    IndexedAt      time.Time `json:"indexed_at"`
}

// Index value becomes []IndexedEntity instead of []string
```

**Pros:** Complete provenance, enables consistency checks
**Cons:** 3-4x storage increase, more complex queries

#### Option B: Store Revision Map Alongside Entity List

```go
type IndexEntry struct {
    Entities        []string          `json:"entities"`
    LastUpdated     time.Time         `json:"last_updated"`
    EntityRevisions map[string]uint64 `json:"entity_revisions"` // entityID → source revision
}
```

**Pros:** Backward compatible structure, rich metadata
**Cons:** Larger payloads, map overhead

#### Option C: Use NATS KV History for Index Keys

NATS KV supports `GetRevision()` to query historical index states.

```go
// Query index state at specific revision
entry, _ := bucket.GetRevision(ctx, key, revision)
```

**Pros:** Already available, no schema change
**Cons:** Only tracks index history, not source entity linkage

#### Option D: Separate Provenance Index

Create a dedicated provenance bucket:

```
INDEX_PROVENANCE bucket:
Key: "PREDICATE_INDEX:ops.fleet.member_of:acme.ops.logistics.hq.fleet.rescue"
Value: {
    "last_updated": "2024-01-15T10:30:00Z",
    "entity_revisions": {
        "drone.001": 42,
        "drone.002": 38
    }
}
```

**Pros:** Clean separation, optional feature
**Cons:** Additional bucket, sync complexity

---

### Recommendation

**Option B (Revision Map)** is the best balance:

1. Maintains simple `[]string` for fast queries
2. Adds provenance metadata alongside
3. Enables consistency verification without separate lookups
4. Backward compatible - old code can ignore new fields

However, this is **low priority** unless debugging index consistency becomes a problem.

---

## References

- `processor/graph/mutations.go` - Graph mutation API
- `processor/graph/indexmanager/` - Index management (5 indexes)
- `pkg/graphclustering/storage.go` - Community storage (separate)
- `processor/rule/` - Rules Engine (uses mutation API)
- `docs/graph/04-indexing.md` - Index documentation
- `processor/graph/indexmanager/interface.go` - EntityChange with Revision field
