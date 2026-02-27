package boid

import (
	"context"
	"log/slog"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/structural"
	"github.com/c360studio/semstreams/processor/rule"
)

// SeparationRule implements the Boids separation behavior.
// It prevents agents from working on overlapping graph neighborhoods
// by generating avoid signals when agents are within k-hop distance.
type SeparationRule struct {
	baseBoidRule

	// positionProvider retrieves other agent positions
	positionProvider PositionProvider

	// pivotIndex provides k-hop distance estimation
	pivotIndex *structural.PivotIndex

	// pendingSignals holds signals generated during evaluation
	pendingSignals []*SteeringSignal
}

// PositionProvider retrieves agent positions for rule evaluation.
type PositionProvider interface {
	Get(ctx context.Context, loopID string) (*AgentPosition, error)
	ListOthers(ctx context.Context, excludeLoopID string) ([]*AgentPosition, error)
}

// NewSeparationRule creates a new separation rule.
func NewSeparationRule(id string, def rule.Definition, config *Config, cooldown time.Duration, logger *slog.Logger) *SeparationRule {
	return &SeparationRule{
		baseBoidRule: newBaseBoidRule(id, def, config, cooldown, logger),
	}
}

// SetPositionProvider sets the provider for retrieving other agent positions.
func (r *SeparationRule) SetPositionProvider(provider PositionProvider) {
	r.positionProvider = provider
}

// SetPivotIndex sets the pivot index for k-hop distance estimation.
func (r *SeparationRule) SetPivotIndex(index *structural.PivotIndex) {
	r.pivotIndex = index
}

// EvaluateEntityState evaluates the separation rule against an agent's position.
// Implements the rule.EntityStateEvaluator interface.
func (r *SeparationRule) EvaluateEntityState(entityState *gtypes.EntityState) bool {
	if !r.enabled || entityState == nil {
		return false
	}

	// Check cooldown
	if !r.canTrigger() {
		return false
	}

	// Check dependencies first
	if r.positionProvider == nil {
		r.logger.Debug("No position provider configured", "rule", r.name)
		return false
	}

	// Get agent position using provider (reads flat JSON directly from KV)
	ctx := context.Background()
	pos, err := r.positionProvider.Get(ctx, entityState.ID)
	if err != nil || pos == nil {
		r.logger.Debug("Failed to get position", "entity_id", entityState.ID, "error", err)
		return false
	}

	// Check role filter
	if !r.matchesRoleFilter(pos.Role) {
		return false
	}

	// Skip if no focus entities
	if len(pos.FocusEntities) == 0 {
		return false
	}

	// Get other agent positions
	others, err := r.positionProvider.ListOthers(ctx, pos.LoopID)
	if err != nil {
		r.logger.Warn("Failed to list other positions", "error", err)
		return false
	}

	// Find entities to avoid
	avoidEntities := r.findOverlappingEntities(pos, others)
	if len(avoidEntities) == 0 {
		return false
	}

	// Generate separation signal
	signal := &SteeringSignal{
		LoopID:        pos.LoopID,
		SignalType:    SignalTypeSeparation,
		AvoidEntities: avoidEntities,
		Strength:      r.config.SteeringStrength,
		SourceRule:    r.id,
		Timestamp:     time.Now(),
		Metadata: map[string]any{
			"overlapping_count": len(avoidEntities),
			"threshold":         r.config.GetSeparationThreshold(pos.Role),
		},
	}

	r.pendingSignals = append(r.pendingSignals, signal)
	r.markTriggered()

	r.logger.Info("Separation rule triggered",
		"loop_id", pos.LoopID,
		"role", pos.Role,
		"avoid_count", len(avoidEntities),
		"threshold", r.config.GetSeparationThreshold(pos.Role))

	return true
}

// findOverlappingEntities finds entities that are within k-hop distance of other agents.
func (r *SeparationRule) findOverlappingEntities(pos *AgentPosition, others []*AgentPosition) []string {
	threshold := r.config.GetSeparationThreshold(pos.Role)
	overlapping := make(map[string]bool)

	for _, other := range others {
		// Skip agents with no focus entities
		if len(other.FocusEntities) == 0 {
			continue
		}

		// Check each pair of focus entities for proximity
		for _, myEntity := range pos.FocusEntities {
			for _, otherEntity := range other.FocusEntities {
				if r.areEntitiesWithinRange(myEntity, otherEntity, threshold) {
					// Mark my entity as overlapping - I should avoid it
					overlapping[myEntity] = true
				}
			}
		}
	}

	result := make([]string, 0, len(overlapping))
	for entity := range overlapping {
		result = append(result, entity)
	}
	return result
}

// areEntitiesWithinRange checks if two entities are within k-hop distance.
func (r *SeparationRule) areEntitiesWithinRange(entityA, entityB string, maxHops int) bool {
	if entityA == entityB {
		return true // Same entity is always within range
	}

	if r.pivotIndex == nil {
		// Without pivot index, fall back to string comparison (same prefix = related)
		// This is a conservative approximation
		return false
	}

	return r.pivotIndex.IsWithinHops(entityA, entityB, maxHops)
}

// GetPendingSignals returns and clears the pending signals.
// This is used by the rule processor to retrieve generated signals.
func (r *SeparationRule) GetPendingSignals() []*SteeringSignal {
	signals := r.pendingSignals
	r.pendingSignals = nil
	return signals
}

// SignalGenerator interface for rules that generate boid signals.
type SignalGenerator interface {
	GetPendingSignals() []*SteeringSignal
}

// Ensure SeparationRule implements the interfaces.
var (
	_ rule.Rule                 = (*SeparationRule)(nil)
	_ rule.EntityStateEvaluator = (*SeparationRule)(nil)
	_ SignalGenerator           = (*SeparationRule)(nil)
)
