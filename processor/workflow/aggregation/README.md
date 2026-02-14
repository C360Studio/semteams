# aggregation

Result aggregation strategies for parallel workflow steps.

## Overview

When a workflow executes parallel steps, multiple agents run concurrently and produce independent results. This package provides strategies to combine those results into a single output for the next workflow step.

## Quick Start

```go
import "github.com/c360studio/semstreams/processor/workflow/aggregation"

results := []aggregation.AgentResult{
    {StepName: "reviewer1", Status: "success", Output: json.RawMessage(`{"score": 8}`)},
    {StepName: "reviewer2", Status: "success", Output: json.RawMessage(`{"score": 9}`)},
}

// Use default registry
aggregated, err := aggregation.DefaultRegistry.Aggregate(ctx, "union", results)
if err != nil {
    return err
}

fmt.Printf("Success: %v, Output: %s\n", aggregated.Success, aggregated.Output)
// Success: true, Output: [{"score": 8}, {"score": 9}]
```

## Built-in Aggregators

### union

Combines all outputs into a JSON array. Only succeeds if all steps succeed.

```go
// Input:
//   reviewer1: {"score": 8}
//   reviewer2: {"score": 9}

// Output (Success=true):
// [{"score": 8}, {"score": 9}]
```

**Use when:** You need all agents to succeed and want all their outputs.

### first

Returns the first successful result. Succeeds if any step succeeds.

```go
// Input:
//   reviewer1: failed
//   reviewer2: {"approved": true}
//   reviewer3: {"approved": false}

// Output (Success=true):
// {"approved": true}
```

**Use when:** You only need one successful result (e.g., "any success" pattern).

### majority

Requires >50% success. Returns array of successful outputs.

```go
// Input (3 results, 2 succeed):
//   reviewer1: {"vote": "approve"}
//   reviewer2: {"vote": "approve"}
//   reviewer3: failed

// Output (Success=true):
// [{"vote": "approve"}, {"vote": "approve"}]
```

**Use when:** You want consensus-based decisions (majority vote).

### merge

Deep merges JSON objects from successful results.

```go
// Input:
//   agent1: {"metrics": {"cpu": 50}}
//   agent2: {"metrics": {"memory": 75}}

// Output (Success=true):
// {"metrics": {"cpu": 50, "memory": 75}}
```

**Use when:** Multiple agents produce complementary data to combine.

**Merge rules:**
- Maps: Recursively merge
- Arrays: Concatenate
- Other types: Later values overwrite earlier

### entity_merge

Merges results with entity deduplication. When multiple agents reason about the same entity, their conclusions are merged.

```go
// Input:
//   reviewer1: {"entity_id": "drone.001", "status": "ok", "battery": 80}
//   reviewer2: {"entity_id": "drone.001", "status": "ok", "location": "zone-a"}
//   reviewer3: {"entity_id": "drone.002", "status": "warning"}

// Output (Success=true):
// {
//   "entities": {
//     "drone.001": {"entity_id": "drone.001", "status": "ok", "battery": 80, "location": "zone-a"},
//     "drone.002": {"entity_id": "drone.002", "status": "warning"}
//   }
// }
```

**Use when:** Multiple agents analyze the same entities and you want deduplicated results.

## Aggregator Comparison

| Aggregator | Success Condition | Output Format | Use Case |
|------------|-------------------|---------------|----------|
| union | All succeed | Array of all outputs | All-or-nothing |
| first | Any succeed | First success only | Any-success |
| majority | >50% succeed | Array of successes | Consensus |
| merge | Any succeed | Merged JSON object | Complementary data |
| entity_merge | Any succeed | Entity-keyed object | Entity analysis |

## Core Types

### AgentResult

Input to aggregators - one per parallel step:

```go
type AgentResult struct {
    StepName string          // Step identifier
    Status   string          // "success" or "failed"
    Output   json.RawMessage // JSON output (may be empty)
    Error    string          // Error message if failed
    TaskID   string          // Agent task ID for tracing
    Entities []string        // Entity IDs referenced (for entity_merge)
}
```

### AggregatedResult

Output from aggregators:

```go
type AggregatedResult struct {
    Success        bool            // Overall success
    Output         json.RawMessage // Combined output
    FailedSteps    []string        // Names of failed steps
    SuccessCount   int             // Number of successes
    FailureCount   int             // Number of failures
    MergedErrors   string          // Combined error messages
    EntityCount    int             // Unique entities (entity_merge)
    AggregatorUsed string          // Aggregator name
}
```

## Custom Aggregators

Implement the `Aggregator` interface:

```go
type Aggregator interface {
    Name() string
    Aggregate(ctx context.Context, results []AgentResult) (*AggregatedResult, error)
}
```

Example:

```go
type WeightedAverageAggregator struct{}

func (a *WeightedAverageAggregator) Name() string { return "weighted_avg" }

func (a *WeightedAverageAggregator) Aggregate(ctx context.Context, results []AgentResult) (*AggregatedResult, error) {
    // Extract scores and compute weighted average
    var total float64
    var count int
    for _, r := range results {
        if r.Status == "success" {
            var data struct{ Score float64 }
            if json.Unmarshal(r.Output, &data) == nil {
                total += data.Score
                count++
            }
        }
    }

    avg := total / float64(count)
    output, _ := json.Marshal(map[string]float64{"average": avg})

    return &AggregatedResult{
        Success:        count > 0,
        Output:         output,
        SuccessCount:   count,
        FailureCount:   len(results) - count,
        AggregatorUsed: a.Name(),
    }, nil
}

// Register
aggregation.DefaultRegistry.Register(&WeightedAverageAggregator{})
```

## Workflow Configuration

Configure in workflow step definitions:

```json
{
  "name": "parallel_review",
  "type": "parallel",
  "steps": [
    {
      "name": "sop_review",
      "action": {"type": "publish_agent", "role": "sop_reviewer", "prompt": "..."}
    },
    {
      "name": "style_review",
      "action": {"type": "publish_agent", "role": "style_reviewer", "prompt": "..."}
    },
    {
      "name": "security_review",
      "action": {"type": "publish_agent", "role": "security_reviewer", "prompt": "..."}
    }
  ],
  "wait": "all",
  "aggregator": "union"
}
```

### Wait Semantics

| Wait | Behavior | Typical Aggregator |
|------|----------|-------------------|
| `all` | Wait for all steps | union, merge |
| `any` | Continue on first success | first |
| `majority` | Wait for >50% | majority |

## Thread Safety

The `Registry` is thread-safe for concurrent access:

- `Register()` acquires write lock
- `Get()` acquires read lock
- `Aggregate()` acquires read lock then calls aggregator

Aggregators themselves should be stateless or handle their own synchronization.

## Related Documentation

- [Parallel Agents](../../../docs/concepts/23-parallel-agents.md) - Concept guide
- [Workflow Configuration](../../../docs/advanced/09-workflow-configuration.md) - Full reference
- [step_parallel.go](../step_parallel.go) - Parallel step execution
