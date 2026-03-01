# ADR-011: Workflow Processor

## Status

Superseded by ADR-021

> **Note**: This ADR describes the original JSON-based workflow processor.
> It has been superseded by [ADR-021: Reactive Workflow Engine](./adr-021-reactive-workflow-engine.md),
> which provides compile-time type safety and eliminates serialization bugs.
>
> See [Reactive Workflows Guide](../advanced/10-reactive-workflows.md) for current documentation.

## Context

SemStreams has two execution models with a gap between them:

| Model | Characteristics | Use Case |
|-------|-----------------|----------|
| **Rules Processor** | Reactive, stateless per-event, immediate | State detection, event publish, simple mutations |
| **External Orchestration** | Full-featured (Temporal, etc.) | Complex sagas, human tasks, long-running processes |

**The gap**: Multi-step sequences that need durability, retry, and timeouts but don't warrant external orchestration infrastructure.

Examples:
- Spec approved → extract tasks → create GitHub issues → link to spec → update status
- Drone mission → preflight → takeoff → navigate → execute → return → land
- Daily report → collect metrics → generate summary → notify team

These require:
- Sequential step execution with state between steps
- Retry with exponential backoff on transient failures
- Step and workflow timeouts
- Durable state that survives restarts
- Conditional branching (on_success/on_fail)

## Decision

Implement a workflow processor with declarative JSON definitions, durable execution state, and composable actions.

### Core Design Principles

| Principle | Rationale |
|-----------|-----------|
| **Declarative** | JSON definitions enable AI generation and UI editing |
| **Durable** | Execution state in NATS KV survives restarts |
| **Composable** | Actions are NATS request/response; any service can implement |
| **Explicit** | No hidden behavior; debuggable and predictable |
| **Tiered** | Works at all tiers (0, 1, 2); no LLM dependency for core |

### Trigger Types

- **Rule trigger**: Workflow starts when rule fires (via ActionTypePublish from ADR-010)
- **Subject trigger**: Workflow starts on NATS message
- **Cron trigger**: Scheduled execution
- **Manual trigger**: API-only invocation

### Action Types

| Type | Description |
|------|-------------|
| `call` | NATS request/response; waits for result |
| `publish` | Fire-and-forget NATS publish |
| `set_state` | Entity state mutation via graph processor |
| `http` | External HTTP request |
| `wait` | Pause execution for duration |

### Storage

```
NATS KV Buckets:
├── WORKFLOW_DEFINITIONS  → Workflow definitions (persistent)
├── WORKFLOW_EXECUTIONS   → Execution state (TTL: 7d)
├── WORKFLOW_TIMERS       → Scheduled timers
├── WORKFLOW_SECRETS      → Encrypted secrets
└── WORKFLOW_IDEMPOTENCY  → Deduplication keys (TTL: 24h)
```

### Non-Goals

- **Not a general orchestrator**: Focused on SemStreams patterns
- **Not Temporal replacement**: For complex needs, use Temporal alongside
- **Not a BPM engine**: No human task assignment or forms
- **Not real-time**: Millisecond latency acceptable, not microseconds

## Consequences

### Positive

- **Bridges the gap**: Fills space between reactive rules and heavy orchestration
- **Declarative definitions**: JSON workflows are AI-generatable and UI-editable
- **Native integration**: Rules trigger workflows; workflows mutate graph state
- **No external dependencies**: Uses existing NATS infrastructure
- **Tier-agnostic**: Core execution works without LLM

### Negative

- **New KV buckets**: Five additional buckets to manage and monitor
- **Timer recovery complexity**: On restart, must reload and reschedule all timers
- **Learning curve**: New abstraction for users to understand
- **Testing surface**: Workflow execution paths need comprehensive coverage

### Neutral

- **Schema generation**: Workflow schemas added to component registry export
- **Metrics namespace**: New `semstreams_workflow_*` Prometheus metrics
- **UI integration**: `_ui` metadata for visual workflow builder (future)

## Implementation Plan

### Phase 1: Core Infrastructure
- WorkflowProcessor component skeleton
- KV bucket management
- Workflow definition registry

### Phase 2: Execution Engine
- Execution state management
- Step executor with variable interpolation
- Basic actions: call, publish, set_state

### Phase 3: Reliability
- Retry with exponential backoff
- Step and workflow timeouts
- Timer service

### Phase 4: Advanced Features
- HTTP action
- Wait action
- Idempotency
- Secrets management

### Phase 5: Observability
- Prometheus metrics
- Structured logging
- Execution events

## Key Files

| File | Purpose |
|------|---------|
| `processor/workflow/processor.go` | Main component |
| `processor/workflow/registry.go` | Definition storage |
| `processor/workflow/execution.go` | State management |
| `processor/workflow/executor.go` | Step execution |
| `processor/workflow/timer.go` | Timer service |
| `processor/workflow/actions/` | Action implementations |

## References

- **Depends on**: [ADR-010: Rules Processor Completion](./adr-010-rules-processor-completion.md)
- **Related**: [ADR-018: Agentic Workflow Orchestration](./adr-018-agentic-workflow-orchestration.md)
- **Concept**: [Orchestration Layers](../concepts/14-orchestration-layers.md)
- **Full specification**: [Workflow Processor Spec](./specs/workflow-processor-spec.md)
