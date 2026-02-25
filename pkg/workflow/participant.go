package workflow

import "sync"

// Participant is implemented by components that participate in stateful workflows.
// This provides a clear contract for workflow-aware components and enables
// observability tooling to discover and visualize workflow participants.
//
// Implementation notes:
// - Implementations MUST use pointer receivers to ensure interface comparison works correctly
// - WorkflowID() should return a stable value (same value each call) for proper registry tracking
type Participant interface {
	// WorkflowID returns the workflow this component participates in.
	// Returns empty string if component handles multiple workflows dynamically.
	WorkflowID() string

	// Phase returns the phase name this component represents in the workflow.
	Phase() string

	// StateManager returns the state manager for updating workflow state.
	StateManager() *StateManager
}

// ParticipantRegistry tracks all workflow participants for observability.
// This enables discovering the workflow topology from registered components.
type ParticipantRegistry struct {
	mu           sync.RWMutex
	participants map[string][]Participant // workflowID -> participants
}

// NewParticipantRegistry creates a new participant registry.
func NewParticipantRegistry() *ParticipantRegistry {
	return &ParticipantRegistry{
		participants: make(map[string][]Participant),
	}
}

// Register adds a workflow participant to the registry.
func (r *ParticipantRegistry) Register(p Participant) {
	r.mu.Lock()
	defer r.mu.Unlock()

	workflowID := p.WorkflowID()
	if workflowID == "" {
		// Skip dynamic participants that don't have a fixed workflow ID
		return
	}

	r.participants[workflowID] = append(r.participants[workflowID], p)
}

// Unregister removes a workflow participant from the registry.
func (r *ParticipantRegistry) Unregister(p Participant) {
	r.mu.Lock()
	defer r.mu.Unlock()

	workflowID := p.WorkflowID()
	participants := r.participants[workflowID]

	for i, participant := range participants {
		if participant == p {
			r.participants[workflowID] = append(participants[:i], participants[i+1:]...)
			break
		}
	}
}

// GetParticipants returns all participants for a given workflow.
func (r *ParticipantRegistry) GetParticipants(workflowID string) []Participant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid race conditions
	participants := r.participants[workflowID]
	result := make([]Participant, len(participants))
	copy(result, participants)
	return result
}

// GetWorkflowTopology returns the unique phases in registration order for a workflow.
// Duplicate phases are deduplicated (only first occurrence is kept).
// This provides a basic view of the workflow structure based on registered components.
func (r *ParticipantRegistry) GetWorkflowTopology(workflowID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	participants := r.participants[workflowID]
	phases := make([]string, 0, len(participants))
	seen := make(map[string]bool)

	for _, p := range participants {
		phase := p.Phase()
		if phase != "" && !seen[phase] {
			phases = append(phases, phase)
			seen[phase] = true
		}
	}

	return phases
}

// ListWorkflows returns all known workflow IDs.
func (r *ParticipantRegistry) ListWorkflows() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workflows := make([]string, 0, len(r.participants))
	for workflowID := range r.participants {
		workflows = append(workflows, workflowID)
	}
	return workflows
}
