// Package main provides structured result comparison for tier runs
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/scenarios"
	"github.com/c360/semstreams/test/e2e/scenarios/search"
)

// StructuredComparisonReport represents comparison of two structured tier runs
type StructuredComparisonReport struct {
	BaselineVariant string                   `json:"baseline_variant"`
	TargetVariant   string                   `json:"target_variant"`
	BaselineFile    string                   `json:"baseline_file"`
	TargetFile      string                   `json:"target_file"`
	GeneratedAt     time.Time                `json:"generated_at"`
	Sections        StructuredComparisonDiff `json:"sections"`
	Summary         StructuredCompareSummary `json:"summary"`
}

// StructuredComparisonDiff contains diffs by section
type StructuredComparisonDiff struct {
	Variant     VariantDiff    `json:"variant"`
	Entities    EntityDiff     `json:"entities"`
	Indexes     IndexDiff      `json:"indexes"`
	Search      SearchDiff     `json:"search"`
	Rules       RuleDiff       `json:"rules"`
	Communities *CommunityDiff `json:"communities,omitempty"`
	// Tier capability sections
	PathRAG       *PathRAGDiff         `json:"pathrag,omitempty"`
	StructuralIdx *StructuralIndexDiff `json:"structural_indexes,omitempty"`
	GraphRAG      *GraphRAGDiff        `json:"graphrag,omitempty"`
}

// VariantDiff compares variant information
type VariantDiff struct {
	BaselineName     string `json:"baseline_name"`
	TargetName       string `json:"target_name"`
	BaselineProvider string `json:"baseline_provider"`
	TargetProvider   string `json:"target_provider"`
}

// EntityDiff compares entity results
type EntityDiff struct {
	BaselineCount int     `json:"baseline_count"`
	TargetCount   int     `json:"target_count"`
	CountDiff     int     `json:"count_diff"`
	BaselineLoss  float64 `json:"baseline_loss_percent"`
	TargetLoss    float64 `json:"target_loss_percent"`
}

// IndexDiff compares index population
type IndexDiff struct {
	BaselinePopulated int      `json:"baseline_populated"`
	TargetPopulated   int      `json:"target_populated"`
	PopulatedDiff     int      `json:"populated_diff"`
	DifferingIndexes  []string `json:"differing_indexes,omitempty"`
}

// SearchDiff compares search quality results
type SearchDiff struct {
	BaselineQueries        int               `json:"baseline_queries"`
	TargetQueries          int               `json:"target_queries"`
	BaselineWithResults    int               `json:"baseline_with_results"`
	TargetWithResults      int               `json:"target_with_results"`
	BaselineAvgScore       float64           `json:"baseline_avg_score"`
	TargetAvgScore         float64           `json:"target_avg_score"`
	BaselineKnownAnswerPct float64           `json:"baseline_known_answer_pct"`
	TargetKnownAnswerPct   float64           `json:"target_known_answer_pct"`
	QueryDiffs             []SearchQueryDiff `json:"query_diffs,omitempty"`
}

// SearchQueryDiff compares results for a single query
type SearchQueryDiff struct {
	Query            string  `json:"query"`
	BaselineHits     int     `json:"baseline_hits"`
	TargetHits       int     `json:"target_hits"`
	HitsDiff         int     `json:"hits_diff"`
	BaselineAvgScore float64 `json:"baseline_avg_score"`
	TargetAvgScore   float64 `json:"target_avg_score"`
	Insight          string  `json:"insight"`
}

// RuleDiff compares rule evaluation results
type RuleDiff struct {
	BaselineEvaluated int  `json:"baseline_evaluated"`
	TargetEvaluated   int  `json:"target_evaluated"`
	BaselineTriggered int  `json:"baseline_triggered"`
	TargetTriggered   int  `json:"target_triggered"`
	BaselinePassed    bool `json:"baseline_passed"`
	TargetPassed      bool `json:"target_passed"`
}

// CommunityDiff compares community detection results
type CommunityDiff struct {
	BaselineTotal   int     `json:"baseline_total"`
	TargetTotal     int     `json:"target_total"`
	TotalDiff       int     `json:"total_diff"`
	BaselineLargest int     `json:"baseline_largest"`
	TargetLargest   int     `json:"target_largest"`
	BaselineAvgSize float64 `json:"baseline_avg_size"`
	TargetAvgSize   float64 `json:"target_avg_size"`
}

// PathRAGDiff compares PathRAG graph traversal results (Tier 0 - runs on all tiers)
type PathRAGDiff struct {
	BaselineEntities int   `json:"baseline_entities"`
	TargetEntities   int   `json:"target_entities"`
	EntitiesDiff     int   `json:"entities_diff"`
	BaselineLatency  int64 `json:"baseline_latency_ms"`
	TargetLatency    int64 `json:"target_latency_ms"`
	BothScoresValid  bool  `json:"both_scores_valid"`
}

// StructuralIndexDiff compares k-core and pivot index results (Tier 0)
type StructuralIndexDiff struct {
	BaselineKCoreMax  int  `json:"baseline_kcore_max"`
	TargetKCoreMax    int  `json:"target_kcore_max"`
	KCoreMaxDiff      int  `json:"kcore_max_diff"`
	BaselinePivots    int  `json:"baseline_pivots"`
	TargetPivots      int  `json:"target_pivots"`
	PivotsDiff        int  `json:"pivots_diff"`
	BothKCoreVerified bool `json:"both_kcore_verified"`
	BothPivotVerified bool `json:"both_pivot_verified"`
}

// GraphRAGDiff compares GraphRAG query results (Tier 2 - semantic only)
type GraphRAGDiff struct {
	BaselineLocalSuccess  bool  `json:"baseline_local_success"`
	TargetLocalSuccess    bool  `json:"target_local_success"`
	BaselineGlobalSuccess bool  `json:"baseline_global_success"`
	TargetGlobalSuccess   bool  `json:"target_global_success"`
	BaselineLocalLatency  int64 `json:"baseline_local_latency_ms"`
	TargetLocalLatency    int64 `json:"target_local_latency_ms"`
	BaselineGlobalLatency int64 `json:"baseline_global_latency_ms"`
	TargetGlobalLatency   int64 `json:"target_global_latency_ms"`
}

// StructuredCompareSummary summarizes the comparison
type StructuredCompareSummary struct {
	TierCapabilityDiff string   `json:"tier_capability_diff"`
	SearchImprovement  string   `json:"search_improvement"`
	Regressions        []string `json:"regressions,omitempty"`
	Improvements       []string `json:"improvements,omitempty"`
}

// handleStructuredCompareCommand compares two structured result files
func handleStructuredCompareCommand(logger *slog.Logger, baselineFile, targetFile string) int {
	if baselineFile == "" || targetFile == "" {
		logger.Error("Both --baseline and --target files required")
		fmt.Println("\nUsage: ./e2e compare-structured --baseline results/structural.json --target results/statistical.json")
		return 1
	}

	// Load baseline
	baseline, err := scenarios.LoadStructuredResults(baselineFile)
	if err != nil {
		logger.Error("Failed to load baseline", "file", baselineFile, "error", err)
		return 1
	}

	// Load target
	target, err := scenarios.LoadStructuredResults(targetFile)
	if err != nil {
		logger.Error("Failed to load target", "file", targetFile, "error", err)
		return 1
	}

	// Generate comparison report
	report := compareStructuredResults(baseline, target, baselineFile, targetFile)

	// Print report
	printStructuredComparisonReport(report)

	// Save report
	outputFile := fmt.Sprintf("structured-comparison-%s-vs-%s-%s.json",
		baseline.Variant.Name,
		target.Variant.Name,
		time.Now().Format("20060102-150405"))

	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		if err := os.WriteFile(outputFile, data, 0644); err == nil {
			logger.Info("Report saved", "file", outputFile)
		}
	}

	return 0
}

// compareStructuredResults generates a comparison between two structured results
func compareStructuredResults(baseline, target *scenarios.TieredResults, baselineFile, targetFile string) *StructuredComparisonReport {
	report := &StructuredComparisonReport{
		BaselineVariant: baseline.Variant.Name,
		TargetVariant:   target.Variant.Name,
		BaselineFile:    baselineFile,
		TargetFile:      targetFile,
		GeneratedAt:     time.Now(),
	}

	// Variant diff
	report.Sections.Variant = VariantDiff{
		BaselineName:     baseline.Variant.Name,
		TargetName:       target.Variant.Name,
		BaselineProvider: baseline.Variant.EmbeddingProvider,
		TargetProvider:   target.Variant.EmbeddingProvider,
	}

	// Entity diff
	report.Sections.Entities = EntityDiff{
		BaselineCount: baseline.Entities.ActualCount,
		TargetCount:   target.Entities.ActualCount,
		CountDiff:     target.Entities.ActualCount - baseline.Entities.ActualCount,
		BaselineLoss:  baseline.Entities.DataLossPercent,
		TargetLoss:    target.Entities.DataLossPercent,
	}

	// Index diff
	report.Sections.Indexes = compareIndexes(baseline.Indexes, target.Indexes)

	// Search diff
	report.Sections.Search = compareSearch(baseline.Search, target.Search)

	// Rule diff
	report.Sections.Rules = RuleDiff{
		BaselineEvaluated: baseline.Rules.EvaluatedCount,
		TargetEvaluated:   target.Rules.EvaluatedCount,
		BaselineTriggered: baseline.Rules.TriggeredCount,
		TargetTriggered:   target.Rules.TriggeredCount,
		BaselinePassed:    baseline.Rules.ValidationPassed,
		TargetPassed:      target.Rules.ValidationPassed,
	}

	// Community diff (if both have it)
	if baseline.Communities != nil && target.Communities != nil {
		report.Sections.Communities = &CommunityDiff{
			BaselineTotal:   baseline.Communities.TotalCommunities,
			TargetTotal:     target.Communities.TotalCommunities,
			TotalDiff:       target.Communities.TotalCommunities - baseline.Communities.TotalCommunities,
			BaselineLargest: baseline.Communities.LargestSize,
			TargetLargest:   target.Communities.LargestSize,
			BaselineAvgSize: baseline.Communities.AverageSize,
			TargetAvgSize:   target.Communities.AverageSize,
		}
	}

	// PathRAG comparison (Tier 0 - should be present in all tiers)
	report.Sections.PathRAG = comparePathRAG(baseline.PathRAGSensor, target.PathRAGSensor)

	// Structural index comparison (Tier 0 - k-core, pivot)
	report.Sections.StructuralIdx = compareStructuralIndexes(baseline.StructuralIndexes, target.StructuralIndexes)

	// GraphRAG comparison (Tier 2 - semantic only)
	report.Sections.GraphRAG = compareGraphRAG(baseline.GraphRAG, target.GraphRAG)

	// Generate summary
	report.Summary = generateStructuredSummary(report, baseline, target)

	return report
}

// compareIndexes compares index population between two results
func compareIndexes(baseline, target scenarios.IndexResults) IndexDiff {
	diff := IndexDiff{
		BaselinePopulated: baseline.PopulatedIndexes,
		TargetPopulated:   target.PopulatedIndexes,
		PopulatedDiff:     target.PopulatedIndexes - baseline.PopulatedIndexes,
	}

	// Find differing indexes
	allIndexes := make(map[string]bool)
	for name := range baseline.IndexDetails {
		allIndexes[name] = true
	}
	for name := range target.IndexDetails {
		allIndexes[name] = true
	}

	for name := range allIndexes {
		baseDetail := baseline.IndexDetails[name]
		targetDetail := target.IndexDetails[name]
		if baseDetail.Populated != targetDetail.Populated {
			if targetDetail.Populated {
				diff.DifferingIndexes = append(diff.DifferingIndexes, fmt.Sprintf("+%s", name))
			} else {
				diff.DifferingIndexes = append(diff.DifferingIndexes, fmt.Sprintf("-%s", name))
			}
		}
	}

	return diff
}

// compareSearch compares search quality between two results
func compareSearch(baseline, target scenarios.SearchResults) SearchDiff {
	diff := SearchDiff{}

	if baseline.Stats != nil {
		diff.BaselineQueries = baseline.Stats.TotalQueries
		diff.BaselineWithResults = baseline.Stats.QueriesWithResults
		diff.BaselineAvgScore = baseline.Stats.OverallAvgScore
		if baseline.Stats.KnownAnswerTestsTotal > 0 {
			diff.BaselineKnownAnswerPct = float64(baseline.Stats.KnownAnswerTestsPassed) / float64(baseline.Stats.KnownAnswerTestsTotal) * 100
		}
	}

	if target.Stats != nil {
		diff.TargetQueries = target.Stats.TotalQueries
		diff.TargetWithResults = target.Stats.QueriesWithResults
		diff.TargetAvgScore = target.Stats.OverallAvgScore
		if target.Stats.KnownAnswerTestsTotal > 0 {
			diff.TargetKnownAnswerPct = float64(target.Stats.KnownAnswerTestsPassed) / float64(target.Stats.KnownAnswerTestsTotal) * 100
		}
	}

	// Compare individual queries
	if baseline.Stats != nil && target.Stats != nil {
		diff.QueryDiffs = compareQueryResults(baseline.Stats.Results, target.Stats.Results)
	}

	return diff
}

// compareQueryResults compares individual query results
func compareQueryResults(baselineResults, targetResults []search.Result) []SearchQueryDiff {
	var diffs []SearchQueryDiff

	// Build maps by query text
	baselineMap := make(map[string]search.Result)
	for _, r := range baselineResults {
		baselineMap[r.Query] = r
	}

	targetMap := make(map[string]search.Result)
	for _, r := range targetResults {
		targetMap[r.Query] = r
	}

	// Get all queries
	allQueries := make(map[string]bool)
	for q := range baselineMap {
		allQueries[q] = true
	}
	for q := range targetMap {
		allQueries[q] = true
	}

	// Sort for consistent output
	var queries []string
	for q := range allQueries {
		queries = append(queries, q)
	}
	sort.Strings(queries)

	for _, query := range queries {
		baseResult := baselineMap[query]
		targetResult := targetMap[query]

		baseHits := len(baseResult.Hits)
		targetHits := len(targetResult.Hits)

		qd := SearchQueryDiff{
			Query:            query,
			BaselineHits:     baseHits,
			TargetHits:       targetHits,
			HitsDiff:         targetHits - baseHits,
			BaselineAvgScore: baseResult.Validation.AvgScore,
			TargetAvgScore:   targetResult.Validation.AvgScore,
		}

		// Generate insight
		qd.Insight = generateQueryInsight(baseHits, targetHits, qd.BaselineAvgScore, qd.TargetAvgScore)

		// Only include if there's a meaningful difference
		if qd.HitsDiff != 0 || (qd.BaselineAvgScore > 0 && qd.TargetAvgScore > 0) {
			diffs = append(diffs, qd)
		}
	}

	return diffs
}

// generateQueryInsight generates a human-readable insight for a query comparison
func generateQueryInsight(baselineHits, targetHits int, baselineAvg, targetAvg float64) string {
	if baselineHits == 0 && targetHits == 0 {
		return "No results from either"
	}
	if baselineHits == 0 {
		return fmt.Sprintf("Target finds %d results where baseline found none", targetHits)
	}
	if targetHits == 0 {
		return fmt.Sprintf("Baseline finds %d results where target found none", baselineHits)
	}
	if targetHits > baselineHits {
		return fmt.Sprintf("Target finds +%d more results", targetHits-baselineHits)
	}
	if baselineHits > targetHits {
		return fmt.Sprintf("Baseline finds +%d more results", baselineHits-targetHits)
	}
	if targetAvg > baselineAvg*1.1 {
		return "Similar hits, target has higher scores"
	}
	if baselineAvg > targetAvg*1.1 {
		return "Similar hits, baseline has higher scores"
	}
	return "Similar results"
}

// comparePathRAG compares PathRAG results between two tier runs
func comparePathRAG(baseline, target *scenarios.PathRAGResults) *PathRAGDiff {
	if baseline == nil && target == nil {
		return nil
	}
	diff := &PathRAGDiff{}
	if baseline != nil {
		diff.BaselineEntities = baseline.EntitiesFound
		diff.BaselineLatency = baseline.LatencyMs
	}
	if target != nil {
		diff.TargetEntities = target.EntitiesFound
		diff.TargetLatency = target.LatencyMs
	}
	diff.EntitiesDiff = diff.TargetEntities - diff.BaselineEntities
	diff.BothScoresValid = (baseline == nil || baseline.ScoresValid) &&
		(target == nil || target.ScoresValid)
	return diff
}

// compareStructuralIndexes compares k-core and pivot index results
func compareStructuralIndexes(baseline, target *scenarios.StructuralIndexResults) *StructuralIndexDiff {
	if baseline == nil && target == nil {
		return nil
	}
	diff := &StructuralIndexDiff{}

	// K-core comparison
	if baseline != nil && baseline.KCore != nil {
		diff.BaselineKCoreMax = baseline.KCore.MaxCore
		diff.BothKCoreVerified = baseline.KCore.Verified
	}
	if target != nil && target.KCore != nil {
		diff.TargetKCoreMax = target.KCore.MaxCore
		if diff.BothKCoreVerified {
			diff.BothKCoreVerified = target.KCore.Verified
		}
	}
	diff.KCoreMaxDiff = diff.TargetKCoreMax - diff.BaselineKCoreMax

	// Pivot comparison
	if baseline != nil && baseline.Pivot != nil {
		diff.BaselinePivots = baseline.Pivot.PivotCount
		diff.BothPivotVerified = baseline.Pivot.Verified
	}
	if target != nil && target.Pivot != nil {
		diff.TargetPivots = target.Pivot.PivotCount
		if diff.BothPivotVerified {
			diff.BothPivotVerified = target.Pivot.Verified
		}
	}
	diff.PivotsDiff = diff.TargetPivots - diff.BaselinePivots

	return diff
}

// compareGraphRAG compares GraphRAG query results
func compareGraphRAG(baseline, target *scenarios.GraphRAGResults) *GraphRAGDiff {
	if baseline == nil && target == nil {
		return nil
	}
	diff := &GraphRAGDiff{}

	// Local query comparison
	if baseline != nil && baseline.LocalQuery != nil {
		diff.BaselineLocalSuccess = baseline.LocalQuery.Success
		diff.BaselineLocalLatency = baseline.LocalQuery.LatencyMs
	}
	if target != nil && target.LocalQuery != nil {
		diff.TargetLocalSuccess = target.LocalQuery.Success
		diff.TargetLocalLatency = target.LocalQuery.LatencyMs
	}

	// Global query comparison
	if baseline != nil && baseline.GlobalQuery != nil {
		diff.BaselineGlobalSuccess = baseline.GlobalQuery.Success
		diff.BaselineGlobalLatency = baseline.GlobalQuery.LatencyMs
	}
	if target != nil && target.GlobalQuery != nil {
		diff.TargetGlobalSuccess = target.GlobalQuery.Success
		diff.TargetGlobalLatency = target.GlobalQuery.LatencyMs
	}

	return diff
}

// generateStructuredSummary generates the comparison summary
func generateStructuredSummary(report *StructuredComparisonReport, baseline, target *scenarios.TieredResults) StructuredCompareSummary {
	summary := StructuredCompareSummary{}

	// Tier capability diff
	summary.TierCapabilityDiff = fmt.Sprintf("%s → %s",
		baseline.Variant.Name, target.Variant.Name)

	// Search improvement based on queries with results and score
	targetBetter := report.Sections.Search.TargetWithResults > report.Sections.Search.BaselineWithResults ||
		report.Sections.Search.TargetAvgScore > report.Sections.Search.BaselineAvgScore*1.1
	baselineBetter := report.Sections.Search.BaselineWithResults > report.Sections.Search.TargetWithResults ||
		report.Sections.Search.BaselineAvgScore > report.Sections.Search.TargetAvgScore*1.1

	if targetBetter && !baselineBetter {
		summary.SearchImprovement = fmt.Sprintf("Target has better search (%d/%d queries with results, avg score %.3f)",
			report.Sections.Search.TargetWithResults, report.Sections.Search.TargetQueries,
			report.Sections.Search.TargetAvgScore)
	} else if baselineBetter && !targetBetter {
		summary.SearchImprovement = fmt.Sprintf("Baseline has better search (%d/%d queries with results, avg score %.3f)",
			report.Sections.Search.BaselineWithResults, report.Sections.Search.BaselineQueries,
			report.Sections.Search.BaselineAvgScore)
	} else {
		summary.SearchImprovement = "Similar search performance"
	}

	// Track regressions and improvements
	if report.Sections.Entities.TargetCount < report.Sections.Entities.BaselineCount {
		summary.Regressions = append(summary.Regressions,
			fmt.Sprintf("Entity count: %d → %d",
				report.Sections.Entities.BaselineCount,
				report.Sections.Entities.TargetCount))
	} else if report.Sections.Entities.TargetCount > report.Sections.Entities.BaselineCount {
		summary.Improvements = append(summary.Improvements,
			fmt.Sprintf("Entity count: %d → %d",
				report.Sections.Entities.BaselineCount,
				report.Sections.Entities.TargetCount))
	}

	if report.Sections.Indexes.PopulatedDiff > 0 {
		summary.Improvements = append(summary.Improvements,
			fmt.Sprintf("Indexes populated: +%d", report.Sections.Indexes.PopulatedDiff))
	} else if report.Sections.Indexes.PopulatedDiff < 0 {
		summary.Regressions = append(summary.Regressions,
			fmt.Sprintf("Indexes populated: %d", report.Sections.Indexes.PopulatedDiff))
	}

	if report.Sections.Communities != nil && report.Sections.Communities.TotalDiff > 0 {
		summary.Improvements = append(summary.Improvements,
			fmt.Sprintf("Communities detected: +%d", report.Sections.Communities.TotalDiff))
	}

	// PathRAG insights
	if report.Sections.PathRAG != nil {
		if report.Sections.PathRAG.EntitiesDiff > 0 {
			summary.Improvements = append(summary.Improvements,
				fmt.Sprintf("PathRAG: +%d entities discovered", report.Sections.PathRAG.EntitiesDiff))
		} else if report.Sections.PathRAG.EntitiesDiff < 0 {
			summary.Regressions = append(summary.Regressions,
				fmt.Sprintf("PathRAG: %d fewer entities discovered", report.Sections.PathRAG.EntitiesDiff))
		}
		if !report.Sections.PathRAG.BothScoresValid {
			summary.Regressions = append(summary.Regressions, "PathRAG score validation failed")
		}
	}

	// GraphRAG capability insights
	if report.Sections.GraphRAG != nil {
		if !report.Sections.GraphRAG.BaselineLocalSuccess && report.Sections.GraphRAG.TargetLocalSuccess {
			summary.Improvements = append(summary.Improvements, "GraphRAG local query now available")
		}
		if !report.Sections.GraphRAG.BaselineGlobalSuccess && report.Sections.GraphRAG.TargetGlobalSuccess {
			summary.Improvements = append(summary.Improvements, "GraphRAG global query now available")
		}
		if report.Sections.GraphRAG.BaselineLocalSuccess && !report.Sections.GraphRAG.TargetLocalSuccess {
			summary.Regressions = append(summary.Regressions, "GraphRAG local query lost")
		}
		if report.Sections.GraphRAG.BaselineGlobalSuccess && !report.Sections.GraphRAG.TargetGlobalSuccess {
			summary.Regressions = append(summary.Regressions, "GraphRAG global query lost")
		}
	}

	return summary
}

// printStructuredComparisonReport prints the comparison report
func printStructuredComparisonReport(report *StructuredComparisonReport) {
	fmt.Println("\n=== Structured Tier Comparison ===")
	fmt.Printf("Baseline: %s (%s)\n", report.BaselineVariant, report.BaselineFile)
	fmt.Printf("Target:   %s (%s)\n", report.TargetVariant, report.TargetFile)
	fmt.Printf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))

	fmt.Println("\n--- Variant ---")
	fmt.Printf("  Provider: %s → %s\n",
		report.Sections.Variant.BaselineProvider,
		report.Sections.Variant.TargetProvider)

	fmt.Println("\n--- Entities ---")
	fmt.Printf("  Count: %d → %d (diff: %+d)\n",
		report.Sections.Entities.BaselineCount,
		report.Sections.Entities.TargetCount,
		report.Sections.Entities.CountDiff)
	fmt.Printf("  Data Loss: %.1f%% → %.1f%%\n",
		report.Sections.Entities.BaselineLoss,
		report.Sections.Entities.TargetLoss)

	fmt.Println("\n--- Indexes ---")
	fmt.Printf("  Populated: %d → %d (diff: %+d)\n",
		report.Sections.Indexes.BaselinePopulated,
		report.Sections.Indexes.TargetPopulated,
		report.Sections.Indexes.PopulatedDiff)
	if len(report.Sections.Indexes.DifferingIndexes) > 0 {
		fmt.Printf("  Differing: %s\n", strings.Join(report.Sections.Indexes.DifferingIndexes, ", "))
	}

	fmt.Println("\n--- Search Quality ---")
	fmt.Printf("  Queries with results: %d/%d → %d/%d\n",
		report.Sections.Search.BaselineWithResults,
		report.Sections.Search.BaselineQueries,
		report.Sections.Search.TargetWithResults,
		report.Sections.Search.TargetQueries)
	fmt.Printf("  Avg Score: %.3f → %.3f\n",
		report.Sections.Search.BaselineAvgScore,
		report.Sections.Search.TargetAvgScore)
	fmt.Printf("  Known Answer Tests: %.0f%% → %.0f%%\n",
		report.Sections.Search.BaselineKnownAnswerPct,
		report.Sections.Search.TargetKnownAnswerPct)

	if len(report.Sections.Search.QueryDiffs) > 0 {
		fmt.Println("\n  Query-by-Query:")
		for _, qd := range report.Sections.Search.QueryDiffs {
			fmt.Printf("    %q: %d → %d hits (%s)\n",
				truncate(qd.Query, 30),
				qd.BaselineHits,
				qd.TargetHits,
				qd.Insight)
		}
	}

	fmt.Println("\n--- Rules ---")
	fmt.Printf("  Evaluated: %d → %d\n",
		report.Sections.Rules.BaselineEvaluated,
		report.Sections.Rules.TargetEvaluated)
	fmt.Printf("  Triggered: %d → %d\n",
		report.Sections.Rules.BaselineTriggered,
		report.Sections.Rules.TargetTriggered)

	if report.Sections.Communities != nil {
		fmt.Println("\n--- Communities ---")
		fmt.Printf("  Total: %d → %d (diff: %+d)\n",
			report.Sections.Communities.BaselineTotal,
			report.Sections.Communities.TargetTotal,
			report.Sections.Communities.TotalDiff)
		fmt.Printf("  Largest: %d → %d\n",
			report.Sections.Communities.BaselineLargest,
			report.Sections.Communities.TargetLargest)
	}

	// PathRAG (Tier 0 - runs on all tiers)
	if report.Sections.PathRAG != nil {
		fmt.Println("\n--- PathRAG (Tier 0) ---")
		fmt.Printf("  Entities Found: %d → %d (diff: %+d)\n",
			report.Sections.PathRAG.BaselineEntities,
			report.Sections.PathRAG.TargetEntities,
			report.Sections.PathRAG.EntitiesDiff)
		fmt.Printf("  Latency: %dms → %dms\n",
			report.Sections.PathRAG.BaselineLatency,
			report.Sections.PathRAG.TargetLatency)
		fmt.Printf("  Scores Valid: %v\n", report.Sections.PathRAG.BothScoresValid)
	}

	// Structural Indexes (Tier 0)
	if report.Sections.StructuralIdx != nil {
		fmt.Println("\n--- Structural Indexes (Tier 0) ---")
		fmt.Printf("  K-Core Max: %d → %d (diff: %+d)\n",
			report.Sections.StructuralIdx.BaselineKCoreMax,
			report.Sections.StructuralIdx.TargetKCoreMax,
			report.Sections.StructuralIdx.KCoreMaxDiff)
		fmt.Printf("  Pivots: %d → %d (diff: %+d)\n",
			report.Sections.StructuralIdx.BaselinePivots,
			report.Sections.StructuralIdx.TargetPivots,
			report.Sections.StructuralIdx.PivotsDiff)
		fmt.Printf("  K-Core Verified: %v, Pivot Verified: %v\n",
			report.Sections.StructuralIdx.BothKCoreVerified,
			report.Sections.StructuralIdx.BothPivotVerified)
	}

	// GraphRAG (Tier 2 - semantic only)
	if report.Sections.GraphRAG != nil {
		fmt.Println("\n--- GraphRAG (Tier 2) ---")
		fmt.Printf("  Local Query: %v → %v",
			report.Sections.GraphRAG.BaselineLocalSuccess,
			report.Sections.GraphRAG.TargetLocalSuccess)
		if report.Sections.GraphRAG.TargetLocalSuccess {
			fmt.Printf(" (%dms)", report.Sections.GraphRAG.TargetLocalLatency)
		}
		fmt.Println()
		fmt.Printf("  Global Query: %v → %v",
			report.Sections.GraphRAG.BaselineGlobalSuccess,
			report.Sections.GraphRAG.TargetGlobalSuccess)
		if report.Sections.GraphRAG.TargetGlobalSuccess {
			fmt.Printf(" (%dms)", report.Sections.GraphRAG.TargetGlobalLatency)
		}
		fmt.Println()
	}

	fmt.Println("\n--- Summary ---")
	fmt.Printf("  Capability: %s\n", report.Summary.TierCapabilityDiff)
	fmt.Printf("  Search: %s\n", report.Summary.SearchImprovement)
	if len(report.Summary.Improvements) > 0 {
		fmt.Printf("  Improvements: %s\n", strings.Join(report.Summary.Improvements, "; "))
	}
	if len(report.Summary.Regressions) > 0 {
		fmt.Printf("  Regressions: %s\n", strings.Join(report.Summary.Regressions, "; "))
	}
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// findLatestStructuredResult finds the latest structured result file for a variant
func findLatestStructuredResult(outputDir, variant string) (string, error) {
	pattern := filepath.Join(outputDir, fmt.Sprintf("%s-*.json", variant))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob failed: %w", err)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no structured result files found for variant %s", variant)
	}

	// Sort by name (timestamp is in filename, so latest will be last)
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// handleAutoCompareCommand automatically finds latest results for two variants and compares
func handleAutoCompareCommand(logger *slog.Logger, outputDir, baselineVariant, targetVariant string) int {
	if outputDir == "" {
		outputDir = "test/e2e/results"
	}

	// Find latest files for each variant
	baselineFile, err := findLatestStructuredResult(outputDir, baselineVariant)
	if err != nil {
		logger.Error("Failed to find baseline", "variant", baselineVariant, "error", err)
		return 1
	}

	targetFile, err := findLatestStructuredResult(outputDir, targetVariant)
	if err != nil {
		logger.Error("Failed to find target", "variant", targetVariant, "error", err)
		return 1
	}

	logger.Info("Comparing latest results",
		"baseline", baselineFile,
		"target", targetFile)

	return handleStructuredCompareCommand(logger, baselineFile, targetFile)
}
