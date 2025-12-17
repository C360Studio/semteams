# Similarity Metrics

SemStreams uses similarity metrics for community detection, summary preservation, and keyword extraction. This document explains each metric and how the system applies it.

## Cosine Similarity

Measures the angle between two embedding vectors.

### How It Works

```text
                    B
                   /
                  / θ
                 /
    ────────────A────────────

    cosine(θ) = (A · B) / (|A| × |B|)
```

Two vectors pointing in similar directions have high cosine similarity, regardless of magnitude.

### Value Interpretation

| Cosine Value | Meaning | Example |
|--------------|---------|---------|
| 0.95-1.0 | Near-identical | Same sentence, minor rephrasing |
| 0.80-0.95 | Highly similar | Same topic, different details |
| 0.60-0.80 | Related | Same domain, different subtopics |
| 0.40-0.60 | Weakly related | Tangential connection |
| 0.0-0.40 | Unrelated | Different domains |
| Negative | Opposite | Rare in practice |

### SemStreams Usage

**Virtual edges in community detection:** Entities with cosine similarity above a configured threshold receive a virtual edge, allowing them to cluster even without explicit relationships. This bridges semantically related entities that lack direct connections in the graph.

The threshold controls community granularity—higher values produce tighter, more focused communities while lower values allow broader groupings. The optimal threshold depends on your embedding model, as different models produce different similarity distributions.

## Jaccard Similarity

Measures set overlap—what proportion of elements are shared.

### How It Works

```text
Set A: {1, 2, 3, 4, 5}
Set B: {3, 4, 5, 6, 7}

Intersection: {3, 4, 5}     (3 elements)
Union: {1, 2, 3, 4, 5, 6, 7} (7 elements)

Jaccard = |Intersection| / |Union| = 3/7 ≈ 0.43
```

### Value Interpretation

| Jaccard Value | Meaning |
|---------------|---------|
| 1.0 | Identical sets |
| 0.8-1.0 | High overlap (few changes) |
| 0.5-0.8 | Moderate overlap |
| 0.2-0.5 | Low overlap (significant changes) |
| 0.0 | No shared elements |

### SemStreams Usage

**Community summary preservation:** When communities are re-detected, Jaccard similarity determines whether to preserve existing summaries. The system compares old and new community membership—if overlap is high, the existing summary remains valid. If membership has changed significantly, a new summary is generated.

This balances summary freshness against LLM cost. Higher thresholds regenerate summaries more often (fresher but more expensive), while lower thresholds preserve summaries longer (cheaper but potentially stale).

## TF-IDF (Term Frequency-Inverse Document Frequency)

Extracts keywords that characterize a community by finding terms common within the community but rare across all communities.

### How It Works

```text
TF (Term Frequency):
  How often a term appears in this community's entities.
  
IDF (Inverse Document Frequency):
  How rare the term is across all communities.
  Rare terms get higher weight.

TF-IDF = TF × IDF
```

### Example

```text
Community A (warehouse sensors):
  Terms: "temperature", "warehouse", "sensor", "zone-A", "humidity"

Community B (fleet drones):
  Terms: "drone", "battery", "fleet", "sensor", "altitude"

For Community A:
  "temperature" → High TF (appears often), High IDF (rare globally) → HIGH SCORE
  "sensor" → High TF, Low IDF (appears everywhere) → MEDIUM SCORE
  "the" → Low TF, Low IDF (common word) → LOW SCORE (filtered)
```

### SemStreams Usage

**Statistical community summaries:** Keywords are TF-IDF extracted from entity text content (via the `ContentStorable` interface). Each community summary includes these extracted keywords alongside representative entities.

The system automatically filters common stopwords. Keyword quality depends on the text content stored with entities—domain-specific terms in entity content produce more meaningful keywords. See [Embeddings](03-embeddings.md) for how `ContentStorable` works.

## PageRank (Representative Selection)

Identifies hub entities—those with many incoming connections.

### How It Works

PageRank scores entities by the importance of entities linking to them:

```text
Entity A ◄── B, C, D         (3 incoming)
Entity E ◄── A               (1 incoming, but from important A)
Entity F ◄── G               (1 incoming from unimportant G)

PageRank: A > E > F (approximately)
```

### SemStreams Usage

**Selecting representative entities for community summaries:** Representatives are high-PageRank entities within the community—the hub nodes with many connections. These appear in community summaries and provide anchors for understanding the community's structure.

The number of representatives per community is configurable. More representatives provide richer context for LLM-generated summaries but increase prompt size.

## Metric Interactions

These metrics work together:

```text
Entity Updates
      │
      ▼
┌─────────────────────────────────────────┐
│ Community Detection                      │
│  └─ Cosine similarity → virtual edges   │
└─────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────┐
│ Summary Generation                       │
│  ├─ TF-IDF → keywords                   │
│  ├─ PageRank → representatives          │
│  └─ Jaccard → preserve or regenerate    │
└─────────────────────────────────────────┘
      │
      ▼
GraphRAG uses summaries for context
```

## Related

**Concepts**
- [Real-Time Inference](00-real-time-inference.md) - How metrics fit in the inference pipeline
- [Embeddings](03-embeddings.md) - Vector generation and ContentStorable interface
- [Community Detection](05-community-detection.md) - How LPA uses similarity for clustering

**Configuration**
- [Clustering Configuration](../advanced/01-clustering.md) - Threshold tuning and parameter reference
