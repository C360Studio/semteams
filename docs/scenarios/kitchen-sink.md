# Semantic Kitchen Sink: The Complete Framework Story

## Overview

The Kitchen Sink scenario demonstrates the full SemStreams capability stack—a **Dynamic Knowledge Graph** with progressive enhancement for edge-to-cloud deployment.

**Key differentiator:** Unlike static knowledge graphs that require batch rebuilds, SemStreams maintains a **real-time, incrementally-updated** knowledge graph via KV watch patterns. Entities and indexes update as data arrives.

### Two Variants (Progressive Enhancement)

- **Core (Edge/Offline)**: Full entity storage, all 7 indexes, BM25 lexical search, statistical community summaries. **No external service dependencies**—runs on Raspberry Pi to data center.
- **ML (Cloud/Connected)**: Everything from Core, plus semembed (neural embeddings) and semsummarize (LLM summaries) for enhanced semantic capabilities.

## The Problem: Data Without Meaning

Traditional stream processing treats data as opaque bytes flowing through pipes. You can filter, transform, and route messages—but the system has no understanding of *what* the data represents or *how* entities relate to each other.

Consider a logistics operation with:

- **IoT sensors** reporting temperature, humidity, and pressure readings
- **Maintenance records** documenting equipment repairs
- **Safety observations** from inspectors
- **Operational documents** like manuals and procedures

In a traditional system, these are just separate data streams. When a cold storage temperature spikes, you might trigger an alert—but you can't automatically answer:

- *"What maintenance was recently done on this unit?"*
- *"Are there related safety observations?"*
- *"What's the trend across all sensors in this zone?"*

## The Solution: Semantic Streaming

SemStreams transforms raw data streams into a **dynamic knowledge graph** that understands entities, relationships, and meaning. Every piece of data becomes a node in a queryable graph with:

- **Federated Entity IDs**: `{org}.{platform}.{domain}.{type}.{instance}`
- **Semantic Triples**: Subject-Predicate-Object facts about each entity
- **Multiple Indexes**: Predicate, incoming, alias, spatial, temporal
- **Search**: BM25 lexical (core) or embedding-based semantic (ML)

## Kitchen Sink Architecture

The kitchen sink scenario demonstrates the complete SemStreams capability stack with **ContentStorable** architecture and **pub/sub topology** (not linear pipelines):

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DYNAMIC KNOWLEDGE GRAPH                              │
│                   (Real-time updates via KV watch)                          │
└─────────────────────────────────────────────────────────────────────────────┘

DATA SOURCES                    DOMAIN PROCESSORS                    STORAGE
─────────────                   ─────────────────                    ───────

documents.jsonl ──┐
maintenance.jsonl ├──► raw.document.corpus ──► document_processor ──┬──► ObjectStore
observations.jsonl│                               (ContentStorable)  │    (body content)
sensor_docs.jsonl ┘                                      │           │
                                                         └───────────┼──► events.graph.entity.document
                                                                     │    (metadata + StorageRef)
                                                                     │
sensors.jsonl ────► raw.sensor.file ──────► iot_sensor ──────────────┼──► events.graph.entity.sensor
                          │                 (Graphable)              │    (triples only)
                          │                                          │
                          └──► rule_processor ──► events.rule.triggered
                                                                     │
                                                                     ▼
                                                        ┌────────────────────────┐
                                                        │ events.graph.entity.*  │
                                                        │   (wildcard subscribe) │
                                                        └───────────┬────────────┘
                                                                    │
                                                                    ▼
                                                        ┌────────────────────────┐
                                                        │    GRAPH PROCESSOR     │
                                                        │  (single subscriber)   │
                                                        └───────────┬────────────┘
                                                                    │
                    ┌───────────────────────────────────────────────┼───────────────────────────────────┐
                    │                                               │                                   │
                    ▼                                               ▼                                   ▼
        ┌───────────────────┐                           ┌───────────────────┐               ┌───────────────────┐
        │   ENTITY_STATES   │                           │      INDEXES      │               │ EMBEDDING_INDEX   │
        │ (metadata+ref)    │                           │                   │               │                   │
        └───────────────────┘                           │ • PREDICATE       │               └─────────┬─────────┘
                                                        │ • INCOMING        │                         │
                                                        │ • OUTGOING        │                         ▼
                                                        │ • ALIAS           │               ┌───────────────────┐
                                                        │ • SPATIAL         │               │ EMBEDDING WORKER  │
                                                        │ • TEMPORAL        │               │                   │
                                                        └───────────────────┘               │  ◄──── ObjectStore│
                                                                                            │  (fetch body via  │
                                                                                            │   StorageRef)     │
                                                                                            └───────────────────┘

OUTPUTS (subscribe to events.graph.entity.>):
─────────────────────────────────────────────
• file output      → JSONL archive
• httppost         → Webhooks
• objectstore      → Message archive
• websocket        → Real-time streaming (from raw.udp.messages)
• api-gateway      → REST API queries
```

**Key architecture points:**

- **NOT linear pipelines** - pub/sub with wildcard subscription
- **ContentStorable types** (Document, Maintenance, Observation) store body to ObjectStore FIRST
- **Graph receives metadata only** - body replaced by StorageRef pointer
- **Embedding worker** fetches full content from ObjectStore when generating vectors
- **All indexes updated atomically** via KV watch pattern

## Data Flow Explained

### 1. Ingestion Layer

**Real-time Telemetry (UDP Path)**

```
UDP :14550 → raw.udp.messages → WebSocket (real-time streaming)
```
Live telemetry streams directly to WebSocket for real-time dashboard updates. The focus is on semantic document processing rather than generic message transformation.

**Document Corpus (File Inputs) - ContentStorable Pattern**
```
documents.jsonl    ┐                                    ┌→ ObjectStore (body content)
maintenance.jsonl  ├→ raw.document.corpus → Document   │
observations.jsonl │                        Processor ─┼→ events.graph.entity.document
sensor_docs.jsonl  ┘                                    │   (metadata triples + StorageRef)
                                                        └→ Graph Processor → KV + Indexes
```
Rich text content implements the **ContentStorable** interface:
1. **Process**: Document processor transforms JSON into semantic entities
2. **Store**: Body content stored in ObjectStore (separate from triples)
3. **Graph**: Metadata triples (Dublin Core) + StorageRef published to graph

This separation keeps entity state lean while enabling semantic search over full content.

**Sensor Readings (File Input)**
```
sensors.jsonl → raw.sensor.file → iot_sensor → events.graph.entity.sensor
                      ↓
                rule_processor → events.rule.triggered (if thresholds exceeded)
```
Time-series sensor data becomes queryable entities. The rule processor monitors sensor streams for threshold violations.

### 2. Semantic Processing Layer

**Document Processor** transforms incoming JSON into federated entities using **Dublin Core** metadata:
```json
// Input
{"id": "doc-001", "title": "Safety Manual", "category": "safety", "body": "Full document text..."}

// Output Entity ID
"c360.logistics.content.document.safety.doc-001"

// Generated Triples (metadata only - NO body)
[
  {"subject": "c360.logistics...", "predicate": "dc.title", "object": "Safety Manual"},
  {"subject": "c360.logistics...", "predicate": "dc.type", "object": "document"},
  {"subject": "c360.logistics...", "predicate": "dc.subject", "object": "safety"}
]

// StorageRef (points to body in ObjectStore)
{"storage_instance": "CONTENT_STORE", "key": "content/2024/01/15/14/doc-001_1705334400"}
```

The **ContentStorable** interface enables:
- **Lean Entity State**: Only Dublin Core metadata triples (no large body text)
- **Content Deduplication**: Body stored once, referenced by multiple entities
- **Semantic Embedding**: Worker fetches content from ObjectStore using StorageRef

**IoT Sensor Processor** transforms readings into temporal entities:
```json
// Input
{"sensor_id": "temp-001", "reading": 72.5, "unit": "fahrenheit", "location": "warehouse-a"}

// Output Entity ID
"c360.logistics.environmental.sensor.temperature.temp-001"

// Generated Triples
[
  {"subject": "c360.logistics...", "predicate": "sensor.measurement.fahrenheit", "object": 72.5},
  {"subject": "c360.logistics...", "predicate": "geo.location.zone", "object": "warehouse-a"},
  {"subject": "c360.logistics...", "predicate": "time.observation.recorded", "object": "2024-01-15T10:30:00Z"}
]
```

**Rule Processor** evaluates 5 domain-specific rules:

| Rule | Conditions | Severity |
|------|------------|----------|
| `low-battery-alert` | `battery.level <= 20` | Warning |
| `high-temperature-alert` | `data.temperature >= 50.0` | Critical |
| `cold-storage-temp-alert` | `reading >= 40.0 AND unit = "fahrenheit" AND location contains "cold-storage"` | Critical |
| `high-humidity-alert` | `reading >= 50.0 AND type = "humidity"` | Warning |
| `low-air-pressure-alert` | `reading < 100.0 AND type = "pressure"` | Warning |

**Note:** The test data includes a cold storage temperature trend from 36.5°F to 48.2°F, which should trigger `cold-storage-temp-alert` after readings exceed 40°F.

### 3. Graph Storage Layer

The **Graph Processor** maintains entity state and relationships:

**Entity Storage (NATS KV: ENTITY_STATES)**
```json
{
  "entity_id": "c360.logistics.environmental.sensor.temperature.temp-001",
  "triples": [...],
  "version": 42,
  "updated_at": "2024-01-15T10:30:00Z"
}
```

**All 7 Indexes:**

| Index | KV Bucket | Purpose | Example Query |
|-------|-----------|---------|---------------|
| Entity States | `ENTITY_STATES` | Primary entity storage | "Get entity by ID" |
| Predicate | `PREDICATE_INDEX` | Find entities by attribute | "All entities with temperature readings" |
| Incoming | `INCOMING_INDEX` | Find entities pointing TO X | "What references warehouse-a?" |
| **Outgoing** | `OUTGOING_INDEX` | Find entities X points TO | "What does this sensor relate to?" |
| Alias | `ALIAS_INDEX` | Resolve alternate identifiers | "temp-001" → full entity ID |
| Spatial | `SPATIAL_INDEX` | Geographic queries (geohash) | "Sensors within 100m of loading dock" |
| Temporal | `TEMPORAL_INDEX` | Time-range queries (hourly) | "Events in last 24 hours" |

**Optional indexes:** EMBEDDING_INDEX, EMBEDDING_DEDUP, COMMUNITY_INDEX

### 4. Query & Output Layer

**HTTP Gateway** exposes semantic search:
```bash
# Semantic similarity search
curl -X POST http://localhost:8080/search/semantic \
  -d '{"query": "cold storage temperature problems", "limit": 10}'

# Entity lookup
curl http://localhost:8080/entity/c360.logistics.environmental.sensor.temperature.temp-001
```

**Multi-destination Output:**

- **File**: JSONL archive for batch analysis
- **HTTP POST**: Webhooks for external integrations
- **WebSocket**: Real-time browser dashboards
- **ObjectStore**: Immutable message archive

### 5. Search & Query Algorithms

SemStreams provides **6 primary query algorithms** plus supporting algorithms. All primary algorithms work in Core mode with graceful degradation.

**Primary Query Algorithms:**

| Algorithm | Implementation | Use Case | Core Mode |
|-----------|---------------|----------|-----------|
| **BM25** | `embedding/bm25_embedder.go` | Lexical/keyword search | Native Go |
| **Vector** | `embedding/http_embedder.go` | Semantic similarity | Falls back to BM25 |
| **PathRAG** | `graph/query/client.go` | Bounded graph traversal | Native Go |
| **Hybrid** | `indexmanager/semantic.go` | Semantic + temporal + spatial | Native Go |
| **GraphRAG Local** | `querymanager/graphrag_search.go` | Within-community search | Native Go |
| **GraphRAG Global** | `querymanager/graphrag_search.go` | Cross-community search | Statistical summaries |

**Supporting Algorithms:**

| Algorithm | Purpose | Location |
|-----------|---------|----------|
| TF-IDF | Community summarization (statistical fallback) | `graphclustering/summarizer.go` |
| PageRank | Entity importance ranking | `graphclustering/pagerank.go` |
| LPA | Hierarchical community detection | `graphclustering/lpa.go` |
| Geohash | Spatial indexing (grid-based) | `indexmanager/indexes.go` |

**Full algorithm documentation:** See `semdocs/docs/advanced/03-algorithm-reference.md`

### 6. ML Services (Optional Enhancement)

Two optional ML services enhance Core capabilities:

| Service | Port | Purpose | Core Fallback |
|---------|------|---------|---------------|
| **semembed** | 8081 | Neural vector embeddings | BM25 lexical embeddings |
| **semsummarize** | 8084 | LLM community summaries | TF-IDF statistical summaries |

**Progressive enhancement model:**

```text
Level 1: Core (Edge/Offline)
├── Full entity storage and all 7 indexes
├── BM25 lexical search + PathRAG graph traversal
├── Statistical community summaries (TF-IDF)
└── No external service dependencies

Level 2: ML Enhanced (Cloud/Connected)
├── Everything from Level 1
├── semembed: Semantic vector search
├── semsummarize: LLM-generated summaries
└── Enhanced GraphRAG with neural embeddings
```

## Why This Matters

### Before: Disconnected Data Silos

```
Sensors → Time-series DB → Dashboard
Documents → Search Engine → Portal
Maintenance → ERP System → Reports
```
Each system has its own data model. Correlating across silos requires custom ETL.

### After: Unified Knowledge Graph

```
All Sources → SemStreams → Knowledge Graph → Any Query
```

**One query can now answer:**
- *"Show me all temperature sensors in warehouse-a with readings above normal, including related maintenance records and safety observations"*
- *"Find documents semantically similar to 'forklift safety procedures' across all content types"*
- *"What entities are related to equipment that triggered alerts this week?"*

## Running the Kitchen Sink

```bash
# Core variant (CI-safe, no ML dependencies)
task e2e:semantic-kitchen-sink-core

# ML variant (with semembed for embeddings)
task e2e:semantic-kitchen-sink-ml

# Compare both variants
task e2e:semantic-kitchen-compare
```

### Manual Execution

```bash
# Core variant
docker compose -f docker-compose.semantic-kitchen.yml up -d --wait
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink --variant core

# ML variant (requires semembed)
docker compose -f docker-compose.semantic-kitchen.yml --profile ml up -d --wait
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink --variant ml
```

### What the Test Validates

| Stage | Validation | What It Proves |
|-------|------------|----------------|
| verify-components | Required components registered | Framework wiring works |
| send-mixed-data | 20 UDP messages sent | Input pipeline accepts data |
| validate-processing | Graph processor healthy | Semantic processing running |
| test-semantic-search | semembed health check | Embedding service available (ML) |
| test-http-gateway | `/search/semantic` returns 200 | Query API operational |
| test-embedding-fallback | Graph healthy w/o semembed | BM25 fallback works (Core) |
| validate-rules | Rule metrics present | Rule processor running |
| validate-metrics | 4 required metrics exposed | Observability working |
| verify-outputs | 4 output components present | Output pipeline configured |

### Key Metrics

| Metric | Description |
|--------|-------------|
| `indexengine_events_processed_total` | Entities successfully processed by graph |
| `indexengine_index_updates_total` | Index update operations performed |
| `semstreams_cache_hits_total` | L1/L2 cache hits in DataManager |
| `semstreams_cache_misses_total` | Cache misses requiring KV lookups |

## Component Reference

### Services (4)
| Service | Port | Description |
|---------|------|-------------|
| service-manager | :8080 | HTTP API + Swagger UI |
| component-manager | - | Lifecycle orchestration |
| metrics | :9090 | Prometheus metrics endpoint |
| message-logger | - | Debug logging with KV query support |

### Inputs (6)
| Component | Type | Subject | Description |
|-----------|------|---------|-------------|
| udp | UDP | `raw.udp.messages` | Real-time telemetry on :14550 |
| file_documents | File | `raw.document.corpus` | documents.jsonl (12 docs) |
| file_maintenance | File | `raw.document.corpus` | maintenance.jsonl (16 records) |
| file_observations | File | `raw.document.corpus` | observations.jsonl (15 records) |
| file_sensors | File | `raw.sensor.file` | sensors.jsonl (30 readings) |
| file_sensor_docs | File | `raw.document.corpus` | sensor_docs.jsonl (15 docs) |

### Processors (4)
| Component | Input | Output | Description |
|-----------|-------|--------|-------------|
| document_processor | `raw.document.corpus` | `events.graph.entity.document` | ContentStorable → ObjectStore → Graph |
| iot_sensor | `raw.sensor.>` | `events.graph.entity.sensor` | Sensor → Graphable |
| rule | `raw.sensor.>` | `events.rule.triggered` | 5 alert rules |
| graph | `events.graph.entity.*` | KV buckets | Entity + Index + Embedding management |

### Outputs (4)
| Component | Source | Destination | Description |
|-----------|--------|-------------|-------------|
| file | `events.graph.entity.>` | /tmp/*.jsonl | JSONL archive of semantic entities |
| httppost | `events.graph.entity.>` | localhost:9999 | Webhook delivery of entities |
| websocket | `raw.udp.messages` | :8082/ws | Real-time streaming |
| objectstore | `events.graph.entity.>` | NATS ObjectStore | Entity message archive |

### Gateways (1)
| Component | Routes | Description |
|-----------|--------|-------------|
| api-gateway | `/search/semantic`, `/entity/:id` | REST API for queries |

### Storage (NATS KV Buckets + ObjectStore)

| Bucket | Purpose |
|--------|---------|
| graph_entities_kitchen | Entity state storage (metadata + StorageRef) |
| PREDICATE_INDEX | Query by predicate |
| INCOMING_INDEX | Query incoming relationships |
| OUTGOING_INDEX | Query outgoing relationships |
| ALIAS_INDEX | Resolve alternate identifiers |
| SPATIAL_INDEX | Geographic queries (geohash) |
| TEMPORAL_INDEX | Time-range queries (hourly buckets) |
| EMBEDDING_INDEX | Embedding generation queue |
| EMBEDDING_DEDUP | Content-addressed deduplication |
| COMMUNITY_INDEX | Community detection results |
| CONTENT_STORE (ObjectStore) | Document body content storage |

## Framework Story: What Kitchen Sink Proves

### Dynamic Knowledge Graph

**Key differentiator:** SemStreams maintains a **real-time, incrementally-updated** knowledge graph—not a static graph requiring batch rebuilds.

- **Live updates**: Entities and indexes update as data arrives via KV watch patterns
- **Eventual consistency**: Graph reflects current state within milliseconds
- **No rebuild cycles**: Add entities, update relationships, query immediately

### Core Variant (Edge/Offline)

The Core variant proves SemStreams runs **anywhere without external dependencies**—from Raspberry Pi to data center:

1. **Ingest heterogeneous data** - Multiple input types (UDP, File) with different formats
2. **Transform to semantic model** - Raw JSON becomes federated entities with Dublin Core metadata
3. **Separate content from triples** - ContentStorable stores body in ObjectStore, metadata in triples
4. **Persist to knowledge graph** - Entities stored in NATS KV with versioning and StorageRef
5. **All 7 indexes operational** - Predicate, incoming, outgoing, alias, spatial, temporal
6. **BM25 lexical search** - Native Go, no ML service dependencies
7. **PathRAG graph traversal** - Bounded DFS with relevance decay
8. **Statistical summaries** - TF-IDF community summaries without LLM
9. **Evaluate rules** - Threshold-based alerting on streaming data
10. **Route to outputs** - Fan-out to multiple destinations (file, HTTP, WebSocket)

### ML Variant (Cloud/Connected)

The ML variant adds **enhanced semantic capabilities** when connected to ML services:

1. **Neural embeddings** - Worker fetches body from ObjectStore via StorageRef, vectors via semembed
2. **Semantic search** - Find similar entities by meaning, not just keywords
3. **LLM summaries** - semsummarize generates richer community descriptions
4. **Enhanced GraphRAG** - Community-based search with neural similarity scoring
5. **Hybrid retrieval** - Combine BM25 lexical with embedding similarity

### Test Data Story

The logistics warehouse scenario tells a coherent story with **ContentStorable** architecture:

1. **Baseline Setup** (file inputs at startup):
   - 12 operational documents (safety manuals, SOPs, procedures) → ObjectStore + triples
   - 16 maintenance records (equipment repairs, inspections) → ObjectStore + triples
   - 15 safety observations (incidents, near-misses, violations) → ObjectStore + triples
   - 15 sensor documentation (metadata about monitoring equipment) → ObjectStore + triples
   - 30 sensor readings (temperature, humidity, pressure trends) → triples only

2. **Content Storage Pattern**:
   - Document body stored in ObjectStore (CONTENT_STORE)
   - Dublin Core metadata in triples (dc.title, dc.type, dc.subject)
   - StorageRef links entity to full content
   - Embedding worker fetches content via StorageRef

3. **Live Operations** (UDP stream during test):
   - Real-time telemetry streams to WebSocket for dashboards
   - Sensor data evaluated by rule processor for alerts

4. **Alert Scenario**:
   - Cold storage sensor (`temp-sensor-001`) shows temperature rising
   - Readings: 36.5°F → 37.1°F → 38.2°F → 41.2°F → 45.1°F → 48.2°F
   - `cold-storage-temp-alert` should trigger when reading >= 40°F

5. **Query Scenario** (what you can ask):
   - "What maintenance was done on cold storage equipment?"
   - "Are there safety observations related to temperature?"
   - "Find all sensors in zone-a"
   - "What documents mention forklift safety?"

## Assertion Gaps (Future Improvements)

Current test validates presence but not behavior. To fully prove the framework story:

### Entity Storage Assertions (Priority: High)

```go
// Verify entities were persisted
entities, _ := kvClient.Keys("graph_entities_kitchen")
assert.GreaterOrEqual(len(entities), 58) // 12+16+15+15 from file inputs

// Verify specific entity exists
entity, _ := kvClient.Get("c360.logistics.content.document.safety.doc-safety-001")
assert.NotNil(entity)
```

### Index Population Assertions (Priority: High)

```go
// Verify predicate index populated with Dublin Core predicates
predicates, _ := kvClient.Keys("PREDICATE_INDEX")
assert.Contains(predicates, "dc.title")
assert.Contains(predicates, "dc.subject")
assert.Contains(predicates, "sensor.type")

// Verify spatial index for sensors with coordinates
spatialKeys, _ := kvClient.Keys("SPATIAL_INDEX")
assert.GreaterOrEqual(len(spatialKeys), 15) // sensors have lat/lon

// Verify content stored in ObjectStore
contentKeys, _ := objStore.List("content/")
assert.GreaterOrEqual(len(contentKeys), 58) // documents + maintenance + observations + sensor_docs
```

### Rule Triggering Assertions (Priority: Medium)

```go
// Subscribe to rule output and verify cold-storage alert fired
sub := nc.Subscribe("events.rule.triggered")
// Send sensor reading > 40F for cold-storage-1
// Assert: received message with rule_id="cold-storage-temp-alert"
```

### Search Quality Assertions (Priority: High for ML)

```go
// Query and verify relevance
results := gateway.SemanticSearch("forklift safety procedures")
assert.GreaterOrEqual(len(results), 2)
assert.Contains(results[0].EntityID, "forklift") // Most relevant
```

### End-to-End Flow Assertions (Priority: Medium)

```go
// Verify file output received semantic entities
files, _ := filepath.Glob("/tmp/semstreams-kitchen-sink*.jsonl")
assert.GreaterOrEqual(len(files), 1)
content, _ := os.ReadFile(files[0])
assert.Contains(string(content), "dc.title") // Dublin Core metadata
assert.Contains(string(content), "storage_ref") // ContentStorable reference
```

## Next Steps

- [Semantic Search Guide](../guides/semantic-search.md) - Deep dive into query capabilities
- [Custom Processors](../guides/custom-processors.md) - Build your own Graphable transformers
- [Rule Engine](../guides/rules.md) - Complex event processing
- [Federation](../scenarios/federation.md) - Edge-to-cloud data flow
