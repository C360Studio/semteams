# SemStreams Core Architecture

## Overview

This document defines the boundary between SemStreams core components and optional feature components. The distinction matters for:

- **Understanding:** What's fundamental vs. what's built on top
- **Deployment:** Minimal vs. full-featured configurations
- **Development:** What changes require careful compatibility vs. what can evolve independently
- **Documentation:** Core concepts vs. optional capabilities

## Design Principle

> SemStreams core is agnostic about how capabilities are created and extended.

Core provides:
- Storage and indexing primitives
- Processing infrastructure
- Tiered operational capability (0/1/2)
- Query routing and execution (template matching, PathRAG, GraphRAG)

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
| NATS Client | `natsclient` | Pub/sub, KV, Object Store, Streams |
| Flow Runtime | `flow` | Component model, execution |
| Gateway | `gateway` | HTTP/GraphQL/MCP API |
| Vocab | `vocab` | Semantic vocabulary, entity types |
| Internal Utils | `metric`, `errors`, `worker`, `cache`, `buffer` | Shared infrastructure |

### Tier 0: Structural/Deterministic

| Component | Package | Purpose |
|-----------|---------|---------|
| Entity Store | `entity` | Graph state, ENTITY_STATES bucket |
| Relationship Index | `entity` | SPO triples, OUTGOING/INCOMING_INDEX |
| Alias Index | `entity` | Entity aliases, ALIAS_INDEX |
| Rule Engine | `rules` | Rule definitions, evaluation, triggers |
| Workflow Engine | `workflow` | Workflow definitions, execution |

**Tier 0 enables:** Deterministic operations, structural queries, rule-based automation.

### Tier 1: Statistical

| Component | Package | Purpose |
|-----------|---------|---------|
| BM25 Index | `index` | Text search |
| Community Index | `index` | LPA clustering, COMMUNITY_INDEX |
| Structural Index | `index` | k-core, centrality, PageRank |

**Tier 1 enables:** Statistical queries, community detection, graph analytics, BM25-based similarity.

### Tier 2: Semantic

| Component | Package | Purpose |
|-----------|---------|---------|
| SemEmbed | `embed` | Embedding models, vector similarity |
| SemInstruct | `instruct` | Lightweight LLM gateway |
| Content Analysis | `content` | LLM-based document processing |

**Tier 2 enables:** Semantic search, LLM-augmented queries, document understanding.

### Query (Required for Useful Operations)

| Component | Package | Purpose |
|-----------|---------|---------|
| Query Router | `query` | Template matching, classification |
| PathRAG | `query` | Path-based retrieval |
| GraphRAG | `query` | Graph-augmented retrieval |

## Core Data Model

### Buckets (NATS KV)

**Tier 0 Buckets:**

| Bucket | Owner | Purpose |
|--------|-------|---------|
| `ENTITY_STATES` | entity | Primary entity storage |
| `OUTGOING_INDEX` | entity | Outbound relationships |
| `INCOMING_INDEX` | entity | Inbound relationships |
| `ALIAS_INDEX` | entity | Entity aliases |
| `PREDICATE_INDEX` | entity | Relationship types |
| `RULE_DEFINITIONS` | rules | Rule specifications |
| `WORKFLOW_DEFINITIONS` | workflow | Workflow specifications |
| `FLOW_DEFINITIONS` | flow | Component configurations |
| `FLOW_STATES` | flow | Runtime state |
| `VOCAB_TYPES` | vocab | Entity type definitions |

**Tier 1 Buckets:**

| Bucket | Owner | Purpose |
|--------|-------|---------|
| `COMMUNITY_INDEX` | index | Detected communities |
| `STRUCTURAL_INDEX` | index | k-core, centrality metrics |

**Tier 2 Buckets:**

| Bucket | Owner | Purpose |
|--------|-------|---------|
| `EMBEDDING_INDEX` | embed | Vector embeddings |

### Object Store

| Store | Owner | Purpose |
|-------|-------|---------|
| `OBJECT_STORE` | flow | Documents, video frames, blobs (storage is infrastructure; analysis is Tier 2) |

### Streams

| Stream | Owner | Purpose |
|--------|-------|---------|
| `QUERY_LOG` | query | Query history with signals |
| `RULE_TRIGGERS` | rules | Rule trigger history |
| `ENTITY_EVENTS` | entity | Entity change events |

## Optional Components

Optional components enable features built on core capabilities. The system functions without them, but with reduced functionality.

### Training

**Enables:** Domain-specific model adaptation

| Component | Purpose |
|-----------|---------|
| training-export | Extracts QA pairs from core buckets |
| training-filter | Deduplication, quality, clustering |
| slm-trainer | QLoRA fine-tuning |
| model-deployer | Adapter distribution |

**Reads from core:**
- `ENTITY_STATES`, `*_INDEX` buckets
- `QUERY_LOG` stream
- `OBJECT_STORE`
- Embeddings via `embed` package (Tier 2, for semantic dedup/clustering)
- LLM via `instruct` package (Tier 2, for synthetic QA generation)

**Owns:**
- `TRAINING_DATA` bucket
- `TRAINING_STATE` bucket
- `MODEL_ADAPTERS` bucket
- `training.*` subjects

**Deployment options:**
- Same node as core (typical)
- Separate node with NATS access
- Shore-only (not deployed to edge)

**Tier considerations:**
- Tier 0/1: Deterministic training data only (entities, relationships, rules, query logs)
- Tier 2: Adds semantic deduplication, synthetic QA from documents

### Multi-Agent

**Enables:** Specialized agent routing and orchestration

| Component | Purpose |
|-----------|---------|
| agent-registry | Agent definitions, capabilities |
| agent-router | Query → agent routing |
| agent-orchestrator | Multi-agent coordination |

**Reads from core:**
- Query capabilities via `query` package
- Embeddings via `embed` package (Tier 2)
- Graph context via `entity` package

**Owns:**
- `AGENT_REGISTRY` bucket
- `ROUTING_LOG` stream
- `agent.*` subjects

### Federation

**Enables:** Multi-node synchronization

| Component | Purpose |
|-----------|---------|
| fed-sync | Entity/relationship replication |
| fed-auth | mTLS, Step CA integration |
| fed-gateway | Cross-node query routing |

**Reads from core:**
- All core buckets (for replication)

**Owns:**
- `FEDERATION_STATE` bucket
- `SYNC_LOG` stream
- `federation.*` subjects

### Domain Packs

**Enables:** Domain-specific operations

| Pack | Domain |
|------|--------|
| SemOps | Robotics, maritime, tactical |
| (future) | Other verticals |

**Contains:**
- Entity schemas for domain
- Pre-built flows
- Domain-specific rules
- Integration adapters (MAVLink, AIS, TAK, etc.)

## Tier Dependencies

```
┌─────────────────────────────────────────────────────────────────┐
│                     Tier Dependencies                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Tier 2 (Semantic)                                              │
│  ├── Requires: Tier 1 + Tier 0 + Infrastructure                │
│  ├── Adds: SemEmbed, SemInstruct, content analysis             │
│  └── Enables: Semantic search, LLM queries, doc understanding  │
│       │                                                         │
│       ▼                                                         │
│  Tier 1 (Statistical)                                           │
│  ├── Requires: Tier 0 + Infrastructure                         │
│  ├── Adds: BM25, communities, structural indices               │
│  └── Enables: Text search, clustering, graph analytics         │
│       │                                                         │
│       ▼                                                         │
│  Tier 0 (Structural)                                            │
│  ├── Requires: Infrastructure                                   │
│  ├── Adds: Entities, relationships, rules, workflows           │
│  └── Enables: Deterministic ops, structural queries, automation│
│       │                                                         │
│       ▼                                                         │
│  Infrastructure                                                  │
│  ├── Requires: Nothing                                          │
│  ├── Provides: NATS, flow runtime, gateway, vocab              │
│  └── Enables: Component execution, API access                   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Optional Component Dependencies

```
┌─────────────────────────────────────────────────────────────────┐
│               Optional Component Dependencies                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Training                                                       │
│  ├── Minimum: Core Tier 0 (deterministic data extraction)      │
│  ├── Better: Core Tier 1 (includes query logs with BM25)       │
│  ├── Best: Core Tier 2 (semantic dedup, synthetic QA)          │
│  └── Produces: Model adapters consumed by SemInstruct          │
│                                                                  │
│  Multi-Agent                                                    │
│  ├── Minimum: Core Tier 1 (BM25-based routing)                 │
│  ├── Best: Core Tier 2 (embedding-based routing)               │
│  └── Consumes: Training outputs (specialized adapters)         │
│                                                                  │
│  Federation                                                     │
│  ├── Requires: Core (any tier)                                  │
│  ├── Independent of: Training, Multi-Agent                     │
│  └── Replicates: Core bucket state                             │
│                                                                  │
│  Domain Packs                                                   │
│  ├── Requires: Core (any tier)                                  │
│  ├── Independent of: Other optional components                 │
│  └── Provides: Schemas, flows, rules, adapters                 │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Configuration

### Tier Selection

```yaml
semstreams:
  tier: 2  # 0, 1, or 2
```

Components for higher tiers are only loaded when tier is set appropriately.

### Optional Features

```yaml
semstreams:
  tier: 2
  
  features:
    training:
      enabled: true
      # Training-specific config...
      
    agents:
      enabled: false
      
    federation:
      enabled: false
```

### Domain Packs

```yaml
semstreams:
  tier: 2
  
  domains:
    - semops  # Loads SemOps domain pack
```

## API Boundaries

### Core API (Stable)

These APIs are stable and optional components depend on them:

| API | Purpose |
|-----|---------|
| `entity.Store` | Entity CRUD, relationship management |
| `index.Search` | BM25, community, structural queries |
| `embed.Embed` | Vector embedding (Tier 2) |
| `instruct.Complete` | LLM completion (Tier 2) |
| `query.Execute` | Query routing and execution |
| `rules.Evaluate` | Rule evaluation |
| `workflow.Execute` | Workflow execution |
| `flow.Runtime` | Component lifecycle |

### Optional APIs (May Change)

These APIs are internal to optional components:

| API | Owner |
|-----|-------|
| `training.Export` | Training component |
| `training.Train` | Training component |
| `agents.Route` | Multi-agent component |
| `agents.Registry` | Multi-agent component |
| `federation.Sync` | Federation component |

## Development Guidelines

### Adding to Core

Core changes require:
- Consideration of all three tiers
- Backward compatibility (or clear migration path)
- Documentation updates
- Impact assessment on optional components

### Adding Optional Components

Optional components should:
- Only depend on core APIs (not other optional components, unless explicitly layered)
- Own their own buckets/streams/subjects
- Be deployable independently
- Degrade gracefully when dependencies unavailable

### Feature Flags vs. Build Tags

Prefer runtime feature flags over build tags:
- Easier deployment (single binary)
- Runtime reconfiguration
- Clearer debugging

Build tags only for:
- Significantly different dependencies (e.g., CGO vs. pure Go)
- Platform-specific code

## Summary

| Category | Components | Required For |
|----------|------------|--------------|
| **Infrastructure** | natsclient, flow, gateway, vocab, utils | Everything |
| **Tier 0** | entity, rules, workflow | Deterministic ops |
| **Tier 1** | index (BM25, community, structural) | Statistical ops |
| **Tier 2** | semembed, seminstruct, content | Semantic ops |
| **Query** | query (router, pathrag, graphrag) | Useful queries |
| **Optional: Training** | training-* | Domain adaptation |
| **Optional: Agents** | agent-* | Specialized routing |
| **Optional: Federation** | fed-* | Multi-node sync |
| **Optional: Domains** | semops, etc. | Domain-specific ops |
