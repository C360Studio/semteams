// Package subjects provides typed NATS subject definitions for compile-time type safety.
//
// This package serves as a self-documenting registry of all typed subjects in the system.
// Each subject definition binds a NATS subject pattern to a specific payload type,
// enabling compile-time type checking for publish/subscribe operations.
//
// Usage:
//
//	// Type-safe publish
//	err := subjects.WorkflowStarted.Publish(ctx, client, event)
//
//	// Type-safe subscribe
//	sub, err := subjects.WorkflowStarted.Subscribe(ctx, client, func(ctx context.Context, event WorkflowStartedEvent) error {
//	    return nil
//	})
package subjects

import (
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// Workflow lifecycle events - these are the internal events published by the workflow executor

// WorkflowStartedEvent is published when a workflow execution begins.
type WorkflowStartedEvent struct {
	ExecutionID  string    `json:"execution_id"`
	WorkflowID   string    `json:"workflow_id"`
	WorkflowName string    `json:"workflow_name,omitempty"`
	StartedAt    time.Time `json:"started_at"`
}

// WorkflowCompletedEvent is published when a workflow execution completes successfully.
type WorkflowCompletedEvent struct {
	ExecutionID  string    `json:"execution_id"`
	WorkflowID   string    `json:"workflow_id"`
	WorkflowName string    `json:"workflow_name,omitempty"`
	Iterations   int       `json:"iterations"`
	CompletedAt  time.Time `json:"completed_at"`
}

// WorkflowFailedEvent is published when a workflow execution fails.
type WorkflowFailedEvent struct {
	ExecutionID  string    `json:"execution_id"`
	WorkflowID   string    `json:"workflow_id"`
	WorkflowName string    `json:"workflow_name,omitempty"`
	Error        string    `json:"error"`
	Iterations   int       `json:"iterations"`
	FailedAt     time.Time `json:"failed_at"`
}

// WorkflowTimedOutEvent is published when a workflow execution times out.
type WorkflowTimedOutEvent struct {
	ExecutionID  string    `json:"execution_id"`
	WorkflowID   string    `json:"workflow_id"`
	WorkflowName string    `json:"workflow_name,omitempty"`
	Iterations   int       `json:"iterations"`
	TimedOutAt   time.Time `json:"timed_out_at"`
}

// StepStartedEvent is published when a workflow step begins execution.
type StepStartedEvent struct {
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	StepName    string    `json:"step_name"`
	Iteration   int       `json:"iteration"`
	StartedAt   time.Time `json:"started_at"`
}

// StepCompletedEvent is published when a workflow step completes.
type StepCompletedEvent struct {
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	StepName    string    `json:"step_name"`
	Status      string    `json:"status"` // success, failed, skipped
	Iteration   int       `json:"iteration"`
	CompletedAt time.Time `json:"completed_at"`
}

// Typed subject definitions for workflow events.
// These provide compile-time type safety for NATS publish/subscribe operations.
var (
	// WorkflowStarted is published when a workflow execution begins.
	WorkflowStarted = natsclient.NewSubject[WorkflowStartedEvent]("workflow.events.started")

	// WorkflowCompleted is published when a workflow execution completes successfully.
	WorkflowCompleted = natsclient.NewSubject[WorkflowCompletedEvent]("workflow.events.completed")

	// WorkflowFailed is published when a workflow execution fails.
	WorkflowFailed = natsclient.NewSubject[WorkflowFailedEvent]("workflow.events.failed")

	// WorkflowTimedOut is published when a workflow execution times out.
	WorkflowTimedOut = natsclient.NewSubject[WorkflowTimedOutEvent]("workflow.events.timed_out")

	// StepStarted is published when a workflow step begins execution.
	StepStarted = natsclient.NewSubject[StepStartedEvent]("workflow.events.step.started")

	// StepCompleted is published when a workflow step completes.
	StepCompleted = natsclient.NewSubject[StepCompletedEvent]("workflow.events.step.completed")

	// WorkflowEvents is a wildcard subject for subscribing to all workflow events.
	// Note: This uses a generic map type since it aggregates multiple event types.
	WorkflowEvents = natsclient.NewSubject[map[string]any]("workflow.events.>")
)
