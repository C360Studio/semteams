# Workflow Processor Specification

**Version**: 1.0.0
**Status**: Draft
**Last Updated**: 2025-01-01

**Note**: This specification uses the unified dataflow pattern from [ADR-020](../adr-020-unified-dataflow-patterns.md). Steps declare explicit `inputs` and `outputs` instead of string interpolation with `payload_mapping`.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Design Principles](#2-design-principles)
3. [Architecture](#3-architecture)
4. [Data Model](#4-data-model)
5. [Workflow Definition Schema](#5-workflow-definition-schema)
6. [Execution Model](#6-execution-model)
7. [Actions](#7-actions)
8. [Timer Service](#8-timer-service)
9. [API Contract](#9-api-contract)
10. [Configuration](#10-configuration)
11. [Integration Patterns](#11-integration-patterns)
12. [Examples](#12-examples)
13. [Secrets Management](#13-secrets-management)
14. [Idempotency](#14-idempotency)
15. [Observability](#15-observability)
16. [Schema Extension for UI](#16-schema-extension-for-ui)

---

## 1. Overview

### 1.1 Purpose

The Workflow Processor provides durable, multi-step execution for SemStreams. It bridges the gap between reactive rules (stateless, event-driven) and complex orchestration (stateful, sequential, with retry and timeout).

### 1.2 Scope

**In Scope:**
- Sequential step execution
- Retry with exponential backoff
- Step and workflow timeouts
- Durable state (survives restarts)
- Conditional branching (on_success/on_fail)
- Integration with rules engine triggers
- Request/response actions via NATS

**Out of Scope (Extension Territory):**
- Parallel step execution
- Child/nested workflows
- Saga/compensation patterns
- Visual workflow designer (UI concern)
- Full Temporal feature parity

### 1.3 Relationship to Other Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        Component Flow                           │
│                                                                 │
│  Rules Processor                    Workflow Processor          │
│  ┌─────────────────┐               ┌─────────────────┐         │
│  │ Reactive logic  │──triggers────►│ Multi-step      │         │
│  │ State detection │               │ orchestration   │         │
│  │ Event publish   │◄──completes───│ Durable state   │         │
│  └─────────────────┘               └────────┬────────┘         │
│           │                                  │                  │
│           │                                  │                  │
│           ▼                                  ▼                  │
│  ┌─────────────────────────────────────────────────────┐       │
│  │              Graph Processor (Entity State)          │       │
│  └─────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Design Principles

### 2.1 Core Principles

| Principle | Description | Rationale |
|-----------|-------------|-----------|
| **Declarative** | Workflows defined in JSON, not code | Enables AI generation, UI editing |
| **Durable** | Execution state persisted to NATS KV | Survives restarts, auditable |
| **Composable** | Actions are NATS request/response | Any service can implement actions |
| **Explicit** | No hidden behavior or magic | Debuggable, predictable |
| **Tiered** | Works at all tiers (0, 1, 2) | No LLM dependency for core execution |

### 2.2 Non-Goals

- **Not a general orchestrator**: Focused on SemStreams integration patterns
- **Not Temporal replacement**: For complex needs, use Temporal alongside
- **Not a BPM engine**: No human task assignment, forms, etc.
- **Not real-time**: Step latency in milliseconds acceptable, not microseconds

---

## 3. Architecture

### 3.1 Component Structure

```
processor/workflow/
├── processor.go        # Main WorkflowProcessor component
├── registry.go         # WorkflowRegistry for definitions
├── execution.go        # Execution state management
├── executor.go         # Step execution logic
├── timer.go            # Timer service for timeouts
├── actions/            # Built-in action implementations
│   ├── call.go         # NATS request/response
│   ├── publish.go      # Fire-and-forget publish
│   ├── set_state.go    # Entity state mutation
│   └── http.go         # HTTP request (optional)
├── schema.go           # JSON schema definitions
└── register.go         # Component registration
```

### 3.2 Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Workflow Processor                        │
│                                                                  │
│  Inputs:                                                         │
│  ┌──────────────────┐  ┌──────────────────┐                     │
│  │ workflow.trigger │  │ workflow.timer   │                     │
│  │ (start new)      │  │ (timeout fired)  │                     │
│  └────────┬─────────┘  └────────┬─────────┘                     │
│           │                      │                               │
│           ▼                      ▼                               │
│  ┌──────────────────────────────────────────┐                   │
│  │            Execution Engine               │                   │
│  │  ┌────────────┐  ┌────────────────────┐  │                   │
│  │  │ Registry   │  │ Execution Store    │  │                   │
│  │  │ (defs)     │  │ (NATS KV)          │  │                   │
│  │  └────────────┘  └────────────────────┘  │                   │
│  └──────────────────────────────────────────┘                   │
│           │                                                      │
│           ▼                                                      │
│  Outputs:                                                        │
│  ┌──────────────────┐  ┌──────────────────┐                     │
│  │ workflow.events  │  │ Action subjects  │                     │
│  │ (status updates) │  │ (step execution) │                     │
│  └──────────────────┘  └──────────────────┘                     │
└─────────────────────────────────────────────────────────────────┘
```

### 3.3 Storage Layout

```
NATS KV Buckets:

WORKFLOW_DEFINITIONS
├── workflow:{id}              → WorkflowDef JSON
└── workflow:{id}:version      → Version metadata

WORKFLOW_EXECUTIONS (TTL: 7d)
├── exec:{execution_id}        → Execution state JSON
├── exec:{execution_id}:steps  → Step results array
└── exec:{execution_id}:log    → Execution log entries

WORKFLOW_TIMERS
├── timer:{id}                 → Timer definition
└── timer:{id}:fire_at         → Unix timestamp (for indexing)

WORKFLOW_SECRETS
└── secrets:{name}             → Encrypted secret value

WORKFLOW_IDEMPOTENCY (TTL: 24h)
└── idem:{idempotency_key}     → Execution reference
```

---

## 4. Data Model

### 4.1 Workflow Definition

```go
// WorkflowDef is the declarative workflow definition
type WorkflowDef struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description,omitempty"`
    Version     string            `json:"version,omitempty"`
    Enabled     bool              `json:"enabled"`
    
    Trigger     TriggerConfig     `json:"trigger"`
    Input       *InputSchema      `json:"input,omitempty"`
    Steps       []StepDef         `json:"steps"`
    
    OnComplete  []ActionDef       `json:"on_complete,omitempty"`
    OnFail      []ActionDef       `json:"on_fail,omitempty"`
    
    Timeout     string            `json:"timeout,omitempty"`  // Overall workflow timeout
    Metadata    map[string]any    `json:"metadata,omitempty"`
}
```

### 4.2 Trigger Configuration

```go
// TriggerConfig defines what starts a workflow
type TriggerConfig struct {
    // Rule trigger - workflow starts when rule fires
    Rule string `json:"rule,omitempty"`
    
    // Subject trigger - workflow starts on NATS message
    Subject string `json:"subject,omitempty"`
    
    // Timer trigger - workflow starts on schedule
    Cron string `json:"cron,omitempty"`
    
    // Manual trigger - workflow started via API only
    Manual bool `json:"manual,omitempty"`
}
```

### 4.3 Step Definition

```go
// StepDef defines a single workflow step
type StepDef struct {
    Name        string       `json:"name"`
    Description string       `json:"description,omitempty"`

    // Data contracts (ADR-020)
    Inputs      map[string]InputDef  `json:"inputs,omitempty"`
    Outputs     map[string]OutputDef `json:"outputs,omitempty"`

    Action      ActionDef    `json:"action"`

    // Flow control
    OnSuccess   string       `json:"on_success,omitempty"` // Step name, "next", or "complete"
    OnFail      string       `json:"on_fail,omitempty"`    // Step name or "abort"

    // Reliability
    Retry       *RetryPolicy `json:"retry,omitempty"`
    Timeout     string       `json:"timeout,omitempty"`

    // Conditions
    Condition   *Condition   `json:"condition,omitempty"`  // Skip step if false
}

// InputDef defines a step input with reference to source data (ADR-020)
type InputDef struct {
    From      string `json:"from"`                 // Reference: "step.output", "trigger.payload.field", "execution.id"
    Interface string `json:"interface,omitempty"`  // Optional type annotation for validation
}

// OutputDef defines a step output with optional type annotation (ADR-020)
type OutputDef struct {
    Interface string `json:"interface,omitempty"`  // Optional type annotation for validation
}

// RetryPolicy configures step retry behavior
type RetryPolicy struct {
    MaxAttempts    int    `json:"max_attempts"`              // Default: 1 (no retry)
    InitialBackoff string `json:"initial_backoff,omitempty"` // Default: "1s"
    MaxBackoff     string `json:"max_backoff,omitempty"`     // Default: "1m"
    Multiplier     float64 `json:"multiplier,omitempty"`     // Default: 2.0
}

// Condition for conditional step execution
type Condition struct {
    Field    string `json:"field"`    // Reference into execution context (uses same syntax as InputDef.From)
    Operator string `json:"operator"` // eq, ne, gt, lt, contains, exists
    Value    any    `json:"value"`
}
```

### 4.4 Action Definition

```go
// ActionDef defines what a step does (ADR-020 unified dataflow)
type ActionDef struct {
    Type string `json:"type"` // call, publish, set_state, http, wait

    // For "call" (request/response)
    // Payload assembled from step inputs
    Subject  string         `json:"subject,omitempty"`

    // For "publish" (fire-and-forget)
    // Payload assembled from step inputs
    // Uses Subject above

    // For "set_state" (entity mutation)
    // EntityID, Predicate, Object read from step inputs

    // For "http"
    // Request body assembled from step inputs
    Method   string            `json:"method,omitempty"`
    URL      string            `json:"url,omitempty"`
    Headers  map[string]string `json:"headers,omitempty"` // Supports {{input_name}} placeholders

    // For "wait" (pause execution)
    Duration string `json:"duration,omitempty"`
}
```

### 4.5 Execution State

```go
// Execution represents a running workflow instance
type Execution struct {
    ID           string         `json:"id"`
    WorkflowID   string         `json:"workflow_id"`
    WorkflowVer  string         `json:"workflow_version"`
    
    State        ExecutionState `json:"state"`
    CurrentStep  int            `json:"current_step"`
    
    // Trigger context
    Trigger      TriggerContext `json:"trigger"`
    
    // Step results (accumulated)
    StepResults  map[string]StepResult `json:"step_results"`
    
    // Timing
    StartedAt    time.Time      `json:"started_at"`
    UpdatedAt    time.Time      `json:"updated_at"`
    CompletedAt  *time.Time     `json:"completed_at,omitempty"`
    
    // Error info (if failed)
    Error        string         `json:"error,omitempty"`
    FailedStep   string         `json:"failed_step,omitempty"`
}

type ExecutionState string

const (
    ExecutionPending   ExecutionState = "pending"
    ExecutionRunning   ExecutionState = "running"
    ExecutionWaiting   ExecutionState = "waiting"   // Waiting for timer/callback
    ExecutionCompleted ExecutionState = "completed"
    ExecutionFailed    ExecutionState = "failed"
    ExecutionCancelled ExecutionState = "cancelled"
    ExecutionTimedOut  ExecutionState = "timed_out"
)

type TriggerContext struct {
    Type     string         `json:"type"`      // "rule", "subject", "cron", "manual"
    Source   string         `json:"source"`    // Rule ID, subject, or cron expression
    EntityID string         `json:"entity_id,omitempty"`
    Payload  map[string]any `json:"payload,omitempty"`
    Time     time.Time      `json:"time"`
}

type StepResult struct {
    Name       string         `json:"name"`
    State      string         `json:"state"`  // "completed", "failed", "skipped"
    Output     any            `json:"output,omitempty"`
    Error      string         `json:"error,omitempty"`
    Attempts   int            `json:"attempts"`
    StartedAt  time.Time      `json:"started_at"`
    Duration   time.Duration  `json:"duration"`
}
```

---

## 5. Workflow Definition Schema

### 5.1 JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "workflow-definition.v1.json",
  "type": "object",
  "title": "Workflow Definition",
  "description": "Declarative workflow for multi-step orchestration",
  
  "properties": {
    "id": {
      "type": "string",
      "pattern": "^[a-z][a-z0-9-]*$",
      "description": "Unique workflow identifier"
    },
    "name": {
      "type": "string",
      "description": "Human-readable workflow name"
    },
    "description": {
      "type": "string",
      "description": "Workflow purpose and behavior"
    },
    "version": {
      "type": "string",
      "pattern": "^\\d+\\.\\d+\\.\\d+$",
      "description": "Semantic version"
    },
    "enabled": {
      "type": "boolean",
      "default": true,
      "description": "Whether workflow accepts new triggers"
    },
    
    "trigger": {
      "type": "object",
      "description": "What starts this workflow",
      "properties": {
        "rule": {
          "type": "string",
          "description": "Rule ID that triggers workflow"
        },
        "subject": {
          "type": "string",
          "description": "NATS subject that triggers workflow"
        },
        "cron": {
          "type": "string",
          "description": "Cron expression for scheduled execution"
        },
        "manual": {
          "type": "boolean",
          "description": "Only triggered via API"
        }
      },
      "oneOf": [
        {"required": ["rule"]},
        {"required": ["subject"]},
        {"required": ["cron"]},
        {"required": ["manual"]}
      ]
    },
    
    "input": {
      "type": "object",
      "description": "Expected input schema for validation",
      "properties": {
        "type": {"const": "object"},
        "properties": {"type": "object"},
        "required": {"type": "array", "items": {"type": "string"}}
      }
    },
    
    "steps": {
      "type": "array",
      "minItems": 1,
      "description": "Ordered list of workflow steps",
      "items": {"$ref": "#/definitions/step"}
    },
    
    "on_complete": {
      "type": "array",
      "description": "Actions to execute on successful completion",
      "items": {"$ref": "#/definitions/action"}
    },
    
    "on_fail": {
      "type": "array",
      "description": "Actions to execute on failure",
      "items": {"$ref": "#/definitions/action"}
    },
    
    "timeout": {
      "type": "string",
      "pattern": "^\\d+[smh]$",
      "description": "Overall workflow timeout (e.g., '1h')"
    },
    
    "metadata": {
      "type": "object",
      "description": "Custom metadata for categorization"
    }
  },
  
  "required": ["id", "name", "trigger", "steps"],
  
  "definitions": {
    "step": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string",
          "pattern": "^[a-z][a-z0-9-]*$",
          "description": "Step identifier (unique within workflow)"
        },
        "description": {
          "type": "string"
        },
        "action": {"$ref": "#/definitions/action"},
        "on_success": {
          "type": "string",
          "description": "Next step name, 'next', or 'complete'"
        },
        "on_fail": {
          "type": "string",
          "description": "Step name to jump to, or 'abort'"
        },
        "retry": {"$ref": "#/definitions/retry"},
        "timeout": {
          "type": "string",
          "pattern": "^\\d+[smh]$"
        },
        "condition": {"$ref": "#/definitions/condition"}
      },
      "required": ["name", "action"]
    },
    
    "action": {
      "type": "object",
      "properties": {
        "type": {
          "type": "string",
          "enum": ["call", "publish", "set_state", "http", "wait"]
        },
        "subject": {"type": "string"},
        "payload": {"type": "object"},
        "entity_id": {"type": "string"},
        "predicate": {"type": "string"},
        "object": {},
        "method": {"type": "string", "enum": ["GET", "POST", "PUT", "DELETE"]},
        "url": {"type": "string", "format": "uri"},
        "headers": {"type": "object"},
        "body": {},
        "duration": {"type": "string", "pattern": "^\\d+[smh]$"}
      },
      "required": ["type"]
    },
    
    "retry": {
      "type": "object",
      "properties": {
        "max_attempts": {"type": "integer", "minimum": 1, "maximum": 10},
        "initial_backoff": {"type": "string", "pattern": "^\\d+[smh]$"},
        "max_backoff": {"type": "string", "pattern": "^\\d+[smh]$"},
        "multiplier": {"type": "number", "minimum": 1.0, "maximum": 10.0}
      },
      "required": ["max_attempts"]
    },
    
    "condition": {
      "type": "object",
      "properties": {
        "field": {"type": "string"},
        "operator": {"type": "string", "enum": ["eq", "ne", "gt", "lt", "gte", "lte", "contains", "exists"]},
        "value": {}
      },
      "required": ["field", "operator"]
    }
  }
}
```

---

## 6. Execution Model

### 6.1 Lifecycle

```
┌─────────────┐
│   Trigger   │
│  Received   │
└──────┬──────┘
       │
       ▼
┌─────────────┐     ┌─────────────┐
│  Validate   │────►│   Failed    │ (invalid input)
│   Input     │     └─────────────┘
└──────┬──────┘
       │ valid
       ▼
┌─────────────┐
│   Create    │
│  Execution  │──────────────────────────┐
└──────┬──────┘                          │
       │                                  │ timeout
       ▼                                  ▼
┌─────────────┐                   ┌─────────────┐
│  Execute    │◄─────────────────►│  Timed Out  │
│   Steps     │    step timeout   └─────────────┘
└──────┬──────┘
       │
       ├─── all steps complete ───►┌─────────────┐
       │                           │  Completed  │
       │                           └─────────────┘
       │
       ├─── step failed (no retry)─►┌─────────────┐
       │                            │   Failed    │
       │                            └─────────────┘
       │
       └─── cancelled ─────────────►┌─────────────┐
                                    │  Cancelled  │
                                    └─────────────┘
```

### 6.2 Step Execution

```go
func (e *Executor) executeStep(ctx context.Context, exec *Execution, step *StepDef) error {
    // 1. Check condition
    if step.Condition != nil && !e.evaluateCondition(exec, step.Condition) {
        e.recordStepSkipped(exec, step.Name)
        return nil
    }
    
    // 2. Setup timeout
    stepCtx := ctx
    if step.Timeout != "" {
        timeout, _ := time.ParseDuration(step.Timeout)
        stepCtx, cancel = context.WithTimeout(ctx, timeout)
        defer cancel()
    }
    
    // 3. Execute with retry
    var lastErr error
    attempts := 1
    maxAttempts := 1
    if step.Retry != nil {
        maxAttempts = step.Retry.MaxAttempts
    }
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        result, err := e.executeAction(stepCtx, exec, step.Action)
        if err == nil {
            e.recordStepSuccess(exec, step.Name, result, attempt)
            return nil
        }
        
        lastErr = err
        if attempt < maxAttempts {
            backoff := e.calculateBackoff(step.Retry, attempt)
            time.Sleep(backoff)
        }
    }
    
    // 4. Handle failure
    e.recordStepFailure(exec, step.Name, lastErr, maxAttempts)
    return lastErr
}
```

### 6.3 Data References (ADR-020)

Workflows use the unified dataflow pattern from [ADR-020](../adr-020-unified-dataflow-patterns.md). Data is referenced via `from` fields in step inputs rather than string interpolation:

| Reference Pattern | Description | Example |
|-------------------|-------------|---------|
| `step_name.output_name` | Named output from step | `"fetch.result"` |
| `step_name.output_name.field` | Deep field access | `"extract.tasks.items"` |
| `trigger.entity_id` | Entity that triggered workflow | `acme.ops.specs.core.spec.auth` |
| `trigger.payload.field` | Field from trigger payload | `"trigger.payload.status"` |
| `execution.id` | Current execution ID | `exec-abc123` |
| `timestamp` | Current ISO timestamp | `2025-01-01T12:00:00Z` |
| `uuid` | Generate new UUID | `550e8400-e29b-41d4-a716...` |

Example usage:
```json
{
  "name": "create-issues",
  "inputs": {
    "spec_id": {"from": "trigger.entity_id"},
    "tasks": {"from": "extract-tasks.tasks"},
    "created_at": {"from": "timestamp"}
  },
  "outputs": {
    "issue_ids": {"interface": "github.issues.v1"}
  },
  "action": {
    "type": "call",
    "subject": "github.issues.create"
  }
}
```

**Validation**: The workflow loader validates all `from` references at load time, ensuring referenced steps and outputs exist before execution.

### 6.4 Error Handling

```
Step Failure
     │
     ▼
┌─────────────────┐
│ Has retry?      │───No──┐
└────────┬────────┘       │
         │ Yes            │
         ▼                │
┌─────────────────┐       │
│ Attempts <      │───No──┤
│ max_attempts?   │       │
└────────┬────────┘       │
         │ Yes            │
         ▼                │
┌─────────────────┐       │
│ Wait backoff    │       │
│ Retry step      │       │
└────────┬────────┘       │
         │                │
         │ (loop)         │
         ▼                ▼
┌─────────────────────────────────┐
│ Check on_fail directive         │
├─────────────────────────────────┤
│ "abort"  → Mark execution failed│
│ "step-x" → Jump to step-x       │
│ (empty)  → Mark execution failed│
└─────────────────────────────────┘
```

---

## 7. Actions

### 7.1 Call (Request/Response)

Executes a NATS request and waits for response. Request payload is assembled from step inputs.

```json
{
  "name": "get-spec",
  "inputs": {
    "id": {"from": "trigger.entity_id"}
  },
  "outputs": {
    "spec": {"interface": "spec.data.v1"}
  },
  "action": {
    "type": "call",
    "subject": "semmem.spec.get"
  }
}
```

**Behavior:**
- Assembles request payload from step inputs
- Sends request to specified subject
- Waits for response (respects step timeout)
- Response becomes step output
- Error response or timeout = step failure

### 7.2 Publish (Fire-and-Forget)

Publishes a message without waiting for response. Message payload is assembled from step inputs.

```json
{
  "name": "notify-approval",
  "inputs": {
    "spec_id": {"from": "trigger.entity_id"},
    "approved_at": {"from": "timestamp"}
  },
  "action": {
    "type": "publish",
    "subject": "events.spec.approved"
  }
}
```

**Behavior:**
- Assembles message payload from step inputs
- Publishes to subject
- Immediately succeeds (no response expected)
- Step output is `{"published": true}`

### 7.3 Set State (Entity Mutation)

Updates entity state via graph processor. Entity and state data are read from step inputs.

```json
{
  "name": "update-status",
  "inputs": {
    "entity_id": {"from": "trigger.entity_id"},
    "predicate": {"value": "spec.status.current"},
    "object": {"value": "implementing"}
  },
  "action": {
    "type": "set_state"
  }
}
```

**Behavior:**
- Reads `entity_id`, `predicate`, and `object` from step inputs
- Publishes triple to graph processor
- Waits for confirmation
- Step output is the triple

### 7.4 HTTP (External Call)

Makes HTTP request to external service. Request body is assembled from step inputs, headers support placeholders.

```json
{
  "name": "create-issue",
  "inputs": {
    "title": {"from": "extract.title"},
    "body": {"from": "extract.description"},
    "github_token": {"from": "secrets.github_token"}
  },
  "outputs": {
    "issue": {"interface": "github.issue.v1"}
  },
  "action": {
    "type": "http",
    "method": "POST",
    "url": "https://api.github.com/repos/org/repo/issues",
    "headers": {
      "Authorization": "Bearer {{github_token}}",
      "Content-Type": "application/json"
    }
  }
}
```

**Behavior:**
- Assembles request body from step inputs
- Resolves header placeholders (`{{input_name}}`) from step inputs
- Makes HTTP request
- Response body becomes step output
- Non-2xx status = step failure

### 7.5 Wait (Pause Execution)

Pauses execution for specified duration.

```json
{
  "type": "wait",
  "duration": "5m"
}
```

**Behavior:**
- Schedules timer via Timer Service
- Execution state becomes "waiting"
- Resumes when timer fires
- Step output is `{"waited": "5m"}`

---

## 8. Timer Service

### 8.1 Purpose

The Timer Service handles:
- Step timeouts
- Workflow timeouts
- Wait actions
- Scheduled (cron) triggers

### 8.2 Implementation

```go
type TimerService struct {
    kv       nats.KeyValue  // WORKFLOW_TIMERS bucket
    nc       *nats.Conn
    subject  string         // workflow.timer.fire
    
    mu       sync.Mutex
    timers   map[string]*time.Timer  // In-memory for leader
}

type TimerDef struct {
    ID          string    `json:"id"`
    ExecutionID string    `json:"execution_id"`
    Type        string    `json:"type"`  // "step_timeout", "workflow_timeout", "wait", "cron"
    FireAt      time.Time `json:"fire_at"`
    Payload     any       `json:"payload"`
}

func (t *TimerService) Schedule(ctx context.Context, timer TimerDef) error {
    // 1. Persist to KV (durable)
    data, _ := json.Marshal(timer)
    _, err := t.kv.Put(ctx, "timer:"+timer.ID, data)
    if err != nil {
        return err
    }
    
    // 2. Set in-memory timer (leader only)
    t.mu.Lock()
    defer t.mu.Unlock()
    
    duration := time.Until(timer.FireAt)
    t.timers[timer.ID] = time.AfterFunc(duration, func() {
        t.fire(timer)
    })
    
    return nil
}

func (t *TimerService) fire(timer TimerDef) {
    // Publish to workflow processor
    msg := TimerFired{
        TimerID:     timer.ID,
        ExecutionID: timer.ExecutionID,
        Type:        timer.Type,
        Payload:     timer.Payload,
    }
    t.nc.Publish(t.subject, msg)
    
    // Clean up
    t.kv.Delete(context.Background(), "timer:"+timer.ID)
}
```

### 8.3 Recovery

On processor startup:
1. Load all timers from WORKFLOW_TIMERS KV
2. For timers with `fire_at` in the past → fire immediately
3. For future timers → schedule in-memory

---

## 9. API Contract

### 9.1 NATS Subjects

| Subject | Direction | Description |
|---------|-----------|-------------|
| `workflow.trigger.{workflow_id}` | Input | Start workflow execution |
| `workflow.timer.fire` | Input | Timer fired (internal) |
| `workflow.execution.cancel` | Input | Cancel running execution |
| `workflow.execution.status` | Request | Get execution status |
| `workflow.events` | Output | Execution lifecycle events |

### 9.2 Trigger Message

```json
{
  "workflow_id": "spec-approval",
  "entity_id": "semmem.semmem.specs.core.spec.auth",
  "payload": {
    "approved_by": "alice",
    "comment": "Looks good"
  },
  "idempotency_key": "trigger-abc123"
}
```

### 9.3 Status Request/Response

**Request:**
```json
{
  "execution_id": "exec-abc123"
}
```

**Response:**
```json
{
  "id": "exec-abc123",
  "workflow_id": "spec-approval",
  "state": "running",
  "current_step": 2,
  "steps": [
    {"name": "load-spec", "state": "completed", "duration_ms": 45},
    {"name": "extract-tasks", "state": "completed", "duration_ms": 120},
    {"name": "create-issues", "state": "running", "attempts": 1}
  ],
  "started_at": "2025-01-01T12:00:00Z",
  "updated_at": "2025-01-01T12:00:05Z"
}
```

### 9.4 Execution Events

Published to `workflow.events`:

```json
{
  "type": "execution.started",
  "execution_id": "exec-abc123",
  "workflow_id": "spec-approval",
  "trigger": {
    "type": "rule",
    "source": "spec-approved-trigger",
    "entity_id": "semmem.semmem.specs.core.spec.auth"
  },
  "timestamp": "2025-01-01T12:00:00Z"
}
```

```json
{
  "type": "step.completed",
  "execution_id": "exec-abc123",
  "step": "load-spec",
  "output": {"id": "...", "title": "User Authentication"},
  "duration_ms": 45,
  "timestamp": "2025-01-01T12:00:00.045Z"
}
```

```json
{
  "type": "execution.completed",
  "execution_id": "exec-abc123",
  "workflow_id": "spec-approval",
  "duration_ms": 5230,
  "timestamp": "2025-01-01T12:00:05.230Z"
}
```

---

## 10. Configuration

### 10.1 Processor Configuration

```json
{
  "workflow": {
    "config": {
      "definitions_bucket": "WORKFLOW_DEFINITIONS",
      "executions_bucket": "WORKFLOW_EXECUTIONS",
      "timers_bucket": "WORKFLOW_TIMERS",
      "secrets_bucket": "WORKFLOW_SECRETS",
      "idempotency_bucket": "WORKFLOW_IDEMPOTENCY",
      
      "trigger_subject_prefix": "workflow.trigger",
      "timer_subject": "workflow.timer.fire",
      "events_subject": "workflow.events",
      "secrets_subject_prefix": "workflow.secrets",
      
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
      },
      
      "metrics": {
        "enabled": true,
        "namespace": "semstreams",
        "subsystem": "workflow"
      }
    }
  }
}
```

### 10.2 Schema Tags

```go
type Config struct {
    // Storage buckets
    DefinitionsBucket  string `json:"definitions_bucket" schema:"type:string,desc:KV bucket for workflow definitions,default:WORKFLOW_DEFINITIONS,category:basic"`
    ExecutionsBucket   string `json:"executions_bucket" schema:"type:string,desc:KV bucket for execution state,default:WORKFLOW_EXECUTIONS,category:basic"`
    TimersBucket       string `json:"timers_bucket" schema:"type:string,desc:KV bucket for timers,default:WORKFLOW_TIMERS,category:basic"`
    SecretsBucket      string `json:"secrets_bucket" schema:"type:string,desc:KV bucket for secrets,default:WORKFLOW_SECRETS,category:advanced"`
    IdempotencyBucket  string `json:"idempotency_bucket" schema:"type:string,desc:KV bucket for idempotency keys,default:WORKFLOW_IDEMPOTENCY,category:advanced"`
    
    // NATS subjects
    TriggerSubjectPrefix string `json:"trigger_subject_prefix" schema:"type:string,desc:NATS subject prefix for triggers,default:workflow.trigger,category:basic"`
    TimerSubject         string `json:"timer_subject" schema:"type:string,desc:NATS subject for timer events,default:workflow.timer.fire,category:advanced"`
    EventsSubject        string `json:"events_subject" schema:"type:string,desc:NATS subject for execution events,default:workflow.events,category:basic"`
    SecretsSubjectPrefix string `json:"secrets_subject_prefix" schema:"type:string,desc:NATS subject prefix for secrets management,default:workflow.secrets,category:advanced"`
    
    // Timeouts
    DefaultStepTimeout     string `json:"default_step_timeout" schema:"type:string,desc:Default timeout per step,default:30s,category:basic"`
    DefaultWorkflowTimeout string `json:"default_workflow_timeout" schema:"type:string,desc:Default overall workflow timeout,default:1h,category:basic"`
    
    // Limits
    MaxConcurrentExecutions int `json:"max_concurrent_executions" schema:"type:int,desc:Max concurrent executions,default:100,min:1,max:1000,category:advanced"`
    
    // Idempotency
    Idempotency IdempotencyConfig `json:"idempotency" schema:"type:object,desc:Idempotency configuration,category:advanced"`
    
    // Retry defaults
    RetryDefaults RetryPolicy `json:"retry_defaults" schema:"type:object,desc:Default retry policy for steps,category:advanced"`
    
    // Metrics
    Metrics MetricsConfig `json:"metrics" schema:"type:object,desc:Prometheus metrics configuration,category:advanced"`
}

type IdempotencyConfig struct {
    Enabled bool   `json:"enabled" schema:"type:bool,desc:Enable idempotency checking,default:true"`
    Window  string `json:"window" schema:"type:string,desc:Deduplication window duration,default:1h"`
}

type MetricsConfig struct {
    Enabled   bool   `json:"enabled" schema:"type:bool,desc:Enable Prometheus metrics,default:true"`
    Namespace string `json:"namespace" schema:"type:string,desc:Metrics namespace,default:semstreams"`
    Subsystem string `json:"subsystem" schema:"type:string,desc:Metrics subsystem,default:workflow"`
}
```

### 10.3 NATS Bucket Configuration

The workflow processor requires these NATS KV buckets:

| Bucket | TTL | Purpose |
|--------|-----|---------|
| `WORKFLOW_DEFINITIONS` | None | Workflow definitions (persistent) |
| `WORKFLOW_EXECUTIONS` | 7d | Execution state and history |
| `WORKFLOW_TIMERS` | None | Scheduled timers |
| `WORKFLOW_SECRETS` | None | Encrypted secrets |
| `WORKFLOW_IDEMPOTENCY` | 24h | Idempotency keys (auto-expire) |

Bucket creation (if not exists):

```go
func (p *WorkflowProcessor) ensureBuckets(ctx context.Context) error {
    buckets := []struct {
        Name string
        TTL  time.Duration
    }{
        {p.config.DefinitionsBucket, 0},
        {p.config.ExecutionsBucket, 7 * 24 * time.Hour},
        {p.config.TimersBucket, 0},
        {p.config.SecretsBucket, 0},
        {p.config.IdempotencyBucket, 24 * time.Hour},
    }
    
    for _, b := range buckets {
        _, err := p.js.CreateKeyValue(ctx, &nats.KeyValueConfig{
            Bucket: b.Name,
            TTL:    b.TTL,
        })
        if err != nil && !errors.Is(err, nats.ErrBucketExists) {
            return fmt.Errorf("create bucket %s: %w", b.Name, err)
        }
    }
    
    return nil
}
```

---

## 11. Integration Patterns

### 11.1 Rules → Workflow Trigger

```json
// Rule definition
{
  "id": "spec-approved-trigger",
  "conditions": [
    {"field": "spec.status.current", "operator": "eq", "value": "approved"}
  ],
  "on_enter": [
    {
      "type": "publish",
      "subject": "workflow.trigger.spec-approval",
      "payload": {
        "entity_id": "${entity_id}"
      }
    }
  ]
}
```

### 11.2 Workflow → Entity State

```json
// Workflow step
{
  "name": "update-status",
  "action": {
    "type": "set_state",
    "entity_id": "${trigger.entity_id}",
    "predicate": "spec.status.current",
    "object": "implementing"
  }
}
```

### 11.3 Workflow → External Service

```json
// Workflow step calling domain service
{
  "name": "create-issues",
  "action": {
    "type": "call",
    "subject": "semmem.github.create-issues",
    "payload": {
      "spec_id": "${trigger.entity_id}",
      "tasks": "${steps.extract-tasks.output.tasks}"
    }
  },
  "retry": {
    "max_attempts": 3,
    "initial_backoff": "5s"
  },
  "timeout": "60s"
}
```

### 11.4 Workflow Chaining

Workflow A triggers Workflow B:

```json
// In workflow A
{
  "name": "trigger-notification-workflow",
  "action": {
    "type": "publish",
    "subject": "workflow.trigger.send-notifications",
    "payload": {
      "recipients": "${steps.get-stakeholders.output.users}",
      "message": "Spec ${trigger.entity_id} approved"
    }
  }
}
```

---

## 12. Examples

### 12.1 SemMem: Spec Approval Workflow

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
      "description": "Load the approved spec",
      "inputs": {
        "id": {"from": "trigger.entity_id"}
      },
      "outputs": {
        "spec": {"interface": "spec.data.v1"}
      },
      "action": {
        "type": "call",
        "subject": "semmem.spec.get"
      },
      "timeout": "10s"
    },
    {
      "name": "extract-tasks",
      "description": "Parse spec for task definitions",
      "inputs": {
        "spec": {"from": "load-spec.spec"}
      },
      "outputs": {
        "tasks": {"interface": "spec.tasks.v1"}
      },
      "action": {
        "type": "call",
        "subject": "semmem.spec.extract-tasks"
      },
      "timeout": "30s",
      "condition": {
        "field": "load-spec.spec.has_tasks",
        "operator": "eq",
        "value": true
      }
    },
    {
      "name": "create-issues",
      "description": "Create GitHub issues for each task",
      "inputs": {
        "spec_id": {"from": "trigger.entity_id"},
        "tasks": {"from": "extract-tasks.tasks"}
      },
      "outputs": {
        "issue_ids": {},
        "count": {}
      },
      "action": {
        "type": "call",
        "subject": "semmem.github.create-issues"
      },
      "retry": {
        "max_attempts": 3,
        "initial_backoff": "5s",
        "multiplier": 2.0
      },
      "timeout": "60s",
      "on_fail": "mark-blocked"
    },
    {
      "name": "link-issues",
      "description": "Link created issues to spec entity",
      "inputs": {
        "spec_id": {"from": "trigger.entity_id"},
        "issue_ids": {"from": "create-issues.issue_ids"}
      },
      "action": {
        "type": "call",
        "subject": "semmem.spec.link-tasks"
      }
    },
    {
      "name": "mark-blocked",
      "description": "Mark spec as blocked if issue creation failed",
      "inputs": {
        "entity_id": {"from": "trigger.entity_id"},
        "predicate": {"value": "spec.status.current"},
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
        "predicate": {"value": "spec.status.current"},
        "object": {"value": "implementing"}
      }
    },
    {
      "type": "publish",
      "subject": "semmem.events.spec.implementing",
      "inputs": {
        "spec_id": {"from": "trigger.entity_id"},
        "issue_count": {"from": "create-issues.count"}
      }
    }
  ],

  "on_fail": [
    {
      "type": "set_state",
      "inputs": {
        "entity_id": {"from": "trigger.entity_id"},
        "predicate": {"value": "workflow.state.current"},
        "object": {"value": "failed"}
      }
    },
    {
      "type": "publish",
      "subject": "semmem.events.workflow.failed",
      "inputs": {
        "workflow_id": {"value": "spec-approval"},
        "entity_id": {"from": "trigger.entity_id"},
        "error": {"from": "execution.error"}
      }
    }
  ],
  
  "timeout": "5m"
}
```

### 12.2 Robotics: Simple Mission Workflow

```json
{
  "id": "survey-mission",
  "name": "Area Survey Mission",
  "description": "Execute survey pattern over designated area",
  "version": "1.0.0",
  "enabled": true,
  
  "trigger": {
    "subject": "missions.survey.start"
  },
  
  "input": {
    "type": "object",
    "properties": {
      "drone_id": {"type": "string"},
      "area": {"type": "object"},
      "altitude": {"type": "number"}
    },
    "required": ["drone_id", "area"]
  },
  
  "steps": [
    {
      "name": "preflight-check",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"}
      },
      "action": {
        "type": "call",
        "subject": "drone.preflight.check"
      },
      "timeout": "30s"
    },
    {
      "name": "arm-and-takeoff",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"},
        "altitude": {"from": "trigger.payload.altitude"}
      },
      "action": {
        "type": "call",
        "subject": "drone.takeoff"
      },
      "timeout": "2m",
      "on_fail": "abort"
    },
    {
      "name": "navigate-to-area",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"},
        "waypoint": {"from": "trigger.payload.area.start"}
      },
      "action": {
        "type": "call",
        "subject": "drone.goto"
      },
      "timeout": "5m",
      "on_fail": "emergency-rtl"
    },
    {
      "name": "execute-survey",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"},
        "pattern": {"from": "trigger.payload.area.pattern"}
      },
      "outputs": {
        "survey_data": {"interface": "drone.survey.result.v1"}
      },
      "action": {
        "type": "call",
        "subject": "drone.survey.execute"
      },
      "timeout": "30m",
      "on_fail": "emergency-rtl"
    },
    {
      "name": "return-to-launch",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"}
      },
      "action": {
        "type": "call",
        "subject": "drone.rtl"
      },
      "timeout": "10m"
    },
    {
      "name": "land",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"}
      },
      "action": {
        "type": "call",
        "subject": "drone.land"
      },
      "timeout": "2m"
    },
    {
      "name": "emergency-rtl",
      "description": "Emergency return to launch",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"}
      },
      "action": {
        "type": "call",
        "subject": "drone.emergency.rtl"
      },
      "on_success": "complete"
    }
  ],

  "on_complete": [
    {
      "type": "publish",
      "subject": "missions.survey.completed",
      "inputs": {
        "drone_id": {"from": "trigger.payload.drone_id"},
        "survey_data": {"from": "execute-survey.survey_data"}
      }
    }
  ],
  
  "timeout": "1h"
}
```

### 12.3 Scheduled Data Aggregation

```json
{
  "id": "daily-progress-report",
  "name": "Daily Progress Report",
  "description": "Generate and send daily progress summary",
  "version": "1.0.0",
  "enabled": true,
  
  "trigger": {
    "cron": "0 9 * * *"
  },
  
  "steps": [
    {
      "name": "collect-metrics",
      "inputs": {
        "period": {"value": "24h"},
        "types": {"value": ["specs", "tasks", "commits"]}
      },
      "outputs": {
        "metrics": {"interface": "metrics.data.v1"}
      },
      "action": {
        "type": "call",
        "subject": "semmem.metrics.collect"
      },
      "timeout": "2m"
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
        "subject": "semmem.report.generate"
      },
      "timeout": "1m"
    },
    {
      "name": "send-notifications",
      "inputs": {
        "channel": {"value": "engineering"},
        "message": {"from": "generate-summary.summary"}
      },
      "action": {
        "type": "call",
        "subject": "semmem.notify.team"
      },
      "retry": {
        "max_attempts": 3,
        "initial_backoff": "10s"
      }
    }
  ],

  "on_complete": [
    {
      "type": "publish",
      "subject": "semmem.events.report.sent",
      "inputs": {
        "report_type": {"value": "daily-progress"},
        "sent_at": {"from": "timestamp"}
      }
    }
  ],
  
  "timeout": "10m"
}
```

---

## 13. Secrets Management

### 13.1 Overview

Workflow secrets are stored in a dedicated NATS KV bucket with restricted access. Only the workflow processor reads from this bucket; secrets are never exposed in execution state or logs.

### 13.2 Storage

```
NATS KV Bucket: WORKFLOW_SECRETS

secrets:{name}              → Encrypted secret value
secrets:{name}:metadata     → Creation time, rotation info
```

### 13.3 Secret Reference

Secrets are referenced in workflow definitions using `${secrets.name}`:

```json
{
  "action": {
    "type": "http",
    "url": "https://api.github.com/repos/...",
    "headers": {
      "Authorization": "Bearer ${secrets.github_token}"
    }
  }
}
```

### 13.4 Resolution

```go
func (e *Executor) resolveSecrets(ctx context.Context, value string) (string, error) {
    // Find all ${secrets.X} patterns
    matches := secretPattern.FindAllStringSubmatch(value, -1)
    
    for _, match := range matches {
        secretName := match[1]
        
        // Fetch from secrets bucket (cached)
        secretValue, err := e.secrets.Get(ctx, secretName)
        if err != nil {
            return "", fmt.Errorf("secret %q not found", secretName)
        }
        
        value = strings.Replace(value, match[0], secretValue, 1)
    }
    
    return value, nil
}
```

### 13.5 Security Considerations

| Concern | Mitigation |
|---------|------------|
| Secrets in logs | Never log resolved secret values; log `${secrets.X}` placeholder |
| Secrets in state | Never persist resolved secrets to execution state |
| Bucket access | NATS permissions restrict bucket to workflow processor only |
| Rotation | Secrets resolved at execution time; rotation is immediate |

### 13.6 Secret Management API

Secrets are managed via dedicated NATS subjects (admin only):

| Subject | Description |
|---------|-------------|
| `workflow.secrets.set` | Create/update secret |
| `workflow.secrets.delete` | Remove secret |
| `workflow.secrets.list` | List secret names (not values) |

```json
// Set secret request
{
  "name": "github_token",
  "value": "ghp_xxxxxxxxxxxx"
}

// List response
{
  "secrets": ["github_token", "slack_webhook", "api_key"]
}
```

---

## 14. Idempotency

### 14.1 Purpose

Prevent duplicate workflow executions from the same trigger event. This handles:
- Message redelivery (NATS at-least-once)
- Duplicate rule firings
- Retry storms from upstream systems

### 14.2 Idempotency Key

Triggers include an optional `idempotency_key`:

```json
{
  "workflow_id": "spec-approval",
  "entity_id": "semmem.semmem.specs.core.spec.auth",
  "idempotency_key": "spec-approval:semmem.semmem.specs.core.spec.auth:2025-01-01T12:00:00Z",
  "payload": {}
}
```

If not provided, one is generated from `workflow_id + entity_id + timestamp` (rounded to minute).

### 14.3 Deduplication Window

```go
const (
    DefaultIdempotencyWindow = 1 * time.Hour
    MaxIdempotencyWindow     = 24 * time.Hour
)

type IdempotencyConfig struct {
    Window time.Duration `json:"window" schema:"type:string,desc:Deduplication window,default:1h"`
}
```

### 14.4 Implementation

```
NATS KV Bucket: WORKFLOW_IDEMPOTENCY (with TTL)

idem:{idempotency_key} → {
    "execution_id": "exec-abc123",
    "workflow_id": "spec-approval", 
    "created_at": "2025-01-01T12:00:00Z"
}
```

```go
func (p *WorkflowProcessor) handleTrigger(ctx context.Context, trigger TriggerMessage) error {
    key := trigger.IdempotencyKey
    if key == "" {
        key = generateIdempotencyKey(trigger)
    }
    
    // Check for existing execution
    existing, err := p.idempotency.Get(ctx, key)
    if err == nil {
        // Duplicate - return existing execution ID
        p.logger.Info("duplicate trigger ignored",
            "idempotency_key", key,
            "existing_execution", existing.ExecutionID)
        return &DuplicateTriggerError{
            IdempotencyKey: key,
            ExecutionID:    existing.ExecutionID,
        }
    }
    
    // Create new execution
    execID := generateExecutionID()
    
    // Record idempotency (with TTL)
    err = p.idempotency.Put(ctx, key, IdempotencyRecord{
        ExecutionID: execID,
        WorkflowID:  trigger.WorkflowID,
        CreatedAt:   time.Now(),
    })
    if err != nil {
        return err
    }
    
    // Proceed with execution
    return p.startExecution(ctx, execID, trigger)
}
```

### 14.5 Duplicate Response

When a duplicate trigger is received, the response includes the existing execution:

```json
{
  "status": "duplicate",
  "idempotency_key": "spec-approval:...:2025-01-01T12:00:00Z",
  "existing_execution_id": "exec-abc123",
  "message": "Workflow already triggered within idempotency window"
}
```

---

## 15. Observability

### 15.1 Structured Logging

All log entries include execution context:

```go
type ExecutionLogger struct {
    base        *slog.Logger
    executionID string
    workflowID  string
}

func (l *ExecutionLogger) Info(msg string, args ...any) {
    l.base.Info(msg, append([]any{
        "execution_id", l.executionID,
        "workflow_id", l.workflowID,
    }, args...)...)
}
```

**Log Levels:**

| Level | Content |
|-------|---------|
| DEBUG | Variable resolution, condition evaluation |
| INFO | Step start/complete, execution lifecycle |
| WARN | Retry attempts, timeout approaching |
| ERROR | Step failure, execution failure |

**Example Log Output:**

```json
{"level":"INFO","msg":"execution started","execution_id":"exec-abc123","workflow_id":"spec-approval","trigger_type":"rule","entity_id":"semmem.semmem.specs.core.spec.auth"}
{"level":"INFO","msg":"step started","execution_id":"exec-abc123","workflow_id":"spec-approval","step":"load-spec"}
{"level":"INFO","msg":"step completed","execution_id":"exec-abc123","workflow_id":"spec-approval","step":"load-spec","duration_ms":45}
{"level":"WARN","msg":"step retry","execution_id":"exec-abc123","workflow_id":"spec-approval","step":"create-issues","attempt":2,"error":"connection timeout"}
{"level":"INFO","msg":"execution completed","execution_id":"exec-abc123","workflow_id":"spec-approval","duration_ms":5230,"steps_completed":4}
```

### 15.2 Prometheus Metrics

```go
var (
    // Execution metrics
    executionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflow_executions_total",
            Help: "Total workflow executions by workflow and status",
        },
        []string{"workflow_id", "status"}, // status: completed, failed, cancelled, timed_out
    )
    
    executionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "workflow_execution_duration_seconds",
            Help:    "Workflow execution duration",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
        },
        []string{"workflow_id", "status"},
    )
    
    executionsActive = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "workflow_executions_active",
            Help: "Currently running workflow executions",
        },
        []string{"workflow_id"},
    )
    
    // Step metrics
    stepsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflow_steps_total",
            Help: "Total workflow steps by workflow, step, and status",
        },
        []string{"workflow_id", "step", "status"}, // status: completed, failed, skipped
    )
    
    stepDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "workflow_step_duration_seconds",
            Help:    "Step execution duration",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 12), // 10ms to ~40s
        },
        []string{"workflow_id", "step"},
    )
    
    stepRetries = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflow_step_retries_total",
            Help: "Total step retry attempts",
        },
        []string{"workflow_id", "step"},
    )
    
    // Timer metrics
    timersActive = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "workflow_timers_active",
            Help: "Currently scheduled timers",
        },
    )
    
    timersFired = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflow_timers_fired_total",
            Help: "Total timers fired by type",
        },
        []string{"type"}, // step_timeout, workflow_timeout, wait, cron
    )
    
    // Idempotency metrics
    duplicateTriggersTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflow_duplicate_triggers_total",
            Help: "Duplicate triggers rejected by idempotency",
        },
        []string{"workflow_id"},
    )
)
```

### 15.3 Metric Examples

**Grafana Dashboard Queries:**

```promql
# Execution success rate (5m window)
sum(rate(workflow_executions_total{status="completed"}[5m])) by (workflow_id)
/
sum(rate(workflow_executions_total[5m])) by (workflow_id)

# P99 execution duration
histogram_quantile(0.99, 
  sum(rate(workflow_execution_duration_seconds_bucket[5m])) by (workflow_id, le)
)

# Active executions
sum(workflow_executions_active) by (workflow_id)

# Retry rate by step
sum(rate(workflow_step_retries_total[5m])) by (workflow_id, step)

# Step failure hotspots
topk(10, sum(rate(workflow_steps_total{status="failed"}[1h])) by (workflow_id, step))
```

### 15.4 Health Check

```go
func (p *WorkflowProcessor) HealthCheck() error {
    // Check KV buckets accessible
    if _, err := p.executions.Status(context.Background()); err != nil {
        return fmt.Errorf("executions bucket: %w", err)
    }
    if _, err := p.timers.Status(context.Background()); err != nil {
        return fmt.Errorf("timers bucket: %w", err)
    }
    
    // Check no stuck executions (running > 2x max timeout)
    // ... 
    
    return nil
}
```

---

## 16. Schema Extension for UI

### 16.1 Extended Workflow Schema

The workflow definition schema extends to support visual builder metadata:

```json
{
  "id": "spec-approval",
  "name": "Spec Approval Workflow",
  
  "_ui": {
    "color": "#4CAF50",
    "icon": "check-circle",
    "category": "lifecycle",
    "tags": ["spec", "github", "automation"],
    
    "canvas": {
      "zoom": 1.0,
      "pan": {"x": 0, "y": 0}
    },
    
    "steps": {
      "load-spec": {
        "position": {"x": 100, "y": 100},
        "color": "#2196F3"
      },
      "extract-tasks": {
        "position": {"x": 100, "y": 200},
        "collapsed": false
      },
      "create-issues": {
        "position": {"x": 100, "y": 300},
        "color": "#FF9800",
        "notes": "May take time due to GitHub rate limits"
      }
    },
    
    "groups": [
      {
        "id": "preparation",
        "name": "Preparation",
        "steps": ["load-spec", "extract-tasks"],
        "color": "#E3F2FD"
      },
      {
        "id": "execution", 
        "name": "GitHub Integration",
        "steps": ["create-issues", "link-issues"],
        "color": "#FFF3E0"
      }
    ]
  },
  
  "trigger": { ... },
  "steps": [ ... ]
}
```

### 16.2 UI Metadata Schema

```json
{
  "$id": "workflow-ui-metadata.v1.json",
  "type": "object",
  "title": "Workflow UI Metadata",
  "description": "Visual builder metadata for workflow definitions",
  
  "properties": {
    "color": {
      "type": "string",
      "pattern": "^#[0-9A-Fa-f]{6}$",
      "description": "Workflow color in hex"
    },
    "icon": {
      "type": "string",
      "description": "Icon name from icon library"
    },
    "category": {
      "type": "string",
      "enum": ["lifecycle", "integration", "notification", "maintenance", "custom"],
      "description": "Workflow category for organization"
    },
    "tags": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Searchable tags"
    },
    
    "canvas": {
      "type": "object",
      "properties": {
        "zoom": {"type": "number", "minimum": 0.1, "maximum": 2.0},
        "pan": {
          "type": "object",
          "properties": {
            "x": {"type": "number"},
            "y": {"type": "number"}
          }
        }
      }
    },
    
    "steps": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "properties": {
          "position": {
            "type": "object",
            "properties": {
              "x": {"type": "number"},
              "y": {"type": "number"}
            },
            "required": ["x", "y"]
          },
          "color": {"type": "string", "pattern": "^#[0-9A-Fa-f]{6}$"},
          "collapsed": {"type": "boolean"},
          "notes": {"type": "string"}
        }
      }
    },
    
    "groups": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "name": {"type": "string"},
          "steps": {"type": "array", "items": {"type": "string"}},
          "color": {"type": "string", "pattern": "^#[0-9A-Fa-f]{6}$"}
        },
        "required": ["id", "name", "steps"]
      }
    }
  }
}
```

### 16.3 Integration with Schema Generation

Extend the component registry schema generation to include workflows:

```go
// cmd/schema-exporter/main.go

func exportWorkflowSchemas(outputDir string) error {
    schemas := []struct {
        Name   string
        Schema any
    }{
        {"workflow-definition", WorkflowDefSchema},
        {"workflow-ui-metadata", UIMetadataSchema},
        {"workflow-execution", ExecutionSchema},
    }
    
    for _, s := range schemas {
        path := filepath.Join(outputDir, s.Name+".v1.json")
        // ... export logic
    }
    
    return nil
}
```

### 16.4 Visual Builder Integration

The flow builder UI uses the schema for:

| Feature | Schema Source |
|---------|---------------|
| Step palette | Action type enum + descriptions |
| Property panels | Step/action property schemas |
| Validation | Required fields, patterns, constraints |
| Auto-complete | Enum values, variable references |
| Visual layout | `_ui.steps.{name}.position` |
| Grouping | `_ui.groups` |

```typescript
// semstreams-ui/src/lib/workflow/types.ts

interface WorkflowDef {
  id: string;
  name: string;
  trigger: TriggerConfig;
  steps: StepDef[];
  
  // UI metadata (stripped before saving to backend)
  _ui?: WorkflowUIMetadata;
}

interface WorkflowUIMetadata {
  color?: string;
  icon?: string;
  category?: string;
  tags?: string[];
  canvas?: { zoom: number; pan: { x: number; y: number } };
  steps?: Record<string, StepUIMetadata>;
  groups?: GroupDef[];
}
```

### 16.5 Schema Export for TypeScript

Add workflow types to the OpenAPI spec generation:

```yaml
# specs/openapi.v3.yaml (generated)

components:
  schemas:
    WorkflowDef:
      $ref: 'schemas/workflow-definition.v1.json'
    WorkflowExecution:
      $ref: 'schemas/workflow-execution.v1.json'
    WorkflowUIMetadata:
      $ref: 'schemas/workflow-ui-metadata.v1.json'
      
paths:
  /workflows:
    get:
      summary: List workflow definitions
      responses:
        '200':
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/WorkflowDef'
    post:
      summary: Create workflow definition
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/WorkflowDef'
              
  /workflows/{id}/executions:
    get:
      summary: List workflow executions
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/WorkflowExecution'
```

---

## Appendix A: Migration from Pre-ADR-020 Workflows

For workflows using the old string interpolation pattern:

| Old Pattern | New Pattern (ADR-020) |
|-------------|----------------------|
| `"payload": {"id": "${trigger.entity_id}"}` | `"inputs": {"id": {"from": "trigger.entity_id"}}` |
| `"payload_mapping": {"data": "steps.fetch.output"}` | `"inputs": {"data": {"from": "fetch.result"}}` |
| `"input_type": "request.v1"` | `"inputs": {"data": {"interface": "request.v1"}}` |
| `"output_type": "response.v1"` | `"outputs": {"result": {"interface": "response.v1"}}` |
| `"pass_through": true` | Explicit input/output declarations |

**Key Changes:**
- Remove `payload` field from actions - data comes from `inputs`
- Remove `payload_mapping` - use `inputs` with `from` references
- Remove `input_type`/`output_type` - use `interface` on inputs/outputs
- Remove `pass_through` - declare what you need explicitly

## Appendix B: Migration from Go-code Workflows

For existing SemMem Go-code workflows, migration path:

| Go Code Pattern | Workflow Equivalent |
|-----------------|---------------------|
| `w.store.Get(ctx, id)` | Step with `{"type": "call", "subject": "semmem.spec.get"}` |
| `w.github.CreateIssue(...)` | Step with `{"type": "call", "subject": "semmem.github.create-issues"}` |
| `w.store.AddTriple(...)` | Step with `{"type": "set_state"}` |
| `for i := 0; i < retries; i++` | `"retry": {"max_attempts": N}` |
| `time.Sleep(backoff)` | Automatic with retry policy |
| Error handling | `"on_fail": "step-name"` or `"abort"` |

The domain-specific logic (parsing specs, GitHub API calls) remains in Go services that respond to NATS request/response.

---

## Appendix C: Future Extensions

Reserved for post-MVP development:

| Extension | Description |
|-----------|-------------|
| Parallel steps | Execute multiple steps concurrently |
| Child workflows | Invoke workflow from workflow |
| Signals | External input during execution |
| Saga pattern | Compensation on failure |
| Human tasks | Wait for manual approval |
| Workflow versioning | Handle in-flight executions during upgrade |
