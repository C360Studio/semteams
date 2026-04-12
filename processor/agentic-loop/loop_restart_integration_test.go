//go:build integration

package agenticloop_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

// newLoopTestConfig returns a Config for restart integration tests.
// Each test gets a unique ConsumerNameSuffix to avoid conflicts with shared NATS.
func newLoopTestConfig(suffix string) agenticloop.Config {
	return agenticloop.Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:       "tasks",
					Type:       "jetstream",
					Subject:    "agent.task.*",
					StreamName: "AGENT",
					Required:   true,
				},
				{
					Name:       "responses",
					Type:       "jetstream",
					Subject:    "agent.response.>",
					StreamName: "AGENT",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:       "requests",
					Type:       "jetstream",
					Subject:    "agent.request.*",
					StreamName: "AGENT",
				},
				{
					Name:       "complete",
					Type:       "jetstream",
					Subject:    "agent.complete.*",
					StreamName: "AGENT",
				},
			},
		},
		MaxIterations:      10,
		Timeout:            "60s",
		StreamName:         "AGENT",
		ConsumerNameSuffix: suffix,
		LoopsBucket:        "AGENT_LOOPS",
	}
}

// startLoopComponent creates, initializes, and starts a loop component.
//
// The caller must pass a context that outlives the test body. The component
// stores the context on internal consumer subscriptions (see
// component.go:setupConsumer → ConsumeStreamWithConfig), so if the caller
// passes a ctx that gets canceled before the test finishes, the JetStream
// consumer shuts down and the component silently stops processing tasks.
//
// This was a real bug in this helper prior to 2026-04-11: the helper created
// its own `ctx, cancel := context.WithTimeout(...)` with a deferred cancel,
// which fired the moment the helper returned — before the test body published
// any tasks. Both restart tests failed their "Should receive model request"
// assertion because the consumer had already been torn down.
func startLoopComponent(t *testing.T, ctx context.Context, config agenticloop.Config) component.LifecycleComponent {
	t.Helper()

	natsClient := getSharedNATSClient(t)
	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := agenticloop.NewComponent(rawConfig, component.Dependencies{NATSClient: natsClient})
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)

	err = lc.Initialize()
	require.NoError(t, err)

	err = lc.Start(ctx)
	require.NoError(t, err)

	return lc
}

// readLoopFromKV reads a LoopEntity directly from the AGENT_LOOPS KV bucket.
func readLoopFromKV(t *testing.T, loopID string) (*agentic.LoopEntity, bool) {
	t.Helper()

	natsClient := getSharedNATSClient(t)
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucket, err := js.KeyValue(ctx, "AGENT_LOOPS")
	require.NoError(t, err)

	entry, err := bucket.Get(ctx, loopID)
	if err != nil {
		return nil, false
	}

	var entity agentic.LoopEntity
	err = json.Unmarshal(entry.Value(), &entity)
	require.NoError(t, err)

	return &entity, true
}

// TestIntegration_LoopKVStateSurvivesRestart proves that loop state persisted to
// AGENT_LOOPS KV survives a component restart. It also documents the current gap:
// after restart, the new component's LoopManager does NOT reload from KV.
func TestIntegration_LoopKVStateSurvivesRestart(t *testing.T) {
	// ctx must live for the whole test body — the loop component's JetStream
	// consumer uses this context for its subscription lifetime.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	natsClient := getSharedNATSClient(t)

	config := newLoopTestConfig("restart-kv-test")
	lc := startLoopComponent(t, ctx, config)

	time.Sleep(200 * time.Millisecond)

	// Subscribe to model requests so we know the loop is active
	var requestMu sync.Mutex
	receivedRequests := make([]agentic.AgentRequest, 0)

	_, err := natsClient.Subscribe(ctx, "agent.request.>", func(_ context.Context, msg *nats.Msg) {
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(msg.Data, &baseMsg); err == nil {
			if req, ok := baseMsg.Payload().(*agentic.AgentRequest); ok {
				requestMu.Lock()
				receivedRequests = append(receivedRequests, *req)
				requestMu.Unlock()
			}
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Inject a task to create a loop
	task := &agentic.TaskMessage{
		LoopID: "restart-test-001",
		TaskID: "task-restart-001",
		Role:   "general",
		Model:  "test-model",
		Prompt: "Test restart scenario",
	}
	publishTaskMessage(t, natsClient, "agent.task.restart", task)

	// Wait for the model request to confirm loop is active
	require.Eventually(t, func() bool {
		requestMu.Lock()
		defer requestMu.Unlock()
		return len(receivedRequests) > 0
	}, 5*time.Second, 50*time.Millisecond, "Should receive model request")

	// Loop is now active and persisted to KV — verify before stopping
	entity, found := readLoopFromKV(t, "restart-test-001")
	require.True(t, found, "Loop entity should be persisted in KV before stop")
	assert.Equal(t, "restart-test-001", entity.ID)
	assert.NotEmpty(t, entity.TaskID)

	// Stop the component
	err = lc.Stop(5 * time.Second)
	require.NoError(t, err)

	// KV state should still be there after stop
	entityAfterStop, found := readLoopFromKV(t, "restart-test-001")
	require.True(t, found, "Loop entity should survive component stop")
	assert.Equal(t, entity.ID, entityAfterStop.ID)
	assert.Equal(t, entity.State, entityAfterStop.State, "State should be preserved in KV")
	assert.Equal(t, entity.Iterations, entityAfterStop.Iterations, "Iterations should be preserved")
	assert.False(t, entityAfterStop.StartedAt.IsZero(), "StartedAt should be preserved")

	// Start a NEW component instance — simulating a restart. Uses the same
	// ctx as the first instance since we're still in the same test body.
	lc2 := startLoopComponent(t, ctx, newLoopTestConfig("restart-kv-test-2"))
	defer lc2.Stop(5 * time.Second)

	// The KV entry should still exist
	entityAfterRestart, found := readLoopFromKV(t, "restart-test-001")
	require.True(t, found, "Loop entity should survive component restart")
	assert.Equal(t, entity.State, entityAfterRestart.State, "KV state should be unchanged after restart")

	// DOCUMENTS THE GAP: The new component does NOT reload loops from KV.
	// The loop is orphaned — it exists in KV but the new LoopManager doesn't know about it.
	// This is the exact semspec restart bug: in-flight state is lost on restart.
}

// TestIntegration_TerminalStateSurvivesRestart proves that a completed loop's
// terminal state in KV survives a component restart.
func TestIntegration_TerminalStateSurvivesRestart(t *testing.T) {
	// ctx must live for the whole test body — see startLoopComponent doc for why.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	natsClient := getSharedNATSClient(t)

	config := newLoopTestConfig("terminal-restart-test")
	lc := startLoopComponent(t, ctx, config)

	time.Sleep(200 * time.Millisecond)

	// Collect model requests
	var requestMu sync.Mutex
	receivedRequests := make([]agentic.AgentRequest, 0)

	_, err := natsClient.Subscribe(ctx, "agent.request.>", func(_ context.Context, msg *nats.Msg) {
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(msg.Data, &baseMsg); err == nil {
			if req, ok := baseMsg.Payload().(*agentic.AgentRequest); ok {
				requestMu.Lock()
				receivedRequests = append(receivedRequests, *req)
				requestMu.Unlock()
			}
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Inject a task
	task := &agentic.TaskMessage{
		LoopID: "terminal-test-001",
		TaskID: "task-terminal-001",
		Role:   "general",
		Model:  "test-model",
		Prompt: "Test terminal state",
	}
	publishTaskMessage(t, natsClient, "agent.task.terminal", task)

	// Wait for model request
	require.Eventually(t, func() bool {
		requestMu.Lock()
		defer requestMu.Unlock()
		return len(receivedRequests) > 0
	}, 5*time.Second, 50*time.Millisecond, "Should receive model request")

	// Send completion response to drive the loop to terminal state
	requestMu.Lock()
	requestID := receivedRequests[0].RequestID
	requestMu.Unlock()

	response := &agentic.AgentResponse{
		RequestID: requestID,
		Status:    "complete",
		Message: agentic.ChatMessage{
			Role:    "assistant",
			Content: "Task completed successfully",
		},
		TokenUsage: agentic.TokenUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
		},
	}
	publishResponseMessage(t, natsClient, "agent.response."+requestID, response)

	// Wait for KV to reflect terminal state
	require.Eventually(t, func() bool {
		entity, found := readLoopFromKV(t, "terminal-test-001")
		return found && entity.State.IsTerminal()
	}, 5*time.Second, 100*time.Millisecond, "Loop should reach terminal state")

	// Verify terminal state details
	entity, _ := readLoopFromKV(t, "terminal-test-001")
	assert.Equal(t, agentic.LoopStateComplete, entity.State)
	assert.False(t, entity.CompletedAt.IsZero(), "CompletedAt should be set")

	// Stop and restart
	err = lc.Stop(5 * time.Second)
	require.NoError(t, err)

	// Terminal state should survive restart
	entityAfterRestart, found := readLoopFromKV(t, "terminal-test-001")
	require.True(t, found, "Terminal state should survive restart")
	assert.Equal(t, agentic.LoopStateComplete, entityAfterRestart.State, "Terminal state should be preserved")
	assert.Equal(t, entity.CompletedAt.Unix(), entityAfterRestart.CompletedAt.Unix(), "CompletedAt should be preserved")
}

// TestIntegration_DuplicateLoopIDOverwritesState proves that CreateLoopWithID does
// NOT check for existing loops — a duplicate task with the same LoopID silently
// overwrites the in-memory state and KV entry.
func TestIntegration_DuplicateLoopIDOverwritesState(t *testing.T) {
	// This test uses the LoopManager directly (not the full component) to prove
	// the overwrite behavior at the API level.
	manager := agenticloop.NewLoopManager()

	// Create a loop and advance it
	loopID, err := manager.CreateLoopWithID("dup-test-001", "task-001", "general", "test-model", 10)
	require.NoError(t, err)
	assert.Equal(t, "dup-test-001", loopID)

	// Advance loop state: transition + increment iterations
	err = manager.TransitionLoop("dup-test-001", agentic.LoopStateExecuting)
	require.NoError(t, err)
	err = manager.IncrementIteration("dup-test-001")
	require.NoError(t, err)
	err = manager.IncrementIteration("dup-test-001")
	require.NoError(t, err)
	err = manager.IncrementIteration("dup-test-001")
	require.NoError(t, err)

	// Verify advanced state
	entity, err := manager.GetLoop("dup-test-001")
	require.NoError(t, err)
	assert.Equal(t, agentic.LoopStateExecuting, entity.State)
	assert.Equal(t, 3, entity.Iterations)

	// DUPLICATE: Create loop with the same ID — this should overwrite
	_, err = manager.CreateLoopWithID("dup-test-001", "task-002", "editor", "new-model", 5)
	require.NoError(t, err, "CreateLoopWithID should not error on duplicate — it silently overwrites")

	// Verify overwrite: state is reset
	overwritten, err := manager.GetLoop("dup-test-001")
	require.NoError(t, err)

	// DOCUMENTS THE GAP: No dedup guard exists.
	// The loop is reset to initial state, losing all progress.
	assert.Equal(t, agentic.LoopStateExploring, overwritten.State, "State should be reset to exploring (overwritten)")
	assert.Equal(t, 0, overwritten.Iterations, "Iterations should be reset to 0 (overwritten)")
	assert.Equal(t, "task-002", overwritten.TaskID, "TaskID should reflect the duplicate task")
	assert.Equal(t, "editor", overwritten.Role, "Role should reflect the duplicate task")
	assert.Equal(t, 5, overwritten.MaxIterations, "MaxIterations should reflect the duplicate task")
}
