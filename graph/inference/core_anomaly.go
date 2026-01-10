package inference

import (
	"context"
	"log/slog"
	"time"

	"github.com/c360/semstreams/graph/structural"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/google/uuid"
)

// CoreAnomalyDetector detects k-core based structural anomalies:
// - Core isolation: high k-core entities with few same-core peers
// - Core demotion: entities that dropped k-core level between computations
type CoreAnomalyDetector struct {
	config CoreAnomalyConfig
	deps   *DetectorDependencies
	logger *slog.Logger
}

// NewCoreAnomalyDetector creates a new core anomaly detector.
func NewCoreAnomalyDetector(deps *DetectorDependencies) *CoreAnomalyDetector {
	logger := slog.Default()
	if deps != nil && deps.Logger != nil {
		logger = deps.Logger
	}

	return &CoreAnomalyDetector{
		config: DefaultConfig().CoreAnomaly,
		deps:   deps,
		logger: logger,
	}
}

// Name returns the detector identifier.
func (d *CoreAnomalyDetector) Name() string {
	return "core_anomaly"
}

// Configure updates the detector configuration.
func (d *CoreAnomalyDetector) Configure(config interface{}) error {
	if cfg, ok := config.(CoreAnomalyConfig); ok {
		d.config = cfg
		return nil
	}
	return errs.WrapInvalid(errs.ErrInvalidConfig, "CoreAnomalyDetector", "Configure",
		"expected CoreAnomalyConfig")
}

// SetDependencies updates the detector's dependencies.
func (d *CoreAnomalyDetector) SetDependencies(deps *DetectorDependencies) {
	d.deps = deps
	if deps != nil && deps.Logger != nil {
		d.logger = deps.Logger
	}
}

// Detect finds core isolation and demotion anomalies.
func (d *CoreAnomalyDetector) Detect(ctx context.Context) ([]*StructuralAnomaly, error) {
	if !d.config.Enabled {
		return nil, nil
	}

	if err := d.validateDependencies(); err != nil {
		return nil, err
	}

	kcore := d.deps.StructuralIndices.KCore
	if kcore == nil {
		d.logger.Warn("k-core index not available, skipping core anomaly detection")
		return nil, nil
	}

	anomalies := make([]*StructuralAnomaly, 0)

	// Detect core isolation
	isolationAnomalies, err := d.detectCoreIsolation(ctx, kcore)
	if err != nil {
		d.logger.Warn("core isolation detection failed", "error", err)
	} else {
		anomalies = append(anomalies, isolationAnomalies...)
	}

	// Detect core demotion (if previous index available)
	if d.config.TrackCoreDemotions && d.deps.PreviousKCore != nil {
		demotionAnomalies, err := d.detectCoreDemotion(ctx, kcore, d.deps.PreviousKCore)
		if err != nil {
			d.logger.Warn("core demotion detection failed", "error", err)
		} else {
			anomalies = append(anomalies, demotionAnomalies...)
		}
	}

	d.logger.Info("core anomaly detection complete", "anomalies", len(anomalies))
	return anomalies, nil
}

// validateDependencies ensures required dependencies are available.
func (d *CoreAnomalyDetector) validateDependencies() error {
	if d.deps == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "CoreAnomalyDetector", "validateDependencies",
			"dependencies not set")
	}
	if d.deps.StructuralIndices == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "CoreAnomalyDetector", "validateDependencies",
			"structural indices not available")
	}
	return nil
}

// detectCoreIsolation finds high-core entities with low peer connectivity.
func (d *CoreAnomalyDetector) detectCoreIsolation(
	ctx context.Context,
	kcore *structural.KCoreIndex,
) ([]*StructuralAnomaly, error) {
	anomalies := make([]*StructuralAnomaly, 0)

	// Get entities in cores >= minCoreForHubAnalysis
	minCore := d.config.MinCoreForHubAnalysis
	highCoreEntities := kcore.GetEntitiesAboveCore(minCore)

	d.logger.Debug("analyzing high-core entities for isolation",
		"min_core", minCore, "count", len(highCoreEntities))

	for _, entityID := range highCoreEntities {
		select {
		case <-ctx.Done():
			return anomalies, ctx.Err()
		default:
		}

		coreLevel := kcore.GetCore(entityID)

		// Count same-core peers
		peerCount, expectedPeers := d.countSameCorePeers(ctx, entityID, coreLevel, kcore)

		// Calculate peer connectivity ratio
		connectivity := 0.0
		if expectedPeers > 0 {
			connectivity = float64(peerCount) / float64(expectedPeers)
		}

		// Check if isolated (below threshold)
		if connectivity < d.config.HubIsolationThreshold {
			anomaly := d.createIsolationAnomaly(entityID, coreLevel, peerCount, expectedPeers, connectivity)
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies, nil
}

// countSameCorePeers counts how many unique same-core neighbors an entity has.
func (d *CoreAnomalyDetector) countSameCorePeers(
	ctx context.Context,
	entityID string,
	coreLevel int,
	kcore *structural.KCoreIndex,
) (actual, expected int) {
	// If relationship querier is available, use it for accurate count
	if d.deps.RelationshipQuerier != nil {
		// Track unique peer IDs to avoid double-counting bidirectional edges
		peers := make(map[string]bool)

		outgoing, err := d.deps.RelationshipQuerier.GetOutgoingRelationships(ctx, entityID)
		if err == nil {
			for _, rel := range outgoing {
				neighborCore := kcore.GetCore(rel.ToEntityID)
				if neighborCore >= coreLevel {
					peers[rel.ToEntityID] = true
				}
			}
		}

		incoming, err := d.deps.RelationshipQuerier.GetIncomingRelationships(ctx, entityID)
		if err == nil {
			for _, rel := range incoming {
				neighborCore := kcore.GetCore(rel.FromEntityID)
				if neighborCore >= coreLevel {
					peers[rel.FromEntityID] = true
				}
			}
		}

		actual = len(peers)
	}

	// Expected peers based on core level definition:
	// An entity in core k should have at least k neighbors also in core k
	expected = coreLevel

	return actual, expected
}

// createIsolationAnomaly creates an anomaly for core isolation.
func (d *CoreAnomalyDetector) createIsolationAnomaly(
	entityID string,
	coreLevel, peerCount, expectedPeers int,
	connectivity float64,
) *StructuralAnomaly {
	now := time.Now()

	// Confidence based on how severe the isolation is
	// Lower connectivity = higher confidence something is wrong
	confidence := 1.0 - connectivity
	if confidence < 0.3 {
		confidence = 0.3 // Minimum confidence for detected anomalies
	}

	evidence := Evidence{
		CoreLevel:         coreLevel,
		PeerCount:         peerCount,
		ExpectedPeerCount: expectedPeers,
		PeerConnectivity:  connectivity,
	}

	return &StructuralAnomaly{
		ID:         uuid.New().String(),
		Type:       AnomalyCoreIsolation,
		EntityA:    entityID,
		Confidence: confidence,
		Evidence:   evidence,
		Status:     StatusPending,
		DetectedAt: now,
	}
}

// detectCoreDemotion finds entities that dropped k-core level.
func (d *CoreAnomalyDetector) detectCoreDemotion(
	ctx context.Context,
	current, previous *structural.KCoreIndex,
) ([]*StructuralAnomaly, error) {
	anomalies := make([]*StructuralAnomaly, 0)

	// Check all entities that were in the previous index
	for entityID, prevCore := range previous.CoreNumbers {
		select {
		case <-ctx.Done():
			return anomalies, ctx.Err()
		default:
		}

		currCore := current.GetCore(entityID)

		// Check for demotion
		delta := prevCore - currCore
		if delta >= d.config.MinDemotionDelta {
			anomaly := d.createDemotionAnomaly(entityID, prevCore, currCore, delta)
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies, nil
}

// createDemotionAnomaly creates an anomaly for core demotion.
func (d *CoreAnomalyDetector) createDemotionAnomaly(
	entityID string,
	previousCore, currentCore, delta int,
) *StructuralAnomaly {
	now := time.Now()

	// Confidence based on magnitude of demotion
	// Larger drops are more significant
	confidence := 0.5 + float64(delta)*0.1
	if confidence > 1.0 {
		confidence = 1.0
	}

	evidence := Evidence{
		PreviousCoreLevel: previousCore,
		CurrentCoreLevel:  currentCore,
		LostConnections:   delta, // Approximation: lost at least delta connections
	}

	return &StructuralAnomaly{
		ID:         uuid.New().String(),
		Type:       AnomalyCoreDemotion,
		EntityA:    entityID,
		Confidence: confidence,
		Evidence:   evidence,
		Status:     StatusPending,
		DetectedAt: now,
	}
}
