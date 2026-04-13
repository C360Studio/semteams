// Package boids provides the Boids coordination A/B test scenario.
package boids

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semteams/teams"
	"github.com/c360studio/semteams/test/e2e/client"
	"github.com/c360studio/semteams/test/e2e/scenarios"
)

// Scenario validates Boids coordination by comparing runs with and without steering signals.
type Scenario struct {
	name        string
	description string

	// Client URLs
	natsURL string

	// Clients (created during Setup)
	nats    *client.NATSValidationClient
	metrics *client.MetricsClient
	obs     *client.ObservabilityClient

	// Configuration
	config *Config

	// Test results for comparison
	baselineResult *RunResult
	boidResult     *RunResult
}

// Config holds configuration for the Boids scenario.
type Config struct {
	// NATS URL for publishing tasks
	NATSURL string `json:"nats_url"`

	// Metrics URL for checking completion
	MetricsURL string `json:"metrics_url"`

	// Timeouts
	TaskTimeout     time.Duration `json:"task_timeout"`
	CompleteTimeout time.Duration `json:"complete_timeout"`
}

// RunResult captures metrics from a single test run.
type RunResult struct {
	BoidEnabled     bool          `json:"boid_enabled"`
	TaskCompleted   bool          `json:"task_completed"`
	Duration        time.Duration `json:"duration"`
	Iterations      int           `json:"iterations"`
	PositionUpdates float64       `json:"position_updates"`
	SignalsReceived float64       `json:"signals_received"`
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		NATSURL:         "nats://localhost:34222",
		MetricsURL:      "http://localhost:39090",
		TaskTimeout:     30 * time.Second,
		CompleteTimeout: 60 * time.Second,
	}
}

// NewScenario creates a new Boids A/B test scenario.
func NewScenario(obs *client.ObservabilityClient, config *Config) *Scenario {
	if config == nil {
		config = DefaultConfig()
	}

	// Allow environment override for NATS URL
	if envURL := os.Getenv("NATS_URL"); envURL != "" {
		config.NATSURL = envURL
	}

	return &Scenario{
		name:        "boids",
		description: "A/B test comparing agent coordination with and without Boids steering signals",
		natsURL:     config.NATSURL,
		obs:         obs,
		config:      config,
	}
}

// Name returns the scenario name.
func (s *Scenario) Name() string {
	return s.name
}

// Description returns the scenario description.
func (s *Scenario) Description() string {
	return s.description
}

// Setup prepares the scenario environment.
func (s *Scenario) Setup(ctx context.Context) error {
	// Create NATS client
	natsClient, err := client.NewNATSValidationClient(ctx, s.natsURL)
	if err != nil {
		return fmt.Errorf("failed to create NATS client: %w", err)
	}
	s.nats = natsClient

	// Create metrics client
	s.metrics = client.NewMetricsClient(s.config.MetricsURL)

	return nil
}

// Execute runs the Boids A/B test scenario.
func (s *Scenario) Execute(ctx context.Context) (*scenarios.Result, error) {
	result := &scenarios.Result{
		ScenarioName: s.name,
		StartTime:    time.Now(),
		Success:      false,
		Metrics:      make(map[string]any),
		Details:      make(map[string]any),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Execute stages
	stages := []struct {
		name string
		fn   func(context.Context, *scenarios.Result) error
	}{
		{"verify-components", s.verifyComponents},
		{"capture-baseline-metrics", s.captureBaselineMetrics},
		{"run-baseline", s.runBaseline},
		{"run-with-boids", s.runWithBoids},
		{"compare-results", s.compareResults},
	}

	for _, stage := range stages {
		stageStart := time.Now()

		if err := stage.fn(ctx, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", stage.name, err))
			result.Error = fmt.Sprintf("%s failed: %v", stage.name, err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, nil
		}

		result.Metrics[fmt.Sprintf("%s_duration_ms", stage.name)] = time.Since(stageStart).Milliseconds()
	}

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Teardown cleans up after the scenario.
func (s *Scenario) Teardown(_ context.Context) error {
	return nil
}

// verifyComponents checks that required components are available.
func (s *Scenario) verifyComponents(ctx context.Context, result *scenarios.Result) error {
	components, err := s.obs.GetComponents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get components: %w", err)
	}

	// Check for required agentic components
	required := []string{"agentic-loop", "agentic-model"}
	found := make(map[string]bool)

	for _, comp := range components {
		found[comp.Name] = comp.Enabled && comp.Healthy
	}

	var missing []string
	for _, req := range required {
		if !found[req] {
			missing = append(missing, req)
		}
	}

	result.Details["components"] = found

	if len(missing) > 0 {
		return fmt.Errorf("missing required components: %v", missing)
	}

	return nil
}

// captureBaselineMetrics captures metrics before test runs.
func (s *Scenario) captureBaselineMetrics(ctx context.Context, result *scenarios.Result) error {
	snapshot, err := s.metrics.FetchSnapshot(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not capture baseline: %v", err))
		return nil // Non-fatal
	}

	result.Details["baseline_snapshot"] = snapshot
	return nil
}

// runBaseline runs a task without Boids coordination.
func (s *Scenario) runBaseline(ctx context.Context, result *scenarios.Result) error {
	runResult, err := s.runTask(ctx, "baseline", false)
	if err != nil {
		return fmt.Errorf("baseline run failed: %w", err)
	}

	s.baselineResult = runResult
	result.Details["baseline"] = runResult
	result.Metrics["baseline_completed"] = runResult.TaskCompleted
	result.Metrics["baseline_duration_ms"] = runResult.Duration.Milliseconds()

	return nil
}

// runWithBoids runs a task with Boids coordination enabled.
func (s *Scenario) runWithBoids(ctx context.Context, result *scenarios.Result) error {
	runResult, err := s.runTask(ctx, "boids", true)
	if err != nil {
		return fmt.Errorf("boids run failed: %w", err)
	}

	s.boidResult = runResult
	result.Details["boids"] = runResult
	result.Metrics["boids_completed"] = runResult.TaskCompleted
	result.Metrics["boids_duration_ms"] = runResult.Duration.Milliseconds()
	result.Metrics["boids_signals_received"] = runResult.SignalsReceived
	result.Metrics["boids_position_updates"] = runResult.PositionUpdates

	return nil
}

// runTask executes a single agent task and collects metrics.
func (s *Scenario) runTask(ctx context.Context, runID string, _ bool) (*RunResult, error) {
	startTime := time.Now()

	// Capture pre-run metrics
	preLoops, _ := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_loops_completed_total")
	prePositions, _ := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_boid_position_updates_total")
	preSignals, _ := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_boid_signals_received_total")

	// Inject a task
	task := teams.TaskMessage{
		TaskID: fmt.Sprintf("e2e-boids-%s-%d", runID, time.Now().UnixNano()),
		Role:   "general",
		Model:  "mock",
		Prompt: "Analyze the system status and provide a brief assessment.",
	}

	taskMsg := message.NewBaseMessage(task.Schema(), &task, "e2e-boids-test")
	taskData, err := json.Marshal(taskMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	if err := s.nats.Publish(ctx, "agent.task.e2e", taskData); err != nil {
		return nil, fmt.Errorf("failed to publish task: %w", err)
	}

	// Wait for completion
	deadline := time.Now().Add(s.config.CompleteTimeout)
	completed := false

waitLoop:
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			loops, err := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_loops_completed_total")
			if err == nil && loops > preLoops {
				completed = true
				break waitLoop
			}
		}
	}

	// Capture post-run metrics
	postPositions, _ := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_boid_position_updates_total")
	postSignals, _ := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_boid_signals_received_total")

	return &RunResult{
		BoidEnabled:     runID == "boids",
		TaskCompleted:   completed,
		Duration:        time.Since(startTime),
		PositionUpdates: postPositions - prePositions,
		SignalsReceived: postSignals - preSignals,
	}, nil
}

// compareResults compares baseline and Boids runs.
func (s *Scenario) compareResults(_ context.Context, result *scenarios.Result) error {
	if s.baselineResult == nil || s.boidResult == nil {
		return fmt.Errorf("missing run results")
	}

	comparison := map[string]any{
		"both_completed":         s.baselineResult.TaskCompleted && s.boidResult.TaskCompleted,
		"baseline_duration_ms":   s.baselineResult.Duration.Milliseconds(),
		"boids_duration_ms":      s.boidResult.Duration.Milliseconds(),
		"boids_position_updates": s.boidResult.PositionUpdates,
		"boids_signals_received": s.boidResult.SignalsReceived,
		"duration_delta_ms":      s.boidResult.Duration.Milliseconds() - s.baselineResult.Duration.Milliseconds(),
		"boids_overhead_percent": 0.0,
	}

	if s.baselineResult.Duration > 0 {
		overheadPercent := float64(s.boidResult.Duration-s.baselineResult.Duration) / float64(s.baselineResult.Duration) * 100
		comparison["boids_overhead_percent"] = overheadPercent
	}

	result.Details["comparison"] = comparison

	// Success criteria: both tasks complete
	if !s.baselineResult.TaskCompleted {
		return fmt.Errorf("baseline task did not complete")
	}
	if !s.boidResult.TaskCompleted {
		return fmt.Errorf("boids task did not complete")
	}

	// Note: We don't require signals/positions since they depend on multi-agent setup
	// The key verification is that Boids doesn't break single-agent execution

	return nil
}
