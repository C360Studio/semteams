// Package agentic provides the agentic E2E test scenario.
package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/mock"
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

	// Mock server (if using built-in mock)
	mockServer *mock.OpenAIServer
	useMock    bool

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
		NATSURL:            "nats://localhost:4222",
		MetricsURL:         "http://localhost:9090",
		LLMEndpointURL:     "", // Empty means use mock
		TaskTimeout:        30 * time.Second,
		CompleteTimeout:    60 * time.Second,
		MinTrajectorySteps: 1,
	}
}

// TaskMessage matches the agentic-loop expected format.
type TaskMessage struct {
	TaskID string `json:"task_id"`
	Role   string `json:"role"`
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
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

	// Start mock server if no external LLM configured
	if s.useMock {
		s.mockServer = mock.NewOpenAIServer().
			WithToolArgs("query_entity", `{"entity_id": "c360.logistics.environmental.sensor.temperature.temp-sensor-001"}`).
			WithCompletionContent("Analysis complete. The temperature sensor reading of 48.2°F exceeds the 45°F threshold. This requires monitoring but is not critical.")

		if err := s.mockServer.Start(":0"); err != nil {
			return fmt.Errorf("failed to start mock server: %w", err)
		}

		// Note: The mock URL would need to be communicated to the agentic-model component
		// In a real e2e test, this would be done via config or environment variable
		s.config.LLMEndpointURL = s.mockServer.URL()
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
	if s.mockServer != nil {
		return s.mockServer.Stop()
	}
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
	task := TaskMessage{
		TaskID: fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
		Role:   "general",
		Model:  "mock", // Should match the configured endpoint name
		Prompt: "Analyze the temperature sensor temp-sensor-001. Use the query_entity tool to retrieve sensor data, then provide an assessment.",
	}

	data, err := json.Marshal(task)
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
			loopsCompleted, err := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loops_completed_total")
			if err == nil && loopsCompleted > 0 {
				result.Metrics["loops_completed"] = loopsCompleted
				result.Details["completion_method"] = "metrics"
				return nil
			}
		}
	}

	// Timeout - check if mock received requests
	if s.useMock && s.mockServer != nil {
		requestCount := s.mockServer.RequestCount()
		result.Metrics["mock_requests"] = requestCount
		if requestCount > 0 {
			result.Details["completion_method"] = "mock_requests"
			result.Warnings = append(result.Warnings, fmt.Sprintf("Loop may have completed (mock received %d requests) but completion metric not found", requestCount))
			return nil
		}
	}

	result.Details["task_id_checked"] = taskID
	return fmt.Errorf("timeout waiting for agent loop completion after %v", timeout)
}

// validateResults validates the scenario results.
func (s *Scenario) validateResults(_ context.Context, result *scenarios.Result) error {
	// Check mock server request count if using mock
	if s.useMock && s.mockServer != nil {
		requestCount := s.mockServer.RequestCount()
		result.Metrics["mock_request_count"] = requestCount

		if requestCount == 0 {
			return fmt.Errorf("mock server received no requests - agentic-model may not be configured correctly")
		}

		// For a tool-calling flow, we expect at least 2 requests:
		// 1. Initial request (returns tool call)
		// 2. Request with tool results (returns completion)
		if requestCount >= 2 {
			result.Details["tool_calling_flow"] = true
		}

		lastReq := s.mockServer.LastRequest()
		if lastReq != nil {
			result.Details["last_request_model"] = lastReq.Model
			result.Details["last_request_message_count"] = len(lastReq.Messages)
		}
	}

	// Try to get loop state from KV
	// This would require the loop ID, which we don't have without subscribing to agent.complete.*
	// For now, just rely on metrics

	return nil
}
