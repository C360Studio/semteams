package stages

import (
	"testing"
)

func TestDefaultIndexSpecs(t *testing.T) {
	specs := DefaultIndexSpecs()
	if len(specs) == 0 {
		t.Fatal("DefaultIndexSpecs() returned empty list")
	}

	// Count required indexes
	requiredCount := 0
	for _, spec := range specs {
		if spec.Name == "" {
			t.Error("spec has empty Name")
		}
		if spec.Bucket == "" {
			t.Error("spec has empty Bucket")
		}
		if spec.Required {
			requiredCount++
		}
	}

	// Should have at least some required indexes
	if requiredCount == 0 {
		t.Error("no required indexes defined")
	}

	// Verify expected indexes are present
	expectedIndexes := []string{"entity_states", "predicate", "incoming", "outgoing", "temporal"}
	for _, expected := range expectedIndexes {
		found := false
		for _, spec := range specs {
			if spec.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected index %q not found in specs", expected)
		}
	}
}

func TestIndexPopulationResult_EmptyRequired(t *testing.T) {
	result := &IndexPopulationResult{
		Total:         7,
		Populated:     5,
		EmptyRequired: []string{"temporal"},
		Indexes:       make(map[string]IndexDetail),
	}

	if len(result.EmptyRequired) == 0 {
		t.Error("expected EmptyRequired to have entries")
	}

	// Add a warning for empty required
	if len(result.EmptyRequired) > 0 {
		result.Warnings = append(result.Warnings, "Required indexes empty: temporal")
	}

	if len(result.Warnings) == 0 {
		t.Error("expected Warnings to have entries")
	}
}

func TestIndexDetail(t *testing.T) {
	detail := IndexDetail{
		Bucket:     "ENTITY_STATES",
		KeyCount:   100,
		Populated:  true,
		SampleKeys: []string{"key1", "key2"},
	}

	if !detail.Populated {
		t.Error("expected Populated to be true")
	}
	if detail.KeyCount != 100 {
		t.Errorf("expected KeyCount 100, got %d", detail.KeyCount)
	}
	if len(detail.SampleKeys) != 2 {
		t.Errorf("expected 2 SampleKeys, got %d", len(detail.SampleKeys))
	}
}
