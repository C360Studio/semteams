package boid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// PositionTracker manages agent positions in the AGENT_POSITIONS KV bucket.
type PositionTracker struct {
	kv     jetstream.KeyValue
	logger *slog.Logger
}

// NewPositionTracker creates a new position tracker.
func NewPositionTracker(kv jetstream.KeyValue, logger *slog.Logger) *PositionTracker {
	if logger == nil {
		logger = slog.Default()
	}
	return &PositionTracker{
		kv:     kv,
		logger: logger,
	}
}

// Get retrieves an agent's position by loop ID.
func (t *PositionTracker) Get(ctx context.Context, loopID string) (*AgentPosition, error) {
	entry, err := t.kv.Get(ctx, loopID)
	if err != nil {
		return nil, fmt.Errorf("get position for %s: %w", loopID, err)
	}

	var pos AgentPosition
	if err := json.Unmarshal(entry.Value(), &pos); err != nil {
		return nil, fmt.Errorf("unmarshal position for %s: %w", loopID, err)
	}

	return &pos, nil
}

// Put stores an agent's position.
func (t *PositionTracker) Put(ctx context.Context, pos *AgentPosition) (uint64, error) {
	if pos == nil {
		return 0, fmt.Errorf("position cannot be nil")
	}

	// Update timestamp
	pos.LastUpdate = time.Now()

	data, err := json.Marshal(pos)
	if err != nil {
		return 0, fmt.Errorf("marshal position for %s: %w", pos.LoopID, err)
	}

	revision, err := t.kv.Put(ctx, pos.LoopID, data)
	if err != nil {
		return 0, fmt.Errorf("put position for %s: %w", pos.LoopID, err)
	}

	t.logger.Debug("Position updated",
		"loop_id", pos.LoopID,
		"role", pos.Role,
		"focus_entities", len(pos.FocusEntities),
		"revision", revision)

	return revision, nil
}

// Delete removes an agent's position.
func (t *PositionTracker) Delete(ctx context.Context, loopID string) error {
	if err := t.kv.Delete(ctx, loopID); err != nil {
		return fmt.Errorf("delete position for %s: %w", loopID, err)
	}

	t.logger.Debug("Position deleted", "loop_id", loopID)
	return nil
}

// ListAll returns all agent positions.
func (t *PositionTracker) ListAll(ctx context.Context) ([]*AgentPosition, error) {
	keys, err := t.kv.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list position keys: %w", err)
	}

	positions := make([]*AgentPosition, 0, len(keys))
	for _, key := range keys {
		pos, err := t.Get(ctx, key)
		if err != nil {
			t.logger.Warn("Failed to get position", "key", key, "error", err)
			continue
		}
		positions = append(positions, pos)
	}

	return positions, nil
}

// ListByRole returns all agent positions with a specific role.
func (t *PositionTracker) ListByRole(ctx context.Context, role string) ([]*AgentPosition, error) {
	all, err := t.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]*AgentPosition, 0)
	for _, pos := range all {
		if pos.Role == role {
			filtered = append(filtered, pos)
		}
	}

	return filtered, nil
}

// ListOthers returns all agent positions except the specified loop ID.
func (t *PositionTracker) ListOthers(ctx context.Context, excludeLoopID string) ([]*AgentPosition, error) {
	all, err := t.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]*AgentPosition, 0)
	for _, pos := range all {
		if pos.LoopID != excludeLoopID {
			filtered = append(filtered, pos)
		}
	}

	return filtered, nil
}

// UpdateFocusEntities updates the focus entities for an agent.
func (t *PositionTracker) UpdateFocusEntities(ctx context.Context, loopID string, entities []string) error {
	pos, err := t.Get(ctx, loopID)
	if err != nil {
		// Create new position if doesn't exist
		pos = &AgentPosition{
			LoopID: loopID,
		}
	}

	pos.FocusEntities = entities
	_, err = t.Put(ctx, pos)
	return err
}

// UpdateTraversalVector updates the traversal vector for an agent.
func (t *PositionTracker) UpdateTraversalVector(ctx context.Context, loopID string, predicates []string) error {
	pos, err := t.Get(ctx, loopID)
	if err != nil {
		// Create new position if doesn't exist
		pos = &AgentPosition{
			LoopID: loopID,
		}
	}

	pos.TraversalVector = predicates
	_, err = t.Put(ctx, pos)
	return err
}

// CalculateVelocity computes velocity based on position changes.
// Velocity is a normalized measure of how much the focus entities changed.
func CalculateVelocity(oldFocus, newFocus []string) float64 {
	if len(oldFocus) == 0 && len(newFocus) == 0 {
		return 0.0
	}

	// Count entities that are in newFocus but not in oldFocus
	oldSet := make(map[string]bool)
	for _, e := range oldFocus {
		oldSet[e] = true
	}

	changed := 0
	for _, e := range newFocus {
		if !oldSet[e] {
			changed++
		}
	}

	// Also count entities that were removed
	newSet := make(map[string]bool)
	for _, e := range newFocus {
		newSet[e] = true
	}
	for _, e := range oldFocus {
		if !newSet[e] {
			changed++
		}
	}

	// Normalize to 0.0-1.0 range
	total := len(oldFocus) + len(newFocus)
	if total == 0 {
		return 0.0
	}

	velocity := float64(changed) / float64(total)
	if velocity > 1.0 {
		velocity = 1.0
	}
	return velocity
}
