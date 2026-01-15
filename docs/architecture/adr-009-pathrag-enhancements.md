# ADR-009: Query Pattern Enhancements

## Status

Implemented

## Context

SemStreams provides two complementary query patterns—PathRAG (structural traversal) and GraphRAG (community-based search). Both have documented features that aren't fully implemented.

### PathRAG Gaps

PathRAG provides bounded graph traversal. Current implementation supports:
- Breadth-first traversal with max_depth and max_nodes bounds
- Decay-based scoring (score = decay_factor ^ depth)
- Path reconstruction showing how entities connect

Missing features:

| Feature | Use Case | Current State |
|---------|----------|---------------|
| Predicate filtering | Focus on specific relationship types | Not implemented |
| Direction control | "What depends on this?" queries | Only outgoing edges |
| max_time timeout | SLA enforcement | Not implemented |
| max_paths bound | Memory protection for dense graphs | Not implemented |

### GraphRAG Gaps

GraphRAG provides community-based search. Current implementation supports:
- Local search (from entity, explore community)
- Global search (across communities by topic)
- Community summaries with keywords and representatives

Missing features:

| Feature | Use Case | Current State |
|---------|----------|---------------|
| `include_summaries` param | Control response verbosity | Not implemented |
| `include_relationships` param | Include entity connections | Not implemented |
| `relationships` response | Show connections between entities | Not in response |
| `sources` response | Attribution for RAG answers | Not in response |

## Decision

Implement enhancements in priority order across both query patterns.

---

## PathRAG Enhancements

### Priority 1: Direction Control

Add `direction` parameter to PathSearchRequest:

```go
type PathSearchRequest struct {
    StartEntity string   `json:"start_entity"`
    MaxDepth    int      `json:"max_depth"`
    MaxNodes    int      `json:"max_nodes"`
    DecayFactor float64  `json:"decay_factor"`
    Direction   string   `json:"direction"` // "outgoing", "incoming", "both"
}
```

Implementation:
- `outgoing`: Use OUTGOING_INDEX (current behavior)
- `incoming`: Use INCOMING_INDEX
- `both`: Merge results from both indexes

### Priority 2: Predicate Filtering

Add `predicates` parameter:

```go
Predicates []string `json:"predicates,omitempty"` // Empty = all predicates
```

Filter edges during traversal to only follow specified relationship types.

### Priority 3: Timeout and Path Limits

Add bounds for production deployments:

```go
Timeout  time.Duration `json:"timeout,omitempty"`   // 0 = no timeout
MaxPaths int           `json:"max_paths,omitempty"` // 0 = unlimited
```

---

## GraphRAG Enhancements

### Priority 1: Relationships in Response

Add entity relationships to search responses:

```go
type GlobalSearchResponse struct {
    Entities         []Entity           `json:"entities"`
    CommunitySummaries []CommunitySummary `json:"community_summaries"`
    Relationships    []Relationship     `json:"relationships"` // NEW
    Answer           string             `json:"answer,omitempty"`
}
```

Extract relationships between returned entities from OUTGOING_INDEX.

### Priority 2: Source Attribution

Add source tracking for RAG grounding:

```go
type GlobalSearchResponse struct {
    // ... existing fields
    Sources []Source `json:"sources,omitempty"` // NEW
}

type Source struct {
    EntityID    string `json:"entity_id"`
    CommunityID string `json:"community_id"`
    Relevance   float64 `json:"relevance"`
}
```

Link answer content to specific entities/communities for explainability.

### Priority 3: Response Control Parameters

Add optional parameters to control response verbosity:

```go
type GlobalSearchRequest struct {
    // ... existing fields
    IncludeSummaries     bool `json:"include_summaries"`     // default: true
    IncludeRelationships bool `json:"include_relationships"` // default: false
}
```

---

## Consequences

### Benefits

- Complete documented APIs for both query patterns
- Enable "what depends on X" queries (PathRAG direction)
- Provide RAG answer attribution (GraphRAG sources)
- Allow response size control (include_* params)
- Production-ready with SLA guarantees (timeouts)

### Costs

- Additional complexity in query handlers
- More response fields to document
- Relationship extraction adds latency to GraphRAG

### Migration

- All new parameters are optional with sensible defaults
- Existing queries continue to work unchanged
- No breaking changes to current API

## Implementation Notes

### Index Requirements

| Feature | Required Index |
|---------|----------------|
| PathRAG direction=incoming | INCOMING_INDEX |
| PathRAG predicate filter | PREDICATE_INDEX |
| GraphRAG relationships | OUTGOING_INDEX |

### Testing

- Unit tests for each new parameter
- Integration tests for relationship extraction
- E2E tests for source attribution accuracy

## References

- [PathRAG Pattern](../concepts/08-pathrag-pattern.md)
- [GraphRAG Pattern](../concepts/07-graphrag-pattern.md)
- `processor/graph-query/pathrag.go`
- `processor/graph-query/graphrag.go`
