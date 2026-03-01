# SemStreams Roadmap

## Alpha Blockers

Items requiring completion before alpha release.

### Search Query Classification
**Status:** Basic Implementation (Tier 0 Active) | **ADR:** [ADR-004](architecture/adr-004-search-query-classification.md)

Hybrid NL intent extraction with progressive fallback:
- Tier 0: Keyword heuristics — **active in production** (temporal, spatial, similarity, path, zone intents)
- Tier 1/2: Embedding similarity to domain examples — **built and tested, not wired up**
- Tier 3: LLM classification for complex queries — **not started**

Current state: `KeywordClassifier` runs in both `graph-gateway` and `graph-query` with 10+ regex patterns covering temporal ranges, spatial bounds, similarity, and path intent. `EmbeddingClassifier` and `ClassifierChain` are fully implemented with tests, and domain example JSON files exist in `configs/domains/` (logistics, IoT, robotics). However, the embedding tier is never instantiated at runtime — no config surface for domain example paths or thresholds.

**Improvements for roadmap:**
- Wire `EmbeddingClassifier` activation in gateway/graph-query startup (config fields + domain JSON loading)
- Expose `UpgradeVectors()` path for hot-swapping BM25 → neural vectors at runtime
- Add aggregation intent handling (`how many`, `count`, `total`)
- Add classifier observability metrics (tier hit rate, confidence distribution, fallback frequency)
- Expose classification to MCP handler (currently only GraphQL `globalSearch` and `semantic`)
- Align `graph-query` to use `ClassifierChain` instead of bare `KeywordClassifier`

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
**Status:** PathRAG Implemented, Gateway Exposure Needed | **ADR:** [ADR-009](architecture/adr-009-pathrag-enhancements.md)

**PathRAG — processor complete:**
- Direction control (incoming, outgoing, both) — **implemented and tested**
- Predicate filtering — **implemented and tested**
- Per-request timeout — **implemented and tested**
- MaxPaths bound — **implemented and tested**
- All features accessible via direct NATS `graph.query.pathSearch` subject

**PathRAG — gateway gap:**
- GraphQL schema only exposes `startEntity`, `maxDepth`, `maxNodes`
- `direction`, `predicates`, `timeout`, `maxPaths` not in GraphQL schema or `transformPathSearchVars()`
- `IncludeSiblings` field declared but not wired in BFS logic

**GraphRAG — not yet implemented:**
- Relationships in response — show connections between returned entities
- Source attribution — link answers to specific entities/communities for explainability
- Response control parameters (include_summaries, include_relationships)

Current state: PathRAG BFS engine is feature-complete with direction control, predicate filtering, timeout, and path limits. All features work via NATS but are not exposed through the GraphQL/MCP gateway. GraphRAG doesn't include relationships or source attribution.

### Rules Processor Completion
**Status:** Partial Implementation | **ADR:** [ADR-010](architecture/adr-010-rules-processor-completion.md)

Complete stubbed action implementations in rules processor:
- ActionTypePublish: Implemented for agentic workflows
- ActionTypePublishAgent: Implemented for spawning agent tasks
- ActionTypeUpdateTriple: Triple metadata updates (partial)
- Dynamic watch pattern reloading without restart

Current state: Stateful ECA rules work. Publish actions implemented for agentic system integration. Update triple actions partially implemented.

### Workflow Processor
**Status:** Implemented (Reactive Engine) | **ADRs:** [ADR-021](architecture/adr-021-reactive-workflow-engine.md),
[ADR-022](architecture/adr-022-workflow-engine-simplification.md)

Reactive workflow engine for stateless rules and stateful multi-step workflows:
- KV watch and stream/subject-based triggers
- Typed Go condition evaluation (no string interpolation)
- Cooldown and debounce for temporal deduplication
- Fire-and-forget publish actions
- Optional stateful workflow support via `pkg/workflow/` participant pattern

Current state: The original DAG-based workflow processor (`processor/workflow/`) was removed in favor of the reactive
engine (`processor/reactive/`). The new engine aligns with semstreams' reactive philosophy where the message topology
IS the execution graph. Components that need stateful workflows implement `WorkflowParticipant` and manage state via
KV buckets as a side effect. This reduced code complexity by 55% (~1550 lines) while maintaining all required
functionality. See [ADR-022](architecture/adr-022-workflow-engine-simplification.md) for migration details.

### Agentic Components
**Status:** Implemented | **ADR:** [ADR-018](architecture/adr-018-agentic-workflow-orchestration.md)

LLM-powered autonomous task execution with six specialized components:
- agentic-loop: State machine, orchestration, trajectory capture
- agentic-model: OpenAI-compatible LLM endpoint caller
- agentic-tools: Tool dispatch with executor registry
- agentic-dispatch: User message routing, commands, permissions
- agentic-memory: Graph-backed persistent memory
- agentic-governance: PII filtering, rate limiting, content governance

Current state: Fully operational. Run `task e2e:agentic` for validation.

### UI Flow Builder
**Status:** WIP | **Repo:** semstreams-ui

Visual flow builder for designing, deploying, and managing flows through a drag-and-drop interface. Backend APIs (Flow CRUD, component lifecycle, live metrics) are implemented in semstreams. The frontend UI is under active development in the `semstreams-ui` repository.

Current state: Backend ready. UI planned for beta release.

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
- **Depends on:** ADR-010 (rules completion), reactive workflow engine (ADR-021/ADR-022)

Current state: ADR and spec complete. Reactive workflow engine is implemented. Content analysis implementation
can proceed when prioritized.

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

### Query & Classification

#### Embedding Classifier Activation
**Priority:** High | **Complexity:** Low

Wire the existing `EmbeddingClassifier` and domain example JSON files into runtime startup:
- Add config fields for domain example paths and embedding threshold
- Load `configs/domains/*.json` at startup in gateway and graph-query
- Instantiate `ClassifierChain` with embedding tier instead of `nil`
- Wire `UpgradeVectors()` for hot-swapping BM25 → neural vectors when embedding service is available

Current state: All code exists and is tested. Needs config surface and startup wiring only.

#### PathRAG Gateway Exposure
**Priority:** High | **Complexity:** Low

Expose PathRAG ADR-009 features through GraphQL/MCP gateway:
- Add `direction`, `predicates`, `timeout`, `maxPaths` to GraphQL schema arguments
- Update `transformPathSearchVars()` to forward these fields
- Wire `IncludeSiblings` into BFS logic or remove the dead field

Current state: All features work via direct NATS. Gateway just needs schema + transform updates.

#### Classifier Observability
**Priority:** Medium | **Complexity:** Low

Add Prometheus metrics for classification behavior:
- Counter per tier (T0/T1/T2) hit rate
- Histogram for classification confidence
- Counter for fallback frequency (embedding miss → keyword)
- Counter for MCP vs GraphQL classification usage

#### Aggregation Intent Support
**Priority:** Medium | **Complexity:** Medium

Add aggregation query classification and handling:
- Add aggregation patterns (`how many`, `count`, `total`) to `KeywordClassifier`
- Add `StrategyAggregation` to search strategies
- Domain JSON files already include `aggregation` intent examples

---

### Graph Providers

#### Spatial/Temporal Graph Providers
**Priority:** Low | **Complexity:** Medium

Add `SpatialGraphProvider` and `TemporalGraphProvider` for clustering:
- Pattern proven by existing `SemanticGraphProvider`
- Indexes exist and are populated, just need provider implementations
- Would enable geo-proximity and time-correlated community detection

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
