package aggregation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/pkg/types"
)

func TestUnionAggregator(t *testing.T) {
	agg := &UnionAggregator{}

	t.Run("all success", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusSuccess, Output: json.RawMessage(`{"a": 1}`)},
			{StepName: "step2", Status: types.StatusSuccess, Output: json.RawMessage(`{"b": 2}`)},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Error("expected success")
		}
		if result.SuccessCount != 2 {
			t.Errorf("expected 2 successes, got %d", result.SuccessCount)
		}
		if result.AggregatorUsed != "union" {
			t.Errorf("expected union aggregator, got %s", result.AggregatorUsed)
		}

		// Check output is array
		var outputs []json.RawMessage
		if err := json.Unmarshal(result.Output, &outputs); err != nil {
			t.Fatalf("output should be array: %v", err)
		}
		if len(outputs) != 2 {
			t.Errorf("expected 2 outputs, got %d", len(outputs))
		}
	})

	t.Run("mixed results", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusSuccess, Output: json.RawMessage(`{"a": 1}`)},
			{StepName: "step2", Status: types.StatusFailed, Error: "test error"},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Success {
			t.Error("expected failure due to failed step")
		}
		if result.FailureCount != 1 {
			t.Errorf("expected 1 failure, got %d", result.FailureCount)
		}
		if len(result.FailedSteps) != 1 {
			t.Errorf("expected 1 failed step, got %d", len(result.FailedSteps))
		}
	})
}

func TestFirstAggregator(t *testing.T) {
	agg := &FirstAggregator{}

	t.Run("returns first success", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusFailed, Error: "error1"},
			{StepName: "step2", Status: types.StatusSuccess, Output: json.RawMessage(`{"first": true}`)},
			{StepName: "step3", Status: types.StatusSuccess, Output: json.RawMessage(`{"second": true}`)},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Error("expected success")
		}

		var output map[string]bool
		if err := json.Unmarshal(result.Output, &output); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}
		if !output["first"] {
			t.Error("expected first successful result")
		}
	})

	t.Run("all failed", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusFailed, Error: "error1"},
			{StepName: "step2", Status: types.StatusFailed, Error: "error2"},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Success {
			t.Error("expected failure")
		}
		if result.SuccessCount != 0 {
			t.Errorf("expected 0 successes, got %d", result.SuccessCount)
		}
	})
}

func TestMajorityAggregator(t *testing.T) {
	agg := &MajorityAggregator{}

	t.Run("majority success", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusSuccess, Output: json.RawMessage(`{"a": 1}`)},
			{StepName: "step2", Status: types.StatusSuccess, Output: json.RawMessage(`{"b": 2}`)},
			{StepName: "step3", Status: types.StatusFailed, Error: "error"},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Error("expected success with 2/3 majority")
		}
	})

	t.Run("majority failure", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusFailed, Error: "error1"},
			{StepName: "step2", Status: types.StatusFailed, Error: "error2"},
			{StepName: "step3", Status: types.StatusSuccess, Output: json.RawMessage(`{"a": 1}`)},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Success {
			t.Error("expected failure with 2/3 failures")
		}
	})

	t.Run("empty results", func(t *testing.T) {
		result, err := agg.Aggregate(context.Background(), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Success {
			t.Error("expected failure for empty results")
		}
	})
}

func TestMergeAggregator(t *testing.T) {
	agg := &MergeAggregator{}

	t.Run("deep merge objects", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusSuccess, Output: json.RawMessage(`{"a": 1, "nested": {"x": 1}}`)},
			{StepName: "step2", Status: types.StatusSuccess, Output: json.RawMessage(`{"b": 2, "nested": {"y": 2}}`)},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Error("expected success")
		}

		var output map[string]any
		if err := json.Unmarshal(result.Output, &output); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}

		if output["a"] != float64(1) || output["b"] != float64(2) {
			t.Error("expected both top-level keys")
		}

		nested, ok := output["nested"].(map[string]any)
		if !ok {
			t.Fatal("expected nested object")
		}
		if nested["x"] != float64(1) || nested["y"] != float64(2) {
			t.Error("expected nested merge")
		}
	})

	t.Run("merge arrays", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusSuccess, Output: json.RawMessage(`{"items": [1, 2]}`)},
			{StepName: "step2", Status: types.StatusSuccess, Output: json.RawMessage(`{"items": [3, 4]}`)},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var output map[string]any
		if err := json.Unmarshal(result.Output, &output); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}

		items, ok := output["items"].([]any)
		if !ok {
			t.Fatal("expected items array")
		}
		if len(items) != 4 {
			t.Errorf("expected 4 items, got %d", len(items))
		}
	})
}

func TestEntityMergeAggregator(t *testing.T) {
	agg := &EntityMergeAggregator{}

	t.Run("merge by entity_id", func(t *testing.T) {
		results := []AgentResult{
			{
				StepName: "reviewer1",
				Status:   types.StatusSuccess,
				Output:   json.RawMessage(`{"entity_id": "drone.001", "safety": "ok"}`),
			},
			{
				StepName: "reviewer2",
				Status:   types.StatusSuccess,
				Output:   json.RawMessage(`{"entity_id": "drone.001", "performance": "good"}`),
			},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Error("expected success")
		}
		if result.EntityCount != 1 {
			t.Errorf("expected 1 unique entity, got %d", result.EntityCount)
		}

		var output map[string]any
		if err := json.Unmarshal(result.Output, &output); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}

		entities, ok := output["entities"].(map[string]any)
		if !ok {
			t.Fatal("expected entities map")
		}

		drone, ok := entities["drone.001"].(map[string]any)
		if !ok {
			t.Fatal("expected drone.001 entity")
		}

		// Check merged fields
		if drone["safety"] != "ok" {
			t.Error("expected safety field")
		}
		if drone["performance"] != "good" {
			t.Error("expected performance field")
		}
	})

	t.Run("merge via entities metadata", func(t *testing.T) {
		results := []AgentResult{
			{
				StepName: "step1",
				Status:   types.StatusSuccess,
				Output:   json.RawMessage(`{"score": 85}`),
				Entities: []string{"mission.alpha"},
			},
			{
				StepName: "step2",
				Status:   types.StatusSuccess,
				Output:   json.RawMessage(`{"status": "active"}`),
				Entities: []string{"mission.alpha"},
			},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.EntityCount != 1 {
			t.Errorf("expected 1 entity, got %d", result.EntityCount)
		}

		var output map[string]any
		if err := json.Unmarshal(result.Output, &output); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}

		entities := output["entities"].(map[string]any)
		mission := entities["mission.alpha"].(map[string]any)

		if mission["score"] != float64(85) || mission["status"] != "active" {
			t.Error("expected merged mission data")
		}
	})

	t.Run("separate entities", func(t *testing.T) {
		results := []AgentResult{
			{
				StepName: "step1",
				Status:   types.StatusSuccess,
				Output:   json.RawMessage(`{"entity_id": "drone.001", "battery": 80}`),
			},
			{
				StepName: "step2",
				Status:   types.StatusSuccess,
				Output:   json.RawMessage(`{"entity_id": "drone.002", "battery": 95}`),
			},
		}

		result, err := agg.Aggregate(context.Background(), results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.EntityCount != 2 {
			t.Errorf("expected 2 entities, got %d", result.EntityCount)
		}
	})
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	t.Run("default aggregators registered", func(t *testing.T) {
		for _, name := range []string{"union", "first", "majority", "merge", "entity_merge"} {
			if _, ok := registry.Get(name); !ok {
				t.Errorf("expected %s aggregator to be registered", name)
			}
		}
	})

	t.Run("aggregate via registry", func(t *testing.T) {
		results := []AgentResult{
			{StepName: "step1", Status: types.StatusSuccess, Output: json.RawMessage(`{"a": 1}`)},
		}

		result, err := registry.Aggregate(context.Background(), "union", results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Error("expected success")
		}
	})

	t.Run("unknown aggregator", func(t *testing.T) {
		_, err := registry.Aggregate(context.Background(), "nonexistent", nil)
		if err == nil {
			t.Error("expected error for unknown aggregator")
		}
	})
}
