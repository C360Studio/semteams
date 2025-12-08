# Tiered Inference Architecture

SemStreams provides a tiered inference architecture that allows progressive enhancement from simple rules-based processing to full ML-powered semantic analysis.

## Tier Overview

| Tier | Name | Inference | Retrieval | Services Required |
|------|------|-----------|-----------|-------------------|
| **0** | Rules | Stateful rules only | PathRAG (explicit edges), index queries | NATS + SemStreams |
| **1** | Native | + BM25 + LPA + statistical | + GraphRAG (statistical), semantic search (BM25) | NATS + SemStreams |
| **2** | LLM | + Neural + LLM summaries | + GraphRAG (LLM), hybrid search (neural) | + semembed + semshimmy + seminstruct |

**Key insight:** Queries depend on what inference has produced. No embeddings = no semantic search. No communities = no GraphRAG.

## Quick Start

```bash
# Run Tier 0 (rules-only, fastest)
task e2e:tier0

# Run Tier 1 (native inference, no external ML)
task e2e:tier1

# Run Tier 2 (full ML stack)
task e2e:tier2

# Run all tiers with comparison
task e2e:tiers
```

## Tier 0: Rules-Only (Deterministic)

Tier 0 demonstrates SemStreams' stateful rules creating dynamic graph relationships without any ML inference.

### Capabilities

- **Stateful Rules**: OnEnter/OnExit/WhileTrue state transitions
- **Graph Actions**: `add_triple`, `remove_triple`, `publish`
- **Index Queries**: Predicate, alias, temporal, spatial
- **PathRAG**: Traversal on explicit edges

### What's Disabled

- Embedding generation (no vectors)
- Clustering (no communities)
- Inferred triples (no `inferred.*` predicates)
- Semantic search
- GraphRAG

### Configuration

```json
{
  "tier": "rules",
  "clustering": { "enabled": false },
  "indexer": {
    "embedding": { "enabled": false }
  }
}
```

### When to Use

- Edge deployments with limited compute
- Regulatory environments requiring full auditability
- Low-latency alerting systems
- Deterministic state machine workflows

## Tier 1: Native Inference (Statistical)

Tier 1 adds statistical ML capabilities that run locally without external services.

### Capabilities

Everything in Tier 0, plus:
- **BM25 Embeddings**: Statistical vectors (384-D) for text similarity
- **LPA Clustering**: Label Propagation for community detection
- **Statistical Summaries**: Keywords, top terms per community
- **Semantic Search**: BM25 fallback mode
- **GraphRAG**: LocalSearch and GlobalSearch with statistical summaries

### What's Disabled

- Neural embeddings (no HTTP calls)
- LLM summaries (no seminstruct)

### Configuration

```json
{
  "tier": "native",
  "embedding": { "provider": "bm25" },
  "clustering": { "enabled": true }
}
```

### When to Use

- CI/CD pipelines (no external dependencies)
- Air-gapped environments
- Cost-sensitive deployments
- Edge devices with moderate compute

## Tier 2: LLM Inference (Semantic)

Tier 2 provides full semantic capabilities with external ML services.

### Capabilities

Everything in Tier 1, plus:
- **Neural Embeddings**: Dense vectors via semembed service
- **LLM Summaries**: Semantic community descriptions
- **Hybrid Search**: Combines neural + BM25 + filters
- **Enhanced GraphRAG**: LLM-quality summaries

### Configuration

```json
{
  "tier": "llm",
  "embedding": {
    "provider": "http",
    "http_endpoint": "http://semembed:8081/v1",
    "http_model": "BAAI/bge-small-en-v1.5"
  },
  "clustering": {
    "llm": {
      "base_url": "http://seminstruct:8083/v1",
      "model": "default"
    }
  }
}
```

**Note:** Both services use OpenAI-compatible APIs. The `base_url` must include `/v1` suffix because the client appends `/chat/completions` or `/embeddings` to the base URL.

**Services:**
- `semembed` - Neural embeddings (port 8081)
- `semshimmy` - LLM inference backend (port 8080, internal)
- `seminstruct` - OpenAI-compatible proxy to semshimmy (port 8083)

### When to Use

- Production deployments with full semantic capabilities
- Research and exploration workloads
- High-quality search requirements
- Knowledge graph enrichment pipelines

## Hotpath vs Async Processing

Understanding what runs synchronously (hotpath) vs asynchronously is critical for latency planning.

### Hotpath (Per-Message)

These run on every message and affect processing latency:

| Operation | Description | Tier |
|-----------|-------------|------|
| Rule evaluation | Expression rules check conditions | 0+ |
| Entity extraction | Parse payload to EntityState | 0+ |
| Triple creation | Extract relationships from payload | 0+ |
| Message routing | Route to downstream components | 0+ |
| KV storage | Write to ENTITY_STATES bucket | 0+ |

### Async (Background)

These run independently and don't block message processing:

| Operation | Description | Tier | Trigger |
|-----------|-------------|------|---------|
| Index maintenance | Update secondary indexes | 0+ | KV watcher |
| BM25 embedding | Generate statistical vectors | 1+ | Entity arrival |
| Neural embedding | Generate dense vectors | 2 | Entity arrival |
| Clustering (LPA) | Detect communities | 1+ | Periodic (2m default) |
| Community summarization | Statistical keywords | 1+ | After LPA |
| LLM summarization | Semantic summaries | 2 | After LPA |
| Triple inference | Infer relationships | 1+ | After LPA |

### Timeline Example (Tier 2)

```
T+0:      Entity arrives (hotpath: ~10ms)
T+0-5s:   Neural embedding via HTTP (async, ~100ms each)
T+10s:    First clustering cycle starts (initial_delay)
T+10-12s: LPA runs with semantic edges
T+12-20s: LLM summarization (async workers, ~1-2s each)
T+20s:    Enhanced communities available for GraphRAG
```

## Stateful Rules: Dynamic Graph Foundation

Stateful rules are the foundation for dynamic graph behavior in SemStreams. They enable the graph to reflect **current state** rather than just historical events.

### State Transitions

| Transition | When | Use Case |
|------------|------|----------|
| `OnEnter` | Condition transitions false → true | Create alert relationship |
| `OnExit` | Condition transitions true → false | Remove alert relationship |
| `WhileTrue` | Condition remains true | Update timestamps, counters |

### Graph-Modifying Actions

```json
{
  "id": "cold-storage-alert",
  "conditions": [
    {"field": "reading", "operator": "gte", "value": 40.0},
    {"field": "location", "operator": "contains", "value": "cold-storage"}
  ],
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.active", "object": "cold-storage-violation"},
    {"type": "publish", "subject": "alerts.cold-storage"}
  ],
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.active", "object": "cold-storage-violation"}
  ]
}
```

### Behavior

1. Temperature rises above 40F in cold-storage
2. OnEnter fires: `add_triple` creates relationship, `publish` sends alert
3. Temperature drops below 40F
4. OnExit fires: `remove_triple` removes relationship
5. Graph reflects **current** sensor state, not historical alerts

## Progressive Enhancement: Inference Enables Retrieval

The tiered architecture follows a key principle: **queries depend on what inference has produced**.

| Inference Capability | Retrieval Enabled |
|---------------------|-------------------|
| Explicit triples | PathRAG traversal |
| Inferred triples | Extended PathRAG |
| BM25 embeddings | Semantic search (fallback) |
| Neural embeddings | Semantic search (quality) |
| Communities | GraphRAG LocalSearch |
| Statistical summaries | GraphRAG GlobalSearch (basic) |
| LLM summaries | GraphRAG GlobalSearch (quality) |

### Example: GraphRAG Fallback

```go
// GraphRAG GlobalSearch automatically falls back:
summary := community.LLMSummary
if summary == "" {
    summary = community.StatisticalSummary  // Tier 1 fallback
}
```

## E2E Test Scenarios

### tier0-rules-iot

**Purpose:** Validate Tier 0 stateful rules with IoT sensor data.

**Test Flow:**
1. Send sensor data that crosses thresholds
2. Verify OnEnter fires (add_triple creates relationships)
3. Send data that returns to normal
4. Verify OnExit fires (remove_triple removes relationships)
5. Validate NO embeddings, NO clustering, NO inferred triples

**Assertions:**
- Rules evaluated > 0
- OnEnter fired for threshold violations
- OnExit fired when thresholds cleared
- Embeddings generated = 0
- Communities detected = 0

### tier1-native (kitchen-sink-core)

**Purpose:** Validate Tier 1 native inference without ML services.

**Test Flow:**
1. Send mixed data (documents, sensors, observations)
2. Wait for BM25 embeddings
3. Wait for clustering cycle
4. Verify semantic search works (BM25 fallback)
5. Verify GraphRAG uses statistical summaries

### tier2-llm (kitchen-sink-ml)

**Purpose:** Validate Tier 2 full ML capabilities.

**Test Flow:**
1. Verify semembed and seminstruct are healthy
2. Send mixed data
3. Verify neural embeddings generated
4. Compare LLM summaries vs statistical summaries
5. Verify search quality improvement

## Configuration Reference

### Unified Tier Shorthand

```json
{
  "tier": "native",  // Sets defaults for this tier

  // Optional overrides take precedence
  "embedding": { "provider": "http" }
}
```

### Tier Defaults

| `tier` | embedding.provider | clustering.enabled | inference.enabled |
|--------|-------------------|-------------------|------------------|
| `"rules"` | disabled | false | false |
| `"native"` | bm25 | true | true |
| `"llm"` | http | true | true |

## Files

| File | Description |
|------|-------------|
| `configs/tier0-rules-iot.json` | Tier 0 rules-only configuration |
| `configs/semantic-kitchen-sink.json` | Tier 1 configuration (BM25 embeddings) |
| `configs/semantic-kitchen-sink-ml.json` | Tier 2 configuration (HTTP neural embeddings) |
| `testdata/tier0/iot-sensors.jsonl` | IoT sensor test data |
| `test/e2e/scenarios/tier0_rules_iot.go` | Tier 0 test scenario |
| `test/e2e/scenarios/semantic_kitchen_sink.go` | Tier 1/2 test scenario |

## Docker Startup Notes

The ML container (`semstreams-ml`) includes a **30-second startup delay** via entrypoint override:
```yaml
entrypoint: ["sh", "-c"]
command: ["sleep 30 && /app/semstreams --config /app/configs/semantic-kitchen-sink-ml.json"]
```

This ensures the semembed service has fully loaded its model before the application attempts HTTP embedding requests.
