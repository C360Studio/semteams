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
	"github.com/c360studio/semstreams/processor/workflow"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
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

// TestWorkflowID is the ID of the test workflow for agentic integration
const TestWorkflowID = "e2e-agentic-integration-test"

// TestWorkflow is a workflow that tests the full agentic integration path.
// It uses ${...} syntax in conditions to ensure the interpolation fix is working.
var TestWorkflow = wfschema.Definition{
	ID:            TestWorkflowID,
	Name:          "E2E Agentic Integration Test",
	Description:   "Tests workflow -> agent -> workflow completion path",
	Enabled:       true,
	MaxIterations: 1,
	Timeout:       "30s",
	Trigger:       wfschema.TriggerDef{Subject: "workflow.trigger.e2e-agentic"},
	Steps: []wfschema.StepDef{
		{
			Name: "analyze_request",
			Action: wfschema.ActionDef{
				Type:    "publish_agent",
				Subject: "agent.task.e2e-agentic",
				Role:    "general", // Valid roles: architect, editor, general
				Model:   "mock",
				Prompt:  "Analyze the request: ${trigger.payload.content}. Respond with JSON: {\"valid\": true, \"summary\": \"analysis complete\"}",
			},
			OnSuccess: "publish_result",
		},
		{
			Name: "publish_result",
			// Use ${...} syntax to catch the condition wrapper bug
			Condition: &wfschema.ConditionDef{
				Field:    "${steps.analyze_request.output.valid}",
				Operator: "eq",
				Value:    true,
			},
			Action: wfschema.ActionDef{
				Type:    "publish",
				Subject: "workflow.result.e2e-agentic",
				Payload: json.RawMessage(`{"status": "completed", "step": "publish_result"}`),
			},
			OnSuccess: "complete",
		},
	},
	OnComplete: []wfschema.ActionDef{
		{
			Type:    "publish",
			Subject: "workflow.complete.e2e-agentic",
			Payload: json.RawMessage(`{"workflow": "e2e-agentic-integration-test", "status": "success"}`),
		},
	},
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
		description: "Validates agentic components and workflow integration end-to-end",
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
		{"register-workflow", s.registerWorkflow},
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
func (s *Scenario) Teardown(ctx context.Context) error {
	// Clean up test workflow definition
	if s.nats != nil {
		_ = s.nats.DeleteKV(ctx, client.BucketWorkflowDefinitions, TestWorkflowID)
	}
	return nil
}

// verifyComponents checks that agentic components are healthy.
func (s *Scenario) verifyComponents(ctx context.Context, result *scenarios.Result) error {
	components, err := s.obs.GetComponents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get components: %w", err)
	}

	// Check for required agentic components AND workflow processor
	required := []string{"agentic-loop", "agentic-model", "workflow-processor"}
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
		// Workflow component is required for this test
		for _, m := range missing {
			if m == "workflow-processor" {
				return fmt.Errorf("workflow-processor component is required for agentic integration test but not found")
			}
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("Missing agentic components: %v (may not be configured)", missing))
	}

	if len(unhealthy) > 0 {
		return fmt.Errorf("unhealthy components: %v", unhealthy)
	}

	return nil
}

// registerWorkflow registers the test workflow definition
func (s *Scenario) registerWorkflow(ctx context.Context, result *scenarios.Result) error {
	data, err := json.Marshal(TestWorkflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	if err := s.nats.PutKV(ctx, client.BucketWorkflowDefinitions, TestWorkflowID, data); err != nil {
		return fmt.Errorf("failed to register workflow: %w", err)
	}

	result.Details["workflow_id"] = TestWorkflowID
	result.Details["workflow_registered"] = true
	result.Details["condition_uses_wrapper"] = true // ${...} syntax

	// Give workflow processor time to pick up the new definition
	time.Sleep(500 * time.Millisecond)

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

// injectTask triggers the workflow (which will spawn an agent task)
func (s *Scenario) injectTask(ctx context.Context, result *scenarios.Result) error {
	// Trigger the workflow using proper TriggerPayload wrapped in BaseMessage
	// This tests the full integration path: workflow -> agent -> workflow completion
	requestID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())

	// Create TriggerPayload with workflow_id and custom data
	triggerPayload := &workflow.TriggerPayload{
		WorkflowID: TestWorkflowID,
		RequestID:  requestID,
		Data:       json.RawMessage(`{"content": "E2E test request for agentic integration validation"}`),
	}

	// Wrap in BaseMessage envelope (required by workflow processor)
	baseMsg := message.NewBaseMessage(triggerPayload.Schema(), triggerPayload, "e2e-test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal trigger: %w", err)
	}

	result.Details["trigger_request_id"] = requestID
	result.Details["trigger_subject"] = "workflow.trigger.e2e-agentic"

	// Publish workflow trigger
	if err := s.nats.Publish(ctx, "workflow.trigger.e2e-agentic", data); err != nil {
		return fmt.Errorf("failed to publish workflow trigger: %w", err)
	}

	// Also inject a direct task for backwards compatibility testing
	task := agentic.TaskMessage{
		TaskID: fmt.Sprintf("e2e-direct-%d", time.Now().UnixNano()),
		Role:   "general",
		Model:  "mock",
		Prompt: "Analyze the temperature sensor temp-sensor-001. Respond with a brief assessment.",
	}

	taskMsg := message.NewBaseMessage(task.Schema(), &task, "e2e-test")
	taskData, err := json.Marshal(taskMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	result.Details["direct_task_id"] = task.TaskID

	if err := s.nats.Publish(ctx, "agent.task.e2e", taskData); err != nil {
		return fmt.Errorf("failed to publish direct task: %w", err)
	}

	return nil
}

// waitForCompletion waits for BOTH agent loop completion AND workflow completion
func (s *Scenario) waitForCompletion(ctx context.Context, result *scenarios.Result) error {
	timeout := s.config.CompleteTimeout
	deadline := time.Now().Add(timeout)

	var loopsCompleted float64
	var workflowCompleted bool
	var lastExec *client.WorkflowExecution

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Check agent loop completion via metrics
			loops, err := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_loops_completed_total")
			if err == nil && loops > loopsCompleted {
				loopsCompleted = loops
				result.Metrics["loops_completed"] = loopsCompleted
			}

			// Check workflow completion via KV state
			exec, err := s.nats.WaitForWorkflowState(ctx, TestWorkflowID, []string{"completed", "failed"}, 100*time.Millisecond)
			if err == nil && exec != nil {
				lastExec = exec
				if exec.State == "completed" {
					workflowCompleted = true
					result.Metrics["workflow_state"] = exec.State
					result.Details["workflow_execution"] = exec
					result.Details["workflow_steps_completed"] = len(exec.StepResults)
				} else if exec.State == "failed" {
					// Workflow failed - this is an error
					result.Details["workflow_execution"] = exec
					result.Details["workflow_error"] = exec.Error
					return fmt.Errorf("workflow failed: %s (current step: %s)", exec.Error, exec.CurrentName)
				}
			}

			// Success: both agent loops completed AND workflow completed
			if loopsCompleted >= 2 && workflowCompleted {
				result.Details["completion_method"] = "workflow_and_metrics"
				return nil
			}
		}
	}

	// Timeout - provide diagnostic info
	result.Details["timeout_loops_completed"] = loopsCompleted
	result.Details["timeout_workflow_completed"] = workflowCompleted
	if lastExec != nil {
		result.Details["timeout_workflow_state"] = lastExec.State
		result.Details["timeout_workflow_step"] = lastExec.CurrentName
		result.Details["timeout_workflow_error"] = lastExec.Error
	}

	if !workflowCompleted {
		return fmt.Errorf("timeout: workflow did not complete (loops_completed=%v, workflow_state=%v)",
			loopsCompleted, func() string {
				if lastExec != nil {
					return lastExec.State
				}
				return "unknown"
			}())
	}

	return fmt.Errorf("timeout waiting for completion after %v", timeout)
}

// validateResults validates the scenario results - checks full integration path
func (s *Scenario) validateResults(_ context.Context, result *scenarios.Result) error {
	// Validate agent loops completed
	loopsCompleted, ok := result.Metrics["loops_completed"].(float64)
	if !ok || loopsCompleted < 2 {
		return fmt.Errorf("expected at least 2 agent loops (direct + workflow), got %v", loopsCompleted)
	}

	// Validate workflow reached completion state
	workflowState, ok := result.Metrics["workflow_state"].(string)
	if !ok || workflowState != "completed" {
		return fmt.Errorf("workflow did not complete successfully, state: %v", workflowState)
	}

	// Validate workflow steps were executed
	exec, ok := result.Details["workflow_execution"].(*client.WorkflowExecution)
	if !ok || exec == nil {
		return fmt.Errorf("workflow execution details not found")
	}

	// Check that the analyze_request step completed successfully
	analyzeResult, hasAnalyze := exec.StepResults["analyze_request"]
	if !hasAnalyze {
		return fmt.Errorf("analyze_request step not found in results - agent completion may not have been correlated")
	}
	if analyzeResult.Status != "success" {
		return fmt.Errorf("analyze_request step failed: %s (this may indicate outcome value mismatch)", analyzeResult.Error)
	}

	// Check that the publish_result step completed (validates condition evaluation)
	publishResult, hasPublish := exec.StepResults["publish_result"]
	if !hasPublish {
		return fmt.Errorf("publish_result step not found - condition evaluation may have failed (check ${...} wrapper handling)")
	}
	if publishResult.Status != "success" {
		return fmt.Errorf("publish_result step failed: %s", publishResult.Error)
	}

	result.Details["validation_passed"] = true
	result.Details["steps_validated"] = []string{"analyze_request", "publish_result"}

	return nil
}
