# Context Construction

Building focused context for agentic tasks using SemStreams building blocks.

## The Problem

Traditional agentic systems suffer from context problems:

1. **Discovery overhead** - Agents spend tokens figuring out what they need
2. **Unknown budgets** - Token count isn't known until runtime
3. **Context pollution** - Accumulated conversation dilutes relevant information
4. **Redundant queries** - Multiple agents rediscover the same entities

Consider a code review workflow with 3 parallel reviewers. Each agent:

- Queries the graph for relevant entities
- Discovers relationships
- Builds its own context

This wastes tokens on discovery, creates inconsistent contexts, and makes token budgets unpredictable.

## The Pattern: Embed Context, Don't Discover It

The solution is to construct context **before** dispatching agents:

```text
┌─────────────────┐      ┌───────────────────┐      ┌─────────────────┐
│   Task Arrives  │─────▶│ Build Context     │─────▶│ Dispatch Agent  │
│                 │      │ (consumer logic)  │      │ (with context)  │
└─────────────────┘      └───────────────────┘      └─────────────────┘
                                │
                                │ Uses pkg/context utilities:
                                │ - BatchQueryEntities
                                │ - FormatEntitiesForContext
                                │ - EstimateTokens
                                │
                                ▼
                         ┌──────────────────┐
                         │ ConstructedContext │
                         │ - Content (string) │
                         │ - TokenCount (int) │
                         │ - Entities (IDs)   │
                         │ - Sources (trace)  │
                         └──────────────────┘
```

Benefits:

- **Exact token budgets** - Know size before dispatch
- **Fresh context per task** - No pollution from prior work
- **Source tracking** - Trace which entities informed decisions
- **Consistent context** - Multiple agents can share the same base context

## Building Blocks

SemStreams provides utilities in `pkg/context/` that any consumer can use.

### Token Estimation

Manage context budgets precisely:

```go
import "github.com/c360studio/semstreams/pkg/context"

// Check if content fits
if context.FitsInBudget(content, 8000) {
    // Use as-is
}

// Truncate to fit
truncated := context.TruncateToBudget(content, 8000)

// Estimate tokens
tokens := context.EstimateTokens(content)
```

### Budget Allocation

Distribute tokens across sections:

```go
budget := context.NewBudgetAllocation(10000)
budget.Allocate("system_prompt", 500)
budget.Allocate("entities", 4000)
budget.Allocate("relationships", 2000)
remaining := budget.Remaining() // 3500 for conversation
```

### Batch Graph Queries

Efficiently fetch entities and relationships:

```go
result, err := context.BatchQueryEntitiesWithOptions(ctx, graphClient, entityIDs,
    context.BatchQueryOptions{
        IncludeRelationships: true,
        Depth:                1,
        MaxConcurrent:        10,
    })
// result.Entities: map[string]json.RawMessage
// result.Relationships: []Relationship
// result.NotFound: []string
```

### Context Formatting

Prepare content for LLM consumption:

```go
opts := context.FormatOptions{
    MaxTokens:      8000,
    PrettyPrint:    true,
    SectionHeaders: true,
}

// Format entities
content, tokens, err := context.FormatEntitiesForContext(entities, opts)

// Or build full ConstructedContext
constructed, err := context.BuildContextFromBatch(result, opts)
```

## ConstructedContext

The `ConstructedContext` type wraps everything needed for embedded context:

```go
type ConstructedContext struct {
    Content       string          // Formatted string for LLM
    TokenCount    int             // Exact token count
    Entities      []string        // Entity IDs included
    Sources       []ContextSource // Provenance tracking
    ConstructedAt time.Time       // For cache management
}
```

Source types track where context came from:

- `graph_entity` - From a knowledge graph entity
- `graph_relationship` - From graph relationships
- `document` - From a document or chunk

## Consumer Pattern

SemStreams provides the HOW (building blocks). Consumers decide the WHAT (relevance).

**Why this separation?**

"What's relevant" is domain knowledge:

- Code review system: recent commits, related files, SOPs
- Logistics system: current missions, nearby assets, schedules
- Healthcare system: patient history, protocols, medications

A framework shouldn't embed domain-specific heuristics.

**Consumer implementation:**

```text
SemSpec flow:
1. Task arrives: "Review authentication changes"
2. SemSpec's context planner analyzes task (domain logic)
3. Determines: "I need auth-related entities, recent commits, SOPs"
4. Uses pkg/context utilities to fetch and format
5. Embeds ConstructedContext in TaskMessage
6. Dispatches agent with pre-built context
```

## Integration with Workflows

Embed context in `publish_agent` actions:

```json
{
  "name": "review",
  "action": {
    "type": "publish_agent",
    "role": "reviewer",
    "prompt": "Review the following code changes",
    "context": "${steps.build_context.output}"
  }
}
```

The `build_context` step produces a `ConstructedContext` that flows through variable interpolation.

For parallel agents, build separate contexts:

```json
{
  "name": "parallel_review",
  "type": "parallel",
  "steps": [
    {
      "name": "sop_review",
      "action": {
        "type": "publish_agent",
        "role": "sop_reviewer",
        "prompt": "Check SOP compliance",
        "context": "${steps.build_context.output.sop_context}"
      }
    },
    {
      "name": "style_review",
      "action": {
        "type": "publish_agent",
        "role": "style_reviewer",
        "prompt": "Check code style",
        "context": "${steps.build_context.output.style_context}"
      }
    }
  ]
}
```

## Example: Building Context for Code Review

```go
func buildReviewContext(ctx context.Context, client context.GraphClient, fileIDs []string) (*context.ConstructedContext, error) {
    // Query files and their relationships
    result, err := context.BatchQueryEntitiesWithOptions(ctx, client, fileIDs,
        context.BatchQueryOptions{
            IncludeRelationships: true,
            Depth:                1,
        })
    if err != nil {
        return nil, err
    }

    // Format with budget
    opts := context.FormatOptions{
        MaxTokens:      6000,
        PrettyPrint:    true,
        SectionHeaders: true,
    }

    constructed, err := context.BuildContextFromBatch(result, opts)
    if err != nil {
        return nil, err
    }

    // Token count is exact
    log.Printf("Built context with %d tokens for %d entities",
        constructed.TokenCount, len(constructed.Entities))

    return constructed, nil
}
```

## Related Documentation

- [Agentic Systems](11-agentic-systems.md) - Overview of agentic loop
- [Parallel Agents](23-parallel-agents.md) - Parallel execution with context
- [Workflow Configuration](../advanced/09-workflow-configuration.md) - Workflow processor
- [pkg/context README](../../pkg/context/README.md) - Package documentation
