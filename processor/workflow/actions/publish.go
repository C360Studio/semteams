package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/google/uuid"
)

// PublishAction performs a fire-and-forget NATS publish
type PublishAction struct {
	Subject string
	Payload json.RawMessage
}

// NewPublishAction creates a new publish action
func NewPublishAction(subject string, payload json.RawMessage) *PublishAction {
	return &PublishAction{
		Subject: subject,
		Payload: payload,
	}
}

// Execute publishes the message to NATS
func (a *PublishAction) Execute(ctx context.Context, actx *Context) Result {
	start := time.Now()

	if actx.NATSClient == nil {
		return Result{
			Success:  false,
			Error:    "publish: NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Parse payload into map for GenericJSONPayload
	dataMap := parsePayloadToMap(a.Payload)

	// Wrap in GenericJSONPayload + BaseMessage envelope
	genericPayload := message.NewGenericJSON(dataMap)
	baseMsg := message.NewBaseMessage(genericPayload.Schema(), genericPayload, "workflow")
	payload, err := json.Marshal(baseMsg)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish: marshal failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Publish to JetStream for durability
	if err := actx.NATSClient.PublishToStream(ctx, a.Subject, payload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish: send failed: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Duration: time.Since(start),
	}
}

// parsePayloadToMap converts a JSON payload to a map suitable for GenericJSONPayload.
// - JSON objects are returned as-is
// - JSON primitives/arrays are wrapped under "value" key
// - Invalid JSON is wrapped under "raw" key as a string
func parsePayloadToMap(payload json.RawMessage) map[string]any {
	if len(payload) == 0 {
		return make(map[string]any)
	}

	// First try to unmarshal as any to check the JSON type
	var rawValue any
	if err := json.Unmarshal(payload, &rawValue); err != nil {
		// Invalid JSON - wrap raw bytes as string
		return map[string]any{"raw": string(payload)}
	}

	// Check if it's already a map (JSON object)
	if dataMap, ok := rawValue.(map[string]any); ok {
		return dataMap
	}

	// Valid JSON but not an object (string, number, array, bool, null)
	// Wrap under "value" key to preserve the actual value
	return map[string]any{"value": rawValue}
}

// PublishAgentAction publishes a task to an agent using structured fields.
// When ExecutionID is available in the action context, it sets a callback subject
// so the executor can publish results back to the workflow.
type PublishAgentAction struct {
	Subject string
	Role    string
	Model   string
	Prompt  string
	TaskID  string // Optional, auto-generated if empty
}

// NewPublishAgentAction creates a new publish_agent action with structured fields
func NewPublishAgentAction(subject, role, model, prompt, taskID string) *PublishAgentAction {
	return &PublishAgentAction{
		Subject: subject,
		Role:    role,
		Model:   model,
		Prompt:  prompt,
		TaskID:  taskID,
	}
}

// Execute publishes the agent task message after validating required fields.
func (a *PublishAgentAction) Execute(ctx context.Context, actx *Context) Result {
	start := time.Now()

	// Build TaskMessage from structured fields
	task := agentic.TaskMessage{
		TaskID: a.TaskID,
		Role:   a.Role,
		Model:  a.Model,
		Prompt: a.Prompt,
	}

	// Auto-generate task_id if not provided
	if task.TaskID == "" {
		task.TaskID = uuid.New().String()
	}

	// Set callback subject if execution ID is available
	// This enables generic async result handling via workflow.step.result subject
	if actx.ExecutionID != "" {
		task.Callback = fmt.Sprintf("workflow.step.result.%s", actx.ExecutionID)
	}

	// Propagate multi-agent hierarchy from action context
	if actx.ParentLoopID != "" {
		task.ParentLoopID = actx.ParentLoopID
	}
	if actx.Depth > 0 || actx.MaxDepth > 0 {
		task.Depth = actx.Depth + 1 // Increment depth for child agent
		task.MaxDepth = actx.MaxDepth
	}

	// Embed pre-constructed context if available
	if actx.EmbeddedContext != nil {
		task.Context = actx.EmbeddedContext
	}

	// Validate using agentic.TaskMessage.Validate()
	if err := task.Validate(); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_agent: invalid task: %v", err),
			Duration: time.Since(start),
		}
	}

	// Check NATS client after validation
	if actx.NATSClient == nil {
		return Result{
			Success:  false,
			Error:    "publish_agent: NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Wrap task in BaseMessage envelope (required by agentic-loop)
	baseMsg := message.NewBaseMessage(task.Schema(), &task, "workflow")
	payload, err := json.Marshal(baseMsg)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_agent: marshal failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Publish to AGENT stream
	if err := actx.NATSClient.PublishToStream(ctx, a.Subject, payload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_agent: send failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Return task_id in output for correlation when agent completes
	output, _ := json.Marshal(map[string]string{"task_id": task.TaskID})
	return Result{
		Success:  true,
		Output:   output,
		Duration: time.Since(start),
	}
}
