package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExecutor_ExecuteOne_Success(t *testing.T) {
	// Mock server returning GraphQL search results (similaritySearch format)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return GraphQL similaritySearch response format
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{
					{"id": "sensor-temp-001", "type": "sensor", "score": 0.85},
					{"id": "sensor-temp-002", "type": "sensor", "score": 0.72},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	query := Query{
		Text:            "temperature sensors",
		Description:     "Test query",
		ExpectedPattern: "temp",
		MinScore:        0.5,
		MinHits:         1,
		MustInclude:     []string{"sensor-temp"},
	}

	result := executor.ExecuteOne(context.Background(), query)

	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if len(result.Hits) != 2 {
		t.Errorf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.LatencyMs == 0 {
		t.Error("expected non-zero latency")
	}
	if !result.Validation.MatchesPattern {
		t.Error("expected pattern match")
	}
	if !result.Validation.MeetsMinScore {
		t.Error("expected min score met")
	}
	if !result.Validation.MeetsMinHits {
		t.Error("expected min hits met")
	}
	if !result.Validation.KnownAnswerPassed {
		t.Error("expected known answer to pass")
	}
}

func TestExecutor_ExecuteOne_NoHits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	query := Query{
		Text:        "nonexistent",
		Description: "Query with no results",
		MinHits:     1,
	}

	result := executor.ExecuteOne(context.Background(), query)

	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if len(result.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(result.Hits))
	}
	if result.Validation.MeetsMinHits {
		t.Error("should not meet min hits")
	}
}

func TestExecutor_ExecuteOne_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	query := Query{Text: "test"}

	result := executor.ExecuteOne(context.Background(), query)

	if result.Error == "" {
		t.Error("expected error for 500 response")
	}
	if result.HTTPStatus != 500 {
		t.Errorf("expected status 500, got %d", result.HTTPStatus)
	}
}

func TestExecutor_ExecuteOne_MustIncludeFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{
					{"id": "sensor-humid-001", "type": "sensor", "score": 0.9},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	query := Query{
		Text:        "temperature",
		MustInclude: []string{"sensor-temp"}, // Not in results
	}

	result := executor.ExecuteOne(context.Background(), query)

	if result.Validation.KnownAnswerPassed {
		t.Error("known answer should fail - missing sensor-temp")
	}
	if len(result.Validation.MissingRequired) != 1 {
		t.Errorf("expected 1 missing required, got %d", len(result.Validation.MissingRequired))
	}
}

func TestExecutor_ExecuteOne_MustExcludeWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{
					{"id": "sensor-temp-001", "type": "sensor", "score": 0.9},
					{"id": "doc-hr-001", "type": "document", "score": 0.5}, // Should be excluded
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	query := Query{
		Text:        "temperature",
		MustInclude: []string{"sensor-temp"},
		MustExclude: []string{"doc-hr"},
	}

	result := executor.ExecuteOne(context.Background(), query)

	// KnownAnswerPassed should still be true (mustExclude is warning only)
	if !result.Validation.KnownAnswerPassed {
		t.Error("known answer should pass - mustExclude is warning only")
	}
	if len(result.Validation.UnexpectedFound) != 1 {
		t.Errorf("expected 1 unexpected found, got %d", len(result.Validation.UnexpectedFound))
	}
}

func TestExecutor_ExecuteAll_Stats(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		var resp map[string]any
		if callCount == 1 {
			resp = map[string]any{
				"data": map[string]any{
					"similaritySearch": []map[string]any{
						{"id": "hit-1", "type": "entity", "score": 0.8},
						{"id": "hit-2", "type": "entity", "score": 0.6},
					},
				},
			}
		} else {
			resp = map[string]any{
				"data": map[string]any{
					"similaritySearch": []map[string]any{},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	queries := []Query{
		{Text: "query1", MinHits: 1, MinScore: 0.5},
		{Text: "query2", MinHits: 1},
	}

	stats := executor.ExecuteAll(context.Background(), queries)

	if stats.TotalQueries != 2 {
		t.Errorf("expected 2 total queries, got %d", stats.TotalQueries)
	}
	if stats.QueriesWithResults != 1 {
		t.Errorf("expected 1 query with results, got %d", stats.QueriesWithResults)
	}
	if stats.QueriesMeetingMinHits != 1 {
		t.Errorf("expected 1 query meeting min hits, got %d", stats.QueriesMeetingMinHits)
	}
	if len(stats.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(stats.Results))
	}
	// Average of 0.8 and 0.6 = 0.7
	if stats.OverallAvgScore < 0.69 || stats.OverallAvgScore > 0.71 {
		t.Errorf("expected avg score ~0.7, got %f", stats.OverallAvgScore)
	}
}

func TestExecutor_ExecuteAll_KnownAnswerTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{
					{"id": "sensor-temp-001", "type": "sensor", "score": 0.8},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	queries := []Query{
		{Text: "q1", MustInclude: []string{"sensor-temp"}}, // Pass
		{Text: "q2", MustInclude: []string{"doc-ops"}},     // Fail
		{Text: "q3"}, // No known-answer validation
	}

	stats := executor.ExecuteAll(context.Background(), queries)

	if stats.KnownAnswerTestsTotal != 2 {
		t.Errorf("expected 2 known-answer tests, got %d", stats.KnownAnswerTestsTotal)
	}
	if stats.KnownAnswerTestsPassed != 1 {
		t.Errorf("expected 1 passed, got %d", stats.KnownAnswerTestsPassed)
	}
	if len(stats.KnownAnswerFailures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(stats.KnownAnswerFailures))
	}
}

func TestValidation_AvgScore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{
					{"id": "a", "type": "entity", "score": 0.9},
					{"id": "b", "type": "entity", "score": 0.7},
					{"id": "c", "type": "entity", "score": 0.5},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	result := executor.ExecuteOne(context.Background(), Query{Text: "test"})

	// Average of 0.9, 0.7, 0.5 = 0.7
	if result.Validation.AvgScore < 0.69 || result.Validation.AvgScore > 0.71 {
		t.Errorf("expected avg score ~0.7, got %f", result.Validation.AvgScore)
	}
}

func TestValidation_HitsAboveMinScore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"similaritySearch": []map[string]any{
					{"id": "a", "type": "entity", "score": 0.9},
					{"id": "b", "type": "entity", "score": 0.4}, // Below 0.5
					{"id": "c", "type": "entity", "score": 0.6},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewSimilarityExecutor(server.URL, 5*time.Second)
	result := executor.ExecuteOne(context.Background(), Query{Text: "test", MinScore: 0.5})

	if result.Validation.HitsAboveMinScore != 2 {
		t.Errorf("expected 2 hits above min score, got %d", result.Validation.HitsAboveMinScore)
	}
}
