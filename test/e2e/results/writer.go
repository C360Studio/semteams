// Package results provides structured result writing for E2E test scenarios
package results

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/scenarios"
)

// TestRun represents a complete E2E test run
type TestRun struct {
	ID          string                `json:"id"`
	Timestamp   time.Time             `json:"timestamp"`
	Duration    time.Duration         `json:"duration_ns"`
	DurationStr string                `json:"duration"`
	Config      TestRunConfig         `json:"config"`
	Scenarios   []scenarios.Result    `json:"scenarios"`
	Metrics     *client.MetricsReport `json:"metrics,omitempty"`
	Summary     Summary               `json:"summary"`
	Environment map[string]string     `json:"environment,omitempty"`
}

// TestRunConfig captures the configuration for a test run
type TestRunConfig struct {
	Variant    string   `json:"variant"` // "structural", "statistical", or "semantic"
	MLEnabled  bool     `json:"ml_enabled"`
	Scenarios  []string `json:"scenarios"`
	BaseURL    string   `json:"base_url"`
	MetricsURL string   `json:"metrics_url"`
}

// Summary provides high-level statistics for the test run
type Summary struct {
	TotalScenarios  int     `json:"total_scenarios"`
	PassedScenarios int     `json:"passed_scenarios"`
	FailedScenarios int     `json:"failed_scenarios"`
	SuccessRate     float64 `json:"success_rate"`
	TotalErrors     int     `json:"total_errors"`
	TotalWarnings   int     `json:"total_warnings"`
	AllPassed       bool    `json:"all_passed"`
}

// Comparison represents a comparison between two test runs
type Comparison struct {
	Timestamp    time.Time         `json:"timestamp"`
	BaselineID   string            `json:"baseline_id"`
	CurrentID    string            `json:"current_id"`
	BaselineDate time.Time         `json:"baseline_date"`
	CurrentDate  time.Time         `json:"current_date"`
	Diffs        []ScenarioDiff    `json:"diffs"`
	MetricsDiff  map[string]Diff   `json:"metrics_diff,omitempty"`
	Overall      ComparisonSummary `json:"overall"`
}

// ScenarioDiff represents the difference between scenario results
type ScenarioDiff struct {
	ScenarioName     string          `json:"scenario_name"`
	BaselineSuccess  bool            `json:"baseline_success"`
	CurrentSuccess   bool            `json:"current_success"`
	StatusChanged    bool            `json:"status_changed"`
	DurationChange   time.Duration   `json:"duration_change_ns"`
	DurationChangeMs int64           `json:"duration_change_ms"`
	MetricsDiff      map[string]Diff `json:"metrics_diff,omitempty"`
}

// Diff represents a numeric difference
type Diff struct {
	Baseline  float64 `json:"baseline"`
	Current   float64 `json:"current"`
	Absolute  float64 `json:"absolute"`
	Percent   float64 `json:"percent"`
	Improved  bool    `json:"improved"`
	Regressed bool    `json:"regressed"`
}

// ComparisonSummary provides high-level comparison statistics
type ComparisonSummary struct {
	StatusChanges    int  `json:"status_changes"`
	Improvements     int  `json:"improvements"`
	Regressions      int  `json:"regressions"`
	MetricsImproved  int  `json:"metrics_improved"`
	MetricsRegressed int  `json:"metrics_regressed"`
	OverallImproved  bool `json:"overall_improved"`
}

// Writer handles persisting test results to disk
type Writer struct {
	outputDir string
}

// NewWriter creates a new results writer
func NewWriter(outputDir string) *Writer {
	return &Writer{outputDir: outputDir}
}

// WriteRun writes a complete test run to disk
func (w *Writer) WriteRun(run *TestRun) (string, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("e2e-results-%s-%s.json",
		run.Config.Variant,
		run.Timestamp.Format("20060102-150405"))
	filepath := filepath.Join(w.outputDir, filename)

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling results: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", fmt.Errorf("writing results file: %w", err)
	}

	return filepath, nil
}

// WriteLatest writes results and also creates/updates a "latest" symlink
func (w *Writer) WriteLatest(run *TestRun) (string, error) {
	filepath, err := w.WriteRun(run)
	if err != nil {
		return "", err
	}

	// Create/update latest symlink (best effort - may fail on Windows)
	latestLink := filepath + "-latest.json"
	_ = os.Remove(latestLink) // Remove existing link if present
	_ = os.Symlink(filepath, latestLink)

	return filepath, nil
}

// WriteComparison writes a comparison report to disk
func (w *Writer) WriteComparison(comparison *Comparison) (string, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	filename := fmt.Sprintf("e2e-comparison-%s.json",
		comparison.Timestamp.Format("20060102-150405"))
	filepath := filepath.Join(w.outputDir, filename)

	data, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling comparison: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", fmt.Errorf("writing comparison file: %w", err)
	}

	return filepath, nil
}

// LoadRun loads a test run from disk
func (w *Writer) LoadRun(filepath string) (*TestRun, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("reading results file: %w", err)
	}

	var run TestRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("unmarshaling results: %w", err)
	}

	return &run, nil
}

// CreateTestRun creates a new TestRun with computed summary
func CreateTestRun(
	config TestRunConfig,
	scenarioResults []scenarios.Result,
	metrics *client.MetricsReport,
	duration time.Duration,
) *TestRun {
	run := &TestRun{
		ID:          generateRunID(),
		Timestamp:   time.Now(),
		Duration:    duration,
		DurationStr: duration.String(),
		Config:      config,
		Scenarios:   scenarioResults,
		Metrics:     metrics,
	}

	// Compute summary
	run.Summary = computeSummary(scenarioResults)

	return run
}

// computeSummary calculates summary statistics from scenario results
func computeSummary(results []scenarios.Result) Summary {
	summary := Summary{
		TotalScenarios: len(results),
	}

	for _, r := range results {
		if r.Success {
			summary.PassedScenarios++
		} else {
			summary.FailedScenarios++
		}
		summary.TotalErrors += len(r.Errors)
		summary.TotalWarnings += len(r.Warnings)
	}

	if summary.TotalScenarios > 0 {
		summary.SuccessRate = float64(summary.PassedScenarios) / float64(summary.TotalScenarios)
	}

	summary.AllPassed = summary.PassedScenarios == summary.TotalScenarios

	return summary
}

// generateRunID creates a unique run identifier
func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

// Compare compares two test runs and produces a comparison report
func Compare(baseline, current *TestRun) *Comparison {
	comparison := &Comparison{
		Timestamp:    time.Now(),
		BaselineID:   baseline.ID,
		CurrentID:    current.ID,
		BaselineDate: baseline.Timestamp,
		CurrentDate:  current.Timestamp,
		Diffs:        make([]ScenarioDiff, 0),
		MetricsDiff:  make(map[string]Diff),
	}

	// Create map of baseline scenarios by name
	baselineMap := make(map[string]scenarios.Result)
	for _, r := range baseline.Scenarios {
		baselineMap[r.ScenarioName] = r
	}

	// Compare each current scenario
	for _, current := range current.Scenarios {
		baseline, hasBaseline := baselineMap[current.ScenarioName]

		diff := ScenarioDiff{
			ScenarioName:   current.ScenarioName,
			CurrentSuccess: current.Success,
		}

		if hasBaseline {
			diff.BaselineSuccess = baseline.Success
			diff.StatusChanged = baseline.Success != current.Success
			diff.DurationChange = current.Duration - baseline.Duration
			diff.DurationChangeMs = diff.DurationChange.Milliseconds()

			// Compare metrics if available
			if baseline.Metrics != nil && current.Metrics != nil {
				diff.MetricsDiff = compareMetricMaps(
					baseline.Metrics,
					current.Metrics,
				)
			}

			if diff.StatusChanged {
				comparison.Overall.StatusChanges++
				if current.Success && !baseline.Success {
					comparison.Overall.Improvements++
				} else if !current.Success && baseline.Success {
					comparison.Overall.Regressions++
				}
			}
		}

		comparison.Diffs = append(comparison.Diffs, diff)
	}

	// Compare overall metrics if available
	if baseline.Metrics != nil && current.Metrics != nil {
		comparison.MetricsDiff = compareMetricReports(
			baseline.Metrics,
			current.Metrics,
		)

		for _, d := range comparison.MetricsDiff {
			if d.Improved {
				comparison.Overall.MetricsImproved++
			}
			if d.Regressed {
				comparison.Overall.MetricsRegressed++
			}
		}
	}

	// Determine overall improvement
	comparison.Overall.OverallImproved = comparison.Overall.Improvements > comparison.Overall.Regressions &&
		comparison.Overall.MetricsImproved >= comparison.Overall.MetricsRegressed

	return comparison
}

// compareMetricMaps compares two metric maps
func compareMetricMaps(baseline, current map[string]any) map[string]Diff {
	diffs := make(map[string]Diff)

	// Get all unique keys
	keys := make(map[string]bool)
	for k := range baseline {
		keys[k] = true
	}
	for k := range current {
		keys[k] = true
	}

	for k := range keys {
		baseVal := toFloat64(baseline[k])
		currVal := toFloat64(current[k])

		if baseVal == 0 && currVal == 0 {
			continue
		}

		diff := Diff{
			Baseline: baseVal,
			Current:  currVal,
			Absolute: currVal - baseVal,
		}

		if baseVal != 0 {
			diff.Percent = ((currVal - baseVal) / baseVal) * 100
		}

		// For most metrics, higher is better (more processed, more hits)
		// For error counts and latencies, lower is better
		isErrorMetric := isNegativeMetric(k)
		if isErrorMetric {
			diff.Improved = currVal < baseVal
			diff.Regressed = currVal > baseVal
		} else {
			diff.Improved = currVal > baseVal
			diff.Regressed = currVal < baseVal
		}

		diffs[k] = diff
	}

	return diffs
}

// compareMetricReports compares two MetricsReports
func compareMetricReports(baseline, current *client.MetricsReport) map[string]Diff {
	diffs := make(map[string]Diff)

	// Compare counters
	for k, currVal := range current.Counters {
		baseVal := baseline.Counters[k]
		diff := createDiff(k, baseVal, currVal)
		diffs["counter:"+k] = diff
	}

	// Compare gauges
	for k, currVal := range current.Gauges {
		baseVal := baseline.Gauges[k]
		diff := createDiff(k, baseVal, currVal)
		diffs["gauge:"+k] = diff
	}

	return diffs
}

// createDiff creates a Diff struct for two values
func createDiff(key string, baseline, current float64) Diff {
	diff := Diff{
		Baseline: baseline,
		Current:  current,
		Absolute: current - baseline,
	}

	if baseline != 0 {
		diff.Percent = ((current - baseline) / baseline) * 100
	}

	isError := isNegativeMetric(key)
	if isError {
		diff.Improved = current < baseline
		diff.Regressed = current > baseline
	} else {
		diff.Improved = current > baseline
		diff.Regressed = current < baseline
	}

	return diff
}

// isNegativeMetric returns true for metrics where lower is better
func isNegativeMetric(name string) bool {
	negativePatterns := []string{
		"error", "fail", "miss", "drop", "reject", "timeout", "latency", "duration",
	}
	lower := strings.ToLower(name)
	for _, pattern := range negativePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// toFloat64 converts an interface value to float64
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		slog.Debug("unexpected type in toFloat64",
			"type", fmt.Sprintf("%T", v),
			"value", v)
		return 0
	}
}

// ListRuns returns all result files in the output directory
func (w *Writer) ListRuns() ([]string, error) {
	entries, err := os.ReadDir(w.outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading output directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" &&
			strings.Contains(entry.Name(), "e2e-results") {
			files = append(files, filepath.Join(w.outputDir, entry.Name()))
		}
	}

	// Sort by name (which includes timestamp)
	sort.Strings(files)

	return files, nil
}

// GetLatestRun returns the most recent test run
func (w *Writer) GetLatestRun() (*TestRun, error) {
	files, err := w.ListRuns()
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no test runs found")
	}

	// Files are sorted, last one is most recent
	return w.LoadRun(files[len(files)-1])
}
