# Concepts

Background knowledge for understanding SemStreams' algorithms and architecture.

## Start Here

**New to SemStreams?** Read this first:

- [Real-Time Inference](00-real-time-inference.md) - How SemStreams combines streaming data with continuous inference (rules, clustering, LLM)

## Where to Go Next

**Coming from Go/streaming?** You know NATS and event-driven systems. Start with:

- [Embeddings](03-embeddings.md) - What vectors are, why they enable semantic similarity
- [Similarity Metrics](04-similarity-metrics.md) - Cosine, Jaccard, TF-IDF with practical guidance
- [GraphRAG Pattern](07-graphrag-pattern.md) - Community-based retrieval for LLM context

**Coming from ML/LLM?** You know models and vectors. Start with:

- [Event-Driven Basics](01-event-driven-basics.md) - Pub/sub, streams, real-time processing
- [Knowledge Graphs](02-knowledge-graphs.md) - Triples, semantic vs property graphs

**Want to tune the system?** Read:

- [Similarity Metrics](04-similarity-metrics.md) - Cosine, Jaccard, TF-IDF with practical guidance
- [Community Detection](05-community-detection.md) - LPA parameters and failure modes

## Concept Map

```text
Events arrive continuously ◄── 00-real-time-inference.md
        │
        ▼
┌─────────────────────────────────────────────────────────┐
│                    TIER 0: STRUCTURAL                    │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐  │
│  │  Explicit   │  │   Rules     │  │   Structural    │  │◄── 06-structural-analysis.md
│  │  Triples    │  │   Engine    │  │   Inference     │  │
│  │  (SPO)      │  │  (patterns) │  │ (k-core, pivot) │  │
│  └──────┬──────┘  └──────┬──────┘  └────────┬────────┘  │
│         │                │                   │           │
│         └────────────────┼───────────────────┘           │
│                          ▼                               │
│                   Knowledge Graph ◄── 02-knowledge-graphs.md
└─────────────────────────────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
   TIER 1: BM25      TIER 2: Neural    (stays at Tier 0)
   Virtual Edges     Virtual Edges
        │                  │
        └────────┬─────────┘
                 ▼
        ┌─────────────────┐
        │   Communities   │◄── 05-community-detection.md
        │  (LPA clusters) │
        └────────┬────────┘
                 │
     ┌───────────┴───────────┐
     │                       │
     ▼                       ▼
  GraphRAG               PathRAG     ◄── Query Patterns
 (semantic)            (structural)       07/08
     │                       │
     └───────────┬───────────┘
                 ▼
           LLM-enhanced
             answers
```

## All Concepts

| Doc | Covers | Audience |
|-----|--------|----------|
| [00-real-time-inference](00-real-time-inference.md) | Hybrid model, inference modes, progressive tiers | Everyone |
| [01-event-driven-basics](01-event-driven-basics.md) | Pub/sub, NATS, streams, eventual consistency | ML/LLM devs |
| [02-knowledge-graphs](02-knowledge-graphs.md) | Triples, SPO model, semantic vs property graphs | Both |
| [03-embeddings](03-embeddings.md) | Vectors, similarity, virtual edges | Go devs |
| [04-similarity-metrics](04-similarity-metrics.md) | Cosine, Jaccard, TF-IDF tuning | Go devs |
| [05-community-detection](05-community-detection.md) | LPA algorithm, tuning, hierarchical levels | Both |
| [06-structural-analysis](06-structural-analysis.md) | Structural inference (Tier 0): k-core, pivot, anomaly detection | Both |
| [07-graphrag-pattern](07-graphrag-pattern.md) | Community-based RAG, local/global search | Go devs |
| [08-pathrag-pattern](08-pathrag-pattern.md) | Bounded traversal, decay, resource limits | Both |
| [09-query-access](09-query-access.md) | GraphQL HTTP, MCP gateway, query operations | Both |

## Related Documentation

- [Basics](../basics/) - How to use SemStreams (implementation)
- [Advanced](../advanced/) - Deep configuration and optimization
- [Reference](../reference/) - Complete configuration options
