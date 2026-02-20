package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
			Error:    "NATS client not available",
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

	// Parse the original payload to inject callback fields
	var payloadMap map[string]any
	if len(a.Payload) > 0 {
		if err := json.Unmarshal(a.Payload, &payloadMap); err != nil {
			// If payload isn't a JSON object, wrap it
			payloadMap = map[string]any{
				"data": json.RawMessage(a.Payload),
			}
		}
	} else {
		payloadMap = make(map[string]any)
	}

	// Inject callback fields
	payloadMap["task_id"] = taskID
	if callbackSubject != "" {
		payloadMap["callback_subject"] = callbackSubject
	}

	// Marshal the enriched payload
	enrichedPayload, err := json.Marshal(payloadMap)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("failed to marshal enriched payload: %v", err),
			Duration: time.Since(start),
		}
	}

	// Publish to the stream
	if err := actx.NATSClient.PublishToStream(ctx, a.Subject, enrichedPayload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_async failed: %v", err),
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
