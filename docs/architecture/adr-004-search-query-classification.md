# ADR-004: Search Query Classification

## Status

Implemented (Phase 1)

> **Implementation**: Phase 1 (Keyword Heuristics) was implemented in `graph/gateway/graphql/`.
> The `KeywordClassifier` extracts temporal, spatial, similarity, and path intents from NL queries
> using regex patterns. See [classifier_keyword.go](../../graph/gateway/graphql/classifier_keyword.go)
> for implementation details.

## Context

The SemStreams gateway needs to route incoming queries to appropriate search strategies. Key challenges:

1. **Current State**: Rule-based strategy inference from structured `SearchOptions` works well for API clients that explicitly set parameters (GeoBounds, TimeRange, UseEmbeddings, etc.)

2. **Gap**: Natural language inputs from MCP chat or conversational interfaces don't map cleanly to structured options. Users ask "What sensors were active yesterday?" not "set TimeRange to last 24 hours and query type sensor".

3. **Testing Reality**: E2E test queries use keyword-aligned phrases ("path traversal", "community detection") that the system naturally handles. Real users phrase queries differently.

4. **Progressive Enhancement**: The system should work without external dependencies (LLM, embedding service) but leverage them when available.

## Decision

Adopt a **hybrid NL intent extraction system** with progressive fallback through three tiers:

### Tier 1: Keyword Heuristics (Always Available)

Pattern-match common phrases to extract structured options:

| Pattern | Intent | SearchOptions Mapping |
|---------|--------|----------------------|
| "yesterday", "last week", "since Monday" | Temporal | TimeRange |
| "near X", "in zone Y", "within 5km" | Spatial | GeoBounds |
| "like X", "similar to Y" | Similarity | UseEmbeddings: true |
| "connected to", "related to", "path from" | Relationship | PathRAG strategy |
| "how many", "count", "total" | Aggregation | (future: aggregation mode) |

Implementation: Regex patterns with named capture groups. Fast, deterministic, no external deps.

### Tier 2: Embedding Similarity (Requires Embedding Service)

Compare query embedding against training examples to classify intent:

1. **Training Examples**: Curated JSON file with example queries and their intent labels
2. **Vector Generation**: Generate embeddings at startup (not saved in repo)
3. **Lazy Warm**: Start in keyword-only mode, background-generate vectors, upgrade when ready
4. **Fallback**: If embedding service unavailable, operate in Tier 1 only

```json
{
  "examples": [
    {"query": "What happened in the warehouse yesterday?", "intent": "temporal", "options": {"time_reference": "relative"}},
    {"query": "Find devices near sensor-001", "intent": "spatial", "options": {"reference_entity": true}},
    {"query": "Show me similar equipment to pump-42", "intent": "similarity", "options": {}}
  ]
}
```

### Tier 3: LLM Classification (Complex/Ambiguous Queries)

For queries that don't match Tier 1 patterns and have low Tier 2 similarity scores:

1. Send query to LLM with structured prompt
2. LLM returns intent classification and extracted parameters
3. Timeout and error handling with fallback to best Tier 2 match

### Training Example Vector Strategy

**Decision**: Generate vectors at startup, not save in repository.

| Approach | Pros | Cons |
|----------|------|------|
| Save in repo | Zero startup cost | Tied to specific model, stale if model changes |
| Generate at startup | Model-agnostic, always fresh | Requires embedding service, startup latency |

**Rationale**:
- Training examples are small (dozens, not thousands) - generation is fast
- Progressive enhancement: if embedding service unavailable, graceful fallback to keywords
- Model upgrades don't require repository changes
- Embeddings warm lazily in background - immediate keyword availability

### Intent Categories

| Category | Indicators | SearchOptions Effect |
|----------|------------|---------------------|
| Temporal | Time references | Set TimeRange |
| Spatial | Location references | Set GeoBounds |
| Similarity | "like", "similar" | UseEmbeddings: true, StrategySemantic |
| Relationship | "connected", "path" | StrategyPathRAG |
| Aggregation | "count", "how many" | (future) |
| Entity Lookup | Entity ID patterns | StrategyExact |
| General Search | Default | StrategyGraphRAG |

## Consequences

### Positive

- **Handles Real NL Queries**: Users can ask natural questions without knowing API structure
- **Progressive Enhancement**: Works without external deps, improves with them
- **Predictable Fallback**: Keyword tier always available, deterministic behavior
- **Model Agnostic**: Training vectors regenerate on model change
- **Testable**: Each tier can be tested independently

### Negative

- **Latency Variability**: Tier 3 (LLM) adds 100-500ms for complex queries
- **Training Example Curation**: Requires ongoing maintenance of example corpus
- **Testing Complexity**: NL variations harder to enumerate than structured inputs
- **Startup Dependency**: Full capability requires embedding service availability

### Neutral

- **Configuration Surface**: New config options for tier thresholds and timeouts
- **Observability**: Need metrics per tier (hit rate, latency, fallback frequency)

## Implementation Plan

### Phase 1: Keyword Heuristics ✓ (Complete)
- ✓ Add `QueryClassifier` interface in `graph/gateway/graphql/classifier.go`
- ✓ Implement `KeywordClassifier` with regex patterns in `classifier_keyword.go`
- ✓ Wire into `Executor.resolveGlobalSearch()` via dependency injection
- ✓ Add `GlobalSearchWithOptions` to Resolver for classified queries
- ✓ Comprehensive unit tests in `classifier_keyword_test.go`
- ✓ Integration tests verifying strategy inference in `classifier_integration_test.go`

### Phase 2: Embedding Similarity (Planned)
- Add training examples JSON file
- Implement `EmbeddingClassifier` with lazy warm
- Background goroutine for vector generation

### Phase 3: LLM Classification
- Implement `LLMClassifier` with structured prompt
- Add timeout and error handling
- Wire fallback chain

## Key Files

| File | Purpose | Status |
|------|---------|--------|
| `graph/gateway/graphql/query_search.go` | SearchOptions.InferStrategy() | Existing |
| `graph/gateway/graphql/classifier.go` | QueryClassifier interface | ✓ Implemented |
| `graph/gateway/graphql/classifier_keyword.go` | Tier 1 regex implementation | ✓ Implemented |
| `graph/gateway/graphql/classifier_keyword_test.go` | Unit tests for KeywordClassifier | ✓ Implemented |
| `graph/gateway/graphql/classifier_integration_test.go` | Integration tests | ✓ Implemented |
| `graph/gateway/graphql/executor.go` | Wired classifier into resolvers | ✓ Modified |
| `graph/gateway/graphql/resolver.go` | GlobalSearchWithOptions method | ✓ Modified |
| `graph/gateway/graphql/classifier_embedding.go` | Tier 2 implementation | Planned |
| `graph/gateway/graphql/classifier_llm.go` | Tier 3 implementation | Planned |
| `configs/training_examples.json` | Example queries for Tier 2 | Planned |

## References

- [ADR-002: Query Capability Discovery](./adr-002-query-capability-discovery.md) - Query routing patterns
- [Query Access Concepts](../concepts/09-query-access.md) - Current query architecture
