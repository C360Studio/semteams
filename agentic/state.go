// Package agentic provides shared types for the agentic components system.
// This includes loop state management, tool execution interfaces, and trajectory tracking.
package agentic

import (
	"fmt"
)

// LoopState represents the current state of an agentic loop
type LoopState string

// Loop states for the agentic state machine.
// The state machine supports fluid transitions (can move backward) except from terminal states.
const (
	LoopStateExploring    LoopState = "exploring"
	LoopStatePlanning     LoopState = "planning"
	LoopStateArchitecting LoopState = "architecting"
	LoopStateExecuting    LoopState = "executing"
	LoopStateReviewing    LoopState = "reviewing"
	LoopStateComplete     LoopState = "complete"
	LoopStateFailed       LoopState = "failed"
)

// String returns the string representation of the state
func (s LoopState) String() string {
	return string(s)
}

// IsTerminal returns true if the state is a terminal state
func (s LoopState) IsTerminal() bool {
	return s == LoopStateComplete || s == LoopStateFailed
}

// LoopEntity represents an agentic loop instance
type LoopEntity struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	State         LoopState `json:"state"`
	Role          string    `json:"role"`
	Model         string    `json:"model"`
	Iterations    int       `json:"iterations"`
	MaxIterations int       `json:"max_iterations"`
}

// Validate checks if the LoopEntity is valid
func (e *LoopEntity) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("id required")
	}
	if e.State != LoopStateExploring &&
		e.State != LoopStatePlanning &&
		e.State != LoopStateArchitecting &&
		e.State != LoopStateExecuting &&
		e.State != LoopStateReviewing &&
		e.State != LoopStateComplete &&
		e.State != LoopStateFailed {
		return fmt.Errorf("invalid state")
	}
	if e.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations must be greater than 0")
	}
	return nil
}

// TransitionTo transitions the entity to a new state
func (e *LoopEntity) TransitionTo(newState LoopState) error {
	// Allow same-state transitions (no-op)
	if e.State == newState {
		return nil
	}
	// Prevent transitions from terminal states
	if e.State.IsTerminal() {
		return fmt.Errorf("cannot transition from terminal state %s", e.State)
	}
	e.State = newState
	return nil
}

// IncrementIteration increments the iteration counter
func (e *LoopEntity) IncrementIteration() error {
	if e.Iterations >= e.MaxIterations {
		return fmt.Errorf("max iterations reached")
	}
	e.Iterations++
	return nil
}

// NewLoopEntity creates a new LoopEntity with default values
func NewLoopEntity(id, taskID, role, model string, maxIterations ...int) LoopEntity {
	maxIter := 20
	if len(maxIterations) > 0 && maxIterations[0] > 0 {
		maxIter = maxIterations[0]
	}
	return LoopEntity{
		ID:            id,
		TaskID:        taskID,
		State:         LoopStateExploring,
		Role:          role,
		Model:         model,
		Iterations:    0,
		MaxIterations: maxIter,
	}
}
