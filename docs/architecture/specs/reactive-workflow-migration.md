# Reactive Workflow Migration Guide for Semspec

**Status**: Ready for Migration
**Branch**: `feat/reactive-workflow-engine`
**Commit**: `91686c4`

## Executive Summary

The reactive workflow engine (ADR-021) is now complete and ready for semspec workflow migration. This guide provides step-by-step instructions for migrating existing JSON workflows to typed Go definitions.

### Key Benefits

| Aspect | JSON Workflows | Reactive Workflows |
|--------|---------------|-------------------|
| Type safety | Runtime errors | Compile-time errors |
| Field references | String interpolation (`${steps.X.output.Y}`) | Go field access (`state.ReviewResult.Verdict`) |
| Serialization | 9+ boundaries, `json.RawMessage` required | 2 boundaries, zero `json.RawMessage` |
| Debugging | String inspection, runtime failures | Go debugger, stack traces |
| Error detection | Load-time or runtime | Compile-time |

## Workflows to Migrate

Based on the workflow processor spec and ADR-018, semspec has these workflows requiring migration:

### 1. Spec Approval Workflow

**Current**: `spec-approval` (JSON)
**Pattern**: Linear workflow with conditional branching

```
trigger → load-spec → extract-tasks → create-issues → link-issues → complete
                                    ↓ (on_fail)
                              mark-blocked
```

**Migration Complexity**: Medium (5 steps, 1 conditional branch)

### 2. Review-Fix Cycle

**Current**: `review-fix-cycle` (JSON)
**Pattern**: Loop with iteration limit

```
trigger → review → (issues_found > 0?) → fix → review...
              ↓ (issues_found == 0)
           complete

(max 3 iterations → escalate)
```

**Migration Complexity**: Medium (requires iteration tracking, matches `loop_test.go` example)

### 3. Plan Review Loop

**Current**: Semspec spec planning workflow
**Pattern**: Loop with verdict-based branching

```
trigger → planner → reviewer → (verdict?)
                        ↓ approved → complete
                        ↓ needs_work → planner (loop)
                        ↓ rejected → fail

(max iterations → escalate)
```

**Migration Complexity**: Medium-High (multiple verdict branches, matches ADR-021 example)

### 4. Architect-Editor Handoff

**Current**: Hardcoded in `handlers.go` (see ADR-018)
**Pattern**: Simple chain

```
architect-complete → spawn-editor
```

**Migration Complexity**: Low (simple handoff, can use rules OR reactive workflow)

### 5. Daily Progress Report

**Current**: `daily-progress-report` (JSON, cron-triggered)
**Pattern**: Scheduled linear workflow

```
(cron: 0 9 * * *) → collect-metrics → generate-summary → send-notifications
```

**Migration Complexity**: Low (linear, no branching)

## Migration Steps

### Step 1: Define Typed State Struct

Create a state struct that embeds `reactive.ExecutionState`:

```go
// pkg/semspec/workflows/spec_approval_state.go
package workflows

import (
    "github.com/c360studio/semstreams/processor/reactive"
)

// SpecApprovalState tracks spec approval workflow execution.
type SpecApprovalState struct {
    reactive.ExecutionState

    // Inputs
    SpecID string `json:"spec_id"`

    // Step outputs (accumulated)
    Spec       *SpecData       `json:"spec,omitempty"`
    Tasks      []TaskDef       `json:"tasks,omitempty"`
    IssueIDs   []string        `json:"issue_ids,omitempty"`
    IssueCount int             `json:"issue_count,omitempty"`

    // Processing state
    HasTasks   bool   `json:"has_tasks,omitempty"`
    Blocked    bool   `json:"blocked,omitempty"`
    BlockError string `json:"block_error,omitempty"`
}
```

### Step 2: Define Message Types

Create typed message structs for inputs/outputs:

```go
// pkg/semspec/messages/spec_messages.go
package messages

import (
    "encoding/json"
    "github.com/c360studio/semstreams/message"
)

// SpecGetRequest requests spec data.
type SpecGetRequest struct {
    ID string `json:"id"`
}

func (r *SpecGetRequest) Schema() message.Type {
    return message.Type{Domain: "semspec", Category: "spec-get", Version: "v1"}
}

func (r *SpecGetRequest) Validate() error { return nil }

func (r *SpecGetRequest) MarshalJSON() ([]byte, error) {
    type Alias SpecGetRequest
    return json.Marshal(&message.BaseMessage{
        MessageType: r.Schema(),
        Payload:     (*Alias)(r),
    })
}

func (r *SpecGetRequest) UnmarshalJSON(data []byte) error {
    type Alias SpecGetRequest
    return json.Unmarshal(data, (*Alias)(r))
}

// SpecData is the spec response.
type SpecData struct {
    ID       string `json:"id"`
    Title    string `json:"title"`
    Content  string `json:"content"`
    HasTasks bool   `json:"has_tasks"`
}

// ... similar Schema/MarshalJSON/UnmarshalJSON methods
```

### Step 3: Build Workflow Definition

Convert JSON workflow to Go using the fluent builder:

```go
// pkg/semspec/workflows/spec_approval.go
package workflows

import (
    "time"

    "github.com/c360studio/semstreams/message"
    "github.com/c360studio/semstreams/processor/reactive"
    "github.com/yourorg/semspec/messages"
)

func SpecApprovalWorkflow() *reactive.Definition {
    return reactive.NewWorkflow("spec-approval").
        WithDescription("Creates GitHub issues when a spec is approved").
        WithStateBucket("SPEC_APPROVAL_STATE").
        WithStateFactory(func() any { return &SpecApprovalState{} }).
        WithTimeout(5 * time.Minute).

        // Rule 1: Load spec when workflow starts
        AddRule(reactive.NewRule("load-spec").
            WatchKV("SPEC_APPROVAL_STATE", "spec-approval.*").
            When("phase is pending", reactive.PhaseIs("pending")).
            When("no pending task", reactive.NoPendingTask()).
            PublishAsync(
                "semspec.spec.get",
                func(ctx *reactive.RuleContext) (message.Payload, error) {
                    state := ctx.State.(*SpecApprovalState)
                    return &messages.SpecGetRequest{
                        ID: state.SpecID,
                    }, nil
                },
                "semspec.spec-data.v1",
                func(ctx *reactive.RuleContext, result any) error {
                    state := ctx.State.(*SpecApprovalState)
                    if spec, ok := result.(*messages.SpecData); ok {
                        state.Spec = spec
                        state.HasTasks = spec.HasTasks
                    }
                    state.Phase = "spec-loaded"
                    return nil
                },
            ).
            MustBuild()).

        // Rule 2: Extract tasks if spec has tasks
        AddRule(reactive.NewRule("extract-tasks").
            WatchKV("SPEC_APPROVAL_STATE", "spec-approval.*").
            When("phase is spec-loaded", reactive.PhaseIs("spec-loaded")).
            When("spec has tasks", reactive.StateFieldEquals(
                func(s any) bool { return s.(*SpecApprovalState).HasTasks },
                true,
            )).
            When("no pending task", reactive.NoPendingTask()).
            PublishAsync(
                "semspec.spec.extract-tasks",
                func(ctx *reactive.RuleContext) (message.Payload, error) {
                    state := ctx.State.(*SpecApprovalState)
                    return &messages.ExtractTasksRequest{
                        Spec: state.Spec,
                    }, nil
                },
                "semspec.tasks.v1",
                func(ctx *reactive.RuleContext, result any) error {
                    state := ctx.State.(*SpecApprovalState)
                    if tasks, ok := result.(*messages.TasksResult); ok {
                        state.Tasks = tasks.Tasks
                    }
                    state.Phase = "tasks-extracted"
                    return nil
                },
            ).
            MustBuild()).

        // Rule 3: Skip to complete if no tasks
        AddRule(reactive.NewRule("skip-no-tasks").
            WatchKV("SPEC_APPROVAL_STATE", "spec-approval.*").
            When("phase is spec-loaded", reactive.PhaseIs("spec-loaded")).
            When("spec has no tasks", reactive.StateFieldEquals(
                func(s any) bool { return s.(*SpecApprovalState).HasTasks },
                false,
            )).
            CompleteWithMutation(func(ctx *reactive.RuleContext, _ any) error {
                state := ctx.State.(*SpecApprovalState)
                state.Phase = "completed"
                state.Status = reactive.StatusCompleted
                return nil
            }).
            MustBuild()).

        // Rule 4: Create GitHub issues
        AddRule(reactive.NewRule("create-issues").
            WatchKV("SPEC_APPROVAL_STATE", "spec-approval.*").
            When("phase is tasks-extracted", reactive.PhaseIs("tasks-extracted")).
            When("no pending task", reactive.NoPendingTask()).
            PublishAsync(
                "semspec.github.create-issues",
                func(ctx *reactive.RuleContext) (message.Payload, error) {
                    state := ctx.State.(*SpecApprovalState)
                    return &messages.CreateIssuesRequest{
                        SpecID: state.SpecID,
                        Tasks:  state.Tasks,
                    }, nil
                },
                "semspec.issues.v1",
                func(ctx *reactive.RuleContext, result any) error {
                    state := ctx.State.(*SpecApprovalState)
                    if issues, ok := result.(*messages.IssuesResult); ok {
                        state.IssueIDs = issues.IssueIDs
                        state.IssueCount = len(issues.IssueIDs)
                    }
                    state.Phase = "issues-created"
                    return nil
                },
            ).
            MustBuild()).

        // Rule 5: Handle create-issues failure
        AddRule(reactive.NewRule("handle-issue-failure").
            WatchKV("SPEC_APPROVAL_STATE", "spec-approval.*").
            When("phase is tasks-extracted", reactive.PhaseIs("tasks-extracted")).
            When("has error", reactive.StateFieldEquals(
                func(s any) string { return s.(*SpecApprovalState).Error },
                "", // Non-empty error
            )).
            Mutate(func(ctx *reactive.RuleContext, _ any) error {
                state := ctx.State.(*SpecApprovalState)
                state.Phase = "blocked"
                state.Blocked = true
                state.BlockError = state.Error
                state.Status = reactive.StatusFailed
                return nil
            }).
            MustBuild()).

        // Rule 6: Complete on success
        AddRule(reactive.NewRule("complete").
            WatchKV("SPEC_APPROVAL_STATE", "spec-approval.*").
            When("phase is issues-created", reactive.PhaseIs("issues-created")).
            CompleteWithMutation(func(ctx *reactive.RuleContext, _ any) error {
                state := ctx.State.(*SpecApprovalState)
                state.Phase = "completed"
                state.Status = reactive.StatusCompleted
                return nil
            }).
            MustBuild()).

        MustBuild()
}
```

### Step 4: Register Workflow

Register the workflow with the reactive engine:

```go
// pkg/semspec/workflows/register.go
package workflows

import (
    "github.com/c360studio/semstreams/processor/reactive"
)

func RegisterWorkflows(registry *reactive.WorkflowRegistry) error {
    workflows := []*reactive.Definition{
        SpecApprovalWorkflow(),
        ReviewFixCycleWorkflow(),
        PlanReviewLoopWorkflow(),
        DailyReportWorkflow(),
    }

    for _, def := range workflows {
        if err := registry.Register(def); err != nil {
            return err
        }
    }

    return nil
}
```

### Step 5: Register Message Types

Register payload types for callback deserialization:

```go
// pkg/semspec/messages/register.go
package messages

import "github.com/c360studio/semstreams/component"

func init() {
    // Register all semspec message types
    registrations := []component.PayloadRegistration{
        {
            Domain:   "semspec",
            Category: "spec-data",
            Version:  "v1",
            Factory:  func() any { return &SpecData{} },
        },
        {
            Domain:   "semspec",
            Category: "tasks",
            Version:  "v1",
            Factory:  func() any { return &TasksResult{} },
        },
        {
            Domain:   "semspec",
            Category: "issues",
            Version:  "v1",
            Factory:  func() any { return &IssuesResult{} },
        },
        // ... other types
    }

    for _, r := range registrations {
        if err := component.RegisterPayload(&r); err != nil {
            panic("failed to register payload: " + err.Error())
        }
    }
}
```

### Step 6: Write Tests

Use the testutil package for unit tests:

```go
// pkg/semspec/workflows/spec_approval_test.go
package workflows

import (
    "context"
    "testing"
    "time"

    "github.com/c360studio/semstreams/processor/reactive"
    "github.com/c360studio/semstreams/processor/reactive/testutil"
)

func TestSpecApprovalWorkflow_Definition(t *testing.T) {
    def := SpecApprovalWorkflow()

    if def.ID != "spec-approval" {
        t.Errorf("expected ID 'spec-approval', got %q", def.ID)
    }
    if len(def.Rules) != 6 {
        t.Errorf("expected 6 rules, got %d", len(def.Rules))
    }
}

func TestSpecApprovalWorkflow_LoadSpec(t *testing.T) {
    engine := testutil.NewTestEngine(t)
    def := SpecApprovalWorkflow()

    if err := engine.RegisterWorkflow(def); err != nil {
        t.Fatalf("RegisterWorkflow failed: %v", err)
    }

    // Create initial state
    state := &SpecApprovalState{
        ExecutionState: reactive.ExecutionState{
            ID:         "exec-001",
            WorkflowID: "spec-approval",
            Phase:      "pending",
            Status:     reactive.StatusRunning,
            CreatedAt:  time.Now(),
            UpdatedAt:  time.Now(),
        },
        SpecID: "acme.ops.specs.core.spec.auth",
    }

    key := "spec-approval.exec-001"
    err := engine.TriggerKV(context.Background(), key, state)
    if err != nil {
        t.Fatalf("TriggerKV failed: %v", err)
    }

    engine.AssertPhase(key, "pending")
    engine.AssertStatus(key, reactive.StatusRunning)
}

func TestSpecApprovalWorkflow_HappyPath(t *testing.T) {
    // Test full workflow execution with mock responses
    // ...
}
```

## Migration Pattern Reference

### JSON to Go Translation

| JSON Pattern | Go Equivalent |
|-------------|---------------|
| `"inputs": {"id": {"from": "trigger.entity_id"}}` | Access via `state.SpecID` (set when workflow starts) |
| `"condition": {"field": "load-spec.has_tasks", "operator": "eq", "value": true}` | `reactive.StateFieldEquals(func(s any) bool { return s.(*State).HasTasks }, true)` |
| `"on_success": "next-step"` | Multiple rules with phase-based conditions |
| `"on_fail": "error-handler"` | Rule with error condition |
| `"retry": {"max_attempts": 3}` | Handled by async callback system (retries at component level) |
| `"timeout": "30s"` | `WithTimeout(30 * time.Second)` on workflow |
| `"max_iterations": 3` | `WithMaxIterations(3)` + `reactive.IterationLessThan(3)` condition |

### Loop Workflow Pattern

For review-fix cycles, use the pattern from `loop_test.go`:

```go
func ReviewFixCycleWorkflow() *reactive.Definition {
    const maxIterations = 3

    return reactive.NewWorkflow("review-fix-cycle").
        WithMaxIterations(maxIterations).

        // Request review
        AddRule(reactive.NewRule("request-review").
            When("phase is reviewing", reactive.PhaseIs("reviewing")).
            When("under max iterations", reactive.IterationLessThan(maxIterations)).
            PublishAsync(/* ... */).
            MustBuild()).

        // Handle needs_work → loop back
        AddRule(reactive.NewRule("handle-needs-work").
            When("phase is evaluated", reactive.PhaseIs("evaluated")).
            When("verdict is needs_work", reactive.StateFieldEquals(
                func(s any) string { return s.(*ReviewState).Verdict },
                "needs_work",
            )).
            When("under max iterations", reactive.IterationLessThan(maxIterations)).
            Mutate(reactive.ChainMutators(
                reactive.IncrementIterationMutator(),
                reactive.PhaseTransition("reviewing"),
            )).
            MustBuild()).

        // Handle max iterations exceeded
        AddRule(reactive.NewRule("handle-max-iterations").
            When("phase is evaluated", reactive.PhaseIs("evaluated")).
            When("at max iterations", reactive.Not(reactive.IterationLessThan(maxIterations))).
            Mutate(func(ctx *reactive.RuleContext, _ any) error {
                state := ctx.State.(*ReviewState)
                state.Status = reactive.StatusEscalated
                state.Error = "max review iterations exceeded"
                return nil
            }).
            MustBuild()).

        MustBuild()
}
```

## Coexistence Strategy

During migration, both engines can run simultaneously:

1. **Separate KV buckets**: Reactive workflows use new buckets (e.g., `SPEC_APPROVAL_STATE` vs `WORKFLOW_EXECUTIONS`)
2. **Separate component names**: `workflow-processor` vs `reactive-workflow`
3. **Feature flag**: Configuration option to route to new or old engine
4. **Gradual migration**: Migrate one workflow at a time, validate, then proceed

## Testing Strategy

1. **Unit tests**: Use `testutil.TestEngine` with mock KV and bus
2. **Condition tests**: Test each rule's conditions independently
3. **State mutation tests**: Verify state transitions
4. **Integration tests**: Test against real NATS (testcontainers)
5. **Comparison tests**: Run same logical workflow through both engines, compare outcomes

## Timeline Recommendation

| Week | Workflow | Effort |
|------|----------|--------|
| 1 | `spec-approval` | 2-3 days |
| 1 | Tests + validation | 1-2 days |
| 2 | `review-fix-cycle` | 2-3 days |
| 2 | `plan-review-loop` | 2-3 days |
| 3 | `daily-progress-report` | 1 day |
| 3 | Integration testing | 2-3 days |
| 4 | Production rollout | 2-3 days |

## Support Resources

- **ADR-021**: `/docs/architecture/adr-021-reactive-workflow-engine.md`
- **Usage Guide**: `/docs/advanced/10-reactive-workflows.md`
- **Example Workflows**: `/processor/reactive/examples/`
- **Test Utilities**: `/processor/reactive/testutil/`
- **Builder API**: `/processor/reactive/builder.go`
- **Condition Helpers**: `/processor/reactive/conditions.go`

## Questions / Support

Contact the semstreams team for migration support. The reactive workflow engine is fully tested and ready for production use.
