package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SetStateAction mutates entity state via the graph processor
type SetStateAction struct {
	Entity string
	State  json.RawMessage
}

// NewSetStateAction creates a new set_state action
func NewSetStateAction(entity string, state json.RawMessage) *SetStateAction {
	return &SetStateAction{
		Entity: entity,
		State:  state,
	}
}

// Execute publishes a state mutation to the graph processor
func (a *SetStateAction) Execute(ctx context.Context, actx *Context) Result {
	start := time.Now()

	if actx.NATSClient == nil {
		return Result{
			Success:  false,
			Error:    "NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Build state mutation message
	mutation := map[string]any{
		"entity_id": a.Entity,
		"operation": "set_state",
		"state":     json.RawMessage(a.State),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	payload, err := json.Marshal(mutation)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("failed to marshal mutation: %v", err),
			Duration: time.Since(start),
		}
	}

	// Publish to graph ingest subject
	subject := fmt.Sprintf("graph.entity.%s", a.Entity)
	if err := actx.NATSClient.PublishToStream(ctx, subject, payload); err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("set_state failed: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Duration: time.Since(start),
	}
}
