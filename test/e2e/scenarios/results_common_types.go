// Package scenarios provides common result types for E2E tests
package scenarios

import "time"

// ComponentResults contains component health results.
type ComponentResults struct {
	// ExpectedCount is the number of expected components
	ExpectedCount int `json:"expected_count"`

	// FoundCount is the number of components found healthy
	FoundCount int `json:"found_count"`

	// Components lists individual component status
	Components map[string]bool `json:"components,omitempty"`
}

// OutputResults contains output processor verification results.
type OutputResults struct {
	// ExpectedCount is the number of expected outputs
	ExpectedCount int `json:"expected_count"`

	// FoundCount is the number of outputs found
	FoundCount int `json:"found_count"`

	// Outputs lists individual output status
	Outputs map[string]bool `json:"outputs,omitempty"`
}

// TimingResults contains timing information for each stage.
type TimingResults struct {
	// TotalDurationMs is the total test duration
	TotalDurationMs int64 `json:"total_duration_ms"`

	// StageDurations maps stage name to duration in ms
	StageDurations map[string]int64 `json:"stage_durations"`
}

// TestMetadata contains test run metadata.
type TestMetadata struct {
	// Variant that was tested
	Variant string `json:"variant"`

	// StartedAt is when the test started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the test completed
	CompletedAt time.Time `json:"completed_at"`

	// Success indicates if the test passed
	Success bool `json:"success"`

	// ErrorCount is the number of errors
	ErrorCount int `json:"error_count"`

	// WarningCount is the number of warnings
	WarningCount int `json:"warning_count"`

	// Version information
	Version string `json:"version,omitempty"`
}
