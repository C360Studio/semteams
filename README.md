# SemStreams

A stream processor that builds semantic knowledge graphs from event data, with automatic community detection and progressive AI enhancement.

## Overview

SemStreams transforms event streams into a living knowledge graph stored in NATS KV. You define a vocabulary of predicates, implement a simple interface, and the system maintains entities, relationships, indexes, and communities automatically.

```
Events → Graphable Interface → Knowledge Graph → Queries
```

**Key characteristics:**

- **Edge-first**: Deploy on a Raspberry Pi with just NATS, or scale to clusters
- **Offline-capable**: NATS JetStream provides local persistence and sync
- **Progressive**: Start with rules, add search, then embeddings and LLM as needed
- **Domain-driven**: No mandatory AI dependencies—you enable what you need

## Quick Start

```bash
# Build
task build

# Run with a flow configuration
./bin/semstreams --config configs/semantic-flow.json
```

## The Graphable Interface

Your domain types implement `Graphable` to become graph entities:

```go
type Graphable interface {
    EntityID() string          // 6-part federated identifier
    Triples() []message.Triple // Facts about this entity
}
```

Example:

```go
func (d *DroneTelemetry) EntityID() string {
    return fmt.Sprintf("acme.ops.robotics.gcs.drone.%s", d.DroneID)
}

func (d *DroneTelemetry) Triples() []message.Triple {
    id := d.EntityID()
    return []message.Triple{
        {Subject: id, Predicate: "drone.telemetry.battery", Object: d.Battery},
        {Subject: id, Predicate: "fleet.membership.current", Object: d.FleetID},
    }
}
```

## Entity IDs

Use 6-part hierarchical identifiers:

```
org.platform.domain.system.type.instance
```

Example: `acme.ops.robotics.gcs.drone.001`

## Predicates

Predicates follow `domain.category.property` format:

```
sensor.measurement.celsius
geo.location.zone
fleet.membership.current
```

Dotted notation enables NATS wildcard queries (`sensor.measurement.*`).

## Progressive Enhancement (Tiers)

| Tier | Capabilities | Requirements |
|------|--------------|--------------|
| Structural | Rules engine, explicit relationships, structural indexing | NATS only |
| Statistical | + BM25 search, statistical communities | + Search index |
| Semantic | + Neural embeddings, LLM summaries | + Embedding service, LLM |

Start with Structural. Add capabilities as resources allow.

## Architecture

Components connect via NATS subjects in flow-based configurations:

```
Input → Processor → Storage → Graph → Gateway
  │         │          │        │        │
 UDP    iot_sensor  ObjectStore KV+   GraphQL
 File   document    (raw docs)  Indexes  MCP
```

| Component Type | Examples | Role |
|----------------|----------|------|
| Input | UDP, WebSocket, File | Ingest external data |
| Processor | Graph, JSONMap, Rule | Transform and enrich |
| Output | File, HTTPPost, WebSocket | Export data |
| Storage | ObjectStore | Persist to NATS JetStream |
| Gateway | HTTP, GraphQL, MCP | Expose query APIs |

## State: NATS KV Buckets

All state lives in NATS JetStream KV buckets:

**Core buckets:**

| Bucket | Contents |
|--------|----------|
| `ENTITY_STATES` | Entity records with triples and version |
| `PREDICATE_INDEX` | Predicate → entity IDs |
| `INCOMING_INDEX` | Entity ID → referencing entities |
| `OUTGOING_INDEX` | Entity ID → referenced entities |
| `ALIAS_INDEX` | Alias → entity ID |
| `SPATIAL_INDEX` | Geohash → entity IDs |
| `TEMPORAL_INDEX` | Time bucket → entity IDs |

**Optional buckets:**

| Bucket | Contents | Feature |
|--------|----------|---------|
| `STRUCTURAL_INDEX` | K-core levels and pivot distances | Structural indexing |
| `EMBEDDING_INDEX` | Entity ID → embedding vector | Semantic search |
| `COMMUNITY_INDEX` | Community records with summaries | Community detection |

## Documentation

| Folder | Purpose |
|--------|---------|
| [docs/basics/](docs/basics/) | Getting started, core interfaces |
| [docs/concepts/](docs/concepts/) | Background knowledge, algorithms |
| [docs/advanced/](docs/advanced/) | Clustering, LLM, performance tuning |
| [docs/operations/](docs/operations/) | Local monitoring, deployment |
| [docs/contributing/](docs/contributing/) | Development, testing, CI |

## Requirements

- Go 1.25+
- NATS Server with JetStream enabled
- (Optional) Embedding service for Statistical/Semantic tiers
- (Optional) LLM service for Semantic tier

## License

See [LICENSE](LICENSE) for details.
