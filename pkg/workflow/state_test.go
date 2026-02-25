package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestState_IsComplete(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected bool
	}{
		{
			name:     "not complete",
			state:    State{Phase: "running"},
			expected: false,
		},
		{
			name: "completed",
			state: State{
				Phase:       "completed",
				CompletedAt: ptrTime(time.Now()),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.IsComplete())
		})
	}
}

func TestState_IsFailed(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected bool
	}{
		{
			name:     "not failed",
			state:    State{Phase: "running"},
			expected: false,
		},
		{
			name: "failed",
			state: State{
				Phase: "failed",
				Error: "something went wrong",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.IsFailed())
		})
	}
}

func TestParticipantRegistry_Register(t *testing.T) {
	registry := NewParticipantRegistry()

	// Create mock participant
	mock := &mockParticipant{
		workflowID: "test-workflow",
		phase:      "processing",
	}

	registry.Register(mock)

	participants := registry.GetParticipants("test-workflow")
	assert.Len(t, participants, 1)
	assert.Equal(t, mock, participants[0])
}

func TestParticipantRegistry_RegisterDynamicParticipant(t *testing.T) {
	registry := NewParticipantRegistry()

	// Create dynamic participant (empty workflow ID)
	mock := &mockParticipant{
		workflowID: "", // Dynamic participant
		phase:      "processing",
	}

	registry.Register(mock)

	// Should not be registered
	participants := registry.GetParticipants("")
	assert.Len(t, participants, 0)
}

func TestParticipantRegistry_Unregister(t *testing.T) {
	registry := NewParticipantRegistry()

	mock1 := &mockParticipant{workflowID: "test-workflow", phase: "phase1"}
	mock2 := &mockParticipant{workflowID: "test-workflow", phase: "phase2"}

	registry.Register(mock1)
	registry.Register(mock2)

	assert.Len(t, registry.GetParticipants("test-workflow"), 2)

	registry.Unregister(mock1)

	participants := registry.GetParticipants("test-workflow")
	assert.Len(t, participants, 1)
	assert.Equal(t, mock2, participants[0])
}

func TestParticipantRegistry_GetWorkflowTopology(t *testing.T) {
	registry := NewParticipantRegistry()

	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "init"})
	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "process"})
	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "complete"})

	topology := registry.GetWorkflowTopology("test-workflow")

	assert.Equal(t, []string{"init", "process", "complete"}, topology)
}

func TestParticipantRegistry_GetWorkflowTopology_Deduplicates(t *testing.T) {
	registry := NewParticipantRegistry()

	// Register multiple participants with the same phase
	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "init"})
	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "process"})
	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "process"}) // Duplicate
	registry.Register(&mockParticipant{workflowID: "test-workflow", phase: "complete"})

	topology := registry.GetWorkflowTopology("test-workflow")

	// Duplicates should be removed
	assert.Equal(t, []string{"init", "process", "complete"}, topology)
}

func TestParticipantRegistry_ListWorkflows(t *testing.T) {
	registry := NewParticipantRegistry()

	registry.Register(&mockParticipant{workflowID: "workflow-a", phase: "init"})
	registry.Register(&mockParticipant{workflowID: "workflow-b", phase: "init"})

	workflows := registry.ListWorkflows()

	assert.Len(t, workflows, 2)
	assert.Contains(t, workflows, "workflow-a")
	assert.Contains(t, workflows, "workflow-b")
}

// Mock implementations for testing

type mockParticipant struct {
	workflowID string
	phase      string
}

func (m *mockParticipant) WorkflowID() string          { return m.workflowID }
func (m *mockParticipant) Phase() string               { return m.phase }
func (m *mockParticipant) StateManager() *StateManager { return nil }

func ptrTime(t time.Time) *time.Time {
	return &t
}
