// Package client provides HTTP clients for SemStreams E2E tests
package client

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// FlowTracer correlates metrics and message logs to trace data flow through the system
type FlowTracer struct {
	metrics *MetricsClient
	logger  *MessageLoggerClient
}

// NewFlowTracer creates a new flow tracer with metrics and message logger clients
func NewFlowTracer(metrics *MetricsClient, logger *MessageLoggerClient) *FlowTracer {
	return &FlowTracer{
		metrics: metrics,
		logger:  logger,
	}
}

// FlowSnapshot captures the state of metrics and messages at a point in time
type FlowSnapshot struct {
	Timestamp       time.Time        `json:"timestamp"`
	MetricsBaseline *MetricsBaseline `json:"metrics_baseline"`
	MessageCount    int              `json:"message_count"`
	LastMessageID   string           `json:"last_message_id,omitempty"`
}

// FlowExpectation defines what we expect to see in a flow
type FlowExpectation struct {
	InputSubject     string        // Expected input subject pattern (e.g., "input.udp")
	ProcessingStages []string      // Expected processing stages (e.g., ["process.rule", "process.graph"])
	MinMessages      int           // Minimum number of messages expected
	MaxLatencyMs     int           // Maximum acceptable p99 latency in ms
	Timeout          time.Duration // How long to wait for flow completion
}

// FlowResult contains the results of flow validation
type FlowResult struct {
	Valid        bool                    `json:"valid"`
	Messages     int                     `json:"messages"`
	AvgLatency   time.Duration           `json:"avg_latency"`
	P99Latency   time.Duration           `json:"p99_latency,omitempty"`
	StageMetrics map[string]StageMetrics `json:"stage_metrics,omitempty"`
	Errors       []string                `json:"errors,omitempty"`
	Warnings     []string                `json:"warnings,omitempty"`
}

// StageMetrics tracks metrics for a specific processing stage
type StageMetrics struct {
	Stage        string             `json:"stage"`
	MessagesIn   int                `json:"messages_in"`
	MessagesOut  int                `json:"messages_out"`
	Errors       int                `json:"errors"`
	AvgLatency   time.Duration      `json:"avg_latency"`
	MetricDeltas map[string]float64 `json:"metric_deltas,omitempty"`
}

// CaptureFlowSnapshot captures the current state of metrics and messages
func (t *FlowTracer) CaptureFlowSnapshot(ctx context.Context) (*FlowSnapshot, error) {
	snapshot := &FlowSnapshot{
		Timestamp: time.Now(),
	}

	// Capture metrics baseline
	baseline, err := t.metrics.CaptureBaseline(ctx)
	if err != nil {
		return nil, fmt.Errorf("capturing metrics baseline: %w", err)
	}
	snapshot.MetricsBaseline = baseline

	// Capture message count
	stats, err := t.logger.GetStats(ctx)
	if err != nil {
		// Message logger might not be available, that's okay
		snapshot.MessageCount = 0
	} else {
		snapshot.MessageCount = int(stats.TotalMessages)
	}

	// Get last message ID if available
	entries, err := t.logger.GetEntries(ctx, 1, "")
	if err == nil && len(entries) > 0 {
		snapshot.LastMessageID = entries[0].MessageID
	}

	return snapshot, nil
}

// ValidateFlow validates that data flowed through expected stages
func (t *FlowTracer) ValidateFlow(ctx context.Context, preSnapshot *FlowSnapshot, expected FlowExpectation) (*FlowResult, error) {
	result := &FlowResult{
		Valid:        true,
		StageMetrics: make(map[string]StageMetrics),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Set default timeout
	if expected.Timeout == 0 {
		expected.Timeout = 30 * time.Second
	}

	// Wait for processing to complete by checking metrics
	if preSnapshot != nil && preSnapshot.MetricsBaseline != nil {
		// Wait for messages to be processed
		if err := t.waitForProcessing(ctx, preSnapshot.MetricsBaseline, expected); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Processing did not complete: %v", err))
			return result, nil
		}
	}

	// Compare current metrics to baseline
	if preSnapshot != nil && preSnapshot.MetricsBaseline != nil {
		diff, err := t.metrics.CompareToBaseline(ctx, preSnapshot.MetricsBaseline)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not compare metrics: %v", err))
		} else {
			// Analyze each processing stage
			for _, stage := range expected.ProcessingStages {
				stageMetrics := t.analyzeStageMetrics(stage, diff)
				result.StageMetrics[stage] = stageMetrics
			}
		}
	}

	// Get message entries for the expected input subject
	entries, err := t.logger.GetEntries(ctx, 1000, expected.InputSubject+"*")
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not fetch message entries: %v", err))
	} else {
		result.Messages = len(entries)

		// Calculate latency from messages
		if len(entries) > 0 {
			latencies := calculateLatencies(entries)
			if len(latencies) > 0 {
				result.AvgLatency = averageDuration(latencies)
				result.P99Latency = percentileDuration(latencies, 99)
			}
		}
	}

	// Validate expectations
	if result.Messages < expected.MinMessages {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("Expected at least %d messages, got %d", expected.MinMessages, result.Messages))
	}

	if expected.MaxLatencyMs > 0 && result.P99Latency > time.Duration(expected.MaxLatencyMs)*time.Millisecond {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("P99 latency %v exceeds max %dms", result.P99Latency, expected.MaxLatencyMs))
	}

	return result, nil
}

// waitForProcessing waits for expected processing to complete based on metrics
func (t *FlowTracer) waitForProcessing(ctx context.Context, baseline *MetricsBaseline, expected FlowExpectation) error {
	// Determine which metric to wait on based on processing stages
	var metricToWatch string
	var expectedDelta float64

	// Common metrics to watch for different stages
	stageMetrics := map[string]string{
		"process.rule":  "semstreams_rule_evaluations_total",
		"process.graph": "semstreams_datamanager_entities_updated_total",
		"input.udp":     "semstreams_udp_packets_received_total",
	}

	// Find the last stage metric
	for _, stage := range expected.ProcessingStages {
		if metric, ok := stageMetrics[stage]; ok {
			metricToWatch = metric
		}
	}

	// If no specific metric found, wait for rule evaluations as default
	if metricToWatch == "" {
		metricToWatch = "semstreams_rule_evaluations_total"
	}

	// Expect at least MinMessages to be processed
	expectedDelta = float64(expected.MinMessages)
	if expectedDelta <= 0 {
		expectedDelta = 1
	}

	baselineValue := baseline.Metrics[metricToWatch]

	// Wait for metric to increase
	opts := WaitOpts{
		Timeout:      expected.Timeout,
		PollInterval: 100 * time.Millisecond,
		Comparator:   ">=",
	}

	return t.metrics.WaitForMetricDelta(ctx, metricToWatch, baselineValue, expectedDelta, opts)
}

// analyzeStageMetrics extracts metrics for a specific processing stage
func (t *FlowTracer) analyzeStageMetrics(stage string, diff *MetricsDiff) StageMetrics {
	sm := StageMetrics{
		Stage:        stage,
		MetricDeltas: make(map[string]float64),
	}

	// Map stage names to metric prefixes
	prefixMap := map[string][]string{
		"process.rule":  {"semstreams_rule_"},
		"process.graph": {"semstreams_datamanager_", "indexengine_"},
		"input.udp":     {"semstreams_udp_"},
	}

	prefixes, ok := prefixMap[stage]
	if !ok {
		return sm
	}

	// Extract relevant metric deltas
	for metricName, delta := range diff.Deltas {
		for _, prefix := range prefixes {
			if len(metricName) >= len(prefix) && metricName[:len(prefix)] == prefix {
				sm.MetricDeltas[metricName] = delta

				// Extract specific values
				switch {
				case contains(metricName, "total") && contains(metricName, "processed"):
					sm.MessagesOut = int(delta)
				case contains(metricName, "total") && contains(metricName, "received"):
					sm.MessagesIn = int(delta)
				case contains(metricName, "errors") || contains(metricName, "failed"):
					sm.Errors += int(delta)
				}
			}
		}
	}

	return sm
}

// WaitForFlowCompletion waits for a complete data flow from input to output
func (t *FlowTracer) WaitForFlowCompletion(ctx context.Context, expected FlowExpectation) error {
	// Capture baseline
	baseline, err := t.CaptureFlowSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("capturing baseline: %w", err)
	}

	// Wait for expected messages in the logger
	timeout := expected.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for flow completion")
			}

			// Check metrics progression
			diff, err := t.metrics.CompareToBaseline(ctx, baseline.MetricsBaseline)
			if err != nil {
				continue
			}

			// Check if we've processed enough
			processed := 0.0
			for _, stage := range expected.ProcessingStages {
				stageMetrics := t.analyzeStageMetrics(stage, diff)
				if stageMetrics.MessagesOut > 0 {
					processed += float64(stageMetrics.MessagesOut)
				}
			}

			if processed >= float64(expected.MinMessages) {
				return nil
			}
		}
	}
}

// Helper functions

func calculateLatencies(entries []MessageEntry) []time.Duration {
	if len(entries) < 2 {
		return nil
	}

	// Group entries by message ID to calculate per-message latency
	messageTimestamps := make(map[string][]time.Time)
	for _, entry := range entries {
		if entry.MessageID != "" {
			messageTimestamps[entry.MessageID] = append(messageTimestamps[entry.MessageID], entry.Timestamp)
		}
	}

	var latencies []time.Duration
	for _, timestamps := range messageTimestamps {
		if len(timestamps) >= 2 {
			// Sort timestamps
			sort.Slice(timestamps, func(i, j int) bool {
				return timestamps[i].Before(timestamps[j])
			})
			latency := timestamps[len(timestamps)-1].Sub(timestamps[0])
			latencies = append(latencies, latency)
		}
	}

	return latencies
}

func averageDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}

func percentileDuration(durations []time.Duration, percentile int) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate index
	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
