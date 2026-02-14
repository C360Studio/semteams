package aggregation

import (
	"context"
	"encoding/json"

	"github.com/c360studio/semstreams/pkg/types"
)

// EntityMergeAggregator merges results with entity deduplication.
// When multiple agents reason about the same entity, it merges their conclusions.
type EntityMergeAggregator struct{}

// Name returns the aggregator name
func (a *EntityMergeAggregator) Name() string {
	return "entity_merge"
}

// Aggregate merges results by entity ID, deduplicating and combining conclusions
func (a *EntityMergeAggregator) Aggregate(_ context.Context, results []AgentResult) (*AggregatedResult, error) {
	successCount, failureCount, failedSteps := countResults(results)

	// Track entities and their associated data across all results
	entityData := make(map[string][]json.RawMessage)
	otherData := []json.RawMessage{}

	// Collect entity references and outputs
	for _, r := range results {
		if r.Status != types.StatusSuccess || len(r.Output) == 0 {
			continue
		}

		// Try to parse output as object with entity references
		var obj map[string]any
		if err := json.Unmarshal(r.Output, &obj); err != nil {
			// Not a JSON object, add to other
			otherData = append(otherData, r.Output)
			continue
		}

		// Check for explicit entity_id field
		if entityID, ok := obj["entity_id"].(string); ok {
			entityData[entityID] = append(entityData[entityID], r.Output)
		} else if len(r.Entities) > 0 {
			// Use entities from result metadata
			for _, entityID := range r.Entities {
				entityData[entityID] = append(entityData[entityID], r.Output)
			}
		} else {
			// No entity association, add to other
			otherData = append(otherData, r.Output)
		}
	}

	// Merge data per entity
	mergedEntities := make(map[string]any)
	for entityID, outputs := range entityData {
		if len(outputs) == 1 {
			// Single output, use directly
			var parsed any
			if err := json.Unmarshal(outputs[0], &parsed); err == nil {
				mergedEntities[entityID] = parsed
			}
		} else {
			// Multiple outputs for same entity, merge them
			merged := make(map[string]any)
			for _, output := range outputs {
				var obj map[string]any
				if err := json.Unmarshal(output, &obj); err == nil {
					deepMerge(merged, obj)
				}
			}
			mergedEntities[entityID] = merged
		}
	}

	// Build final output
	finalOutput := map[string]any{
		"entities": mergedEntities,
	}

	// Include non-entity data if present
	if len(otherData) > 0 {
		var other []any
		for _, data := range otherData {
			var parsed any
			if err := json.Unmarshal(data, &parsed); err == nil {
				other = append(other, parsed)
			}
		}
		finalOutput["other"] = other
	}

	// Count unique entities
	entityCount := len(mergedEntities)

	output, err := json.Marshal(finalOutput)
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
		EntityCount:    entityCount,
		AggregatorUsed: a.Name(),
	}, nil
}
