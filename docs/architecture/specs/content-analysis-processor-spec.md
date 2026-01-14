# Content Analysis Processor Specification

**Version**: 1.0.0
**Status**: Draft
**Last Updated**: 2025-01-14

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Data Model](#3-data-model)
4. [Analysis Pipeline](#4-analysis-pipeline)
5. [Prompt Templates](#5-prompt-templates)
6. [Review Workflow](#6-review-workflow)
7. [API Contract](#7-api-contract)
8. [Configuration](#8-configuration)
9. [Observability](#9-observability)
10. [Examples](#10-examples)

---

## 1. Overview

### 1.1 Purpose

The Content Analysis Processor automatically analyzes operational documents (SOPs, runbooks, procedures) to identify automation opportunities and suggest rules and workflows for user approval.

### 1.2 Scope

**In Scope:**
- Watch for new documents by configurable patterns
- Fetch document content from ObjectStore
- LLM-powered candidate detection
- LLM-powered definition extraction
- Suggestion storage and lifecycle management
- HTTP API for review workflow
- Integration with rules and workflow processors

**Out of Scope:**
- Document parsing (assumes text already extracted)
- OCR or image-based document processing
- Real-time document editing
- Multi-language support (English only for v1)

### 1.3 Tier Requirement

**Semantic tier required.** This processor depends on LLM integration and is intended for connected environments before field deployment.

### 1.4 Relationship to Other Components

```
┌─────────────────────────────────────────────────────────────────┐
│                     Content Analysis Flow                        │
│                                                                  │
│  Document Upload              Content Analysis Processor         │
│  ┌─────────────────┐         ┌─────────────────────────┐        │
│  │ File Input      │         │ Watch ENTITY_STATES     │        │
│  │ → graph-ingest  │────────►│ → Fetch from ObjectStore│        │
│  │ → ObjectStore   │         │ → LLM Analysis          │        │
│  └─────────────────┘         │ → Store Suggestions     │        │
│                              └───────────┬─────────────┘        │
│                                          │                       │
│                                          ▼                       │
│                              ┌─────────────────────────┐        │
│                              │ SUGGESTION_INDEX        │        │
│                              │ (pending suggestions)   │        │
│                              └───────────┬─────────────┘        │
│                                          │                       │
│                                          ▼                       │
│  User Review                 SuggestionReviewWorker              │
│  ┌─────────────────┐         ┌─────────────────────────┐        │
│  │ HTTP API        │◄───────►│ Process reviews         │        │
│  │ List/Get/Review │         │ Approve → Create        │        │
│  └─────────────────┘         │ Reject → Dismiss        │        │
│                              └───────────┬─────────────┘        │
│                                          │                       │
│                                          ▼                       │
│                              ┌─────────────────────────┐        │
│                              │ Rules/Workflow          │        │
│                              │ Processors              │        │
│                              └─────────────────────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Architecture

### 2.1 Package Structure

```
processor/content-analysis/
├── component.go           # Processor component lifecycle
├── config.go              # Configuration with schema tags
├── worker.go              # Async analysis worker (KV-watching)
├── detector.go            # Phase 1: Candidate detection
├── extractor.go           # Phase 2: Definition extraction
├── prompts.go             # Prompt templates (detection + extraction)
├── suggestion.go          # Suggestion data types + states
├── storage.go             # SUGGESTION_INDEX KV operations
├── review_worker.go       # SuggestionReviewWorker
├── http_handlers.go       # Review API endpoints
├── applier.go             # Creates rules/workflows from suggestions
└── register.go            # Component registration
```

### 2.2 Component Lifecycle

```go
type Component struct {
    config       Config
    nc           *nats.Conn
    js           jetstream.JetStream
    llm          llm.Client
    storage      SuggestionStorage
    objectStore  *objectstore.Store
    worker       *AnalysisWorker
    reviewWorker *SuggestionReviewWorker
    logger       *slog.Logger
    metrics      *Metrics
}

func (c *Component) Initialize(ctx context.Context) error {
    // 1. Validate configuration
    // 2. Create SUGGESTION_INDEX bucket
    // 3. Initialize LLM client
    // 4. Initialize ObjectStore client
    return nil
}

func (c *Component) Start(ctx context.Context) error {
    // 1. Start KV watcher for entity states
    // 2. Start analysis worker
    // 3. Start review worker (if enabled)
    // 4. Register HTTP handlers (if gateway configured)
    return nil
}

func (c *Component) Stop() error {
    // 1. Stop workers gracefully
    // 2. Close KV watchers
    // 3. Flush pending metrics
    return nil
}
```

### 2.3 Storage Layout

```
NATS KV Buckets:

SUGGESTION_INDEX (TTL: 30d)
├── suggestion:{id}                    → Suggestion JSON
│   {
│     "id": "sug-abc123",
│     "type": "rule",                  // or "workflow"
│     "status": "pending",
│     "source_entity_id": "acme.ops.content.document.sop.battery-check",
│     "source_location": "Section 3.2",
│     "candidate": { ... },            // Detection phase output
│     "definition": { ... },           // Extraction phase output (RuleDef or WorkflowDef)
│     "confidence": 0.85,
│     "created_at": "2025-01-14T10:00:00Z",
│     "updated_at": "2025-01-14T10:00:00Z"
│   }
│
├── suggestion:{id}:review             → Review decision
│   {
│     "decision": "approved",          // approved, rejected, edited
│     "reviewer": "user@example.com",
│     "notes": "Minor edits to conditions",
│     "edited_definition": { ... },    // If edited
│     "reviewed_at": "2025-01-14T11:00:00Z"
│   }
│
└── suggestion:source:{entity_id}      → Index by source document
    ["sug-abc123", "sug-def456"]       // List of suggestion IDs
```

---

## 3. Data Model

### 3.1 Suggestion

```go
type Suggestion struct {
    ID             string          `json:"id"`
    Type           SuggestionType  `json:"type"`            // "rule" or "workflow"
    Status         SuggestionStatus `json:"status"`

    // Source document
    SourceEntityID string          `json:"source_entity_id"`
    SourceLocation string          `json:"source_location"` // Section/paragraph ref

    // Detection phase output
    Candidate      Candidate       `json:"candidate"`

    // Extraction phase output
    Definition     json.RawMessage `json:"definition"`      // RuleDef or WorkflowDef JSON

    // Metadata
    Confidence     float64         `json:"confidence"`
    CreatedAt      time.Time       `json:"created_at"`
    UpdatedAt      time.Time       `json:"updated_at"`
}

type SuggestionType string

const (
    SuggestionTypeRule     SuggestionType = "rule"
    SuggestionTypeWorkflow SuggestionType = "workflow"
)

type SuggestionStatus string

const (
    StatusPending    SuggestionStatus = "pending"     // Awaiting review
    StatusApproved   SuggestionStatus = "approved"    // User approved
    StatusRejected   SuggestionStatus = "rejected"    // User rejected
    StatusApplied    SuggestionStatus = "applied"     // Rule/workflow created
    StatusFailed     SuggestionStatus = "failed"      // Creation failed
)
```

### 3.2 Candidate

```go
type Candidate struct {
    Type        SuggestionType `json:"type"`
    Title       string         `json:"title"`       // Brief title
    Description string         `json:"description"` // What it does
    Location    string         `json:"location"`    // Where in document
    Rationale   string         `json:"rationale"`   // Why it's a candidate
    Confidence  float64        `json:"confidence"`  // Detection confidence
}
```

### 3.3 Review Decision

```go
type ReviewDecision struct {
    Decision         ReviewStatus    `json:"decision"`
    Reviewer         string          `json:"reviewer"`
    Notes            string          `json:"notes,omitempty"`
    EditedDefinition json.RawMessage `json:"edited_definition,omitempty"`
    ReviewedAt       time.Time       `json:"reviewed_at"`
}

type ReviewStatus string

const (
    ReviewApproved ReviewStatus = "approved"
    ReviewRejected ReviewStatus = "rejected"
    ReviewEdited   ReviewStatus = "edited"   // Approved with modifications
)
```

---

## 4. Analysis Pipeline

### 4.1 Pipeline Overview

```
Document Entity in ENTITY_STATES
         │
         ▼
┌─────────────────────────┐
│ 1. Filter Check         │ Does entity match watch patterns?
│    - entity_type        │ Skip if no match
│    - category           │
│    - predicate/value    │
└───────────┬─────────────┘
            │ match
            ▼
┌─────────────────────────┐
│ 2. Content Fetch        │ Fetch from ObjectStore via StorageRef
│    - title              │ Extract text fields
│    - body               │
│    - abstract           │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│ 3. Phase 1: Detection   │ LLM identifies candidates
│    - Rule candidates    │ Conditional logic patterns
│    - Workflow candidates│ Multi-step procedures
└───────────┬─────────────┘
            │ candidates[]
            ▼
┌─────────────────────────┐
│ 4. Phase 2: Extraction  │ LLM extracts definitions
│    (per candidate)      │ Full RuleDef or WorkflowDef JSON
└───────────┬─────────────┘
            │ suggestions[]
            ▼
┌─────────────────────────┐
│ 5. Store Suggestions    │ Write to SUGGESTION_INDEX
│    - status=pending     │ Ready for review
│    - index by source    │
└─────────────────────────┘
```

### 4.2 Watch Pattern Matching

```go
type WatchPattern struct {
    EntityType string `json:"entity_type,omitempty"` // e.g., "document"
    Category   string `json:"category,omitempty"`    // e.g., "sop"
    Predicate  string `json:"predicate,omitempty"`   // e.g., "document.type"
    Value      string `json:"value,omitempty"`       // e.g., "standard-operating-procedure"
}

func (w *Worker) matchesPattern(entity *EntityState, patterns []WatchPattern) bool {
    for _, p := range patterns {
        if p.EntityType != "" && !matchEntityType(entity, p.EntityType) {
            continue
        }
        if p.Category != "" && !matchCategory(entity, p.Category) {
            continue
        }
        if p.Predicate != "" && !matchPredicate(entity, p.Predicate, p.Value) {
            continue
        }
        return true // All specified conditions match
    }
    return false
}
```

### 4.3 Content Fetching

```go
func (w *Worker) fetchContent(ctx context.Context, entity *EntityState) (*DocumentContent, error) {
    // Check for StorageRef
    if entity.StorageRef == nil {
        return nil, ErrNoStorageRef
    }

    // Fetch from ObjectStore
    stored, err := w.objectStore.Get(ctx, entity.StorageRef.Key)
    if err != nil {
        return nil, fmt.Errorf("fetch content: %w", err)
    }

    // Extract text by semantic role
    content := &DocumentContent{
        EntityID: entity.ID,
        Title:    stored.GetFieldByRole("title"),
        Body:     stored.GetFieldByRole("body"),
        Abstract: stored.GetFieldByRole("abstract"),
    }

    return content, nil
}
```

---

## 5. Prompt Templates

### 5.1 Phase 1: Detection Prompt

```go
const DetectionPromptTemplate = `You are analyzing an operational document to identify automation opportunities.

## Document
Title: {{.Title}}
Content:
{{.Body}}

## Task
Identify patterns in this document that could be automated:

1. **Rules** - Conditional logic that triggers actions
   - IF-THEN patterns
   - Threshold-based triggers
   - State transition triggers
   - Alert conditions

2. **Workflows** - Multi-step procedures
   - Sequential task lists
   - Approval chains
   - Escalation procedures
   - Checklists with dependencies

## Output Format
Return a JSON array of candidates:
{
  "candidates": [
    {
      "type": "rule",
      "title": "Brief title",
      "description": "What the rule does",
      "location": "Section/paragraph reference",
      "rationale": "Why this is a good automation candidate",
      "confidence": 0.85
    },
    {
      "type": "workflow",
      "title": "Brief title",
      "description": "What the workflow does",
      "location": "Section/paragraph reference",
      "rationale": "Why this is a good automation candidate",
      "confidence": 0.80
    }
  ]
}

Only include candidates with confidence >= {{.MinConfidence}}.
Focus on actionable, well-defined patterns.
`
```

### 5.2 Phase 2: Rule Extraction Prompt

```go
const RuleExtractionPromptTemplate = `You are extracting a rule definition from an operational document.

## Document Context
Title: {{.Title}}
Relevant Section:
{{.Body}}

## Candidate to Extract
Title: {{.Candidate.Title}}
Description: {{.Candidate.Description}}
Location: {{.Candidate.Location}}

## Task
Extract a complete rule definition in JSON format.

## Rule Schema
{
  "id": "unique-rule-id",
  "name": "Human-readable name",
  "description": "What this rule does",
  "enabled": true,
  "type": "threshold",
  "conditions": [
    {
      "field": "entity.field.path",
      "operator": "gt",  // eq, ne, gt, gte, lt, lte, contains, starts_with, ends_with
      "value": 100
    }
  ],
  "logic": "and",  // "and" or "or" for multiple conditions
  "entity": {
    "type_pattern": "*.*.*.*.sensor.*",
    "id_pattern": ""
  },
  "on_enter": [
    {
      "type": "add_triple",
      "predicate": "alert.status",
      "object": "warning"
    }
  ],
  "on_exit": [
    {
      "type": "remove_triple",
      "predicate": "alert.status"
    }
  ],
  "cooldown": "5m"
}

## Output
Return only the JSON rule definition, no explanation.
`
```

### 5.3 Phase 2: Workflow Extraction Prompt

```go
const WorkflowExtractionPromptTemplate = `You are extracting a workflow definition from an operational document.

## Document Context
Title: {{.Title}}
Relevant Section:
{{.Body}}

## Candidate to Extract
Title: {{.Candidate.Title}}
Description: {{.Candidate.Description}}
Location: {{.Candidate.Location}}

## Task
Extract a complete workflow definition in JSON format.

## Workflow Schema
{
  "id": "unique-workflow-id",
  "name": "Human-readable name",
  "description": "What this workflow does",
  "version": "1.0.0",
  "enabled": true,

  "trigger": {
    "rule": "rule-id",           // Triggered by rule
    "subject": "events.topic",   // Or by NATS subject
    "manual": true               // Or manual only
  },

  "steps": [
    {
      "name": "step-name",
      "description": "What this step does",
      "action": {
        "type": "call",          // call, publish, set_state, http, wait
        "subject": "service.action",
        "payload": {"key": "value"}
      },
      "timeout": "30s",
      "retry": {
        "max_attempts": 3,
        "initial_backoff": "5s"
      },
      "on_success": "next",      // "next", "complete", or step name
      "on_fail": "abort"         // "abort" or step name
    }
  ],

  "on_complete": [
    {"type": "publish", "subject": "events.workflow.completed"}
  ],

  "on_fail": [
    {"type": "publish", "subject": "events.workflow.failed"}
  ],

  "timeout": "1h"
}

## Output
Return only the JSON workflow definition, no explanation.
`
```

---

## 6. Review Workflow

### 6.1 Review Worker

```go
type SuggestionReviewWorker struct {
    storage     SuggestionStorage
    ruleApplier RuleApplier
    workflowApplier WorkflowApplier
    logger      *slog.Logger
}

func (w *SuggestionReviewWorker) ProcessReview(ctx context.Context, id string, decision ReviewDecision) error {
    suggestion, err := w.storage.Get(ctx, id)
    if err != nil {
        return err
    }

    // Store review decision
    if err := w.storage.StoreReview(ctx, id, decision); err != nil {
        return err
    }

    switch decision.Decision {
    case ReviewApproved:
        return w.applyApproved(ctx, suggestion, suggestion.Definition)
    case ReviewEdited:
        return w.applyApproved(ctx, suggestion, decision.EditedDefinition)
    case ReviewRejected:
        return w.storage.UpdateStatus(ctx, id, StatusRejected)
    }

    return nil
}

func (w *SuggestionReviewWorker) applyApproved(ctx context.Context, suggestion *Suggestion, definition json.RawMessage) error {
    var err error

    switch suggestion.Type {
    case SuggestionTypeRule:
        err = w.ruleApplier.Apply(ctx, definition)
    case SuggestionTypeWorkflow:
        err = w.workflowApplier.Apply(ctx, definition)
    }

    if err != nil {
        w.storage.UpdateStatus(ctx, suggestion.ID, StatusFailed)
        return err
    }

    return w.storage.UpdateStatus(ctx, suggestion.ID, StatusApplied)
}
```

### 6.2 Suggestion Lifecycle

```
                    ┌──────────────┐
                    │   pending    │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
              ▼            ▼            ▼
       ┌──────────┐ ┌──────────┐ ┌──────────┐
       │ approved │ │ rejected │ │  edited  │
       └────┬─────┘ └──────────┘ └────┬─────┘
            │                         │
            └───────────┬─────────────┘
                        │
              ┌─────────┴─────────┐
              │                   │
              ▼                   ▼
       ┌──────────┐        ┌──────────┐
       │ applied  │        │  failed  │
       └──────────┘        └──────────┘
```

---

## 7. API Contract

### 7.1 HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/suggestions` | List suggestions with filters |
| GET | `/suggestions/{id}` | Get suggestion details |
| POST | `/suggestions/{id}/review` | Submit review decision |
| GET | `/suggestions/source/{entity_id}` | List suggestions from document |
| GET | `/suggestions/stats` | Analysis and review statistics |
| POST | `/suggestions/{id}/extract` | Re-run extraction for candidate |

### 7.2 List Suggestions

**Request:**
```
GET /suggestions?status=pending&type=rule&limit=50
```

**Response:**
```json
{
  "suggestions": [
    {
      "id": "sug-abc123",
      "type": "rule",
      "status": "pending",
      "source_entity_id": "acme.ops.content.document.sop.battery-check",
      "candidate": {
        "title": "Low Battery Alert",
        "description": "Trigger alert when battery drops below 20%",
        "confidence": 0.85
      },
      "created_at": "2025-01-14T10:00:00Z"
    }
  ],
  "total": 1,
  "has_more": false
}
```

### 7.3 Get Suggestion Details

**Request:**
```
GET /suggestions/sug-abc123
```

**Response:**
```json
{
  "id": "sug-abc123",
  "type": "rule",
  "status": "pending",
  "source_entity_id": "acme.ops.content.document.sop.battery-check",
  "source_location": "Section 3.2 - Battery Monitoring",
  "candidate": {
    "type": "rule",
    "title": "Low Battery Alert",
    "description": "Trigger alert when battery drops below 20%",
    "location": "Section 3.2",
    "rationale": "Document specifies monitoring battery level and alerting operator",
    "confidence": 0.85
  },
  "definition": {
    "id": "low-battery-alert",
    "name": "Low Battery Alert",
    "description": "Trigger alert when battery drops below 20%",
    "enabled": true,
    "conditions": [
      {"field": "battery.level", "operator": "lt", "value": 20}
    ],
    "on_enter": [
      {"type": "add_triple", "predicate": "alert.status", "object": "battery_low"}
    ]
  },
  "confidence": 0.85,
  "created_at": "2025-01-14T10:00:00Z",
  "updated_at": "2025-01-14T10:00:00Z"
}
```

### 7.4 Submit Review

**Request:**
```
POST /suggestions/sug-abc123/review
Content-Type: application/json

{
  "decision": "edited",
  "reviewer": "user@example.com",
  "notes": "Adjusted threshold from 20% to 15%",
  "edited_definition": {
    "id": "low-battery-alert",
    "name": "Low Battery Alert",
    "conditions": [
      {"field": "battery.level", "operator": "lt", "value": 15}
    ],
    "on_enter": [
      {"type": "add_triple", "predicate": "alert.status", "object": "battery_low"}
    ]
  }
}
```

**Response:**
```json
{
  "id": "sug-abc123",
  "status": "applied",
  "applied_id": "low-battery-alert",
  "applied_at": "2025-01-14T11:00:00Z"
}
```

---

## 8. Configuration

### 8.1 Component Configuration

```go
type Config struct {
    // Storage
    SuggestionBucket string `json:"suggestion_bucket" schema:"type:string,desc:KV bucket for suggestions,default:SUGGESTION_INDEX,category:basic"`
    SuggestionTTL    string `json:"suggestion_ttl" schema:"type:string,desc:Suggestion retention period,default:720h,category:basic"` // 30 days

    // Watch Patterns
    WatchPatterns []WatchPattern `json:"watch_patterns" schema:"type:array,desc:Entity patterns to trigger analysis,category:basic"`

    // LLM
    LLM LLMConfig `json:"llm" schema:"type:object,desc:LLM configuration,category:basic"`

    // Analysis
    Analysis AnalysisConfig `json:"analysis" schema:"type:object,desc:Analysis settings,category:basic"`

    // Review
    Review ReviewConfig `json:"review" schema:"type:object,desc:Review workflow settings,category:advanced"`

    // HTTP
    HTTP HTTPConfig `json:"http" schema:"type:object,desc:HTTP API settings,category:advanced"`
}

type LLMConfig struct {
    Provider string `json:"provider" schema:"type:string,desc:LLM provider (openai/shimmy/ollama),default:openai"`
    Model    string `json:"model" schema:"type:string,desc:Model name,default:gpt-4"`
    Timeout  string `json:"timeout" schema:"type:string,desc:LLM request timeout,default:120s"`
    MaxRetries int  `json:"max_retries" schema:"type:int,desc:Max retry attempts,default:3"`
}

type AnalysisConfig struct {
    ExtractRules     bool    `json:"extract_rules" schema:"type:bool,desc:Extract rule candidates,default:true"`
    ExtractWorkflows bool    `json:"extract_workflows" schema:"type:bool,desc:Extract workflow candidates,default:true"`
    MinConfidence    float64 `json:"min_confidence" schema:"type:float,desc:Minimum detection confidence,default:0.7,min:0.0,max:1.0"`
    MaxCandidates    int     `json:"max_candidates" schema:"type:int,desc:Max candidates per document,default:20"`
    Workers          int     `json:"workers" schema:"type:int,desc:Concurrent analysis workers,default:2"`
}

type ReviewConfig struct {
    Enabled       bool   `json:"enabled" schema:"type:bool,desc:Enable review workflow,default:true"`
    AutoApply     bool   `json:"auto_apply" schema:"type:bool,desc:Auto-apply high confidence suggestions,default:false"`
    AutoThreshold float64 `json:"auto_threshold" schema:"type:float,desc:Confidence threshold for auto-apply,default:0.95"`
}

type HTTPConfig struct {
    Enabled bool   `json:"enabled" schema:"type:bool,desc:Enable HTTP API,default:true"`
    Prefix  string `json:"prefix" schema:"type:string,desc:API path prefix,default:/suggestions"`
}
```

### 8.2 Example Configuration

```yaml
content_analysis:
  suggestion_bucket: "SUGGESTION_INDEX"
  suggestion_ttl: "720h"

  watch_patterns:
    - entity_type: "document"
      category: "sop"
    - entity_type: "document"
      category: "operations"
    - predicate: "document.type"
      value: "standard-operating-procedure"

  llm:
    provider: "openai"
    model: "gpt-4"
    timeout: "120s"
    max_retries: 3

  analysis:
    extract_rules: true
    extract_workflows: true
    min_confidence: 0.7
    max_candidates: 20
    workers: 2

  review:
    enabled: true
    auto_apply: false
    auto_threshold: 0.95

  http:
    enabled: true
    prefix: "/suggestions"
```

---

## 9. Observability

### 9.1 Metrics

```go
var (
    // Analysis metrics
    documentsAnalyzed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "content_analysis_documents_total",
            Help: "Total documents analyzed",
        },
        []string{"status"}, // success, failed, skipped
    )

    candidatesDetected = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "content_analysis_candidates_total",
            Help: "Total candidates detected",
        },
        []string{"type"}, // rule, workflow
    )

    extractionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "content_analysis_extraction_duration_seconds",
            Help:    "Time to extract definition from candidate",
            Buckets: prometheus.ExponentialBuckets(1, 2, 8), // 1s to ~256s
        },
        []string{"type"},
    )

    suggestionsCreated = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "content_analysis_suggestions_total",
            Help: "Total suggestions created",
        },
        []string{"type", "status"},
    )

    // Review metrics
    reviewsProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "content_analysis_reviews_total",
            Help: "Total reviews processed",
        },
        []string{"type", "decision"}, // approved, rejected, edited
    )

    suggestionsApplied = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "content_analysis_applied_total",
            Help: "Total suggestions successfully applied",
        },
        []string{"type"},
    )

    // Queue metrics
    suggestionsPending = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "content_analysis_suggestions_pending",
            Help: "Current pending suggestions",
        },
    )
)
```

### 9.2 Logging

```go
// Analysis events
logger.Info("document analysis started",
    "entity_id", entityID,
    "content_length", len(content.Body))

logger.Info("candidates detected",
    "entity_id", entityID,
    "rule_count", ruleCount,
    "workflow_count", workflowCount)

logger.Info("suggestion created",
    "suggestion_id", suggestion.ID,
    "type", suggestion.Type,
    "confidence", suggestion.Confidence,
    "source_entity_id", suggestion.SourceEntityID)

// Review events
logger.Info("review submitted",
    "suggestion_id", id,
    "decision", decision.Decision,
    "reviewer", decision.Reviewer)

logger.Info("suggestion applied",
    "suggestion_id", id,
    "type", suggestion.Type,
    "applied_id", appliedID)
```

---

## 10. Examples

### 10.1 Example SOP Document

```json
{
  "id": "sop-battery-maintenance",
  "type": "document",
  "category": "sop",
  "title": "Drone Battery Maintenance Procedures",
  "body": "## 1. Pre-Flight Battery Check\n\nBefore each flight, verify:\n- Battery charge level is above 80%\n- Battery temperature is between 15°C and 35°C\n- No visible damage to battery casing\n\nIF battery level drops below 20% during flight, THEN initiate return-to-home procedure immediately.\n\n## 2. Post-Flight Battery Handling\n\n1. Allow battery to cool for 10 minutes\n2. Inspect for swelling or damage\n3. Log flight time in maintenance system\n4. If battery shows signs of wear, tag for replacement\n5. Store in climate-controlled cabinet\n\n## 3. Monthly Maintenance\n\nEvery 30 days:\n1. Run full discharge/charge cycle\n2. Check internal resistance\n3. Update battery health record\n4. Replace if health drops below 70%",
  "created_at": "2025-01-14T10:00:00Z"
}
```

### 10.2 Expected Detection Output

```json
{
  "candidates": [
    {
      "type": "rule",
      "title": "Low Battery Return-to-Home",
      "description": "Trigger RTH when battery drops below 20%",
      "location": "Section 1 - Pre-Flight Battery Check",
      "rationale": "Clear IF-THEN pattern with specific threshold",
      "confidence": 0.92
    },
    {
      "type": "workflow",
      "title": "Post-Flight Battery Handling",
      "description": "5-step procedure for battery handling after flight",
      "location": "Section 2 - Post-Flight Battery Handling",
      "rationale": "Sequential checklist with ordered steps",
      "confidence": 0.88
    },
    {
      "type": "workflow",
      "title": "Monthly Battery Maintenance",
      "description": "4-step monthly maintenance procedure",
      "location": "Section 3 - Monthly Maintenance",
      "rationale": "Scheduled maintenance workflow with health check",
      "confidence": 0.85
    },
    {
      "type": "rule",
      "title": "Battery Replacement Trigger",
      "description": "Flag for replacement when health below 70%",
      "location": "Section 3 - Monthly Maintenance",
      "rationale": "Threshold-based condition with action",
      "confidence": 0.78
    }
  ]
}
```

### 10.3 Expected Rule Extraction

For "Low Battery Return-to-Home" candidate:

```json
{
  "id": "low-battery-rth",
  "name": "Low Battery Return-to-Home",
  "description": "Trigger return-to-home when battery drops below 20% during flight",
  "enabled": true,
  "type": "threshold",
  "conditions": [
    {
      "field": "battery.level",
      "operator": "lt",
      "value": 20
    },
    {
      "field": "flight.status",
      "operator": "eq",
      "value": "in_flight"
    }
  ],
  "logic": "and",
  "entity": {
    "type_pattern": "*.*.*.*.drone.*"
  },
  "on_enter": [
    {
      "type": "publish",
      "subject": "drone.command.rth",
      "payload": {
        "reason": "low_battery",
        "battery_level": "$entity.battery.level"
      }
    },
    {
      "type": "add_triple",
      "predicate": "alert.status",
      "object": "battery_critical"
    }
  ],
  "on_exit": [
    {
      "type": "remove_triple",
      "predicate": "alert.status"
    }
  ],
  "cooldown": "1m"
}
```

### 10.4 Expected Workflow Extraction

For "Post-Flight Battery Handling" candidate:

```json
{
  "id": "post-flight-battery",
  "name": "Post-Flight Battery Handling",
  "description": "Standard procedure for battery handling after flight completion",
  "version": "1.0.0",
  "enabled": true,

  "trigger": {
    "subject": "drone.events.landed"
  },

  "steps": [
    {
      "name": "cool-down-wait",
      "description": "Allow battery to cool for 10 minutes",
      "action": {
        "type": "wait",
        "duration": "10m"
      }
    },
    {
      "name": "inspect-battery",
      "description": "Inspect for swelling or damage",
      "action": {
        "type": "call",
        "subject": "maintenance.battery.inspect",
        "payload": {
          "drone_id": "${trigger.payload.drone_id}",
          "battery_id": "${trigger.payload.battery_id}"
        }
      },
      "timeout": "5m",
      "on_fail": "flag-for-review"
    },
    {
      "name": "log-flight-time",
      "description": "Log flight time in maintenance system",
      "action": {
        "type": "call",
        "subject": "maintenance.flight.log",
        "payload": {
          "drone_id": "${trigger.payload.drone_id}",
          "flight_time": "${trigger.payload.flight_time}"
        }
      }
    },
    {
      "name": "check-wear",
      "description": "Check battery wear status",
      "action": {
        "type": "call",
        "subject": "maintenance.battery.check-wear",
        "payload": {
          "battery_id": "${trigger.payload.battery_id}"
        }
      },
      "on_success": "storage-instructions",
      "on_fail": "flag-for-replacement"
    },
    {
      "name": "flag-for-replacement",
      "description": "Tag battery for replacement",
      "action": {
        "type": "set_state",
        "entity_id": "${trigger.payload.battery_id}",
        "predicate": "battery.status",
        "object": "replacement_needed"
      },
      "on_success": "complete"
    },
    {
      "name": "flag-for-review",
      "description": "Flag battery for manual review",
      "action": {
        "type": "set_state",
        "entity_id": "${trigger.payload.battery_id}",
        "predicate": "battery.status",
        "object": "review_needed"
      },
      "on_success": "complete"
    },
    {
      "name": "storage-instructions",
      "description": "Notify operator of storage requirements",
      "action": {
        "type": "publish",
        "subject": "operator.notify",
        "payload": {
          "message": "Store battery in climate-controlled cabinet",
          "battery_id": "${trigger.payload.battery_id}"
        }
      }
    }
  ],

  "on_complete": [
    {
      "type": "publish",
      "subject": "maintenance.events.battery-handled",
      "payload": {
        "battery_id": "${trigger.payload.battery_id}",
        "status": "completed"
      }
    }
  ],

  "timeout": "30m"
}
```

---

## Appendix A: Integration with Rules Processor

When a rule suggestion is approved, the Content Analysis Processor creates the rule via the rules processor's runtime configuration API:

```go
func (a *RuleApplier) Apply(ctx context.Context, definition json.RawMessage) error {
    var ruleDef rule.Definition
    if err := json.Unmarshal(definition, &ruleDef); err != nil {
        return fmt.Errorf("unmarshal rule definition: %w", err)
    }

    // Validate definition
    if err := a.validator.Validate(ruleDef); err != nil {
        return fmt.Errorf("invalid rule definition: %w", err)
    }

    // Add to rules processor via NATS request
    req := &AddRuleRequest{Definition: ruleDef}
    resp, err := a.nc.Request(ctx, "rules.config.add", req, 10*time.Second)
    if err != nil {
        return fmt.Errorf("add rule: %w", err)
    }

    return nil
}
```

---

## Appendix B: Integration with Workflow Processor

When a workflow suggestion is approved, the Content Analysis Processor creates the workflow via the workflow processor's definition API:

```go
func (a *WorkflowApplier) Apply(ctx context.Context, definition json.RawMessage) error {
    var workflowDef workflow.WorkflowDef
    if err := json.Unmarshal(definition, &workflowDef); err != nil {
        return fmt.Errorf("unmarshal workflow definition: %w", err)
    }

    // Validate definition
    if err := a.validator.Validate(workflowDef); err != nil {
        return fmt.Errorf("invalid workflow definition: %w", err)
    }

    // Store in workflow definitions bucket
    key := fmt.Sprintf("workflow:%s", workflowDef.ID)
    data, _ := json.Marshal(workflowDef)

    _, err := a.definitionsBucket.Put(ctx, key, data)
    if err != nil {
        return fmt.Errorf("store workflow: %w", err)
    }

    return nil
}
```
