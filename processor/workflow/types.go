package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/message"
)

// AsyncStepResult represents a generic result from an asynchronous step execution.
// This type is executor-agnostic and can be used by any async action handler
// (agentic-loop, HTTP calls, custom processors) to report step completion.
//
// Usage pattern:
//  1. Workflow publishes async action with callback subject
//  2. Executor processes the action
//  3. Executor publishes AsyncStepResult to callback subject
//  4. Workflow receives result and continues
type AsyncStepResult struct {
	// TaskID correlates this result with the original task request.
	// This is used to match the result with the pending execution.
	TaskID string `json:"task_id"`

	// ExecutionID is the workflow execution this result belongs to.
	// Optional - can be used for direct correlation when TaskID lookup fails.
	ExecutionID string `json:"execution_id,omitempty"`

	// Status indicates the outcome: "success", "failed", "cancelled"
	Status string `json:"status"`

	// Output contains the result payload from the executor.
	// The structure depends on the executor type.
	Output json.RawMessage `json:"output,omitempty"`

	// Error contains the error message if Status is "failed".
	Error string `json:"error,omitempty"`
}

// AsyncStepResult status constants
const (
	AsyncStatusSuccess   = "success"
	AsyncStatusFailed    = "failed"
	AsyncStatusCancelled = "cancelled"
)

// IsSuccess returns true if the result indicates successful completion.
func (r AsyncStepResult) IsSuccess() bool {
	return r.Status == AsyncStatusSuccess
}

// Validate checks if the AsyncStepResult is valid.
func (r AsyncStepResult) Validate() error {
	if r.TaskID == "" && r.ExecutionID == "" {
		return fmt.Errorf("either task_id or execution_id is required")
	}
	if r.Status == "" {
		return fmt.Errorf("status is required")
	}
	return nil
}

// Schema implements message.Payload for proper serialization with BaseMessage.
func (r *AsyncStepResult) Schema() message.Type {
	return message.Type{Domain: "workflow", Category: "step_result", Version: "v1"}
}

// MarshalJSON implements json.Marshaler.
func (r *AsyncStepResult) MarshalJSON() ([]byte, error) {
	type Alias AsyncStepResult
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler
func (r *AsyncStepResult) UnmarshalJSON(data []byte) error {
	type Alias AsyncStepResult
	return json.Unmarshal(data, (*Alias)(r))
}
