# SemStreams Configurations

This directory contains configuration files for SemStreams deployment tiers and external services.

## Deployment Tiers

SemStreams supports a 3-tier deployment model with progressively more capable component configurations:

| Tier | Name | Description | External Services |
|------|------|-------------|-------------------|
| 0 | **Structural** | Base tier - rules-only, deterministic behavior | None |
| 1 | **Statistical** | Adds BM25 embeddings, statistical clustering, anomaly detection | None |
| 2 | **Semantic** | Full ML with external embedding and LLM services | semembed, seminstruct |

### Component-to-Tier Matrix

| Component | Tier 0 (Structural) | Tier 1 (Statistical) | Tier 2 (Semantic) | Description |
|-----------|:-------------------:|:--------------------:|:-----------------:|-------------|
| `graph-ingest` | Required | Required | Required | Entity/triple ingestion |
| `graph-index` | Required | Required | Required | Core relationship indexes |
| `graph-gateway` | Required | Required | Required | HTTP API (GraphQL/MCP) |
| `graph-embedding` | - | BM25 | HTTP | Vector embeddings |
| `graph-clustering` | - | Statistical | + LLM | Community detection |
| `graph-anomalies` | - | Required | Required | Cluster/community anomaly detection |
| `graph-index-spatial` | Optional | Optional | Optional | Geospatial queries |
| `graph-index-temporal` | Optional | Optional | Optional | Time-based queries |

### Tier 0: Structural (`structural.json`)

Rules-only deployment without ML inference. Deterministic and predictable.

**Components:**
- `graph-ingest` - Entity/triple ingestion from streams
- `graph-index` - Core relationship indexing
- `graph-gateway` - GraphQL/MCP HTTP gateway
- `rule-processor` - Stateful rule evaluation

### Tier 1: Statistical (`statistical.json`)

Adds statistical algorithms for embeddings and clustering without external ML services.

**Additional Components:**
- `graph-embedding` (BM25) - Statistical term-frequency embeddings
- `graph-clustering` (no LLM) - Statistical community detection
- `graph-anomalies` - Cluster/community anomaly detection via k-core and pivot analysis

### Tier 2: Semantic (`semantic.json`)

Full-featured deployment with external ML services for embeddings and LLM enhancement.

**Enhanced Components:**
- `graph-embedding` (HTTP) - ML-based vector embeddings via external service
- `graph-clustering` (+ LLM) - Community detection with LLM-based summarization
- `graph-anomalies` - Full anomaly detection capabilities
- `graph-index-spatial` - Geospatial indexing
- `graph-index-temporal` - Time-based indexing

**Required External Services:**
- `semembed:8081` - Embedding service
- `seminstruct:8083` - LLM instruction service

## Graph Component Architecture

The graph processing layer uses a modular component architecture with KV-watch based event flow:

```
Entity Streams ──► graph-ingest ──► ENTITY_STATES KV
                                          │
                                          ▼ (kv-watch)
                                    graph-index
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    ▼                     ▼                     ▼
             OUTGOING_INDEX        INCOMING_INDEX        ALIAS_INDEX
                    │                     │
                    └─────────┬───────────┘
                              ▼
                       graph-anomalies
                              │
                              ▼
                       STRUCTURAL_INDEX

Tier 1+ Only:
        ENTITY_STATES
              │
              ▼ (kv-watch)
        graph-embedding ──► EMBEDDINGS_CACHE
              │
              ▼ (reads KV)
        graph-clustering ──► COMMUNITY_INDEX
```

## Graph Component Configuration

### graph-ingest

Ingests entities and triples from JetStream.

**Tier:** All tiers (required)

```json
{
  "type": "processor",
  "name": "graph-ingest",
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_in", "subject": "entity.>", "type": "jetstream"}
      ],
      "outputs": [
        {"name": "entity_states", "subject": "ENTITY_STATES", "type": "kv"}
      ]
    },
    "enable_hierarchy": true
  }
}
```

### graph-index

Maintains relationship indexes from entity state changes.

**Tier:** All tiers (required)

```json
{
  "type": "processor",
  "name": "graph-index",
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
      ],
      "outputs": [
        {"name": "outgoing_index", "subject": "OUTGOING_INDEX", "type": "kv"},
        {"name": "incoming_index", "subject": "INCOMING_INDEX", "type": "kv"},
        {"name": "alias_index", "subject": "ALIAS_INDEX", "type": "kv"},
        {"name": "predicate_index", "subject": "PREDICATE_INDEX", "type": "kv"}
      ]
    },
    "workers": 4,
    "batch_size": 50
  }
}
```

### graph-embedding

Generates vector embeddings for entities.

**Tier:** Statistical (BM25), Semantic (HTTP)

```json
{
  "type": "processor",
  "name": "graph-embedding",
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
      ],
      "outputs": [
        {"name": "embeddings", "subject": "EMBEDDINGS_CACHE", "type": "kv"}
      ]
    },
    "embedder_type": "bm25",
    "batch_size": 50,
    "cache_ttl": "1h"
  }
}
```

For Semantic tier, use `"embedder_type": "http"` with `"embedder_url": "http://semembed:8081/v1"`.

### graph-clustering

Performs community detection with optional LLM enhancement.

**Tier:** Statistical (no LLM), Semantic (with LLM)

```json
{
  "type": "processor",
  "name": "graph-clustering",
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
      ],
      "outputs": [
        {"name": "communities", "subject": "COMMUNITY_INDEX", "type": "kv"}
      ]
    },
    "detection_interval": "30s",
    "batch_size": 100,
    "enable_llm": false,
    "min_community_size": 2
  }
}
```

For Semantic tier, use `"enable_llm": true` with `"llm_endpoint": "http://seminstruct:8083/v1"`.

### graph-anomalies

Detects cluster/community anomalies via k-core decomposition and pivot distance analysis.

**Tier:** Statistical, Semantic

```json
{
  "type": "processor",
  "name": "graph-anomalies",
  "config": {
    "ports": {
      "inputs": [
        {"name": "outgoing_watch", "subject": "OUTGOING_INDEX", "type": "kv-watch"},
        {"name": "incoming_watch", "subject": "INCOMING_INDEX", "type": "kv-watch"}
      ],
      "outputs": [
        {"name": "structural_index", "subject": "STRUCTURAL_INDEX", "type": "kv"}
      ]
    },
    "compute_interval": "1h",
    "pivot_count": 16,
    "max_hop_distance": 10,
    "compute_on_startup": true
  }
}
```

### graph-index-spatial (Optional)

Provides geospatial indexing for location-aware queries.

**Tier:** Any tier (optional)

```json
{
  "type": "processor",
  "name": "graph-index-spatial",
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
      ],
      "outputs": [
        {"name": "spatial_index", "subject": "SPATIAL_INDEX", "type": "kv"}
      ]
    },
    "geohash_precision": 6,
    "workers": 4
  }
}
```

### graph-index-temporal (Optional)

Provides time-based indexing for temporal queries.

**Tier:** Any tier (optional)

```json
{
  "type": "processor",
  "name": "graph-index-temporal",
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
      ],
      "outputs": [
        {"name": "temporal_index", "subject": "TEMPORAL_INDEX", "type": "kv"}
      ]
    },
    "time_resolution": "hour",
    "workers": 4
  }
}
```

### graph-gateway

HTTP gateway for GraphQL and MCP access.

**Tier:** All tiers (required)

```json
{
  "type": "gateway",
  "name": "graph-gateway",
  "config": {
    "ports": {
      "inputs": [
        {"name": "http", "subject": ":8084", "type": "http"}
      ],
      "outputs": [
        {"name": "mutations", "subject": "graph.mutation.*", "type": "nats-request"}
      ]
    },
    "graphql_path": "/graphql",
    "mcp_path": "/mcp",
    "bind_address": ":8084",
    "enable_playground": true
  }
}
```

## KV Buckets

| Bucket | Written By | Watched By | Tier |
|--------|------------|------------|------|
| `ENTITY_STATES` | graph-ingest | graph-index, graph-embedding, graph-clustering | All |
| `OUTGOING_INDEX` | graph-index | graph-anomalies | All |
| `INCOMING_INDEX` | graph-index | graph-anomalies | All |
| `ALIAS_INDEX` | graph-index | - | All |
| `PREDICATE_INDEX` | graph-index | - | All |
| `EMBEDDINGS_CACHE` | graph-embedding | graph-clustering | 1+ |
| `COMMUNITY_INDEX` | graph-clustering | - | 1+ |
| `STRUCTURAL_INDEX` | graph-anomalies | - | 1+ |
| `SPATIAL_INDEX` | graph-index-spatial | - | Optional |
| `TEMPORAL_INDEX` | graph-index-temporal | - | Optional |

## Observability Stack

### Directory Structure

```
configs/
├── semantic.json               # Tier 2: Semantic configuration
├── statistical.json            # Tier 1: Statistical configuration
├── structural.json             # Tier 0: Structural configuration
├── prometheus/
│   └── prometheus.yml          # Prometheus scraping configuration
├── grafana/
│   ├── provisioning/
│   │   ├── datasources/
│   │   │   └── prometheus.yml  # Auto-configure Prometheus datasource
│   │   └── dashboards/
│   │       └── default.yml     # Dashboard provider configuration
│   └── dashboards/
│       └── semstreams-overview.json  # Overview dashboard
└── README.md
```

### Usage

Start observability stack:
```bash
task services:start:observability
```

Access:
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin)

See `docs/OPTIONAL_SERVICES.md` for complete documentation.
