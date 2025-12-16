# Architecture

SemStreams processes event streams into a semantic knowledge graph stored in NATS KV buckets. This document explains the core components and data flow.

## System Overview

```text
Input → Domain Processor → Storage → Graph → Output/Gateway
  │           │               │        │           │
 UDP      iot_sensor     ObjectStore  KV+      File, HTTP,
 File     document       (raw docs)   Indexes  WebSocket,
                                               API Gateway
```

## Components

SemStreams uses a component-based architecture. Components are self-describing units that connect via NATS:

| Type | Examples | Role |
|------|----------|------|
| Input | UDP, WebSocket, File | Ingest data from external sources |
| Processor | Graph, JSONMap, Rule | Transform and enrich data |
| Output | File, HTTPPost, WebSocket | Export data to external systems |
| Storage | ObjectStore | Persist data to NATS JetStream |
| Gateway | HTTP, GraphQL, MCP | Expose APIs for queries and mutations |

### Flow-Based Design

Components connect through NATS subjects rather than direct calls:

- **Loose coupling**: Components don't know about each other—they publish/subscribe to subjects
- **Hook points**: Add components at any point by subscribing to existing subjects
- **Configuration-driven**: Flows are JSON configs declaring which components to use and how to connect them

The Graph processor is central to semantic processing, but it's one component among many. You can build flows with just protocol-layer components (UDP → JSONMap → File) or add semantic processing (UDP → Graph → GraphQL).

## Processing Flow

### 1. Message Arrival

Messages arrive via NATS JetStream. Each message contains a payload that your processor understands.

### 2. Transformation (Your Code)

Your processor implements the `Graphable` interface to transform incoming data:

```go
type Graphable interface {
    EntityID() string      // 6-part federated identifier
    Triples() []Triple     // Facts about this entity
}
```

This is where domain knowledge lives. Generic processors cannot make semantic decisions about your data.

### 3. Entity Storage

Entities are stored in `ENTITY_STATES` with version tracking:

```json
{
  "id": "acme.logistics.environmental.sensor.temperature.sensor-042",
  "triples": [
    {"predicate": "sensor.measurement.celsius", "object": 23.5},
    {"predicate": "geo.location.zone", "object": "acme.logistics.facility.zone.area.warehouse-7"}
  ],
  "version": 5
}
```

Updates use compare-and-swap with version numbers (optimistic concurrency).

### 4. Index Maintenance

Seven indexes are maintained automatically via KV watchers:

| Index | Question Answered |
|-------|-------------------|
| `PREDICATE_INDEX` | "All entities with this property" |
| `INCOMING_INDEX` | "Who references this entity?" |
| `OUTGOING_INDEX` | "What does this entity reference?" |
| `ALIAS_INDEX` | "Resolve friendly name to entity ID" |
| `SPATIAL_INDEX` | "Entities near this location" |
| `TEMPORAL_INDEX` | "Entities in this time range" |
| `EMBEDDING_INDEX` | "Semantically similar entities" |

Indexes update asynchronously after entity saves. There's a brief window where an entity exists but isn't fully indexed.

### 5. Rules Evaluation

Stateful rules evaluate conditions against entity state:

```json
{
  "id": "battery-low-alert",
  "expression": "drone.telemetry.battery < 20",
  "on_enter": [
    {"action": "add_triple", "predicate": "alert.status", "object": "battery_low"},
    {"action": "publish", "subject": "alerts.battery"}
  ],
  "on_exit": [
    {"action": "remove_triple", "predicate": "alert.status"}
  ]
}
```

Rules can add/remove triples and publish messages, creating derived facts dynamically.

### 6. Community Detection

Entities that reference each other cluster into communities. Detection runs:

- After a threshold of entity changes (e.g., 100)
- At configured intervals (e.g., 30s)
- Using Label Propagation Algorithm (LPA)

Communities enable GraphRAG-style queries at different granularity levels.

## State: NATS KV Buckets

All state lives in NATS JetStream KV buckets.

> **Note**: SemStreams connects to a single NATS server. NATS clustering is not in scope for MVP due to edge/offline-first deployment focus. See [Known Limitations](../reference/known-limitations.md).

| Bucket | Contents |
|--------|----------|
| `ENTITY_STATES` | Entity records with triples and version |
| `PREDICATE_INDEX` | Predicate → entity IDs |
| `INCOMING_INDEX` | Entity ID → referencing entities |
| `OUTGOING_INDEX` | Entity ID → referenced entities |
| `ALIAS_INDEX` | Alias → entity ID |
| `SPATIAL_INDEX` | Geohash → entity IDs |
| `TEMPORAL_INDEX` | Time bucket → entity IDs |
| `EMBEDDING_INDEX` | Entity ID → embedding vector |
| `COMMUNITY_INDEX` | Community records with members and summaries |
| `RULE_STATE` | Rule evaluation state per entity |

## Data Flow Example

A sensor reading arrives:

```text
1. NATS message: {"device_id": "sensor-042", "reading": 23.5, ...}
                              │
2. Processor transforms:      ▼
   EntityID: "acme.logistics.environmental.sensor.temperature.sensor-042"
   Triples: [sensor.measurement.celsius: 23.5, geo.location.zone: zone-id]
                              │
3. DataManager stores:        ▼
   ENTITY_STATES["acme.logistics..."] = {triples, version: 6}
                              │
4. IndexManager updates:      ▼
   PREDICATE_INDEX["sensor.measurement.celsius"] += entity_id
   INCOMING_INDEX[zone-id] += entity_id
   OUTGOING_INDEX[entity_id] += zone-id
                              │
5. RuleProcessor evaluates:   ▼
   No rules matched (or matched → add_triple/publish)
                              │
6. Entity change count: 99 → 100, threshold reached
                              │
7. ClusterManager runs:       ▼
   LPA groups entities → communities updated
   Statistical summaries generated
   LLM summaries queued (if Tier 2)
```

## Consistency Model

| Operation | Consistency |
|-----------|-------------|
| Entity by ID | Immediate |
| Index queries | Eventually consistent (milliseconds) |
| Community membership | Batch (seconds to minutes) |
| Community summaries | Async (depends on LLM) |

## What SemStreams Is Not

- **Not a database replacement**: No arbitrary SQL or ACID transactions—but dotted notation with NATS subject/KV wildcards provides SQL-like query basics (prefix matching, pattern queries)
- **Hybrid streaming/batch**: Entity updates flow continuously, but community detection and summarization run periodically (configurable intervals)
- **Not a time-series DB**: Use InfluxDB/Prometheus for metrics
- **Not full-text search**: Use Elasticsearch for document search

## Background Concepts

New to knowledge graphs or event-driven systems? See [Concepts](../concepts/) for background on:

- [Event-Driven Basics](../concepts/01-event-driven-basics.md) - Pub/sub, streams, NATS
- [Knowledge Graphs](../concepts/02-knowledge-graphs.md) - Triples, SPO model
- [Community Detection](../concepts/04-community-detection.md) - LPA algorithm details
- [GraphRAG Pattern](../concepts/05-graphrag-pattern.md) - Community-based RAG

## Next Steps

- [Graphable Interface](03-graphable-interface.md) - Implement entity transformation
- [Vocabulary](04-vocabulary.md) - Design your predicates
- [Configuration](06-configuration.md) - Choose your capability level
