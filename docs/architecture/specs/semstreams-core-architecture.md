# SemStreams Core Architecture

## Overview

This document defines SemStreams core components and their boundaries.
It reflects the **current implementation** as the stable API contract for alpha.

The distinction between core and optional matters for:

- **Understanding**: What's fundamental vs. what's built on top
- **Deployment**: Minimal vs. full-featured configurations
- **Development**: What changes require careful compatibility vs. what can evolve independently
- **Documentation**: Core concepts vs. optional capabilities

For optional components (training, multi-agent, federation), see [semstreams-optional-components.md](./semstreams-optional-components.md).

## Design Principle

> SemStreams core is agnostic about how capabilities are created and extended.

Core provides:

- Storage and indexing primitives
- Processing infrastructure
- Tiered operational capability (0/1/2)
- Query routing and execution (ClassifierChain, PathRAG, GraphRAG)

Core does not care:

- How flows are created (manual, AI-generated, imported)
- How models are trained (or whether training happens at all)
- How the UI presents capabilities
- Whether multi-agent orchestration is layered on top of query routing

## Core Components

Core components are required for tiered operations. Without them, SemStreams cannot function at the specified tier.

### Infrastructure (Always Required)

| Component | Package | Purpose |
|-----------|---------|---------|
| NATS Client | `natsclient/` | Pub/sub, KV, Object Store, Streams, circuit breaker |
| Flow Runtime | `service/`, `engine/`, `flowstore/` | Component lifecycle, validation, persistence |
| Gateway | `gateway/`, `gateway/http/`, `gateway/graph-gateway/` | HTTP/GraphQL API |
| Vocabulary | `vocabulary/` | Semantic predicates, IRI mappings, hierarchy |
| Component Framework | `component/`, `componentregistry/` | Registry, lifecycle, port definitions, discovery |
| Message Types | `message/` | Graphable interface, Triple, behavioral interfaces |
| Configuration | `config/` | Loading, validation, NATS KV watching |
| Health | `health/` | Health monitoring and status aggregation |
| Metrics | `metric/` | Prometheus metrics registry and collectors |

**Shared utilities** in `pkg/`:

- `pkg/cache/` - LRU, TTL, hybrid caching
- `pkg/buffer/` - Ring buffer, streaming
- `pkg/worker/` - Worker pools, concurrency
- `pkg/errs/` - Error wrapping
- `pkg/retry/` - Retry policies
- `pkg/logging/` - NATS handler, multi-handler

### Tier 0: Structural/Deterministic

| Component | Package | Purpose |
|-----------|---------|---------|
| Entity Storage | `graph/datamanager/` | Entity CRUD, batch operations, edge operations |
| Graph Types | `graph/` | EntityState, Triple, constants, query types |
| KV Buckets | `graph/kvbuckets/` | Bucket access patterns |
| Rule Engine | `processor/rule/` | CEL expressions, entity watching, state tracking |

**Tier 0 enables**: Deterministic operations, structural queries, rule-based automation.

### Tier 1: Statistical

| Component | Package | Purpose |
|-----------|---------|---------|
| BM25/Text Index | `processor/graph-index/` | Full-text search indexing |
| Spatial Index | `processor/graph-index-spatial/` | Geohash-based spatial queries |
| Temporal Index | `processor/graph-index-temporal/` | Time-window queries |
| Community Detection | `graph/clustering/` | LPA, PageRank, summarization |
| Structural Analysis | `graph/structural/` | k-core, centrality metrics |
| BM25 Embeddings | `graph/embedding/` (BM25 mode) | Statistical embeddings |

**Tier 1 enables**: Statistical queries, community detection, graph analytics, BM25-based similarity.

### Tier 2: Semantic

| Component | Package | Purpose |
|-----------|---------|---------|
| Neural Embeddings | `graph/embedding/` (HTTP mode) | Vector embeddings via semembed service |
| LLM Integration | `graph/llm/` | OpenAI client, prompts, content fetching |
| Inference | `graph/inference/` | Anomaly detection, hierarchy inference, semantic gaps |

**Tier 2 enables**: Semantic search, LLM-augmented queries, document understanding.

### Query

| Component | Package | Purpose |
|-----------|---------|---------|
| Query Client | `graph/query/` | Entity lookup, path queries, caching |
| Classifier Chain | `graph/query/classifier_chain.go` | Tiered classification (T0 keyword → T1/T2 embedding) |
| PathRAG | `processor/graph-query/pathrag.go` | Path-based retrieval |
| GraphRAG | `processor/graph-query/graphrag.go` | Graph-augmented retrieval |
| Query Gateway | `gateway/graph-gateway/` | GraphQL resolvers, HTTP handlers |

## Core Data Model

### Buckets (NATS KV)

All bucket constants defined in `graph/constants.go`:

**Tier 0 Buckets:**

| Bucket | Purpose |
|--------|---------|
| `ENTITY_STATES` | Primary entity storage with triples and versions |
| `PREDICATE_INDEX` | Predicate → entity IDs mapping |
| `INCOMING_INDEX` | Entity ID → referencing entities |
| `OUTGOING_INDEX` | Entity ID → referenced entities |
| `ALIAS_INDEX` | Alias → entity ID resolution |
| `SPATIAL_INDEX` | Geohash-based spatial indexing |
| `TEMPORAL_INDEX` | Time-based temporal indexing |
| `CONTEXT_INDEX` | Context values storage |
| `COMPONENT_STATUS` | Component lifecycle and status |

**Tier 1 Buckets:**

| Bucket | Purpose |
|--------|---------|
| `COMMUNITY_INDEX` | Community records with members and summaries |
| `STRUCTURAL_INDEX` | k-core levels and pivot distances |

**Tier 2 Buckets:**

| Bucket | Purpose |
|--------|---------|
| `EMBEDDING_INDEX` | Entity ID → embedding vector storage |
| `EMBEDDINGS_CACHE` | Embedding cache layer |
| `EMBEDDING_DEDUP` | Content-addressed deduplication |
| `ANOMALY_INDEX` | Structural anomaly detection results |

### Streams

System streams defined in `config/streams.go`:

| Stream | Storage | TTL | Purpose |
|--------|---------|-----|---------|
| `LOGS` | file | 1h | Application logs |
| `HEALTH` | memory | 5m | Component health updates |
| `METRICS` | memory | 5m | Prometheus metrics snapshots |
| `FLOWS` | memory | 5m | Flow status changes |

Additional streams are derived dynamically from component JetStream output ports.
Convention: subject `component.action.type` → stream `COMPONENT` (uppercase).

### Object Store

The `storage/objectstore/` component provides ObjectStore functionality for documents,
video frames, and blobs. Storage is infrastructure (Tier 0); content analysis requires Tier 2.

## Configuration

### Tier Selection

Tiers are selected by choosing the appropriate configuration file at startup:

| Config File | Tier | Description |
|-------------|------|-------------|
| `configs/structural.json` | 0 | Rules + graph traversal (no ML) |
| `configs/statistical.json` | 1 | + BM25 embeddings + community detection |
| `configs/semantic.json` | 2 | + Neural embeddings + LLM |

Config files include a `tier` field with string values: `"rules"`, `"statistical"`, or the full semantic config.

### Component Enable/Disable

Components are enabled/disabled individually in config:

```json
{
  "components": {
    "graph-embedding": {
      "type": "processor",
      "name": "graph-embedding",
      "enabled": true,
      "config": {
        "embedder_type": "bm25"
      }
    }
  }
}
```

### Services Configuration

```json
{
  "services": {
    "message-logger": { "enabled": true },
    "discovery": { "enabled": false },
    "heartbeat": { "enabled": true }
  }
}
```

### Environment Variables

Limited environment variable overrides supported with `STREAMKIT_` prefix:

- `STREAMKIT_PLATFORM_ID`, `STREAMKIT_PLATFORM_TYPE`, `STREAMKIT_PLATFORM_REGION`
- `STREAMKIT_NATS_URLS`, `STREAMKIT_NATS_USERNAME`, `STREAMKIT_NATS_PASSWORD`, `STREAMKIT_NATS_TOKEN`

## Tier Dependencies

```text
Tier 2 (Semantic)
├── Requires: Tier 1 + Tier 0 + Infrastructure
├── Adds: Neural embeddings, LLM integration, inference
└── Enables: Semantic search, LLM queries, document understanding
     │
     ▼
Tier 1 (Statistical)
├── Requires: Tier 0 + Infrastructure
├── Adds: BM25, communities, structural indices
└── Enables: Text search, clustering, graph analytics
     │
     ▼
Tier 0 (Structural)
├── Requires: Infrastructure
├── Adds: Entities, relationships, rules
└── Enables: Deterministic ops, structural queries, automation
     │
     ▼
Infrastructure
├── Requires: Nothing
├── Provides: NATS, flow runtime, gateway, vocabulary
└── Enables: Component execution, API access
```

## API Boundaries

### Core Interfaces (Stable)

These interfaces are stable and optional components depend on them:

| Interface | Package | Purpose |
|-----------|---------|---------|
| `query.Client` | `graph/query/` | Entity lookup, path queries |
| `embedding.Embedder` | `graph/embedding/` | Vector embedding |
| `llm.Client` | `graph/llm/` | LLM completion |
| `datamanager.DataManager` | `graph/datamanager/` | Entity CRUD |
| `component.Discoverable` | `component/` | Component metadata |

### Extension Points

Core provides these extension points for optional components:

1. **ClassifierChain** (`graph/query/classifier_chain.go`): Tiered query classification,
   extensible with additional classifiers
2. **Component Registry** (`componentregistry/`): Register new component types
3. **JetStream Streams**: Derived streams from component output ports
4. **KV Buckets**: Optional components can create their own buckets

## Development Guidelines

### Adding to Core

Core changes require:

- Consideration of all three tiers
- Backward compatibility (or clear migration path)
- Documentation updates
- Impact assessment on optional components

### Feature Flags vs. Build Tags

Prefer runtime feature flags over build tags:

- Easier deployment (single binary)
- Runtime reconfiguration
- Clearer debugging

Build tags only for:

- Significantly different dependencies (e.g., CGO vs. pure Go)
- Platform-specific code

## Summary

| Category | Packages | Required For |
|----------|----------|--------------|
| **Infrastructure** | `natsclient/`, `service/`, `engine/`, `flowstore/`, `gateway/`, `vocabulary/`, `component/`, `config/`, `health/`, `metric/` | Everything |
| **Tier 0** | `graph/datamanager/`, `graph/kvbuckets/`, `processor/rule/` | Deterministic ops |
| **Tier 1** | `processor/graph-index*/`, `graph/clustering/`, `graph/structural/`, `graph/embedding/` (BM25) | Statistical ops |
| **Tier 2** | `graph/embedding/` (HTTP), `graph/llm/`, `graph/inference/` | Semantic ops |
| **Query** | `graph/query/`, `processor/graph-query/`, `gateway/graph-gateway/` | Useful queries |
