package context

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "short text",
			input:    "hello world",
			expected: 2, // 11 chars / 4 = 2
		},
		{
			name:     "longer text",
			input:    "The quick brown fox jumps over the lazy dog.",
			expected: 11, // 44 chars / 4 = 11
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokens(tt.input)
			if got != tt.expected {
				t.Errorf("EstimateTokens() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFitsInBudget(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		budget   int
		expected bool
	}{
		{
			name:     "fits within budget",
			content:  "short text",
			budget:   100,
			expected: true,
		},
		{
			name:     "exceeds budget",
			content:  "This is a longer text that might exceed a small budget",
			budget:   5,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FitsInBudget(tt.content, tt.budget)
			if got != tt.expected {
				t.Errorf("FitsInBudget() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTruncateToBudget(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		budget int
	}{
		{
			name:   "no truncation needed",
			input:  "short",
			budget: 100,
		},
		{
			name:   "truncation needed",
			input:  "This is a very long text that needs to be truncated to fit within the budget",
			budget: 5,
		},
		{
			name:   "zero budget",
			input:  "any text",
			budget: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToBudget(tt.input, tt.budget)

			if tt.budget == 0 {
				if result != "" {
					t.Errorf("expected empty string for zero budget")
				}
				return
			}

			resultTokens := EstimateTokens(result)
			if resultTokens > tt.budget+1 { // Allow small margin for ellipsis
				t.Errorf("result tokens %d exceeds budget %d", resultTokens, tt.budget)
			}
		})
	}
}

func TestBudgetAllocation(t *testing.T) {
	t.Run("basic allocation", func(t *testing.T) {
		budget := NewBudgetAllocation(100)

		allocated := budget.Allocate("section1", 30)
		if allocated != 30 {
			t.Errorf("expected 30, got %d", allocated)
		}

		if budget.Remaining() != 70 {
			t.Errorf("expected 70 remaining, got %d", budget.Remaining())
		}
	})

	t.Run("exceeds budget", func(t *testing.T) {
		budget := NewBudgetAllocation(50)

		allocated := budget.Allocate("big_section", 100)
		if allocated != 50 {
			t.Errorf("expected 50 (capped), got %d", allocated)
		}

		if budget.Remaining() != 0 {
			t.Errorf("expected 0 remaining, got %d", budget.Remaining())
		}
	})

	t.Run("proportional allocation", func(t *testing.T) {
		budget := NewBudgetAllocation(100)

		result := budget.AllocateProportionally(
			[]string{"a", "b", "c"},
			[]float64{0.5, 0.3, 0.2},
		)

		if result["a"] != 50 {
			t.Errorf("expected 50 for 'a', got %d", result["a"])
		}
		if result["b"] != 30 {
			t.Errorf("expected 30 for 'b', got %d", result["b"])
		}
	})
}

func TestNewConstructedContext(t *testing.T) {
	content := "Test content for context"
	entities := []string{"entity1", "entity2"}
	sources := []Source{
		EntitySource("entity1"),
		EntitySource("entity2"),
	}

	ctx := NewConstructedContext(content, entities, sources)

	if ctx.Content != content {
		t.Errorf("content mismatch")
	}
	if ctx.TokenCount != EstimateTokens(content) {
		t.Errorf("token count mismatch: got %d, want %d", ctx.TokenCount, EstimateTokens(content))
	}
	if len(ctx.Entities) != 2 {
		t.Errorf("expected 2 entities")
	}
	if len(ctx.Sources) != 2 {
		t.Errorf("expected 2 sources")
	}
	if ctx.ConstructedAt.IsZero() {
		t.Error("ConstructedAt should be set")
	}
}

func TestFormatEntitiesForContext(t *testing.T) {
	entities := map[string]json.RawMessage{
		"entity.one": json.RawMessage(`{"name": "Entity One", "value": 42}`),
		"entity.two": json.RawMessage(`{"name": "Entity Two", "value": 100}`),
	}

	content, tokenCount, err := FormatEntitiesForContext(entities, DefaultFormatOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if content == "" {
		t.Error("expected non-empty content")
	}
	if tokenCount == 0 {
		t.Error("expected non-zero token count")
	}

	// Check structure
	if !contains(content, "entity.one") || !contains(content, "entity.two") {
		t.Error("content should contain entity IDs")
	}
}

func TestFormatRelationshipsForContext(t *testing.T) {
	relationships := []Relationship{
		{Subject: "a", Predicate: "relates_to", Object: "b"},
		{Subject: "b", Predicate: "connects", Object: "c"},
	}

	content, tokenCount, err := FormatRelationshipsForContext(relationships, DefaultFormatOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if content == "" {
		t.Error("expected non-empty content")
	}
	if tokenCount == 0 {
		t.Error("expected non-zero token count")
	}
	if !contains(content, "relates_to") {
		t.Error("content should contain relationship predicates")
	}
}

func TestCollectEntityIDs(t *testing.T) {
	relationships := []Relationship{
		{Subject: "a", Predicate: "rel", Object: "b"},
		{Subject: "b", Predicate: "rel", Object: "c"},
		{Subject: "a", Predicate: "rel", Object: "c"}, // Duplicate a and c
	}

	ids := CollectEntityIDs(relationships)

	// Should have 3 unique IDs
	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs, got %d", len(ids))
	}

	// Check all are present
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}
	for _, expected := range []string{"a", "b", "c"} {
		if !idMap[expected] {
			t.Errorf("missing expected ID: %s", expected)
		}
	}
}

// Mock graph client for testing
type mockGraphClient struct {
	entities      map[string]json.RawMessage
	relationships map[string][]Relationship
}

func (m *mockGraphClient) QueryEntities(_ context.Context, entityIDs []string) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage)
	for _, id := range entityIDs {
		if data, ok := m.entities[id]; ok {
			result[id] = data
		}
	}
	return result, nil
}

func (m *mockGraphClient) QueryRelationships(_ context.Context, entityID string, _ int) ([]Relationship, error) {
	if rels, ok := m.relationships[entityID]; ok {
		return rels, nil
	}
	return nil, nil
}

func TestBatchQueryEntities(t *testing.T) {
	client := &mockGraphClient{
		entities: map[string]json.RawMessage{
			"e1": json.RawMessage(`{"id": "e1"}`),
			"e2": json.RawMessage(`{"id": "e2"}`),
		},
	}

	result, err := BatchQueryEntities(context.Background(), client, []string{"e1", "e2", "e3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(result.Entities))
	}
	if len(result.NotFound) != 1 {
		t.Errorf("expected 1 not found, got %d", len(result.NotFound))
	}
	if result.NotFound[0] != "e3" {
		t.Errorf("expected e3 not found")
	}
}

func TestBuildContextFromBatch(t *testing.T) {
	batchResult := &BatchQueryResult{
		Entities: map[string]json.RawMessage{
			"entity.test": json.RawMessage(`{"name": "Test", "value": 123}`),
		},
		Relationships: []Relationship{
			{Subject: "entity.test", Predicate: "links_to", Object: "other"},
		},
	}

	ctx, err := BuildContextFromBatch(batchResult, DefaultFormatOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Content == "" {
		t.Error("expected non-empty content")
	}
	if ctx.TokenCount == 0 {
		t.Error("expected non-zero token count")
	}
	if len(ctx.Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(ctx.Entities))
	}
	if len(ctx.Sources) != 2 { // 1 entity + 1 relationship
		t.Errorf("expected 2 sources, got %d", len(ctx.Sources))
	}
	if ctx.ConstructedAt.After(time.Now()) {
		t.Error("ConstructedAt should not be in the future")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
