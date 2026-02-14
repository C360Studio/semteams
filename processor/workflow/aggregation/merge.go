package aggregation

import (
	"context"
	"encoding/json"

	"github.com/c360studio/semstreams/pkg/types"
)

// MergeAggregator deep merges JSON objects from all successful results
type MergeAggregator struct{}

// Name returns the aggregator name
func (a *MergeAggregator) Name() string {
	return "merge"
}

// Aggregate deep merges all successful JSON objects
func (a *MergeAggregator) Aggregate(_ context.Context, results []AgentResult) (*AggregatedResult, error) {
	successCount, failureCount, failedSteps := countResults(results)

	// Start with empty object
	merged := make(map[string]any)

	// Merge each successful result
	for _, r := range results {
		if r.Status != types.StatusSuccess || len(r.Output) == 0 {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal(r.Output, &obj); err != nil {
			// Not a JSON object, skip
			continue
		}

		deepMerge(merged, obj)
	}

	// Marshal merged result
	output, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}

	return &AggregatedResult{
		Success:        failureCount == 0 || successCount > 0,
		Output:         output,
		FailedSteps:    failedSteps,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		MergedErrors:   collectErrors(results),
		AggregatorUsed: a.Name(),
	}, nil
}

// deepMerge recursively merges src into dst
func deepMerge(dst, src map[string]any) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		// If both are maps, merge recursively
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			deepMerge(dstMap, srcMap)
			continue
		}

		// If both are slices, concatenate
		srcSlice, srcIsSlice := srcVal.([]any)
		dstSlice, dstIsSlice := dstVal.([]any)
		if srcIsSlice && dstIsSlice {
			dst[key] = append(dstSlice, srcSlice...)
			continue
		}

		// Otherwise, source wins
		dst[key] = srcVal
	}
}
