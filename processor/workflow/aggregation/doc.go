// Package aggregation provides result aggregation strategies for parallel workflow steps.
//
// # Overview
//
// When a workflow executes parallel steps, multiple agents run concurrently and produce
// independent results. This package provides strategies to combine those results into
// a single output for the next workflow step.
//
// Each aggregator implements different semantics for what constitutes success and how
// outputs are combined. Choose the aggregator based on your workflow's requirements.
//
// # Built-in Aggregators
//
// The following aggregators are registered in [DefaultRegistry] automatically:
//
//   - union: Combines all outputs into a JSON array, succeeds only if all steps succeed
//   - first: Returns the first successful result, useful for "any success" patterns
//   - majority: Requires >50% success, returns array of successful outputs
//   - merge: Deep merges JSON objects from successful results
//   - entity_merge: Merges with entity deduplication across results
//
// # Aggregator Comparison
//
//	| Aggregator   | Success Condition  | Output Format                    |
//	|--------------|--------------------|---------------------------------|
//	| union        | All succeed        | Array of all outputs            |
//	| first        | Any succeed        | Single first successful output  |
//	| majority     | >50% succeed       | Array of successful outputs     |
//	| merge        | Any succeed        | Merged JSON object              |
//	| entity_merge | Any succeed        | Entity-keyed merged object      |
//
// # Registry
//
// Aggregators are registered in a thread-safe [Registry]. The [DefaultRegistry] is
// pre-populated with all built-in aggregators:
//
//	// Use the default registry
//	result, err := aggregation.DefaultRegistry.Aggregate(ctx, "union", results)
//
//	// Or create a custom registry
//	registry := aggregation.NewRegistry()
//	registry.Register(&MyCustomAggregator{})
//
// # Custom Aggregators
//
// Implement the [Aggregator] interface to create custom strategies:
//
//	type Aggregator interface {
//	    Name() string
//	    Aggregate(ctx context.Context, results []AgentResult) (*AggregatedResult, error)
//	}
//
// Example custom aggregator:
//
//	type WeightedVoteAggregator struct {
//	    weights map[string]float64 // step name -> weight
//	}
//
//	func (a *WeightedVoteAggregator) Name() string { return "weighted_vote" }
//
//	func (a *WeightedVoteAggregator) Aggregate(ctx context.Context, results []AgentResult) (*AggregatedResult, error) {
//	    // Custom logic here
//	}
//
// Register custom aggregators:
//
//	aggregation.DefaultRegistry.Register(&WeightedVoteAggregator{weights: weights})
//
// # AgentResult Structure
//
// Each parallel step produces an [AgentResult] with:
//
//   - StepName: Identifier for the parallel step
//   - Status: "success" or "failed"
//   - Output: JSON output from the agent (may be empty on failure)
//   - Error: Error message if failed
//   - TaskID: Agent task ID for tracing
//   - Entities: Entity IDs referenced in output (for entity_merge)
//
// # AggregatedResult Structure
//
// Aggregators produce an [AggregatedResult] with:
//
//   - Success: Overall success based on aggregator semantics
//   - Output: Combined JSON output
//   - FailedSteps: Names of steps that failed
//   - SuccessCount: Number of successful steps
//   - FailureCount: Number of failed steps
//   - MergedErrors: Combined error messages from failed steps
//   - EntityCount: Unique entities after deduplication (entity_merge only)
//   - AggregatorUsed: Name of the aggregator that produced this result
//
// # Example Usage
//
//	results := []aggregation.AgentResult{
//	    {StepName: "reviewer1", Status: "success", Output: json.RawMessage(`{"score": 8}`)},
//	    {StepName: "reviewer2", Status: "success", Output: json.RawMessage(`{"score": 9}`)},
//	    {StepName: "reviewer3", Status: "failed", Error: "timeout"},
//	}
//
//	// Union aggregator - fails because reviewer3 failed
//	result, _ := aggregation.DefaultRegistry.Aggregate(ctx, "union", results)
//	// result.Success = false
//	// result.Output = [{"score": 8}, {"score": 9}]
//
//	// Majority aggregator - succeeds because 2/3 succeeded
//	result, _ = aggregation.DefaultRegistry.Aggregate(ctx, "majority", results)
//	// result.Success = true
//	// result.Output = [{"score": 8}, {"score": 9}]
//
//	// Merge aggregator - merges successful JSON objects
//	result, _ = aggregation.DefaultRegistry.Aggregate(ctx, "merge", results)
//	// result.Success = true
//	// result.Output = {"score": 9} (last value wins for same keys)
//
// # Workflow Integration
//
// Configure aggregators in workflow step definitions:
//
//	{
//	  "name": "parallel_review",
//	  "type": "parallel",
//	  "steps": [
//	    {"name": "reviewer1", "action": {...}},
//	    {"name": "reviewer2", "action": {...}},
//	    {"name": "reviewer3", "action": {...}}
//	  ],
//	  "wait": "all",
//	  "aggregator": "union"
//	}
//
// The workflow processor uses the specified aggregator when all parallel steps
// complete (based on wait semantics).
package aggregation
