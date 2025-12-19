// Package main provides comparison analysis for Core vs ML search results
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

// ComparisonReport represents the full Core vs ML comparison report
type ComparisonReport struct {
	CoreVariant   string            `json:"core_variant"`
	MLVariant     string            `json:"ml_variant"`
	CoreTimestamp time.Time         `json:"core_timestamp"`
	MLTimestamp   time.Time         `json:"ml_timestamp"`
	Queries       []QueryComparison `json:"queries"`
	Summary       ComparisonSummary `json:"summary"`
}

// QueryComparison represents comparison results for a single query
type QueryComparison struct {
	Query        string   `json:"query"`
	CoreHits     []string `json:"core_hits"`
	MLHits       []string `json:"ml_hits"`
	HitOverlap   float64  `json:"hit_overlap"` // Jaccard similarity (intersection/union)
	ScoreCorr    float64  `json:"score_corr"`  // Pearson correlation of shared hit scores
	CoreAvgScore float64  `json:"core_avg_score"`
	MLAvgScore   float64  `json:"ml_avg_score"`
	Insight      string   `json:"insight"` // "ML finds more results", "Similar results", etc.
}

// ComparisonSummary summarizes the overall comparison
type ComparisonSummary struct {
	AvgHitOverlap   float64 `json:"avg_hit_overlap"`   // Average Jaccard across queries
	AvgScoreCorr    float64 `json:"avg_score_corr"`    // Average correlation
	MLBetterCount   int     `json:"ml_better_count"`   // Queries where ML found more relevant results
	CoreBetterCount int     `json:"core_better_count"` // Queries where Core found more relevant results
	TiedCount       int     `json:"tied_count"`        // Queries where results were similar
	Verdict         string  `json:"verdict"`           // "ML provides semantic lift" / "Marginal difference"
}

// analyzeComparison generates a comparison report from Core and ML comparison files
func analyzeComparison(outputDir string) (*ComparisonReport, error) {
	// Find latest core and ml comparison files
	coreFile, err := findLatestComparison(outputDir, "core")
	if err != nil {
		return nil, fmt.Errorf("failed to find core comparison: %w", err)
	}

	mlFile, err := findLatestComparison(outputDir, "ml")
	if err != nil {
		return nil, fmt.Errorf("failed to find ml comparison: %w", err)
	}

	// Load comparison data
	coreData, err := loadComparisonData(coreFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load core data: %w", err)
	}

	mlData, err := loadComparisonData(mlFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load ml data: %w", err)
	}

	// Generate comparison report
	return generateComparisonReport(coreData, mlData), nil
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

// generateComparisonReport creates a comparison report from Core and ML data
func generateComparisonReport(coreData, mlData *scenarios.ComparisonData) *ComparisonReport {
	report := &ComparisonReport{
		CoreVariant:   coreData.Variant,
		MLVariant:     mlData.Variant,
		CoreTimestamp: coreData.Timestamp,
		MLTimestamp:   mlData.Timestamp,
		Queries:       []QueryComparison{},
	}

	// Get all unique queries
	querySet := make(map[string]bool)
	for query := range coreData.SearchResults {
		querySet[query] = true
	}
	for query := range mlData.SearchResults {
		querySet[query] = true
	}

	// Compare each query
	var totalJaccard, totalCorr float64
	var corrCount int
	mlBetter, coreBetter, tied := 0, 0, 0

	for query := range querySet {
		coreResult := coreData.SearchResults[query]
		mlResult := mlData.SearchResults[query]

		qc := QueryComparison{
			Query:    query,
			CoreHits: coreResult.Hits,
			MLHits:   mlResult.Hits,
		}

		// Calculate Jaccard similarity
		qc.HitOverlap = jaccard(coreResult.Hits, mlResult.Hits)
		totalJaccard += qc.HitOverlap

		// Calculate average scores
		qc.CoreAvgScore = avgScore(coreResult.Scores)
		qc.MLAvgScore = avgScore(mlResult.Scores)

		// Calculate score correlation for shared hits
		coreScoreMap := buildScoreMap(coreResult.Hits, coreResult.Scores)
		mlScoreMap := buildScoreMap(mlResult.Hits, mlResult.Scores)
		qc.ScoreCorr = scoreCorrelation(coreScoreMap, mlScoreMap)
		if !math.IsNaN(qc.ScoreCorr) {
			totalCorr += qc.ScoreCorr
			corrCount++
		}

		// Determine insight
		qc.Insight = determineInsight(coreResult, mlResult, qc.HitOverlap)

		// Track which is better
		if len(mlResult.Hits) > len(coreResult.Hits) && qc.MLAvgScore >= qc.CoreAvgScore {
			mlBetter++
		} else if len(coreResult.Hits) > len(mlResult.Hits) && qc.CoreAvgScore > qc.MLAvgScore {
			coreBetter++
		} else {
			tied++
		}

		report.Queries = append(report.Queries, qc)
	}

	// Calculate summary
	queryCount := len(querySet)
	report.Summary = ComparisonSummary{
		AvgHitOverlap:   totalJaccard / float64(queryCount),
		MLBetterCount:   mlBetter,
		CoreBetterCount: coreBetter,
		TiedCount:       tied,
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
func determineInsight(coreResult, mlResult scenarios.SearchQueryResult, overlap float64) string {
	coreCnt := len(coreResult.Hits)
	mlCnt := len(mlResult.Hits)

	if coreCnt == 0 && mlCnt == 0 {
		return "No results from either variant"
	}
	if coreCnt == 0 {
		return "ML found results where Core found none"
	}
	if mlCnt == 0 {
		return "Core found results where ML found none"
	}
	if overlap > 0.8 {
		return "Very similar results"
	}
	if overlap > 0.5 {
		return "Moderate overlap in results"
	}
	if mlCnt > coreCnt {
		return "ML finds more results"
	}
	if coreCnt > mlCnt {
		return "Core finds more results"
	}
	return "Different but equal-sized result sets"
}

// determineVerdict generates the overall comparison verdict
func determineVerdict(summary ComparisonSummary) string {
	if summary.MLBetterCount > summary.CoreBetterCount+summary.TiedCount {
		return "ML provides significant semantic lift"
	}
	if summary.MLBetterCount > summary.CoreBetterCount {
		return "ML provides moderate semantic improvement"
	}
	if summary.CoreBetterCount > summary.MLBetterCount {
		return "Core performs better for this dataset"
	}
	if summary.AvgHitOverlap > 0.7 {
		return "Marginal difference - results highly similar"
	}
	return "Mixed results - no clear winner"
}

// printAnalysisReport prints the comparison report to stdout
func printAnalysisReport(report *ComparisonReport) {
	fmt.Println("\n=== Core vs ML Search Comparison Report ===")
	fmt.Printf("Core timestamp: %s\n", report.CoreTimestamp.Format(time.RFC3339))
	fmt.Printf("ML timestamp:   %s\n", report.MLTimestamp.Format(time.RFC3339))

	fmt.Println("\n--- Query-by-Query Comparison ---")
	for _, qc := range report.Queries {
		fmt.Printf("\nQuery: %q\n", qc.Query)
		fmt.Printf("  Core hits: %d, avg score: %.3f\n", len(qc.CoreHits), qc.CoreAvgScore)
		fmt.Printf("  ML hits:   %d, avg score: %.3f\n", len(qc.MLHits), qc.MLAvgScore)
		fmt.Printf("  Hit overlap (Jaccard): %.2f\n", qc.HitOverlap)
		if !math.IsNaN(qc.ScoreCorr) {
			fmt.Printf("  Score correlation: %.2f\n", qc.ScoreCorr)
		}
		fmt.Printf("  Insight: %s\n", qc.Insight)

		// Show hit details if different
		if len(qc.CoreHits) > 0 || len(qc.MLHits) > 0 {
			fmt.Printf("  Core top 3: %s\n", formatTopHits(qc.CoreHits, 3))
			fmt.Printf("  ML top 3:   %s\n", formatTopHits(qc.MLHits, 3))
		}
	}

	fmt.Println("\n--- Summary ---")
	fmt.Printf("Average hit overlap (Jaccard): %.2f\n", report.Summary.AvgHitOverlap)
	if !math.IsNaN(report.Summary.AvgScoreCorr) {
		fmt.Printf("Average score correlation:     %.2f\n", report.Summary.AvgScoreCorr)
	}
	fmt.Printf("Queries where ML better:   %d\n", report.Summary.MLBetterCount)
	fmt.Printf("Queries where Core better: %d\n", report.Summary.CoreBetterCount)
	fmt.Printf("Queries tied:              %d\n", report.Summary.TiedCount)
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
