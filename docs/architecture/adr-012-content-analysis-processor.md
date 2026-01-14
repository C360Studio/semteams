# ADR-012: Content Analysis Processor

## Status

Proposed

## Context

Early adopters want to upload operational documents (SOPs, runbooks, procedures) and have the system automatically identify automation opportunities. Currently:

- Documents can be ingested via `graph-ingest` and stored in ObjectStore
- LLM integration exists for embeddings and community summarization
- Rules and workflows must be manually authored
- No automated discovery of automation candidates from unstructured content

The gap: **No path from operational documentation to suggested rules/workflows**.

Use case scenario:
1. Customer uploads fleet of SOP documents before field deployment
2. System analyzes each document for automation opportunities
3. Customer reviews suggestions and approves/edits/rejects
4. Approved suggestions become deployed rules and workflows

This feature requires **semantic tier** (LLM available) and is intended for connected environments before field deployment.

## Decision

Implement a Content Analysis Processor that follows the existing **KV-Watching Async Worker** pattern (like `embedding.Worker` and `EnhancementWorker`).

### Architecture

```
Document Arrives → ENTITY_STATES (via graph-ingest)
                        ↓
        Content Analysis Processor watches
        (filters by configurable entity type/category patterns)
                        ↓
        Fetch content via StorageRef from ObjectStore
                        ↓
        Phase 1: Detection (LLM)
          - Identify conditional logic → rule candidates
          - Identify multi-step procedures → workflow candidates
                        ↓
        Phase 2: Extraction (LLM per candidate)
          - Extract full RuleDef JSON
          - Extract full WorkflowDef JSON
                        ↓
        Store in SUGGESTION_INDEX (status=pending)
                        ↓
        SuggestionReviewWorker
          - HTTP API for user review
          - Approve → instantiate rule/workflow
          - Edit → modify then approve
          - Reject → dismiss
```

### Key Design Decisions

**1. Dedicated Suggestion System (not anomaly framework)**

Suggestions are stored in a separate `SUGGESTION_INDEX` bucket with a dedicated `SuggestionReviewWorker`, not mixed with the anomaly detection system.

Rationale:
- Clean domain separation (document analysis vs graph anomalies)
- Suggestion-specific features (edit before approve, bulk operations)
- Independent lifecycle and TTL configuration
- Clearer API surface for the document analysis use case

**2. Two-Phase Prompt Strategy**

Analysis uses two LLM phases rather than a single comprehensive prompt:

- **Phase 1 (Detection)**: Identify candidates with brief descriptions
- **Phase 2 (Extraction)**: Extract full definition JSON per candidate

Rationale:
- Better quality through focused prompts
- User can filter candidates before expensive extraction
- Easier debugging when suggestions are incorrect
- Cost trade-off acceptable at semantic tier

**3. Configurable Watch Patterns**

The processor watches `ENTITY_STATES` for documents matching configurable patterns:

```yaml
watch_patterns:
  - entity_type: "document"
    category: "sop"
  - predicate: "document.type"
    value: "standard-operating-procedure"
```

This allows customers to control which documents trigger analysis.

## Consequences

### Positive

- **Accelerates onboarding**: Customers get automation suggestions from existing docs
- **Follows existing patterns**: KV-watching worker, LLM client, ObjectStore integration
- **Human-in-the-loop**: All suggestions require approval before deployment
- **Separation of concerns**: Dedicated storage and review workflow
- **Extensible**: Pattern can be extended to other content types

### Negative

- **LLM dependency**: Requires semantic tier; not available in disconnected deployments
- **Cost**: Two-phase prompts mean 2+ LLM calls per document
- **Quality variance**: LLM extraction quality depends on document structure
- **New KV bucket**: Additional storage to manage and monitor

### Neutral

- **Depends on ADR-010/011**: Cannot create rules/workflows until those are implemented
- **Review API**: New HTTP endpoints for suggestion management
- **Prompt engineering**: Detection and extraction prompts need tuning

## Implementation Plan

### Phase 1: Core Infrastructure
- Create `processor/content-analysis/` package
- Implement component lifecycle (Initialize, Start, Stop)
- Set up `SUGGESTION_INDEX` KV bucket
- Implement `SuggestionStorage` interface

### Phase 2: Analysis Pipeline
- Implement `Detector` with Phase 1 prompt
- Implement `Extractor` with Phase 2 prompts
- Wire async worker to watch entity states
- Integrate with ObjectStore content fetching

### Phase 3: Review Workflow
- Implement `SuggestionReviewWorker`
- Create HTTP handlers for review API
- Wire approval to rule/workflow creation (depends on ADR-010/011)

### Phase 4: Testing & Observability
- Unit tests for detection and extraction
- Integration tests with mock LLM
- Prometheus metrics for analysis pipeline
- Structured logging for debugging

## Key Files

| File | Purpose |
|------|---------|
| `processor/content-analysis/component.go` | Component lifecycle |
| `processor/content-analysis/worker.go` | KV-watching async worker |
| `processor/content-analysis/detector.go` | Phase 1: Candidate detection |
| `processor/content-analysis/extractor.go` | Phase 2: Definition extraction |
| `processor/content-analysis/prompts.go` | LLM prompt templates |
| `processor/content-analysis/storage.go` | SUGGESTION_INDEX operations |
| `processor/content-analysis/review_worker.go` | Suggestion review workflow |
| `processor/content-analysis/http_handlers.go` | Review API endpoints |

## References

- **Pattern**: [ADR-013: Content Enrichment Worker Pattern](./adr-013-content-enrichment-pattern.md) - architectural foundation
- **Depends on**: [ADR-010: Rules Processor Completion](./adr-010-rules-processor-completion.md)
- **Depends on**: [ADR-011: Workflow Processor](./adr-011-workflow-processor.md)
- **Full specification**: [Content Analysis Processor Spec](./specs/content-analysis-processor-spec.md)
