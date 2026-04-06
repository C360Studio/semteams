//go:build integration

package agenticmemory_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	agenticmemory "github.com/c360studio/semstreams/processor/agentic-memory"
)

var (
	sharedTestClient *natsclient.TestClient
	sharedNATSClient *natsclient.Client
)

// TestMain sets up shared NATS container for all memory integration tests
func TestMain(m *testing.M) {
	streams := []natsclient.TestStreamConfig{
		{
			Name: "AGENT",
			Subjects: []string{
				"agent.context.compaction.>",
				"agent.context.injected.>",
				"memory.hydrate.request.>",
				"graph.mutation.>",
				"memory.checkpoint.created.>",
			},
		},
	}

	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithKVBuckets("AGENT_MEMORY_CHECKPOINTS", "ENTITY_STATES"),
		natsclient.WithStreams(streams...),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)
	if err != nil {
		panic("Failed to create shared test client: " + err.Error())
	}

	sharedTestClient = testClient
	sharedNATSClient = testClient.Client

	exitCode := m.Run()

	sharedTestClient.Terminate()

	os.Exit(exitCode)
}

func getSharedNATSClient(t *testing.T) *natsclient.Client {
	if sharedNATSClient == nil {
		t.Fatal("Shared NATS client not initialized")
	}
	return sharedNATSClient
}

// waitForConsumerReady waits for the JetStream consumer to be ready
func waitForConsumerReady(t *testing.T, comp component.Discoverable) {
	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.Healthy
	}, 3*time.Second, 50*time.Millisecond, "Consumer should become ready")
}

// TestIntegration_ComponentLifecycle_StartStop tests component lifecycle
func TestIntegration_ComponentLifecycle_StartStop(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "lifecycle-test"
	// Disable hydration to avoid graph client requirements
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok, "Component should implement LifecycleComponent")

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)

	// Verify health shows running
	waitForConsumerReady(t, comp)
	health := comp.Health()
	assert.True(t, health.Healthy, "Component should be healthy after start")
	assert.Equal(t, "running", health.Status)

	// Stop component
	err = lc.Stop(5 * time.Second)
	require.NoError(t, err)

	// Verify health shows stopped
	health = comp.Health()
	assert.False(t, health.Healthy, "Component should be unhealthy after stop")
	assert.Equal(t, "stopped", health.Status)
}

// TestIntegration_ComponentHealth_ReflectsState tests that health status reflects component state
func TestIntegration_ComponentHealth_ReflectsState(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "health-test"
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	// Health before initialize/start
	health := comp.Health()
	assert.False(t, health.Healthy, "Should be unhealthy before start")

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	// Health after start
	waitForConsumerReady(t, comp)
	health = comp.Health()
	assert.True(t, health.Healthy, "Should be healthy after start")
	assert.Equal(t, 0, health.ErrorCount, "Should have no errors initially")
}

// TestIntegration_CompactionComplete_ReceivesEvent tests that the component receives compaction events
func TestIntegration_CompactionComplete_ReceivesEvent(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "compact-complete-test"
	// Disable features that need external clients
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	// Publish compaction_complete event
	event := agenticmemory.ContextEvent{
		Type:        "compaction_complete",
		LoopID:      "loop-001",
		Iteration:   5,
		TokensSaved: 1000,
		Summary:     "Compacted context successfully",
	}

	eventData, err := json.Marshal(event)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "agent.context.compaction.loop-001", eventData)
	require.NoError(t, err)

	// Verify the event was processed (component stays healthy)
	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.Healthy
	}, 3*time.Second, 50*time.Millisecond, "Component should remain healthy after processing event")
}

// TestIntegration_CompactionStarting_ReceivesEvent tests that compaction_starting events are received
func TestIntegration_CompactionStarting_ReceivesEvent(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "compact-starting-test"
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	// Publish compaction_starting event
	event := agenticmemory.ContextEvent{
		Type:        "compaction_starting",
		LoopID:      "loop-002",
		Iteration:   3,
		Utilization: 0.85,
	}

	eventData, err := json.Marshal(event)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "agent.context.compaction.loop-002", eventData)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.Healthy
	}, 3*time.Second, 50*time.Millisecond, "Component should remain healthy")
}

// TestIntegration_HydrateRequest_PreTask tests pre-task hydration requests
func TestIntegration_HydrateRequest_PreTask(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "hydrate-pretask-test"
	// Keep pre-task enabled but no graph client means no actual hydration output
	config.Hydration.PreTask.Enabled = true
	config.Hydration.PostCompaction.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	// Subscribe to injected context output
	receivedContext := make([]agenticmemory.InjectedContextMessage, 0)
	var mu sync.Mutex

	_, err = natsClient.Subscribe(ctx, "agent.context.injected.>", func(_ context.Context, msg *nats.Msg) {
		var ctxMsg agenticmemory.InjectedContextMessage
		if err := json.Unmarshal(msg.Data, &ctxMsg); err == nil {
			mu.Lock()
			receivedContext = append(receivedContext, ctxMsg)
			mu.Unlock()
		}
	})
	require.NoError(t, err)

	waitForConsumerReady(t, comp)

	// Publish pre-task hydration request
	request := agenticmemory.HydrateRequest{
		LoopID:          "loop-pretask-001",
		TaskDescription: "Analyze code for security issues",
		Type:            "pre_task",
	}

	reqData, err := json.Marshal(request)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "memory.hydrate.request.loop-pretask-001", reqData)
	require.NoError(t, err)

	// Verify injected context was published (even if empty due to no graph client)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedContext) > 0
	}, 3*time.Second, 50*time.Millisecond, "Should receive injected context")

	mu.Lock()
	defer mu.Unlock()

	msg := receivedContext[0]
	assert.Equal(t, "loop-pretask-001", msg.LoopID)
	assert.Equal(t, "pre_task", msg.Source)
}

// TestIntegration_HydrateRequest_PreTask_MissingDescription tests that pre-task requests without description are rejected
func TestIntegration_HydrateRequest_PreTask_MissingDescription(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "hydrate-pretask-nodesc-test"
	config.Hydration.PreTask.Enabled = true
	config.Hydration.PostCompaction.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	initialHealth := comp.Health()
	initialErrors := initialHealth.ErrorCount

	// Publish pre-task hydration request WITHOUT task description
	request := agenticmemory.HydrateRequest{
		LoopID:          "loop-pretask-nodesc",
		TaskDescription: "", // Missing!
		Type:            "pre_task",
	}

	reqData, err := json.Marshal(request)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "memory.hydrate.request.loop-pretask-nodesc", reqData)
	require.NoError(t, err)

	// Verify error count increased
	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.ErrorCount > initialErrors
	}, 3*time.Second, 50*time.Millisecond, "Error count should increase for missing task_description")
}

// TestIntegration_HydrateRequest_PostCompaction tests post-compaction hydration requests
func TestIntegration_HydrateRequest_PostCompaction(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "hydrate-postcompact-test"
	config.Hydration.PreTask.Enabled = false
	config.Hydration.PostCompaction.Enabled = true
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	// Subscribe to injected context output
	receivedContext := make([]agenticmemory.InjectedContextMessage, 0)
	var mu sync.Mutex

	_, err = natsClient.Subscribe(ctx, "agent.context.injected.>", func(_ context.Context, msg *nats.Msg) {
		var ctxMsg agenticmemory.InjectedContextMessage
		if err := json.Unmarshal(msg.Data, &ctxMsg); err == nil {
			mu.Lock()
			receivedContext = append(receivedContext, ctxMsg)
			mu.Unlock()
		}
	})
	require.NoError(t, err)

	waitForConsumerReady(t, comp)

	// Publish post-compaction hydration request
	request := agenticmemory.HydrateRequest{
		LoopID: "loop-postcompact-001",
		Type:   "post_compaction",
	}

	reqData, err := json.Marshal(request)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "memory.hydrate.request.loop-postcompact-001", reqData)
	require.NoError(t, err)

	// Verify injected context was published
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedContext) > 0
	}, 3*time.Second, 50*time.Millisecond, "Should receive injected context")

	mu.Lock()
	defer mu.Unlock()

	msg := receivedContext[0]
	assert.Equal(t, "loop-postcompact-001", msg.LoopID)
	assert.Equal(t, "post_compaction", msg.Source)
}

// TestIntegration_InvalidCompactionEvent_IncrementErrors tests that invalid events increment error count
func TestIntegration_InvalidCompactionEvent_IncrementErrors(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "invalid-event-test"
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	// Get initial error count
	initialHealth := comp.Health()
	initialErrors := initialHealth.ErrorCount

	// Publish invalid JSON
	err = natsClient.PublishToStream(ctx, "agent.context.compaction.invalid", []byte("{invalid json"))
	require.NoError(t, err)

	// Verify error count increased
	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.ErrorCount > initialErrors
	}, 3*time.Second, 50*time.Millisecond, "Error count should increase for invalid JSON")
}

// TestIntegration_EmptyLoopID_Rejected tests that events with empty loop_id are rejected
func TestIntegration_EmptyLoopID_Rejected(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "empty-loopid-test"
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	initialHealth := comp.Health()
	initialErrors := initialHealth.ErrorCount

	// Publish event with empty loop_id
	event := agenticmemory.ContextEvent{
		Type:      "compaction_complete",
		LoopID:    "", // Empty!
		Iteration: 1,
	}

	eventData, err := json.Marshal(event)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "agent.context.compaction.empty", eventData)
	require.NoError(t, err)

	// Verify error count increased
	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.ErrorCount > initialErrors
	}, 3*time.Second, 50*time.Millisecond, "Error count should increase for empty loop_id")
}

// TestIntegration_PublishInjectedContext_ToJetStream tests that injected context is published
func TestIntegration_PublishInjectedContext_ToJetStream(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "publish-context-test"
	config.Hydration.PostCompaction.Enabled = true
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	// Subscribe to injected context output
	receivedContext := make([]agenticmemory.InjectedContextMessage, 0)
	var mu sync.Mutex

	_, err = natsClient.Subscribe(ctx, "agent.context.injected.>", func(_ context.Context, msg *nats.Msg) {
		var ctxMsg agenticmemory.InjectedContextMessage
		if err := json.Unmarshal(msg.Data, &ctxMsg); err == nil {
			mu.Lock()
			receivedContext = append(receivedContext, ctxMsg)
			mu.Unlock()
		}
	})
	require.NoError(t, err)

	waitForConsumerReady(t, comp)

	// Publish compaction_complete event to trigger hydration
	event := agenticmemory.ContextEvent{
		Type:        "compaction_complete",
		LoopID:      "loop-publish-001",
		Iteration:   5,
		TokensSaved: 500,
		Summary:     "Test compaction",
	}

	eventData, err := json.Marshal(event)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "agent.context.compaction.loop-publish-001", eventData)
	require.NoError(t, err)

	// Wait for processing and publishing
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedContext) > 0
	}, 3*time.Second, 50*time.Millisecond, "Should receive injected context")

	mu.Lock()
	defer mu.Unlock()

	msg := receivedContext[0]
	assert.Equal(t, "loop-publish-001", msg.LoopID)
	assert.Equal(t, "post_compaction", msg.Source)
	assert.Greater(t, msg.Timestamp, int64(0), "Timestamp should be set")
}

// TestIntegration_MultipleEvents_ProcessedSequentially tests multiple events are processed
func TestIntegration_MultipleEvents_ProcessedSequentially(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "multi-event-test"
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	// Publish multiple events
	loopIDs := []string{"loop-multi-001", "loop-multi-002", "loop-multi-003"}
	for _, loopID := range loopIDs {
		event := agenticmemory.ContextEvent{
			Type:      "compaction_complete",
			LoopID:    loopID,
			Iteration: 1,
		}
		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		err = natsClient.PublishToStream(ctx, "agent.context.compaction."+loopID, eventData)
		require.NoError(t, err)
	}

	// Verify component processed all events and remains healthy
	require.Eventually(t, func() bool {
		health := comp.Health()
		return health.Healthy
	}, 3*time.Second, 50*time.Millisecond, "Component should remain healthy after processing multiple events")
}

// TestIntegration_UnknownEventType_NoError tests that unknown event types don't cause errors
func TestIntegration_UnknownEventType_NoError(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := agenticmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "unknown-type-test"
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := agenticmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = lc.Start(ctx)
	require.NoError(t, err)
	defer lc.Stop(5 * time.Second)

	waitForConsumerReady(t, comp)

	initialHealth := comp.Health()
	initialErrors := initialHealth.ErrorCount

	// Publish event with unknown type
	event := agenticmemory.ContextEvent{
		Type:      "unknown_event_type",
		LoopID:    "loop-unknown-001",
		Iteration: 1,
	}

	eventData, err := json.Marshal(event)
	require.NoError(t, err)

	err = natsClient.PublishToStream(ctx, "agent.context.compaction.unknown", eventData)
	require.NoError(t, err)

	// Wait a bit then verify error count did NOT increase
	// Use Eventually to wait for potential processing, then check
	require.Eventually(t, func() bool {
		health := comp.Health()
		// Event should be processed (healthy) and error count unchanged
		return health.Healthy && health.ErrorCount == initialErrors
	}, 3*time.Second, 50*time.Millisecond, "Unknown event types should not increment errors")
}
