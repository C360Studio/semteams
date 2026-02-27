package boid

import (
	"context"
	"log/slog"
	"sort"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/processor/rule"
)

// AlignmentRule implements the Boids alignment behavior.
// It steers agents to match the traversal direction of same-role agents
// by suggesting common predicate patterns to follow.
type AlignmentRule struct {
	baseBoidRule

	// positionProvider retrieves other agent positions
	positionProvider PositionProvider

	// pendingSignals holds signals generated during evaluation
	pendingSignals []*SteeringSignal
}

// NewAlignmentRule creates a new alignment rule.
func NewAlignmentRule(id string, def rule.Definition, config *Config, cooldown time.Duration, logger *slog.Logger) *AlignmentRule {
	return &AlignmentRule{
		baseBoidRule: newBaseBoidRule(id, def, config, cooldown, logger),
	}
}

// SetPositionProvider sets the provider for retrieving other agent positions.
func (r *AlignmentRule) SetPositionProvider(provider PositionProvider) {
	r.positionProvider = provider
}

// EvaluateEntityState evaluates the alignment rule against an agent's position.
// Implements the rule.EntityStateEvaluator interface.
func (r *AlignmentRule) EvaluateEntityState(entityState *gtypes.EntityState) bool {
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

	// Get same-role agent positions
	others, err := r.positionProvider.ListOthers(ctx, pos.LoopID)
	if err != nil {
		r.logger.Warn("Failed to list other positions", "error", err)
		return false
	}

	// Filter to same role only
	sameRole := make([]*AgentPosition, 0)
	for _, other := range others {
		if other.Role == pos.Role {
			sameRole = append(sameRole, other)
		}
	}

	if len(sameRole) == 0 {
		// No same-role agents to align with
		return false
	}

	// Find common traversal patterns
	alignWith := r.findCommonTraversalPatterns(pos, sameRole)
	if len(alignWith) == 0 {
		return false
	}

	// Generate alignment signal
	signal := &SteeringSignal{
		LoopID:     pos.LoopID,
		SignalType: SignalTypeAlignment,
		AlignWith:  alignWith,
		Strength:   r.config.SteeringStrength,
		SourceRule: r.id,
		Timestamp:  time.Now(),
		Metadata: map[string]any{
			"same_role_count":  len(sameRole),
			"alignment_window": r.config.AlignmentWindow,
			"pattern_count":    len(alignWith),
		},
	}

	r.pendingSignals = append(r.pendingSignals, signal)
	r.markTriggered()

	r.logger.Info("Alignment rule triggered",
		"loop_id", pos.LoopID,
		"role", pos.Role,
		"same_role_count", len(sameRole),
		"align_patterns", len(alignWith))

	return true
}

// findCommonTraversalPatterns finds predicates commonly used by same-role agents.
func (r *AlignmentRule) findCommonTraversalPatterns(pos *AgentPosition, sameRole []*AgentPosition) []string {
	// Count predicate occurrences across all same-role agents
	predicateCounts := make(map[string]int)
	for _, other := range sameRole {
		for _, predicate := range other.TraversalVector {
			predicateCounts[predicate]++
		}
	}

	if len(predicateCounts) == 0 {
		return nil
	}

	// Convert to sorted list by frequency
	type counted struct {
		predicate string
		count     int
	}
	countedPredicates := make([]counted, 0, len(predicateCounts))
	for pred, count := range predicateCounts {
		countedPredicates = append(countedPredicates, counted{pred, count})
	}

	sort.Slice(countedPredicates, func(i, j int) bool {
		return countedPredicates[i].count > countedPredicates[j].count
	})

	// Return top predicates up to alignment window
	result := make([]string, 0)
	window := r.config.AlignmentWindow
	if window <= 0 {
		window = DefaultAlignmentWindow
	}

	for i, cp := range countedPredicates {
		if i >= window {
			break
		}
		// Only include predicates used by multiple agents
		if cp.count > 1 || len(sameRole) == 1 {
			result = append(result, cp.predicate)
		}
	}

	// Filter out predicates the agent is already following
	currentPredicates := make(map[string]bool)
	for _, p := range pos.TraversalVector {
		currentPredicates[p] = true
	}

	filtered := make([]string, 0)
	for _, p := range result {
		if !currentPredicates[p] {
			filtered = append(filtered, p)
		}
	}

	return filtered
}

// GetPendingSignals returns and clears the pending signals.
func (r *AlignmentRule) GetPendingSignals() []*SteeringSignal {
	signals := r.pendingSignals
	r.pendingSignals = nil
	return signals
}

// Ensure AlignmentRule implements the interfaces.
var (
	_ rule.Rule                 = (*AlignmentRule)(nil)
	_ rule.EntityStateEvaluator = (*AlignmentRule)(nil)
	_ SignalGenerator           = (*AlignmentRule)(nil)
)
