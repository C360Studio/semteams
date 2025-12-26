package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SearchMode determines which GraphQL query to use for search.
type SearchMode string

const (
	// SearchModeGlobal uses globalSearch (community-based GraphRAG search).
	SearchModeGlobal SearchMode = "global"
	// SearchModeSimilarity uses similaritySearch (embedding similarity search).
	// Works on both statistical (BM25) and semantic (neural) tiers.
	SearchModeSimilarity SearchMode = "similarity"
)

// Executor executes search queries against the GraphQL gateway.
type Executor struct {
	httpClient *http.Client
	graphqlURL string     // GraphQL endpoint (e.g., http://localhost:8084/graphql)
	mode       SearchMode // Which search query to use
}

// NewExecutor creates a new search executor using globalSearch.
// graphqlURL should be the full GraphQL endpoint (e.g., http://localhost:8084/graphql).
func NewExecutor(graphqlURL string, timeout time.Duration) *Executor {
	return &Executor{
		httpClient: &http.Client{Timeout: timeout},
		graphqlURL: graphqlURL,
		mode:       SearchModeGlobal,
	}
}

// NewSimilarityExecutor creates a new search executor using similaritySearch.
// This provides embedding-based similarity search with real scores.
// Works on both statistical (BM25) and semantic (neural) tiers.
func NewSimilarityExecutor(graphqlURL string, timeout time.Duration) *Executor {
	return &Executor{
		httpClient: &http.Client{Timeout: timeout},
		graphqlURL: graphqlURL,
		mode:       SearchModeSimilarity,
	}
}

// ExecuteAll runs all queries and returns aggregated stats.
func (e *Executor) ExecuteAll(ctx context.Context, queries []Query) *Stats {
	stats := &Stats{
		TotalQueries: len(queries),
		Results:      make([]Result, 0, len(queries)),
		ExecutedAt:   time.Now(),
	}

	var allScores []float64

	for _, q := range queries {
		result := e.ExecuteOne(ctx, q)
		stats.Results = append(stats.Results, result)
		stats.TotalLatencyMs += result.LatencyMs

		if len(result.Hits) > 0 {
			stats.QueriesWithResults++
		}

		if result.Validation.MeetsMinScore {
			stats.QueriesMeetingMinScore++
		}

		if result.Validation.MeetsMinHits {
			stats.QueriesMeetingMinHits++
		}

		// Track known-answer results
		if len(q.MustInclude) > 0 {
			stats.KnownAnswerTestsTotal++
			if result.Validation.KnownAnswerPassed {
				stats.KnownAnswerTestsPassed++
			} else {
				stats.KnownAnswerFailures = append(stats.KnownAnswerFailures,
					fmt.Sprintf("query %q: missing required %v - %s",
						q.Text, result.Validation.MissingRequired, q.Description))
			}
		}

		// Collect all scores for overall average
		for _, hit := range result.Hits {
			allScores = append(allScores, hit.Score)
		}
	}

	// Calculate overall average score
	if len(allScores) > 0 {
		var sum float64
		for _, s := range allScores {
			sum += s
		}
		stats.OverallAvgScore = sum / float64(len(allScores))
	}

	return stats
}

// ExecuteOne runs a single query via GraphQL and returns the result.
// Uses globalSearch or semanticSearch based on executor mode.
func (e *Executor) ExecuteOne(ctx context.Context, q Query) Result {
	result := Result{
		Query:       q.Text,
		Description: q.Description,
	}

	// Set defaults
	limit := q.Limit
	if limit == 0 {
		limit = 10
	}

	// Build GraphQL query based on mode
	var graphqlQuery map[string]any
	if e.mode == SearchModeSimilarity {
		// Use similaritySearch for embedding-based similarity with real scores
		// Works on both statistical (BM25) and semantic (neural) tiers
		graphqlQuery = map[string]any{
			"query": `query($query: String!, $limit: Int) {
				similaritySearch(query: $query, limit: $limit) {
					id
					type
					score
				}
			}`,
			"variables": map[string]any{
				"query": q.Text,
				"limit": limit,
			},
		}
	} else {
		// Use globalSearch for community-based GraphRAG search
		graphqlQuery = map[string]any{
			"query": `query($query: String!, $level: Int, $maxCommunities: Int) {
				globalSearch(query: $query, level: $level, maxCommunities: $maxCommunities) {
					entities { id type }
					communitySummaries { communityId summary relevance }
					count
				}
			}`,
			"variables": map[string]any{
				"query":          q.Text,
				"level":          0,
				"maxCommunities": limit,
			},
		}
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		result.Error = fmt.Sprintf("marshal error: %v", err)
		return result
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		e.graphqlURL, strings.NewReader(string(queryJSON)))
	if err != nil {
		result.Error = fmt.Sprintf("request error: %v", err)
		return result
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute with timing
	start := time.Now()
	resp, err := e.httpClient.Do(req)
	elapsed := time.Since(start)
	result.LatencyMs = elapsed.Milliseconds()
	if result.LatencyMs == 0 && elapsed > 0 {
		result.LatencyMs = 1 // Minimum 1ms for non-zero duration
	}

	if err != nil {
		result.Error = fmt.Sprintf("http error: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.HTTPStatus = resp.StatusCode
		result.Error = fmt.Sprintf("http status %d", resp.StatusCode)
		return result
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read error: %v", err)
		return result
	}

	// Parse GraphQL response based on mode
	if e.mode == SearchModeSimilarity {
		var gqlResp struct {
			Data struct {
				SimilaritySearch []struct {
					ID    string  `json:"id"`
					Type  string  `json:"type"`
					Score float64 `json:"score"`
				} `json:"similaritySearch"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}

		if err := json.Unmarshal(bodyBytes, &gqlResp); err != nil {
			result.Error = fmt.Sprintf("parse error: %v", err)
			return result
		}

		if len(gqlResp.Errors) > 0 {
			result.Error = gqlResp.Errors[0].Message
			return result
		}

		// Convert similaritySearch results to hits with real scores
		for _, entity := range gqlResp.Data.SimilaritySearch {
			result.Hits = append(result.Hits, Hit{
				EntityID: entity.ID,
				Score:    entity.Score,
			})
		}
	} else {
		var gqlResp struct {
			Data struct {
				GlobalSearch struct {
					Entities []struct {
						ID   string `json:"id"`
						Type string `json:"type"`
					} `json:"entities"`
					CommunitySummaries []struct {
						CommunityID string  `json:"communityId"`
						Summary     string  `json:"summary"`
						Relevance   float64 `json:"relevance"`
					} `json:"communitySummaries"`
					Count int `json:"count"`
				} `json:"globalSearch"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}

		if err := json.Unmarshal(bodyBytes, &gqlResp); err != nil {
			result.Error = fmt.Sprintf("parse error: %v", err)
			return result
		}

		if len(gqlResp.Errors) > 0 {
			result.Error = gqlResp.Errors[0].Message
			return result
		}

		// Convert entities to hits
		// GraphQL globalSearch returns entities from matching communities
		// Use community relevance as score proxy (entities inherit community relevance)
		for _, entity := range gqlResp.Data.GlobalSearch.Entities {
			// Default score of 1.0 since globalSearch doesn't return per-entity scores
			// The relevance is at the community level, not entity level
			result.Hits = append(result.Hits, Hit{
				EntityID: entity.ID,
				Score:    1.0,
			})
		}
	}

	// Validate result
	result.Validation = e.validate(q, result.Hits)

	return result
}

// validate checks the hits against query expectations.
func (e *Executor) validate(q Query, hits []Hit) ValidationResult {
	v := ValidationResult{
		KnownAnswerPassed: true, // Assume pass until proven otherwise
		MeetsMinHits:      len(hits) >= q.MinHits,
	}

	var scoreSum float64
	for _, hit := range hits {
		scoreSum += hit.Score

		if hit.Score >= q.MinScore {
			v.HitsAboveMinScore++
			v.MeetsMinScore = true
		}

		if q.ExpectedPattern != "" &&
			strings.Contains(strings.ToLower(hit.EntityID), strings.ToLower(q.ExpectedPattern)) {
			v.MatchesPattern = true
		}
	}

	if len(hits) > 0 {
		v.AvgScore = scoreSum / float64(len(hits))
	}

	// Check mustInclude - required entities that MUST appear
	for _, required := range q.MustInclude {
		found := false
		for _, hit := range hits {
			if strings.Contains(strings.ToLower(hit.EntityID), strings.ToLower(required)) {
				found = true
				break
			}
		}
		if !found {
			v.KnownAnswerPassed = false
			v.MissingRequired = append(v.MissingRequired, required)
		}
	}

	// Check mustExclude - entities that should NOT appear (warning only)
	for _, forbidden := range q.MustExclude {
		for _, hit := range hits {
			if strings.Contains(strings.ToLower(hit.EntityID), strings.ToLower(forbidden)) {
				v.UnexpectedFound = append(v.UnexpectedFound, hit.EntityID)
				break
			}
		}
	}

	return v
}
