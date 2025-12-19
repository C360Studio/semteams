// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"context"
	"fmt"
)

// Structural variant validation functions (ported from tier0_rules_iot.go)

// executeValidateZeroEmbeddings validates that NO embeddings were generated (structural tier constraint)
func (s *TieredScenario) executeValidateZeroEmbeddings(ctx context.Context, result *Result) error {
	embeddingCount, _ := s.metrics.SumMetricsByName(ctx, "indexengine_embeddings_generated_total")

	result.Metrics["embeddings_generated"] = int(embeddingCount)

	if int(embeddingCount) > s.config.ExpectedEmbeddings {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Structural tier constraint violated: embeddings=%d (expected %d)",
				int(embeddingCount), s.config.ExpectedEmbeddings))
	}

	result.Details["zero_embeddings_validation"] = map[string]any{
		"embeddings_generated": int(embeddingCount),
		"expected":             s.config.ExpectedEmbeddings,
		"constraint_met":       int(embeddingCount) <= s.config.ExpectedEmbeddings,
		"message":              fmt.Sprintf("Embeddings: %d (expected %d for structural tier)", int(embeddingCount), s.config.ExpectedEmbeddings),
	}

	return nil
}

// executeValidateZeroClusters validates that NO clustering occurred (structural tier constraint)
func (s *TieredScenario) executeValidateZeroClusters(ctx context.Context, result *Result) error {
	clusteringCount, _ := s.metrics.SumMetricsByName(ctx, "semstreams_clustering_runs_total")

	result.Metrics["clustering_runs"] = int(clusteringCount)

	if int(clusteringCount) > s.config.ExpectedClusters {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Structural tier constraint violated: clustering_runs=%d (expected %d)",
				int(clusteringCount), s.config.ExpectedClusters))
	}

	result.Details["zero_clusters_validation"] = map[string]any{
		"clustering_runs": int(clusteringCount),
		"expected":        s.config.ExpectedClusters,
		"constraint_met":  int(clusteringCount) <= s.config.ExpectedClusters,
		"message":         fmt.Sprintf("Clustering runs: %d (expected %d for structural tier)", int(clusteringCount), s.config.ExpectedClusters),
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
