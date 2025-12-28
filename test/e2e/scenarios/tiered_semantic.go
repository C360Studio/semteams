// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
)

// Semantic tier validation functions (community comparison, LLM enhancement)

// detectCommunityVariant determines if running structural, statistical, or semantic variant
func (s *TieredScenario) detectCommunityVariant(result *Result) string {
	// Check if already detected in comparison stage
	if v, ok := result.Metrics["comparison_variant"].(string); ok {
		return v
	}
	// Check if variant was set in result metrics
	if v, ok := result.Metrics["variant"].(string); ok {
		return v
	}
	// Fallback to semembed detection
	if semembedAvailable, ok := result.Details["semembed_available"].(bool); ok && semembedAvailable {
		return "semantic"
	}
	return "statistical"
}

// waitForCommunities polls until communities are available
func (s *TieredScenario) waitForCommunities(ctx context.Context) ([]*client.Community, error) {
	var communities []*client.Community
	var err error
	for i := 0; i < 50; i++ { // Max 5 seconds (50 * 100ms)
		communities, err = s.natsClient.GetAllCommunities(ctx)
		if err == nil && len(communities) > 0 {
			return communities, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return communities, err
}

// waitForLLMEnhancement waits for LLM enhancement to complete for ML variant
func (s *TieredScenario) waitForLLMEnhancement(
	ctx context.Context,
	communityCount int,
	result *Result,
) llmWaitResult {
	fmt.Printf("[LLM WAIT] Waiting for LLM enhancement to complete (ML variant, %d communities)...\n", communityCount)

	enhanceStart := time.Now()
	enhanced, failed, pending, waitErr := s.natsClient.WaitForCommunityEnhancement(
		ctx, 2*time.Minute, 2*time.Second,
	)
	waitResult := llmWaitResult{
		durationMs:   time.Since(enhanceStart).Milliseconds(),
		failedCount:  failed,
		pendingCount: pending,
	}

	fmt.Printf("[LLM WAIT] Complete: enhanced=%d, failed=%d, pending=%d, duration=%dms\n",
		enhanced, failed, pending, waitResult.durationMs)

	if waitErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("LLM enhancement wait error: %v", waitErr))
	}
	if enhanced == 0 && failed == 0 && pending > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("No LLM enhancements completed within 2 minute timeout (%d still pending)", pending))
	}

	result.Metrics["llm_wait_duration_ms"] = float64(waitResult.durationMs)
	result.Metrics["llm_failed_count"] = float64(waitResult.failedCount)
	result.Metrics["llm_pending_count"] = float64(waitResult.pendingCount)

	return waitResult
}

// analyzeCommunities computes statistics and comparisons for communities
func (s *TieredScenario) analyzeCommunities(communities []*client.Community) communityStats {
	stats := communityStats{comparisons: make([]CommunityComparison, 0, len(communities))}
	var totalLengthRatio, totalWordOverlap float64
	var ratioCount, totalNonSingletonMembers int

	for _, comm := range communities {
		comparison := s.buildCommunityComparison(comm, &totalLengthRatio, &totalWordOverlap, &ratioCount)

		if len(comm.Members) > 1 {
			stats.nonSingletonCount++
			totalNonSingletonMembers += len(comm.Members)
			if len(comm.Members) > stats.largestCommunitySize {
				stats.largestCommunitySize = len(comm.Members)
			}
		}

		switch comm.SummaryStatus {
		case "llm-enhanced":
			stats.llmEnhancedCount++
		case "statistical", "":
			stats.statisticalOnlyCount++
		}

		stats.comparisons = append(stats.comparisons, comparison)
	}

	if ratioCount > 0 {
		stats.avgLengthRatio = totalLengthRatio / float64(ratioCount)
		stats.avgWordOverlap = totalWordOverlap / float64(ratioCount)
	}
	if stats.nonSingletonCount > 0 {
		stats.avgNonSingletonSize = float64(totalNonSingletonMembers) / float64(stats.nonSingletonCount)
	}

	return stats
}

// buildCommunityComparison creates a comparison record for a single community
func (s *TieredScenario) buildCommunityComparison(
	comm *client.Community,
	totalLengthRatio, totalWordOverlap *float64,
	ratioCount *int,
) CommunityComparison {
	comparison := CommunityComparison{
		CommunityID:        comm.ID,
		Level:              comm.Level,
		MemberCount:        len(comm.Members),
		StatisticalSummary: comm.StatisticalSummary,
		LLMSummary:         comm.LLMSummary,
		SummaryStatus:      comm.SummaryStatus,
		Keywords:           comm.Keywords,
	}

	if comm.LLMSummary != "" && comm.StatisticalSummary != "" && len(comm.StatisticalSummary) > 0 {
		comparison.SummaryLengthRatio = float64(len(comm.LLMSummary)) / float64(len(comm.StatisticalSummary))
		*totalLengthRatio += comparison.SummaryLengthRatio
		*ratioCount++
		comparison.WordOverlap = wordJaccard(comm.StatisticalSummary, comm.LLMSummary)
		*totalWordOverlap += comparison.WordOverlap
	}

	return comparison
}

// llmQualityIssue represents a quality issue found in LLM summaries
type llmQualityIssue struct {
	CommunityID string
	Issue       string
}

// validateLLMSummaryQuality validates quality of LLM-enhanced community summaries
func (s *TieredScenario) validateLLMSummaryQuality(communities []*client.Community) []llmQualityIssue {
	var issues []llmQualityIssue

	for _, comm := range communities {
		if comm.SummaryStatus != "llm-enhanced" {
			continue
		}

		// Check minimum summary length (50 chars)
		if len(comm.LLMSummary) < 50 {
			issues = append(issues, llmQualityIssue{
				CommunityID: comm.ID,
				Issue:       fmt.Sprintf("LLM summary too short: %d chars (min 50)", len(comm.LLMSummary)),
			})
			continue
		}

		// Check that at least one keyword appears in the summary
		keywordFound := false
		summaryLower := strings.ToLower(comm.LLMSummary)
		for _, kw := range comm.Keywords {
			if strings.Contains(summaryLower, strings.ToLower(kw)) {
				keywordFound = true
				break
			}
		}

		if !keywordFound && len(comm.Keywords) > 0 {
			issues = append(issues, llmQualityIssue{
				CommunityID: comm.ID,
				Issue:       fmt.Sprintf("LLM summary contains no keywords (keywords: %v)", comm.Keywords),
			})
		}

		// Check that LLM summary is more detailed (longer) than statistical summary
		if comm.StatisticalSummary != "" && len(comm.LLMSummary) <= len(comm.StatisticalSummary) {
			issues = append(issues, llmQualityIssue{
				CommunityID: comm.ID,
				Issue: fmt.Sprintf("LLM summary (%d chars) not longer than statistical (%d chars)",
					len(comm.LLMSummary), len(comm.StatisticalSummary)),
			})
		}
	}

	return issues
}

// persistCommunityReport saves the community comparison report to a JSON file
func (s *TieredScenario) persistCommunityReport(
	variant string,
	stats communityStats,
	llmWait llmWaitResult,
	result *Result,
) string {
	if s.config.OutputDir == "" {
		return ""
	}

	report := CommunitySummaryReport{
		Variant:               variant,
		Timestamp:             time.Now(),
		CommunitiesTotal:      len(stats.comparisons),
		LLMEnhancedCount:      stats.llmEnhancedCount,
		StatisticalOnlyCount:  stats.statisticalOnlyCount,
		LLMFailedCount:        llmWait.failedCount,
		LLMPendingCount:       llmWait.pendingCount,
		LLMWaitDurationMs:     llmWait.durationMs,
		AvgSummaryLengthRatio: stats.avgLengthRatio,
		AvgWordOverlap:        stats.avgWordOverlap,
		NonSingletonCount:     stats.nonSingletonCount,
		LargestCommunitySize:  stats.largestCommunitySize,
		AvgNonSingletonSize:   stats.avgNonSingletonSize,
		Communities:           stats.comparisons,
	}

	filename := fmt.Sprintf("community-comparison-%s-%s.json", variant, time.Now().Format("20060102-150405"))
	comparisonFile := filepath.Join(s.config.OutputDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		if err := os.WriteFile(comparisonFile, data, 0644); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to write community comparison file: %v", err))
		}
	}

	return comparisonFile
}

// recordCommunityMetrics records community statistics to result metrics
func (s *TieredScenario) recordCommunityMetrics(stats communityStats, result *Result) {
	result.Metrics["communities_total"] = len(stats.comparisons)
	result.Metrics["communities_llm_enhanced"] = stats.llmEnhancedCount
	result.Metrics["communities_statistical_only"] = stats.statisticalOnlyCount
	result.Metrics["avg_summary_length_ratio"] = stats.avgLengthRatio
	result.Metrics["avg_word_overlap"] = stats.avgWordOverlap
	result.Metrics["communities_non_singleton"] = stats.nonSingletonCount
	result.Metrics["largest_community_size"] = stats.largestCommunitySize
	result.Metrics["avg_non_singleton_size"] = stats.avgNonSingletonSize
}

// executeValidateLLMEnhancement validates LLM enhancement of communities for semantic tier.
// This step waits for LLM enhancement to complete (up to 2 min), analyzes community
// summary status, and validates that enhancement is working properly.
func (s *TieredScenario) executeValidateLLMEnhancement(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping LLM enhancement validation")
		return nil
	}

	fmt.Println("[LLM ENHANCEMENT] Starting LLM enhancement validation...")

	// Wait for communities to be available
	communities, err := s.waitForCommunities(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get communities: %v", err))
		return nil
	}

	if len(communities) == 0 {
		result.Warnings = append(result.Warnings, "No communities found for LLM enhancement validation")
		return nil
	}

	fmt.Printf("[LLM ENHANCEMENT] Found %d communities, waiting for LLM enhancement...\n", len(communities))

	// Wait for LLM enhancement to complete
	llmWait := s.waitForLLMEnhancement(ctx, len(communities), result)

	// Re-fetch communities after waiting (they may have been updated)
	communities, err = s.natsClient.GetAllCommunities(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to re-fetch communities after LLM wait: %v", err))
		return nil
	}

	// Analyze communities for summary status
	stats := s.analyzeCommunities(communities)

	// Record metrics
	s.recordCommunityMetrics(stats, result)

	// Persist detailed report
	variant := s.detectCommunityVariant(result)
	reportFile := s.persistCommunityReport(variant, stats, llmWait, result)
	if reportFile != "" {
		fmt.Printf("[LLM ENHANCEMENT] Wrote community report to %s\n", reportFile)
	}

	// Validate LLM summary quality for enhanced communities
	issues := s.validateLLMSummaryQuality(communities)
	if len(issues) > 0 {
		for _, issue := range issues {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("LLM quality issue in %s: %s", issue.CommunityID, issue.Issue))
		}
	}

	// Log summary
	fmt.Printf("[LLM ENHANCEMENT] Results: llm_enhanced=%d, statistical_only=%d, failed=%d, pending=%d\n",
		stats.llmEnhancedCount, stats.statisticalOnlyCount, llmWait.failedCount, llmWait.pendingCount)

	// Validate enhancement is working
	if stats.llmEnhancedCount == 0 {
		if llmWait.failedCount > 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("LLM enhancement failed for all %d communities - check seminstruct logs", llmWait.failedCount))
		} else if llmWait.pendingCount > 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("No LLM enhancements completed within timeout (%d still pending) - enhancement may be slow or worker not started", llmWait.pendingCount))
		} else {
			result.Warnings = append(result.Warnings,
				"No communities have LLM enhancement (all show statistical status) - verify enhancement worker is enabled")
		}
	} else {
		fmt.Printf("[LLM ENHANCEMENT] Success: %d/%d communities LLM-enhanced\n",
			stats.llmEnhancedCount, len(communities))
	}

	return nil
}

// executeValidateAnomalyDetection validates structural anomaly detection results for semantic tier.
// This step waits for anomaly detection to complete, then retrieves anomaly counts from the
// ANOMALY_INDEX KV bucket and records metrics for semantic gaps (pivot distance), core anomalies
// (k-core analysis), and transitivity gaps.
func (s *TieredScenario) executeValidateAnomalyDetection(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping anomaly detection validation")
		return nil
	}

	fmt.Println("[ANOMALY DETECTION] Waiting for anomaly detection to complete...")

	// Wait for anomaly detection to stabilize (30s timeout, 2s poll interval)
	// Anomaly detection runs asynchronously during community detection, so we need to wait
	// for it to complete before reading final counts
	total, waitErr := s.natsClient.WaitForAnomalyDetection(ctx, 30*time.Second, 2*time.Second)
	if waitErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Anomaly detection wait error: %v", waitErr))
	}

	fmt.Printf("[ANOMALY DETECTION] Detection complete, found %d anomalies\n", total)

	// Get anomaly counts by type and status
	counts, err := s.natsClient.GetAnomalyCounts(ctx)
	if err != nil {
		// Anomaly detection may not have run or bucket may not exist - this is a warning, not error
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get anomaly counts: %v", err))
		result.Metrics["anomalies_total"] = 0
		result.Metrics["anomalies_semantic_gap"] = 0
		result.Metrics["anomalies_core_isolation"] = 0
		result.Metrics["anomalies_core_demotion"] = 0
		result.Metrics["anomalies_transitivity"] = 0
		return nil
	}

	// Record metrics by anomaly type
	result.Metrics["anomalies_total"] = counts.Total
	result.Metrics["anomalies_semantic_gap"] = counts.ByType["semantic_structural_gap"]
	result.Metrics["anomalies_core_isolation"] = counts.ByType["core_isolation"]
	result.Metrics["anomalies_core_demotion"] = counts.ByType["core_demotion"]
	result.Metrics["anomalies_transitivity"] = counts.ByType["transitivity_gap"]

	// Record metrics by status
	result.Metrics["anomalies_pending"] = counts.ByStatus["pending"]
	result.Metrics["anomalies_confirmed"] = counts.ByStatus["confirmed"]
	result.Metrics["anomalies_dismissed"] = counts.ByStatus["dismissed"]

	// Log results
	fmt.Printf("[ANOMALY DETECTION] Results: total=%d, semantic_gap=%d, core_isolation=%d, core_demotion=%d, transitivity=%d\n",
		counts.Total,
		counts.ByType["semantic_structural_gap"],
		counts.ByType["core_isolation"],
		counts.ByType["core_demotion"],
		counts.ByType["transitivity_gap"])

	fmt.Printf("[ANOMALY DETECTION] Status: pending=%d, confirmed=%d, dismissed=%d\n",
		counts.ByStatus["pending"],
		counts.ByStatus["confirmed"],
		counts.ByStatus["dismissed"])

	// Validation: at least some anomalies should be detected for semantic tier
	if counts.Total == 0 {
		result.Warnings = append(result.Warnings,
			"No anomalies detected - verify anomaly detection is enabled and running during community detection")
	} else {
		fmt.Printf("[ANOMALY DETECTION] Success: %d total anomalies detected\n", counts.Total)
	}

	// Semantic gap detector requires embeddings - should have results for semantic tier
	if counts.ByType["semantic_structural_gap"] == 0 {
		result.Warnings = append(result.Warnings,
			"No semantic gap anomalies detected - verify semembed is available and pivot index is built")
	}

	return nil
}

// NOTE: executeCompareCommunities removed - use CLI compare instead:
//   ./e2e --compare-structured --baseline results/statistical.json --target results/semantic.json
// Community data is captured in structured results by executeValidateCommunityStructure.

// wordJaccard calculates Jaccard similarity on word sets
func wordJaccard(a, b string) float64 {
	wordsA := toWordSet(strings.ToLower(a))
	wordsB := toWordSet(strings.ToLower(b))

	intersection := 0
	for word := range wordsA {
		if wordsB[word] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// toWordSet converts a string to a set of words (excluding short words and punctuation)
func toWordSet(s string) map[string]bool {
	words := strings.Fields(s)
	set := make(map[string]bool)
	for _, w := range words {
		// Remove punctuation
		w = strings.Trim(w, ".,!?;:()[]{}\"'")
		// Skip short words (less than 3 characters)
		if len(w) > 2 {
			set[w] = true
		}
	}
	return set
}
