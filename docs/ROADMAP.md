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
**Status:** Implemented | **ADR:** [ADR-005](architecture/adr-005-anomaly-approval-workflow.md)

ReviewWorker and HTTP handlers wired into runtime:
- ReviewWorker started in graph-clustering when enabled
- `/inference/*` HTTP handlers registered in graph-gateway
- Suggestion generation added to core anomaly detectors
- `TargetEntity` support for approving anomalies with empty targets

Current state: Fully operational. Human-only mode works without LLM.

### Mutation E2E Testing
**Status:** Partial Coverage | **ADR:** [ADR-006](architecture/adr-006-mutation-e2e-testing.md)

Add explicit mutation tests beyond rule-driven coverage:
- Direct API tests for AddTriple/RemoveTriple
- Relationship mutation tests (Create/Delete)
- Index consistency verification after mutations

Current state: Mutations only tested indirectly via rules engine.

### Transitivity Detector Wiring
**Status:** Implemented | **ADR:** [ADR-008](architecture/adr-008-transitivity-detector.md)

Transitivity gap detector wired into anomaly detection pipeline:
- `kvRelationshipQuerier` implementation preserves predicate information
- Transitivity detector registered with anomaly orchestrator
- Detection of missing edges in transitive predicate chains enabled

Current state: Fully operational. Detects transitivity gaps for configured predicates.

### Query Pattern Enhancements
**Status:** Partial Implementation | **ADR:** [ADR-009](architecture/adr-009-pathrag-enhancements.md)

Complete PathRAG and GraphRAG with missing documented features:

**PathRAG:**
- Direction control (incoming, outgoing, both) - enables "what depends on X" queries
- Predicate filtering - focus traversal on specific relationship types
- Timeout and path limits - SLA enforcement and memory protection

**GraphRAG:**
- Relationships in response - show connections between returned entities
- Source attribution - link answers to specific entities/communities for explainability
- Response control parameters (include_summaries, include_relationships)

Current state: PathRAG only supports outgoing traversal. GraphRAG doesn't include relationships or source attribution.

### Rules Processor Completion
**Status:** Needs Implementation | **ADR:** [ADR-010](architecture/adr-010-rules-processor-completion.md)

Complete stubbed action implementations in rules processor:
- ActionTypePublish: Actual NATS publish (currently logging only)
- ActionTypeUpdateTriple: Triple metadata updates
- Dynamic watch pattern reloading without restart

Current state: Stateful ECA rules work, but publish/update actions are stubs that only log.

### Workflow Processor
**Status:** Needs Implementation | **ADR:** [ADR-011](architecture/adr-011-workflow-processor.md) | **Depends:** ADR-010

Durable multi-step workflow execution bridging reactive rules and orchestration:
- Declarative JSON workflow definitions
- Rule-triggered, scheduled, or manual execution
- Retry with backoff, step/workflow timeouts
- Timer service for wait actions and schedules

Current state: Detailed spec complete, implementation not started. Blocked by rules processor completion.

---

## Future Enhancements

Items planned but not required for alpha.

### Content Processing

#### LLM-Generated Abstracts
**Priority:** Medium | **Complexity:** Medium | **Pattern:** [ADR-013](architecture/adr-013-content-enrichment-pattern.md)

Auto-generate abstracts/summaries for content using LLM agents.

- **Use cases:** Documents without descriptions, long-form content needing summaries
- **Integration:** ContentStorable processing pipeline
- **Approach:** Send `RawContent()` fields to LLM, store generated abstract in content fields
- **Pattern:** KV-watching async worker (follows ADR-013)
- **Tier requirement:** Semantic (LLM required)

#### Content Analysis Processor
**Priority:** Medium | **Complexity:** High | **ADR:** [ADR-012](architecture/adr-012-content-analysis-processor.md) | **Pattern:** [ADR-013](architecture/adr-013-content-enrichment-pattern.md)

LLM-powered analysis of operational documents to suggest rules and workflows:
- Watch for new documents by configurable type/category patterns
- Two-phase analysis: detect candidates, then extract full definitions
- Extract conditional logic as rule suggestions
- Extract multi-step procedures as workflow suggestions
- User review/approval via HTTP API before deployment

- **Use cases:** Early adopters uploading SOPs before field deployment
- **Tier requirement:** Semantic (LLM required)
- **Pattern:** KV-watching async worker (follows ADR-013)
- **Depends on:** ADR-010 (rules completion), ADR-011 (workflow processor)

Current state: ADR and spec complete. Implementation blocked by rules and workflow processor completion.

---

### Community Detection

#### Content-Aware Keyword Extraction
**Priority:** Medium | **Complexity:** Medium | **ADR:** [ADR-007](architecture/adr-007-content-aware-keywords.md)

Enhance keyword extraction to use ContentStorable document content.

- **Current state:** Keywords from entity types and triple metadata only
- **Gap:** ContentStorable content (body, abstract, title) is used for embeddings but ignored for keywords
- **Proposed:** Hybrid extraction with weighted combination of metadata and content terms
- **Benefit:** Richer, more descriptive community keywords

---

### Embeddings & Retrieval

#### Multimodal Video Embeddings
**Priority:** Low | **Complexity:** High | **Pattern:** [ADR-013](architecture/adr-013-content-enrichment-pattern.md)

Generate embeddings from video content for semantic search.

- **Approach options:**
  1. Embed thumbnail only (simple, fast)
  2. Extract keyframes, send to vision LLM for descriptions, embed descriptions
  3. Use video-specific embedding models (expensive, specialized)
- **Integration:** Extends `BinaryStorable` pipeline via `ContentRoleMedia`
- **Pattern:** KV-watching async worker (follows ADR-013)
- **Tier requirement:** Semantic (vision LLM or multimodal model)

#### Image Embeddings
**Priority:** Medium | **Complexity:** Medium | **Pattern:** [ADR-013](architecture/adr-013-content-enrichment-pattern.md)

Generate embeddings directly from images.

- **Approach options:**
  1. Vision LLM generates description, embed the text
  2. Direct image-to-vector using multimodal models (CLIP, etc.)
- **Integration:** Extends `BinaryStorable` pipeline via `ContentRoleMedia`
- **Pattern:** KV-watching async worker (follows ADR-013)
- **Tier requirement:** Semantic (multimodal embedding provider)

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
