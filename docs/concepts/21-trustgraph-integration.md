# TrustGraph Integration

This document describes how SemStreams integrates with TrustGraph to combine
document-extracted knowledge with real-time operational data.

## Overview

TrustGraph is an open-source platform that uses LLMs to extract entities and
relationships from documents, building knowledge graphs that power GraphRAG
retrieval. SemStreams processes real-time operational data at the edge. The
integration bridges these two systems, enabling:

- Document knowledge to inform operational reasoning
- Operational data to enhance document-based queries
- Unified knowledge graphs spanning both domains

```
                    TrustGraph Ecosystem
    ┌───────────────────────────────────────────────────────────┐
    │                                                           │
    │   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │
    │   │  Documents  │───▶│    LLM      │───▶│  Knowledge  │  │
    │   │  (PDFs)     │    │  Extraction │    │    Graph    │  │
    │   └─────────────┘    └─────────────┘    └──────┬──────┘  │
    │                                                │         │
    │                           ┌────────────────────┤         │
    │                           │                    │         │
    │                      REST API              GraphRAG      │
    │                           │                    │         │
    └───────────────────────────┼────────────────────┼─────────┘
                                │                    │
    ┌───────────────────────────┼────────────────────┼─────────┐
    │                           │                    │         │
    │   ┌───────────────────────▼────────────────────▼───────┐ │
    │   │              Vocabulary Translation                │ │
    │   │           (RDF URIs ↔ Entity IDs)                  │ │
    │   └───────────────────────┬────────────────────┬───────┘ │
    │                           │                    │         │
    │   ┌───────────────────────▼───┐    ┌──────────▼───────┐ │
    │   │    trustgraph-input       │    │ trustgraph-output│ │
    │   │    (poll and import)      │    │ (watch and export│ │
    │   └───────────────────────┬───┘    └──────────▲───────┘ │
    │                           │                    │         │
    │   ┌───────────────────────▼────────────────────┴───────┐ │
    │   │                  NATS / ENTITY_STATES              │ │
    │   └────────────────────────────────────────────────────┘ │
    │                                                          │
    │                        SemStreams                        │
    └──────────────────────────────────────────────────────────┘
```

## Core Concepts

### Knowledge Graph Bridging

TrustGraph and SemStreams both maintain knowledge graphs, but serve different
purposes and use different representations.

| Aspect | TrustGraph | SemStreams |
|--------|------------|------------|
| Data source | Documents, reports, PDFs | Sensors, telemetry, events |
| Update frequency | Batch (document ingestion) | Real-time (streaming) |
| Triple format | RDF URIs | 6-part dotted entity IDs |
| Query style | GraphRAG (LLM-powered) | Rules, workflows, path queries |
| Deployment | Cloud/datacenter | Edge to cloud |

The bridge translates between these representations, enabling knowledge to flow
in both directions while each system maintains its efficient internal format.

### Vocabulary Translation

The core challenge is mapping between two different identifier schemes:

TrustGraph uses RDF URIs following W3C standards:
```
http://trustgraph.ai/e/supply-chain-risk
http://www.w3.org/1999/02/22-rdf-syntax-ns#type
http://www.w3.org/2000/01/rdf-schema#label
```

SemStreams uses hierarchical dotted identifiers:
```
acme.intel.knowledge.trustgraph.concept.supply-chain-risk
entity.classification.type
entity.metadata.label
```

Translation happens at system boundaries via configurable mappings:

| Direction | Example |
|-----------|---------|
| URI → Entity ID | `http://trustgraph.ai/e/threat-actor` → `acme.intel.threats.trustgraph.actor.threat-actor` |
| Entity ID → URI | `acme.ops.sensor.temp.zone-7` → `https://data.acme.com/ops/sensor/temp/zone-7` |
| Predicate → URI | `sensor.measurement.celsius` → `http://www.w3.org/ns/sosa/hasSimpleResult` |
| URI → Predicate | `http://www.w3.org/1999/02/22-rdf-syntax-ns#type` → `entity.classification.type` |

See: `vocabulary/trustgraph/`

### Import Flow

The input component polls TrustGraph's triples-query API and imports entities
into SemStreams.

Process:
1. Query TrustGraph for triples matching configured filters
2. Group triples by subject URI (each subject becomes one entity)
3. Translate URIs to entity IDs and predicates
4. Compute hash of triple set for change detection
5. Skip unchanged entities (hash matches previous import)
6. Publish changed entities to NATS for graph-ingest

Change detection prevents redundant processing when TrustGraph data hasn't
changed. The sync state is stored in a NATS KV bucket.

See: `input/trustgraph/README.md`

### Export Flow

The output component watches SemStreams entity changes and exports them to
TrustGraph knowledge cores.

Process:
1. Subscribe to NATS entity subjects
2. Filter by entity prefix (only export operational data)
3. Filter by source (exclude entities imported from TrustGraph)
4. Translate entity IDs to URIs and predicates to RDF
5. Batch triples for efficient API calls
6. Flush batch to TrustGraph on size threshold or interval

Batching reduces API overhead when many entities change rapidly. The batch
buffer is bounded to prevent memory growth during TrustGraph outages.

See: `output/trustgraph/README.md`

### Loop Prevention

When both import and export are deployed, circular data flow must be prevented.

The mechanism uses the source field on triples:

```
    TrustGraph ──import──► SemStreams ──export──► TrustGraph
                 │                        │
                 │ source="trustgraph"    │ exclude_sources=["trustgraph"]
                 │                        │
                 └────── BLOCKED ─────────┘
```

Import stamps all triples with `source: "trustgraph"`. Export filters out
entities where all triples have an excluded source. This prevents re-exporting
imported data while still allowing mixed entities (some imported, some local)
to flow back with their local additions.

## Integration Patterns

### Import Only (Most Common)

Use TrustGraph for document intelligence, SemStreams for operational reasoning.

```
    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │ TrustGraph  │────▶│ trustgraph  │────▶│  Rules /    │
    │ (documents) │     │   -input    │     │  Workflows  │
    └─────────────┘     └─────────────┘     └─────────────┘
```

Example: Cybersecurity threat intelligence extracted from reports informs
rules that correlate with real-time network sensor data.

### Export Only

Push operational data to TrustGraph for enhanced GraphRAG queries.

```
    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │  Sensors /  │────▶│ trustgraph  │────▶│ TrustGraph  │
    │  Telemetry  │     │   -output   │     │  (GraphRAG) │
    └─────────────┘     └─────────────┘     └─────────────┘
```

Example: Temperature sensor readings exported to TrustGraph enable agents
to answer "What is the current temperature in zone 7?" by combining live
data with maintenance procedure documents.

### Bidirectional Sync

Full knowledge sharing between systems with loop prevention.

```
    ┌─────────────────────────────────────────────────────────┐
    │                     SemStreams                          │
    │                                                         │
    │   ┌─────────────┐                   ┌─────────────┐    │
    │   │ trustgraph  │    NATS /         │ trustgraph  │    │
    │   │   -input    │◄── ENTITY_STATES ─►│   -output   │    │
    │   └──────▲──────┘                   └──────┬──────┘    │
    │          │                                  │           │
    └──────────┼──────────────────────────────────┼───────────┘
               │                                  │
               │  source filtering prevents loop  │
               │                                  │
    ┌──────────┼──────────────────────────────────┼───────────┐
    │          │                                  │           │
    │   ┌──────┴──────┐                   ┌──────▼──────┐    │
    │   │   Triples   │                   │  Knowledge  │    │
    │   │   Query     │                   │    Core     │    │
    │   └─────────────┘                   └─────────────┘    │
    │                                                         │
    │                     TrustGraph                          │
    └─────────────────────────────────────────────────────────┘
```

Requirements for bidirectional sync:
- Consistent vocabulary configuration in both components
- Source-based loop prevention enabled
- Separate knowledge cores for imported vs. exported data (recommended)

## Use Cases

### Threat Intelligence Correlation

Documents containing threat reports are ingested into TrustGraph. The LLM
extracts CVEs, threat actors, affected software, and attack patterns.
SemStreams imports these entities and rules correlate them with real-time
network monitoring data.

When a network anomaly matches a known threat pattern, the system can
immediately surface the relevant threat intelligence context.

### Maintenance Procedure Context

Maintenance procedures and equipment manuals are extracted into TrustGraph.
SemStreams exports sensor readings and equipment status. When an operator
asks "What maintenance is needed for pump-7?", GraphRAG can combine the
live equipment state with documented procedures.

### Unified Operational Dashboard

Export operational entity state to TrustGraph. Use GraphRAG to power natural
language queries across both document knowledge and live operations.
Operators ask questions in plain language and receive answers that synthesize
both historical documentation and current system state.

## Performance Considerations

### Import

- **Poll interval**: Balance freshness against API load (60s typical)
- **Limit per poll**: Larger batches reduce API calls but increase memory
- **Change detection**: Hash comparison prevents redundant processing
- **Subject filtering**: Reduce import scope to relevant data

### Export

- **Batch size**: Larger batches are more efficient but delay visibility
- **Flush interval**: Maximum latency before data reaches TrustGraph
- **Entity prefixes**: Filter to only export operational data
- **Buffer cap**: Prevents unbounded growth during outages

### Network

- **Timeout settings**: Accommodate network latency and TrustGraph load
- **Retry behavior**: Transient failures don't lose data
- **Connection reuse**: HTTP clients maintain persistent connections

## Configuration Summary

| Component | Purpose | Key Settings |
|-----------|---------|--------------|
| `trustgraph-input` | Import from TrustGraph | `endpoint`, `poll_interval`, `source` |
| `trustgraph-output` | Export to TrustGraph | `endpoint`, `kg_core_id`, `exclude_sources` |
| `vocab` (both) | Translation mappings | `uri_mappings`, `predicate_mappings` |

## References

- [TrustGraph Documentation](https://docs.trustgraph.ai)
- [TrustGraph GitHub](https://github.com/trustgraph-ai/trustgraph)
- [W3C RDF Concepts](https://www.w3.org/TR/rdf11-concepts/)
- [SOSA/SSN Ontology](https://www.w3.org/TR/vocab-ssn/)
