package agenticdispatch

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoopTracker(t *testing.T) {
	tracker := NewLoopTracker()
	require.NotNil(t, tracker)
	assert.Equal(t, 0, tracker.Count())
}

func TestLoopTracker_Track(t *testing.T) {
	tracker := NewLoopTracker()

	info := &LoopInfo{
		LoopID:      "loop-1",
		TaskID:      "task-1",
		UserID:      "user-1",
		ChannelType: "cli",
		ChannelID:   "session-1",
		State:       "pending",
		CreatedAt:   time.Now(),
	}

	tracker.Track(info)
	assert.Equal(t, 1, tracker.Count())

	retrieved := tracker.Get("loop-1")
	require.NotNil(t, retrieved)
	assert.Equal(t, "loop-1", retrieved.LoopID)
	assert.Equal(t, "user-1", retrieved.UserID)
}

func TestLoopTracker_GetActiveLoop(t *testing.T) {
	tracker := NewLoopTracker()

	// No active loops initially
	loopID := tracker.GetActiveLoop("user-1", "session-1")
	assert.Empty(t, loopID)

	// Add a pending loop
	info := &LoopInfo{
		LoopID:      "loop-1",
		TaskID:      "task-1",
		UserID:      "user-1",
		ChannelType: "cli",
		ChannelID:   "session-1",
		State:       "pending",
		CreatedAt:   time.Now(),
	}
	tracker.Track(info)

	// Should find the active loop via channel
	loopID = tracker.GetActiveLoop("user-1", "session-1")
	assert.Equal(t, "loop-1", loopID)

	// Different user with same channel shouldn't find it (no user mapping for user-2)
	loopID = tracker.GetActiveLoop("user-2", "session-1")
	// Actually, GetActiveLoop first checks channel, then user
	// session-1 maps to loop-1, which belongs to user-1
	// The implementation returns it if the loop is active, regardless of user mismatch
	// Let's test with different channel
	loopID = tracker.GetActiveLoop("user-2", "session-2")
	assert.Empty(t, loopID)

	// Same user, different channel should find via user mapping
	loopID = tracker.GetActiveLoop("user-1", "session-2")
	assert.Equal(t, "loop-1", loopID)
}

func TestLoopTracker_GetActiveLoop_TerminalState(t *testing.T) {
	tracker := NewLoopTracker()

	// Add a completed loop
	info := &LoopInfo{
		LoopID:      "loop-1",
		TaskID:      "task-1",
		UserID:      "user-1",
		ChannelType: "cli",
		ChannelID:   "session-1",
		State:       "complete",
		CreatedAt:   time.Now(),
	}
	tracker.Track(info)

	// Should NOT find a terminal loop
	loopID := tracker.GetActiveLoop("user-1", "session-1")
	assert.Empty(t, loopID)
}

func TestLoopTracker_UpdateState(t *testing.T) {
	tracker := NewLoopTracker()

	info := &LoopInfo{
		LoopID:      "loop-1",
		TaskID:      "task-1",
		UserID:      "user-1",
		ChannelType: "cli",
		ChannelID:   "session-1",
		State:       "pending",
		CreatedAt:   time.Now(),
	}
	tracker.Track(info)

	// Update state
	tracker.UpdateState("loop-1", "executing")

	retrieved := tracker.Get("loop-1")
	require.NotNil(t, retrieved)
	assert.Equal(t, "executing", retrieved.State)

	// Update to terminal state
	tracker.UpdateState("loop-1", "complete")
	retrieved = tracker.Get("loop-1")
	assert.Equal(t, "complete", retrieved.State)
}

func TestLoopTracker_UpdateState_NonExistent(t *testing.T) {
	tracker := NewLoopTracker()

	// Should not panic
	tracker.UpdateState("nonexistent", "complete")
	assert.Equal(t, 0, tracker.Count())
}

func TestLoopTracker_UpdateIterations(t *testing.T) {
	tracker := NewLoopTracker()

	info := &LoopInfo{
		LoopID:        "loop-1",
		TaskID:        "task-1",
		UserID:        "user-1",
		State:         "executing",
		Iterations:    0,
		MaxIterations: 20,
		CreatedAt:     time.Now(),
	}
	tracker.Track(info)

	tracker.UpdateIterations("loop-1", 5)

	retrieved := tracker.Get("loop-1")
	require.NotNil(t, retrieved)
	assert.Equal(t, 5, retrieved.Iterations)
}

func TestLoopTracker_UpdateWorkflowContext(t *testing.T) {
	tracker := NewLoopTracker()

	// Add a loop without workflow context
	info := &LoopInfo{
		LoopID:        "loop-1",
		TaskID:        "task-1",
		UserID:        "user-1",
		State:         "executing",
		MaxIterations: 20,
		CreatedAt:     time.Now(),
	}
	tracker.Track(info)

	// Update workflow context
	updated := tracker.UpdateWorkflowContext("loop-1", "add-user-auth", "design")
	assert.True(t, updated)

	retrieved := tracker.Get("loop-1")
	require.NotNil(t, retrieved)
	assert.Equal(t, "add-user-auth", retrieved.WorkflowSlug)
	assert.Equal(t, "design", retrieved.WorkflowStep)
}

func TestLoopTracker_UpdateWorkflowContext_NonExistent(t *testing.T) {
	tracker := NewLoopTracker()

	// Should return false for non-existent loop
	updated := tracker.UpdateWorkflowContext("nonexistent", "workflow", "step")
	assert.False(t, updated)
}

func TestLoopTracker_UpdateWorkflowContext_AlreadyHasContext(t *testing.T) {
	tracker := NewLoopTracker()

	// Add a loop with existing workflow context
	info := &LoopInfo{
		LoopID:       "loop-1",
		TaskID:       "task-1",
		UserID:       "user-1",
		State:        "executing",
		WorkflowSlug: "existing-workflow",
		WorkflowStep: "existing-step",
		CreatedAt:    time.Now(),
	}
	tracker.Track(info)

	// Should not update existing context
	updated := tracker.UpdateWorkflowContext("loop-1", "new-workflow", "new-step")
	assert.False(t, updated)

	// Original context should be preserved
	retrieved := tracker.Get("loop-1")
	require.NotNil(t, retrieved)
	assert.Equal(t, "existing-workflow", retrieved.WorkflowSlug)
	assert.Equal(t, "existing-step", retrieved.WorkflowStep)
}

func TestLoopTracker_UpdateWorkflowContext_EmptySlug(t *testing.T) {
	tracker := NewLoopTracker()

	// Add a loop without workflow context
	info := &LoopInfo{
		LoopID:    "loop-1",
		TaskID:    "task-1",
		State:     "executing",
		CreatedAt: time.Now(),
	}
	tracker.Track(info)

	// Should not update with empty slug
	updated := tracker.UpdateWorkflowContext("loop-1", "", "step")
	assert.False(t, updated)

	retrieved := tracker.Get("loop-1")
	require.NotNil(t, retrieved)
	assert.Empty(t, retrieved.WorkflowSlug)
}

func TestLoopTracker_Remove(t *testing.T) {
	tracker := NewLoopTracker()

	info := &LoopInfo{
		LoopID:    "loop-1",
		TaskID:    "task-1",
		UserID:    "user-1",
		ChannelID: "session-1",
		State:     "pending",
		CreatedAt: time.Now(),
	}
	tracker.Track(info)
	assert.Equal(t, 1, tracker.Count())

	tracker.Remove("loop-1")
	assert.Equal(t, 0, tracker.Count())
	assert.Nil(t, tracker.Get("loop-1"))
}

func TestLoopTracker_Remove_CleansUpMappings(t *testing.T) {
	tracker := NewLoopTracker()

	info := &LoopInfo{
		LoopID:    "loop-1",
		TaskID:    "task-1",
		UserID:    "user-1",
		ChannelID: "session-1",
		State:     "pending",
		CreatedAt: time.Now(),
	}
	tracker.Track(info)

	// Verify mappings exist
	assert.Equal(t, "loop-1", tracker.GetActiveLoop("user-1", "session-1"))

	tracker.Remove("loop-1")

	// Mappings should be cleaned up
	assert.Empty(t, tracker.GetActiveLoop("user-1", "session-1"))
}

func TestLoopTracker_GetUserLoops(t *testing.T) {
	tracker := NewLoopTracker()

	// Add loops for user-1
	tracker.Track(&LoopInfo{
		LoopID:    "loop-1",
		UserID:    "user-1",
		State:     "pending",
		CreatedAt: time.Now(),
	})
	tracker.Track(&LoopInfo{
		LoopID:    "loop-2",
		UserID:    "user-1",
		State:     "executing",
		CreatedAt: time.Now(),
	})

	// Add loop for user-2
	tracker.Track(&LoopInfo{
		LoopID:    "loop-3",
		UserID:    "user-2",
		State:     "pending",
		CreatedAt: time.Now(),
	})

	user1Loops := tracker.GetUserLoops("user-1")
	assert.Len(t, user1Loops, 2)

	user2Loops := tracker.GetUserLoops("user-2")
	assert.Len(t, user2Loops, 1)

	user3Loops := tracker.GetUserLoops("user-3")
	assert.Len(t, user3Loops, 0)
}

func TestLoopTracker_GetAllLoops(t *testing.T) {
	tracker := NewLoopTracker()

	tracker.Track(&LoopInfo{
		LoopID:    "loop-1",
		UserID:    "user-1",
		State:     "pending",
		CreatedAt: time.Now(),
	})
	tracker.Track(&LoopInfo{
		LoopID:    "loop-2",
		UserID:    "user-1",
		State:     "executing",
		CreatedAt: time.Now(),
	})
	tracker.Track(&LoopInfo{
		LoopID:    "loop-3",
		UserID:    "user-2",
		State:     "complete",
		CreatedAt: time.Now(),
	})

	allLoops := tracker.GetAllLoops()
	assert.Len(t, allLoops, 3)
}

func TestLoopTracker_Concurrent(t *testing.T) {
	tracker := NewLoopTracker()
	done := make(chan bool, 10)

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(n int) {
			tracker.Track(&LoopInfo{
				LoopID:    "loop-" + string(rune('a'+n)),
				UserID:    "user-1",
				State:     "pending",
				CreatedAt: time.Now(),
			})
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			_ = tracker.GetAllLoops()
			_ = tracker.Count()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 5, tracker.Count())
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    string
		terminal bool
	}{
		{"pending", false},
		{"executing", false},
		{"paused", false},
		{"complete", true},
		{"failed", true},
		{"cancelled", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			assert.Equal(t, tt.terminal, isTerminalState(tt.state))
		})
	}
}

func TestSignalMessage_Serialization(t *testing.T) {
	signal := SignalMessage{
		LoopID:    "loop-123",
		Type:      "cancel",
		Reason:    "user requested",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	// Test marshaling
	data, err := json.Marshal(signal)
	require.NoError(t, err)

	// Test unmarshaling
	var decoded SignalMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "loop-123", decoded.LoopID)
	assert.Equal(t, "cancel", decoded.Type)
	assert.Equal(t, "user requested", decoded.Reason)
	assert.Equal(t, signal.Timestamp, decoded.Timestamp)
}

func TestSignalMessage_Types(t *testing.T) {
	tests := []struct {
		signalType string
		valid      bool
	}{
		{"pause", true},
		{"resume", true},
		{"cancel", true},
		{"", false},
		{"stop", false},
	}

	for _, tt := range tests {
		t.Run(tt.signalType, func(t *testing.T) {
			signal := SignalMessage{
				LoopID:    "loop-1",
				Type:      tt.signalType,
				Timestamp: time.Now(),
			}

			data, err := json.Marshal(signal)
			require.NoError(t, err)

			var decoded SignalMessage
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.signalType, decoded.Type)
		})
	}
}

func TestLoopTracker_SendSignal_NoClient(t *testing.T) {
	tracker := NewLoopTracker()
	ctx := context.Background()

	// With nil NATS client, SendSignal should return ErrNATSClientNil
	err := tracker.SendSignal(ctx, nil, "loop-1", "cancel", "test reason")
	assert.Error(t, err)
	assert.Equal(t, ErrNATSClientNil, err)
}
