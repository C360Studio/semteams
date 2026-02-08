// Package agentic provides the agentic E2E test scenario.
package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/scenarios"
)

// Scenario validates the agentic components (loop, model, tools) work together.
type Scenario struct {
	name        string
	description string

	// Client URLs (clients created during Setup)
	natsURL string

	// Clients (created during Setup)
	nats    *client.NATSValidationClient
	metrics *client.MetricsClient
	obs     *client.ObservabilityClient

	// useMock indicates Docker compose provides mock-llm
	useMock bool

	// Configuration
	config *Config
}

// Config holds configuration for the agentic scenario.
type Config struct {
	// NATS URL for publishing tasks
	NATSURL string `json:"nats_url"`

	// Metrics URL for checking completion
	MetricsURL string `json:"metrics_url"`

	// LLM endpoint URL (default: start mock server)
	LLMEndpointURL string `json:"llm_endpoint_url"`

	// Timeouts
	TaskTimeout     time.Duration `json:"task_timeout"`
	CompleteTimeout time.Duration `json:"complete_timeout"`

	// Expected results
	MinTrajectorySteps int `json:"min_trajectory_steps"`
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		NATSURL:            "nats://localhost:34222",
		MetricsURL:         "http://localhost:39090",
		LLMEndpointURL:     "", // Empty means use mock
		TaskTimeout:        30 * time.Second,
		CompleteTimeout:    60 * time.Second,
		MinTrajectorySteps: 1,
	}
}

// NewScenario creates a new agentic scenario.
func NewScenario(
	obs *client.ObservabilityClient,
	config *Config,
) *Scenario {
	if config == nil {
		config = DefaultConfig()
	}

	// Check for environment override
	if envURL := os.Getenv("AGENTIC_LLM_URL"); envURL != "" {
		config.LLMEndpointURL = envURL
	}

	return &Scenario{
		name:        "agentic",
		description: "Validates agentic components (loop, model, tools) work together end-to-end",
		natsURL:     config.NATSURL,
		obs:         obs,
		config:      config,
		useMock:     config.LLMEndpointURL == "",
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

	// Docker compose provides mock-llm at http://mock-llm:8080 (within Docker network)
	// and http://localhost:38180 (from host). The semstreams container uses the Docker-internal
	// URL, so we don't need to start a mock server here.
	if s.useMock {
		s.config.LLMEndpointURL = "http://localhost:38180" // For reference in results
	}

	return nil
}

// Execute runs the agentic scenario.
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

	// Store mock info
	if s.useMock {
		result.Details["llm_endpoint"] = "mock (built-in)"
		result.Details["mock_url"] = s.config.LLMEndpointURL
	} else {
		result.Details["llm_endpoint"] = s.config.LLMEndpointURL
	}

	// Execute stages
	stages := []struct {
		name string
		fn   func(context.Context, *scenarios.Result) error
	}{
		{"verify-components", s.verifyComponents},
		{"capture-baseline", s.captureBaseline},
		{"inject-task", s.injectTask},
		{"wait-for-completion", s.waitForCompletion},
		{"validate-results", s.validateResults},
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
	// Docker compose manages the mock-llm lifecycle, nothing to clean up here
	return nil
}

// verifyComponents checks that agentic components are healthy.
func (s *Scenario) verifyComponents(ctx context.Context, result *scenarios.Result) error {
	components, err := s.obs.GetComponents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get components: %w", err)
	}

	// Check for required agentic components
	required := []string{"agentic-loop", "agentic-model", "agentic-tools"}
	found := make(map[string]bool)

	for _, comp := range components {
		found[comp.Name] = comp.Enabled && comp.Healthy
	}

	missing := []string{}
	unhealthy := []string{}

	for _, req := range required {
		healthy, exists := found[req]
		if !exists {
			missing = append(missing, req)
		} else if !healthy {
			unhealthy = append(unhealthy, req)
		}
	}

	result.Details["agentic_components"] = found

	if len(missing) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Missing agentic components: %v (may not be configured)", missing))
		// Don't fail - components may not be in this flow config
	}

	if len(unhealthy) > 0 {
		return fmt.Errorf("unhealthy agentic components: %v", unhealthy)
	}

	return nil
}

// captureBaseline captures metrics baseline before task injection.
func (s *Scenario) captureBaseline(ctx context.Context, result *scenarios.Result) error {
	snapshot, err := s.metrics.FetchSnapshot(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not capture metrics baseline: %v", err))
		return nil // Non-fatal
	}

	result.Details["baseline_snapshot"] = snapshot
	return nil
}

// injectTask publishes a task message to trigger an agentic loop.
func (s *Scenario) injectTask(ctx context.Context, result *scenarios.Result) error {
	task := agentic.TaskMessage{
		TaskID: fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
		Role:   "general",
		Model:  "mock", // Should match the configured endpoint name
		Prompt: "Analyze the temperature sensor temp-sensor-001. Use the query_entity tool to retrieve sensor data, then provide an assessment.",
	}

	// Wrap task in BaseMessage envelope (required by agentic-loop)
	baseMsg := message.NewBaseMessage(task.Schema(), &task, "e2e-test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	result.Details["task_id"] = task.TaskID
	result.Details["task_role"] = task.Role
	result.Details["task_model"] = task.Model

	// Publish to agent.task subject
	if err := s.nats.Publish(ctx, "agent.task.e2e", data); err != nil {
		return fmt.Errorf("failed to publish task: %w", err)
	}

	return nil
}

// waitForCompletion waits for the agent loop to complete.
func (s *Scenario) waitForCompletion(ctx context.Context, result *scenarios.Result) error {
	taskID, ok := result.Details["task_id"].(string)
	if !ok {
		return fmt.Errorf("task_id not found in result details")
	}

	// Poll for loop completion via KV bucket or metrics
	// For now, just wait a bit and check metrics
	timeout := s.config.CompleteTimeout
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Check if loop completed via metrics
			loopsCompleted, err := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_loops_completed_total")
			if err == nil && loopsCompleted > 0 {
				result.Metrics["loops_completed"] = loopsCompleted
				result.Details["completion_method"] = "metrics"
				return nil
			}
		}
	}

	result.Details["task_id_checked"] = taskID
	return fmt.Errorf("timeout waiting for agent loop completion after %v", timeout)
}

// validateResults validates the scenario results.
func (s *Scenario) validateResults(_ context.Context, result *scenarios.Result) error {
	// The Docker compose mock-llm handles LLM requests.
	// Validation relies on metrics since we can't directly query the Docker mock server.
	// Check that loops were completed
	loopsCompleted, ok := result.Metrics["loops_completed"].(float64)
	if !ok || loopsCompleted == 0 {
		result.Warnings = append(result.Warnings, "No completed loops found in metrics")
	}

	return nil
}
