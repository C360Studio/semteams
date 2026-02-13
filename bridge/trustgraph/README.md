# TrustGraph Bridge

Bidirectional integration between SemStreams and TrustGraph for knowledge sharing between operational and document-extracted knowledge graphs.

## Overview

TrustGraph is a document processing pipeline that extracts knowledge graphs from unstructured documents using LLMs. This bridge enables:

- **Import**: Bring document-extracted entities into SemStreams for operational use
- **Export**: Push operational data to TrustGraph for inclusion in GraphRAG queries
- **Query**: Enable agents to query TrustGraph's knowledge via the GraphRAG tool

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        TrustGraph                               │
│  ┌────────────┐   ┌────────────┐   ┌────────────┐              │
│  │  Pulsar    │   │   Graph    │   │  Vector    │              │
│  │  Topics    │   │    DB      │   │    DB      │              │
│  └─────▲──────┘   └─────▲──────┘   └─────▲──────┘              │
│        │                │                │                      │
│  ┌─────┴────────────────┴────────────────┴─────┐               │
│  │              REST API Gateway                │               │
│  │  /triples-query  /knowledge  /graph-rag     │               │
│  └─────────────────────▲────────────────────────┘               │
└────────────────────────┼────────────────────────────────────────┘
                         │
    ┌────────────────────┼────────────────────┐
    │                    │    SemStreams      │
    │  ┌─────────────────┼─────────────────┐  │
    │  │      trustgraph bridge            │  │
    │  │                 │                 │  │
    │  │ ┌───────┐  ┌────┴────┐  ┌──────┐ │  │
    │  │ │ input │  │  query  │  │output│ │  │
    │  │ └───┬───┘  └────┬────┘  └──┬───┘ │  │
    │  │     │           │          │     │  │
    │  └─────┼───────────┼──────────┼─────┘  │
    │        │           │          │        │
    │        ▼           ▼          ▲        │
    │   entity.>    tool.result  entity.>    │
    │     (NATS)      (NATS)      (NATS)     │
    └────────────────────────────────────────┘
```

## Components

### trustgraph-input

Polls TrustGraph's triples-query API and emits SemStreams entities.

```json
{
  "trustgraph-input": {
    "type": "input",
    "name": "trustgraph-input",
    "enabled": true,
    "config": {
      "endpoint": "http://trustgraph:8088",
      "poll_interval": "60s",
      "source": "trustgraph",
      "limit": 1000,
      "vocab": {
        "uri_mappings": {
          "trustgraph.ai": {
            "org": "intel",
            "platform": "trustgraph",
            "domain": "knowledge",
            "system": "entity",
            "type": "concept"
          }
        }
      },
      "ports": {
        "outputs": [
          {"name": "entity", "type": "nats", "subject": "entity.trustgraph.>"}
        ]
      }
    }
  }
}
```

### trustgraph-output

Exports SemStreams entities to TrustGraph knowledge cores.

```json
{
  "trustgraph-output": {
    "type": "output",
    "name": "trustgraph-output",
    "enabled": true,
    "config": {
      "endpoint": "http://trustgraph:8088",
      "kg_core_id": "operational-data",
      "user": "semstreams",
      "collection": "sensors",
      "batch_size": 100,
      "flush_interval": "5s",
      "exclude_sources": ["trustgraph"],
      "vocab": {
        "org_mappings": {
          "acme": "http://acme.org/"
        }
      },
      "ports": {
        "inputs": [
          {"name": "entity", "type": "nats", "subject": "entity.>"}
        ]
      }
    }
  }
}
```

### trustgraph-query (Agentic Tool)

Enables agents to query TrustGraph's GraphRAG API. Auto-registered when imported:

```go
import _ "github.com/c360studio/semstreams/bridge/trustgraph/query"
```

Environment configuration:
- `TRUSTGRAPH_ENDPOINT`: API URL (default: `http://localhost:8088`)
- `TRUSTGRAPH_API_KEY`: Optional API key
- `TRUSTGRAPH_FLOW_ID`: GraphRAG flow (default: `graph-rag`)
- `TRUSTGRAPH_TIMEOUT`: Query timeout (default: `120s`)

## Vocabulary Translation

SemStreams uses 6-part dotted entity IDs while TrustGraph uses RDF URIs.

### Entity IDs

```
acme.ops.robotics.gcs.drone.001  ↔  http://acme.org/ops/robotics/gcs/drone/001
```

Configure org mappings:

```json
{
  "vocab": {
    "org_mappings": {
      "acme": "http://acme.org/",
      "c360": "http://semstreams.io/c360/"
    },
    "uri_mappings": {
      "trustgraph.ai": {
        "org": "intel",
        "platform": "trustgraph"
      }
    }
  }
}
```

### Predicates

Well-known predicates map to standard RDF URIs:

| SemStreams Predicate | RDF URI |
|---------------------|---------|
| `rdfs.label` | `http://www.w3.org/2000/01/rdf-schema#label` |
| `rdf.type` | `http://www.w3.org/1999/02/22-rdf-syntax-ns#type` |
| `sensor.measurement.value` | `http://www.w3.org/ns/sosa/hasSimpleResult` |

Custom predicates use structural fallback:
```
custom.domain.property → http://{base}/predicate/custom/domain/property
```

## Loop Prevention

When both input and output components are deployed:

1. **Input** sets `Source="trustgraph"` on all imported entities
2. **Output** excludes entities matching `exclude_sources`

This prevents infinite loops where imported entities get re-exported.

## Deployment Patterns

### Import Only

Bring TrustGraph knowledge into SemStreams:

```
TrustGraph → trustgraph-input → graph-ingest → ENTITY_STATES
```

Use case: Enrich operational context with document-extracted knowledge.

### Export Only

Push operational data to TrustGraph:

```
sensors → graph-ingest → ENTITY_STATES → trustgraph-output → TrustGraph
```

Use case: Include real-time sensor data in GraphRAG queries.

### Bidirectional

Full knowledge sharing with loop prevention:

```
TrustGraph ←→ trustgraph-input/output ←→ SemStreams
```

Use case: Unified knowledge graph across operational and document sources.

### Query Only

Enable agents to query TrustGraph without data movement:

```
agentic-tools → trustgraph-query → TrustGraph GraphRAG API
```

Use case: Agent needs document context without importing full knowledge graph.

## Subpackages

| Package | Purpose |
|---------|---------|
| `client/` | HTTP client for TrustGraph REST APIs |
| `input/` | Polling component that imports triples as entities |
| `output/` | Export component that sends entities as triples |
| `query/` | Agentic tool for GraphRAG queries |
| `vocab/` | Vocabulary translation between entity IDs and RDF URIs |

## Testing

Integration tests require a running TrustGraph instance:

```bash
# Unit tests (no external dependencies)
go test ./bridge/trustgraph/...

# Integration tests (requires TrustGraph)
go test -tags=integration ./bridge/trustgraph/...

# E2E tests use mock TrustGraph server
task e2e:agentic
```

## References

- [TrustGraph Documentation](https://trustgraph.ai/docs/)
- [TrustGraph Integration Guide](../../docs/integration/trustgraph-integration.md)
- [Vocabulary Guide](../../docs/basics/04-vocabulary.md)
