# ADR-021: Reactive Workflow Engine

## Status

Accepted

## Context

The current workflow-processor is the most imperative component in an otherwise declarative, reactive architecture. It has produced three distinct data corruption bugs sharing one root cause: typed Go structs dissolve into `map[string]interface{}` at NATS serialization boundaries, and string interpolation of `${step.output.field}` references cannot reconstruct them.

### Evidence from Previous Work

| Bug | Root Cause |
|-----|------------|
| `findings []TaskReviewFinding` → unmarshal failure | Stringified during interpolation |
| `tasks []workflow.Task` → unmarshal failure | Stringified during KV round-trip |
| `plan_content` structured JSON | Required `json.RawMessage` band-aid |

### Serialization Boundary Problem

Current model (9 serialization boundaries):

```
Component produces typed Result struct
  → json.Marshal to []byte
  → Wrap in BaseMessage for type metadata
  → Publish to NATS
  → Workflow executor receives, unwraps BaseMessage
  → Stores raw JSON in StepResult.Output (json.RawMessage)
  → Interpolates ${steps.X.output.Y} by walking raw JSON
  → Builds new JSON blob via string templates
  → Wraps in AsyncTaskPayload.Data
  → Wraps in BaseMessage
  → Publishes to NATS
  → Consumer unwraps BaseMessage → unwraps AsyncTaskPayload → json.Unmarshal into typed trigger
```

The root cause is architectural: **the workflow-processor treats data as opaque JSON blobs flowing through string templates, but producers and consumers are typed Go structs.**

### Existing Rules Engine Capabilities

The semstreams rules engine already implements:

- KV watch-based triggers — react to entity state changes
- Condition evaluation — field/operator/value checks before actions fire
- Cooldown and debounce — temporal deduplication
- on_enter / on_exit / while_true — state-transition lifecycle semantics
- Graph integration — entity creation from rule evaluations

A "workflow step" is fundamentally a rule that fires when the previous step's output entity appears in KV. A "workflow" is a coordinated set of rules whose trigger conditions reference shared KV state.

## Decision

Replace the imperative workflow-processor with a reactive workflow engine built on rules engine primitives.

### Core Design

#### 1. Dual Reactive Primitives

NATS provides two fundamentally different reactive primitives:

| Primitive | Pattern | Use Case |
|-----------|---------|----------|
| **KV watch** | "when state X becomes Y, do Z" | Inter-rule coordination within a workflow |
| **Subject consumer** | "when event X arrives, do Z" | Workflow entry points, external events, callbacks |
| **Combined** | "when event X arrives AND state Y is condition Z, do W" | Async callback handling with accumulated context |

#### 2. Typed Go Functions Replace String Interpolation

```go
// Instead of: "${steps.reviewer.verdict} == 'approved'"
Conditions: []Condition{
    {
        Description: "reviewer approved",
        Evaluate: func(ctx *RuleContext) bool {
            s := ctx.State.(*PlanReviewState)
            return s.ReviewVerdict != nil && *s.ReviewVerdict == VerdictApproved
        },
    },
}

// Instead of: payload_mapping with ${...} references
BuildPayload: func(ctx *RuleContext) (message.Payload, error) {
    s := ctx.State.(*PlanReviewState)
    return &PlannerTrigger{
        RequestID: s.RequestID,
        Slug:      s.Slug,
        Title:     s.Title,
        Prompt:    s.Prompt,
    }, nil
}
```

#### 3. Data at Rest Instead of In Flight

The critical shift: **data moves from "in flight" (message payloads between steps) to "at rest" (typed entities in KV buckets).**

New model (2 serialization boundaries):

```
Component produces typed Result struct
  → Engine calls MutateState(ctx, result) — typed function, no serialization
  → Engine calls json.Marshal(state) once, writes to KV
  → KV watch fires on next rule
  → Engine calls json.Unmarshal into typed state struct (StateFactory)
  → Engine calls Condition.Evaluate(ctx) — typed function, compiler-checked
  → Engine calls BuildPayload(ctx) — typed function, produces typed trigger
  → json.Marshal trigger, publish to NATS
  → Component receives, json.Unmarshal into typed trigger struct
```

#### 4. Go-Native Workflow Definitions

Workflows are defined in Go code, not JSON files:

```go
func PlanReviewLoop() *workflow.Definition {
    return &workflow.Definition{
        ID:          "plan-review-loop",
        Description: "Plan → Review → Revise feedback loop",
        StateBucket: "PLAN_REVIEW_EXECUTIONS",
        StateFactory: func() interface{} { return &PlanReviewState{} },
        MaxIterations: 3,
        Timeout:       30 * time.Minute,

        Rules: []workflow.RuleDef{
            {
                ID: "fire-planner",
                Trigger: workflow.TriggerSource{
                    WatchBucket:  "PLAN_REVIEW_EXECUTIONS",
                    WatchPattern: "plan-review.*",
                },
                Conditions: []workflow.Condition{
                    {
                        Description: "execution is pending or needs revision",
                        Evaluate: func(ctx *workflow.RuleContext) bool {
                            s := ctx.State.(*PlanReviewState)
                            return s.Phase == "pending" || s.Phase == "needs_revision"
                        },
                    },
                },
                Action: workflow.Action{
                    Type:               workflow.ActionPublishAsync,
                    PublishSubject:     "workflow.async.planner",
                    ExpectedResultType: "workflow.planner-result.v1",
                    BuildPayload: func(ctx *workflow.RuleContext) (message.Payload, error) {
                        s := ctx.State.(*PlanReviewState)
                        return &PlannerTrigger{
                            RequestID: s.RequestID,
                            Slug:      s.Slug,
                            Title:     s.Title,
                            Prompt:    s.Prompt,
                        }, nil
                    },
                    MutateState: func(ctx *workflow.RuleContext, result interface{}) error {
                        s := ctx.State.(*PlanReviewState)
                        r := result.(*PlannerResult)
                        s.PlanContent = r.Content
                        s.Phase = "awaiting_review"
                        s.Iteration++
                        return nil
                    },
                },
                MaxFirings: 3,
            },
            // ... additional rules
        },
    }
}
```

### Key Types

```go
// TriggerSource defines what causes a rule to evaluate
type TriggerSource struct {
    // KV-based trigger
    WatchBucket  string
    WatchPattern string

    // Stream/subject-based trigger
    Subject        string
    StreamName     string
    MessageFactory func() interface{}

    // Combined trigger: message + state
    StateBucket  string
    StateKeyFunc func(msg interface{}) string
}

// RuleContext provides typed access to state and message
type RuleContext struct {
    State      interface{} // Typed execution entity from KV
    Message    interface{} // Typed triggering message from NATS
    KVRevision uint64      // For optimistic concurrency
    Subject    string      // NATS subject (for message triggers)
}

// ConditionFunc evaluates a condition against RuleContext
type ConditionFunc func(ctx *RuleContext) bool

// PayloadBuilderFunc constructs a typed payload from RuleContext
type PayloadBuilderFunc func(ctx *RuleContext) (message.Payload, error)

// StateMutatorFunc updates execution state after action completes
type StateMutatorFunc func(ctx *RuleContext, result interface{}) error
```

### Coexistence Strategy

The reactive engine runs **alongside** the existing workflow-processor during migration:

- Both registered as separate components
- Separate KV buckets
- Feature flag in configuration
- Gradual workflow-by-workflow migration

## Consequences

### Positive

- **Zero `json.RawMessage` fields required** in workflow state types
- **Compile-time detection** of field name typos, type mismatches, and missing wiring
- **Two serialization boundaries** per inter-rule data flow vs. current 9
- **Workflow definitions are readable Go** that a developer can understand without knowing interpolation semantics
- **All three trigger modes work** — KV-only, subject-only, and message+state
- **Subsumes rules engine** — unified reactive primitive for workflows, event processing, and monitoring

### Negative

- **Non-developers cannot edit workflow definitions** — acceptable since semstreams is a developer platform
- **Existing JSON workflows require migration** — mitigated by parallel operation period
- **Learning curve** for new reactive patterns — mitigated by fluent builder API

### Neutral

- **Same runtime behavior** for external components — they still receive typed triggers, publish typed results
- **Same BaseMessage pattern** for NATS messages — no transport layer changes
- **Same KV storage** for execution state — just typed instead of `json.RawMessage`

## Implementation

### Package Structure

```
processor/reactive/
├── types.go          # ExecutionState, TriggerSource, RuleDef, RuleContext
├── engine.go         # Engine struct, Start/Stop, trigger loop management
├── trigger_kv.go     # KV watch trigger loop
├── trigger_stream.go # Stream/subject consumer trigger loop
├── evaluator.go      # Condition evaluation against RuleContext
├── dispatcher.go     # Action dispatch (publish, publish_async, mutate, complete)
├── callback.go       # Async callback handler
├── registry.go       # Workflow definition + result type registration
├── store.go          # Execution state persistence to KV
├── component.go      # Discoverable component implementation
├── config.go         # Configuration types
├── metrics.go        # Prometheus metrics
├── builder.go        # Fluent API for workflow definitions
└── engine_test.go    # Unit tests
```

### Phased Implementation

| Phase | Focus | Deliverables |
|-------|-------|--------------|
| 1 | Core Types & Triggers | types.go, trigger_kv.go, trigger_stream.go |
| 2 | Condition Evaluation | evaluator.go |
| 3 | Action Dispatch | dispatcher.go |
| 4 | Async Callbacks | callback.go |
| 5 | Execution State | store.go |
| 6 | Engine Assembly | engine.go, component.go, metrics.go |
| 7 | Builder API | builder.go |
| 8 | Testing & Docs | testutil/, examples/, documentation |
| 9 | Migration Support | adapter.go, migration guide |

## References

- [ADR-011: Workflow Processor](./adr-011-workflow-processor.md) — Original workflow processor design
- [ADR-020: Unified Dataflow Patterns](./adr-020-unified-dataflow-patterns.md) — Input/output improvements
- [Orchestration Layers](../concepts/12-orchestration-layers.md) — Three-layer orchestration model
- [Payload Registry](../concepts/13-payload-registry.md) — Typed payload deserialization
