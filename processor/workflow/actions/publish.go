package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
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
			Error:    "NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Prepare payload
	payload := a.Payload
	if payload == nil {
		payload = []byte("{}")
	}

	// Publish to JetStream for durability
	if err := actx.NATSClient.PublishToStream(ctx, a.Subject, payload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish failed: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Duration: time.Since(start),
	}
}

// PublishAgentAction publishes a task to an agent
type PublishAgentAction struct {
	Subject string
	Payload json.RawMessage
}

// NewPublishAgentAction creates a new publish_agent action
func NewPublishAgentAction(subject string, payload json.RawMessage) *PublishAgentAction {
	return &PublishAgentAction{
		Subject: subject,
		Payload: payload,
	}
}

// Execute publishes the agent task message
func (a *PublishAgentAction) Execute(ctx context.Context, actx *Context) Result {
	start := time.Now()

	if actx.NATSClient == nil {
		return Result{
			Success:  false,
			Error:    "NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Prepare payload
	payload := a.Payload
	if payload == nil {
		payload = []byte("{}")
	}

	// Publish to AGENT stream
	if err := actx.NATSClient.PublishToStream(ctx, a.Subject, payload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("publish_agent failed: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Duration: time.Since(start),
	}
}
