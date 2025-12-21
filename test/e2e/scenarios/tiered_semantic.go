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

// executeCompareCommunities compares statistical vs LLM-enhanced community summaries
func (s *TieredScenario) executeCompareCommunities(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping community comparison")
		return nil
	}

	variant := s.detectCommunityVariant(result)
	communities, err := s.waitForCommunities(ctx)

	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get communities: %v", err))
		return nil
	}
	if len(communities) == 0 {
		result.Warnings = append(result.Warnings, "No communities found for comparison (clustering may not have completed)")
		result.Metrics["communities_total"] = 0
		return nil
	}

	var llmWait llmWaitResult
	if variant == "semantic" {
		llmWait = s.waitForLLMEnhancement(ctx, len(communities), result)
		// Refresh communities after waiting
		if refreshed, err := s.natsClient.GetAllCommunities(ctx); err == nil {
			communities = refreshed
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to refresh communities after LLM wait: %v", err))
		}
	}

	stats := s.analyzeCommunities(communities)
	s.recordCommunityMetrics(stats, result)

	// For semantic tier, require at least one LLM-enhanced community
	// This verifies the progressive enhancement workflow is working:
	// 1. Communities detected by clustering
	// 2. Statistical summaries generated immediately
	// 3. LLM enhancement worker processes communities asynchronously
	// 4. At least one community gets LLM-enhanced summary
	if variant == "semantic" && stats.llmEnhancedCount == 0 {
		return fmt.Errorf("semantic tier requires at least one LLM-enhanced community, got 0 (progressive enhancement failed)")
	}

	// Semantic tier MUST produce non-singleton communities (neural embeddings find semantic similarity)
	if variant == "semantic" && stats.nonSingletonCount == 0 {
		return fmt.Errorf("semantic tier should produce non-singleton communities but found 0 (clustering may have failed)")
	}

	// Validate LLM summary quality for semantic tier
	if variant == "semantic" {
		qualityIssues := s.validateLLMSummaryQuality(communities)
		for _, issue := range qualityIssues {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("LLM quality issue in %s: %s", issue.CommunityID, issue.Issue))
		}
		result.Metrics["llm_quality_issues"] = len(qualityIssues)
	}

	comparisonFile := s.persistCommunityReport(variant, stats, llmWait, result)

	result.Details["community_comparison"] = map[string]any{
		"total":                  len(stats.comparisons),
		"llm_enhanced":           stats.llmEnhancedCount,
		"statistical_only":       stats.statisticalOnlyCount,
		"avg_length_ratio":       stats.avgLengthRatio,
		"avg_word_overlap":       stats.avgWordOverlap,
		"non_singleton_count":    stats.nonSingletonCount,
		"largest_community_size": stats.largestCommunitySize,
		"avg_non_singleton_size": stats.avgNonSingletonSize,
		"comparison_file":        comparisonFile,
		"communities":            stats.comparisons,
		"message": fmt.Sprintf("Compared %d communities: %d LLM-enhanced, %d statistical only, %d non-singleton",
			len(stats.comparisons), stats.llmEnhancedCount, stats.statisticalOnlyCount, stats.nonSingletonCount),
	}

	return nil
}

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
