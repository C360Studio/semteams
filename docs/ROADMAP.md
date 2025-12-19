# SemStreams Roadmap

Future enhancements planned but out of scope for MVP.

---

## Content Processing

### LLM-Generated Abstracts
**Priority:** Medium | **Complexity:** Medium

Auto-generate abstracts/summaries for content using LLM agents.

- **Use cases:** Documents without descriptions, long-form content needing summaries
- **Integration:** ContentStorable processing pipeline
- **Approach:** Send `RawContent()` fields to LLM, store generated abstract in content fields
- **Dependency:** LLM provider integration (already exists for embeddings)

---

## Embeddings & Retrieval

### Multimodal Video Embeddings
**Priority:** Low | **Complexity:** High

Generate embeddings from video content for semantic search.

- **Approach options:**
  1. Embed thumbnail only (simple, fast)
  2. Extract keyframes, send to vision LLM for descriptions, embed descriptions
  3. Use video-specific embedding models (expensive, specialized)
- **Integration:** Extends `BinaryStorable` pipeline
- **Dependency:** Vision-capable LLM or multimodal embedding model

### Image Embeddings
**Priority:** Medium | **Complexity:** Medium

Generate embeddings directly from images.

- **Approach options:**
  1. Vision LLM generates description, embed the text
  2. Direct image-to-vector using multimodal models (CLIP, etc.)
- **Integration:** Extends `BinaryStorable` pipeline
- **Dependency:** Multimodal embedding provider

---

## Query Processing

### Query Classification with Templates
**Priority:** Medium | **Complexity:** Medium

Classify incoming queries to route to specialized handlers.

- **Use cases:**
  - Intent detection (search vs. action vs. question)
  - Query routing to domain-specific retrieval strategies
  - Pre-filtering by entity type or predicate
- **Approach:** Template-based classification system
- **Integration:** Gateway/GraphQL query layer

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
