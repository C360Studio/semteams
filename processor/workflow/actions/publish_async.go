package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/google/uuid"
)

// PublishAsyncAction publishes a payload with injected callback_subject and task_id,
// then parks the workflow waiting for an async response. The response is correlated
// by task_id via the workflow.step.result callback mechanism.
type PublishAsyncAction struct {
	Subject string
	Payload json.RawMessage
	TaskID  string // Optional, auto-generated if empty
}

// NewPublishAsyncAction creates a new publish_async action
func NewPublishAsyncAction(subject string, payload json.RawMessage, taskID string) *PublishAsyncAction {
	return &PublishAsyncAction{
		Subject: subject,
		Payload: payload,
		TaskID:  taskID,
	}
}

// Execute publishes the payload with injected callback_subject and task_id.
// The caller is expected to park the workflow and wait for an async result.
func (a *PublishAsyncAction) Execute(ctx context.Context, actx *Context) Result {
	start := time.Now()

	if actx.NATSClient == nil {
		return Result{
			Success:  false,
			Error:    "publish_async: NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Auto-generate task_id if not provided
	taskID := a.TaskID
	if taskID == "" {
		taskID = uuid.New().String()
	}

	// Build callback subject for async result correlation
	callbackSubject := ""
	if actx.ExecutionID != "" {
		callbackSubject = fmt.Sprintf("workflow.step.result.%s", actx.ExecutionID)
	}

	// Build AsyncTaskPayload with correlation fields
	asyncTask := &AsyncTaskPayload{
		TaskID:          taskID,
		CallbackSubject: callbackSubject,
		Data:            a.Payload,
	}

	// Validate payload (defense-in-depth, consistent with PublishAgentAction)
	if err := asyncTask.Validate(); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_async: invalid payload: %v", err),
			Duration: time.Since(start),
		}
	}

	// Wrap in BaseMessage envelope for proper type handling
	baseMsg := message.NewBaseMessage(asyncTask.Schema(), asyncTask, "workflow")
	payload, err := json.Marshal(baseMsg)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_async: marshal failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Publish to the stream
	if err := actx.NATSClient.PublishToStream(ctx, a.Subject, payload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_async: send failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Return task_id in output for correlation when async response arrives
	output, _ := json.Marshal(map[string]string{"task_id": taskID})
	return Result{
		Success:  true,
		Output:   output,
		Duration: time.Since(start),
	}
}
