# SemStreams Optional Components

## Overview

This document describes optional components that extend SemStreams core capabilities.
These components are **planned** and do not yet exist in the codebase.

For the current core architecture, see [semstreams-core-architecture.md](./semstreams-core-architecture.md).

## Status

| Component | Status | Detailed Spec |
|-----------|--------|---------------|
| Training Pipeline | Planned | [training-pipeline-spec.md](./training-pipeline-spec.md) |
| Multi-Agent | Planned | [multi-agent-spec.md](./multi-agent-spec.md) |
| Federation | Planned | (no detailed spec yet) |

## Core Dependencies

Optional components depend on core APIs. Before implementing optional components, these core extensions are needed:

| Extension | Priority | Used By | Status |
|-----------|----------|---------|--------|
| QUERY_LOG stream | High | Training, Multi-Agent | Not implemented |
| Component model selection | High | All Tier 2 components | Not implemented |
| Agent interface | Medium | Multi-Agent | Not implemented |
| Destination in ClassificationResult | Medium | Multi-Agent | Not implemented |

## Training Pipeline

**Purpose**: Generate training data for domain-specific SLM fine-tuning.

**Detailed spec**: [training-pipeline-spec.md](./training-pipeline-spec.md)

### Summary

The training processor transforms graph state, query logs, and documents into
instruction-response pairs suitable for QLoRA fine-tuning. Key features:

- **Tiered autonomy**: Works without LLM (Tier 0/1) or with LLM (Tier 2)
- **Incremental processing**: Only processes new data since last export
- **Graceful degradation**: Falls back to simpler methods when resources unavailable

### Core Dependencies

**Reads from core**:

- `ENTITY_STATES`, `*_INDEX` buckets (Tier 0)
- `QUERY_LOG` stream (**not yet implemented in core**)
- Object Store (Tier 0)
- Embeddings via `graph/embedding/` (Tier 2)
- LLM via `graph/llm/` (Tier 2)

**Owns** (new buckets/streams):

- `TRAINING_DATA` bucket
- `TRAINING_STATE` bucket
- `MODEL_ADAPTERS` bucket
- `training.*` subjects

### Deployment Options

- Same node as core (typical)
- Separate node with NATS access
- Shore-only (not deployed to edge)

## Multi-Agent

**Purpose**: Specialized agent routing and orchestration for queries.

**Detailed spec**: [multi-agent-spec.md](./multi-agent-spec.md)

### Summary

Multi-agent provides query routing to specialized agents (entity-agent, graph-agent, rules-agent, etc.). Key features:

- **Tiered routing**: Pattern matching (T0) → BM25 classifier (T1) → LLM routing (T2)
- **Registry-driven**: Agent capabilities declared, not hardcoded
- **Graceful degradation**: Falls back to general-agent when specialized unavailable

### Integration with Core

Multi-agent should integrate with the existing `ClassifierChain` in
`graph/query/classifier_chain.go`. The chain already supports:

- T0: Keyword pattern matching
- T1/T2: Embedding-based classification
- Extensibility (commented LLM classifier slot)

**Recommended approach**: Add `Destination` field to `ClassificationResult` to indicate
which agent should handle the query, rather than duplicating classification logic.

### Core Dependencies

**Reads from core**:

- Query capabilities via `graph/query/`
- Embeddings via `graph/embedding/` (Tier 2)
- Graph context via `graph/datamanager/`

**Needs from core** (not yet implemented):

- `Agent` interface definition
- `Destination` field in `ClassificationResult`
- `ROUTING_LOG` stream (for training feedback)

**Owns** (new buckets/streams):

- `AGENT_REGISTRY` bucket
- `agent.*` subjects

### Component Model Selection

A related concern is **component-level model selection**: allowing components to specify
which SLM/agent endpoint handles their LLM tasks.

This is different from query routing:

- Query routing: "User asks question → route to agent"
- Model selection: "graph-clustering component → uses summarization-SLM"

At the edge with limited compute, a stable of well-trained SLMs is better than one LLM.
This capability should be in **core** (not multi-agent) because:

- Affects how all Tier 2 components get LLM access
- Is infrastructure, not optional feature
- Needed before multi-agent query routing

## Federation

**Purpose**: Multi-node synchronization for distributed deployments.

**Status**: High-level design only, no detailed spec.

### Summary

Federation enables SemStreams instances to synchronize state across nodes (edge ↔ shore, multi-region, etc.).

### Components (Planned)

| Component | Purpose |
|-----------|---------|
| fed-sync | Entity/relationship replication |
| fed-auth | mTLS, Step CA integration |
| fed-gateway | Cross-node query routing |

### Core Dependencies

**Reads from core**:

- All core buckets (for replication)

**Owns** (new buckets/streams):

- `FEDERATION_STATE` bucket
- `SYNC_LOG` stream
- `federation.*` subjects

### Current State

Platform config already includes federation-related fields:

```json
{
  "platform": {
    "instance_id": "west-1",
    "environment": "prod"
  }
}
```

These support node identification but full federation is not implemented.

## Domain Packs

**Purpose**: Domain-specific schemas, flows, rules, and adapters.

**Status**: Partial - domain example files exist but formal pack system does not.

### Current State

Domain example files exist in `/configs/domains/`:

- `iot.json` - IoT domain examples
- `logistics.json` - Logistics domain examples
- `robotics.json` - Robotics domain examples

These provide query examples for the classifier but are not formal "packs" with:

- Entity schemas
- Pre-built flows
- Domain-specific rules
- Integration adapters

### Planned Packs

| Pack | Domain |
|------|--------|
| SemOps | Robotics, maritime, tactical |
| (future) | Other verticals |

## Optional Component Guidelines

### Design Principles

Optional components should:

1. **Only depend on core APIs** - Not on other optional components (unless explicitly layered)
2. **Own their own buckets/streams/subjects** - Clear namespace boundaries
3. **Be deployable independently** - Can run same node or separate
4. **Degrade gracefully** - Function (with reduced capability) when dependencies unavailable

### Integration Patterns

1. **Bucket ownership**: Each optional component owns its buckets with clear naming (`TRAINING_*`, `AGENT_*`, etc.)
2. **Subject namespacing**: Use component prefix (`training.*`, `agent.*`, `federation.*`)
3. **Core extension points**: Use ClassifierChain, Component Registry, derived streams
4. **Interface contracts**: Implement core interfaces where applicable

### Deployment Considerations

| Component | Same Node | Separate Node | Shore Only |
|-----------|-----------|---------------|------------|
| Training | Typical | Supported | Supported |
| Multi-Agent | Typical | Supported | No |
| Federation | No | Required | No |
| Domain Packs | Typical | Supported | Supported |
