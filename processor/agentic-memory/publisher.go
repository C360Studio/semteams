package agenticmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
)

// Publisher defines the interface for publishing messages
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// InjectedContextMessage represents context injected back to the agent loop
type InjectedContextMessage struct {
	LoopID     string `json:"loop_id"`
	Context    string `json:"context"`
	TokenCount int    `json:"token_count"`
	Source     string `json:"source"`    // "post_compaction", "pre_task"
	Timestamp  int64  `json:"timestamp"` // Unix timestamp
}

// GraphMutationMessage represents a graph mutation command
type GraphMutationMessage struct {
	Operation string           `json:"operation"` // "add_triples", "delete_triples"
	LoopID    string           `json:"loop_id"`
	Triples   []message.Triple `json:"triples"`
	Timestamp int64            `json:"timestamp"` // Unix timestamp
}

// CheckpointEventMessage represents a checkpoint creation event
type CheckpointEventMessage struct {
	LoopID       string `json:"loop_id"`
	CheckpointID string `json:"checkpoint_id"`
	Bucket       string `json:"bucket"`
	Timestamp    int64  `json:"timestamp"` // Unix timestamp
}

// publishInjectedContext publishes hydrated context back to the agent loop
func (c *Component) publishInjectedContext(ctx context.Context, loopID, source string, hydrated *HydratedContext) error {
	// Validate inputs
	if loopID == "" {
		return fmt.Errorf("loopID cannot be empty")
	}
	if hydrated == nil {
		return fmt.Errorf("hydrated context cannot be nil")
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Build message
	msg := InjectedContextMessage{
		LoopID:     loopID,
		Context:    hydrated.Context,
		TokenCount: hydrated.TokenCount,
		Source:     source,
		Timestamp:  time.Now().Unix(),
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal injected context message: %w", err)
	}

	// Build subject: agent.context.injected.{loopID}
	subject := fmt.Sprintf("agent.context.injected.%s", loopID)

	// Publish if NATS client available
	if c.natsClient != nil {
		if err := c.natsClient.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("failed to publish injected context: %w", err)
		}
		c.logger.Debug("Published injected context",
			"loop_id", loopID,
			"source", source,
			"subject", subject)
	}

	return nil
}

// publishGraphMutations publishes graph mutation commands
func (c *Component) publishGraphMutations(ctx context.Context, loopID, operation string, triples []message.Triple) error {
	// Validate inputs
	if loopID == "" {
		return fmt.Errorf("loopID cannot be empty")
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Build message
	msg := GraphMutationMessage{
		Operation: operation,
		LoopID:    loopID,
		Triples:   triples,
		Timestamp: time.Now().Unix(),
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal graph mutation message: %w", err)
	}

	// Build subject: graph.mutation.{loopID}
	subject := fmt.Sprintf("graph.mutation.%s", loopID)

	// Publish if NATS client available
	if c.natsClient != nil {
		if err := c.natsClient.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("failed to publish graph mutation: %w", err)
		}
		c.logger.Debug("Published graph mutation",
			"loop_id", loopID,
			"operation", operation,
			"triple_count", len(triples),
			"subject", subject)
	}

	return nil
}

// publishCheckpointEvent publishes a checkpoint creation event
func (c *Component) publishCheckpointEvent(ctx context.Context, loopID, checkpointID string) error {
	// Validate inputs
	if loopID == "" {
		return fmt.Errorf("loopID cannot be empty")
	}
	if checkpointID == "" {
		return fmt.Errorf("checkpointID cannot be empty")
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Build message
	msg := CheckpointEventMessage{
		LoopID:       loopID,
		CheckpointID: checkpointID,
		Bucket:       c.config.Checkpoint.StorageBucket,
		Timestamp:    time.Now().Unix(),
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint event message: %w", err)
	}

	// Build subject: memory.checkpoint.created.{loopID}
	subject := fmt.Sprintf("memory.checkpoint.created.%s", loopID)

	// Publish if NATS client available
	if c.natsClient != nil {
		if err := c.natsClient.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("failed to publish checkpoint event: %w", err)
		}
		c.logger.Debug("Published checkpoint event",
			"loop_id", loopID,
			"checkpoint_id", checkpointID,
			"subject", subject)
	}

	return nil
}
