# Orchestration Layers

SemStreams uses a three-layer orchestration model that separates concerns between reactive rules,
multi-step workflows, and component execution. Understanding these layers and their boundaries is
essential for building maintainable, scalable systems.

## The Three Layers

```text
┌─────────────────────────────────────────────────────────────┐
│  RULES ENGINE (ECA)                                         │
│  "When condition X, trigger action Y"                       │
│  - Watches KV state                                         │
│  - Fires single actions (publish, add_triple, etc.)         │
│  - Can trigger: workflows OR direct component actions       │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼ triggers
┌─────────────────────────────────────────────────────────────┐
│  WORKFLOW PROCESSOR (multi-step orchestration)              │
│  "Execute steps A → B → C with timeouts and loop limits"    │
│  - Owns workflow state (current step, iteration count)      │
│  - Enforces timeouts, loop limits                           │
│  - Spawns work via component subjects                       │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼ spawns
┌─────────────────────────────────────────────────────────────┐
│  COMPONENTS (execution)                                     │
│  - agentic-loop: execute agent turns                        │
│  - graph: process triples                                   │
│  - etc.                                                     │
└─────────────────────────────────────────────────────────────┘
```

### Layer Responsibilities

| Layer | Responsibility | Owns | Does NOT Own |
|-------|---------------|------|--------------|
| **Rules** | React to state, trigger workflows or actions | Conditions, single actions | Multi-step state, timeouts |
| **Workflow** | Multi-step orchestration with state | Step sequence, loop limits, timeouts | Actual work execution |
| **Component** | Execute single units of work | Execution mechanics | Workflow awareness |

## Rules Layer

The rules engine implements the Event-Condition-Action (ECA) pattern. Rules watch for state changes
in NATS KV buckets and fire actions when conditions are met.

### What Rules Do Well

- **State detection**: React when an entity enters a specific state
- **Single actions**: Publish a message, add a triple, update state
- **Triggering workflows**: Start a multi-step process based on conditions
- **Agent handoffs (simple)**: When architect completes, spawn editor

### What Rules Should NOT Do

- **Multi-step coordination**: Rules fire once; they don't track progress through steps
- **Loop management**: Rules have no iteration counters or loop limits
- **Timeout enforcement**: Rules react to state; they don't enforce time bounds
- **Complex branching**: Rules handle if/then; they don't handle if/then/else/retry/timeout

### Example: Simple Agent Handoff

A rule-based handoff works when there's no loop or retry logic needed:

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
    "prompt": "Implement the following architecture:\n\n$entity.result"
  }]
}
```

This is appropriate because:
- Single condition triggers single action
- No iteration tracking needed
- No timeout enforcement required
- Fire-and-forget semantics

## Workflow Layer

The workflow processor handles multi-step orchestration that requires state tracking, loop limits,
and timeouts. It fills the gap between reactive rules and external orchestration systems like Temporal.

### What Workflows Do Well

- **Step sequencing**: Execute A → B → C in order
- **Loop limits**: Stop after N iterations of a cycle
- **Workflow timeouts**: Fail the entire workflow after T seconds
- **Step timeouts**: Fail individual steps that run too long
- **State tracking**: Know which step we're on, how many iterations completed

### What Workflows Should NOT Do

- **Execute work directly**: Workflows spawn components; they don't do the work
- **Complex conditionals**: Use rules for condition evaluation, workflows for sequencing
- **Human tasks**: Not a BPM engine; use external systems for human-in-the-loop

### Example: Review-Fix Cycle with Loop Limit

A workflow is required when there's iteration with a termination condition:

```json
{
  "id": "review_fix_cycle",
  "description": "Review code, fix issues, re-review until clean or max attempts",
  "max_iterations": 3,
  "timeout": "300s",
  "steps": [
    {
      "id": "review",
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.$workflow.id.reviewer",
        "role": "reviewer",
        "prompt": "Review the code for issues"
      },
      "on_complete": {
        "condition": {"field": "issues_found", "operator": "eq", "value": 0},
        "then": "complete",
        "else": "fix"
      }
    },
    {
      "id": "fix",
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.$workflow.id.fixer",
        "role": "fixer",
        "prompt": "Fix the issues:\n\n$steps.review.result"
      },
      "on_complete": "review"
    }
  ]
}
```

This requires a workflow because:
- Multiple steps with ordering
- Loop that can repeat (fix → review → fix → review)
- Loop limit (max 3 iterations)
- Workflow timeout (300 seconds total)
- State tracking (which step, iteration count)

## Component Layer

Components execute single units of work. They are workflow-agnostic — a component doesn't know or
care whether it's being invoked by a rule, a workflow, or directly.

### What Components Do

- **Execute work**: Process messages, call LLMs, write files
- **Manage internal state**: Tool execution, context, pending operations
- **Report results**: Publish completion events with outcomes

### What Components Should NOT Do

- **Know about workflows**: Components are isolated units
- **Coordinate with other components**: That's the workflow's job
- **Enforce cross-step timeouts**: Only per-operation timeouts

### Example: agentic-loop Component

The agentic-loop component executes agent turns:

```text
Input: Task message on agent.task.*
Process: Model calls, tool execution, iteration
Output: Completion event on agent.complete.*
```

The component:
- Manages its own state machine (exploring → planning → executing → complete)
- Enforces per-loop iteration limits and timeouts
- Publishes results without knowing what triggered it or what comes next

## Rules of Thumb

### 1. Rules Trigger, They Don't Orchestrate

A rule should fire one action, not manage a sequence. If you find yourself writing rules that
depend on previous rule firings to build up state, you need a workflow.

**Anti-pattern**: Rule A sets `step=1`, Rule B watches for `step=1` and sets `step=2`...

**Correct**: Rule A triggers a workflow that manages step progression.

### 2. Workflows Coordinate, They Don't Execute

Workflows spawn components; they don't do the work themselves. If your workflow step is doing
complex processing inline, that logic belongs in a component.

**Anti-pattern**: Workflow step contains 100 lines of processing logic.

**Correct**: Workflow step publishes to a component that does the processing.

### 3. Components Are Workflow-Agnostic

A component doesn't know if it's in a workflow or standalone. It receives input, does work,
publishes output. This isolation enables reuse and testing.

**Anti-pattern**: Component checks `workflow_id` to decide behavior.

**Correct**: Component behaves the same regardless of caller.

### 4. State Ownership Is Exclusive

Only one layer owns any piece of state. If both rules and workflows are tracking the same state,
you have a design problem.

| State | Owner |
|-------|-------|
| Trigger conditions | Rules |
| Step progress, iteration count | Workflow |
| Execution state (pending tools, etc.) | Component |

### 5. If It Needs a Loop Limit, It's a Workflow

Simple handoffs (A completes → B starts) use rules. Cycles with termination conditions
(A → B → A → B... until X or N times) use workflows.

**Rule**: `when architect completes → spawn editor`

**Workflow**: `reviewer → fixer → reviewer → fixer... max 3 iterations`

## Use Case Examples

### Semspec-Driven Development

A semspec workflow involves multiple agent passes with iteration limits:

```text
1. Parse semspec for tasks (once)
2. For each task:
   a. Architect designs approach
   b. Editor implements
   c. Reviewer checks
   d. If issues: fix and re-review (max 3 times)
3. Report completion
```

**Layer mapping**:
- **Workflow**: Owns the multi-step process, loop limits, overall timeout
- **Rules**: Can trigger the workflow when semspec is approved
- **Components**: agentic-loop executes each agent role

### Simple Agent Handoff

Architect produces a plan, editor implements it (no retry loop):

```text
1. Architect completes with plan
2. Editor receives plan, implements
3. Done
```

**Layer mapping**:
- **Rules**: `when architect completes → publish_agent editor`
- **Components**: agentic-loop executes architect, then editor

No workflow needed — it's a simple A → B handoff without loops.

### Data Pipeline with Validation

Ingest data, validate, retry if malformed, fail after 3 attempts:

```text
1. Receive data
2. Validate format
3. If invalid and attempts < 3: request correction, goto 2
4. If invalid and attempts >= 3: fail with error
5. If valid: process and store
```

**Layer mapping**:
- **Workflow**: Owns validation loop, attempt counter, timeout
- **Rules**: Can trigger workflow on new data arrival
- **Components**: validator component, storage component

## Debugging Orchestration Issues

### Symptom: Action Fires Multiple Times Unexpectedly

**Likely cause**: Rule is re-triggering because state oscillates.

**Check**: Is the rule's action modifying state that causes the condition to re-match?

**Fix**: Use idempotent state updates or track "already processed" flags.

### Symptom: Workflow Never Completes

**Likely cause**: Missing termination condition or loop limit.

**Check**: Does every loop have a max_iterations? Does every branch eventually reach completion?

**Fix**: Add explicit loop limits and ensure all conditional branches terminate.

### Symptom: Component Behaves Differently in Workflow vs. Standalone

**Likely cause**: Component has workflow awareness it shouldn't.

**Check**: Is the component checking context about its caller?

**Fix**: Remove caller awareness; component behavior should be input-determined only.

## State Storage Boundaries

The system distinguishes between three categories of data with different storage patterns:

| Category | Storage | Rules Observable? | In Knowledge Graph? |
|----------|---------|-------------------|---------------------|
| **Domain Entities** | `ENTITY_STATES` KV | Yes | Yes (Graphable) |
| **Operational Results** | Component-specific KV | Yes | No |
| **Events** | JetStream streams | No | No |

### Domain Entities (ENTITY_STATES)

Semantic domain objects that implement the `Graphable` interface:
- Have 6-part hierarchical entity IDs (e.g., `acme.ops.robotics.gcs.drone.001`)
- Persist across multiple events
- Queryable in knowledge graph
- Examples: sensors, documents, zones, relationships

**Only `graph-ingest` writes to ENTITY_STATES.**

### Operational Results (Component KV)

Execution outcomes that are NOT semantic entities:
- Use `COMPLETE_{id}` key pattern for rules observability
- Stored in component-specific buckets:
  - `AGENT_LOOPS`: Agent completion state (`COMPLETE_{loopID}`)
  - `WORKFLOW_EXECUTIONS`: Workflow completion state (`COMPLETE_{executionID}`)
- Transient — represent what happened, not what exists
- Examples: agent completion, workflow completion, step results

**Rules can watch these buckets using `entity_watch_buckets` config:**

```json
{
  "entity_watch_buckets": {
    "ENTITY_STATES": ["telemetry.>"],
    "WORKFLOW_EXECUTIONS": ["COMPLETE_*"],
    "AGENT_LOOPS": ["COMPLETE_*"]
  }
}
```

### Events (JetStream)

Immediate notifications for downstream processing:
- Published to streams for subscribers
- Not directly observable by rules (rules watch KV, not streams)
- Examples: `agent.complete.*`, `workflow.events`

### Anti-Pattern: Mixing Categories

Do NOT write operational results to ENTITY_STATES:
- Pollutes knowledge graph with non-semantic data
- Breaks Graphable contract (no entity ID, no triples)
- Makes graph queries less meaningful

**Example anti-pattern**:
```go
// WRONG: Writing workflow completion to ENTITY_STATES
entityBucket.Put(ctx, "workflow.review.exec123", completionData)
```

**Correct pattern**:
```go
// RIGHT: Writing to component-specific bucket with COMPLETE_ prefix
executionsBucket.Put(ctx, "COMPLETE_exec123", completionData)
```

## References

- [ADR-010: Rules Processor Completion](../architecture/adr-010-rules-processor-completion.md) — Rules engine design
- [ADR-011: Workflow Processor](../architecture/adr-011-workflow-processor.md) — Workflow processor design
- [ADR-018: Agentic Workflow Orchestration](../architecture/adr-018-agentic-workflow-orchestration.md) — Agent-specific orchestration
- [Concept: Agentic Systems](./11-agentic-systems.md) — Agentic loop fundamentals
