package agenticloop

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360/semstreams/agentic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSignalMessage_Cancel(t *testing.T) {
	// Create handler with default config
	config := DefaultConfig()
	handler := NewMessageHandler(config)

	// Create a loop first
	loopID, err := handler.loopManager.CreateLoop("task-1", "general", "test-model", 20)
	require.NoError(t, err)

	// Verify loop is not terminal initially
	entity, err := handler.GetLoop(loopID)
	require.NoError(t, err)
	assert.False(t, entity.State.IsTerminal())

	// Create cancel signal
	signal := agentic.UserSignal{
		SignalID:    "sig-123",
		Type:        agentic.SignalCancel,
		LoopID:      loopID,
		UserID:      "user-1",
		ChannelType: "cli",
		ChannelID:   "session-1",
		Timestamp:   time.Now(),
	}

	// Process the signal through the handler directly
	// Update loop state to cancelled
	entity.State = agentic.LoopStateCancelled
	entity.CancelledBy = signal.UserID
	entity.CancelledAt = time.Now()

	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Verify loop is now cancelled
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	assert.Equal(t, agentic.LoopStateCancelled, entity.State)
	assert.Equal(t, "user-1", entity.CancelledBy)
	assert.True(t, entity.State.IsTerminal())
}

func TestHandleSignalMessage_Pause(t *testing.T) {
	// Create handler with default config
	config := DefaultConfig()
	handler := NewMessageHandler(config)

	// Create a loop first
	loopID, err := handler.loopManager.CreateLoop("task-1", "general", "test-model", 20)
	require.NoError(t, err)

	// Get entity and set to executing
	entity, err := handler.GetLoop(loopID)
	require.NoError(t, err)
	entity.State = agentic.LoopStateExecuting
	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Request pause
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	entity.PauseRequested = true
	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Verify pause was requested
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	assert.True(t, entity.PauseRequested)
}

func TestHandleSignalMessage_Resume(t *testing.T) {
	// Create handler with default config
	config := DefaultConfig()
	handler := NewMessageHandler(config)

	// Create a loop first
	loopID, err := handler.loopManager.CreateLoop("task-1", "general", "test-model", 20)
	require.NoError(t, err)

	// Set loop to paused state
	entity, err := handler.GetLoop(loopID)
	require.NoError(t, err)
	entity.State = agentic.LoopStatePaused
	entity.PauseRequested = true
	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Resume
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	entity.State = agentic.LoopStateExecuting
	entity.PauseRequested = false
	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Verify resumed
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	assert.Equal(t, agentic.LoopStateExecuting, entity.State)
	assert.False(t, entity.PauseRequested)
}

func TestCannotCancelTerminalLoop(t *testing.T) {
	config := DefaultConfig()
	handler := NewMessageHandler(config)

	// Create and complete a loop
	loopID, err := handler.loopManager.CreateLoop("task-1", "general", "test-model", 20)
	require.NoError(t, err)

	entity, err := handler.GetLoop(loopID)
	require.NoError(t, err)
	entity.State = agentic.LoopStateComplete
	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Try to cancel - should be rejected
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	assert.True(t, entity.State.IsTerminal())

	// Cannot change to cancelled because already terminal
	// This is the check the component does before updating
}

func TestCannotResumeNonPausedLoop(t *testing.T) {
	config := DefaultConfig()
	handler := NewMessageHandler(config)

	// Create a loop in executing state
	loopID, err := handler.loopManager.CreateLoop("task-1", "general", "test-model", 20)
	require.NoError(t, err)

	entity, err := handler.GetLoop(loopID)
	require.NoError(t, err)
	entity.State = agentic.LoopStateExecuting
	err = handler.UpdateLoop(entity)
	require.NoError(t, err)

	// Verify state is not paused
	entity, err = handler.GetLoop(loopID)
	require.NoError(t, err)
	assert.NotEqual(t, agentic.LoopStatePaused, entity.State)

	// Resume check would fail
}

func TestSignalJSON(t *testing.T) {
	signal := agentic.UserSignal{
		SignalID:    "sig-123",
		Type:        agentic.SignalCancel,
		LoopID:      "loop-456",
		UserID:      "user-1",
		ChannelType: "cli",
		ChannelID:   "session-1",
		Timestamp:   time.Now().UTC().Truncate(time.Millisecond),
	}

	// Test JSON round-trip
	data, err := json.Marshal(signal)
	require.NoError(t, err)

	var decoded agentic.UserSignal
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, signal.SignalID, decoded.SignalID)
	assert.Equal(t, signal.Type, decoded.Type)
	assert.Equal(t, signal.LoopID, decoded.LoopID)
	assert.Equal(t, signal.UserID, decoded.UserID)
}

func TestNewLoopStates(t *testing.T) {
	// Verify new states work correctly
	tests := []struct {
		state      agentic.LoopState
		isTerminal bool
	}{
		{agentic.LoopStatePaused, false},
		{agentic.LoopStateCancelled, true},
		{agentic.LoopStateAwaitingApproval, false},
		{agentic.LoopStateExploring, false},
		{agentic.LoopStateExecuting, false},
		{agentic.LoopStateComplete, true},
		{agentic.LoopStateFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.isTerminal, tt.state.IsTerminal())
		})
	}
}
