# ADR-022: Workflow Engine Simplification

## Status

Accepted

## Context

The reactive workflow engine (`processor/reactive/`) solved the type safety problem identified in ADR-021 but introduced significant complexity (~3500 lines) for the common case of stateless reactive rules.

### The Semspec Observation

> "You built Airflow on top of Kafka. But Kafka doesn't need Airflow — the consumer groups ARE the DAG."

Semstreams is already a reactive dataflow engine. Components subscribe to subjects, process messages, publish to other subjects. The message topology IS the execution graph. The workflow engine re-declares what the topology already defines.

### Two Patterns Conflated

| Pattern | Usage | Code Complexity |
|---------|-------|-----------------|
| Stateless reactive rules | 90% of actual usage | Should be ~20% of code |
| Stateful multi-step workflows | 10% of usage | Currently ~80% of code |

### Actual Usage in E2E (workflows.go)

- Watch ENTITY_STATES KV bucket
- Evaluate typed conditions against `graph.EntityState`
- Publish alerts fire-and-forget
- Use cooldowns to prevent rapid re-firing

This is Pattern A (stateless rules). No async callbacks, no execution state persistence, no phase transitions.

## Decision

1. **Simplify** `processor/reactive/` to handle stateless reactive rules only
2. **Delete** workflow orchestration complexity (callbacks, execution store, state machine)
3. **For true stateful workflows**, components report state to a shared KV bucket as a side effect
4. **Workflows become inferred** from registered components and their message topology

## What Was Deleted (~1550 lines)

| File | Lines | Reason |
|------|-------|--------|
| callback.go, callback_test.go | ~600 | Components handle their own async correlation |
| store.go, store_test.go | ~1200 | No execution state persistence for stateless rules |
| Most of state.go | ~350 | Keep only minimal helpers |
| Most of dispatcher.go | ~600 | Simplify to fire-and-forget publish |
| conditions.go | ~200 | Workflow-specific, move to caller |
| examples/ | ~500 | Examples of unused workflow patterns |

## What Remains (~1950 lines, 55% reduction)

**Renamed:** `processor/reactive/` → `processor/workflow/`
**Component name:** `workflow` (short and clear)

| File | Purpose |
|------|---------|
| trigger_kv.go | KV watch loop - core reactive primitive |
| trigger_stream.go | Subject consumer - core reactive primitive |
| evaluator.go (simplified) | Condition eval + cooldowns |
| dispatcher.go (simplified) | Fire-and-forget publish only |
| builder.go (simplified) | Rule builder fluent API |
| registry.go (simplified) | Rule registry |
| component.go | Component wrapper |
| config.go (simplified) | Configuration |
| metrics.go | Prometheus metrics (`semstreams_workflow_*`) |

## Workflow State Package

For components that participate in stateful workflows, a new `pkg/workflow/` package provides:

### WorkflowParticipant Interface

```go
// pkg/workflow/participant.go

// WorkflowParticipant is implemented by components that participate in stateful workflows.
type WorkflowParticipant interface {
    WorkflowID() string
    Phase() string
    StateManager() *StateManager
}

// ParticipantRegistry tracks all workflow participants for observability.
type ParticipantRegistry struct {
    participants map[string][]WorkflowParticipant
}
```

### State Types

```go
// pkg/workflow/state.go

type State struct {
    ID          string         `json:"id"`
    WorkflowID  string         `json:"workflow_id"`
    Phase       string         `json:"phase"`
    Iteration   int            `json:"iteration"`
    MaxIter     int            `json:"max_iter,omitempty"`
    StartedAt   time.Time      `json:"started_at"`
    UpdatedAt   time.Time      `json:"updated_at"`
    CompletedAt *time.Time     `json:"completed_at,omitempty"`
    Error       string         `json:"error,omitempty"`
    Context     map[string]any `json:"context,omitempty"`
}

type StateManager struct {
    bucket jetstream.KeyValue
}

func (m *StateManager) Get(ctx context.Context, id string) (*State, error)
func (m *StateManager) Put(ctx context.Context, state *State) error
func (m *StateManager) Transition(ctx context.Context, id, phase string) error
func (m *StateManager) Complete(ctx context.Context, id string) error
func (m *StateManager) Fail(ctx context.Context, id, errMsg string) error
```

### Component Implementation Pattern

```go
// Example: ReviewerComponent implements WorkflowParticipant
type ReviewerComponent struct {
    states *workflow.StateManager
}

func (c *ReviewerComponent) WorkflowID() string { return "plan-review" }
func (c *ReviewerComponent) Phase() string      { return "reviewing" }
func (c *ReviewerComponent) StateManager() *workflow.StateManager { return c.states }

func (c *ReviewerComponent) handleMessage(ctx context.Context, msg ReviewRequest) {
    // Update workflow state (interface makes this discoverable)
    c.states.Transition(ctx, msg.ExecutionID, c.Phase())

    // Do actual work
    result := c.review(ctx, msg)

    // Publish to next topic - topology IS the workflow
    c.publish(ctx, "reviewed."+msg.ExecutionID, result)
}
```

## Consequences

### Positive

- 55% code reduction (~1550 lines deleted)
- Simpler mental model: "rules react to KV/subjects"
- Components remain workflow-agnostic
- True workflows emerge from topology, not definitions
- Aligns with semstreams' reactive philosophy

### Negative

- Less "workflow definition language" expressiveness
- Components needing state must manage it themselves

### Neutral

- Same E2E test behavior (same metrics, same data flow)
- Same component interface pattern

## Related ADRs

- ADR-020: Unified Dataflow Patterns
- ADR-021: Reactive Workflow Engine (superseded by this ADR)
- ADR-018: Agentic Workflow Orchestration
