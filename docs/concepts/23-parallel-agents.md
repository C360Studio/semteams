# Parallel Agent Execution

Running multiple agents concurrently and aggregating their results.

## Overview

Some tasks benefit from multiple perspectives. Code review can examine security, style, and SOP compliance simultaneously. Analysis can gather data from multiple sources concurrently. Parallel agent execution enables these patterns efficiently.

```text
                    ┌────────────────┐
                    │  Parallel Step │
                    └───────┬────────┘
                            │
            ┌───────────────┼───────────────┐
            │               │               │
            ▼               ▼               ▼
     ┌──────────┐    ┌──────────┐    ┌──────────┐
     │ Agent 1  │    │ Agent 2  │    │ Agent 3  │
     │(reviewer)│    │(reviewer)│    │(reviewer)│
     └────┬─────┘    └────┬─────┘    └────┬─────┘
          │               │               │
          └───────────────┼───────────────┘
                          │
                    ┌─────▼─────┐
                    │ Aggregator│
                    └─────┬─────┘
                          │
                    ┌─────▼─────┐
                    │  Result   │
                    └───────────┘
```

## Parallel Steps

Define parallel steps in workflow definitions using `type: "parallel"`:

```json
{
  "name": "parallel_review",
  "type": "parallel",
  "steps": [
    {
      "name": "security_review",
      "action": {
        "type": "publish_agent",
        "role": "security_reviewer",
        "prompt": "Review for security vulnerabilities"
      }
    },
    {
      "name": "style_review",
      "action": {
        "type": "publish_agent",
        "role": "style_reviewer",
        "prompt": "Review code style and patterns"
      }
    },
    {
      "name": "sop_review",
      "action": {
        "type": "publish_agent",
        "role": "sop_reviewer",
        "prompt": "Check SOP compliance"
      }
    }
  ],
  "wait": "all",
  "aggregator": "union"
}
```

Each nested step runs as an independent agent task. The workflow processor tracks all tasks and applies the aggregator when they complete.

## Wait Semantics

The `wait` field controls when the parallel step completes:

| Wait | Behavior | Use Case |
|------|----------|----------|
| `all` | Wait for all steps to complete | Need results from everyone |
| `any` | Continue on first success | Need one good answer |
| `majority` | Wait for >50% success | Consensus decisions |

### wait: "all"

All nested steps must complete before aggregation. Failed steps are tracked and included in the aggregated result.

```json
{
  "wait": "all",
  "aggregator": "union"
}
```

### wait: "any"

The parallel step completes when any nested step succeeds. Remaining steps may continue running (for resource cleanup) but their results are ignored.

```json
{
  "wait": "any",
  "aggregator": "first"
}
```

### wait: "majority"

The parallel step completes when >50% of nested steps have a result (success or failure). If majority succeeded, aggregation proceeds. Otherwise, the step fails.

```json
{
  "wait": "majority",
  "aggregator": "majority"
}
```

## Result Aggregation

Aggregators combine results from parallel steps into a single output.

### Built-in Aggregators

| Aggregator | Success Condition | Output |
|------------|-------------------|--------|
| `union` | All succeed | Array of all outputs |
| `first` | Any succeed | First successful output |
| `majority` | >50% succeed | Array of successful outputs |
| `merge` | Any succeed | Deep-merged JSON object |
| `entity_merge` | Any succeed | Entity-keyed merged object |

### union

Combines all outputs into a JSON array. Only succeeds if all steps succeed.

```go
// Inputs:
//   reviewer1: {"score": 8, "issues": []}
//   reviewer2: {"score": 9, "issues": ["minor"]}

// Output:
// [{"score": 8, "issues": []}, {"score": 9, "issues": ["minor"]}]
```

### first

Returns the first successful result. Useful with `wait: "any"`.

```go
// First success returns immediately
// reviewer1: (still running)
// reviewer2: {"approved": true}

// Output: {"approved": true}
```

### majority

Returns array of successful outputs. Requires >50% success for overall success.

```go
// 3 reviewers, 2 succeed:
//   reviewer1: {"vote": "approve"}
//   reviewer2: {"vote": "approve"}
//   reviewer3: (failed)

// Output: [{"vote": "approve"}, {"vote": "approve"}]
// Success: true (2/3 > 50%)
```

### merge

Deep merges JSON objects from successful results.

```go
// Inputs:
//   agent1: {"metrics": {"cpu": 50}, "status": "ok"}
//   agent2: {"metrics": {"memory": 75}, "alerts": []}

// Output:
// {"metrics": {"cpu": 50, "memory": 75}, "status": "ok", "alerts": []}
```

### entity_merge

Merges results with entity deduplication. When multiple agents analyze the same entity, their findings are combined.

```go
// Inputs with overlapping entities:
//   reviewer1: {"entity_id": "file.go", "security_score": 8}
//   reviewer2: {"entity_id": "file.go", "style_score": 9}

// Output:
// {
//   "entities": {
//     "file.go": {"entity_id": "file.go", "security_score": 8, "style_score": 9}
//   }
// }
```

## Depth Tracking

Nested agents can spawn sub-agents. Depth tracking prevents runaway recursion.

```json
{
  "action": {
    "type": "publish_agent",
    "role": "coordinator",
    "max_depth": 3
  }
}
```

Fields:

- `depth` - Current depth in agent tree (0 = root)
- `max_depth` - Maximum allowed depth
- `parent_loop_id` - Parent agent loop ID (for tracing)

When an agent at `depth == max_depth` tries to spawn a sub-agent, the spawn is rejected with an error.

## Example: Multi-Perspective Code Review

```json
{
  "id": "code-review-workflow",
  "name": "Multi-Perspective Code Review",
  "trigger": {"subject": "workflow.trigger.review"},
  "steps": [
    {
      "name": "build_context",
      "action": {
        "type": "call",
        "subject": "context.build",
        "payload": {"file_ids": "${trigger.payload.files}"}
      }
    },
    {
      "name": "parallel_review",
      "type": "parallel",
      "steps": [
        {
          "name": "security_review",
          "action": {
            "type": "publish_agent",
            "role": "security_reviewer",
            "model": "claude-sonnet",
            "prompt": "Review for OWASP Top 10 vulnerabilities",
            "context": "${steps.build_context.output.security_context}"
          }
        },
        {
          "name": "style_review",
          "action": {
            "type": "publish_agent",
            "role": "style_reviewer",
            "model": "gpt-4o",
            "prompt": "Review for code style and Go best practices",
            "context": "${steps.build_context.output.style_context}"
          }
        },
        {
          "name": "sop_review",
          "action": {
            "type": "publish_agent",
            "role": "sop_reviewer",
            "model": "claude-sonnet",
            "prompt": "Check compliance with team SOPs",
            "context": "${steps.build_context.output.sop_context}"
          }
        }
      ],
      "wait": "all",
      "aggregator": "union"
    },
    {
      "name": "synthesize",
      "action": {
        "type": "publish_agent",
        "role": "synthesizer",
        "prompt": "Combine review findings: ${steps.parallel_review.output}"
      }
    }
  ]
}
```

## Fresh Context Per Agent

Each parallel agent receives its own context, preventing cross-contamination:

```text
                    ┌─────────────────┐
                    │ build_context   │
                    │ (single step)   │
                    └────────┬────────┘
                             │
          ┌──────────────────┼──────────────────┐
          │                  │                  │
          ▼                  ▼                  ▼
   ┌────────────┐     ┌────────────┐     ┌────────────┐
   │ Security   │     │ Style      │     │ SOP        │
   │ Context    │     │ Context    │     │ Context    │
   │ (focused)  │     │ (focused)  │     │ (focused)  │
   └─────┬──────┘     └─────┬──────┘     └─────┬──────┘
         │                  │                  │
         ▼                  ▼                  ▼
   ┌────────────┐     ┌────────────┐     ┌────────────┐
   │ Agent 1    │     │ Agent 2    │     │ Agent 3    │
   └────────────┘     └────────────┘     └────────────┘
```

Each context is constructed specifically for that agent's task using [pkg/context](../../pkg/context/README.md) building blocks.

## Observability

Parallel execution emits metrics:

| Metric | Description |
|--------|-------------|
| `workflow_parallel_steps_total` | Parallel steps started |
| `workflow_parallel_tasks_total` | Tasks spawned in parallel steps |
| `workflow_aggregation_duration_seconds` | Time spent aggregating |

Each task includes:
- `parent_step` - Name of the parallel step
- `parallel_index` - Position in parallel set
- Inherited trace context for distributed tracing

## Related Documentation

- [Workflow Configuration](../advanced/09-workflow-configuration.md) - Full workflow reference
- [Context Construction](22-context-construction.md) - Building agent context
- [Agentic Systems](11-agentic-systems.md) - Core agentic concepts
- [Aggregation Package](../../processor/workflow/aggregation/README.md) - Aggregator details
