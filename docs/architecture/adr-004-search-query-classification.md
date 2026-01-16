# ADR-004: Search Query Classification

## Status

Implemented (Phase 1)

> **Implementation**: Phase 1 (Keyword Heuristics) is implemented in `graph/query/`.
> The `KeywordClassifier` extracts temporal, spatial, similarity, and path intents from NL queries
> using regex patterns. See [classifier.go](../../graph/query/classifier.go) for implementation details.

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

Compare query embedding against domain-registered training examples to classify intent:

1. **Domain-Specific Examples**: Each domain registers query examples alongside its vocabulary
2. **Vector Generation**: Generate embeddings at runtime when seminstruct is available
3. **Lazy Warm**: Start in keyword-only mode, background-generate vectors, upgrade when ready
4. **Fallback**: If embedding service unavailable, operate in Tier 1 only

#### Domain Example Registration

Domains register query examples using the vocabulary system:

```go
// vocabulary/examples/logistics.go
func RegisterLogisticsVocabulary() {
    // Register predicates (existing pattern)
    vocabulary.Register("logistics.shipment.status", ...)

    // Register query examples for this domain
    vocabulary.RegisterQueryExamples("logistics",
        vocabulary.QueryExample{
            Query:  "Where is shipment ABC-123?",
            Intent: "entity_lookup",
            Options: map[string]any{"entity_pattern": "shipment"},
        },
        vocabulary.QueryExample{
            Query:  "Show delayed deliveries this week",
            Intent: "temporal_filter",
            Options: map[string]any{"status_filter": "delayed"},
        },
        vocabulary.QueryExample{
            Query:  "Find packages similar to order-456",
            Intent: "similarity",
            Options: map[string]any{"reference_entity": true},
        },
        vocabulary.QueryExample{
            Query:  "Which trucks are near warehouse-east?",
            Intent: "spatial_relationship",
            Options: map[string]any{"relationship": "near"},
        },
    )
}
```

#### Example Structure

```go
// vocabulary/query_examples.go
type QueryExample struct {
    Query   string         // Natural language query
    Intent  string         // Intent category (temporal, spatial, similarity, etc.)
    Options map[string]any // SearchOptions hints

    // Runtime fields (not persisted)
    Vector  []float32      // Generated when embedding service available
}
```

#### Runtime Vector Generation

When seminstruct becomes available:

1. Collect all registered examples across domains
2. Batch-generate embeddings via `graph.embedding.generate`
3. Store vectors in memory (not persisted - regenerated on restart)
4. Enable Tier 2 classification

```
Startup (no embedding service):
    KeywordClassifier only (Tier 1)

Background (embedding service detected):
    1. Enumerate registered QueryExamples
    2. Generate vectors in batches
    3. Build similarity index
    4. Upgrade to EmbeddingClassifier (Tier 2)
```

### Tier 3: LLM Classification (Complex/Ambiguous Queries)

For queries that don't match Tier 1 patterns and have low Tier 2 similarity scores:

1. Send query to LLM with structured prompt
2. LLM returns intent classification and extracted parameters
3. Timeout and error handling with fallback to best Tier 2 match

### Training Example Vector Strategy

**Decision**: Generate vectors at runtime, not save in repository.

| Approach | Pros | Cons |
|----------|------|------|
| Save in repo | Zero startup cost | Tied to specific model, stale if model changes |
| Generate at runtime | Model-agnostic, always fresh, domain-extensible | Requires embedding service, startup latency |

**Rationale**:
- Training examples are small (dozens per domain, ~100 total) - generation is fast
- Progressive enhancement: if embedding service unavailable, graceful fallback to keywords
- Model upgrades don't require repository changes
- Embeddings warm lazily in background - immediate keyword availability
- Domain vocabularies can be loaded dynamically (plugins, config-driven)

### Domain Example Best Practices

When authoring query examples for a domain:

1. **Cover intent variety**: Include temporal, spatial, similarity, relationship queries
2. **Use domain terminology**: "shipment", "delivery", "route" for logistics; "sensor", "reading", "calibration" for IoT
3. **Vary phrasing**: "Where is X?", "Show me X", "Find X", "What happened to X?"
4. **Include entity patterns**: Use realistic entity ID formats (e.g., "shipment-ABC123", "sensor-001")
5. **Keep examples concise**: 5-20 examples per domain is typically sufficient
6. **Test similarity**: Ensure distinct intents have dissimilar example vectors

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
- ✓ Add `QueryClassifier` interface in `graph/query/classifier.go`
- ✓ Implement `KeywordClassifier` with regex patterns for temporal, spatial, path intents
- ✓ Wire into gateway resolvers via dependency injection
- ✓ Comprehensive unit tests in `graph/query/classifier_test.go`

### Phase 2: Embedding Similarity ✓ (Complete)
- ✓ Add `QueryExample` type and JSON loader in `graph/query/examples.go`
- ✓ Domain examples as JSON config files in `configs/domains/`
- ✓ Implement `EmbeddingClassifier` in `graph/query/classifier_embedding.go`
- ✓ BM25 warm cache pattern (instant vectors, upgrade when neural available)
- ✓ Implement `ClassifierChain` for tiered routing (T0→T1/T2)
- ✓ Add domain examples: logistics, IoT, robotics

### Phase 3: LLM Classification (Future)
- Implement `LLMClassifier` with structured prompt
- Add timeout and error handling
- Wire fallback chain: Keywords → Embedding → LLM
- Use domain examples as few-shot context for LLM

## Key Files

| File | Purpose | Status |
|------|---------|--------|
| `graph/query/classifier.go` | QueryClassifier interface + KeywordClassifier | ✓ Implemented |
| `graph/query/classifier_test.go` | Unit tests for KeywordClassifier | ✓ Implemented |
| `graph/query/classifier_embedding.go` | Tier 1/2 embedding similarity | ✓ Implemented |
| `graph/query/classifier_chain.go` | Tiered classifier chain (T0→T1/T2) | ✓ Implemented |
| `graph/query/examples.go` | QueryExample type + JSON loader | ✓ Implemented |
| `graph/query/classifier_llm.go` | Tier 3 LLM classification | Future |
| `configs/domains/logistics.json` | Logistics domain examples | ✓ Implemented |
| `configs/domains/iot.json` | IoT domain examples | ✓ Implemented |
| `configs/domains/robotics.json` | Robotics domain examples | ✓ Implemented |

## References

- [ADR-002: Query Capability Discovery](./adr-002-query-capability-discovery.md) - Query routing patterns
- [Query Access Concepts](../concepts/09-query-access.md) - Current query architecture
