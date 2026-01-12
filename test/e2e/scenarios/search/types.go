// Package search provides unified search query execution for e2e tests.
package search

import "time"

// Query defines a search test case with validation expectations.
type Query struct {
	// Query text to send to the search endpoint
	Text string `json:"text"`

	// Description explains what this query tests
	Description string `json:"description"`

	// ExpectedPattern is a substring expected in matching entity IDs
	ExpectedPattern string `json:"expected_pattern,omitempty"`

	// MinScore is the minimum acceptable score for hits
	MinScore float64 `json:"min_score"`

	// MinHits is the minimum number of hits expected
	MinHits int `json:"min_hits"`

	// MustInclude contains entity ID substrings that MUST appear in results
	MustInclude []string `json:"must_include,omitempty"`

	// MustExclude contains entity ID substrings that should NOT appear (warning only)
	MustExclude []string `json:"must_exclude,omitempty"`

	// MustIncludeInTopN maps position N to entity patterns that must appear in top N results
	// Example: {3: ["doc-ops-001"]} means doc-ops-001 must be in top 3 results
	MustIncludeInTopN map[int][]string `json:"must_include_in_top_n,omitempty"`

	// MustRankHigherThan maps entity pattern to entities it must rank above
	// Example: {"doc-ops-001": ["doc-hr-001"]} means doc-ops-001 must rank higher than doc-hr-001
	MustRankHigherThan map[string][]string `json:"must_rank_higher_than,omitempty"`

	// Threshold is the similarity threshold to use (default 0.1)
	Threshold float64 `json:"threshold,omitempty"`

	// Limit is the max results to return (default 10)
	Limit int `json:"limit,omitempty"`
}

// Hit represents a single search result hit.
type Hit struct {
	EntityID string  `json:"entity_id"`
	Score    float64 `json:"score"`
}

// Result contains the outcome of executing a single search query.
type Result struct {
	// Query that was executed
	Query string `json:"query"`

	// Description of what was tested
	Description string `json:"description"`

	// Hits returned by the search
	Hits []Hit `json:"hits"`

	// LatencyMs is how long the query took
	LatencyMs int64 `json:"latency_ms"`

	// Error if the query failed
	Error string `json:"error,omitempty"`

	// HTTPStatus if non-200
	HTTPStatus int `json:"http_status,omitempty"`

	// Validation results
	Validation ValidationResult `json:"validation"`
}

// ValidationResult contains the validation outcome for a query.
type ValidationResult struct {
	// MatchesPattern indicates if any hit matched the expected pattern
	MatchesPattern bool `json:"matches_pattern"`

	// MeetsMinScore indicates if any hit met the minimum score
	MeetsMinScore bool `json:"meets_min_score"`

	// MeetsMinHits indicates if enough hits were returned
	MeetsMinHits bool `json:"meets_min_hits"`

	// HitsAboveMinScore is the count of hits meeting min score
	HitsAboveMinScore int `json:"hits_above_min_score"`

	// AvgScore is the average score across all hits
	AvgScore float64 `json:"avg_score"`

	// KnownAnswerPassed indicates if all mustInclude entities were found
	KnownAnswerPassed bool `json:"known_answer_passed"`

	// MissingRequired lists mustInclude patterns not found
	MissingRequired []string `json:"missing_required,omitempty"`

	// UnexpectedFound lists mustExclude patterns that were found
	UnexpectedFound []string `json:"unexpected_found,omitempty"`

	// PositionViolations lists entities that violated position constraints
	PositionViolations []PositionViolation `json:"position_violations,omitempty"`

	// RankingViolations lists entities that violated ranking constraints
	RankingViolations []RankingViolation `json:"ranking_violations,omitempty"`
}

// PositionViolation represents an entity that failed to appear in required top N.
type PositionViolation struct {
	Pattern      string `json:"pattern"`        // Entity pattern that failed
	RequiredTopN int    `json:"required_top_n"` // Required position (top N)
	ActualRank   int    `json:"actual_rank"`    // Actual rank (-1 if not found)
}

// RankingViolation represents an entity that failed to rank higher than another.
type RankingViolation struct {
	Higher     string `json:"higher"`      // Entity that should rank higher
	Lower      string `json:"lower"`       // Entity that should rank lower
	HigherRank int    `json:"higher_rank"` // Actual rank of "higher" (-1 if not found)
	LowerRank  int    `json:"lower_rank"`  // Actual rank of "lower" (-1 if not found)
}

// Stats aggregates results across multiple queries.
type Stats struct {
	// TotalQueries is the number of queries executed
	TotalQueries int `json:"total_queries"`

	// QueriesWithResults is count of queries that returned hits
	QueriesWithResults int `json:"queries_with_results"`

	// QueriesMeetingMinScore is count meeting score threshold
	QueriesMeetingMinScore int `json:"queries_meeting_min_score"`

	// QueriesMeetingMinHits is count meeting hit count threshold
	QueriesMeetingMinHits int `json:"queries_meeting_min_hits"`

	// OverallAvgScore is the average score across all hits from all queries
	OverallAvgScore float64 `json:"overall_avg_score"`

	// KnownAnswerTestsPassed is count of queries with mustInclude that passed
	KnownAnswerTestsPassed int `json:"known_answer_tests_passed"`

	// KnownAnswerTestsTotal is count of queries with mustInclude defined
	KnownAnswerTestsTotal int `json:"known_answer_tests_total"`

	// KnownAnswerFailures describes which known-answer tests failed
	KnownAnswerFailures []string `json:"known_answer_failures,omitempty"`

	// PositionTestsPassed is count of queries with position constraints that passed
	PositionTestsPassed int `json:"position_tests_passed"`

	// PositionTestsTotal is count of queries with position constraints defined
	PositionTestsTotal int `json:"position_tests_total"`

	// PositionTestFailures describes which position tests failed
	PositionTestFailures []string `json:"position_test_failures,omitempty"`

	// RankingTestsPassed is count of queries with ranking constraints that passed
	RankingTestsPassed int `json:"ranking_tests_passed"`

	// RankingTestsTotal is count of queries with ranking constraints defined
	RankingTestsTotal int `json:"ranking_tests_total"`

	// RankingTestFailures describes which ranking tests failed
	RankingTestFailures []string `json:"ranking_test_failures,omitempty"`

	// Results contains individual query results
	Results []Result `json:"results"`

	// ExecutedAt is when the queries were run
	ExecutedAt time.Time `json:"executed_at"`

	// TotalLatencyMs is the sum of all query latencies
	TotalLatencyMs int64 `json:"total_latency_ms"`
}

// PositionValidationPassed returns true if no position violations were recorded.
func (v *ValidationResult) PositionValidationPassed() bool {
	return len(v.PositionViolations) == 0
}

// RankingValidationPassed returns true if no ranking violations were recorded.
func (v *ValidationResult) RankingValidationPassed() bool {
	return len(v.RankingViolations) == 0
}
