# Configuration: Progressive Enhancement

SemStreams capabilities are controlled entirely by JSON configuration. Enable features as needed—start simple, add capabilities progressively.

## Common Configurations

These are typical configuration presets. Mix and match features based on your needs.

| Config | Name | Capabilities | Requirements |
|--------|------|--------------|--------------|
| **Rules-Only** | Tier 0 | Stateful rules, explicit relationships | NATS only |
| **Native** | Tier 1 | + BM25 search, statistical communities | Same as above |
| **LLM** | Tier 2 | + Neural embeddings, LLM summaries | + semembed + LLM service |

> **Default**: Native configuration. The Graph processor enables BM25 embeddings by default. For Rules-Only (no search), explicitly disable embeddings in your config.

## Rules-Only Configuration

Deterministic processing with stateful rules. No search, no external services. **Requires explicit configuration** to disable default BM25 embeddings.

### Capabilities

- Stateful rules (OnEnter/OnExit/WhileTrue)
- Graph actions: `add_triple`, `remove_triple`, `publish`
- Index queries: predicate, alias, spatial, temporal
- PathRAG: Traverse explicit edges

### Not Available

- Embeddings (no vectors)
- Community detection
- Semantic search
- GraphRAG

### Configuration

```json
{
  "clustering": { "enabled": false },
  "indexer": {
    "embedding": { "enabled": false }
  }
}
```

### When to Use

- Edge deployments with limited compute
- Environments requiring full auditability
- Low-latency alerting
- Deterministic state machines

## Native Inference Configuration

Statistical capabilities that run locally. No external services required.

### Native Capabilities

Everything in Rules-Only, plus:

- BM25 embeddings (384-D vectors)
- LPA clustering (community detection)
- Statistical summaries (keywords, top terms)
- Semantic search (BM25 fallback)
- GraphRAG with statistical summaries

### Not Available

- Neural embeddings
- LLM summaries

### Configuration

```json
{
  "embedding": { "provider": "bm25" },
  "clustering": { "enabled": true }
}
```

### When to Use

- CI/CD pipelines (no external dependencies)
- Air-gapped environments
- Cost-sensitive deployments
- Keyword search sufficient for your domain

## LLM Inference Configuration

Full semantic capabilities with external ML services.

### Capabilities

Everything in Native, plus:

- Neural embeddings (dense vectors via semembed)
- LLM summaries (semantic community descriptions)
- Hybrid search (neural + BM25 + filters)
- GraphRAG with LLM-enhanced summaries

### Configuration

```json
{
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

### Services Required

| Service | Port | Purpose |
|---------|------|---------|
| semembed | 8081 | Neural embedding generation |
| semshimmy | 8080 | LLM inference backend |
| seminstruct | 8083 | OpenAI-compatible proxy |

### When to Use

- Production with full semantic capabilities
- Dense vector search required for your domain
- LLM-enhanced summaries for knowledge graph enrichment

## Processing: Hotpath vs Async

### Hotpath (Per-Message)

These affect processing latency:

| Operation | Tier |
|-----------|------|
| Rule evaluation | 0+ |
| Entity extraction | 0+ |
| Triple creation | 0+ |
| KV storage | 0+ |

### Async (Background)

These run independently:

| Operation | Tier | Trigger |
|-----------|------|---------|
| Index maintenance | 0+ | KV watcher |
| BM25 embedding | 1+ | Entity arrival |
| Neural embedding | 2 | Entity arrival |
| Clustering (LPA) | 1+ | Periodic |
| Statistical summaries | 1+ | After LPA |
| LLM summaries | 2 | After LPA |

### Timeline (Tier 2)

```text
T+0:      Entity arrives (hotpath ~10ms)
T+0-5s:   Neural embedding (async ~100ms each)
T+10s:    Clustering starts (initial_delay)
T+10-12s: LPA runs with semantic edges
T+12-20s: LLM summarization (async ~1-2s each)
T+20s:    Enhanced communities available
```

## Queries Depend on Inference

| Inference | Retrieval Enabled |
|-----------|-------------------|
| Explicit triples | PathRAG traversal |
| BM25 embeddings | Semantic search (keyword-based) |
| Neural embeddings | Semantic search (dense vectors) |
| Communities | GraphRAG LocalSearch |
| Statistical summaries | GraphRAG GlobalSearch (statistical) |
| LLM summaries | GraphRAG GlobalSearch (LLM-enhanced) |

No embeddings = no semantic search. No communities = no GraphRAG.

## Graceful Fallback

Higher configurations fall back automatically:

```go
// GraphRAG GlobalSearch
summary := community.LLMSummary
if summary == "" {
    summary = community.StatisticalSummary  // Native config fallback
}
```

## Choosing a Configuration

### Decision Flowchart

```text
Need deterministic only? → Rules-Only
Need semantic search? → Do you have ML infrastructure?
  Yes → LLM
  No  → Native
Need LLM summaries? → LLM
```

### Cost/Benefit

| Config | Compute | Dependencies | Search Type |
|--------|---------|--------------|-------------|
| Rules-Only | Low | None | Explicit edges only |
| Native | Medium | None | Keyword-based (BM25) |
| LLM | High | semembed, LLM | Semantic + LLM summaries |

## Next Steps

- [Rules](../rules/01-overview.md) - Stateful rules engine
- [Communities](../graph/04-communities.md) - How clustering works
- [Advanced: LLM Enhancement](../advanced/02-llm-enhancement.md) - LLM details
