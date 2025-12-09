# Glossary

Terms used throughout SemStreams documentation and code.

## Core Concepts

### Entity

A uniquely identified object in the graph. Entities have:
- **ID**: 6-part hierarchical identifier (e.g., `acme.robotics.aerial.fleet.drone.drone-007`)
- **Triples**: Facts about the entity
- **Version**: For optimistic concurrency control

### Triple

A single fact about an entity: `{Predicate, Object}`. The Subject is always the entity itself.

Example: Entity `drone-007` has triple `{battery.level, 78}`.

### Predicate

The "type" of a fact. Used for querying and indexing.

Examples: `entity.type`, `sensor.measurement.celsius`, `fleet.membership`

### Object

The "value" of a fact. Can be:
- **Value**: Scalar data (`78`, `"active"`, `true`)
- **Entity reference**: Creates a relationship (`acme.robotics.aerial.fleet.rescue`)

### Graphable

The interface your processor implements to transform domain messages into graph entities:

```go
type Graphable interface {
    EntityID() string           // 6-part hierarchical identifier
    Triples() []Triple          // Facts about this entity
    TextContent() string        // Text for embeddings (Tier 2)
    Embedding() []float32       // Pre-computed embedding (optional)
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

A group of entities more connected to each other than to the rest of the graph. Detected automatically by LPA algorithm.

### Level

Hierarchical community granularity:
- **Level 0**: Finest (smallest communities)
- **Level 1+**: Coarser (aggregated communities)

## Detection and Clustering

### LPA (Label Propagation Algorithm)

Algorithm that detects communities by iteratively assigning entities to their neighbors' dominant label. Fast and scalable.

### Jaccard Similarity

Measures membership overlap between old and new communities:

```
Jaccard = |intersection| / |union|
```

Used to preserve summaries when community membership changes slightly.

### PageRank

Algorithm that identifies "important" entities based on incoming edges. Used for selecting representative entities in communities.

### TF-IDF (Term Frequency-Inverse Document Frequency)

Text analysis method for keyword extraction. Identifies distinctive terms across entity content.

## Enhancement

### Statistical Summary

Immediately generated summary using TF-IDF keywords and PageRank. No LLM required. Always available.

### LLM Summary

AI-generated narrative from statistical summary + entity data. Async generation, typically 1-5 seconds.

### Enhancement Window

Period after detection when new detections are paused, allowing LLM enhancement to complete.

### Summary Status

| Status | Meaning |
|--------|---------|
| `statistical` | Initial summary, awaiting LLM |
| `llm-enhanced` | LLM narrative complete |
| `llm-failed` | LLM unavailable, using fallback |
| `statistical-fallback` | LLM disabled, using statistical |

## Infrastructure

### KV Bucket

NATS JetStream Key-Value store. Used for entities, indexes, communities, rule state.

### KV Watcher

Pattern that triggers callbacks on KV changes. Used by IndexManager and EnhancementWorker for real-time updates.

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

### ENTITY_STATES

Primary entity storage. Key pattern: `{entity_id}`

### PREDICATE_INDEX

Maps predicate to entity IDs. "Find all entities with this property."

### INCOMING_INDEX

Maps entity to entities that reference it. "Who points to me?"

### OUTGOING_INDEX

Maps entity to entities it references. "What do I point to?"

### COMMUNITY_INDEX

Stores community records. Key pattern: `graph.community.{community_id}`

### ALIAS_INDEX

Maps friendly name to canonical entity ID. "Resolve human-readable name."

### SPATIAL_INDEX

Maps geohash to entities. "Find entities near this location."

### TEMPORAL_INDEX

Maps timestamp range to entities. "Find entities in this time period."

### EMBEDDING_INDEX

Maps entity to vector embedding. "Find semantically similar entities."

## Tiers

### Tier 0

Rules only. No clustering, no embeddings. Just explicit relationships via rules. Minimal resources.

### Tier 1

Rules + BM25 search + statistical clustering. No LLM, no neural embeddings. Moderate resources.

### Tier 2

Full stack: Rules + neural embeddings + LLM summaries + semantic clustering. Requires LLM service.

## Rules

### Rule

Condition + actions that fire when entity state matches.

### on_enter

Actions that fire when condition becomes true (false → true transition).

### on_exit

Actions that fire when condition becomes false (true → false transition).

### while_true

Actions that fire on every evaluation while condition remains true.

### add_triple

Rule action that creates a new triple on the entity.

### remove_triple

Rule action that removes a triple from the entity.

### publish

Rule action that sends a message to a NATS subject.

## Processing Components

### Processor

Component that orchestrates message transformation and graph updates.

### MessageManager

Handles message → entity transformation via Graphable interface.

### DataManager

Handles entity + triple CRUD operations with optimistic concurrency.

### IndexManager

Maintains all secondary indexes via KV watchers.

### QueryManager

Provides graph queries with caching.

### EnhancementWorker

Async worker that generates LLM summaries for communities.

## Entity ID Format

6-part hierarchical format:

```
{org}.{platform}.{domain}.{system}.{type}.{instance}
```

Example: `acme.robotics.aerial.fleet.drone.drone-007`

| Segment | Purpose | Example |
|---------|---------|---------|
| org | Organization | `acme` |
| platform | Deployment | `robotics` |
| domain | Business area | `aerial` |
| system | Subsystem | `fleet` |
| type | Entity type | `drone` |
| instance | Unique ID | `drone-007` |

Benefits:
- Federation across organizations
- Queryable by prefix
- Obvious entity type from structure
- Globally unique
