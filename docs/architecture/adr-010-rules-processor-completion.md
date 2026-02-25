# ADR-010: Rules Processor Completion

## Status

Superseded by ADR-021

> **Note**: This ADR describes completion work for the JSON-based rules processor.
> The rules processor has been superseded by [ADR-021: Reactive Workflow Engine](./adr-021-reactive-workflow-engine.md),
> which provides typed Go conditions, dual reactive primitives (KV + subject), and eliminates string interpolation.
>
> See [Reactive Workflows Guide](../advanced/10-reactive-workflows.md) for current documentation.

## Context

The rules processor implements a stateful Event-Condition-Action (ECA) pattern with:

- **Expression-based conditions**: Operators (eq, ne, lt, gt, contains, etc.) with nested field extraction
- **State tracking**: Persistent rule match state in NATS KV (`RULE_STATE` bucket)
- **Transition detection**: OnEnter/OnExit/WhileTrue actions based on state changes
- **Entity watching**: KV pattern-based watching with debouncing

However, critical action types are **stubbed out** with TODO comments:

| Action Type | Status | Location | Impact |
|-------------|--------|----------|--------|
| `ActionTypePublish` | Logging only | `actions.go:247` | Cannot trigger workflows or publish events |
| `ActionTypeUpdateTriple` | Logging only | `actions.go:266` | Cannot update triple metadata |
| Dynamic watch patterns | Not implemented | `runtime_config.go:29` | Config changes require restart |

The workflow processor (ADR-011) depends on `ActionTypePublish` to trigger workflows from rule transitions. Without this, the rules → workflow integration path is broken.

## Decision

Complete the rules processor implementation in three phases:

### Phase 1: ActionTypePublish (Critical Path)

Implement actual NATS publish in `actions.go`:

```go
case ActionTypePublish:
    subject := substituteVariables(action.Subject, ctx)
    payload := substituteVariables(action.Payload, ctx)

    data, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("marshal publish payload: %w", err)
    }

    if err := e.nc.Publish(subject, data); err != nil {
        return fmt.Errorf("publish to %s: %w", subject, err)
    }

    e.logger.Info("published message",
        "subject", subject,
        "rule_id", ctx.RuleID,
        "entity_id", ctx.EntityID)
```

**Requirements:**
- Variable substitution in subject and payload (`$entity.id`, `$related.id`, field paths)
- JSON serialization of payload
- Error handling for publish failures
- Structured logging for observability

### Phase 2: ActionTypeUpdateTriple

Implement triple metadata updates:

```go
case ActionTypeUpdateTriple:
    req := &UpdateTripleRequest{
        Subject:   substituteVariables(action.Subject, ctx),
        Predicate: action.Predicate,
        Metadata:  action.Metadata,
    }

    // Request/response to graph processor
    resp, err := e.nc.Request(subjectTripleUpdate, req, 5*time.Second)
    if err != nil {
        return fmt.Errorf("update triple: %w", err)
    }
```

**Requirements:**
- Define update request/response types
- Graph processor must support update subject
- Handle revision conflicts

### Phase 3: Dynamic Watch Pattern Reloading

Complete `ApplyConfigUpdate` in `runtime_config.go`:

```go
func (p *Processor) ApplyConfigUpdate(update ConfigUpdate) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Handle watch pattern changes
    if update.EntityWatchPatterns != nil {
        if err := p.entityWatcher.UpdatePatterns(update.EntityWatchPatterns); err != nil {
            return fmt.Errorf("update watch patterns: %w", err)
        }
    }

    // Existing rule add/remove logic...
}
```

**Requirements:**
- EntityWatcher needs `UpdatePatterns()` method
- Graceful transition (don't miss events during update)
- Validate patterns before applying

## Consequences

### Positive

- **Unblocks workflow processor**: Rules can trigger workflows via publish actions
- **Completes ECA implementation**: All documented action types functional
- **Enables rule-driven automation**: Publish to any NATS subject from rules
- **Hot-reload support**: Configuration changes without restarts

### Negative

- **Testing complexity**: Publish actions need integration tests with NATS
- **Error handling decisions**: What happens when publish fails mid-rule execution?
- **Backwards compatibility**: Existing rules with publish actions will start actually publishing

### Neutral

- **Metrics additions**: New metrics for publish success/failure rates
- **Documentation updates**: Action type documentation needs completion

## Implementation Plan

### Phase 1: ActionTypePublish
1. Add NATS connection to ActionExecutor
2. Implement publish with variable substitution
3. Add error handling and logging
4. Write unit tests with mock NATS
5. Write integration test with embedded NATS

### Phase 2: ActionTypeUpdateTriple
1. Define UpdateTripleRequest/Response in message package
2. Add update subject handler to graph processor
3. Implement action execution
4. Test with graph processor integration

### Phase 3: Dynamic Watch Patterns
1. Add UpdatePatterns to EntityWatcher interface
2. Implement graceful pattern transition
3. Update ApplyConfigUpdate
4. Test hot-reload scenarios

## Key Files

| File | Change |
|------|--------|
| `processor/rule/actions.go` | Implement Publish (line 247) and UpdateTriple (line 266) |
| `processor/rule/action_executor.go` | Add NATS connection dependency |
| `processor/rule/runtime_config.go` | Complete dynamic watch pattern reload (line 29) |
| `processor/rule/entity_watcher.go` | Add UpdatePatterns method |
| `processor/rule/actions_test.go` | Integration tests for new actions |

## References

- [ADR-011: Workflow Processor](./adr-011-workflow-processor.md) - depends on this ADR
- [Workflow Processor Spec](./specs/workflow-processor-spec.md) - defines rule trigger integration
