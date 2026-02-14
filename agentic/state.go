// Package agentic provides shared types for the agentic components system.
// This includes loop state management, tool execution interfaces, and trajectory tracking.
package agentic

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic/identity"
)

// LoopState represents the current state of an agentic loop
type LoopState string

// Loop states for the agentic state machine.
// The state machine supports fluid transitions (can move backward) except from terminal states.
const (
	// Standard workflow states
	LoopStateExploring    LoopState = "exploring"
	LoopStatePlanning     LoopState = "planning"
	LoopStateArchitecting LoopState = "architecting"
	LoopStateExecuting    LoopState = "executing"
	LoopStateReviewing    LoopState = "reviewing"

	// Terminal states
	LoopStateComplete  LoopState = "complete"
	LoopStateFailed    LoopState = "failed"
	LoopStateCancelled LoopState = "cancelled" // Cancelled by user signal

	// Signal-related states
	LoopStatePaused           LoopState = "paused"            // Paused by user signal
	LoopStateAwaitingApproval LoopState = "awaiting_approval" // Waiting for user approval
)

// String returns the string representation of the state
func (s LoopState) String() string {
	return string(s)
}

// IsTerminal returns true if the state is a terminal state
func (s LoopState) IsTerminal() bool {
	return s == LoopStateComplete || s == LoopStateFailed || s == LoopStateCancelled
}

// LoopEntity represents an agentic loop instance
type LoopEntity struct {
	ID                 string                `json:"id"`
	TaskID             string                `json:"task_id"`
	State              LoopState             `json:"state"`
	Role               string                `json:"role"`
	Model              string                `json:"model"`
	Iterations         int                   `json:"iterations"`
	MaxIterations      int                   `json:"max_iterations"`
	PendingToolResults map[string]ToolResult `json:"pending_tool_results,omitempty"` // Accumulated tool results by call ID
	StartedAt          time.Time             `json:"started_at,omitempty"`           // When the loop was created
	TimeoutAt          time.Time             `json:"timeout_at,omitempty"`           // When the loop should timeout
	ParentLoopID       string                `json:"parent_loop_id,omitempty"`       // Parent loop ID for architect->editor relationship

	// Multi-agent depth tracking
	Depth    int `json:"depth,omitempty"`     // Current depth in agent tree (0 = root)
	MaxDepth int `json:"max_depth,omitempty"` // Maximum allowed depth for spawned agents

	// Signal support fields
	PauseRequested   bool      `json:"pause_requested,omitempty"`    // Pause requested, will pause at next checkpoint
	PauseRequestedBy string    `json:"pause_requested_by,omitempty"` // User who requested pause
	StateBeforePause LoopState `json:"state_before_pause,omitempty"` // State before pause (for resume)
	CancelledBy      string    `json:"cancelled_by,omitempty"`       // User who cancelled the loop
	CancelledAt      time.Time `json:"cancelled_at,omitempty"`       // When the loop was cancelled

	// User context (for routing responses)
	UserID      string `json:"user_id,omitempty"`      // User who initiated the loop
	ChannelType string `json:"channel_type,omitempty"` // cli, slack, discord, web
	ChannelID   string `json:"channel_id,omitempty"`   // Channel/session ID for routing responses

	// Workflow context (for loops created by workflow commands)
	WorkflowSlug string `json:"workflow_slug,omitempty"` // e.g., "add-user-auth"
	WorkflowStep string `json:"workflow_step,omitempty"` // e.g., "design"

	// Callback subject for async result delivery (set by workflow)
	// When present, the loop publishes completion to this subject
	// in addition to the default agent.complete subject.
	Callback string `json:"callback,omitempty"` // e.g., "workflow.step.result.{exec_id}"

	// Completion data (populated when loop completes)
	// These fields enable SSE delivery of results via KV watch
	Outcome     string    `json:"outcome,omitempty"`      // success, failed, cancelled
	Result      string    `json:"result,omitempty"`       // LLM response content
	Error       string    `json:"error,omitempty"`        // Error message on failure
	CompletedAt time.Time `json:"completed_at,omitempty"` // When the loop completed

	// AGNTCY identity (Phase 2 AGNTCY integration)
	// When set, provides DID-based cryptographic identity for this agent loop.
	Identity *identity.AgentIdentity `json:"identity,omitempty"`
}

// Validate checks if the LoopEntity is valid
func (e *LoopEntity) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("id required")
	}
	if !isValidLoopState(e.State) {
		return fmt.Errorf("invalid state: %s", e.State)
	}
	if e.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations must be greater than 0")
	}
	return nil
}

// isValidLoopState checks if the state is a valid LoopState
func isValidLoopState(s LoopState) bool {
	switch s {
	case LoopStateExploring, LoopStatePlanning, LoopStateArchitecting,
		LoopStateExecuting, LoopStateReviewing, LoopStateComplete,
		LoopStateFailed, LoopStateCancelled, LoopStatePaused,
		LoopStateAwaitingApproval:
		return true
	default:
		return false
	}
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
