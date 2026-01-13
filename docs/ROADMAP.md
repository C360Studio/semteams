# SemStreams Roadmap

## Alpha Blockers

Items requiring completion before alpha release.

### Search Query Classification
**Status:** Needs Implementation | **ADR:** [ADR-004](architecture/adr-004-search-query-classification.md)

Hybrid NL intent extraction with progressive fallback:
- Tier 1: Keyword heuristics (always available)
- Tier 2: Embedding similarity to training examples
- Tier 3: LLM classification for complex queries

Current state: Rule-based strategy inference from structured `SearchOptions`. Works for API clients but doesn't handle natural language inputs.

### Anomaly Approval Workflow
**Status:** Needs Wiring | **ADR:** [ADR-005](architecture/adr-005-anomaly-approval-workflow.md)

Wire existing ReviewWorker and HTTP handlers into runtime:
- Start ReviewWorker in graph-clustering when enabled
- Register `/inference/*` HTTP handlers in graph-gateway
- Add suggestion generation to core anomaly detectors

Current state: Detection works, review code exists but is disconnected from runtime.

### Mutation E2E Testing
**Status:** Partial Coverage | **ADR:** [ADR-006](architecture/adr-006-mutation-e2e-testing.md)

Add explicit mutation tests beyond rule-driven coverage:
- Direct API tests for AddTriple/RemoveTriple
- Relationship mutation tests (Create/Delete)
- Index consistency verification after mutations

Current state: Mutations only tested indirectly via rules engine.

---

## Future Enhancements

Items planned but not required for alpha.

### Content Processing

#### LLM-Generated Abstracts
**Priority:** Medium | **Complexity:** Medium

Auto-generate abstracts/summaries for content using LLM agents.

- **Use cases:** Documents without descriptions, long-form content needing summaries
- **Integration:** ContentStorable processing pipeline
- **Approach:** Send `RawContent()` fields to LLM, store generated abstract in content fields
- **Dependency:** LLM provider integration (already exists for embeddings)

---

### Embeddings & Retrieval

#### Multimodal Video Embeddings
**Priority:** Low | **Complexity:** High

Generate embeddings from video content for semantic search.

- **Approach options:**
  1. Embed thumbnail only (simple, fast)
  2. Extract keyframes, send to vision LLM for descriptions, embed descriptions
  3. Use video-specific embedding models (expensive, specialized)
- **Integration:** Extends `BinaryStorable` pipeline
- **Dependency:** Vision-capable LLM or multimodal embedding model

#### Image Embeddings
**Priority:** Medium | **Complexity:** Medium

Generate embeddings directly from images.

- **Approach options:**
  1. Vision LLM generates description, embed the text
  2. Direct image-to-vector using multimodal models (CLIP, etc.)
- **Integration:** Extends `BinaryStorable` pipeline
- **Dependency:** Multimodal embedding provider

---

## Legend

| Priority | Description |
|----------|-------------|
| High | Customer-requested or blocking other work |
| Medium | Significant value, plan for next iteration |
| Low | Nice to have, opportunistic |

| Complexity | Description |
|------------|-------------|
| Low | < 1 day, isolated change |
| Medium | 1-3 days, touches multiple components |
| High | > 3 days, architectural impact |
