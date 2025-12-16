# Similarity Metrics

SemStreams uses similarity metrics for community detection, summary preservation, and keyword extraction. This guide explains each metric and how to tune it.

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

**Virtual edges in community detection:** Entities with cosine similarity ≥ the configured threshold get a virtual edge, allowing them to cluster even without explicit relationships. The default threshold is 0.6.

### Tuning `similarity_threshold`

| Symptom | Current | Try | Why |
|---------|---------|-----|-----|
| Communities too large | 0.6 | 0.75 | Fewer virtual edges |
| Communities too fragmented | 0.6 | 0.45 | More virtual edges |
| Unrelated entities clustering | 0.5 | 0.7 | Stricter similarity |
| Related entities not clustering | 0.7 | 0.55 | Looser similarity |

**Start with 0.6** (default). Adjust based on your domain:
- Technical domains (precise terms): Higher threshold (0.7-0.8)
- Natural language (fuzzy concepts): Lower threshold (0.5-0.6)

### Model Dependency

Thresholds depend on your embedding model:

| Model Type | Typical Good Threshold |
|------------|----------------------|
| OpenAI ada-002 | 0.75-0.85 |
| Sentence transformers | 0.6-0.75 |
| Nomic embed-text | 0.55-0.70 |

Different models have different similarity distributions. Test with your data.

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

**Community summary preservation:** When communities are re-detected, Jaccard similarity determines whether to preserve existing summaries. If old community members and new community members have Jaccard similarity above the threshold (default 0.8), the summary is kept. Otherwise, a new summary is generated.

### Tuning `summary_preservation_threshold`

| Value | Effect |
|-------|--------|
| 0.9+ | Strict: Regenerate summary on small membership changes |
| 0.8 | Balanced (default): Preserve unless significant change |
| 0.6 | Lenient: Keep summaries even with moderate churn |

**Higher threshold** = More accurate summaries, more LLM calls.
**Lower threshold** = Fewer LLM calls, potentially stale summaries.

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

**Statistical community summaries:** Keywords are TF-IDF extracted from entity text content (via `ContentStorable` interface). Each community summary includes extracted keywords and representative entities.

### Tuning TF-IDF

**Improve keyword quality:**

1. **Better content fields**: Include domain-specific terms in stored content. See [Embeddings](03-embeddings.md) for how ContentStorable works.

2. **Stopword filtering**: Common words are filtered automatically.

3. **Keyword count**: Configure max keywords per community (default: 10).

### When TF-IDF Fails

| Problem | Cause | Mitigation |
|---------|-------|------------|
| Generic keywords | Text content too similar across entities | Add distinguishing content |
| Missing important terms | Term appears in many communities | Consider domain-specific weighting |
| Noise keywords | Unique but meaningless terms | Improve content quality |

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

Configure the number of representatives per community (default: 3). More representatives provide richer context for LLM summaries.

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

## Quick Tuning Reference

| Want | Adjust | Direction |
|------|--------|-----------|
| Tighter communities | `similarity_threshold` | ↑ Increase |
| Looser communities | `similarity_threshold` | ↓ Decrease |
| Fresher summaries | `summary_preservation_threshold` | ↑ Increase |
| Fewer LLM calls | `summary_preservation_threshold` | ↓ Decrease |
| More keywords | `max_keywords` | ↑ Increase |
| Better keywords | Content fields | Improve stored text |
| More representatives | `max_representatives` | ↑ Increase |

## Debugging Similarity Issues

### Check Embedding Quality

Query individual entities via the API to verify embeddings are being generated. If embeddings are null, check your embedding service configuration and ensure the entity implements `ContentStorable` with text content.

### Inspect Community Formation

List communities via the API to see member counts and extracted keywords. This helps identify whether communities are forming as expected or if tuning is needed.

### Compare Entity Similarity

Use the similarity endpoint to check the computed similarity between specific entity pairs. This helps diagnose why entities are or aren't clustering together.

## Related

**Concepts**
- [Real-Time Inference](00-real-time-inference.md) - How metrics fit in the inference pipeline
- [Embeddings](03-embeddings.md) - Vector generation and ContentStorable interface
- [Community Detection](04-community-detection.md) - How LPA uses similarity for clustering

**Configuration**
- [Clustering Configuration](../advanced/01-clustering.md) - Full parameter reference for thresholds
