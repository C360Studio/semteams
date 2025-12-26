// Package main provides comparison analysis for Statistical vs Semantic search results
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/scenarios"
)

// ComparisonReport represents the full Statistical vs Semantic comparison report
type ComparisonReport struct {
	StatisticalVariant   string            `json:"statistical_variant"`
	SemanticVariant      string            `json:"semantic_variant"`
	StatisticalTimestamp time.Time         `json:"statistical_timestamp"`
	SemanticTimestamp    time.Time         `json:"semantic_timestamp"`
	Queries              []QueryComparison `json:"queries"`
	Summary              ComparisonSummary `json:"summary"`
}

// QueryComparison represents comparison results for a single query
type QueryComparison struct {
	Query               string   `json:"query"`
	StatisticalHits     []string `json:"statistical_hits"`
	SemanticHits        []string `json:"semantic_hits"`
	HitOverlap          float64  `json:"hit_overlap"` // Jaccard similarity (intersection/union)
	ScoreCorr           float64  `json:"score_corr"`  // Pearson correlation of shared hit scores
	StatisticalAvgScore float64  `json:"statistical_avg_score"`
	SemanticAvgScore    float64  `json:"semantic_avg_score"`
	Insight             string   `json:"insight"` // "Semantic finds more results", "Similar results", etc.
}

// ComparisonSummary summarizes the overall comparison
type ComparisonSummary struct {
	AvgHitOverlap          float64 `json:"avg_hit_overlap"`          // Average Jaccard across queries
	AvgScoreCorr           float64 `json:"avg_score_corr"`           // Average correlation
	SemanticBetterCount    int     `json:"semantic_better_count"`    // Queries where Semantic found more relevant results
	StatisticalBetterCount int     `json:"statistical_better_count"` // Queries where Statistical found more relevant results
	TiedCount              int     `json:"tied_count"`               // Queries where results were similar
	Verdict                string  `json:"verdict"`                  // "Semantic provides semantic lift" / "Marginal difference"
}

// analyzeComparison generates a comparison report from Statistical and Semantic comparison files
func analyzeComparison(outputDir string) (*ComparisonReport, error) {
	// Find latest statistical and semantic comparison files
	statisticalFile, err := findLatestComparison(outputDir, "statistical")
	if err != nil {
		return nil, fmt.Errorf("failed to find statistical comparison: %w", err)
	}

	semanticFile, err := findLatestComparison(outputDir, "semantic")
	if err != nil {
		return nil, fmt.Errorf("failed to find semantic comparison: %w", err)
	}

	// Load comparison data
	statisticalData, err := loadComparisonData(statisticalFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load statistical data: %w", err)
	}

	semanticData, err := loadComparisonData(semanticFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load semantic data: %w", err)
	}

	// Generate comparison report
	return generateComparisonReport(statisticalData, semanticData), nil
}

// findLatestComparison finds the most recent comparison file for a variant
func findLatestComparison(outputDir, variant string) (string, error) {
	pattern := filepath.Join(outputDir, fmt.Sprintf("comparison-%s-*.json", variant))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob failed: %w", err)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no comparison files found for variant %s", variant)
	}

	// Sort by name (timestamp is in filename, so latest will be last)
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// loadComparisonData loads comparison data from a JSON file
func loadComparisonData(filepath string) (*scenarios.ComparisonData, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var compData scenarios.ComparisonData
	if err := json.Unmarshal(data, &compData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &compData, nil
}

// generateComparisonReport creates a comparison report from Statistical and Semantic data
func generateComparisonReport(statisticalData, semanticData *scenarios.ComparisonData) *ComparisonReport {
	report := &ComparisonReport{
		StatisticalVariant:   statisticalData.Variant,
		SemanticVariant:      semanticData.Variant,
		StatisticalTimestamp: statisticalData.Timestamp,
		SemanticTimestamp:    semanticData.Timestamp,
		Queries:              []QueryComparison{},
	}

	// Get all unique queries
	querySet := make(map[string]bool)
	for query := range statisticalData.SearchResults {
		querySet[query] = true
	}
	for query := range semanticData.SearchResults {
		querySet[query] = true
	}

	// Compare each query
	var totalJaccard, totalCorr float64
	var corrCount int
	semanticBetter, statisticalBetter, tied := 0, 0, 0

	for query := range querySet {
		statisticalResult := statisticalData.SearchResults[query]
		semanticResult := semanticData.SearchResults[query]

		qc := QueryComparison{
			Query:           query,
			StatisticalHits: statisticalResult.Hits,
			SemanticHits:    semanticResult.Hits,
		}

		// Calculate Jaccard similarity
		qc.HitOverlap = jaccard(statisticalResult.Hits, semanticResult.Hits)
		totalJaccard += qc.HitOverlap

		// Calculate average scores
		qc.StatisticalAvgScore = avgScore(statisticalResult.Scores)
		qc.SemanticAvgScore = avgScore(semanticResult.Scores)

		// Calculate score correlation for shared hits
		statisticalScoreMap := buildScoreMap(statisticalResult.Hits, statisticalResult.Scores)
		semanticScoreMap := buildScoreMap(semanticResult.Hits, semanticResult.Scores)
		qc.ScoreCorr = scoreCorrelation(statisticalScoreMap, semanticScoreMap)
		if !math.IsNaN(qc.ScoreCorr) {
			totalCorr += qc.ScoreCorr
			corrCount++
		}

		// Determine insight
		qc.Insight = determineInsight(statisticalResult, semanticResult, qc.HitOverlap)

		// Track which is better
		if len(semanticResult.Hits) > len(statisticalResult.Hits) && qc.SemanticAvgScore >= qc.StatisticalAvgScore {
			semanticBetter++
		} else if len(statisticalResult.Hits) > len(semanticResult.Hits) && qc.StatisticalAvgScore > qc.SemanticAvgScore {
			statisticalBetter++
		} else {
			tied++
		}

		report.Queries = append(report.Queries, qc)
	}

	// Calculate summary
	queryCount := len(querySet)
	report.Summary = ComparisonSummary{
		AvgHitOverlap:          totalJaccard / float64(queryCount),
		SemanticBetterCount:    semanticBetter,
		StatisticalBetterCount: statisticalBetter,
		TiedCount:              tied,
	}

	if corrCount > 0 {
		report.Summary.AvgScoreCorr = totalCorr / float64(corrCount)
	}

	// Determine verdict
	report.Summary.Verdict = determineVerdict(report.Summary)

	return report
}

// jaccard calculates Jaccard similarity between two string slices
func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0 // Both empty = perfect match
	}

	setA := toSet(a)
	setB := toSet(b)

	intersection := 0
	for item := range setA {
		if setB[item] {
			intersection++
		}
	}

	union := len(setA)
	for item := range setB {
		if !setA[item] {
			union++
		}
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// toSet converts a slice to a set (map)
func toSet(items []string) map[string]bool {
	set := make(map[string]bool)
	for _, item := range items {
		set[item] = true
	}
	return set
}

// avgScore calculates the average of a score slice
func avgScore(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range scores {
		sum += s
	}
	return sum / float64(len(scores))
}

// buildScoreMap creates a map of entity ID to score
func buildScoreMap(hits []string, scores []float64) map[string]float64 {
	scoreMap := make(map[string]float64)
	for i, hit := range hits {
		if i < len(scores) {
			scoreMap[hit] = scores[i]
		}
	}
	return scoreMap
}

// scoreCorrelation calculates Pearson correlation for shared hits
func scoreCorrelation(coreScores, mlScores map[string]float64) float64 {
	// Find shared entity IDs
	var coreVals, mlVals []float64
	for entityID, coreScore := range coreScores {
		if mlScore, exists := mlScores[entityID]; exists {
			coreVals = append(coreVals, coreScore)
			mlVals = append(mlVals, mlScore)
		}
	}

	if len(coreVals) < 2 {
		return math.NaN() // Need at least 2 points for correlation
	}

	// Calculate Pearson correlation
	return pearsonCorrelation(coreVals, mlVals)
}

// pearsonCorrelation calculates the Pearson correlation coefficient
func pearsonCorrelation(x, y []float64) float64 {
	n := float64(len(x))
	if n == 0 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}

	numerator := n*sumXY - sumX*sumY
	denominator := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))

	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

// determineInsight generates a human-readable insight for a query comparison
func determineInsight(statisticalResult, semanticResult scenarios.SearchQueryResult, overlap float64) string {
	statisticalCnt := len(statisticalResult.Hits)
	semanticCnt := len(semanticResult.Hits)

	if statisticalCnt == 0 && semanticCnt == 0 {
		return "No results from either variant"
	}
	if statisticalCnt == 0 {
		return "Semantic found results where Statistical found none"
	}
	if semanticCnt == 0 {
		return "Statistical found results where Semantic found none"
	}
	if overlap > 0.8 {
		return "Very similar results"
	}
	if overlap > 0.5 {
		return "Moderate overlap in results"
	}
	if semanticCnt > statisticalCnt {
		return "Semantic finds more results"
	}
	if statisticalCnt > semanticCnt {
		return "Statistical finds more results"
	}
	return "Different but equal-sized result sets"
}

// determineVerdict generates the overall comparison verdict
func determineVerdict(summary ComparisonSummary) string {
	if summary.SemanticBetterCount > summary.StatisticalBetterCount+summary.TiedCount {
		return "Semantic provides significant improvement"
	}
	if summary.SemanticBetterCount > summary.StatisticalBetterCount {
		return "Semantic provides moderate improvement"
	}
	if summary.StatisticalBetterCount > summary.SemanticBetterCount {
		return "Statistical performs better for this dataset"
	}
	if summary.AvgHitOverlap > 0.7 {
		return "Marginal difference - results highly similar"
	}
	return "Mixed results - no clear winner"
}

// printAnalysisReport prints the comparison report to stdout
func printAnalysisReport(report *ComparisonReport) {
	fmt.Println("\n=== Statistical vs Semantic Search Comparison Report ===")
	fmt.Printf("Statistical timestamp: %s\n", report.StatisticalTimestamp.Format(time.RFC3339))
	fmt.Printf("Semantic timestamp:    %s\n", report.SemanticTimestamp.Format(time.RFC3339))

	fmt.Println("\n--- Query-by-Query Comparison ---")
	for _, qc := range report.Queries {
		fmt.Printf("\nQuery: %q\n", qc.Query)
		fmt.Printf("  Statistical hits: %d, avg score: %.3f\n", len(qc.StatisticalHits), qc.StatisticalAvgScore)
		fmt.Printf("  Semantic hits:    %d, avg score: %.3f\n", len(qc.SemanticHits), qc.SemanticAvgScore)
		fmt.Printf("  Hit overlap (Jaccard): %.2f\n", qc.HitOverlap)
		if !math.IsNaN(qc.ScoreCorr) {
			fmt.Printf("  Score correlation: %.2f\n", qc.ScoreCorr)
		}
		fmt.Printf("  Insight: %s\n", qc.Insight)

		// Show hit details if different
		if len(qc.StatisticalHits) > 0 || len(qc.SemanticHits) > 0 {
			fmt.Printf("  Statistical top 3: %s\n", formatTopHits(qc.StatisticalHits, 3))
			fmt.Printf("  Semantic top 3:    %s\n", formatTopHits(qc.SemanticHits, 3))
		}
	}

	fmt.Println("\n--- Summary ---")
	fmt.Printf("Average hit overlap (Jaccard): %.2f\n", report.Summary.AvgHitOverlap)
	if !math.IsNaN(report.Summary.AvgScoreCorr) {
		fmt.Printf("Average score correlation:     %.2f\n", report.Summary.AvgScoreCorr)
	}
	fmt.Printf("Queries where Semantic better:    %d\n", report.Summary.SemanticBetterCount)
	fmt.Printf("Queries where Statistical better: %d\n", report.Summary.StatisticalBetterCount)
	fmt.Printf("Queries tied:                     %d\n", report.Summary.TiedCount)
	fmt.Printf("\nVerdict: %s\n", report.Summary.Verdict)
}

// formatTopHits formats the top N hits as a string
func formatTopHits(hits []string, n int) string {
	if len(hits) == 0 {
		return "(none)"
	}
	if len(hits) <= n {
		return strings.Join(hits, ", ")
	}
	return strings.Join(hits[:n], ", ") + fmt.Sprintf(" (+%d more)", len(hits)-n)
}
