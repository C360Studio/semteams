# TrustGraph Integration

This document describes how SemStreams integrates with TrustGraph for bidirectional
knowledge sharing between operational and document-extracted knowledge graphs.

## Overview

TrustGraph is a document processing pipeline that extracts knowledge graphs from
unstructured documents using LLMs. It ingests PDFs, web pages, and other documents,
extracts entities and relationships, and makes them queryable via GraphRAG.

SemStreams implements three integration points:

```
                         TrustGraph
    ┌─────────────────────────────────────────────────────────┐
    │                                                         │
    │   ┌─────────────┐    ┌─────────────┐    ┌───────────┐  │
    │   │  Triples    │    │  Knowledge  │    │  GraphRAG │  │
    │   │  Query API  │    │     API     │    │    API    │  │
    │   └──────▲──────┘    └──────▲──────┘    └─────▲─────┘  │
    │          │                  │                  │        │
    └──────────┼──────────────────┼──────────────────┼────────┘
               │                  │                  │
    ┌──────────┼──────────────────┼──────────────────┼────────┐
    │          │                  │                  │        │
    │   ┌──────┴──────┐    ┌──────┴──────┐    ┌─────┴─────┐  │
    │   │ trustgraph  │    │ trustgraph  │    │trustgraph │  │
    │   │   -input    │    │   -output   │    │  -query   │  │
    │   └──────┬──────┘    └──────▲──────┘    └─────┬─────┘  │
    │          │                  │                  │        │
    │          ▼                  │                  ▼        │
    │      entity.>           entity.>         tool.result   │
    │       (NATS)             (NATS)            (NATS)      │
    │                                                         │
    │                    SemStreams                           │
    └─────────────────────────────────────────────────────────┘
```

## Core Concepts

### Document vs Operational Knowledge

TrustGraph and SemStreams serve complementary purposes:

| Aspect | TrustGraph | SemStreams |
|--------|------------|------------|
| Source | Documents (PDFs, web pages) | Events (sensors, telemetry) |
| Update rate | Batch (document ingestion) | Real-time (streaming) |
| Query pattern | Natural language (GraphRAG) | Structured (GraphQL, MCP) |
| Primary use | Research, analysis | Operations, monitoring |

The bridge enables:
- **Operational context enrichment**: Sensor readings annotated with document knowledge
- **Document query enhancement**: GraphRAG answers include real-time operational data
- **Unified agent access**: Agents query both knowledge sources seamlessly

### Vocabulary Translation

SemStreams uses hierarchical 6-part entity IDs:
```
org.platform.domain.system.type.instance
acme.ops.robotics.gcs.drone.001
```

TrustGraph uses RDF URIs:
```
http://trustgraph.ai/e/threat-report-001
http://acme.org/ops/robotics/gcs/drone/001
```

The vocab package handles bidirectional translation with configurable mappings.

### Loop Prevention

When both input and output are deployed, the bridge prevents infinite loops:

1. **Input component** marks all imported entities with `Source="trustgraph"`
2. **Output component** filters out entities where `Source` matches `exclude_sources`

This ensures document-extracted knowledge flows into SemStreams without being
re-exported back to TrustGraph.

## Components

### trustgraph-input

Polls TrustGraph's triples-query API and converts RDF triples to SemStreams entities.

**Features:**
- Configurable polling interval
- Subject/predicate filtering
- URI to entity ID translation
- Deduplication via change detection
- Source tagging for loop prevention

**Data flow:**
```
TrustGraph triples → vocab translation → entity messages → NATS
```

### trustgraph-output

Subscribes to entity changes and exports them as RDF triples to TrustGraph knowledge cores.

**Features:**
- Entity prefix filtering
- Source-based exclusion (loop prevention)
- Batching for efficient API calls
- Entity ID to URI translation
- Configurable flush interval

**Data flow:**
```
NATS entities → filter → vocab translation → batch → TrustGraph knowledge API
```

### trustgraph-query (Agentic Tool)

Enables agents to query TrustGraph's GraphRAG API using natural language.

**Tool definition:**
```
Name: trustgraph_query
Parameters:
  - query (required): Natural language question
  - collection (optional): Target collection
```

**Example agent interaction:**
```
Agent: "What are the maintenance procedures for pump model X?"
→ trustgraph_query tool invoked
→ GraphRAG API returns document-extracted answer
→ Agent incorporates answer in response
```

## Configuration

### Import Configuration

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
      "collections": ["intelligence", "operations"],
      "subject_filter": "http://trustgraph.ai/",
      "vocab": {
        "uri_mappings": {
          "trustgraph.ai": {
            "org": "intel",
            "platform": "trustgraph",
            "domain": "knowledge",
            "system": "entity",
            "type": "concept"
          }
        },
        "default_org": "external"
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

### Export Configuration

```json
{
  "trustgraph-output": {
    "type": "output",
    "name": "trustgraph-output",
    "enabled": true,
    "config": {
      "endpoint": "http://trustgraph:8088",
      "kg_core_id": "semstreams-operational",
      "user": "semstreams",
      "collection": "sensors",
      "batch_size": 100,
      "flush_interval": "5s",
      "entity_prefixes": ["acme.ops."],
      "exclude_sources": ["trustgraph"],
      "vocab": {
        "org_mappings": {
          "acme": "http://acme.org/",
          "c360": "http://semstreams.io/c360/"
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

### Query Tool Configuration

Environment variables:
```bash
TRUSTGRAPH_ENDPOINT=http://trustgraph:8088
TRUSTGRAPH_API_KEY=optional-api-key
TRUSTGRAPH_FLOW_ID=graph-rag
TRUSTGRAPH_TIMEOUT=120s
```

## Deployment Patterns

### Pattern 1: Import Only

Enrich SemStreams with document knowledge without exporting operational data.

```
┌─────────────┐      ┌─────────────────┐      ┌─────────────┐
│ TrustGraph  │─────►│ trustgraph-input│─────►│ graph-ingest│
│  (docs)     │      │                 │      │             │
└─────────────┘      └─────────────────┘      └─────────────┘
                                                     │
                                                     ▼
                                              ENTITY_STATES
```

**Use cases:**
- Agents need document context for decision making
- Operational dashboards show related documentation
- Alerts include relevant procedure references

### Pattern 2: Export Only

Share operational data with TrustGraph for GraphRAG queries.

```
┌─────────────┐      ┌─────────────────┐      ┌─────────────┐
│   Sensors   │─────►│   graph-ingest  │─────►│ trustgraph  │
│             │      │                 │      │   -output   │
└─────────────┘      └─────────────────┘      └──────┬──────┘
                                                     │
                                                     ▼
                                              ┌─────────────┐
                                              │ TrustGraph  │
                                              │  (unified)  │
                                              └─────────────┘
```

**Use cases:**
- GraphRAG queries can reference current sensor readings
- Document analysis includes operational context
- Unified knowledge graph for comprehensive queries

### Pattern 3: Bidirectional with Loop Prevention

Full knowledge sharing in both directions.

```
┌─────────────────────────────────────────────────────────────┐
│                        TrustGraph                           │
└───────────────────────────▲─────────────────────────────────┘
                            │
              ┌─────────────┼─────────────┐
              │             │             │
              ▼             │             │
    ┌─────────────────┐     │     ┌───────┴───────┐
    │ trustgraph-input│     │     │trustgraph     │
    │ (source=tg)     │     │     │  -output      │
    └────────┬────────┘     │     │(exclude tg)   │
             │              │     └───────▲───────┘
             │              │             │
             ▼              │             │
    ┌─────────────────────────────────────────────┐
    │              ENTITY_STATES                  │
    │  (tg entities + operational entities)       │
    └─────────────────────────────────────────────┘
```

**Key configuration for loop prevention:**
- Input: `"source": "trustgraph"`
- Output: `"exclude_sources": ["trustgraph"]`

### Pattern 4: Query Only (Lightweight)

Agents query TrustGraph without any data synchronization.

```
┌─────────────────┐      ┌─────────────────┐      ┌─────────────┐
│  agentic-tools  │─────►│ trustgraph-query│─────►│ TrustGraph  │
│                 │      │     (tool)      │      │  GraphRAG   │
└─────────────────┘      └─────────────────┘      └─────────────┘
```

**Use cases:**
- Occasional document queries without data movement
- Lightweight integration for testing
- Agents that need specific document answers

## Vocabulary Mapping

### Standard Predicates

| SemStreams | RDF URI |
|------------|---------|
| `rdfs.label` | `http://www.w3.org/2000/01/rdf-schema#label` |
| `rdf.type` | `http://www.w3.org/1999/02/22-rdf-syntax-ns#type` |
| `sensor.measurement.value` | `http://www.w3.org/ns/sosa/hasSimpleResult` |
| `sensor.location` | `http://www.w3.org/ns/sosa/hasFeatureOfInterest` |

### Org Mappings

Map SemStreams org segment to RDF base URI:

```json
{
  "org_mappings": {
    "acme": "http://data.acme-corp.com/",
    "c360": "http://semstreams.io/c360/",
    "intel": "http://trustgraph.ai/"
  }
}
```

### URI Mappings

Map TrustGraph URI domains to SemStreams entity ID components:

```json
{
  "uri_mappings": {
    "trustgraph.ai": {
      "org": "intel",
      "platform": "trustgraph",
      "domain": "knowledge",
      "system": "entity",
      "type": "concept"
    },
    "example.org": {
      "org": "external",
      "platform": "default"
    }
  }
}
```

## Testing

### E2E Testing with Mock Server

The agentic E2E tier includes TrustGraph validation using a mock server:

```bash
task e2e:agentic
```

The mock server:
- Seeds test triples (threat reports, sensor metadata)
- Accepts triple exports
- Returns configured GraphRAG responses
- Tracks stats for validation

### Integration Testing

For testing against a real TrustGraph instance:

```bash
# Set TrustGraph endpoint
export TRUSTGRAPH_ENDPOINT=http://localhost:8088

# Run integration tests
go test -tags=integration ./bridge/trustgraph/...
```

## Troubleshooting

### No Entities Imported

1. Check TrustGraph connectivity: `curl http://trustgraph:8088/health`
2. Verify triples exist: Query TrustGraph directly
3. Check poll_interval: Default is 60s, reduce for testing
4. Review logs for vocabulary translation errors

### Entities Not Exported

1. Verify entity source: Check `Source` field in ENTITY_STATES
2. Check exclude_sources: Imported entities have `Source="trustgraph"`
3. Verify entity_prefixes: Ensure entities match configured prefixes
4. Check batch timing: Wait for flush_interval

### GraphRAG Tool Not Working

1. Verify tool registration: Check agentic-tools allowed_tools
2. Test API directly: `curl -X POST http://trustgraph:8088/api/v1/graph-rag`
3. Check timeout: Default 120s may be insufficient for complex queries

## References

- [TrustGraph Bridge Package](../../bridge/trustgraph/README.md)
- [Vocabulary Guide](../basics/04-vocabulary.md)
- [Agentic Components](../advanced/08-agentic-components.md)
- [TrustGraph Documentation](https://trustgraph.ai/docs/)
