# Indexes

SemStreams maintains seven indexes. Understanding which indexes affect community detection helps you design triples that enable the queries you need.

## The Seven Indexes

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

## Developer Impact: Triple Design

Your triple design directly affects what's discoverable and how entities cluster.

### Entity References Create Edges

```go
// This creates a traversable edge (affects clustering):
Triple{Predicate: "fleet.membership", Object: "acme.robotics.fleet.rescue"}

// This is just a property (no edge, no clustering impact):
Triple{Predicate: "fleet.name", Object: "Rescue Fleet"}
```

The difference: Is the Object another entity ID, or just a value?

### Predicates Are Query Keys

```go
// All entities with this predicate are findable via PREDICATE_INDEX:
Triple{Predicate: "sensor.measurement.temperature", Object: "72.5"}
```

**Critical:** Inconsistent naming fragments your index:

```go
// These are THREE separate index entries:
"temp"                          → entities in bucket 1
"temperature"                   → entities in bucket 2
"sensor.measurement.temperature" → entities in bucket 3
```

Use consistent, hierarchical predicate names.

### Relationships Build Communities

```
Drone-1 → references → Fleet-A    (via fleet.membership triple)
Drone-2 → references → Fleet-A    (via fleet.membership triple)
Sensor-X → references → Drone-1   (via equipment.parent triple)

LPA traverses these edges:
  GetNeighbors(Drone-1) → [Fleet-A, Sensor-X]
  GetNeighbors(Drone-2) → [Fleet-A]
  GetNeighbors(Fleet-A) → [Drone-1, Drone-2]

Result: All four entities cluster together
```

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

## Best Practices

### 1. Consistent Predicate Naming

Use hierarchical, consistent names:

```go
// Good
"sensor.measurement.temperature"
"sensor.measurement.humidity"
"sensor.status.battery"

// Bad - fragments the index
"temp"
"Humidity"
"battery_status"
```

### 2. Explicit Entity References

When you want entities to cluster, create explicit relationships:

```go
// Creates clustering edge
Triple{Predicate: "fleet.membership", Object: fleetEntityID}

// No clustering edge (just a property)
Triple{Predicate: "fleet.membership", Object: "Fleet Alpha"}
```

### 3. Alias for Human-Friendly Names

```go
// Store canonical ID
Triple{Predicate: "entity.id", Object: "acme.robotics.aerial.drone.drone-007"}

// Add alias for human queries
Triple{Predicate: "alias.name", Object: "drone-alpha"}
```

### 4. Geo Predicates for Spatial

```go
// Enables spatial index
Triple{Predicate: "geo.position.latitude", Object: "37.7749"}
Triple{Predicate: "geo.position.longitude", Object: "-122.4194"}
// Or combined geohash
Triple{Predicate: "geo.position.hash", Object: "9q8yy"}
```

### 5. Text Content for Embeddings

```go
func (d *Drone) TextContent() string {
    return fmt.Sprintf("Drone %s operating in %s zone. Battery: %d%%. Status: %s",
        d.DroneID, d.Zone, d.Battery, d.Status)
}
```

This feeds embedding generation and TF-IDF keyword extraction.

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

- [Communities](04-communities.md) - How indexes enable clustering
- [Triples](02-triples.md) - Design triples that feed indexes
- [Vocabulary](../basics/04-vocabulary.md) - Predicate naming conventions
