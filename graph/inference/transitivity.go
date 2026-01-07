package inference

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/graph/structural"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/google/uuid"
)

// TransitivityDetector detects missing transitive relationships.
// When A->B and B->C exist for transitive predicates, checks if A-C distance
// is greater than expected, suggesting a missing A->C relationship.
type TransitivityDetector struct {
	config TransitivityConfig
	deps   *DetectorDependencies
	logger *slog.Logger
}

// NewTransitivityDetector creates a new transitivity gap detector.
func NewTransitivityDetector(deps *DetectorDependencies) *TransitivityDetector {
	logger := slog.Default()
	if deps != nil && deps.Logger != nil {
		logger = deps.Logger
	}

	return &TransitivityDetector{
		config: DefaultConfig().Transitivity,
		deps:   deps,
		logger: logger,
	}
}

// Name returns the detector identifier.
func (d *TransitivityDetector) Name() string {
	return "transitivity"
}

// Configure updates the detector configuration.
func (d *TransitivityDetector) Configure(config interface{}) error {
	if cfg, ok := config.(TransitivityConfig); ok {
		d.config = cfg
		return nil
	}
	return errs.WrapInvalid(errs.ErrInvalidConfig, "TransitivityDetector", "Configure",
		"expected TransitivityConfig")
}

// SetDependencies updates the detector's dependencies.
func (d *TransitivityDetector) SetDependencies(deps *DetectorDependencies) {
	d.deps = deps
	if deps != nil && deps.Logger != nil {
		d.logger = deps.Logger
	}
}

// Detect finds transitivity gaps in the graph.
func (d *TransitivityDetector) Detect(ctx context.Context) ([]*StructuralAnomaly, error) {
	if !d.config.Enabled {
		return nil, nil
	}

	if err := d.validateDependencies(); err != nil {
		return nil, err
	}

	if len(d.config.TransitivePredicates) == 0 {
		d.logger.Debug("no transitive predicates configured, skipping")
		return nil, nil
	}

	pivotIndex := d.deps.StructuralIndices.Pivot
	if pivotIndex == nil {
		d.logger.Warn("pivot index not available, skipping transitivity detection")
		return nil, nil
	}

	anomalies := make([]*StructuralAnomaly, 0)
	seen := make(map[string]bool) // Track seen A-C pairs

	// Process each transitive predicate
	for _, predicate := range d.config.TransitivePredicates {
		select {
		case <-ctx.Done():
			return anomalies, ctx.Err()
		default:
		}

		predicateAnomalies, err := d.detectGapsForPredicate(ctx, predicate, pivotIndex, seen)
		if err != nil {
			d.logger.Warn("failed to detect gaps for predicate", "predicate", predicate, "error", err)
			continue
		}
		anomalies = append(anomalies, predicateAnomalies...)
	}

	d.logger.Info("transitivity detection complete", "anomalies", len(anomalies))
	return anomalies, nil
}

// validateDependencies ensures required dependencies are available.
func (d *TransitivityDetector) validateDependencies() error {
	if d.deps == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "TransitivityDetector", "validateDependencies",
			"dependencies not set")
	}
	if d.deps.StructuralIndices == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "TransitivityDetector", "validateDependencies",
			"structural indices not available")
	}
	if d.deps.RelationshipQuerier == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "TransitivityDetector", "validateDependencies",
			"relationship querier not available")
	}
	return nil
}

// detectGapsForPredicate finds transitivity gaps for a specific predicate.
func (d *TransitivityDetector) detectGapsForPredicate(
	ctx context.Context,
	predicate string,
	pivotIndex *structural.PivotIndex,
	seen map[string]bool,
) ([]*StructuralAnomaly, error) {
	anomalies := make([]*StructuralAnomaly, 0)

	// Get all entities from pivot index as starting points
	entities := d.getEntitiesFromIndex(pivotIndex)

	for _, entityA := range entities {
		select {
		case <-ctx.Done():
			return anomalies, ctx.Err()
		default:
		}

		// Find chains starting from entityA with the transitive predicate
		chains := d.findTransitiveChains(ctx, entityA, predicate)

		for _, chain := range chains {
			entityC := chain[len(chain)-1]

			// Skip if we've already seen this A-C pair
			pairKey := makePairKey(entityA, entityC)
			if seen[pairKey] {
				continue
			}
			seen[pairKey] = true

			// Check A-C structural distance
			lower, upper := pivotIndex.EstimateDistance(entityA, entityC)

			// If A-C distance is greater than expected transitivity, flag it
			if lower > d.config.MinExpectedTransitivity {
				anomaly := d.createTransitivityAnomaly(entityA, entityC, predicate, chain, lower, upper)
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	return anomalies, nil
}

// getEntitiesFromIndex extracts entity IDs from the pivot index.
func (d *TransitivityDetector) getEntitiesFromIndex(pivotIndex *structural.PivotIndex) []string {
	entities := make([]string, 0, len(pivotIndex.DistanceVectors))
	for entityID := range pivotIndex.DistanceVectors {
		entities = append(entities, entityID)
	}
	return entities
}

// findTransitiveChains finds A->B->...->C chains using the specified predicate.
func (d *TransitivityDetector) findTransitiveChains(
	ctx context.Context,
	startEntity string,
	predicate string,
) [][]string {
	chains := make([][]string, 0)

	// BFS to find chains up to maxIntermediateHops
	type queueItem struct {
		entityID string
		path     []string
		depth    int
	}

	queue := []queueItem{{entityID: startEntity, path: []string{startEntity}, depth: 0}}
	visited := make(map[string]bool)
	visited[startEntity] = true

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return chains
		default:
		}

		item := queue[0]
		queue = queue[1:]

		// Get outgoing relationships with the transitive predicate
		relationships, err := d.deps.RelationshipQuerier.GetOutgoingRelationships(ctx, item.entityID)
		if err != nil {
			continue
		}

		for _, rel := range relationships {
			// Only follow edges with the transitive predicate
			if rel.Predicate != predicate {
				continue
			}

			targetID := rel.ToEntityID
			if visited[targetID] {
				continue
			}

			newPath := append(append([]string{}, item.path...), targetID)
			newDepth := item.depth + 1

			// If we've found a chain with at least 2 hops (A->B->C), record it
			if len(newPath) >= 3 {
				chains = append(chains, newPath)
			}

			// Continue exploring if under max depth
			if newDepth < d.config.MaxIntermediateHops {
				visited[targetID] = true
				queue = append(queue, queueItem{entityID: targetID, path: newPath, depth: newDepth})
			}
		}
	}

	return chains
}

// createTransitivityAnomaly creates an anomaly for a transitivity gap.
func (d *TransitivityDetector) createTransitivityAnomaly(
	entityA, entityC, predicate string,
	chainPath []string,
	distanceLower, distanceUpper int,
) *StructuralAnomaly {
	now := time.Now()

	// Confidence based on how far the actual distance exceeds expected
	excess := distanceLower - d.config.MinExpectedTransitivity
	confidence := 0.5 + float64(excess)*0.1
	if confidence > 0.9 {
		confidence = 0.9 // Cap at 0.9 since transitivity isn't always appropriate
	}

	evidence := Evidence{
		Predicate:          predicate,
		ChainPath:          chainPath,
		ActualDistance:     distanceLower,
		ExpectedMaxHops:    d.config.MinExpectedTransitivity,
		DistanceLowerBound: distanceLower,
		DistanceUpperBound: distanceUpper,
	}

	reasoning := fmt.Sprintf(
		"Transitive chain exists via %s (%d hops) but A-C distance is %d+ (expected ≤%d)",
		predicate, len(chainPath)-1, distanceLower, d.config.MinExpectedTransitivity,
	)

	return &StructuralAnomaly{
		ID:         uuid.New().String(),
		Type:       AnomalyTransitivityGap,
		EntityA:    entityA,
		EntityB:    entityC,
		Confidence: confidence,
		Evidence:   evidence,
		Suggestion: &RelationshipSuggestion{
			FromEntity: entityA,
			ToEntity:   entityC,
			Predicate:  predicate,
			Confidence: confidence,
			Reasoning:  reasoning,
		},
		Status:     StatusPending,
		DetectedAt: now,
	}
}
