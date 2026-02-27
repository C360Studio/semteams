package boid

import (
	"context"
	"log/slog"
	"sort"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/structural"
	"github.com/c360studio/semstreams/processor/rule"
)

// CohesionRule implements the Boids cohesion behavior.
// It steers agents toward high-centrality nodes in their active subgraph
// that match their role's objective function.
type CohesionRule struct {
	baseBoidRule

	// centralityProvider provides PageRank scores
	centralityProvider CentralityProvider

	// pivotIndex provides reachable candidates
	pivotIndex *structural.PivotIndex

	// pendingSignals holds signals generated during evaluation
	pendingSignals []*SteeringSignal
}

// CentralityProvider provides centrality scores for entities.
type CentralityProvider interface {
	// GetPageRankScores returns PageRank scores for a set of entities.
	// Returns a map of entity ID to score (0.0-1.0).
	GetPageRankScores(ctx context.Context, entityIDs []string) (map[string]float64, error)
}

// NewCohesionRule creates a new cohesion rule.
func NewCohesionRule(id string, def rule.Definition, config *Config, cooldown time.Duration, logger *slog.Logger) *CohesionRule {
	return &CohesionRule{
		baseBoidRule: newBaseBoidRule(id, def, config, cooldown, logger),
	}
}

// SetCentralityProvider sets the provider for centrality scores.
func (r *CohesionRule) SetCentralityProvider(provider CentralityProvider) {
	r.centralityProvider = provider
}

// SetPivotIndex sets the pivot index for finding reachable candidates.
func (r *CohesionRule) SetPivotIndex(index *structural.PivotIndex) {
	r.pivotIndex = index
}

// EvaluateEntityState evaluates the cohesion rule against an agent's position.
// Implements the rule.EntityStateEvaluator interface.
func (r *CohesionRule) EvaluateEntityState(entityState *gtypes.EntityState) bool {
	if !r.enabled || entityState == nil {
		return false
	}

	// Check cooldown
	if !r.canTrigger() {
		return false
	}

	// Extract agent position from entity state
	pos, err := extractAgentPosition(entityState)
	if err != nil {
		r.logger.Debug("Failed to extract position", "entity_id", entityState.ID, "error", err)
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

	// Find high-centrality candidates
	candidates := r.findHighCentralityCandidates(pos)
	if len(candidates) == 0 {
		return false
	}

	// Generate cohesion signal
	signal := &SteeringSignal{
		LoopID:         pos.LoopID,
		SignalType:     SignalTypeCohesion,
		SuggestedFocus: candidates,
		Strength:       r.config.SteeringStrength,
		SourceRule:     r.id,
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			"candidate_count":   len(candidates),
			"centrality_weight": r.config.CentralityWeight,
		},
	}

	r.pendingSignals = append(r.pendingSignals, signal)
	r.markTriggered()

	r.logger.Info("Cohesion rule triggered",
		"loop_id", pos.LoopID,
		"role", pos.Role,
		"candidates", len(candidates))

	return true
}

// findHighCentralityCandidates finds entities with high centrality near the agent's focus.
func (r *CohesionRule) findHighCentralityCandidates(pos *AgentPosition) []string {
	ctx := context.Background()

	// Get reachable candidates from each focus entity
	candidates := make(map[string]bool)
	for _, focus := range pos.FocusEntities {
		// Default search radius: 3 hops
		searchRadius := 3

		if r.pivotIndex != nil {
			reachable := r.pivotIndex.GetReachableCandidates(focus, searchRadius)
			for _, id := range reachable {
				candidates[id] = true
			}
		} else {
			// Without pivot index, just include current focus
			candidates[focus] = true
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Convert to slice
	candidateList := make([]string, 0, len(candidates))
	for id := range candidates {
		candidateList = append(candidateList, id)
	}

	// Get centrality scores
	if r.centralityProvider == nil {
		// Without centrality provider, return candidates sorted by ID (deterministic)
		sort.Strings(candidateList)
		if len(candidateList) > 5 {
			candidateList = candidateList[:5]
		}
		return candidateList
	}

	scores, err := r.centralityProvider.GetPageRankScores(ctx, candidateList)
	if err != nil {
		r.logger.Warn("Failed to get centrality scores", "error", err)
		return nil
	}

	// Filter by centrality weight threshold and sort by score
	type scored struct {
		id    string
		score float64
	}
	scoredCandidates := make([]scored, 0)
	for id, score := range scores {
		// Apply centrality weight as threshold
		if score >= r.config.CentralityWeight*0.1 { // Scale threshold
			scoredCandidates = append(scoredCandidates, scored{id, score})
		}
	}

	// Sort by score descending
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	// Return top candidates (up to 5)
	result := make([]string, 0)
	for i, sc := range scoredCandidates {
		if i >= 5 {
			break
		}
		result = append(result, sc.id)
	}

	return result
}

// GetPendingSignals returns and clears the pending signals.
func (r *CohesionRule) GetPendingSignals() []*SteeringSignal {
	signals := r.pendingSignals
	r.pendingSignals = nil
	return signals
}

// Ensure CohesionRule implements the interfaces.
var (
	_ rule.Rule                 = (*CohesionRule)(nil)
	_ rule.EntityStateEvaluator = (*CohesionRule)(nil)
	_ SignalGenerator           = (*CohesionRule)(nil)
)
