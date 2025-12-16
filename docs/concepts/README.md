# Concepts

Background knowledge for understanding SemStreams' algorithms and architecture.

## Start Here

**New to SemStreams?** Read this first:

- [Real-Time Inference](00-real-time-inference.md) - How SemStreams combines streaming data with continuous inference (rules, clustering, LLM)

## Where to Go Next

**Coming from Go/streaming?** You know NATS and event-driven systems. Start with:

- [Embeddings](03-embeddings.md) - What vectors are, why they enable semantic similarity
- [GraphRAG Pattern](05-graphrag-pattern.md) - Community-based retrieval for LLM context
- [Community Detection](04-community-detection.md) - How LPA clusters entities

**Coming from ML/LLM?** You know models and vectors. Start with:

- [Event-Driven Basics](01-event-driven-basics.md) - Pub/sub, streams, real-time processing
- [Knowledge Graphs](02-knowledge-graphs.md) - Triples, semantic vs property graphs

**Want to tune the system?** Read:

- [Similarity Metrics](07-similarity-metrics.md) - Cosine, Jaccard, TF-IDF with practical guidance
- [Community Detection](04-community-detection.md) - LPA parameters and failure modes

## Concept Map

```text
Events arrive continuously ◄── 00-real-time-inference.md
        │
        ├─────────────────────────────┐
        │                             │
        ▼                             ▼
┌───────────────┐            ┌────────────────┐
│ Rules Engine  │            │ Knowledge Graph │◄── 02-knowledge-graphs.md
│ (sync/event)  │            │  (Triples/SPO)  │
└───────────────┘            └────────┬───────┘
        │                             │
        │                        ┌────┴────┐
        │                        ▼         ▼
        │                    Explicit   Virtual     ◄── 03-embeddings.md
        │                     Edges      Edges
        │                   (triples) (similarity)
        │                        │         │
        │                        └────┬────┘
        │                             ▼
        │                    ┌─────────────────┐
        └───────────────────►│   Communities    │◄── 04-community-detection.md
                             │   (LPA clusters) │
                             └────────┬────────┘
                                      │
                                 ┌────┴────┐
                                 ▼         ▼
                              GraphRAG   PathRAG    ◄── 05/06-*rag-pattern.md
                             (semantic) (structural)
                                 │         │
                                 └────┬────┘
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
| [04-community-detection](04-community-detection.md) | LPA algorithm, tuning, hierarchical levels | Both |
| [05-graphrag-pattern](05-graphrag-pattern.md) | Community-based RAG, local/global search | Go devs |
| [06-pathrag-pattern](06-pathrag-pattern.md) | Bounded traversal, decay, resource limits | Both |
| [07-similarity-metrics](07-similarity-metrics.md) | Cosine, Jaccard, TF-IDF tuning | Go devs |

## Related Documentation

- [Basics](../basics/) - How to use SemStreams (implementation)
- [Advanced](../advanced/) - Deep configuration and optimization
- [Reference](../reference/) - Complete configuration options
