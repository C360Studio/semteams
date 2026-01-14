# ADR-013: Content Enrichment Worker Pattern

## Status

Accepted

## Context

Multiple features in SemStreams require asynchronous content enrichment that follows the same architectural pattern:

**Existing Implementations:**

| Worker | Location | Function |
|--------|----------|----------|
| Embedding Worker | `graph/embedding/worker.go` | Entity text → Vector embeddings |
| Enhancement Worker | `graph/clustering/enhancement_worker.go` | Community → LLM-enhanced summary |
| Review Worker | `graph/inference/review_worker.go` | Anomaly → Approval decision |

**Planned Enrichment Processors:**

| Processor | Function | Tier | ADR |
|-----------|----------|------|-----|
| Abstract Generator | Document body → Summary | Semantic | ROADMAP |
| Image Embedder | Image → Vector/Description | Semantic | ROADMAP |
| Video Analyzer | Video → Keyframes → Descriptions | Semantic | ROADMAP |
| Content Analyzer | SOP → Rules/Workflows | Semantic | [ADR-012](./adr-012-content-analysis-processor.md) |

All these features share the same **KV-Watching Async Worker** pattern but implementations are currently independent with duplicated code patterns.

## Decision

Establish the "Content Enrichment Worker" as a documented architectural pattern that:

1. Provides a template for new enrichment processor implementations
2. References existing implementations as canonical examples
3. Documents shared interfaces to leverage
4. Defines the dependency chain for planned features

### Pattern Components

```
┌─────────────────────────────────────────────────────────────────┐
│              Content Enrichment Worker Pattern                   │
│                                                                  │
│  1. TRIGGER (KV Watch)                                          │
│     - Watch bucket for status transitions (e.g., "pending")     │
│     - Filter by entity type, content role, or predicates        │
│     - Debounce/coalesce for batch efficiency                    │
│                                                                  │
│  2. FETCH (Content Access)                                      │
│     - ContentStorable → Text via StorageRef + ContentFields     │
│     - BinaryStorable → Binary via BinaryRefs                    │
│     - Semantic roles: body, abstract, title, media, thumbnail   │
│                                                                  │
│  3. PROCESS (Worker Pool)                                       │
│     - N concurrent workers (configurable, typically 2-5)        │
│     - Timeout per operation (context deadline)                  │
│     - Retry with exponential backoff                            │
│     - Deduplication by content hash                             │
│                                                                  │
│  4. STORE (Results)                                             │
│     - Update status: pending → generated/failed                 │
│     - Save enrichment to output bucket                          │
│     - Optional: callback notification for downstream            │
│                                                                  │
│  5. OBSERVE (Metrics + Logging)                                 │
│     - Processing latency histograms                             │
│     - Success/failure/dedup counters                            │
│     - Queue depth gauges                                        │
│     - Structured logging with entity context                    │
└─────────────────────────────────────────────────────────────────┘
```

### Key Interfaces

**Content Access** (`message/content_storable.go`):

```go
// ContentStorable separates metadata (triples) from large content (ObjectStore)
type ContentStorable interface {
    Storable
    ContentFields() map[string]string  // Semantic role → field name mapping
    RawContent() map[string]string     // Actual content by field name
}

// BinaryStorable extends for binary content (images, video)
type BinaryStorable interface {
    ContentStorable
    BinaryFields() map[string]BinaryContent
}

// Standard semantic roles (constants in message package)
const (
    ContentRoleBody      = "body"       // Primary text content
    ContentRoleAbstract  = "abstract"   // Brief summary
    ContentRoleTitle     = "title"      // Document title
    ContentRoleMedia     = "media"      // Primary binary content
    ContentRoleThumbnail = "thumbnail"  // Preview image
)
```

**Content Fetching** (`graph/llm/content_fetcher.go`):

```go
// ContentFetcher retrieves entity content for LLM prompts
type ContentFetcher interface {
    FetchEntityContent(ctx context.Context, entities []*EntityState) (map[string]*EntityContent, error)
}

type EntityContent struct {
    Title    string
    Abstract string
}
```

**Stored Content** (`storage/objectstore/content.go`):

```go
type StoredContent struct {
    EntityID      string
    Fields        map[string]string      // Text content by field name
    BinaryRefs    map[string]BinaryRef   // Binary content references
    ContentFields map[string]string      // Role → field mapping
    StoredAt      time.Time
}

// Access by semantic role (not hardcoded field names)
func (s *StoredContent) GetFieldByRole(role string) string
func (s *StoredContent) GetBinaryRefByRole(role string) *BinaryRef
func (s *StoredContent) HasRole(role string) bool
```

### Status State Machine

All enrichment workers use a status-based state machine:

```
┌─────────────┐
│   pending   │  Initial state, ready for processing
└──────┬──────┘
       │
       ├─────────────────┐
       │                 │
       ▼                 ▼
┌─────────────┐   ┌─────────────┐
│  generated  │   │   failed    │  Terminal states
│  (success)  │   │   (error)   │
└─────────────┘   └─────────────┘
```

Some workers have intermediate states (e.g., Enhancement Worker: `statistical` → `llm-enhanced`).

### Worker Lifecycle

```go
type EnrichmentWorker interface {
    Start(ctx context.Context) error  // Begin watching and processing
    Stop() error                      // Graceful shutdown
    Pause()                           // Temporarily halt processing
    Resume()                          // Resume after pause
}
```

### Deduplication Pattern

Used by Embedding Worker, applicable to other enrichment types:

```go
// Tier 1: Check content hash before processing
existingRecord, err := storage.GetByContentHash(ctx, contentHash)
if existingRecord != nil {
    // Reuse existing result, increment reference count
    return existingRecord.Result, nil
}

// Tier 2: Process and save
result, err := processor.Process(ctx, content)
if err == nil {
    storage.SaveWithDedup(ctx, entityID, contentHash, result)
}
```

**Storage pattern:**
- Primary bucket: Entity ID → Result mapping
- Dedup bucket: Content hash → Result (shared across entities)

### Metrics Interface

```go
type WorkerMetrics interface {
    IncProcessed(status string)      // success, failed
    IncDedupHits()                   // Content hash matches
    SetQueueDepth(count float64)    // Pending items
    ObserveLatency(duration float64) // Processing time
}
```

### Reference Implementations

| Pattern Element | Embedding Worker | Enhancement Worker | Review Worker |
|-----------------|------------------|-------------------|---------------|
| **Trigger** | Entity has StorageRef | Community status="statistical" | Anomaly status="pending" |
| **Fetch** | ObjectStore via StorageRef | Entity states for context | Anomaly record |
| **Process** | HTTP embedding API | LLM summarization | Decision logic + optional LLM |
| **Output** | EMBEDDING_INDEX | COMMUNITY_INDEX (updated) | ANOMALY_INDEX (updated) |
| **Dedup** | Content hash → vector | None (per-community) | None (per-anomaly) |
| **Pause/Resume** | No | Yes | Yes |
| **Callbacks** | GeneratedCallback | None | None |

### Dependency Chain

```
                    ADR-013 (Content Enrichment Pattern)
                                    │
            ┌───────────────────────┼───────────────────────┐
            │                       │                       │
            ▼                       ▼                       ▼
    LLM Abstracts            Image Embeddings        Video Analysis
    (ROADMAP)                (ROADMAP)               (ROADMAP)
            │                       │                       │
            └───────────────────────┼───────────────────────┘
                                    │
                                    ▼
                    ADR-012 (Content Analysis Processor)
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
            ADR-010 (Rules)                 ADR-011 (Workflow)
```

## Consequences

### Positive

- **Consistent Architecture**: All enrichment features follow the same pattern
- **Clear Template**: New implementations have a documented blueprint
- **Leverage Existing Code**: Reference implementations provide copy-paste starting points
- **Documented Dependencies**: Clear chain from pattern to specific processors
- **Shared Interfaces**: ContentStorable, BinaryStorable, ContentFetcher are reusable

### Negative

- **Pattern, Not Library**: This ADR documents the pattern but doesn't extract shared code
- **Implementation Duplication**: Each processor still duplicates some boilerplate
- **Pattern Evolution**: Changes to the pattern require updating multiple implementations

### Neutral

- **Future Extraction**: A shared `enrichment.Worker` base could be extracted if duplication becomes problematic
- **Tier Requirements**: Most planned processors require Semantic tier (LLM available)

## Implementation Guidance

### For New Enrichment Processors

1. **Copy structure** from `graph/embedding/worker.go` as starting template
2. **Implement processor interface** for your specific enrichment logic
3. **Use ContentStorable/BinaryStorable** for content access, not hardcoded field names
4. **Follow status state machine** (pending → generated/failed)
5. **Add metrics** using the WorkerMetrics interface pattern
6. **Implement graceful shutdown** with context cancellation
7. **Consider deduplication** if content may be duplicated across entities

### Configuration Template

```yaml
processor_name:
  enabled: true
  workers: 3                    # Concurrent workers
  bucket: "OUTPUT_BUCKET_NAME"  # Results storage
  timeout: "60s"                # Per-operation timeout
  retry:
    max_attempts: 3
    initial_backoff: "1s"
  dedup:
    enabled: true
    bucket: "DEDUP_BUCKET_NAME"
```

## Key Files

| File | Purpose |
|------|---------|
| `message/content_storable.go` | ContentStorable, BinaryStorable interfaces |
| `storage/objectstore/content.go` | StoredContent with role-based access |
| `graph/llm/content_fetcher.go` | ContentFetcher interface |
| `graph/embedding/worker.go` | Reference: Embedding Worker implementation |
| `graph/clustering/enhancement_worker.go` | Reference: Enhancement Worker implementation |
| `graph/inference/review_worker.go` | Reference: Review Worker implementation |

## References

- [ADR-012: Content Analysis Processor](./adr-012-content-analysis-processor.md) - First processor using this pattern
- [Workflow Processor Spec](./specs/workflow-processor-spec.md) - Workflow definitions created by content analysis
- [Content Analysis Processor Spec](./specs/content-analysis-processor-spec.md) - Detailed spec for SOP analysis
