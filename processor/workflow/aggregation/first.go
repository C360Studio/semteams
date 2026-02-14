package aggregation

import (
	"context"

	"github.com/c360studio/semstreams/pkg/types"
)

// FirstAggregator returns the first successful result
type FirstAggregator struct{}

// Name returns the aggregator name
func (a *FirstAggregator) Name() string {
	return "first"
}

// Aggregate returns the first successful result
func (a *FirstAggregator) Aggregate(_ context.Context, results []AgentResult) (*AggregatedResult, error) {
	successCount, failureCount, failedSteps := countResults(results)

	// Find first success
	for _, r := range results {
		if r.Status == types.StatusSuccess {
			return &AggregatedResult{
				Success:        true,
				Output:         r.Output,
				FailedSteps:    failedSteps,
				SuccessCount:   successCount,
				FailureCount:   failureCount,
				AggregatorUsed: a.Name(),
			}, nil
		}
	}

	// No successes
	return &AggregatedResult{
		Success:        false,
		FailedSteps:    failedSteps,
		SuccessCount:   0,
		FailureCount:   failureCount,
		MergedErrors:   collectErrors(results),
		AggregatorUsed: a.Name(),
	}, nil
}
