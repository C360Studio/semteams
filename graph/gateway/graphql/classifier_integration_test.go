//go:build integration

package graphql

import (
	"context"
	"testing"
)

// TestKeywordClassifier_Integration_TemporalToStrategy verifies temporal queries
// route to StrategyTemporalGraphRAG via InferStrategy().
func TestKeywordClassifier_Integration_TemporalToStrategy(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	tests := []struct {
		name             string
		query            string
		expectedStrategy SearchStrategy
	}{
		{
			name:             "yesterday_temporal",
			query:            "events from yesterday",
			expectedStrategy: StrategyTemporalGraphRAG,
		},
		{
			name:             "last_24_hours_temporal",
			query:            "alerts last 24 hours",
			expectedStrategy: StrategyTemporalGraphRAG,
		},
		{
			name:             "this_week_temporal",
			query:            "sensors active this week",
			expectedStrategy: StrategyTemporalGraphRAG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := classifier.ClassifyQuery(ctx, tt.query)

			if opts == nil {
				t.Fatal("expected non-nil SearchOptions")
			}

			if opts.TimeRange == nil {
				t.Fatal("expected TimeRange to be populated")
			}

			strategy := opts.InferStrategy()
			if strategy != tt.expectedStrategy {
				t.Errorf("strategy mismatch: got %q, want %q", strategy, tt.expectedStrategy)
			}
		})
	}
}

// TestKeywordClassifier_Integration_SimilarityToStrategy verifies similarity queries
// route to StrategySemantic or StrategyHybridGraphRAG via InferStrategy().
func TestKeywordClassifier_Integration_SimilarityToStrategy(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	tests := []struct {
		name               string
		query              string
		expectedStrategies []SearchStrategy
	}{
		{
			name:  "similarity_semantic",
			query: "similar to pump-42",
			// Without filters, should be StrategySemantic
			expectedStrategies: []SearchStrategy{StrategySemantic},
		},
		{
			name:               "like_semantic",
			query:              "devices like sensor-001",
			expectedStrategies: []SearchStrategy{StrategySemantic},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := classifier.ClassifyQuery(ctx, tt.query)

			if opts == nil {
				t.Fatal("expected non-nil SearchOptions")
			}

			if !opts.UseEmbeddings {
				t.Fatal("expected UseEmbeddings to be true for similarity query")
			}

			strategy := opts.InferStrategy()
			found := false
			for _, expected := range tt.expectedStrategies {
				if strategy == expected {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("strategy %q not in expected strategies %v", strategy, tt.expectedStrategies)
			}
		})
	}
}

// TestKeywordClassifier_Integration_CombinedIntents verifies combined intent queries
// route to StrategyHybridGraphRAG when multiple filters are present.
func TestKeywordClassifier_Integration_CombinedIntents(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	tests := []struct {
		name             string
		query            string
		expectedStrategy SearchStrategy
	}{
		{
			name:             "temporal_and_similarity",
			query:            "similar sensors yesterday",
			expectedStrategy: StrategyHybridGraphRAG,
		},
		{
			name:             "temporal_and_text",
			query:            "pump failures last week",
			expectedStrategy: StrategyTemporalGraphRAG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := classifier.ClassifyQuery(ctx, tt.query)

			if opts == nil {
				t.Fatal("expected non-nil SearchOptions")
			}

			strategy := opts.InferStrategy()
			if strategy != tt.expectedStrategy {
				t.Errorf("strategy mismatch: got %q, want %q", strategy, tt.expectedStrategy)
			}
		})
	}
}

// TestKeywordClassifier_Integration_GenericQuery verifies generic queries
// without special intents route to StrategyGraphRAG.
func TestKeywordClassifier_Integration_GenericQuery(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	tests := []struct {
		name             string
		query            string
		expectedStrategy SearchStrategy
	}{
		{
			name:             "simple_query",
			query:            "pump sensor data",
			expectedStrategy: StrategyGraphRAG,
		},
		{
			name:             "question",
			query:            "what is a sensor?",
			expectedStrategy: StrategyGraphRAG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := classifier.ClassifyQuery(ctx, tt.query)

			if opts == nil {
				t.Fatal("expected non-nil SearchOptions")
			}

			strategy := opts.InferStrategy()
			if strategy != tt.expectedStrategy {
				t.Errorf("strategy mismatch: got %q, want %q", strategy, tt.expectedStrategy)
			}
		})
	}
}

// TestKeywordClassifier_Integration_TemporalRangeAccuracy verifies temporal extraction
// produces accurate time ranges that align with expectations.
func TestKeywordClassifier_Integration_TemporalRangeAccuracy(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "yesterday",
			query: "yesterday's events",
		},
		{
			name:  "last_7_days",
			query: "last 7 days",
		},
		{
			name:  "this_month",
			query: "this month's data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := classifier.ClassifyQuery(ctx, tt.query)

			if opts == nil {
				t.Fatal("expected non-nil SearchOptions")
			}

			if opts.TimeRange == nil {
				t.Fatal("expected TimeRange to be populated")
			}

			// Verify time range is valid
			if opts.TimeRange.Start.IsZero() {
				t.Error("Start time should not be zero")
			}

			if opts.TimeRange.End.IsZero() {
				t.Error("End time should not be zero")
			}

			if !opts.TimeRange.Start.Before(opts.TimeRange.End) {
				t.Errorf("Start (%v) should be before End (%v)",
					opts.TimeRange.Start, opts.TimeRange.End)
			}
		})
	}
}

// TestKeywordClassifier_Integration_PreservesQuery verifies original query text
// is always preserved through classification and strategy inference.
func TestKeywordClassifier_Integration_PreservesQuery(t *testing.T) {
	classifier := &KeywordClassifier{}
	ctx := context.Background()

	queries := []string{
		"sensors near warehouse yesterday",
		"similar to pump-42 last week",
		"what happened today?",
		"show me all devices",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			opts := classifier.ClassifyQuery(ctx, query)

			if opts == nil {
				t.Fatal("expected non-nil SearchOptions")
			}

			if opts.Query != query {
				t.Errorf("Query not preserved: got %q, want %q", opts.Query, query)
			}

			// Verify strategy inference still works with preserved query
			strategy := opts.InferStrategy()
			if strategy == "" {
				t.Error("InferStrategy should return non-empty strategy")
			}
		})
	}
}
