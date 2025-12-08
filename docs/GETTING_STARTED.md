# Getting Started with SemStreams

SemStreams is a stream processor that builds a semantic knowledge graph from your domain data. It enables semantic search, automatic relationship discovery, and AI-ready queries without requiring you to define the schema upfront.

## The 30-Second Concept

```
Your Data → Entities with Relationships → Searchable Knowledge Graph
```

You write a processor that transforms your domain messages (telemetry, events, records) into entities with properties and relationships. SemStreams maintains a living graph, automatically detects communities of related entities, and provides progressive enhancement from simple queries to AI-powered insights.

## Choose Your Path

| I want to... | Start here |
|--------------|------------|
| Process domain data | [Creating a Processor](#creating-a-processor) |
| Understand communities | [What is a Community?](concepts/WHAT_IS_A_COMMUNITY.md) |
| Understand the architecture | [Dynamic Graph](concepts/DYNAMIC_GRAPH.md) |
| See how rules build graphs | [Rules and Graph](concepts/RULES_AND_GRAPH.md) |
| Design good triples | [Index Usage](concepts/INDEX_USAGE.md) |

## Quick Start (10 minutes)

### Prerequisites

- Go 1.21+
- Docker (for NATS)
- Task runner (`go install github.com/go-task/task/v3/cmd/task@latest`)

### 1. Run the Example

```bash
# Start NATS and run the basic semantic test
task e2e:semantic-basic
```

This starts a NATS server, loads test data, and runs community detection.

### 2. See What Happened

The test creates entities from sensor data, detects communities, and generates summaries. Check the output for:

- Entities created in `ENTITY_STATES` KV bucket
- Relationships indexed in `INCOMING_INDEX` and `OUTGOING_INDEX`
- Communities detected and stored in `COMMUNITY_INDEX`

### 3. Query the Graph

```bash
# Query entities by predicate
nats kv get PREDICATE_INDEX "sensor.measurement.type"

# Get a specific entity
nats kv get ENTITY_STATES "your-entity-id"

# See detected communities
nats kv get COMMUNITY_INDEX "graph.community.level0.*"
```

## Creating a Processor

A processor transforms your domain messages into graph entities. You implement the `Graphable` interface:

```go
type Graphable interface {
    EntityID() string           // Unique identifier
    Triples() []Triple          // Facts about this entity
    TextContent() string        // Text for embeddings (optional)
    Embedding() []float32       // Pre-computed embedding (optional)
}
```

### Example: Drone Telemetry

```go
type DroneTelemetry struct {
    DroneID   string  `json:"drone_id"`
    Battery   int     `json:"battery"`
    Altitude  float64 `json:"altitude"`
    MissionID string  `json:"mission_id"`
    Zone      string  `json:"zone"`
}

func (d *DroneTelemetry) EntityID() string {
    return fmt.Sprintf("acme.robotics.aerial.drone.%s", d.DroneID)
}

func (d *DroneTelemetry) Triples() []graph.Triple {
    return []graph.Triple{
        {Predicate: "entity.type", Object: "drone"},
        {Predicate: "drone.telemetry.battery", Object: fmt.Sprintf("%d", d.Battery)},
        {Predicate: "drone.telemetry.altitude", Object: fmt.Sprintf("%.1f", d.Altitude)},
        {Predicate: "drone.zone", Object: d.Zone},
        // This creates a RELATIONSHIP (edge in the graph):
        {Predicate: "mission.assignment.current", Object: fmt.Sprintf("acme.robotics.mission.%s", d.MissionID)},
    }
}
```

### Key Design Decisions

1. **Entity IDs**: Use hierarchical format for federation and querying
   - `org.platform.domain.system.type.instance`
   - Example: `acme.robotics.aerial.drone.drone-007`

2. **Triples create edges**: When the Object is another entity ID, it creates a traversable relationship that affects community detection

3. **Predicate naming**: Use consistent naming - `sensor.temp`, `temperature`, and `sensor.temperature` are three separate indexes

## The Three Tiers

SemStreams supports progressive enhancement:

| Tier | What You Get | Requirements |
|------|--------------|--------------|
| **Tier 0** | Rules engine, explicit relationships | Just NATS |
| **Tier 1** | + BM25 search, statistical communities | + Search index |
| **Tier 2** | + Neural embeddings, LLM summaries | + Embedding service, LLM |

Start with Tier 0 and add capabilities as needed. The system degrades gracefully - if LLM is unavailable, you still get statistical summaries.

## What Comes Out

### Entities

Stored in `ENTITY_STATES` with version tracking:

```json
{
  "id": "acme.robotics.aerial.drone.drone-007",
  "triples": [
    {"predicate": "entity.type", "object": "drone"},
    {"predicate": "drone.telemetry.battery", "object": "78"},
    {"predicate": "mission.assignment.current", "object": "acme.robotics.mission.delivery-42"}
  ],
  "version": 5
}
```

### Communities

Automatically detected groups of related entities:

```json
{
  "id": "community-level0-abc123",
  "level": 0,
  "members": ["drone-007", "drone-008", "sensor-alpha", "mission-delivery-42"],
  "statistical_summary": "Community of 4 entities. Keywords: drone, delivery, warehouse-7",
  "llm_summary": "A fleet of cargo drones operating delivery missions in warehouse 7...",
  "summary_status": "llm-enhanced"
}
```

### Indexes

Seven indexes for different query patterns:

| Index | Query Pattern |
|-------|---------------|
| `PREDICATE_INDEX` | "All entities with this property" |
| `INCOMING_INDEX` | "Who references this entity?" |
| `OUTGOING_INDEX` | "What does this entity reference?" |
| `ALIAS_INDEX` | "Resolve friendly name to entity ID" |
| `SPATIAL_INDEX` | "Entities near this location" |
| `TEMPORAL_INDEX` | "Entities in this time range" |
| `EMBEDDING_INDEX` | "Semantically similar entities" |

## Next Steps

- [What is a Community?](concepts/WHAT_IS_A_COMMUNITY.md) - Plain English explanation
- [Rules and Graph](concepts/RULES_AND_GRAPH.md) - How rules shape communities
- [Index Usage](concepts/INDEX_USAGE.md) - Design triples for the queries you need
- [Dynamic Graph](concepts/DYNAMIC_GRAPH.md) - How the graph stays current
- [Processor Design Philosophy](PROCESSOR-DESIGN-PHILOSOPHY.md) - Deep dive into design decisions
