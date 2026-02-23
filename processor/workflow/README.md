# workflow

Multi-step workflow orchestration processor for the agentic processing system.

## Overview

The `workflow` processor orchestrates multi-step agentic patterns that require loops, limits, and timeouts beyond what the rules engine can handle. It manages workflow definitions, executes steps with data references, tracks execution state, and enforces timeout/iteration constraints.

**Note**: This processor uses the unified dataflow pattern from [ADR-020](../../docs/architecture/adr-020-unified-dataflow-patterns.md). Steps declare explicit `inputs` and `outputs` instead of string interpolation with `payload_mapping`.

## Architecture

```
                         ┌─────────────────┐
    workflow.trigger.> ─►│                 │────► agent.task.*
                         │    workflow     │
    workflow.step.     ─►│                 │────► workflow.events
    complete.>           │                 │
                         │                 │
    agent.complete.>   ─►│                 │
                         └────────┬────────┘
                                  │
                         ┌────────┴────────┐
                         │    NATS KV      │
                         │  WORKFLOW_DEFS  │
                         │  WORKFLOW_EXEC  │
                         └─────────────────┘
```

## Features

- **Workflow Definitions**: JSON-based workflow definitions stored in KV
- **Step Sequencing**: Execute steps with success/failure transitions
- **Loop Limits**: Configurable max iterations for loop workflows
- **Timeout Enforcement**: Per-workflow and per-step timeouts
- **Variable Interpolation**: Reference execution, trigger, and step data
- **Conditional Branching**: Execute steps based on conditions
- **Action Types**: call, publish, publish_agent, set_state

## Configuration

```json
{
  "type": "processor",
  "name": "workflow",
  "enabled": true,
  "config": {
    "definitions_bucket": "WORKFLOW_DEFINITIONS",
    "executions_bucket": "WORKFLOW_EXECUTIONS",
    "stream_name": "WORKFLOW",
    "default_timeout": "10m",
    "default_max_iterations": 10,
    "request_timeout": "30s"
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `definitions_bucket` | string | "WORKFLOW_DEFINITIONS" | KV bucket for definitions |
| `executions_bucket` | string | "WORKFLOW_EXECUTIONS" | KV bucket for execution state |
| `stream_name` | string | "WORKFLOW" | JetStream stream name |
| `default_timeout` | string | "10m" | Default workflow timeout |
| `default_max_iterations` | int | 10 | Default max iterations |
| `request_timeout` | string | "30s" | Timeout for call actions |
| `consumer_name_suffix` | string | "" | Consumer name suffix (for testing) |

## Ports

### Inputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| workflow.trigger | jetstream | workflow.trigger.> | Workflow trigger events |
| workflow.step.complete | jetstream | workflow.step.complete.> | Step completion events |
| agent.complete | jetstream | agent.complete.> | Agent completion for tracking |

### Outputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| workflow.events | jetstream | workflow.events | Workflow lifecycle events |
| agent.task | jetstream | agent.task.* | Agent tasks for publish_agent |

### KV Write

| Name | Bucket | Description |
|------|--------|-------------|
| definitions | WORKFLOW_DEFINITIONS | Workflow definitions |
| executions | WORKFLOW_EXECUTIONS | Execution state (7d TTL) |

## Workflow Definition

Workflows are defined in JSON and stored in the WORKFLOW_DEFINITIONS bucket:

```json
{
  "id": "review-fix-loop",
  "name": "Review and Fix Loop",
  "description": "Iterative code review and fix workflow",
  "version": "1.0.0",
  "enabled": true,
  "trigger": {
    "subject": "code.review.requested"
  },
  "steps": [
    {
      "name": "review",
      "inputs": {
        "code": {"from": "trigger.payload.code"}
      },
      "outputs": {
        "issues": {"interface": "review.issues.v1"},
        "issues_count": {}
      },
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.review"
      },
      "on_success": "fix",
      "on_fail": "fail"
    },
    {
      "name": "fix",
      "inputs": {
        "issues": {"from": "review.issues"}
      },
      "outputs": {
        "fixed_code": {}
      },
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.fix"
      },
      "condition": {
        "field": "review.issues_count",
        "operator": "gt",
        "value": 0
      },
      "on_success": "review",
      "on_fail": "fail"
    }
  ],
  "on_complete": [
    {
      "type": "publish",
      "subject": "code.review.complete",
      "inputs": {
        "result": {"from": "fix.fixed_code"}
      }
    }
  ],
  "timeout": "30m",
  "max_iterations": 5
}
```

### Definition Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique workflow identifier |
| `name` | string | yes | Human-readable name |
| `description` | string | no | Workflow description |
| `version` | string | no | Version string |
| `enabled` | bool | yes | Whether workflow is active |
| `trigger` | object | yes | Trigger configuration |
| `steps` | array | yes | Step definitions |
| `on_complete` | array | no | Actions on completion |
| `on_fail` | array | no | Actions on failure |
| `timeout` | string | no | Workflow timeout |
| `max_iterations` | int | no | Max loop iterations |

## Step Definition

Each step defines inputs, outputs, an action, and transitions:

```json
{
  "name": "process",
  "inputs": {
    "data": {"from": "fetch.result"},
    "user_id": {"from": "trigger.payload.user_id"},
    "exec_id": {"from": "execution.id"}
  },
  "outputs": {
    "processed": {"interface": "processor.result.v1"}
  },
  "action": {
    "type": "call",
    "subject": "service.process",
    "timeout": "30s"
  },
  "condition": {
    "field": "trigger.payload.enabled",
    "operator": "eq",
    "value": true
  },
  "on_success": "next_step",
  "on_fail": "fail",
  "timeout": "1m"
}
```

### Step Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique step name |
| `inputs` | object | no | Input declarations with `from` references |
| `outputs` | object | no | Output declarations with optional `interface` types |
| `action` | object | yes | Action to execute |
| `condition` | object | no | Condition for execution |
| `on_success` | string | no | Next step on success |
| `on_fail` | string | no | Next step on failure |
| `timeout` | string | no | Step-specific timeout |

### Inputs and Outputs

Steps use explicit input/output declarations following the unified dataflow pattern from [ADR-020](../../docs/architecture/adr-020-unified-dataflow-patterns.md):

**Inputs** - Reference data from previous steps, trigger, or execution context:

```json
{
  "inputs": {
    "data": {"from": "fetch.result", "interface": "data.response.v1"},
    "user_id": {"from": "trigger.payload.user_id"},
    "exec_id": {"from": "execution.id"}
  }
}
```

**Outputs** - Declare what the step produces:

```json
{
  "outputs": {
    "result": {"interface": "processor.result.v1"},
    "status": {}
  }
}
```

**`from` Reference Syntax**:

| Pattern | Description | Example |
|---------|-------------|---------|
| `step_name.output_name` | Reference another step's output | `"fetch.result"` |
| `step_name.output_name.field` | Deep field reference | `"fetch.result.items"` |
| `trigger.payload.field` | Trigger data reference | `"trigger.payload.user_id"` |
| `execution.id` | Execution context reference | `"execution.id"` |

The `interface` field is optional but enables load-time validation against the PayloadRegistry.

## Action Types

### call

NATS request/response with timeout. Step inputs are assembled into the request payload:

```json
{
  "name": "fetch",
  "inputs": {
    "id": {"from": "trigger.payload.entity_id"}
  },
  "outputs": {
    "data": {"interface": "entity.data.v1"}
  },
  "action": {
    "type": "call",
    "subject": "service.request",
    "timeout": "30s"
  }
}
```

### publish

Fire-and-forget NATS publish. Step inputs are assembled into the message payload:

```json
{
  "name": "notify",
  "inputs": {
    "message": {"from": "process.result.summary"}
  },
  "action": {
    "type": "publish",
    "subject": "events.notification"
  }
}
```

### publish_agent

Spawn an agentic task. Step inputs are assembled into the agent task payload:

```json
{
  "name": "analyze",
  "inputs": {
    "content": {"from": "trigger.payload.content"},
    "task_id": {"from": "execution.id"}
  },
  "action": {
    "type": "publish_agent",
    "subject": "agent.task.analyze"
  }
}
```

### set_state

Mutate entity state via graph processor. Step inputs define the entity and state:

```json
{
  "name": "update_status",
  "inputs": {
    "entity": {"from": "trigger.payload.entity_id"},
    "status": {"value": "processed"}
  },
  "action": {
    "type": "set_state"
  }
}
```

## Data References

Steps reference data using the `from` field in input declarations (see [ADR-020](../../docs/architecture/adr-020-unified-dataflow-patterns.md)):

### Step Outputs

Reference data produced by previous steps:

| Pattern | Description | Example |
|---------|-------------|---------|
| `step_name.output_name` | Named output from step | `"fetch.result"` |
| `step_name.output_name.field` | Deep field access | `"fetch.result.items"` |
| `step_name.output_name.field.nested` | Nested field access | `"fetch.result.data.id"` |

### Trigger Data

Reference data from the workflow trigger:

| Path | Description |
|------|-------------|
| `trigger.subject` | Trigger subject |
| `trigger.payload.{field}` | Trigger payload fields |
| `trigger.timestamp` | Trigger timestamp |
| `trigger.headers.{key}` | Trigger headers |

### Execution Context

Reference workflow execution metadata:

| Path | Description |
|------|-------------|
| `execution.id` | Execution ID |
| `execution.workflow_id` | Workflow ID |
| `execution.workflow_name` | Workflow name |
| `execution.state` | Current state |
| `execution.iteration` | Current iteration |
| `execution.current_step` | Current step index |
| `execution.current_name` | Current step name |

## Condition Operators

| Operator | Description |
|----------|-------------|
| `eq` | Equal |
| `ne` | Not equal |
| `gt` | Greater than |
| `lt` | Less than |
| `gte` | Greater than or equal |
| `lte` | Less than or equal |
| `exists` | Field exists |
| `not_exists` | Field does not exist |

## Execution States

```
pending → running → completed
                 ↘ failed
                 ↘ timed_out
```

| State | Terminal | Description |
|-------|----------|-------------|
| `pending` | No | Execution created, not started |
| `running` | No | Workflow actively executing |
| `completed` | Yes | All steps finished successfully |
| `failed` | Yes | Step failed or error occurred |
| `timed_out` | Yes | Workflow or step timeout exceeded |

## NATS Subjects

| Subject | Direction | Description |
|---------|-----------|-------------|
| `workflow.trigger.{id}` | Subscribe | Start workflow |
| `workflow.step.complete.{exec_id}` | Subscribe | Step completed |
| `workflow.events` | Publish | Lifecycle events |
| `agent.task.*` | Publish | Agent task dispatch |

## KV Buckets

### WORKFLOW_DEFINITIONS

Stores workflow definitions keyed by workflow ID.

### WORKFLOW_EXECUTIONS

Stores execution state with 7-day TTL, keyed by execution ID.

## Workflow Events

Published to `workflow.events`:

```json
{
  "type": "started",
  "execution_id": "exec_123",
  "workflow_id": "review-fix",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

```json
{
  "type": "step_completed",
  "execution_id": "exec_123",
  "workflow_id": "review-fix",
  "step_name": "review",
  "iteration": 1,
  "timestamp": "2024-01-15T10:31:00Z"
}
```

```json
{
  "type": "completed",
  "execution_id": "exec_123",
  "workflow_id": "review-fix",
  "state": "completed",
  "timestamp": "2024-01-15T10:35:00Z"
}
```

## Step Completion Message

Agents/services signal step completion via `workflow.step.complete.{exec_id}`:

```json
{
  "execution_id": "exec_123",
  "step_name": "review",
  "status": "success",
  "output": {"issues_count": 3, "issues": ["..."]},
  "error": ""
}
```

## Troubleshooting

### Workflow not triggering

- Verify workflow definition is in WORKFLOW_DEFINITIONS bucket
- Check `enabled` is true
- Verify trigger subject matches published message
- Check WORKFLOW stream exists

### Step timeout

- Increase step-specific timeout
- Verify target service is responsive
- Check `request_timeout` for call actions

### Max iterations exceeded

- Review loop logic in step transitions
- Increase `max_iterations` if appropriate
- Add condition to break loop

### Variable not interpolating

- Check path syntax: `${root.field.subfield}`
- Verify source data exists (trigger, previous step)
- Check for typos in field names

## Related Components

- [agentic-loop](../agentic-loop/) - Loop orchestration for agents
- [agentic-dispatch](../agentic-dispatch/) - User message routing
- [rules](../rules/) - Simple condition-action patterns
- [agentic types](../../agentic/) - Shared type definitions
