// Package workflow provides E2E test scenario for workflow + agentic integration.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/c360studio/semstreams/processor/workflow"
	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/mock"
	"github.com/c360studio/semstreams/test/e2e/scenarios"
)

// Scenario validates workflow orchestration with agentic components.
type Scenario struct {
	name        string
	description string

	// Client URLs
	natsURL string

	// Clients (created during Setup)
	nats    *client.NATSValidationClient
	metrics *client.MetricsClient
	obs     *client.ObservabilityClient

	// Mock server
	mockServer *mock.OpenAIServer
	useMock    bool

	// Configuration
	config *Config
}

// Config holds configuration for the workflow scenario.
type Config struct {
	NATSURL        string        `json:"nats_url"`
	MetricsURL     string        `json:"metrics_url"`
	LLMEndpointURL string        `json:"llm_endpoint_url"`
	TaskTimeout    time.Duration `json:"task_timeout"`
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		NATSURL:        "nats://localhost:34222",
		MetricsURL:     "http://localhost:39090",
		LLMEndpointURL: "", // Empty means use mock
		TaskTimeout:    60 * time.Second,
	}
}

// ContentModerationWorkflow is the test workflow definition.
// It validates: triggers, steps, publish_agent, conditions, loops.
var ContentModerationWorkflow = workflow.Definition{
	ID:            "content-moderation-review",
	Name:          "Content Moderation Review",
	Description:   "E2E test workflow for validating workflow + agentic integration",
	Enabled:       true,
	MaxIterations: 3,
	Timeout:       "60s",
	Trigger:       workflow.TriggerDef{Subject: "workflow.trigger.content-review"},
	Steps: []workflow.StepDef{
		{
			Name: "analyze_content",
			Action: workflow.ActionDef{
				Type:    "publish_agent",
				Subject: "agent.task.content-review",
				Role:    "reviewer",
				Model:   "mock",
				Prompt:  "Analyze the following content for policy violations: ${trigger.payload.content}. Respond with JSON: {\"issues_found\": <count>, \"issues\": [<list>]}",
			},
			OnSuccess: "evaluate_result",
		},
		{
			Name: "evaluate_result",
			Condition: &workflow.ConditionDef{
				Field:    "steps.analyze_content.output.issues_found",
				Operator: "eq",
				Value:    0,
			},
			Action: workflow.ActionDef{
				Type:    "publish",
				Subject: "workflow.content.approved",
				Payload: json.RawMessage(`{"status": "approved"}`),
			},
			OnSuccess: "complete",
			OnFail:    "suggest_fixes",
		},
		{
			Name: "suggest_fixes",
			Action: workflow.ActionDef{
				Type:    "publish_agent",
				Subject: "agent.task.content-fix",
				Role:    "editor",
				Model:   "mock",
				Prompt:  "Suggest fixes for issues: ${steps.analyze_content.output.issues}. Respond with JSON: {\"fixed_content\": \"...\"}",
			},
			OnSuccess: "analyze_content", // Loop back for re-review
		},
	},
	OnComplete: []workflow.ActionDef{
		{
			Type:    "publish",
			Subject: "workflow.content.disposition",
			Payload: json.RawMessage(`{"status": "complete"}`),
		},
	},
}

// NewScenario creates a new workflow scenario.
func NewScenario(obs *client.ObservabilityClient, config *Config) *Scenario {
	if config == nil {
		config = DefaultConfig()
	}

	if envURL := os.Getenv("AGENTIC_LLM_URL"); envURL != "" {
		config.LLMEndpointURL = envURL
	}

	return &Scenario{
		name:        "workflow-agentic",
		description: "Validates workflow orchestration with agentic components (publish_agent, conditions, loops)",
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
		// Configure mock response sequence for multi-turn workflow:
		// 1. First review: issues found
		// 2. Fix suggestion
		// 3. Second review: fewer issues
		// 4. Another fix
		// 5. Third review: clean
		s.mockServer = mock.NewOpenAIServer().
			WithResponseSequence([]string{
				`{"issues_found": 2, "issues": ["profanity", "spam"]}`,
				`{"fixed_content": "cleaned text v1", "changes_made": ["removed profanity"]}`,
				`{"issues_found": 1, "issues": ["spam"]}`,
				`{"fixed_content": "cleaned text v2", "changes_made": ["condensed spam"]}`,
				`{"issues_found": 0, "issues": []}`,
			})

		if err := s.mockServer.Start(":0"); err != nil {
			return fmt.Errorf("failed to start mock server: %w", err)
		}
		s.config.LLMEndpointURL = s.mockServer.URL()
	}

	return nil
}

// Execute runs the workflow scenario.
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

	if s.useMock {
		result.Details["llm_endpoint"] = "mock (built-in)"
		result.Details["mock_url"] = s.config.LLMEndpointURL
	} else {
		result.Details["llm_endpoint"] = s.config.LLMEndpointURL
	}

	stages := []struct {
		name string
		fn   func(context.Context, *scenarios.Result) error
	}{
		{"verify-components", s.verifyComponents},
		{"register-workflow", s.registerWorkflow},
		{"capture-baseline", s.captureBaseline},
		{"trigger-workflow", s.triggerWorkflow},
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

// verifyComponents checks that required components are healthy.
func (s *Scenario) verifyComponents(ctx context.Context, result *scenarios.Result) error {
	components, err := s.obs.GetComponents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get components: %w", err)
	}

	required := []string{"workflow", "agentic-loop", "agentic-model"}
	found := make(map[string]bool)

	for _, comp := range components {
		found[comp.Name] = comp.Enabled && comp.Healthy
	}

	result.Details["components"] = found

	missing := []string{}
	for _, req := range required {
		if healthy, exists := found[req]; !exists || !healthy {
			missing = append(missing, req)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("required components not healthy: %v", missing)
	}

	return nil
}

// registerWorkflow registers the test workflow definition in KV.
func (s *Scenario) registerWorkflow(ctx context.Context, result *scenarios.Result) error {
	data, err := json.Marshal(ContentModerationWorkflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	if err := s.nats.PutKV(ctx, "WORKFLOW_DEFINITIONS", ContentModerationWorkflow.ID, data); err != nil {
		return fmt.Errorf("failed to register workflow: %w", err)
	}

	result.Details["workflow_id"] = ContentModerationWorkflow.ID
	result.Details["workflow_registered"] = true

	return nil
}

// captureBaseline captures metrics baseline.
func (s *Scenario) captureBaseline(ctx context.Context, result *scenarios.Result) error {
	snapshot, err := s.metrics.FetchSnapshot(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not capture baseline: %v", err))
		return nil
	}

	result.Details["baseline_snapshot"] = snapshot
	return nil
}

// triggerWorkflow publishes a trigger message to start the workflow.
func (s *Scenario) triggerWorkflow(ctx context.Context, result *scenarios.Result) error {
	trigger := map[string]any{
		"content":    "This is test content with some issues that need review.",
		"content_id": fmt.Sprintf("test-%d", time.Now().UnixNano()),
	}

	data, err := json.Marshal(trigger)
	if err != nil {
		return fmt.Errorf("failed to marshal trigger: %w", err)
	}

	result.Details["trigger_content_id"] = trigger["content_id"]

	if err := s.nats.Publish(ctx, "workflow.trigger.content-review", data); err != nil {
		return fmt.Errorf("failed to publish trigger: %w", err)
	}

	return nil
}

// waitForCompletion waits for the workflow to complete.
func (s *Scenario) waitForCompletion(ctx context.Context, result *scenarios.Result) error {
	deadline := time.Now().Add(s.config.TaskTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// Check workflow completion via metrics
			completed, err := s.metrics.SumMetricsByName(ctx, "semstreams_workflow_completed_total")
			if err == nil && completed > 0 {
				result.Metrics["workflows_completed"] = completed
				return nil
			}

			// Also check mock server requests as progress indicator
			if s.useMock && s.mockServer != nil {
				reqCount := s.mockServer.RequestCount()
				result.Metrics["mock_requests_current"] = reqCount
			}
		}
	}

	// Timeout - gather diagnostics
	if s.useMock && s.mockServer != nil {
		result.Metrics["mock_requests_final"] = s.mockServer.RequestCount()
		result.Metrics["mock_sequence_index"] = s.mockServer.SequenceIndex()
	}

	return fmt.Errorf("timeout waiting for workflow completion after %v", s.config.TaskTimeout)
}

// validateResults validates the scenario results.
func (s *Scenario) validateResults(_ context.Context, result *scenarios.Result) error {
	if s.useMock && s.mockServer != nil {
		requestCount := s.mockServer.RequestCount()
		result.Metrics["mock_request_count"] = requestCount

		if requestCount == 0 {
			return fmt.Errorf("mock server received no requests - agentic pipeline may not be configured")
		}

		// For the content moderation workflow with issues:
		// - Multiple review/fix cycles expected
		// - Each cycle = 2 requests (review + fix)
		// - Final clean review = 1 request
		// Expect at least 3 requests (review -> fix -> review)
		if requestCount < 3 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Expected at least 3 mock requests for review/fix cycle, got %d", requestCount))
		}

		sequenceIndex := s.mockServer.SequenceIndex()
		result.Metrics["mock_sequence_index"] = sequenceIndex
		result.Details["mock_sequence_exhausted"] = sequenceIndex >= 5
	}

	return nil
}
