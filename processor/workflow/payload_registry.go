package workflow

import (
	"encoding/json"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Builder functions for workflow payload types

func buildTriggerPayload(fields map[string]any) (any, error) {
	msg := &TriggerPayload{}

	if v, ok := fields["workflow_id"].(string); ok {
		msg.WorkflowID = v
	}
	if v, ok := fields["role"].(string); ok {
		msg.Role = v
	}
	if v, ok := fields["model"].(string); ok {
		msg.Model = v
	}
	if v, ok := fields["prompt"].(string); ok {
		msg.Prompt = v
	}
	if v, ok := fields["user_id"].(string); ok {
		msg.UserID = v
	}
	if v, ok := fields["channel_type"].(string); ok {
		msg.ChannelType = v
	}
	if v, ok := fields["channel_id"].(string); ok {
		msg.ChannelID = v
	}
	if v, ok := fields["request_id"].(string); ok {
		msg.RequestID = v
	}

	// Handle data (json.RawMessage)
	if v, ok := fields["data"]; ok {
		if data, err := json.Marshal(v); err == nil {
			msg.Data = data
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, errs.Wrap(err, "TriggerPayload", "buildTriggerPayload", "validation failed")
	}

	return msg, nil
}

func buildStepCompleteMessage(fields map[string]any) (any, error) {
	msg := &StepCompleteMessage{}

	if v, ok := fields["execution_id"].(string); ok {
		msg.ExecutionID = v
	}
	if v, ok := fields["step_name"].(string); ok {
		msg.StepName = v
	}
	if v, ok := fields["status"].(string); ok {
		msg.Status = v
	}
	if v, ok := fields["error"].(string); ok {
		msg.Error = v
	}
	if v, ok := fields["duration"].(string); ok {
		msg.Duration = v
	}

	// Handle iteration
	if v, ok := fields["iteration"].(int); ok {
		msg.Iteration = v
	} else if v, ok := fields["iteration"].(float64); ok {
		msg.Iteration = int(v)
	}

	// Handle started_at
	if v, ok := fields["started_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.StartedAt = t
		}
	}

	// Handle completed_at
	if v, ok := fields["completed_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.CompletedAt = t
		}
	}

	// Handle output (json.RawMessage)
	if v, ok := fields["output"]; ok {
		if data, err := json.Marshal(v); err == nil {
			msg.Output = data
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, errs.Wrap(err, "StepCompleteMessage", "buildStepCompleteMessage", "validation failed")
	}

	return msg, nil
}

func buildEvent(fields map[string]any) (any, error) {
	msg := &event{}

	if v, ok := fields["type"].(string); ok {
		msg.Type = v
	}
	if v, ok := fields["execution_id"].(string); ok {
		msg.ExecutionID = v
	}
	if v, ok := fields["workflow_id"].(string); ok {
		msg.WorkflowID = v
	}
	if v, ok := fields["step_name"].(string); ok {
		msg.StepName = v
	}
	if v, ok := fields["error"].(string); ok {
		msg.Error = v
	}

	// Handle iteration
	if v, ok := fields["iteration"].(int); ok {
		msg.Iteration = v
	} else if v, ok := fields["iteration"].(float64); ok {
		msg.Iteration = int(v)
	}

	// Handle state (ExecutionState)
	if v, ok := fields["state"].(string); ok {
		msg.State = ExecutionState(v)
	}

	// Handle timestamp
	if v, ok := fields["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.Timestamp = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, errs.Wrap(err, "event", "buildEvent", "validation failed")
	}

	return msg, nil
}

func buildAsyncStepResult(fields map[string]any) (any, error) {
	msg := &AsyncStepResult{}

	if v, ok := fields["task_id"].(string); ok {
		msg.TaskID = v
	}
	if v, ok := fields["execution_id"].(string); ok {
		msg.ExecutionID = v
	}
	if v, ok := fields["status"].(string); ok {
		msg.Status = v
	}
	if v, ok := fields["error"].(string); ok {
		msg.Error = v
	}

	// Handle output (json.RawMessage)
	if v, ok := fields["output"]; ok {
		if data, err := json.Marshal(v); err == nil {
			msg.Output = data
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, errs.Wrap(err, "AsyncStepResult", "buildAsyncStepResult", "validation failed")
	}

	return msg, nil
}

// init registers all workflow payload types with the global PayloadRegistry.
// This enables BaseMessage.UnmarshalJSON to recreate typed payloads from JSON
// when the message type matches one of the workflow types.
func init() {
	// Register TriggerPayload factory
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "trigger",
		Version:     "v1",
		Description: "Workflow trigger event",
		Factory:     func() any { return &TriggerPayload{} },
		Builder:     buildTriggerPayload,
	})
	if err != nil {
		panic("failed to register TriggerPayload: " + err.Error())
	}

	// Register StepCompleteMessage factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "step_complete",
		Version:     "v1",
		Description: "Step completion from agents",
		Factory:     func() any { return &StepCompleteMessage{} },
		Builder:     buildStepCompleteMessage,
	})
	if err != nil {
		panic("failed to register StepCompleteMessage: " + err.Error())
	}

	// Register event factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "event",
		Version:     "v1",
		Description: "Workflow lifecycle event",
		Factory:     func() any { return &event{} },
		Builder:     buildEvent,
	})
	if err != nil {
		panic("failed to register event: " + err.Error())
	}

	// Register AsyncStepResult factory for generic async callback handling
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "step_result",
		Version:     "v1",
		Description: "Async step result from any executor",
		Factory:     func() any { return &AsyncStepResult{} },
		Builder:     buildAsyncStepResult,
	})
	if err != nil {
		panic("failed to register AsyncStepResult: " + err.Error())
	}
}
