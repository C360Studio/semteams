# KV Bucket Reference

SemStreams stores all graph state in NATS JetStream KV buckets. This document describes each bucket, its purpose, and data format.

## Overview

Buckets are created by their respective processors and are organized by tier:

- **Tier 0 (Structural)**: Core graph storage, available in all deployments
- **Tier 1+ (Statistical/Semantic)**: Advanced indexes requiring embeddings or community detection

## Tier 0 Buckets

These buckets are created by the core graph processors and available in all deployments.

### ENTITY_STATES

Primary entity storage. Each entity is stored as a complete JSON record.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-ingest |
| **Key format** | Entity ID (6-part dotted notation) |
| **Value** | JSON entity with triples, aliases, metadata |

**Example key**: `acme.ops.sensors.warehouse.temperature.001`

### OUTGOING_INDEX

Forward relationship lookup. Maps entity to its outgoing relationships.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index |
| **Key format** | Entity ID |
| **Value** | Array of `{predicate, to_entity_id}` |

**Use case**: "What entities does X connect to?"

### INCOMING_INDEX

Reverse relationship lookup. Maps entity to entities pointing at it.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index |
| **Key format** | Entity ID |
| **Value** | Array of `{predicate, from_entity_id}` |

**Use case**: "What entities point to X?"

### ALIAS_INDEX

Entity alias resolution. Maps alias values to entity IDs.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index |
| **Key format** | Alias value |
| **Value** | Entity ID |

**Use case**: Resolve "sensor-001" to full entity ID.

### PREDICATE_INDEX

Predicate-based entity lookup. Maps predicates to entities that have them.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index |
| **Key format** | Predicate (dotted notation) |
| **Value** | Array of entity IDs |

**Use case**: "Find all entities with `located_in` predicate."

### SPATIAL_INDEX

Geographic bounds lookup. Maps geohash prefixes to entities.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index-spatial |
| **Key format** | Geohash prefix |
| **Value** | Array of entity IDs with coordinates |

**Use case**: "Find entities within geographic bounds."

### TEMPORAL_INDEX

Time range lookup. Maps time buckets to entities.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index-temporal |
| **Key format** | Time bucket (e.g., `2024-01-15T10`) |
| **Value** | Array of entity IDs with timestamps |

**Use case**: "Find entities active in time range."

### COMPONENT_STATUS

Component lifecycle status. Tracks current processing stage of long-running components.

| Attribute | Value |
|-----------|-------|
| **Created by** | Any component implementing LifecycleReporter |
| **Key format** | Component name |
| **Value** | Status JSON (stage, cycle_id, timestamps) |

**Use case**: Operational monitoring, "What stage is graph-clustering in?"

### CONTEXT_INDEX

Provenance tracking. Maps context values to triples that have them.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-index |
| **Key format** | Context value (e.g., `inference.hierarchy`) |
| **Value** | Array of `{entity_id, predicate}` |

**Use case**: "Find all triples from hierarchy inference."

### STRUCTURAL_INDEX

K-core decomposition and pivot distance data for structural analysis.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-clustering |
| **Key format** | Entity ID |
| **Value** | K-core level, pivot distances |

**Use case**: Anomaly detection, structural importance ranking.

## Tier 1+ Buckets

These buckets require embeddings or community detection and are created by statistical/semantic tier processors.

### EMBEDDING_INDEX

Embedding vectors for similarity search.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-embedding |
| **Key format** | Entity ID |
| **Value** | Vector array + metadata (model, dimensions) |

**Use case**: Semantic similarity search, clustering virtual edges.

### EMBEDDING_DEDUP

Deduplication tracking to avoid re-embedding unchanged content.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-embedding |
| **Key format** | Content hash |
| **Value** | Entity ID + timestamp |

**Use case**: Skip embedding if content unchanged.

### COMMUNITY_INDEX

Community membership and metadata.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-clustering |
| **Key format** | Community ID |
| **Value** | Community JSON (members, level, summary) |

**Use case**: GraphRAG queries, community-based search.

### ANOMALY_INDEX

Detected anomalies awaiting review.

| Attribute | Value |
|-----------|-------|
| **Created by** | graph-clustering |
| **Key format** | Anomaly ID |
| **Value** | Anomaly JSON (type, entities, confidence, suggestion) |

**Use case**: Anomaly approval workflow, gap detection results.

## Bucket Lifecycle

Buckets are created on-demand when their processor starts. They persist across restarts via JetStream.

### Watching for Changes

All buckets support reactive watching:

```go
watcher, _ := kv.Watch("ENTITY_STATES.>")
for entry := range watcher.Updates() {
    // React to entity changes
}
```

### Retention

Bucket retention is configured per-processor. Default is to keep latest value only (KV semantics), but history can be enabled for audit trails.

## Related

- [Event-Driven Basics](../concepts/01-event-driven-basics.md) - How KV buckets fit into the architecture
- [ADR-001: Pragmatic Semantic Web](../architecture/adr-001-pragmatic-semantic-web.md) - Design decisions for bucket schema
