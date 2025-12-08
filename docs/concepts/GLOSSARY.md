# Glossary

Terms used throughout SemStreams documentation and code.

## Core Concepts

### Entity

A uniquely identified object in the graph. Entities have:
- **ID**: Hierarchical identifier (e.g., `acme.robotics.drone.drone-007`)
- **Triples**: Facts about the entity
- **Version**: For concurrency control

### Triple

A single fact about an entity: `{Predicate, Object}`. The Subject is always the entity itself.

Example: `drone-007` has triple `{battery.level, 78}`.

### Predicate

The "type" of a fact. Used for querying and indexing.

Examples: `entity.type`, `sensor.measurement.temperature`, `fleet.membership`

### Object

The "value" of a fact. Can be:
- **Value**: Just data (`"78"`, `"active"`)
- **Entity reference**: Creates a relationship (`"acme.robotics.fleet.rescue"`)

### Graphable

The interface your processor implements:

```go
type Graphable interface {
    EntityID() string           // Unique identifier
    Triples() []Triple          // Facts about this entity
    TextContent() string        // Text for embeddings
    Embedding() []float32       // Pre-computed embedding
}
```

## Graph Structure

### Edge

A relationship between two entities. Created when a triple's Object is another entity ID.

### Neighbor

An entity connected by an edge. Can be:
- **Incoming**: Entities that reference this one
- **Outgoing**: Entities this one references

### Community

A group of entities more connected to each other than to the rest of the graph. Detected automatically by LPA.

### Level

Hierarchical community granularity:
- **Level 0**: Finest (smallest communities)
- **Level 1+**: Coarser (aggregated communities)

## Detection and Clustering

### LPA (Label Propagation Algorithm)

Algorithm that detects communities by iteratively assigning entities to their neighbors' dominant label.

### Jaccard Similarity

Measures membership overlap between old and new communities: `|intersection| / |union|`. Used to preserve summaries.

### PageRank

Algorithm that identifies "important" entities based on incoming edges. Used for representative entity selection.

### TF-IDF

Text analysis method for keyword extraction. Identifies distinctive terms across entity content.

## Enhancement

### Statistical Summary

Immediately generated summary using TF-IDF keywords and PageRank. No LLM required.

### LLM Summary

AI-generated narrative from statistical summary + entity data. Async, 1-5 seconds.

### Enhancement Window

Period after detection when new detections are paused, allowing LLM enhancement to complete.

### Summary Status

| Status | Meaning |
|--------|---------|
| `statistical` | Initial summary, awaiting LLM |
| `llm-enhanced` | LLM narrative complete |
| `llm-failed` | LLM unavailable, using fallback |

## Infrastructure

### KV Bucket

NATS JetStream Key-Value store. Used for entities, indexes, communities.

### KV Watcher

Pattern that triggers on KV changes. Used by IndexManager and EnhancementWorker.

### GraphProvider

Interface for traversing the graph:

```go
type GraphProvider interface {
    GetAllEntityIDs(ctx context.Context) ([]string, error)
    GetNeighbors(ctx context.Context, entityID, direction string) ([]string, error)
    GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error)
}
```

### SemanticGraphProvider

GraphProvider wrapper that adds virtual edges from embedding similarity.

## Indexes

### PREDICATE_INDEX

Maps predicate → entity IDs. "Find all entities with this property."

### INCOMING_INDEX

Maps entity → entities that reference it. "Who points to me?"

### OUTGOING_INDEX

Maps entity → entities it references. "What do I point to?"

### ALIAS_INDEX

Maps friendly name → canonical entity ID. "Resolve human-readable name."

### SPATIAL_INDEX

Maps geohash → entities. "Find entities near this location."

### TEMPORAL_INDEX

Maps timestamp range → entities. "Find entities in this time period."

### EMBEDDING_INDEX

Maps entity → vector embedding. "Find semantically similar entities."

## Tiers

### Tier 0

Rules only. No clustering, no embeddings. Just explicit relationships.

### Tier 1

Rules + BM25 search + statistical clustering. No LLM, no neural embeddings.

### Tier 2

Full stack: Rules + neural embeddings + LLM summaries + semantic clustering.

## Rules

### Rule

Condition + actions that fire when entity state matches.

### on_enter

Actions that fire when condition becomes true.

### on_exit

Actions that fire when condition becomes false.

### add_triple

Rule action that creates a new triple on the entity.

### remove_triple

Rule action that removes a triple from the entity.

## Processing

### Processor

Component that transforms domain messages into graph entities.

### MessageManager

Handles message → entity transformation.

### DataManager

Handles entity + triple CRUD operations.

### IndexManager

Maintains all 7 indexes via KV watchers.

### QueryManager

Provides graph queries with caching.

### EnhancementWorker

Async worker that generates LLM summaries.

## Entity ID Format

Recommended hierarchical format:

```
{org}.{platform}.{domain}.{system}.{type}.{instance}
```

Example: `acme.robotics.aerial.fleet.drone.drone-007`

Benefits:
- Federation across organizations
- Queryable by prefix
- Obvious entity type
- Globally unique
