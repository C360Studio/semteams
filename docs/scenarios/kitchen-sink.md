# Semantic Kitchen Sink

## Why a Warehouse?

We needed a test scenario that exercises the full capability stack. A logistics warehouse works because it naturally includes:

- **Mixed data types**: Sensors (time-series), documents (text), maintenance records (structured), safety observations (events)
- **Two Graphable patterns**:
  - IoT telemetry → triples only (lightweight, high-frequency)
  - Documents/records → body stored separately, metadata as triples (ContentStorable)
- **Temporal patterns**: Data arrives continuously with time-based relationships
- **Spatial relationships**: Equipment has zones, sensors have locations
- **Threshold alerting**: Temperature violations need stateful detection
- **Text search**: Finding related documents by content similarity

This validates that flow-based streaming, graph storage, indexing, clustering, and search work correctly with both lightweight telemetry and rich documents.

## What We Test

| Capability | Warehouse Scenario Coverage |
|------------|----------------------------|
| Flow-based streaming | Sensors → processors → graph |
| Entity storage | 74 entities across 5 types |
| 7 index types | Predicate, spatial, temporal, incoming, outgoing, alias |
| Stateful rules | Cold storage temp alert with OnEnter/OnExit |
| BM25 search | "forklift safety" finds related docs |
| LPA clustering | Groups related sensors/docs by zone |
| Semantic edges | Similar documents cluster together (ML) |

## Deployment Profiles

SemStreams is a single unified system with configurable capabilities:

| Profile | Target | What It Enables |
|---------|--------|-----------------|
| **Edge** | Raspberry Pi, offline devices | Rules, basic queries, graph storage |
| **Core** | Laptop, edge server | + BM25 search, clustering, GraphRAG |
| **ML** | Cloud, GPU available | + Neural search, semantic edges, LLM summaries |

### Edge Profile

**Config:** `configs/tier0-rules-iot.json`

Minimal footprint for resource-constrained devices:

- Stateful rules with OnEnter/OnExit state machines
- Entity storage with 7 indexes
- No external dependencies

### Core Profile

**Config:** `configs/semantic-kitchen-sink.json`

Full graph capabilities without ML services:

- BM25 keyword search (native Go, zero deps)
- LPA clustering on explicit edges
- TF-IDF community summaries
- GraphRAG traversal

### ML Profile

**Config:** `configs/semantic-kitchen-sink-ml.json`

Everything above, plus neural capabilities:

- Neural embeddings via semembed
- Semantic edges enhance clustering
- LLM community summaries via seminstruct (OpenAI-compatible API)

## Progressive Enhancement

Each capability degrades gracefully:

| When... | Falls back to... |
|---------|------------------|
| Neural embedding service down | BM25 keyword search |
| LLM summarizer unavailable | TF-IDF statistical summaries |
| Embeddings disabled in config | Clustering uses explicit edges only |

**Always available (any profile):**

- Entity/predicate/spatial/temporal queries
- Relationship traversal
- PathRAG graph walks
- Rule-based alerting

## Kitchen Sink Architecture

The kitchen sink scenario demonstrates Graphable processing with flow-based subject routing:

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DYNAMIC KNOWLEDGE GRAPH                              │
│                   (Real-time updates via KV watch)                          │
└─────────────────────────────────────────────────────────────────────────────┘

DATA SOURCES (File)             DOMAIN PROCESSORS                    STORAGE
───────────────────             ─────────────────                    ───────

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
                               (OnEnter/OnExit)                      │
                                                                     ▼
                                                        ┌────────────────────────┐
                                                        │ events.graph.entity.*  │
                                                        │   (wildcard subscribe) │
                                                        └───────────┬────────────┘
                                                                    │
                                                                    ▼
                                                        ┌────────────────────────┐
                                                        │    GRAPH PROCESSOR     │
                                                        │  • Entity Storage      │
                                                        │  • 7 Indexes           │
                                                        │  • Clustering (Core+)  │
                                                        │  • Inference (Core+)   │
                                                        └───────────┬────────────┘
                                                                    │
                    ┌───────────────────────────────────────────────┼───────────────────────────────────┐
                    │                                               │                                   │
                    ▼                                               ▼                                   ▼
        ┌───────────────────┐                           ┌───────────────────┐               ┌───────────────────┐
        │   ENTITY_STATES   │                           │      INDEXES      │               │ EMBEDDING_INDEX   │
        │ (metadata+ref)    │                           │                   │               │   (Core+)         │
        └───────────────────┘                           │ • PREDICATE       │               └─────────┬─────────┘
                                                        │ • INCOMING        │                         │
                                                        │ • OUTGOING        │                         ▼
                                                        │ • ALIAS           │               ┌───────────────────┐
                                                        │ • SPATIAL         │               │ EMBEDDING WORKER  │
                                                        │ • TEMPORAL        │               │   (Core+)         │
                                                        └───────────────────┘               │  ◄──── ObjectStore│
                                                                                            │  (fetch body via  │
                                                                                            │   StorageRef)     │
                                                                                            └───────────────────┘

OUTPUTS (subscribe to events.graph.entity.>):
─────────────────────────────────────────────
• file output      → JSONL archive
• httppost         → Webhooks
• objectstore      → Message archive
• api-gateway      → REST API queries
```

**Key architecture points:**

- **Subject-based routing** - processors subscribe to subject patterns (e.g., `events.graph.entity.>`)
- **ContentStorable types** (Document, Maintenance, Observation) store body to ObjectStore FIRST
- **Graph receives metadata only** - body replaced by StorageRef pointer
- **Embedding worker** fetches full content from ObjectStore when generating vectors
- **All indexes updated atomically** via KV watch pattern

## Data Flow Explained

### 1. Ingestion Layer

#### Document Corpus (File Inputs) - ContentStorable Pattern

```text
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

#### Sensor Readings (File Input)

```text
sensors.jsonl → raw.sensor.file → iot_sensor → events.graph.entity.sensor
                      ↓
                rule_processor → events.rule.triggered (if thresholds exceeded)
```

Time-series sensor data becomes queryable entities. The rule processor monitors sensor streams for threshold violations.

### 2. Semantic Processing Layer

#### Document Processor

Transforms incoming JSON into federated entities using **Dublin Core** metadata:

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

#### IoT Sensor Processor

Transforms readings into temporal entities:

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

#### Rule Processor

Evaluates domain-specific rules with stateful transitions:

| Rule | Conditions | Severity | OnEnter | OnExit |
|------|------------|----------|---------|--------|
| `cold-storage-temp-alert` | `sensor.measurement.fahrenheit >= 40 AND geo.location.zone contains "cold-storage"` | Critical | Add `alert.active` triple | Remove `alert.active` triple |
| `high-humidity-alert` | `sensor.measurement.percent >= 50 AND sensor.classification.type = "humidity"` | Warning | Add `alert.active` triple | Remove `alert.active` triple |
| `low-pressure-alert` | `sensor.measurement.psi < 100 AND sensor.classification.type = "pressure"` | Warning | Add `alert.active` triple | Remove `alert.active` triple |

**Stateful Rule Behavior (Edge+):**

- **OnEnter**: When conditions become true, execute actions (add_triple, publish)
- **OnExit**: When conditions become false, execute cleanup (remove_triple, publish)
- **Cooldown**: Prevent rapid-fire alerts with configurable cooldown periods

**Note:** The test data includes a cold storage temperature trend from 36.5°F to 48.2°F, which triggers `cold-storage-temp-alert` after readings exceed 40°F.

### 3. Graph Storage Layer

The **Graph Processor** maintains entity state and relationships:

#### Entity Storage (NATS KV: ENTITY_STATES)

```json
{
  "entity_id": "c360.logistics.environmental.sensor.temperature.temp-001",
  "triples": [...],
  "version": 42,
  "updated_at": "2024-01-15T10:30:00Z"
}
```

#### All 7 Indexes

| Index | KV Bucket | Purpose | Example Query |
|-------|-----------|---------|---------------|
| Entity States | `ENTITY_STATES` | Primary entity storage | "Get entity by ID" |
| Predicate | `PREDICATE_INDEX` | Find entities by attribute | "All entities with temperature readings" |
| Incoming | `INCOMING_INDEX` | Find entities pointing TO X | "What references warehouse-a?" |
| **Outgoing** | `OUTGOING_INDEX` | Find entities X points TO | "What does this sensor relate to?" |
| Alias | `ALIAS_INDEX` | Resolve alternate identifiers | "temp-001" → full entity ID |
| Spatial | `SPATIAL_INDEX` | Geographic queries (geohash) | "Sensors within 100m of loading dock" |
| Temporal | `TEMPORAL_INDEX` | Time-range queries (hourly) | "Events in last 24 hours" |

**Optional indexes (Core+):** EMBEDDING_INDEX, EMBEDDING_DEDUP, COMMUNITY_INDEX

### 4. Inference System

SemStreams generates inferred relationships through two mechanisms:

#### Rule-Based Inference (Edge+)

Rules with `add_triple`/`remove_triple` actions create explicit relationships dynamically:

```json
{
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.active", "object": "cold-storage-violation"}
  ],
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.active", "object": "cold-storage-violation"}
  ]
}
```

These relationships are **deterministic** - they exist exactly when rule conditions are met.

#### Community-Based Inference (Core+)

The graph processor runs periodic community detection:

**What triggers inference:**

- Community detection loop runs every 2 minutes (configurable)
- Initial delay: 10 seconds after startup
- Readiness checks: entity count >= 10, embedding coverage >= 50% (ML only)

**How it works:**

1. **LPA (Label Propagation Algorithm)** detects communities in the entity graph
2. **Core**: Uses explicit relationship edges only
3. **ML**: Augments explicit edges with **semantic edges** (virtual neighbors from embedding similarity, threshold: 0.6)
4. For each community, generate **inferred triples** between co-members

**Inferred triple format:**

```json
{
  "subject": "entity-A",
  "predicate": "inferred.clustered_with",
  "object": "entity-B",
  "source": "lpa_community_detection",
  "confidence": 0.65,
  "community_id": "community-123"
}
```

**Confidence scoring:**

- Base: 0.5 (any community co-membership)
- Bonus: Up to +0.3 based on community "tightness" (edge density)
- Range: 0.5 (loose community) to 0.8 (dense community)

**Key configuration options:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `clustering.enabled` | false | Enable community detection |
| `clustering.schedule.detection_interval` | 2m | How often to run detection |
| `clustering.schedule.min_entities` | 10 | Skip if fewer entities |
| `semantic_edges.enabled` | false | Enable virtual edges from embeddings (ML) |
| `semantic_edges.similarity_threshold` | 0.6 | Minimum similarity for virtual edge |
| `inference.enabled` | false | Generate inferred triples |
| `inference.min_community_size` | 2 | Skip singleton communities |

### 5. Query & Output Layer

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
- **ObjectStore**: Immutable message archive

### 6. Search & Query Algorithms

SemStreams provides **6 primary query algorithms** plus supporting algorithms. All primary algorithms work in Core with graceful degradation.

**Primary Query Algorithms:**

| Algorithm | Implementation | Use Case | Core Profile |
|-----------|---------------|----------|--------|
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

**Algorithm implementations:** See `processor/graph/embedding/` (BM25, HTTP embedder) and `processor/graph/querymanager/` (PathRAG, GraphRAG, Hybrid)

#### How Embeddings Work

Both embedding providers generate fixed-dimension vectors for cosine similarity search:

| Provider | How It Works | Output |
|----------|--------------|--------|
| **BM25** | Hashes keywords into fixed dimensions, weights by term frequency (TF-IDF) | 384-dim vector |
| **HTTP** | Neural model encodes semantic meaning | 384-dim vector |

**Key difference:** BM25 finds exact keyword matches. Neural embeddings understand meaning—"car" matches "vehicle" even without shared words.

Both use the same `Embedder` interface and search treats them identically via cosine similarity. The system automatically falls back from HTTP to BM25 if the embedding service is unavailable.

### 7. ML Services (ML Profile Only)

Two optional ML services enhance Core profile capabilities:

| Service | Port | Purpose | Core Fallback |
|---------|------|---------|---------------|
| **semembed** | 8081 | Neural vector embeddings | BM25 lexical embeddings |
| **seminstruct** | 8080 | LLM community summaries (OpenAI API) | TF-IDF statistical summaries |

See [Progressive Enhancement](#progressive-enhancement) for capability fallback behavior.

## Running the Kitchen Sink

### Tiered Commands (Recommended)

```bash
# Tier 0: Rules-only (stateful alerting, no clustering, no embeddings)
task e2e:tier0

# Tier 1: Core variant (BM25 embeddings, LPA clustering, no ML dependencies)
task e2e:tier1
# or: task e2e:semantic-kitchen-sink-core

# Tier 2: ML variant (HTTP neural embeddings via semembed)
task e2e:tier2
# or: task e2e:semantic-kitchen-sink-ml

# Run all tiers with comparison
task e2e:tiers

# Compare Tier 1 vs Tier 2 results
task e2e:semantic-kitchen-compare
```

### Manual Execution

```bash
# Tier 0: Rules-only
docker compose -f docker-compose.rules.yml up -d --wait
cd cmd/e2e && ./e2e --scenario tier0-rules-iot

# Tier 1: Core variant
docker compose -f docker-compose.semantic-kitchen.yml up -d --wait
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink --variant core

# Tier 2: ML variant (requires semembed + seminstruct)
docker compose -f docker-compose.semantic-kitchen.yml --profile ml up -d --wait
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink --variant ml --base-url http://localhost:8180 --metrics-url http://localhost:9190
```

### Configuration Files

Each profile uses a separate configuration file:

| File | Profile | Embedding | Clustering | Description |
|------|---------|-----------|------------|-------------|
| `configs/tier0-rules-iot.json` | Edge | Disabled | Disabled | Rules-only, deterministic |
| `configs/semantic-kitchen-sink.json` | Core | BM25 | Enabled | CI-safe, no external deps |
| `configs/semantic-kitchen-sink-ml.json` | ML | HTTP | Enabled | Full neural semantic |

The ML config (`semantic-kitchen-sink-ml.json`) adds:

```json
"embedding": {
  "enabled": true,
  "provider": "http",
  "http_endpoint": "http://semembed:8081/v1",
  "http_model": "BAAI/bge-small-en-v1.5"
}
```

**Note:** The `http_endpoint` must include `/v1` suffix because the OpenAI-compatible client appends `/embeddings` to the base URL.

### Comparison Output Files

Each profile run generates comparison files in `cmd/e2e/test/e2e/results/`:

- `comparison-core-{timestamp}.json` - Core profile (BM25) results
- `comparison-ml-{timestamp}.json` - ML profile (HTTP) results
- `community-comparison-core-{timestamp}.json` - Core profile community detection
- `community-comparison-ml-{timestamp}.json` - ML profile community detection

Key metrics to compare:

| Metric | Core (BM25) | ML (HTTP) |
|--------|---------------|---------------|
| `embedding_provider` | bm25 | http |
| Search: "safety documentation" | ~1 hit | ~5 hits |
| Search: "temperature sensor" | ~2 hits | ~3 hits |
| `communities_llm_enhanced` | 0 | 18+ |

### What Each Profile Validates

**Edge Profile Validation:**

| Stage | Validation | What It Proves |
|-------|------------|----------------|
| verify-tier0-config | Embedding/clustering disabled | Deterministic mode active |
| verify-components | rule, graph, iot_sensor present | Framework wiring works |
| send-iot-data-trigger | Data crossing thresholds | OnEnter actions fire |
| validate-on-enter | Triples added, alerts created | Dynamic graph mutations work |
| send-iot-data-clear | Data returning to normal | OnExit actions fire |
| validate-on-exit | Triples removed | Graph cleanup works |
| validate-no-inference | 0 embeddings, 0 clusters | No ML inference occurred |

**Core/ML Profile Validation:**

| Stage | Validation | What It Proves |
|-------|------------|----------------|
| verify-components | Required components registered | Framework wiring works |
| validate-processing | Graph processor healthy | Semantic processing running |
| verify-entity-count | 74 entities from test data | Data ingestion complete |
| verify-index-population | All 7 indexes populated | Index pipeline works |
| test-semantic-search | semembed health check | Embedding service available (ML) |
| test-http-gateway | `/search/semantic` returns 200 | Query API operational |
| test-embedding-fallback | Graph healthy w/o semembed | BM25 fallback works (Core) |
| validate-rules | Rule metrics present | Rule processor running |
| validate-metrics | 4 required metrics exposed | Observability working |
| verify-outputs | Output components present | Output pipeline configured |

### Key Metrics

| Metric | Description |
|--------|-------------|
| `indexengine_events_processed_total` | Entities successfully processed by graph |
| `indexengine_index_updates_total` | Index update operations performed |
| `semstreams_cache_hits_total` | L1/L2 cache hits in DataManager |
| `semstreams_cache_misses_total` | Cache misses requiring KV lookups |
| `semstreams_rule_evaluations_total` | Rule conditions evaluated |
| `semstreams_rule_triggers_total` | Rules that fired (conditions met) |
| `semstreams_rule_state_transitions_total` | OnEnter/OnExit transitions |

## Component Reference

### Services (4)

| Service | Port | Description |
|---------|------|-------------|
| service-manager | :8080 | HTTP API + Swagger UI |
| component-manager | - | Lifecycle orchestration |
| metrics | :9090 | Prometheus metrics endpoint |
| message-logger | - | Debug logging with KV query support |

### Inputs (5)

| Component | Type | Subject | Description |
|-----------|------|---------|-------------|
| file_documents | File | `raw.document.corpus` | documents.jsonl (12 docs) |
| file_maintenance | File | `raw.document.corpus` | maintenance.jsonl (16 records) |
| file_observations | File | `raw.document.corpus` | observations.jsonl (15 records) |
| file_sensor_docs | File | `raw.document.corpus` | sensor_docs.jsonl (15 docs) |
| file_sensors | File | `raw.sensor.file` | sensors.jsonl (41 readings, 16 unique devices) |

### Processors (4)

| Component | Input | Output | Description |
|-----------|-------|--------|-------------|
| document_processor | `raw.document.corpus` | `events.graph.entity.document` | ContentStorable → ObjectStore → Graph |
| iot_sensor | `raw.sensor.>` | `events.graph.entity.sensor` | Sensor → Graphable |
| rule | `events.graph.entity.>` | `events.rule.triggered` | Stateful rules with OnEnter/OnExit |
| graph | `events.graph.entity.*` | KV buckets | Entity + Index + Clustering + Inference |

### Outputs (3)

| Component | Source | Destination | Description |
|-----------|--------|-------------|-------------|
| file | `events.graph.entity.>` | /tmp/*.jsonl | JSONL archive of semantic entities |
| httppost | `events.graph.entity.>` | localhost:9999 | Webhook delivery of entities |
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
| EMBEDDING_INDEX | Embedding generation queue (Core+) |
| EMBEDDING_DEDUP | Content-addressed deduplication (Core+) |
| COMMUNITY_INDEX | Community detection results (Core+) |
| CONTENT_STORE (ObjectStore) | Document body content storage |

## Framework Story: What Kitchen Sink Proves

### Dynamic Knowledge Graph

**Key differentiator:** SemStreams maintains a **real-time, incrementally-updated** knowledge graph—not a static graph requiring batch rebuilds.

- **Live updates**: Entities and indexes update as data arrives via KV watch patterns
- **Eventual consistency**: Graph reflects current state within milliseconds
- **No rebuild cycles**: Add entities, update relationships, query immediately

### What Edge Profile Proves (Rules-Only)

Proves SemStreams can run **deterministic, low-latency alerting** without any ML overhead:

1. **Stateful rules** - OnEnter/OnExit transitions track alert state
2. **Dynamic graph mutations** - Rules create/remove relationships in real-time
3. **Zero inference** - No embeddings, no clustering, purely deterministic
4. **Hotpath-only** - Minimal latency for real-time alerting

### What Core Profile Proves

Proves SemStreams runs **anywhere without external dependencies**—from Raspberry Pi to data center:

1. **Ingest heterogeneous data** - Multiple input types with different formats
2. **Transform to semantic model** - Raw JSON becomes federated entities with Dublin Core metadata
3. **Separate content from triples** - ContentStorable stores body in ObjectStore, metadata in triples
4. **Persist to knowledge graph** - Entities stored in NATS KV with versioning and StorageRef
5. **All 7 indexes operational** - Predicate, incoming, outgoing, alias, spatial, temporal
6. **BM25 lexical search** - Native Go, no ML service dependencies
7. **LPA community detection** - Group related entities automatically
8. **Statistical summaries** - TF-IDF community summaries without LLM
9. **PathRAG graph traversal** - Bounded DFS with relevance decay
10. **Evaluate rules** - Threshold-based alerting on streaming data
11. **Route to outputs** - Fan-out to multiple destinations (file, HTTP)

### What ML Profile Proves

Adds **enhanced semantic capabilities** when connected to ML services:

1. **Neural embeddings** - Worker fetches body from ObjectStore via StorageRef, vectors via semembed
2. **Semantic edges** - Virtual neighbors from embedding similarity enhance clustering
3. **Semantic search** - Find similar entities by meaning, not just keywords
4. **LLM summaries** - seminstruct generates richer community descriptions via OpenAI API
5. **Enhanced GraphRAG** - Community-based search with neural similarity scoring
6. **Hybrid retrieval** - Combine BM25 lexical with embedding similarity

### Test Data Story

The logistics warehouse scenario tells a coherent story with **ContentStorable** architecture:

1. **Baseline Setup** (file inputs at startup):
   - 12 operational documents (safety manuals, SOPs, procedures) → ObjectStore + triples
   - 16 maintenance records (equipment repairs, inspections) → ObjectStore + triples
   - 15 safety observations (incidents, near-misses, violations) → ObjectStore + triples
   - 15 sensor documentation (metadata about monitoring equipment) → ObjectStore + triples
   - 41 sensor readings from 16 unique devices → triples only

2. **Total entities: 74**
   - documents: 12
   - maintenance: 16
   - observations: 15
   - sensor_docs: 15
   - sensors: 16 unique device IDs

3. **Content Storage Pattern**:
   - Document body stored in ObjectStore (CONTENT_STORE)
   - Dublin Core metadata in triples (dc.title, dc.type, dc.subject)
   - StorageRef links entity to full content
   - Embedding worker fetches content via StorageRef

4. **Alert Scenario**:
   - Cold storage sensor (`temp-sensor-001`) shows temperature rising
   - Readings: 36.5°F → 37.1°F → 38.2°F → 41.2°F → 45.1°F → 48.2°F
   - `cold-storage-temp-alert` triggers when reading >= 40°F
   - OnEnter adds `alert.active` triple
   - OnExit removes it when temperature drops below threshold

5. **Query Scenario** (tested in E2E):
   - "What maintenance was done on cold storage equipment?"
   - "Are there safety observations related to temperature?"
   - "Find all sensors in zone-a"
   - "What documents mention forklift safety?"

## Implemented Assertions

The E2E test validates actual behavior with comprehensive assertions:

### Entity Storage Assertions

- **Entity count verification**: Compares expected (74 from test data) vs actual count with data loss detection
- **Entity retrieval**: Validates specific entities can be retrieved with fully-qualified IDs
- **Entity structure validation**: Samples 5 entities and validates:
  - ID format (non-empty, dot-separated segments)
  - Triples array (non-empty, valid subject/predicate)
  - Version (positive integer)
  - UpdatedAt timestamp (if present)

### Index Population Assertions

- **All 7 indexes verified**: entity_states, predicate, incoming, outgoing, alias, spatial, temporal
- **Key count per bucket**: Reports how many keys in each index
- **Sample keys**: Returns sample keys for debugging

### Rule Triggering Assertions

- **Rule metrics verification**: Extracts actual counter values (not just presence)
- **Before/after comparison**: Measures rules triggered during test
- **Delta tracking**: Reports `rules_triggered_delta` and `rules_evaluated_delta`
- **State transitions**: Validates OnEnter/OnExit fired expected times (Edge)

### Search Quality Assertions

- **Score thresholds**: Each query has `minScore` and `minHits` requirements
- **Quality metrics**:
  - `search_quality_score`: Average score across all hits
  - `search_min_score_met`: Count of queries meeting minimum score
  - `search_min_hits_met`: Count of queries with enough hits
- **Weak results warning**: Alerts if average score < 0.5

### Profile Comparison Assertions

- **Result persistence**: Saves `comparison-{variant}-{timestamp}.json` for each run
- **Automated analysis**: `--analyze-comparison` generates report with:
  - Jaccard similarity (hit overlap) per query
  - Pearson correlation for shared hit scores
  - Verdict: "ML provides semantic lift" / "Marginal difference"

### Rule Configuration Reference

Rules are defined in config files with expression-based conditions:

```json
{
  "id": "cold-storage-temp-alert",
  "type": "expression",
  "name": "Cold Storage Temperature Alert",
  "enabled": true,
  "conditions": [
    {"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0},
    {"field": "geo.location.zone", "operator": "contains", "value": "cold-storage"}
  ],
  "logic": "and",
  "cooldown": "5s",
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.active", "object": "cold-storage-violation"},
    {"type": "publish", "subject": "alerts.cold-storage"}
  ],
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.active", "object": "cold-storage-violation"},
    {"type": "publish", "subject": "alerts.cold-storage.cleared"}
  ],
  "metadata": {"severity": "critical", "category": "food_safety"}
}
```

## Future Improvements

Areas for further enhancement:

### Content Storage Assertions

```go
// Verify content stored in ObjectStore
contentKeys, _ := objStore.List("content/")
assert.GreaterOrEqual(len(contentKeys), 58) // documents + maintenance + observations + sensor_docs
```

### End-to-End Flow Assertions

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
