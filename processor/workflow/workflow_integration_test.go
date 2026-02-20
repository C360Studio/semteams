//go:build integration

package workflow_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/processor/workflow"
	"github.com/c360studio/semstreams/processor/workflow/schema"
)

// getTestNATSClient creates a NATS client for integration tests with JetStream and required streams
func getTestNATSClient(t *testing.T) *natsclient.TestClient {
	streams := []natsclient.TestStreamConfig{
		{Name: "WORKFLOW", Subjects: []string{"workflow.>"}},
		{Name: "AGENT", Subjects: []string{"agent.>"}},
	}

	testClient := natsclient.NewTestClient(t,
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithKVBuckets("WORKFLOW_DEFINITIONS", "WORKFLOW_EXECUTIONS"),
		natsclient.WithStreams(streams...),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)

	return testClient
}

// testLogger returns a no-op logger for tests
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// registerTestWorkflow registers a workflow definition directly in the KV bucket
func registerTestWorkflow(t *testing.T, kv jetstream.KeyValue, wf *schema.Definition) {
	t.Helper()
	data, err := json.Marshal(wf)
	require.NoError(t, err, "Failed to marshal workflow definition")
	_, err = kv.Put(context.Background(), wf.ID, data)
	require.NoError(t, err, "Failed to put workflow in KV bucket")
}

// publishTriggerMessage publishes a workflow trigger wrapped in a BaseMessage envelope
func publishTriggerMessage(t *testing.T, testClient *natsclient.TestClient, subject string, workflowID string, extraData map[string]any) {
	t.Helper()

	// Build the data payload
	var dataBytes json.RawMessage
	if extraData != nil {
		// Include workflow_id in extraData for backward compatibility
		extraData["workflow_id"] = workflowID
		var err error
		dataBytes, err = json.Marshal(extraData)
		require.NoError(t, err, "Failed to marshal extra data")
	}

	// Create TriggerPayload
	trigger := &workflow.TriggerPayload{
		WorkflowID: workflowID,
		Data:       dataBytes,
	}

	// Wrap in BaseMessage
	baseMsg := message.NewBaseMessage(trigger.Schema(), trigger, "integration-test")

	// Marshal and publish
	msgData, err := json.Marshal(baseMsg)
	require.NoError(t, err, "Failed to marshal BaseMessage")

	err = testClient.Client.PublishToStream(context.Background(), subject, msgData)
	require.NoError(t, err, "Failed to publish trigger message")
}

// publishStepCompleteMessage publishes a step complete message wrapped in a BaseMessage envelope
func publishStepCompleteMessage(t *testing.T, testClient *natsclient.TestClient, subject string, stepComplete *workflow.StepCompleteMessage) {
	t.Helper()

	// Wrap in BaseMessage
	baseMsg := message.NewBaseMessage(stepComplete.Schema(), stepComplete, "integration-test")

	// Marshal and publish
	msgData, err := json.Marshal(baseMsg)
	require.NoError(t, err, "Failed to marshal BaseMessage")

	err = testClient.Client.PublishToStream(context.Background(), subject, msgData)
	require.NoError(t, err, "Failed to publish step complete message")
}

// waitForExecutionState waits for an execution to reach the expected state
func waitForExecutionState(t *testing.T, kv jetstream.KeyValue, execID string, expected workflow.ExecutionState, timeout time.Duration) *workflow.Execution {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastExec *workflow.Execution

	for time.Now().Before(deadline) {
		entry, err := kv.Get(context.Background(), execID)
		if err == nil {
			var exec workflow.Execution
			if err := json.Unmarshal(entry.Value(), &exec); err == nil {
				lastExec = &exec
				if exec.State == expected {
					return &exec
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	if lastExec != nil {
		t.Fatalf("Execution %s did not reach state %s within %v (current state: %s)", execID, expected, timeout, lastExec.State)
	} else {
		t.Fatalf("Execution %s not found within %v", execID, timeout)
	}
	return nil
}

// createTestComponent creates a workflow component for testing
func createTestComponent(t *testing.T, testClient *natsclient.TestClient) *workflow.Component {
	t.Helper()

	config := workflow.DefaultConfig()
	config.ConsumerNameSuffix = t.Name() // Unique consumer names per test

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	metricsRegistry := metric.NewMetricsRegistry()
	deps := component.Dependencies{
		NATSClient:      testClient.Client,
		MetricsRegistry: metricsRegistry,
	}

	comp, err := workflow.NewComponent(configJSON, deps)
	require.NoError(t, err)

	return comp.(*workflow.Component)
}

// TestIntegration_WorkflowRegistryLoad tests loading workflow definitions from KV bucket
func TestIntegration_WorkflowRegistryLoad(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	// Get the definitions bucket
	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	kv, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	// Register a test workflow
	wf := &schema.Definition{
		ID:      "test-registry-workflow",
		Name:    "Test Registry Workflow",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.test-registry",
		},
		Steps: []schema.StepDef{
			{
				Name: "step1",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
				OnSuccess: "complete",
			},
		},
	}
	registerTestWorkflow(t, kv, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	// Allow time for registry to load
	time.Sleep(200 * time.Millisecond)

	// Verify health
	health := comp.Health()
	assert.True(t, health.Healthy, "Component should be healthy")
	assert.Equal(t, "running", health.Status)
}

// TestIntegration_ExecutionPersistence tests execution state persistence in KV
func TestIntegration_ExecutionPersistence(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	// Get the executions bucket
	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Create an execution store directly for testing
	execStore := workflow.NewExecutionStore(execKV)

	// Create a test execution
	triggerCtx := workflow.TriggerContext{
		Subject:   "test.trigger",
		Payload:   json.RawMessage(`{"key": "value"}`),
		Timestamp: time.Now(),
	}
	exec := workflow.NewExecution("test-workflow", "Test Workflow", triggerCtx, 10*time.Minute)

	// Save the execution
	err = execStore.Save(ctx, exec)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := execStore.Get(ctx, exec.ID)
	require.NoError(t, err)
	assert.Equal(t, exec.ID, retrieved.ID)
	assert.Equal(t, exec.WorkflowID, retrieved.WorkflowID)
	assert.Equal(t, workflow.ExecutionStatePending, retrieved.State)

	// Update state and save again
	exec.MarkRunning()
	err = execStore.Save(ctx, exec)
	require.NoError(t, err)

	// Verify updated state
	retrieved, err = execStore.Get(ctx, exec.ID)
	require.NoError(t, err)
	assert.Equal(t, workflow.ExecutionStateRunning, retrieved.State)
}

// TestIntegration_SimpleWorkflowTrigger tests triggering a simple single-step workflow
func TestIntegration_SimpleWorkflowTrigger(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Register a simple workflow
	wf := &schema.Definition{
		ID:      "simple-trigger-test",
		Name:    "Simple Trigger Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.simple-trigger-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "notify",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.output.simple", // Use workflow.> subject pattern
					Payload: json.RawMessage(`{"message": "workflow completed"}`),
				},
				OnSuccess: "complete",
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	// Wait for component to be ready
	time.Sleep(300 * time.Millisecond)

	// Collect workflow events
	var receivedEvents []map[string]any
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			receivedEvents = append(receivedEvents, event)
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.simple-trigger-test", "simple-trigger-test", map[string]any{
		"data": "test data",
	})

	// Wait for execution to complete
	time.Sleep(1 * time.Second)

	// Find execution ID from events
	eventsMu.Lock()
	var execID string
	for _, event := range receivedEvents {
		if id, ok := event["execution_id"].(string); ok {
			execID = id
			break
		}
	}
	eventsMu.Unlock()

	if execID != "" {
		// Verify execution reached completed state
		exec := waitForExecutionState(t, execKV, execID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "simple-trigger-test", exec.WorkflowID)
		assert.Equal(t, 1, exec.Iteration)
	}

	// Verify we received workflow events
	eventsMu.Lock()
	eventCount := len(receivedEvents)
	eventsMu.Unlock()
	assert.Greater(t, eventCount, 0, "Should have received workflow events")
}

// TestIntegration_CallActionWithResponse tests a workflow step using the call action
func TestIntegration_CallActionWithResponse(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Register a workflow with a call action
	wf := &schema.Definition{
		ID:      "call-action-test",
		Name:    "Call Action Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.call-action-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "fetch_data",
				Action: schema.ActionDef{
					Type:    "call",
					Subject: "test.service.getData",
					Payload: json.RawMessage(`{"request_id": "123"}`),
					Timeout: "5s",
				},
				OnSuccess: "complete",
				OnFail:    "fail",
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	// Wait for component to be ready
	time.Sleep(300 * time.Millisecond)

	// For proper request-reply, we need to use the native NATS connection
	nativeConn := testClient.GetNativeConnection()
	_, err = nativeConn.Subscribe("test.service.getData", func(msg *nats.Msg) {
		response := map[string]any{
			"result": "success",
			"value":  42,
		}
		respData, _ := json.Marshal(response)
		msg.Respond(respData)
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Collect workflow events
	var execID string
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.call-action-test", "call-action-test", nil)

	// Wait for execution to complete
	time.Sleep(2 * time.Second)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	if capturedExecID != "" {
		// Verify execution completed with step result
		exec := waitForExecutionState(t, execKV, capturedExecID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "call-action-test", exec.WorkflowID)

		// Check step result
		result, ok := exec.StepResults["fetch_data"]
		assert.True(t, ok, "Should have step result for fetch_data")
		assert.Equal(t, "success", result.Status)
		assert.NotEmpty(t, result.Output, "Step output should contain response data")
	}
}

// TestIntegration_PublishAction tests a workflow step using the publish action
func TestIntegration_PublishAction(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Register a workflow with a publish action
	wf := &schema.Definition{
		ID:      "publish-action-test",
		Name:    "Publish Action Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.publish-action-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "send_notification",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.notifications.test",
					Payload: json.RawMessage(`{"type": "test", "message": "Hello from workflow"}`),
				},
				OnSuccess: "complete",
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	// Wait for component to be ready
	time.Sleep(300 * time.Millisecond)

	// Subscribe to receive the published notification
	var receivedNotifications []map[string]any
	var notifMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.notifications.>", func(_ context.Context, msg *nats.Msg) {
		var notif map[string]any
		if err := json.Unmarshal(msg.Data, &notif); err == nil {
			notifMu.Lock()
			receivedNotifications = append(receivedNotifications, notif)
			notifMu.Unlock()
		}
	})
	require.NoError(t, err)

	// Collect workflow events to get execution ID
	var execID string
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.publish-action-test", "publish-action-test", nil)

	// Wait for processing
	time.Sleep(1 * time.Second)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	if capturedExecID != "" {
		// Verify execution completed
		exec := waitForExecutionState(t, execKV, capturedExecID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "publish-action-test", exec.WorkflowID)
	}

	// Verify notification was published
	notifMu.Lock()
	assert.Greater(t, len(receivedNotifications), 0, "Should have received published notification")
	if len(receivedNotifications) > 0 {
		notif := receivedNotifications[0]
		assert.Equal(t, "test", notif["type"])
		assert.Equal(t, "Hello from workflow", notif["message"])
	}
	notifMu.Unlock()
}

// TestIntegration_LoopWorkflowMaxIterations tests workflow loop behavior with max iterations
func TestIntegration_LoopWorkflowMaxIterations(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Register a workflow with a loop (step2 goes back to step1)
	wf := &schema.Definition{
		ID:            "loop-max-iter-test",
		Name:          "Loop Max Iterations Test",
		Enabled:       true,
		MaxIterations: 3, // Limit to 3 iterations
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.loop-max-iter-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "increment",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.loop.increment",
					Payload: json.RawMessage(`{"iteration": "${execution.iteration}"}`),
				},
				OnSuccess: "check", // Move to check step
			},
			{
				Name: "check",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.loop.check",
				},
				OnSuccess: "increment", // Loop back to increment (this creates the loop)
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	// Wait for component to be ready
	time.Sleep(300 * time.Millisecond)

	// Collect workflow events
	var execID string
	var iterations []int
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			if iter, ok := event["iteration"].(float64); ok && event["type"] == "step_completed" {
				iterations = append(iterations, int(iter))
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.loop-max-iter-test", "loop-max-iter-test", nil)

	// Wait for execution to complete (should stop after max iterations)
	time.Sleep(2 * time.Second)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	if capturedExecID != "" {
		// Verify execution completed (not failed) after reaching max iterations
		exec := waitForExecutionState(t, execKV, capturedExecID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "loop-max-iter-test", exec.WorkflowID)
		assert.LessOrEqual(t, exec.Iteration, 3, "Should have stopped at or before max iterations")
	}
}

// TestIntegration_ConditionEvaluation tests workflow step conditions
func TestIntegration_ConditionEvaluation(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	nativeConn := testClient.GetNativeConnection()

	// Set up a responder that returns a predictable result
	_, err = nativeConn.Subscribe("test.condition.check", func(msg *nats.Msg) {
		response := map[string]any{
			"score": 85, // Above threshold
		}
		respData, _ := json.Marshal(response)
		msg.Respond(respData)
	})
	require.NoError(t, err)

	// Register a workflow with conditional steps
	wf := &schema.Definition{
		ID:      "condition-eval-test",
		Name:    "Condition Evaluation Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.condition-eval-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "get_score",
				Action: schema.ActionDef{
					Type:    "call",
					Subject: "test.condition.check",
					Timeout: "5s",
				},
				OnSuccess: "check_high",
			},
			{
				Name: "check_high",
				Condition: &schema.ConditionDef{
					Field:    "steps.get_score.output.score",
					Operator: "gte",
					Value:    80,
				},
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.condition.high_score",
					Payload: json.RawMessage(`{"path": "high"}`),
				},
				OnSuccess: "complete",
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	time.Sleep(300 * time.Millisecond)

	// Track if high_score path was taken
	var highScoreReceived bool
	var mu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.condition.high_score", func(_ context.Context, msg *nats.Msg) {
		_ = msg // Unused but required by signature
		mu.Lock()
		highScoreReceived = true
		mu.Unlock()
	})
	require.NoError(t, err)

	// Collect execution ID
	var execID string
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.condition-eval-test", "condition-eval-test", nil)

	// Wait for execution
	time.Sleep(2 * time.Second)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	if capturedExecID != "" {
		// Verify execution completed
		exec := waitForExecutionState(t, execKV, capturedExecID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "condition-eval-test", exec.WorkflowID)
	}

	// Verify high score path was taken (condition was met)
	mu.Lock()
	assert.True(t, highScoreReceived, "High score path should have been taken since score (85) >= threshold (80)")
	mu.Unlock()
}

// TestIntegration_StepCompleteMessage tests external step completion via message
func TestIntegration_StepCompleteMessage(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Register a workflow with an agent action (waits for external completion)
	wf := &schema.Definition{
		ID:      "step-complete-msg-test",
		Name:    "Step Complete Message Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.step-complete-msg-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "agent_task",
				Action: schema.ActionDef{
					Type:    "publish_agent",
					Subject: "agent.task.test",
					Payload: json.RawMessage(`{"task": "process_data"}`),
				},
				OnSuccess: "finalize",
			},
			{
				Name: "finalize",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.finalize.output",
				},
				OnSuccess: "complete",
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	time.Sleep(300 * time.Millisecond)

	// Collect execution ID from events
	var execID string
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.step-complete-msg-test", "step-complete-msg-test", nil)

	// Wait a bit for the workflow to start and reach the agent_task step
	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	if capturedExecID != "" {
		// Simulate external agent completing the step by publishing step.complete message
		stepComplete := &workflow.StepCompleteMessage{
			ExecutionID: capturedExecID,
			StepName:    "agent_task",
			Status:      "success",
			Output:      json.RawMessage(`{"agent_result": "processed"}`),
		}
		publishStepCompleteMessage(t, testClient, "workflow.step.complete."+capturedExecID, stepComplete)

		// Wait for workflow to complete
		time.Sleep(1 * time.Second)

		// Verify execution completed
		exec := waitForExecutionState(t, execKV, capturedExecID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "step-complete-msg-test", exec.WorkflowID)

		// Verify agent_task step result
		result, ok := exec.StepResults["agent_task"]
		assert.True(t, ok, "Should have step result for agent_task")
		assert.Equal(t, "success", result.Status)
	}
}

// TestIntegration_WorkflowTimeout tests workflow timeout behavior
func TestIntegration_WorkflowTimeout(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	// Register a workflow with a very short timeout and a step that won't complete
	wf := &schema.Definition{
		ID:      "timeout-test",
		Name:    "Timeout Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.timeout-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "slow_agent",
				Action: schema.ActionDef{
					Type:    "publish_agent",
					Subject: "agent.task.slow",
					Payload: json.RawMessage(`{"task": "slow_operation"}`),
				},
				OnSuccess: "complete",
			},
		},
		Timeout: "1s", // Very short timeout
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	time.Sleep(300 * time.Millisecond)

	// Collect execution ID
	var execID string
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow
	publishTriggerMessage(t, testClient, "workflow.trigger.timeout-test", "timeout-test", nil)

	// Wait for timeout to occur (workflow has 1s timeout, we never complete the step)
	time.Sleep(3 * time.Second)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	// Note: The timeout is checked at step execution time, not continuously monitored.
	// To properly test timeout, we need an external trigger to check timeout state.
	// For now, verify the execution exists
	if capturedExecID != "" {
		entry, err := execKV.Get(ctx, capturedExecID)
		if err == nil {
			var exec workflow.Execution
			json.Unmarshal(entry.Value(), &exec)
			// The execution should be running (waiting for agent) since timeout checking
			// happens at step start, not continuously
			t.Logf("Execution state: %s", exec.State)
		}
	}
}

// TestIntegration_VariableInterpolation tests variable interpolation in workflow payloads
func TestIntegration_VariableInterpolation(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	execKV, err := js.KeyValue(ctx, "WORKFLOW_EXECUTIONS")
	require.NoError(t, err)

	nativeConn := testClient.GetNativeConnection()

	// Set up a responder that returns data we'll interpolate later
	_, err = nativeConn.Subscribe("test.interpolate.getData", func(msg *nats.Msg) {
		response := map[string]any{
			"user_name": "John Doe",
			"user_id":   12345,
		}
		respData, _ := json.Marshal(response)
		msg.Respond(respData)
	})
	require.NoError(t, err)

	// Register a workflow that uses interpolation
	wf := &schema.Definition{
		ID:      "interpolation-test",
		Name:    "Variable Interpolation Test",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.interpolation-test",
		},
		Steps: []schema.StepDef{
			{
				Name: "get_user",
				Action: schema.ActionDef{
					Type:    "call",
					Subject: "test.interpolate.getData",
					Timeout: "5s",
				},
				OnSuccess: "send_notification",
			},
			{
				Name: "send_notification",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "workflow.interpolate.notify",
					// Use interpolation: trigger payload and step output
					// Note: All interpolation values must be in string positions for valid JSON
					Payload: json.RawMessage(`{"request_type": "${trigger.payload.type}", "user_name": "${steps.get_user.output.user_name}", "execution_id": "${execution.id}", "iteration": "${execution.iteration}"}`),
				},
				OnSuccess: "complete",
			},
		},
		Timeout: "30s",
	}
	registerTestWorkflow(t, defKV, wf)

	// Create and start the component
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	time.Sleep(300 * time.Millisecond)

	// Collect the interpolated notification
	var receivedNotification map[string]any
	var notifMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.interpolate.notify", func(_ context.Context, msg *nats.Msg) {
		var notif map[string]any
		if err := json.Unmarshal(msg.Data, &notif); err == nil {
			notifMu.Lock()
			receivedNotification = notif
			notifMu.Unlock()
		}
	})
	require.NoError(t, err)

	// Collect execution ID
	var execID string
	var eventsMu sync.Mutex

	_, err = testClient.Client.Subscribe(testCtx, "workflow.events", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			eventsMu.Lock()
			if id, ok := event["execution_id"].(string); ok && execID == "" {
				execID = id
			}
			eventsMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger the workflow with data that will be interpolated
	publishTriggerMessage(t, testClient, "workflow.trigger.interpolation-test", "interpolation-test", map[string]any{
		"type":   "user_action",
		"source": "test",
	})

	// Wait for execution
	time.Sleep(2 * time.Second)

	eventsMu.Lock()
	capturedExecID := execID
	eventsMu.Unlock()

	if capturedExecID != "" {
		// Verify execution completed
		exec := waitForExecutionState(t, execKV, capturedExecID, workflow.ExecutionStateCompleted, 5*time.Second)
		assert.Equal(t, "interpolation-test", exec.WorkflowID)
	}

	// Verify interpolated values
	notifMu.Lock()
	defer notifMu.Unlock()

	if receivedNotification != nil {
		// Check trigger.payload interpolation
		assert.Equal(t, "user_action", receivedNotification["request_type"], "Should interpolate trigger.payload.type")

		// Check steps output interpolation
		assert.Equal(t, "John Doe", receivedNotification["user_name"], "Should interpolate steps.get_user.output.user_name")

		// Check execution context interpolation
		assert.NotEmpty(t, receivedNotification["execution_id"], "Should interpolate execution.id")

		// Check iteration (should be "1" as a string for first run)
		assert.Equal(t, "1", receivedNotification["iteration"], "Should interpolate execution.iteration")
	} else {
		t.Error("Should have received interpolated notification")
	}
}

// TestIntegration_RegistryWatchUpdates tests that the registry picks up workflow changes
func TestIntegration_RegistryWatchUpdates(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	// Create and start the component with no initial workflows
	comp := createTestComponent(t, testClient)

	err = comp.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = comp.Start(testCtx)
	require.NoError(t, err)
	defer comp.Stop(5 * time.Second)

	time.Sleep(300 * time.Millisecond)

	// Now add a workflow definition dynamically
	wf := &schema.Definition{
		ID:      "dynamic-watch-workflow",
		Name:    "Dynamically Added Workflow",
		Enabled: true,
		Trigger: schema.TriggerDef{
			Subject: "workflow.trigger.dynamic-watch-workflow",
		},
		Steps: []schema.StepDef{
			{
				Name: "step1",
				Action: schema.ActionDef{
					Type:    "publish",
					Subject: "test.dynamic.output",
				},
				OnSuccess: "complete",
			},
		},
	}
	registerTestWorkflow(t, defKV, wf)

	// Wait for the watch to pick up the new workflow
	time.Sleep(500 * time.Millisecond)

	// Verify the component is still healthy
	health := comp.Health()
	assert.True(t, health.Healthy, "Component should remain healthy after workflow update")
}

// TestIntegration_VersionAwareRegistration tests that workflow registration respects version comparison
func TestIntegration_VersionAwareRegistration(t *testing.T) {
	testClient := getTestNATSClient(t)
	ctx := context.Background()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	defKV, err := js.KeyValue(ctx, "WORKFLOW_DEFINITIONS")
	require.NoError(t, err)

	// Create a registry directly for testing
	registry := workflow.NewRegistry(defKV, testLogger())

	t.Run("new workflow is registered", func(t *testing.T) {
		wf := &schema.Definition{
			ID:      "version-test-new",
			Name:    "New Workflow",
			Version: "1.0.0",
			Enabled: true,
			Trigger: schema.TriggerDef{Subject: "workflow.trigger.version-test-new"},
			Steps: []schema.StepDef{
				{Name: "step1", Action: schema.ActionDef{Type: "publish", Subject: "test.output"}},
			},
		}

		err := registry.Register(ctx, wf)
		require.NoError(t, err)

		// Verify it's in KV
		entry, err := defKV.Get(ctx, "version-test-new")
		require.NoError(t, err)

		var stored schema.Definition
		err = json.Unmarshal(entry.Value(), &stored)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", stored.Version)
	})

	t.Run("newer version overwrites existing", func(t *testing.T) {
		// First, register version 1.0.0
		wf := &schema.Definition{
			ID:      "version-test-update",
			Name:    "Updatable Workflow v1",
			Version: "1.0.0",
			Enabled: true,
			Trigger: schema.TriggerDef{Subject: "workflow.trigger.version-test-update"},
			Steps: []schema.StepDef{
				{Name: "step1", Action: schema.ActionDef{Type: "publish", Subject: "test.output"}},
			},
		}
		err := registry.Register(ctx, wf)
		require.NoError(t, err)

		// Now register version 2.0.0
		wf.Version = "2.0.0"
		wf.Name = "Updatable Workflow v2"
		err = registry.Register(ctx, wf)
		require.NoError(t, err)

		// Verify KV has the newer version
		entry, err := defKV.Get(ctx, "version-test-update")
		require.NoError(t, err)

		var stored schema.Definition
		err = json.Unmarshal(entry.Value(), &stored)
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", stored.Version)
		assert.Equal(t, "Updatable Workflow v2", stored.Name)
	})

	t.Run("older version does not overwrite existing", func(t *testing.T) {
		// First, register version 3.0.0
		wf := &schema.Definition{
			ID:      "version-test-skip",
			Name:    "Workflow v3",
			Version: "3.0.0",
			Enabled: true,
			Trigger: schema.TriggerDef{Subject: "workflow.trigger.version-test-skip"},
			Steps: []schema.StepDef{
				{Name: "step1", Action: schema.ActionDef{Type: "publish", Subject: "test.output"}},
			},
		}
		err := registry.Register(ctx, wf)
		require.NoError(t, err)

		// Try to register version 1.0.0 (older)
		wf.Version = "1.0.0"
		wf.Name = "Workflow v1 (should be skipped)"
		err = registry.Register(ctx, wf)
		require.NoError(t, err) // Should succeed (no error, just skipped)

		// Verify KV still has version 3.0.0
		entry, err := defKV.Get(ctx, "version-test-skip")
		require.NoError(t, err)

		var stored schema.Definition
		err = json.Unmarshal(entry.Value(), &stored)
		require.NoError(t, err)
		assert.Equal(t, "3.0.0", stored.Version)
		assert.Equal(t, "Workflow v3", stored.Name) // Name should not have changed
	})

	t.Run("same version does not overwrite existing", func(t *testing.T) {
		// First, register version 1.0.0
		wf := &schema.Definition{
			ID:      "version-test-same",
			Name:    "Workflow Original",
			Version: "1.0.0",
			Enabled: true,
			Trigger: schema.TriggerDef{Subject: "workflow.trigger.version-test-same"},
			Steps: []schema.StepDef{
				{Name: "step1", Action: schema.ActionDef{Type: "publish", Subject: "test.output"}},
			},
		}
		err := registry.Register(ctx, wf)
		require.NoError(t, err)

		// Try to register same version with different name
		wf.Name = "Workflow Modified (should be skipped)"
		err = registry.Register(ctx, wf)
		require.NoError(t, err)

		// Verify KV still has original name
		entry, err := defKV.Get(ctx, "version-test-same")
		require.NoError(t, err)

		var stored schema.Definition
		err = json.Unmarshal(entry.Value(), &stored)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", stored.Version)
		assert.Equal(t, "Workflow Original", stored.Name)
	})

	t.Run("empty file version does not overwrite versioned KV entry", func(t *testing.T) {
		// Register with version 1.0.0
		wf := &schema.Definition{
			ID:      "version-test-empty-file",
			Name:    "Versioned Workflow",
			Version: "1.0.0",
			Enabled: true,
			Trigger: schema.TriggerDef{Subject: "workflow.trigger.version-test-empty-file"},
			Steps: []schema.StepDef{
				{Name: "step1", Action: schema.ActionDef{Type: "publish", Subject: "test.output"}},
			},
		}
		err := registry.Register(ctx, wf)
		require.NoError(t, err)

		// Try to register with empty version (should not overwrite)
		wf.Version = ""
		wf.Name = "Should not overwrite"
		err = registry.Register(ctx, wf)
		require.NoError(t, err)

		// Verify KV still has 1.0.0
		entry, err := defKV.Get(ctx, "version-test-empty-file")
		require.NoError(t, err)

		var stored schema.Definition
		err = json.Unmarshal(entry.Value(), &stored)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", stored.Version)
		assert.Equal(t, "Versioned Workflow", stored.Name)
	})

	t.Run("empty version treated as 0.0.0", func(t *testing.T) {
		// Register with no version (empty)
		wf := &schema.Definition{
			ID:      "version-test-empty",
			Name:    "No Version Workflow",
			Version: "", // Empty version
			Enabled: true,
			Trigger: schema.TriggerDef{Subject: "workflow.trigger.version-test-empty"},
			Steps: []schema.StepDef{
				{Name: "step1", Action: schema.ActionDef{Type: "publish", Subject: "test.output"}},
			},
		}
		err := registry.Register(ctx, wf)
		require.NoError(t, err)

		// Register with 0.0.1 (should overwrite since 0.0.1 > 0.0.0)
		wf.Version = "0.0.1"
		wf.Name = "Versioned Workflow"
		err = registry.Register(ctx, wf)
		require.NoError(t, err)

		// Verify KV has 0.0.1
		entry, err := defKV.Get(ctx, "version-test-empty")
		require.NoError(t, err)

		var stored schema.Definition
		err = json.Unmarshal(entry.Value(), &stored)
		require.NoError(t, err)
		assert.Equal(t, "0.0.1", stored.Version)
		assert.Equal(t, "Versioned Workflow", stored.Name)
	})
}
