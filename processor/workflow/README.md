# workflow

Multi-step workflow orchestration processor for the agentic processing system.

## Overview

The `workflow` processor orchestrates multi-step agentic patterns that require loops, limits, and timeouts beyond what the rules engine can handle. It manages workflow definitions, executes steps with variable interpolation, tracks execution state, and enforces timeout/iteration constraints.

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
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.review",
        "payload": {
          "role": "reviewer",
          "model": "gpt-4",
          "prompt": "Review the code: ${trigger.payload.code}"
        }
      },
      "on_success": "fix",
      "on_fail": "fail"
    },
    {
      "name": "fix",
      "action": {
        "type": "publish_agent",
        "subject": "agent.task.fix",
        "payload": {
          "role": "editor",
          "model": "gpt-4",
          "prompt": "Fix issues: ${steps.review.output.issues}"
        }
      },
      "condition": {
        "field": "steps.review.output.issues_count",
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
      "payload": {"result": "${steps.fix.output}"}
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

Each step defines an action and transitions:

```json
{
  "name": "process",
  "action": {
    "type": "call",
    "subject": "service.process",
    "payload": {"data": "${trigger.payload.input}"},
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
| `action` | object | yes | Action to execute |
| `condition` | object | no | Condition for execution |
| `on_success` | string | no | Next step on success |
| `on_fail` | string | no | Next step on failure |
| `timeout` | string | no | Step-specific timeout |

## Action Types

### call

NATS request/response with timeout:

```json
{
  "type": "call",
  "subject": "service.request",
  "payload": {"key": "value"},
  "timeout": "30s"
}
```

### publish

Fire-and-forget NATS publish:

```json
{
  "type": "publish",
  "subject": "events.notification",
  "payload": {"message": "Step complete"}
}
```

### publish_agent

Spawn an agentic task:

```json
{
  "type": "publish_agent",
  "subject": "agent.task.analyze",
  "payload": {
    "task_id": "${execution.id}",
    "role": "general",
    "model": "gpt-4",
    "prompt": "Analyze: ${trigger.payload.content}"
  }
}
```

### set_state

Mutate entity state via graph processor:

```json
{
  "type": "set_state",
  "entity": "entity:${trigger.payload.entity_id}",
  "state": {"status": "processed"}
}
```

## Variable Interpolation

Variables use `${path.to.value}` syntax with these roots:

### execution.*

| Path | Description |
|------|-------------|
| `${execution.id}` | Execution ID |
| `${execution.workflow_id}` | Workflow ID |
| `${execution.workflow_name}` | Workflow name |
| `${execution.state}` | Current state |
| `${execution.iteration}` | Current iteration |
| `${execution.current_step}` | Current step index |
| `${execution.current_name}` | Current step name |

### trigger.*

| Path | Description |
|------|-------------|
| `${trigger.subject}` | Trigger subject |
| `${trigger.payload.*}` | Trigger payload fields |
| `${trigger.timestamp}` | Trigger timestamp |
| `${trigger.headers.*}` | Trigger headers |

### steps.*

| Path | Description |
|------|-------------|
| `${steps.{name}.status}` | Step status (success/failed/skipped) |
| `${steps.{name}.output}` | Step output (full object) |
| `${steps.{name}.output.*}` | Step output fields |
| `${steps.{name}.error}` | Step error message |
| `${steps.{name}.duration}` | Step duration |
| `${steps.{name}.iteration}` | Iteration when step ran |

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
