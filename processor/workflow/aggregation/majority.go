package aggregation

import (
	"context"
	"encoding/json"

	"github.com/c360studio/semstreams/pkg/types"
)

// MajorityAggregator requires >50% success for overall success
type MajorityAggregator struct{}

// Name returns the aggregator name
func (a *MajorityAggregator) Name() string {
	return "majority"
}

// Aggregate requires majority success and combines successful outputs
func (a *MajorityAggregator) Aggregate(_ context.Context, results []AgentResult) (*AggregatedResult, error) {
	if len(results) == 0 {
		return &AggregatedResult{
			Success:        false,
			AggregatorUsed: a.Name(),
		}, nil
	}

	successCount, failureCount, failedSteps := countResults(results)

	// Check if majority succeeded
	majoritySuccess := float64(successCount) > float64(len(results))/2

	// Collect successful outputs
	var successOutputs []json.RawMessage
	for _, r := range results {
		if r.Status == types.StatusSuccess && len(r.Output) > 0 {
			successOutputs = append(successOutputs, r.Output)
		}
	}

	// Create output array from successful results
	output, err := json.Marshal(successOutputs)
	if err != nil {
		return nil, err
	}

	return &AggregatedResult{
		Success:        majoritySuccess,
		Output:         output,
		FailedSteps:    failedSteps,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		MergedErrors:   collectErrors(results),
		AggregatorUsed: a.Name(),
	}, nil
}
