# Workflow Quickstart

Get started with SemStreams workflow orchestration for multi-step processes.

## What are Workflows?

Workflows are declarative definitions for multi-step processes that need:

- **Sequential execution** with state between steps
- **Loop limits** to prevent runaway processes
- **Timeouts** at step and workflow levels
- **Retry logic** with configurable backoff

Workflows fill the gap between reactive rules (stateless, event-driven) and external orchestration systems like Temporal.

## When to Use Workflows

| Pattern | Use |
|---------|-----|
| A completes → B starts (no retry) | **Rules** - simple handoff |
| A → B → A → B... (max N times) | **Workflows** - loop with limit |
| Execute LLM call, process tools | **Components** - execution mechanics |

**Quick decision**: If it needs a loop limit, it's a workflow.

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  RULES ENGINE (ECA)                                         │
│  "When condition X, trigger workflow Y"                     │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼ triggers
┌─────────────────────────────────────────────────────────────┐
│  WORKFLOW PROCESSOR (multi-step orchestration)              │
│  "Execute steps A → B → C with timeouts and loop limits"    │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼ spawns
┌─────────────────────────────────────────────────────────────┐
│  COMPONENTS (execution)                                     │
│  - agentic-loop: execute agent turns                        │
│  - graph: process triples                                   │
└─────────────────────────────────────────────────────────────┘
```

## Your First Workflow

### Example: Review-Fix Cycle

A code review workflow that loops between reviewer and fixer agents:

```json
{
  "id": "review-fix-cycle",
  "name": "Review and Fix Cycle",
  "description": "Review code, fix issues, re-review until clean or max attempts",
  "version": "1.0.0",
  "enabled": true,

  "trigger": {
    "subject": "workflow.trigger.review"
  },

  "steps": [
    {
      "name": "review",
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.reviewer",
        "role": "reviewer",
        "prompt": "Review the code for issues"
      },
      "on_complete": {
        "condition": {"field": "issues_found", "operator": "eq", "value": 0},
        "then": "complete",
        "else": "fix"
      },
      "timeout": "60s"
    },
    {
      "name": "fix",
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.fixer",
        "role": "fixer",
        "prompt": "Fix the issues:\n\n${steps.review.result}"
      },
      "on_complete": "review"
    }
  ],

  "max_iterations": 3,
  "timeout": "300s"
}
```

**Why this needs a workflow**:
- Loop between review and fix steps
- Maximum 3 iterations to prevent infinite loops
- Overall timeout of 5 minutes
- Step results passed between iterations

## Workflow Definition Schema

### Core Structure

```json
{
  "id": "workflow-id",          // Unique identifier
  "name": "Human Name",         // Display name
  "description": "What it does",
  "version": "1.0.0",
  "enabled": true,

  "trigger": { ... },           // What starts the workflow
  "input": { ... },             // Expected input schema (optional)
  "steps": [ ... ],             // Ordered step definitions

  "on_complete": [ ... ],       // Actions on success
  "on_fail": [ ... ],           // Actions on failure

  "timeout": "5m",              // Overall workflow timeout
  "max_iterations": 10,         // Loop limit
  "metadata": { ... }           // Custom metadata
}
```

### Trigger Types

```json
// Rule-triggered (when a rule fires)
{ "trigger": { "rule": "spec-approved-trigger" } }

// Subject-triggered (on NATS message)
{ "trigger": { "subject": "workflow.trigger.review" } }

// Scheduled (cron expression)
{ "trigger": { "cron": "0 9 * * *" } }

// Manual (API only)
{ "trigger": { "manual": true } }
```

### Step Definition

```json
{
  "name": "step-name",
  "description": "What this step does",

  "action": {
    "type": "call",
    "subject": "service.action",
    "payload": { "key": "${trigger.entity_id}" }
  },

  "on_success": "next-step",     // Or "next", "complete"
  "on_fail": "error-handler",    // Or "abort"

  "retry": {
    "max_attempts": 3,
    "initial_backoff": "1s",
    "max_backoff": "30s",
    "multiplier": 2.0
  },

  "timeout": "30s",

  "condition": {                 // Skip if false
    "field": "steps.previous.output.skip",
    "operator": "eq",
    "value": false
  }
}
```

## Action Types

### call (Request/Response)

Executes a NATS request and waits for response:

```json
{
  "type": "call",
  "subject": "service.get-data",
  "payload": {
    "id": "${trigger.entity_id}"
  }
}
```

### publish (Fire-and-Forget)

Publishes a message without waiting:

```json
{
  "type": "publish",
  "subject": "events.workflow.step-complete",
  "payload": {
    "step": "extract-tasks",
    "timestamp": "${timestamp}"
  }
}
```

### publish_agent (Spawn Agent)

Spawns an agentic loop task:

```json
{
  "type": "publish_agent",
  "subject": "agent.task.reviewer",
  "role": "reviewer",
  "model": "gpt-4",
  "prompt": "Review the following code:\n\n${steps.load-code.output}"
}
```

### set_state (Entity Mutation)

Updates entity state via graph processor:

```json
{
  "type": "set_state",
  "entity_id": "${trigger.entity_id}",
  "predicate": "workflow.status",
  "object": "in-progress"
}
```

## Variable Interpolation

Use `${...}` syntax to reference dynamic values:

| Variable | Description | Example |
|----------|-------------|---------|
| `${trigger.entity_id}` | Entity that triggered workflow | `acme.specs.auth` |
| `${trigger.payload.X}` | Field from trigger payload | `${trigger.payload.status}` |
| `${steps.X.output}` | Output from completed step | `${steps.load-spec.output}` |
| `${steps.X.output.field}` | Field from step output | `${steps.extract.output.tasks}` |
| `${execution.id}` | Current execution ID | `exec-abc123` |
| `${timestamp}` | Current ISO timestamp | `2025-01-01T12:00:00Z` |

### Example

```json
{
  "action": {
    "type": "call",
    "subject": "github.issues.create",
    "payload": {
      "spec_id": "${trigger.entity_id}",
      "tasks": "${steps.extract-tasks.output.tasks}",
      "created_at": "${timestamp}"
    }
  }
}
```

## Error Handling

### Step-Level Retry

```json
{
  "name": "create-issues",
  "action": { ... },
  "retry": {
    "max_attempts": 3,
    "initial_backoff": "5s",
    "multiplier": 2.0,
    "max_backoff": "1m"
  }
}
```

### Failure Routing

```json
{
  "name": "risky-step",
  "action": { ... },
  "on_fail": "error-handler"
},
{
  "name": "error-handler",
  "action": {
    "type": "set_state",
    "entity_id": "${trigger.entity_id}",
    "predicate": "workflow.status",
    "object": "failed"
  },
  "on_success": "complete"
}
```

### Workflow-Level Handlers

```json
{
  "on_complete": [
    {
      "type": "set_state",
      "entity_id": "${trigger.entity_id}",
      "predicate": "workflow.status",
      "object": "completed"
    }
  ],
  "on_fail": [
    {
      "type": "publish",
      "subject": "alerts.workflow.failed",
      "payload": {
        "workflow": "review-fix-cycle",
        "error": "${execution.error}"
      }
    }
  ]
}
```

## Observability

### NATS KV Storage

```bash
# List workflow executions
nats kv list WORKFLOW_EXECUTIONS

# Get execution state
nats kv get WORKFLOW_EXECUTIONS exec-abc123

# Watch for new executions
nats kv watch WORKFLOW_EXECUTIONS
```

### Prometheus Metrics

```
workflow_executions_total{workflow_id="review-fix-cycle",status="completed"}
workflow_execution_duration_seconds{workflow_id="..."}
workflow_steps_total{workflow_id="...",step="review",status="completed"}
workflow_step_retries_total{workflow_id="...",step="create-issues"}
```

## Common Patterns

### Conditional Steps

Skip steps based on previous output:

```json
{
  "name": "extract-tasks",
  "action": { ... },
  "condition": {
    "field": "steps.load-spec.output.has_tasks",
    "operator": "eq",
    "value": true
  }
}
```

### Loop with Termination

Loop until condition met or max iterations:

```json
{
  "steps": [
    {
      "name": "check",
      "action": { "type": "call", "subject": "service.check-status" },
      "on_complete": {
        "condition": {"field": "output.ready", "operator": "eq", "value": true},
        "then": "complete",
        "else": "wait"
      }
    },
    {
      "name": "wait",
      "action": { "type": "wait", "duration": "30s" },
      "on_complete": "check"
    }
  ],
  "max_iterations": 10
}
```

### Multi-Agent Pipeline

Chain multiple agents through workflow:

```json
{
  "steps": [
    {
      "name": "architect",
      "action": {
        "type": "publish_agent",
        "role": "architect",
        "prompt": "Design the solution"
      }
    },
    {
      "name": "editor",
      "action": {
        "type": "publish_agent",
        "role": "editor",
        "prompt": "Implement:\n${steps.architect.result}"
      }
    },
    {
      "name": "reviewer",
      "action": {
        "type": "publish_agent",
        "role": "reviewer",
        "prompt": "Review:\n${steps.editor.result}"
      }
    }
  ]
}
```

## Running Workflows

### Manual Trigger

```bash
# Trigger via NATS
nats pub workflow.trigger.review-fix-cycle '{"entity_id": "test-123"}'

# Trigger via HTTP API (if enabled)
curl -X POST http://localhost:8080/api/workflow/review-fix-cycle/trigger \
  -H "Content-Type: application/json" \
  -d '{"entity_id": "test-123"}'
```

### Rule-Triggered

```json
{
  "id": "spec-approved-trigger",
  "conditions": [
    {"field": "spec.status.current", "operator": "eq", "value": "approved"}
  ],
  "on_enter": [
    {
      "type": "publish",
      "subject": "workflow.trigger.spec-approval",
      "payload": {"entity_id": "${entity_id}"}
    }
  ]
}
```

## Debugging

### Check Execution State

```bash
nats kv get WORKFLOW_EXECUTIONS exec-abc123
```

Output:
```json
{
  "id": "exec-abc123",
  "workflow_id": "review-fix-cycle",
  "state": "running",
  "current_step": 2,
  "step_results": {
    "review": {"state": "completed", "output": {...}, "attempts": 1},
    "fix": {"state": "running", "attempts": 1}
  },
  "started_at": "2025-01-01T12:00:00Z"
}
```

### Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| Workflow never completes | Missing termination condition | Add `on_success: "complete"` to final step |
| Step times out | Action taking too long | Increase `timeout` or optimize action |
| Infinite loop | No `max_iterations` | Add `max_iterations` to workflow |
| Variable not resolved | Wrong path | Check `${steps.X.output.Y}` syntax |

## Next Steps

- [Orchestration Layers](../concepts/12-orchestration-layers.md) — When to use rules vs. workflows
- [Workflow Configuration Reference](../advanced/09-workflow-configuration.md) — Complete schema reference
- [Agentic Quickstart](07-agentic-quickstart.md) — LLM-powered agents
- [Troubleshooting](../operations/02-troubleshooting.md) — Common issues and solutions
