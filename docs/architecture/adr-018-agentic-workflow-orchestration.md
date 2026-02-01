# ADR-018: Agentic Workflow Orchestration

## Status

Accepted

## Context

The rules engine has powerful workflow capabilities that are underutilized for agentic workflow orchestration.
Currently, workflow logic like agent handoffs is hardcoded in the agentic-loop component, mixing execution
mechanics with orchestration logic.

**Key insight**: After analysis, we determined that both rules AND workflows are needed:
- **Simple handoffs** (A → B, no retry): Rules engine is sufficient
- **Complex workflows** (loops, timeouts, limits): Requires workflow processor (ADR-011)

This ADR focuses on which orchestration layer handles which agentic patterns. See
[Orchestration Layers](../concepts/12-orchestration-layers.md) for the complete three-layer model.

### The Separation Problem

**Current state:** The `architect → editor` handoff is hardcoded in `processor/agentic-loop/handlers.go:320-358`:

```go
// handlers.go:320-323
if entity.Role == "architect" {
    return h.spawnEditorFromArchitect(result, entity, responseContent)
}
```

**Problems with this approach:**

| Issue | Description |
|-------|-------------|
| Role check is string literal | `"architect"` hardcoded, no configuration |
| Downstream role hardcoded | `"editor"` fixed, can't change target |
| Prompt template hardcoded | `"Implement based on architecture: "` fixed |
| No configuration options | Requires code changes for new patterns |
| Completion event lacks data | No `role`, `result` content for rules to react |

### What the Rules Engine Already Has

The rules processor (`processor/rule/`) already provides the infrastructure for workflow orchestration:

| Capability | Status | Location |
|------------|--------|----------|
| ECA pattern (event-condition-action) | ✅ Ready | `processor/rule/stateful_evaluator.go` |
| `publish_agent` action type | ✅ Ready | `processor/rule/actions.go:428-498` |
| State tracking with KV persistence | ✅ Ready | `RULE_STATE` bucket |
| Hot-reloadable rules from JSON | ✅ Ready | `processor/rule/processor.go` |
| Variable substitution | ⚠️ Limited | `$entity.id`, `$related.id` only |
| NATS subject watching | ❌ Missing | Only watches KV changes |

### Completion Event Gap

The current completion event lacks the data rules need to make orchestration decisions:

```go
// handlers.go:306-310 - CURRENT (insufficient)
completion := map[string]any{
    "loop_id": loopID,
    "task_id": entity.TaskID,
    "outcome": "success",
    // MISSING: role, result, model, parent_loop, iterations
}
```

## Decision

Define a clear separation principle: **Components own execution mechanics; rules own orchestration logic.**

### Separation Principle

| Question | Belongs In |
|----------|-----------|
| How to execute a single agent turn? | **Component** (agentic-loop) |
| What happens after agent X completes? | **Rules/Workflow** |
| Under what conditions should agent Y start? | **Rules** |
| Which model to use for a request? | **Config** (agentic-model aliases) |

### Options Evaluated

#### Option A: Keep in agentic-loop (Status Quo)

- **Pro**: Simple, co-located with loop orchestration
- **Con**: Hardcoded workflow, requires code changes for new patterns
- **Verdict**: Not scalable

#### Option B: Rules engine only

- **Pro**: Hot-reloadable, consistent with existing ECA pattern, `publish_agent` already exists
- **Con**: Cannot handle loops with limits, timeouts, or complex sequencing
- **Verdict**: Sufficient for simple handoffs only

#### Option C: Workflow processor only (ADR-011)

- **Pro**: Clean abstraction for multi-step orchestration
- **Con**: Overkill for simple A → B handoffs
- **Verdict**: Needed for complex patterns, but not all patterns

#### Option D: Hybrid — rules + minimal workflow processor (Recommended)

- **Pro**: Right tool for each pattern; clear separation of concerns
- **Con**: Two systems to understand
- **Verdict**: Best fit given the range of orchestration needs

### Recommended Architecture (Option D)

Use **rules for simple handoffs** and **workflows for loops/timeouts**:

| Pattern | Orchestration Layer |
|---------|---------------------|
| Architect completes → spawn editor | Rules |
| Reviewer → fixer → reviewer (max 3x) | Workflow |
| Semspec → multi-agent pipeline | Workflow |
| Entity state change → spawn agent | Rules |

This hybrid approach requires:

1. Enriching completion events with orchestration-relevant data
2. Writing completion state to KV for rules engine to watch
3. Creating JSON rules that express workflow transitions
4. Extending variable substitution for entity fields

**Rule-based workflow pattern:**

```text
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  agentic-loop   │────►│  AGENT_LOOPS KV  │────►│  rules engine   │
│  (completion)   │     │  (state)         │     │  (ECA)          │
└─────────────────┘     └──────────────────┘     └────────┬────────┘
                                                          │
                                                  ┌───────▼────────┐
                                                  │ publish_agent  │
                                                  │ (spawn next)   │
                                                  └───────┬────────┘
                                                          │
┌─────────────────┐     ┌──────────────────┐     ┌───────▼────────┐
│  agentic-loop   │◄────│  agent.task.*    │◄────│  NATS subject  │
│  (new loop)     │     │  (task message)  │     │  (output)      │
└─────────────────┘     └──────────────────┘     └────────────────┘
```

### Example: Architect → Editor Workflow Rule

Replace the hardcoded logic with a JSON rule definition:

```json
{
  "id": "architect_complete_spawn_editor",
  "description": "When architect completes, spawn editor to implement",
  "entity": {
    "pattern": "LOOP_*"
  },
  "conditions": [
    {"field": "role", "operator": "eq", "value": "architect"},
    {"field": "outcome", "operator": "eq", "value": "success"}
  ],
  "on_enter": [{
    "type": "publish_agent",
    "subject": "agent.task.$entity.task_id.editor",
    "role": "editor",
    "model": "$entity.model",
    "prompt": "Implement the following architecture specification:\n\n$entity.result"
  }]
}
```

### Beyond Architect → Editor

For **simple chains** (no loops), rules are sufficient:

```json
{
  "id": "editor_complete_spawn_reviewer",
  "description": "When editor completes, spawn reviewer",
  "conditions": [
    {"field": "role", "operator": "eq", "value": "editor"},
    {"field": "outcome", "operator": "eq", "value": "success"}
  ],
  "on_enter": [{
    "type": "publish_agent",
    "role": "reviewer",
    "prompt": "Review the implementation:\n\n$entity.result"
  }]
}
```

For **loops with limits** (reviewer → fixer → reviewer...), use the workflow processor:

```json
{
  "id": "review_fix_cycle",
  "description": "Review and fix until clean or max attempts",
  "max_iterations": 3,
  "timeout": "300s",
  "steps": [
    {
      "id": "review",
      "action": {"type": "publish_agent", "role": "reviewer"},
      "on_complete": {
        "condition": {"field": "issues_found", "operator": "eq", "value": 0},
        "then": "complete",
        "else": "fix"
      }
    },
    {
      "id": "fix",
      "action": {"type": "publish_agent", "role": "fixer"},
      "on_complete": "review"
    }
  ]
}
```

**Why workflows for loops?** The reviewer → fixer → reviewer pattern can loop indefinitely.
Rules cannot track iteration counts or enforce loop limits. Workflows own this state.

## Consequences

### Positive

- **Right Tool for Each Pattern**: Simple handoffs use rules; complex workflows use workflow processor
- **Configurable**: Agent orchestration defined in JSON, not code
- **Hot-Reloadable**: Both rules and workflows reload without redeployment
- **Clear Boundaries**: See [Orchestration Layers](../concepts/12-orchestration-layers.md) for separation
- **Loop Safety**: Workflow processor enforces iteration limits and timeouts
- **Extensible**: New patterns without code changes
- **Observable**: Both rules and workflows emit metrics and audit trails

### Negative

- **Two Systems**: Developers must understand when to use rules vs. workflows
- **Requires Enriched Events**: Completion events must include role, result, model
- **KV Write Overhead**: Additional KV write per completion (mitigated by batching)
- **Variable Substitution Gaps**: Need to extend `substituteVariables()` for entity fields
- **Migration Effort**: Must migrate hardcoded logic to rules/workflows

### Neutral

- **Minimal Workflow Processor**: Only core features needed initially (see ADR-011)
- **Hybrid Period**: Both patterns may coexist during migration
- **Documentation**: Clear concept doc guides correct usage

## Implementation Requirements (for future work)

### Phase 1: Enrich Completion Events

Modify `handleCompleteResponse` in `handlers.go` to include orchestration data:

```go
completion := map[string]any{
    "loop_id":     loopID,
    "task_id":     entity.TaskID,
    "outcome":     "success",
    "role":        entity.Role,           // NEW
    "result":      responseContent,       // NEW
    "model":       entity.Model,          // NEW
    "iterations":  entity.Iterations,     // NEW
    "parent_loop": entity.ParentLoopID,   // NEW
}
```

### Phase 2: Write Completion State to KV

Add KV write after NATS publish for rules engine to watch:

```go
key := fmt.Sprintf("LOOP_%s", loopID)
kvData, _ := json.Marshal(completion)
c.natsClient.KVPut(ctx, "AGENT_LOOPS", key, kvData)
```

### Phase 3: Extend Variable Substitution

Enhance `substituteVariables()` in `processor/rule/actions.go` to support:

- `$entity.role` - Agent role from completion
- `$entity.result` - Agent output content
- `$entity.model` - Model used for completion
- `$entity.parent_loop` - Parent loop ID for hierarchical workflows

### Phase 4: Create Workflow Rules

Add workflow rule definitions to configuration:

```text
configs/rules/agentic-workflow/
├── architect-editor.json      # architect → editor handoff
├── reviewer-fixer.json        # reviewer → fixer cycle
└── approval-workflow.json     # multi-step approval chains
```

### Phase 5: Remove Hardcoded Logic

Delete from `handlers.go`:

- Lines 320-323 (role check)
- Lines 327-358 (`spawnEditorFromArchitect`)
- `spawnEditorLoop` function

### Files to Modify

| File | Change |
|------|--------|
| `processor/agentic-loop/handlers.go` | Enrich completion events; remove hardcoded handoff |
| `processor/agentic-loop/component.go` | Add KV write for completion state |
| `processor/rule/actions.go` | Extend variable substitution |
| `configs/agentic.json` or `configs/rules/` | Add workflow rules |

### Migration Path

1. **Additive changes first**: Enrich completion events (old consumers ignore new fields)
2. **Create rules in config**: Rules inactive until KV conditions met
3. **Feature flag**: Add config option to disable hardcoded logic
4. **Test in parallel**: Log both rule-based and hardcoded paths
5. **Remove hardcoded**: After rules proven in testing

## Open Questions

1. **Should rules watch NATS subjects directly?** Currently rules only watch KV. Adding subject
   watching would simplify the pattern but increases rules engine scope.

2. **Role validation in `publish_agent`**: Currently hardcoded to "general", "architect", "editor"
   (`actions.go:446`). Should this be configurable via schema?

3. **Workflow-rules interaction**: When a workflow step completes, should it write to KV for rules
   to observe? This enables rules to react to workflow progress without coupling.

## Key Files

| File | Purpose |
|------|---------|
| `processor/agentic-loop/handlers.go` | Contains hardcoded orchestration logic to migrate |
| `processor/rule/actions.go` | `publish_agent` action and `substituteVariables()` |
| `processor/rule/stateful_evaluator.go` | ECA pattern implementation |
| `processor/rule/processor.go` | Rule loading and hot-reload |

## References

- [Orchestration Layers](../concepts/12-orchestration-layers.md) - Three-layer model (rules, workflows, components)
- [ADR-010: Rules Processor Completion](./adr-010-rules-processor-completion.md) - Rules engine design
- [ADR-011: Workflow Processor](./adr-011-workflow-processor.md) - Multi-step workflow patterns (minimal implementation)
- [ADR-016: Agentic Governance Layer](./adr-016-agentic-governance-layer.md) - Related agent infrastructure
- [ADR-017: Graph-Backed Agent Memory](./adr-017-graph-backed-agent-memory.md) - Agent state management
