// Package graphclustering provides anomaly detection integration for graph-clustering.
package graphclustering

import (
	"context"
	"log/slog"
	"time"

	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/clustering"
	"github.com/c360/semstreams/graph/inference"
	"github.com/c360/semstreams/graph/structural"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// graphProviderAdapter wraps Provider to implement inference.RelationshipQuerier.
// This allows core anomaly detection to query graph relationships for peer counting.
type graphProviderAdapter struct {
	provider clustering.Provider
}

func (a *graphProviderAdapter) GetOutgoingRelationships(ctx context.Context, entityID string) ([]inference.RelationshipInfo, error) {
	neighbors, err := a.provider.GetNeighbors(ctx, entityID, "outgoing")
	if err != nil {
		return nil, err
	}
	result := make([]inference.RelationshipInfo, len(neighbors))
	for i, n := range neighbors {
		result[i] = inference.RelationshipInfo{
			FromEntityID: entityID,
			ToEntityID:   n,
		}
	}
	return result, nil
}

func (a *graphProviderAdapter) GetIncomingRelationships(ctx context.Context, entityID string) ([]inference.RelationshipInfo, error) {
	neighbors, err := a.provider.GetNeighbors(ctx, entityID, "incoming")
	if err != nil {
		return nil, err
	}
	result := make([]inference.RelationshipInfo, len(neighbors))
	for i, n := range neighbors {
		result[i] = inference.RelationshipInfo{
			FromEntityID: n,
			ToEntityID:   entityID,
		}
	}
	return result, nil
}

// initAnomalyDetection initializes anomaly detection resources.
// Called during Start() when EnableAnomalyDetection is true and structural is initialized.
func (c *Component) initAnomalyDetection(ctx context.Context) error {
	// Create ANOMALY_INDEX bucket (we are the WRITER)
	anomalyBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketAnomalyIndex,
		Description: "Anomaly detection index for structural gaps and inferences",
	})
	if err != nil {
		if ctx.Err() != nil {
			return errs.Wrap(ctx.Err(), "Component", "initAnomalyDetection", "context cancelled during bucket creation")
		}
		return errs.Wrap(err, "Component", "initAnomalyDetection", "create ANOMALY_INDEX bucket")
	}
	c.anomalyBucket = anomalyBucket

	// Create anomaly storage
	anomalyStorage := inference.NewNATSAnomalyStorage(anomalyBucket, c.logger)
	c.anomalyStorage = anomalyStorage

	// Create orchestrator
	orchestrator, err := inference.NewOrchestrator(inference.OrchestratorConfig{
		Config:  c.config.AnomalyConfig,
		Storage: anomalyStorage,
		Logger:  c.logger,
	})
	if err != nil {
		return errs.Wrap(err, "Component", "initAnomalyDetection", "create orchestrator")
	}
	c.anomalyOrchestrator = orchestrator

	// Register enabled detectors
	if err := c.registerAnomalyDetectors(); err != nil {
		return errs.Wrap(err, "Component", "initAnomalyDetection", "register detectors")
	}

	// Initialize similarity finder for semantic gap detection (optional)
	// Uses query path to graph-embedding instead of direct KV access
	if c.config.AnomalyConfig.SemanticGap.Enabled {
		finder := c.initQuerySimilarityFinder()
		if finder != nil {
			c.similarityFinder = finder
		}
	}

	c.logger.Info("anomaly detection initialized",
		slog.Any("enabled_detectors", c.config.AnomalyConfig.GetEnabledDetectors()),
		slog.Int("max_anomalies_per_run", c.config.AnomalyConfig.MaxAnomaliesPerRun),
		slog.Bool("similarity_finder_available", c.similarityFinder != nil))

	return nil
}

// registerAnomalyDetectors registers enabled detectors with the orchestrator.
func (c *Component) registerAnomalyDetectors() error {
	cfg := c.config.AnomalyConfig

	// Register core anomaly detector (k-core based)
	// Dependencies will be set via SetDependencies() before each detection run
	if cfg.CoreAnomaly.Enabled {
		coreDetector := inference.NewCoreAnomalyDetector(nil)
		if err := coreDetector.Configure(cfg.CoreAnomaly); err != nil {
			return errs.Wrap(err, "Component", "registerAnomalyDetectors", "configure core anomaly detector")
		}
		c.anomalyOrchestrator.RegisterDetector(coreDetector)
		c.logger.Debug("registered core anomaly detector")
	}

	// Semantic gap detector is registered but requires SimilarityFinder at runtime
	// The detector will skip if SimilarityFinder is not available in dependencies
	if cfg.SemanticGap.Enabled {
		semanticDetector := inference.NewSemanticGapDetector(nil)
		if err := semanticDetector.Configure(cfg.SemanticGap); err != nil {
			return errs.Wrap(err, "Component", "registerAnomalyDetectors", "configure semantic gap detector")
		}
		c.anomalyOrchestrator.RegisterDetector(semanticDetector)
		c.logger.Debug("registered semantic gap detector")
	}

	// Note: Transitivity detector requires RelationshipQuerier (future work)
	if cfg.Transitivity.Enabled {
		c.logger.Warn("transitivity detector enabled but RelationshipQuerier not yet wired - skipping")
	}

	return nil
}

// runAnomalyDetection runs anomaly detection using the current structural indices.
// Called after structural computation completes when EnableAnomalyDetection is true.
func (c *Component) runAnomalyDetection(ctx context.Context, kcoreIndex *structural.KCoreIndex, pivotIndex *structural.PivotIndex) error {
	if c.anomalyOrchestrator == nil {
		return nil // Not initialized
	}

	c.logger.Info("running anomaly detection")
	start := time.Now()

	// Build structural indices bundle
	indices := &structural.Indices{
		KCore: kcoreIndex,
		Pivot: pivotIndex,
	}

	// Get communities for scoped detection
	communities, err := c.getCommunitiesForDetection(ctx)
	if err != nil {
		c.logger.Warn("failed to get communities for anomaly detection, using global detection",
			slog.Any("error", err))
	}

	// Set dependencies for detectors
	deps := &inference.DetectorDependencies{
		StructuralIndices:   indices,
		PreviousKCore:       c.previousKCore, // May be nil on first run
		Communities:         communities,
		SimilarityFinder:    c.similarityFinder,
		RelationshipQuerier: &graphProviderAdapter{provider: c.graphProvider},
		AnomalyStorage:      c.anomalyStorage,
		Logger:              c.logger,
	}
	c.anomalyOrchestrator.SetDependencies(deps)

	// Run detection with timeout
	detectionCtx := ctx
	if c.config.AnomalyConfig.DetectionTimeout > 0 {
		var cancel context.CancelFunc
		detectionCtx, cancel = context.WithTimeout(ctx, c.config.AnomalyConfig.DetectionTimeout)
		defer cancel()
	}

	result, err := c.anomalyOrchestrator.RunDetection(detectionCtx)
	if err != nil {
		return errs.Wrap(err, "Component", "runAnomalyDetection", "run detection")
	}

	// Store previous k-core for demotion detection in next cycle
	c.previousKCore = kcoreIndex

	anomalyCount := 0
	if result != nil {
		anomalyCount = len(result.Anomalies)
	}

	c.logger.Info("anomaly detection complete",
		slog.Duration("duration", time.Since(start)),
		slog.Int("anomalies_found", anomalyCount))

	return nil
}

// getCommunitiesForDetection retrieves communities and converts them to CommunityInfo.
func (c *Component) getCommunitiesForDetection(ctx context.Context) ([]inference.CommunityInfo, error) {
	if c.storage == nil {
		return nil, nil
	}

	// Get all communities from storage
	communities, err := c.storage.GetAllCommunities(ctx)
	if err != nil {
		return nil, errs.Wrap(err, "Component", "getCommunitiesForDetection", "get communities")
	}

	if len(communities) == 0 {
		return nil, nil
	}

	// Convert to CommunityInfo
	infos := make([]inference.CommunityInfo, 0, len(communities))
	for _, comm := range communities {
		infos = append(infos, inference.CommunityInfo{
			ID:      comm.ID,
			Members: comm.Members,
			Level:   comm.Level,
		})
	}

	c.logger.Debug("loaded communities for anomaly detection",
		slog.Int("count", len(infos)))

	return infos, nil
}
