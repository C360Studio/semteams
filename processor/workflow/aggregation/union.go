package aggregation

import (
	"context"
	"encoding/json"
)

// UnionAggregator combines all outputs into an array
type UnionAggregator struct{}

// Name returns the aggregator name
func (a *UnionAggregator) Name() string {
	return "union"
}

// Aggregate combines all results into a single array output
func (a *UnionAggregator) Aggregate(_ context.Context, results []AgentResult) (*AggregatedResult, error) {
	successCount, failureCount, failedSteps := countResults(results)

	// Collect all outputs (including from failed steps if they have output)
	var outputs []json.RawMessage
	for _, r := range results {
		if len(r.Output) > 0 {
			outputs = append(outputs, r.Output)
		}
	}

	// Create array output
	output, err := json.Marshal(outputs)
	if err != nil {
		return nil, err
	}

	return &AggregatedResult{
		Success:        failureCount == 0,
		Output:         output,
		FailedSteps:    failedSteps,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		MergedErrors:   collectErrors(results),
		AggregatorUsed: a.Name(),
	}, nil
}
