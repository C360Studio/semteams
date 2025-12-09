# What is SemStreams?

SemStreams is a stream processor that builds a semantic knowledge graph from event data. You define a vocabulary of predicates for your domain, implement a simple interface, and the system maintains a living graph with automatic community detection.

## The Core Concept

```text
Your Events → Graphable Interface → Knowledge Graph → Queries
```

1. Events arrive (telemetry, records, notifications)
2. Your processor transforms them into entities with triples
3. SemStreams maintains the graph, indexes, and communities
4. Query by relationships, predicates, or semantic similarity

## What You Provide

### 1. A Predicate Vocabulary

Define the facts you want to capture about your domain:

```go
const (
    DroneType        = "drone.type"
    DroneBattery     = "drone.telemetry.battery"
    DroneAltitude    = "drone.telemetry.altitude"
    FleetMembership  = "fleet.membership"
)
```

Predicates follow `domain.category.property` format. This is not optional - consistent predicate naming is how you make your graph queryable.

### 2. A Graphable Implementation

Transform your domain messages into graph entities:

```go
type Graphable interface {
    EntityID() string          // 6-part federated identifier
    Triples() []message.Triple // Facts about this entity
}
```

Example:

```go
type DroneTelemetry struct {
    DroneID  string  `json:"drone_id"`
    Battery  int     `json:"battery"`
    Altitude float64 `json:"altitude"`
    FleetID  string  `json:"fleet_id"`
}

func (d *DroneTelemetry) EntityID() string {
    return fmt.Sprintf("acme.ops.robotics.gcs.drone.%s", d.DroneID)
}

func (d *DroneTelemetry) Triples() []message.Triple {
    id := d.EntityID()
    return []message.Triple{
        {Subject: id, Predicate: DroneType, Object: "cargo"},
        {Subject: id, Predicate: DroneBattery, Object: d.Battery},
        {Subject: id, Predicate: DroneAltitude, Object: d.Altitude},
        // This creates a relationship (edge) to another entity:
        {Subject: id, Predicate: FleetMembership, Object: d.FleetID},
    }
}
```

## What SemStreams Provides

### Entity Storage

Entities stored in NATS KV with version tracking:

```json
{
  "id": "acme.ops.robotics.gcs.drone.001",
  "triples": [
    {"predicate": "drone.type", "object": "cargo"},
    {"predicate": "drone.telemetry.battery", "object": 78}
  ],
  "version": 5
}
```

### Seven Indexes

| Index | Question It Answers |
|-------|---------------------|
| PREDICATE_INDEX | "All entities with this property" |
| INCOMING_INDEX | "Who references this entity?" |
| OUTGOING_INDEX | "What does this entity reference?" |
| ALIAS_INDEX | "Resolve friendly name to entity ID" |
| SPATIAL_INDEX | "Entities near this location" |
| TEMPORAL_INDEX | "Entities in this time range" |
| EMBEDDING_INDEX | "Semantically similar entities" |

### Automatic Community Detection

Entities that reference each other cluster into communities. You don't define communities - they emerge from relationships in your data.

### Progressive Enhancement (Tiers)

| Tier | Capabilities | Requirements |
|------|--------------|--------------|
| 0 | Rules engine, explicit relationships | NATS only |
| 1 | + BM25 search, statistical communities | + Search index |
| 2 | + Neural embeddings, LLM summaries | + Embedding service, LLM |

Start with Tier 0. Add capabilities as needed.

## Entity ID Format

Use 6-part hierarchical identifiers:

```text
org.platform.domain.system.type.instance
 │      │       │      │     │      │
 │      │       │      │     │      └─ Instance (001, rescue, sensor-a)
 │      │       │      │     └─ Entity type (drone, fleet, sensor)
 │      │       │      └─ Source system (gcs, hq, factory)
 │      │       └─ Data domain (robotics, ops, iot)
 │      └─ Platform (rescue, warehouse, production)
 └─ Organization (acme, globex)
```

Benefits:

- Federation across organizations
- Queryable by prefix (`acme.ops.*`)
- Self-documenting entity provenance

## Relationships Create Edges

When a triple's Object is another entity ID, it creates a traversable edge:

```go
// This creates an edge in the graph:
{Subject: droneID, Predicate: "fleet.membership", Object: "acme.ops.fleet.rescue"}

// This is just a property (no edge):
{Subject: droneID, Predicate: "drone.name", Object: "Alpha-1"}
```

Edges are what community detection uses to cluster related entities.

## When to Use SemStreams

**Good fit:**

- Real-time entity tracking with relationships
- Need to discover connected entities automatically
- Want community-based grouping without manual rules
- Progressive enhancement from basic to AI-powered

**Not designed for:**

- Pure time-series metrics (use InfluxDB, Prometheus)
- Full-text search only (use Elasticsearch)
- Batch ETL pipelines (use Airflow + dbt)
- Simple key-value lookups (use Redis)

## Next Steps

- [Architecture](02-architecture.md) - How the system works
- [Graphable Interface](03-graphable-interface.md) - Implementation details
- [Vocabulary](04-vocabulary.md) - Designing your predicates
- [First Processor](05-first-processor.md) - Step-by-step tutorial
- [Tiers](06-tiers.md) - Progressive enhancement options
