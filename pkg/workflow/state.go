// Package workflow provides workflow state management primitives for components
// that participate in stateful workflows.
//
// This package enables components to track and coordinate workflow state
// without tight coupling to specific workflow engines. Components implement
// Participant to expose their workflow role for observability.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// DefaultStateBucket is the default KV bucket name for workflow state.
const DefaultStateBucket = "WORKFLOW_STATE"

// State represents the state of a workflow execution.
// Components use this to track progress through multi-step workflows.
type State struct {
	// ID is the unique execution identifier.
	ID string `json:"id"`

	// WorkflowID identifies which workflow definition this execution follows.
	WorkflowID string `json:"workflow_id"`

	// Phase is the current execution phase/step name.
	Phase string `json:"phase"`

	// Iteration tracks loop count for retry/iteration patterns.
	Iteration int `json:"iteration"`

	// MaxIter is the maximum allowed iterations (0 = unlimited).
	MaxIter int `json:"max_iter,omitempty"`

	// StartedAt is when the execution started.
	StartedAt time.Time `json:"started_at"`

	// UpdatedAt is when the state was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// CompletedAt is when the execution completed (nil if still running).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Error holds the last error message if the execution failed.
	Error string `json:"error,omitempty"`

	// Context holds arbitrary workflow-specific context data.
	Context map[string]any `json:"context,omitempty"`
}

// IsComplete returns true if the execution has completed.
func (s *State) IsComplete() bool {
	return s.CompletedAt != nil
}

// IsFailed returns true if the execution failed with an error.
func (s *State) IsFailed() bool {
	return s.Error != ""
}

// StateEntry wraps a State with its KV revision for optimistic concurrency.
type StateEntry struct {
	State    *State
	Revision uint64
}

// StateManager provides operations for managing workflow state in NATS KV.
type StateManager struct {
	bucket jetstream.KeyValue
	logger *slog.Logger
}

// NewStateManager creates a new StateManager backed by the given KV bucket.
func NewStateManager(bucket jetstream.KeyValue, logger *slog.Logger) *StateManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &StateManager{
		bucket: bucket,
		logger: logger,
	}
}

// Get retrieves a workflow state by ID.
func (m *StateManager) Get(ctx context.Context, id string) (*State, error) {
	entry, err := m.bucket.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get workflow state %s: %w", id, err)
	}

	var state State
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		return nil, fmt.Errorf("unmarshal workflow state %s: %w", id, err)
	}

	return &state, nil
}

// GetWithRevision retrieves a workflow state by ID along with its revision
// for use with optimistic concurrency control.
func (m *StateManager) GetWithRevision(ctx context.Context, id string) (*StateEntry, error) {
	entry, err := m.bucket.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get workflow state %s: %w", id, err)
	}

	var state State
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		return nil, fmt.Errorf("unmarshal workflow state %s: %w", id, err)
	}

	return &StateEntry{
		State:    &state,
		Revision: entry.Revision(),
	}, nil
}

// Put stores a workflow state, creating or updating as needed.
func (m *StateManager) Put(ctx context.Context, state *State) error {
	state.UpdatedAt = time.Now()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal workflow state %s: %w", state.ID, err)
	}

	_, err = m.bucket.Put(ctx, state.ID, data)
	if err != nil {
		return fmt.Errorf("put workflow state %s: %w", state.ID, err)
	}
	return nil
}

// Update stores a workflow state with optimistic concurrency control.
// Returns the new revision on success. Fails if the revision has changed.
func (m *StateManager) Update(ctx context.Context, state *State, expectedRevision uint64) (uint64, error) {
	state.UpdatedAt = time.Now()

	data, err := json.Marshal(state)
	if err != nil {
		return 0, fmt.Errorf("marshal workflow state %s: %w", state.ID, err)
	}

	rev, err := m.bucket.Update(ctx, state.ID, data, expectedRevision)
	if err != nil {
		return 0, fmt.Errorf("update workflow state %s (rev %d): %w", state.ID, expectedRevision, err)
	}
	return rev, nil
}

// Create creates a new workflow state, failing if it already exists.
func (m *StateManager) Create(ctx context.Context, state *State) error {
	now := time.Now()
	state.StartedAt = now
	state.UpdatedAt = now

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal workflow state %s: %w", state.ID, err)
	}

	_, err = m.bucket.Create(ctx, state.ID, data)
	if err != nil {
		return fmt.Errorf("create workflow state %s: %w", state.ID, err)
	}
	return nil
}

// Transition updates the workflow state to a new phase.
// Note: This uses read-modify-write without optimistic concurrency.
// For concurrent access, use GetWithRevision + Update instead.
func (m *StateManager) Transition(ctx context.Context, id, phase string) error {
	state, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	state.Phase = phase
	return m.Put(ctx, state)
}

// TransitionWithRevision updates the workflow state to a new phase with optimistic concurrency.
// Returns the new revision on success. Fails if the revision has changed.
func (m *StateManager) TransitionWithRevision(ctx context.Context, id, phase string, expectedRevision uint64) (uint64, error) {
	state, err := m.Get(ctx, id)
	if err != nil {
		return 0, err
	}

	state.Phase = phase
	return m.Update(ctx, state, expectedRevision)
}

// IncrementIteration increments the iteration counter.
// Note: This uses read-modify-write without optimistic concurrency.
// For concurrent access, use GetWithRevision + Update instead.
func (m *StateManager) IncrementIteration(ctx context.Context, id string) error {
	state, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	state.Iteration++
	return m.Put(ctx, state)
}

// IncrementIterationWithRevision increments the iteration counter with optimistic concurrency.
// Returns the new revision on success. Fails if the revision has changed.
func (m *StateManager) IncrementIterationWithRevision(ctx context.Context, id string, expectedRevision uint64) (uint64, error) {
	state, err := m.Get(ctx, id)
	if err != nil {
		return 0, err
	}

	state.Iteration++
	return m.Update(ctx, state, expectedRevision)
}

// Complete marks the workflow execution as completed successfully.
func (m *StateManager) Complete(ctx context.Context, id string) error {
	state, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	state.CompletedAt = &now
	state.Phase = "completed"
	return m.Put(ctx, state)
}

// Fail marks the workflow execution as failed with an error message.
func (m *StateManager) Fail(ctx context.Context, id, errMsg string) error {
	state, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	state.CompletedAt = &now
	state.Phase = "failed"
	state.Error = errMsg
	return m.Put(ctx, state)
}

// List returns all workflow states in the bucket.
func (m *StateManager) List(ctx context.Context) ([]*State, error) {
	keys, err := m.bucket.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflow state keys: %w", err)
	}

	states := make([]*State, 0, len(keys))
	for _, key := range keys {
		state, err := m.Get(ctx, key)
		if err != nil {
			m.logger.Warn("Failed to get state", "key", key, "error", err)
			continue
		}
		states = append(states, state)
	}

	return states, nil
}

// Delete removes a workflow state.
func (m *StateManager) Delete(ctx context.Context, id string) error {
	if err := m.bucket.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete workflow state %s: %w", id, err)
	}
	return nil
}
