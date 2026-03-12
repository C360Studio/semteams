# Migration Guide: alpha.30 — Rule Processor Enrichment

## Overview

alpha.30 adds four targeted improvements to the rule processor's stateful evaluation pipeline:

1. **ExecutionContext** — replaces `(entityID, relatedID string)` action signature
2. **Conditional Actions** — `When` clause on actions for action-level guard conditions
3. **Iteration Tracking** — `Iteration` and `MaxIterations` fields in `MatchState`
4. **`$state.*` Pseudo-fields** — `$state.iteration`, `$state.max_iterations`, `$state.last_transition` in expression evaluator

## Breaking Changes

### ActionExecutorInterface.Execute Signature

**Before:**
```go
Execute(ctx context.Context, action Action, entityID string, relatedID string) error
```

**After:**
```go
Execute(ctx context.Context, action Action, ec *ExecutionContext) error
```

**Migration:** Update all `ActionExecutorInterface` implementations. Access entity/related IDs via `ec.EntityID` and `ec.RelatedID`. Full entity state is available via `ec.Entity` and `ec.Related`.

### EntityContext Removed

The `EntityContext` struct and `substituteVariablesWithContext()` function have been removed from `actions.go`. Variable substitution now uses `ec.SubstituteVariables()` on `ExecutionContext`, which resolves `$entity.*` variables from entity triples.

**Migration:** Replace `substituteVariablesWithContext(value, entityCtx)` with `ec.SubstituteVariables(value)`.

### EvaluateWithState Signature

**Before:**
```go
EvaluateWithState(ctx, ruleDef, entityID, relatedID, triggered) (Transition, error)
```

**After:**
```go
EvaluateWithState(ctx, ruleDef, entityID, relatedID, triggered, entity, related) (Transition, error)
```

**Migration:** Add `*gtypes.EntityState` parameters. Pass `nil, nil` for message-path rules where entity state is not available.

### MatchState New Fields

Two new fields added to `MatchState`:

```go
Iteration     int `json:"iteration"`
MaxIterations int `json:"max_iterations,omitempty"`
```

These fields use `omitempty` (MaxIterations) or zero-value (Iteration), so existing serialized state deserializes cleanly with no migration needed.

## New Features

### When Clause on Actions

Actions can now include a `when` array of conditions. The action only executes if all conditions match (AND logic):

```json
{
  "on_enter": [
    {
      "type": "publish",
      "subject": "ripple.plan",
      "when": [{"field": "proposal.affects_plans", "operator": "eq", "value": true}]
    }
  ]
}
```

When `entity` is nil (message-path rules), When clauses on non-`$state.*` fields are skipped (action executes).

### $state.* Pseudo-fields

Available in both rule-level conditions and action-level When clauses:

| Field | Type | Description |
|-------|------|-------------|
| `$state.iteration` | int | Number of times rule has entered matching state for this entity |
| `$state.max_iterations` | int | Configured limit from rule definition (0 = unlimited) |
| `$state.last_transition` | string | `"entered"`, `"exited"`, or `"none"` |

Example — retry budget with escalation:
```json
{
  "max_iterations": 3,
  "on_enter": [
    {
      "type": "publish", "subject": "retry.task",
      "when": [{"field": "$state.iteration", "operator": "lt", "value": 3}]
    },
    {
      "type": "publish", "subject": "escalate.task",
      "when": [{"field": "$state.iteration", "operator": "gte", "value": 3}]
    }
  ]
}
```

### MaxIterations on Definition

Rule definitions can set `max_iterations` to expose the limit via `$state.max_iterations` in When clauses. No enforcement is built in — use When clauses for branching logic.

## Affected Files

| File | Change |
|------|--------|
| `processor/rule/execution_context.go` | NEW |
| `processor/rule/actions.go` | Execute signature, When field, removed EntityContext |
| `processor/rule/stateful_evaluator.go` | ExecutionContext, When eval, iteration tracking |
| `processor/rule/message_handler.go` | Updated EvaluateWithState call sites |
| `processor/rule/state_tracker.go` | Iteration/MaxIterations fields |
| `processor/rule/rule_factory.go` | MaxIterations on Definition |
| `processor/rule/expression/evaluator.go` | $state.* resolution, nil entity guard |
| `processor/rule/expression/types.go` | StateFields type |
| `schemas/rule-processor.v1.json` | Updated with new fields |
