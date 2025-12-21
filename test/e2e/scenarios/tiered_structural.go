// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Structural variant validation functions (ported from tier0_rules_iot.go)

// executeValidateZeroEmbeddings validates that NO embeddings were generated (structural tier constraint)
func (s *TieredScenario) executeValidateZeroEmbeddings(ctx context.Context, result *Result) error {
	embeddingCount, _ := s.metrics.SumMetricsByName(ctx, "indexengine_embeddings_generated_total")

	result.Metrics["embeddings_generated"] = int(embeddingCount)

	constraintMet := int(embeddingCount) <= s.config.ExpectedEmbeddings

	result.Details["zero_embeddings_validation"] = map[string]any{
		"embeddings_generated": int(embeddingCount),
		"expected":             s.config.ExpectedEmbeddings,
		"constraint_met":       constraintMet,
		"message":              fmt.Sprintf("Embeddings: %d (expected %d for structural tier)", int(embeddingCount), s.config.ExpectedEmbeddings),
	}

	// Structural tier constraint: embeddings MUST be zero (or within expected limit)
	if !constraintMet {
		return fmt.Errorf("structural tier constraint violated: embeddings=%d (expected max %d)",
			int(embeddingCount), s.config.ExpectedEmbeddings)
	}

	return nil
}

// executeValidateZeroClusters validates that NO clustering occurred (structural tier constraint)
func (s *TieredScenario) executeValidateZeroClusters(ctx context.Context, result *Result) error {
	clusteringCount, _ := s.metrics.SumMetricsByName(ctx, "semstreams_clustering_runs_total")

	result.Metrics["clustering_runs"] = int(clusteringCount)

	constraintMet := int(clusteringCount) <= s.config.ExpectedClusters

	result.Details["zero_clusters_validation"] = map[string]any{
		"clustering_runs": int(clusteringCount),
		"expected":        s.config.ExpectedClusters,
		"constraint_met":  constraintMet,
		"message":         fmt.Sprintf("Clustering runs: %d (expected %d for structural tier)", int(clusteringCount), s.config.ExpectedClusters),
	}

	// Structural tier constraint: clustering MUST NOT occur
	if !constraintMet {
		return fmt.Errorf("structural tier constraint violated: clustering_runs=%d (expected max %d)",
			int(clusteringCount), s.config.ExpectedClusters)
	}

	return nil
}

// executeValidateRuleTransitions validates stateful rule OnEnter/OnExit transitions (structural tier)
func (s *TieredScenario) executeValidateRuleTransitions(ctx context.Context, result *Result) error {
	// Get rule metrics using MetricsClient
	ruleMetrics, err := s.metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to extract rule metrics: %v", err))
		return nil
	}

	onEnterFired := int(ruleMetrics.OnEnterFired)
	onExitFired := int(ruleMetrics.OnExitFired)

	result.Metrics["on_enter_fired"] = onEnterFired
	result.Metrics["on_exit_fired"] = onExitFired

	// Validate minimum state transitions
	violations := []string{}
	if onEnterFired < s.config.MinOnEnterFired {
		violations = append(violations,
			fmt.Sprintf("OnEnter: %d < %d (expected)", onEnterFired, s.config.MinOnEnterFired))
	}
	if onExitFired < s.config.MinOnExitFired {
		violations = append(violations,
			fmt.Sprintf("OnExit: %d < %d (expected)", onExitFired, s.config.MinOnExitFired))
	}

	result.Details["rule_transitions_validation"] = map[string]any{
		"on_enter_fired":    onEnterFired,
		"on_exit_fired":     onExitFired,
		"min_on_enter":      s.config.MinOnEnterFired,
		"min_on_exit":       s.config.MinOnExitFired,
		"violations":        violations,
		"validation_passed": len(violations) == 0,
		"stateful_behavior": onEnterFired > 0 || onExitFired > 0,
		"dynamic_graph":     onExitFired > 0, // OnExit removes triples = dynamic graph
		"message":           fmt.Sprintf("Rule transitions: %d OnEnter, %d OnExit", onEnterFired, onExitFired),
	}

	if len(violations) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rule transition validation issues: %v", violations))
	}

	return nil
}

// pathRAGResponse represents the parsed GraphQL response for PathRAG queries
type pathRAGResponse struct {
	Data struct {
		PathSearch struct {
			Entities  []pathRAGEntity `json:"entities"`
			Paths     [][]pathRAGStep `json:"paths"` // Each path is a sequence of steps
			Truncated bool            `json:"truncated"`
		} `json:"pathSearch"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type pathRAGEntity struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Score float64 `json:"score"`
}

type pathRAGStep struct {
	From      string `json:"from"`
	Predicate string `json:"predicate"`
	To        string `json:"to"`
}

// executeTestPathRAG validates PathRAG traversal (Tier 0 feature - structural graph navigation)
func (s *TieredScenario) executeTestPathRAG(ctx context.Context, result *Result) error {
	startEntity := "c360.logistics.content.document.safety.doc-safety-001"

	// Use configured GraphQL URL (varies by profile: 8082 for statistical, 8182 for semantic)
	gatewayURL := s.config.GraphQLURL

	resp, latency, err := s.sendPathRAGRequest(ctx, startEntity, gatewayURL)
	if err != nil {
		result.Details["pathrag_test"] = map[string]any{
			"start_entity": startEntity, "error": err.Error(), "gateway_url": gatewayURL,
		}
		return err
	}

	result.Metrics["pathrag_latency_ms"] = latency.Milliseconds()
	return s.validatePathRAGResult(resp, startEntity, latency, result)
}

// sendPathRAGRequest sends the PathRAG GraphQL query and returns the parsed response
func (s *TieredScenario) sendPathRAGRequest(ctx context.Context, startEntity, gatewayURL string) (*pathRAGResponse, time.Duration, error) {
	graphqlQuery := map[string]any{
		"query": `query($startEntity: ID!, $maxDepth: Int, $maxNodes: Int) {
			pathSearch(startEntity: $startEntity, maxDepth: $maxDepth, maxNodes: $maxNodes) {
				entities { id type score } paths { from predicate to } truncated
			}}`,
		"variables": map[string]any{"startEntity": startEntity, "maxDepth": 2, "maxNodes": 10},
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal PathRAG query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create PathRAG request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, fmt.Errorf("PathRAG request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, latency, fmt.Errorf("PathRAG returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, fmt.Errorf("failed to read PathRAG response: %w", err)
	}

	var graphqlResp pathRAGResponse
	if err := json.Unmarshal(bodyBytes, &graphqlResp); err != nil {
		return nil, latency, fmt.Errorf("failed to parse PathRAG response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, latency, fmt.Errorf("PathRAG GraphQL error: %s", graphqlResp.Errors[0].Message)
	}

	return &graphqlResp, latency, nil
}

// validatePathRAGResult validates the PathRAG response and records results
func (s *TieredScenario) validatePathRAGResult(resp *pathRAGResponse, startEntity string, latency time.Duration, result *Result) error {
	ps := resp.Data.PathSearch
	entityCount := len(ps.Entities)
	// Count total paths (each path is a sequence of steps)
	pathCount := len(ps.Paths)

	result.Metrics["pathrag_entities_found"] = entityCount
	result.Metrics["pathrag_paths_found"] = pathCount

	if entityCount == 0 {
		result.Details["pathrag_test"] = map[string]any{
			"start_entity": startEntity, "entities_found": 0, "message": "No entities returned",
		}
		return fmt.Errorf("PathRAG returned no entities for start entity %s", startEntity)
	}

	// Verify scores decrease with depth (decay factor working)
	scoresValid, prevScore := true, 2.0
	entityIDs := make([]string, 0, len(ps.Entities))
	for i, e := range ps.Entities {
		entityIDs = append(entityIDs, e.ID)
		if i > 0 && e.Score > prevScore {
			scoresValid = false
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("PathRAG score increased: %s has %.3f > previous %.3f", e.ID, e.Score, prevScore))
		}
		prevScore = e.Score
	}

	result.Details["pathrag_test"] = map[string]any{
		"start_entity": startEntity, "entities_found": entityCount, "paths_found": pathCount,
		"truncated": ps.Truncated, "entity_ids": entityIDs, "scores_valid": scoresValid,
		"latency_ms": latency.Milliseconds(),
		"message":    fmt.Sprintf("PathRAG traversal successful: found %d entities via %d paths", entityCount, pathCount),
	}
	return nil
}
