# ADR-007: Content-Aware Keyword Extraction

## Status

Proposed

## Context

Community keyword extraction currently uses only metadata:
- Entity type parts from ID (e.g., `robotics.drone` → `["robotics", "drone"]`)
- Triple predicates
- Triple string values

ContentStorable documents have rich text (title, body, abstract) stored in ObjectStore. The embedding pipeline correctly fetches this content to generate vectors, but keyword extraction ignores it entirely.

This creates an inconsistency:
- Entities cluster by content similarity (embeddings use body/abstract/title)
- Keywords reflect only metadata (types, predicates)

Example: A community of safety documents clusters together because their content is similar ("emergency procedures", "evacuation routes"), but keywords are generic ("document", "report", "manual") because they come from entity types.

## Decision

Implement hybrid keyword extraction:

1. **Metadata extraction (current behavior)**: Entity types, predicates, triple values
2. **Content extraction (new)**: If entity has StorageRef, fetch content and extract terms from body/abstract/title using ContentFields mapping
3. **Weighted combination**: Weight content terms higher (configurable) since they're semantically richer

```go
type KeywordConfig struct {
    MetadataWeight float64 // Default: 1.0
    ContentWeight  float64 // Default: 2.0 (content terms weighted higher)
    MaxContentFetch int    // Default: 20 (limit I/O per community)
}
```

## Implementation

1. Add optional `objectstore.Store` to `StatisticalSummarizer`
2. In `extractKeywords()`, check for `StorageRef` in entity state
3. Batch fetch content for entities with refs (respect `MaxContentFetch`)
4. Use `ContentFields` to extract body, abstract, title text
5. Extract terms from content, apply `ContentWeight` multiplier
6. Combine with metadata terms for final scoring

Graceful fallback: If ObjectStore unavailable or entity has no StorageRef, use metadata-only (current behavior).

## Consequences

### Benefits

- Richer keywords from actual content
- Consistency with embedding pipeline (both use ContentStorable)
- Better community descriptions for document-heavy graphs

### Costs

- Optional ObjectStore dependency in summarizer
- Additional I/O during summarization (mitigated by MaxContentFetch)
- More complex configuration

### Neutral

- No change for deployments without ContentStorable entities
- Backward compatible (defaults to current metadata-only behavior)

## Alternatives Considered

1. **Metadata-only (current)**: Simpler but semantically limited. Chosen for alpha, this ADR proposes enhancement.

2. **Content-only**: Would miss metadata signals (entity types are useful) and would require ObjectStore for all deployments.

3. **Pre-computed keywords in triples**: Could add `meta.keywords` triple during ingestion. Shifts work to ingestion time, requires domain-specific logic, wouldn't adapt to changing stopwords.

4. **Use embedding vector directly**: Could reverse-engineer keywords from embedding. Expensive, model-dependent, less interpretable.

## References

- `graph/clustering/summarizer.go:91-162` - Current keyword extraction
- `graph/embedding/worker.go:319-384` - ContentStorable text extraction pattern
- `message/content_storable.go` - Interface definition
