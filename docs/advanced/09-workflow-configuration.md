# Workflow Configuration Reference

Complete reference for workflow processor configuration and definition schemas.

**Note**: This document describes the unified dataflow pattern from [ADR-020](../architecture/adr-020-unified-dataflow-patterns.md). Workflows use explicit `inputs` and `outputs` declarations instead of string interpolation with `payload_mapping`.

## Processor Configuration

### Component Config

```json
{
  "type": "processor",
  "name": "workflow-processor",
  "config": {
    "definitions_bucket": "WORKFLOW_DEFINITIONS",
    "executions_bucket": "WORKFLOW_EXECUTIONS",
    "timers_bucket": "WORKFLOW_TIMERS",
    "secrets_bucket": "WORKFLOW_SECRETS",
    "idempotency_bucket": "WORKFLOW_IDEMPOTENCY",

    "trigger_subject_prefix": "workflow.trigger",
    "timer_subject": "workflow.timer.fire",
    "events_subject": "workflow.events",

    "default_step_timeout": "30s",
    "default_workflow_timeout": "1h",
    "max_concurrent_executions": 100,

    "idempotency": {
      "enabled": true,
      "window": "1h"
    },

    "retry_defaults": {
      "initial_backoff": "1s",
      "max_backoff": "1m",
      "multiplier": 2.0
    }
  }
}
```

### Configuration Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `definitions_bucket` | string | `WORKFLOW_DEFINITIONS` | KV bucket for workflow definitions |
| `executions_bucket` | string | `WORKFLOW_EXECUTIONS` | KV bucket for execution state |
| `timers_bucket` | string | `WORKFLOW_TIMERS` | KV bucket for scheduled timers |
| `secrets_bucket` | string | `WORKFLOW_SECRETS` | KV bucket for encrypted secrets |
| `idempotency_bucket` | string | `WORKFLOW_IDEMPOTENCY` | KV bucket for idempotency keys |
| `trigger_subject_prefix` | string | `workflow.trigger` | NATS subject prefix for triggers |
| `timer_subject` | string | `workflow.timer.fire` | NATS subject for timer events |
| `events_subject` | string | `workflow.events` | NATS subject for execution events |
| `default_step_timeout` | duration | `30s` | Default timeout per step |
| `default_workflow_timeout` | duration | `1h` | Default overall timeout |
| `max_concurrent_executions` | int | `100` | Maximum concurrent executions |
| `idempotency.enabled` | bool | `true` | Enable duplicate detection |
| `idempotency.window` | duration | `1h` | Deduplication window |

### NATS Bucket Configuration

| Bucket | TTL | Purpose |
|--------|-----|---------|
| `WORKFLOW_DEFINITIONS` | None | Workflow definitions (persistent) |
| `WORKFLOW_EXECUTIONS` | 7d | Execution state and history |
| `WORKFLOW_TIMERS` | None | Scheduled timers |
| `WORKFLOW_SECRETS` | None | Encrypted secrets |
| `WORKFLOW_IDEMPOTENCY` | 24h | Idempotency keys (auto-expire) |

## Workflow Definition Schema

### Complete Structure

```json
{
  "id": "string",
  "name": "string",
  "description": "string",
  "version": "string",
  "enabled": true,

  "trigger": {
    "rule": "string",
    "subject": "string",
    "cron": "string",
    "manual": false
  },

  "input": {
    "type": "object",
    "properties": {},
    "required": []
  },

  "steps": [
    {
      "name": "string",
      "description": "string",
      "action": {},
      "on_success": "string",
      "on_fail": "string",
      "retry": {},
      "timeout": "string",
      "condition": {}
    }
  ],

  "on_complete": [],
  "on_fail": [],

  "timeout": "string",
  "max_iterations": 10,
  "metadata": {}
}
```

### Field Reference

#### Root Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique identifier (lowercase, alphanumeric, hyphens) |
| `name` | string | Yes | Human-readable name |
| `description` | string | No | Purpose and behavior description |
| `version` | string | No | Semantic version (e.g., "1.0.0") |
| `enabled` | bool | No | Accept new triggers (default: true) |
| `trigger` | object | Yes | Trigger configuration |
| `input` | object | No | Input validation schema |
| `steps` | array | Yes | Step definitions (min 1) |
| `on_complete` | array | No | Actions on successful completion |
| `on_fail` | array | No | Actions on failure |
| `timeout` | duration | No | Overall workflow timeout |
| `max_iterations` | int | No | Maximum loop iterations |
| `metadata` | object | No | Custom metadata |

#### Trigger Configuration

Exactly one trigger type must be specified:

| Field | Type | Description |
|-------|------|-------------|
| `rule` | string | Rule ID that triggers workflow |
| `subject` | string | NATS subject that triggers workflow |
| `cron` | string | Cron expression for scheduled execution |
| `manual` | bool | Only triggered via API |

**Cron Expression Format**:
```
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of week (0 - 7, 0 and 7 are Sunday)
│ │ │ │ │
* * * * *
```

Examples:
- `0 9 * * *` - Daily at 9:00 AM
- `*/15 * * * *` - Every 15 minutes
- `0 0 * * 0` - Weekly on Sunday at midnight

#### Step Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Step identifier (unique within workflow) |
| `description` | string | No | Step purpose |
| `inputs` | object | No | Input declarations with `from` references |
| `outputs` | object | No | Output declarations with optional `interface` types |
| `action` | object | Yes | Action to execute |
| `on_success` | string | No | Next step, "next", or "complete" |
| `on_fail` | string | No | Step name or "abort" |
| `retry` | object | No | Retry policy |
| `timeout` | duration | No | Step timeout |
| `condition` | object | No | Skip condition |

#### Inputs and Outputs (ADR-020)

Steps use explicit input/output declarations following the unified dataflow pattern from [ADR-020](../architecture/adr-020-unified-dataflow-patterns.md):

**Input Declaration:**

```json
{
  "inputs": {
    "data": {"from": "fetch.result", "interface": "data.response.v1"},
    "user_id": {"from": "trigger.payload.user_id"},
    "exec_id": {"from": "execution.id"}
  }
}
```

**Output Declaration:**

```json
{
  "outputs": {
    "result": {"interface": "processor.result.v1"},
    "status": {},
    "metadata": {"interface": "common.metadata.v1"}
  }
}
```

**`from` Reference Patterns:**

| Pattern | Description | Example |
|---------|-------------|---------|
| `step_name.output_name` | Reference another step's output | `"fetch.result"` |
| `step_name.output_name.field` | Deep field reference | `"fetch.result.items"` |
| `trigger.payload.field` | Trigger data reference | `"trigger.payload.user_id"` |
| `execution.field` | Execution context reference | `"execution.id"` |

The `interface` field is optional but enables:
- Load-time validation against the PayloadRegistry
- Type reconstruction for downstream steps
- Self-documenting step contracts

#### Retry Policy

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | int | 1 | Maximum retry attempts |
| `initial_backoff` | duration | 1s | Initial delay between retries |
| `max_backoff` | duration | 1m | Maximum delay between retries |
| `multiplier` | float | 2.0 | Backoff multiplier |

Backoff calculation: `delay = min(initial_backoff * multiplier^(attempt-1), max_backoff)`

#### Condition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `field` | string | Yes | JSONPath into execution context |
| `operator` | string | Yes | Comparison operator |
| `value` | any | Depends | Value to compare against |

**Operators**:

| Operator | Description | Value Required |
|----------|-------------|----------------|
| `eq` | Equals | Yes |
| `ne` | Not equals | Yes |
| `gt` | Greater than | Yes |
| `gte` | Greater than or equal | Yes |
| `lt` | Less than | Yes |
| `lte` | Less than or equal | Yes |
| `contains` | String contains | Yes |
| `exists` | Field exists | No |

## Action Types

### call

Request/response NATS call. Step inputs are assembled into the request payload:

```json
{
  "name": "process",
  "inputs": {
    "id": {"from": "trigger.entity_id"},
    "data": {"from": "fetch.result"}
  },
  "outputs": {
    "result": {"interface": "service.result.v1"}
  },
  "action": {
    "type": "call",
    "subject": "service.action"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "call" |
| `subject` | string | Yes | NATS subject to call |

**Behavior**: Assembles payload from step inputs, sends request, waits for response. Response becomes step output. Non-response or error = step failure.

### publish

Fire-and-forget NATS publish. Step inputs are assembled into the message payload:

```json
{
  "name": "notify",
  "inputs": {
    "workflow": {"from": "execution.workflow_id"},
    "step": {"value": "extract-tasks"},
    "result": {"from": "extract.tasks"}
  },
  "action": {
    "type": "publish",
    "subject": "events.workflow.step-complete"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "publish" |
| `subject` | string | Yes | NATS subject |

**Behavior**: Assembles payload from step inputs, publishes immediately. No response expected. Step output: `{"published": true}`.

### publish_agent

Spawn agentic loop task. Step inputs are assembled into the agent task payload:

```json
{
  "name": "review",
  "inputs": {
    "code": {"from": "load-code.output"},
    "role": {"value": "reviewer"},
    "model": {"value": "gpt-4"}
  },
  "outputs": {
    "review_result": {"interface": "agent.result.v1"}
  },
  "action": {
    "type": "publish_agent",
    "subject": "agent.task.reviewer"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "publish_agent" |
| `subject` | string | Yes | Agent task subject |

**Behavior**: Assembles task payload from step inputs (role, model, prompt, etc.). Publishes task message to agentic-loop. Waits for completion on `agent.complete.*`. Step output includes agent result.

### set_state

Update entity state via graph processor. Step inputs define the entity and state:

```json
{
  "name": "update_status",
  "inputs": {
    "entity_id": {"from": "trigger.entity_id"},
    "predicate": {"value": "workflow.status"},
    "object": {"value": "in-progress"}
  },
  "action": {
    "type": "set_state"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "set_state" |

**Behavior**: Reads `entity_id`, `predicate`, and `object` from step inputs. Publishes triple update to graph processor. Waits for confirmation. Step output is the triple.

### http

External HTTP request. Step inputs are assembled into the request body and headers:

```json
{
  "name": "call_api",
  "inputs": {
    "data": {"from": "prepare.output"},
    "auth_token": {"from": "secrets.api_token"}
  },
  "outputs": {
    "response": {"interface": "http.response.v1"}
  },
  "action": {
    "type": "http",
    "method": "POST",
    "url": "https://api.example.com/action",
    "headers": {
      "Authorization": "Bearer {{auth_token}}",
      "Content-Type": "application/json"
    }
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "http" |
| `method` | string | Yes | HTTP method (GET, POST, PUT, DELETE) |
| `url` | string | Yes | Request URL |
| `headers` | object | No | Request headers (supports `{{input_name}}` placeholders) |

**Behavior**: Assembles request body from step inputs. Headers support `{{input_name}}` placeholders for resolved inputs. Makes HTTP request. Response body becomes step output. Non-2xx status = step failure.

### wait

Pause execution:

```json
{
  "type": "wait",
  "duration": "5m"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "wait" |
| `duration` | duration | Yes | Wait duration |

**Behavior**: Schedules timer, execution becomes "waiting". Resumes when timer fires. Step output: `{"waited": "5m"}`.

### tool_batch

Execute multiple tools concurrently:

```json
{
  "type": "tool_batch",
  "tools": [
    "query_entity:drone.001",
    "query_entity:drone.002",
    "query_entity:mission.current"
  ],
  "fail_fast": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "tool_batch" |
| `tools` | array | Yes | Tool invocations (name:args format) |
| `fail_fast` | bool | No | Stop on first failure (default: false) |

**Behavior**: Executes all tools concurrently. Graph tools (query_entity) are automatically batched into a single query. Step output includes all tool results.

### graph_query

Batch graph query for entities and relationships:

```json
{
  "type": "graph_query",
  "entities": ["${trigger.entity_id}", "related.entity.001"],
  "relationships": true,
  "depth": 2
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be "graph_query" |
| `entities` | array | Yes | Entity IDs to query |
| `relationships` | bool | No | Include relationships (default: false) |
| `depth` | int | No | Relationship traversal depth (default: 1) |

**Behavior**: Queries multiple entities efficiently. Returns JSON with entities map and relationships array. Step output includes token count estimate.

## Parallel Steps

Parallel steps execute multiple nested steps concurrently and aggregate their results.

### Parallel Step Configuration

```json
{
  "name": "parallel_review",
  "type": "parallel",
  "steps": [
    {
      "name": "security_review",
      "action": {"type": "publish_agent", "role": "security", "prompt": "..."}
    },
    {
      "name": "style_review",
      "action": {"type": "publish_agent", "role": "style", "prompt": "..."}
    }
  ],
  "wait": "all",
  "aggregator": "union"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Step identifier |
| `type` | string | Yes | Must be "parallel" |
| `steps` | array | Yes | Nested steps to execute concurrently |
| `wait` | string | No | Wait semantics: "all", "any", "majority" (default: "all") |
| `aggregator` | string | No | Result aggregator (default: "union") |

### Wait Semantics

| Wait | Behavior |
|------|----------|
| `all` | Wait for all nested steps to complete |
| `any` | Continue when first step succeeds |
| `majority` | Wait for >50% to complete |

### Depth Tracking

Parallel steps that spawn agents can track recursion depth:

```json
{
  "action": {
    "type": "publish_agent",
    "max_depth": 3
  }
}
```

When an agent at `depth == max_depth` tries to spawn a sub-agent, the spawn is rejected.

## Result Aggregation

Aggregators combine results from parallel steps into a single output.

### Built-in Aggregators

| Aggregator | Success Condition | Output Format |
|------------|-------------------|---------------|
| `union` | All succeed | Array of all outputs |
| `first` | Any succeed | First successful output |
| `majority` | >50% succeed | Array of successful outputs |
| `merge` | Any succeed | Deep-merged JSON object |
| `entity_merge` | Any succeed | Entity-keyed merged object |

### Aggregator Examples

**union** - Combine all outputs:

```json
// Input: [{"score": 8}, {"score": 9}]
// Output: [{"score": 8}, {"score": 9}]
```

**merge** - Deep merge JSON objects:

```json
// Input: [{"a": 1}, {"b": 2}]
// Output: {"a": 1, "b": 2}
```

**entity_merge** - Deduplicate by entity:

```json
// Input: [{"entity_id": "x", "score": 8}, {"entity_id": "x", "style": "ok"}]
// Output: {"entities": {"x": {"entity_id": "x", "score": 8, "style": "ok"}}}
```

## Data References (ADR-020)

Workflows use the unified dataflow pattern from [ADR-020](../architecture/adr-020-unified-dataflow-patterns.md). Data is referenced via `from` fields in step inputs rather than string interpolation.

### Available References

**Step Outputs:**

| Pattern | Description |
|---------|-------------|
| `step_name.output_name` | Named output from step |
| `step_name.output_name.field` | Deep field access |

**Trigger Data:**

| Reference | Description |
|-----------|-------------|
| `trigger.entity_id` | Entity that triggered workflow |
| `trigger.payload` | Full trigger payload object |
| `trigger.payload.field` | Field from trigger payload |
| `trigger.type` | Trigger type (rule, subject, cron, manual) |
| `trigger.source` | Trigger source (rule ID, subject, cron expression) |

**Execution Context:**

| Reference | Description |
|-----------|-------------|
| `execution.id` | Current execution ID |
| `execution.workflow_id` | Workflow definition ID |
| `execution.error` | Error message (in on_fail) |
| `execution.iteration` | Current iteration count |

**Special Values:**

| Reference | Description |
|-----------|-------------|
| `timestamp` | Current ISO timestamp |
| `uuid` | Generate new UUID |
| `secrets.name` | Resolved secret value |

### Reference Examples

```json
{
  "name": "process",
  "inputs": {
    "entity": {"from": "trigger.entity_id"},
    "data": {"from": "fetch-data.items"},
    "timestamp": {"from": "timestamp"},
    "request_id": {"from": "uuid"}
  },
  "outputs": {
    "result": {"interface": "service.result.v1"}
  },
  "action": {
    "type": "call",
    "subject": "service.process"
  }
}
```

### Secrets

Secrets are stored in `WORKFLOW_SECRETS` bucket and resolved at execution time:

```json
{
  "type": "http",
  "headers": {
    "Authorization": "Bearer ${secrets.github_token}"
  }
}
```

**Security**: Secrets are never logged or persisted to execution state.

## Execution States

| State | Terminal | Description |
|-------|----------|-------------|
| `pending` | No | Created, not yet started |
| `running` | No | Actively executing steps |
| `waiting` | No | Waiting for timer/callback |
| `completed` | Yes | Successfully finished |
| `failed` | Yes | Failed due to error |
| `cancelled` | Yes | Cancelled by user |
| `timed_out` | Yes | Exceeded workflow timeout |

## Events

Published to `workflow.events`:

### execution.started

```json
{
  "type": "execution.started",
  "execution_id": "exec-abc123",
  "workflow_id": "review-fix-cycle",
  "trigger": {
    "type": "subject",
    "source": "workflow.trigger.review",
    "entity_id": "test-123"
  },
  "timestamp": "2025-01-01T12:00:00Z"
}
```

### step.completed

```json
{
  "type": "step.completed",
  "execution_id": "exec-abc123",
  "step": "review",
  "output": { "issues_found": 2 },
  "duration_ms": 1500,
  "attempts": 1,
  "timestamp": "2025-01-01T12:00:01.500Z"
}
```

### step.failed

```json
{
  "type": "step.failed",
  "execution_id": "exec-abc123",
  "step": "create-issues",
  "error": "connection timeout",
  "attempts": 3,
  "timestamp": "2025-01-01T12:00:30Z"
}
```

### execution.completed

```json
{
  "type": "execution.completed",
  "execution_id": "exec-abc123",
  "workflow_id": "review-fix-cycle",
  "duration_ms": 5230,
  "timestamp": "2025-01-01T12:00:05.230Z"
}
```

### execution.failed

```json
{
  "type": "execution.failed",
  "execution_id": "exec-abc123",
  "workflow_id": "review-fix-cycle",
  "failed_step": "create-issues",
  "error": "max retries exceeded",
  "duration_ms": 35000,
  "timestamp": "2025-01-01T12:00:35Z"
}
```

## Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `workflow_executions_total` | counter | workflow_id, status | Total executions |
| `workflow_execution_duration_seconds` | histogram | workflow_id, status | Execution duration |
| `workflow_executions_active` | gauge | workflow_id | Active executions |
| `workflow_steps_total` | counter | workflow_id, step, status | Total steps |
| `workflow_step_duration_seconds` | histogram | workflow_id, step | Step duration |
| `workflow_step_retries_total` | counter | workflow_id, step | Retry attempts |
| `workflow_timers_active` | gauge | - | Scheduled timers |
| `workflow_timers_fired_total` | counter | type | Timers fired |
| `workflow_duplicate_triggers_total` | counter | workflow_id | Deduplicated triggers |

## Example Workflows

### Spec Approval Pipeline

```json
{
  "id": "spec-approval",
  "name": "Spec Approval Workflow",
  "description": "Creates GitHub issues when a spec is approved",
  "version": "1.0.0",
  "enabled": true,

  "trigger": {
    "rule": "spec-approved-trigger"
  },

  "steps": [
    {
      "name": "load-spec",
      "inputs": {
        "id": {"from": "trigger.entity_id"}
      },
      "outputs": {
        "spec": {"interface": "spec.data.v1"}
      },
      "action": {
        "type": "call",
        "subject": "spec.get"
      },
      "timeout": "10s"
    },
    {
      "name": "extract-tasks",
      "inputs": {
        "spec": {"from": "load-spec.spec"}
      },
      "outputs": {
        "tasks": {"interface": "spec.tasks.v1"}
      },
      "action": {
        "type": "call",
        "subject": "spec.extract-tasks"
      },
      "condition": {
        "field": "load-spec.spec.has_tasks",
        "operator": "eq",
        "value": true
      }
    },
    {
      "name": "create-issues",
      "inputs": {
        "tasks": {"from": "extract-tasks.tasks"},
        "github_token": {"from": "secrets.github_token"}
      },
      "outputs": {
        "issue_ids": {}
      },
      "action": {
        "type": "http",
        "method": "POST",
        "url": "https://api.github.com/repos/org/repo/issues",
        "headers": {
          "Authorization": "Bearer {{github_token}}"
        }
      },
      "retry": {
        "max_attempts": 3,
        "initial_backoff": "5s"
      },
      "on_fail": "mark-blocked"
    },
    {
      "name": "mark-blocked",
      "inputs": {
        "entity_id": {"from": "trigger.entity_id"},
        "predicate": {"value": "spec.status"},
        "object": {"value": "blocked"}
      },
      "action": {
        "type": "set_state"
      },
      "on_success": "complete"
    }
  ],

  "on_complete": [
    {
      "type": "set_state",
      "inputs": {
        "entity_id": {"from": "trigger.entity_id"},
        "predicate": {"value": "spec.status"},
        "object": {"value": "implementing"}
      }
    }
  ],

  "timeout": "5m"
}
```

### Daily Report

```json
{
  "id": "daily-report",
  "name": "Daily Progress Report",
  "version": "1.0.0",
  "enabled": true,

  "trigger": {
    "cron": "0 9 * * *"
  },

  "steps": [
    {
      "name": "collect-metrics",
      "inputs": {
        "period": {"value": "24h"}
      },
      "outputs": {
        "metrics": {"interface": "metrics.data.v1"}
      },
      "action": {
        "type": "call",
        "subject": "metrics.collect"
      }
    },
    {
      "name": "generate-summary",
      "inputs": {
        "template": {"value": "daily-progress"},
        "data": {"from": "collect-metrics.metrics"}
      },
      "outputs": {
        "summary": {}
      },
      "action": {
        "type": "call",
        "subject": "report.generate"
      }
    },
    {
      "name": "send-notification",
      "inputs": {
        "channel": {"value": "#engineering"},
        "text": {"from": "generate-summary.summary"},
        "slack_token": {"from": "secrets.slack_token"}
      },
      "action": {
        "type": "http",
        "method": "POST",
        "url": "https://slack.com/api/chat.postMessage",
        "headers": {
          "Authorization": "Bearer {{slack_token}}"
        }
      },
      "retry": {
        "max_attempts": 3
      }
    }
  ],

  "timeout": "10m"
}
```

## Related Documentation

- [Workflow Quickstart](../basics/08-workflow-quickstart.md) — Getting started
- [Orchestration Layers](../concepts/12-orchestration-layers.md) — Rules vs. workflows
- [Parallel Agents](../concepts/23-parallel-agents.md) — Parallel execution patterns
- [Context Construction](../concepts/22-context-construction.md) — Building agent context
- [Agentic Components](08-agentic-components.md) — Agent integration
- [Aggregation Package](../../processor/workflow/aggregation/README.md) — Aggregator details
- [Workflow Processor Spec](../architecture/specs/workflow-processor-spec.md) — Full specification
