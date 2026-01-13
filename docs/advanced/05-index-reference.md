# Index Reference

SemStreams maintains multiple indexes for graph traversal and query. Understanding which indexes affect community detection helps you design triples that enable the queries you need. For the full list of KV buckets, see [KV Buckets Reference](../reference/kv-buckets.md).

## Core Indexes

| Index | Purpose | Affects Clustering? | LLM Context? |
|-------|---------|---------------------|--------------|
| **PREDICATE_INDEX** | "Find entities with this property" | YES | No |
| **INCOMING_INDEX** | "Who references this entity?" | YES | No |
| **OUTGOING_INDEX** | "What does this entity reference?" | YES | No |
| **EMBEDDING_INDEX** | Semantic similarity search | YES (Tier 2) | No |
| **ALIAS_INDEX** | Resolve friendly names | No | No |
| **SPATIAL_INDEX** | Geographic queries | No* | No* |
| **TEMPORAL_INDEX** | Time-range queries | No* | No* |

*Future enhancement - architecture supports it, not yet implemented.

## Which Indexes Feed Community Detection?

### Used by LPA

The Label Propagation Algorithm traverses edges via:

1. **OUTGOING_INDEX**: "What entities does this entity reference?"
2. **INCOMING_INDEX**: "What entities reference this entity?"
3. **PREDICATE_INDEX**: Entity filtering by property

### Used by SemanticGraphProvider (Tier 2)

4. **EMBEDDING_INDEX**: Creates virtual edges between semantically similar entities

### NOT Used in Clustering

5. **ALIAS_INDEX**: ID resolution only
6. **SPATIAL_INDEX**: Index exists, no provider implementation
7. **TEMPORAL_INDEX**: Index exists, no provider implementation

## Edge Weights

### Base Providers: Always 1.0

```go
// PredicateGraphProvider, OutgoingGraphProvider, IncomingGraphProvider
func (p *Provider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
    // ... edge lookup ...
    return 1.0, nil  // All explicit edges weighted equally
}
```

LPA treats all explicit edges the same.

### SemanticGraphProvider: Actual Similarity

```go
// Virtual edges from embeddings get actual similarity scores
func (p *SemanticGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
    // Explicit edge takes precedence
    weight, _ := p.base.GetEdgeWeight(ctx, fromID, toID)
    if weight > 0 {
        return weight, nil
    }
    // Virtual edge returns actual similarity (e.g., 0.85)
    return cachedSimilarity, nil
}
```

## Index Details

### PREDICATE_INDEX

**Key pattern:** `{predicate}`
**Value:** List of entity IDs with that predicate

**Created by:** Any triple
**Used for:** "Find all sensors" → query predicate `entity.type` value `sensor`

### INCOMING_INDEX

**Key pattern:** `{entity_id}`
**Value:** List of entities that reference this entity

**Created by:** Triples where Object is another entity ID
**Used for:** "Who references fleet-A?" → all drones assigned to that fleet

### OUTGOING_INDEX

**Key pattern:** `{entity_id}`
**Value:** List of entities this entity references

**Created by:** Triples where Object is another entity ID
**Used for:** "What does drone-007 reference?" → its fleet, mission, etc.

### ALIAS_INDEX

**Key pattern:** `{alias}`
**Value:** Canonical entity ID

**Created by:** Triples with `alias.*` predicates
**Used for:** Resolve "drone-alpha" → "acme.robotics.aerial.drone.drone-007"

### SPATIAL_INDEX

**Key pattern:** Geohash-based
**Value:** Entities at that location

**Created by:** Triples with `geo.*` predicates containing lat/lon
**Used for:** "Find entities within bounding box"
**Gap:** Not fed to clustering or LLM context

### TEMPORAL_INDEX

**Key pattern:** Timestamp-based
**Value:** Entities at that time

**Created by:** Triples with timestamp values
**Used for:** "Find entities in time range"
**Gap:** Not fed to clustering or LLM context

### EMBEDDING_INDEX

**Key pattern:** Entity ID
**Value:** Vector embedding

**Created by:** `TextContent()` → embedding service → stored
**Used for:** Semantic similarity search, virtual edges in Tier 2

## Gap: Spatial/Temporal in Clustering

The architecture supports spatial/temporal clustering - it's just not implemented.

**Current state:**
- Indexes exist and are populated
- No `SpatialGraphProvider` or `TemporalGraphProvider`
- LLM summaries don't include geo/time context

**Future enhancement pattern** (proven by SemanticGraphProvider):

```go
type SpatialGraphProvider struct {
    base      GraphProvider
    spatial   SpatialIndex
    proximity float64  // e.g., 100 meters
}

func (p *SpatialGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
    // 1. Get explicit neighbors
    neighbors, _ := p.base.GetNeighbors(ctx, entityID, direction)

    // 2. Add spatially proximate entities as virtual neighbors
    nearby, _ := p.spatial.GetNearby(ctx, entityID, p.proximity)
    neighbors = append(neighbors, nearby...)

    return neighbors, nil
}
```

## Debugging Index Issues

```bash
# Check if predicate exists
nats kv get PREDICATE_INDEX "sensor.measurement.temperature"

# Check entity relationships
nats kv get OUTGOING_INDEX "drone-007"
nats kv get INCOMING_INDEX "fleet-warehouse-7"

# Check spatial coverage
nats kv keys SPATIAL_INDEX | head -20

# Check embedding exists
nats kv get EMBEDDING_INDEX "drone-007"
```

## Next Steps

- [Community Detection](../concepts/05-community-detection.md) - How indexes enable clustering
- [Vocabulary](../basics/04-vocabulary.md) - Predicate naming conventions
- [Clustering Configuration](01-clustering.md) - LPA and hierarchical detection
