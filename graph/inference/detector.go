package inference

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360/semstreams/graph/structural"
	"github.com/c360/semstreams/pkg/errs"
)

// Detector defines the interface for anomaly detection algorithms.
type Detector interface {
	// Name returns the detector identifier for logging and metrics.
	Name() string

	// Detect runs the detection algorithm and returns discovered anomalies.
	// The context should be used for cancellation and timeouts.
	Detect(ctx context.Context) ([]*StructuralAnomaly, error)

	// Configure updates the detector configuration.
	// Called during orchestrator initialization and on config updates.
	Configure(config interface{}) error

	// SetDependencies updates the detector's shared dependencies.
	// Called by the orchestrator before running detection.
	SetDependencies(deps *DetectorDependencies)
}

// DetectorDependencies provides access to shared resources needed by detectors.
type DetectorDependencies struct {
	// StructuralIndices provides access to k-core and pivot indices.
	StructuralIndices *structural.Indices

	// PreviousKCore is the k-core index from the previous computation (for demotion detection).
	PreviousKCore *structural.KCoreIndex

	// Communities provides community membership for scoped detection.
	// Core isolation is analyzed within each community rather than globally.
	Communities []CommunityInfo

	// SimilarityFinder provides semantic similarity queries.
	SimilarityFinder SimilarityFinder

	// RelationshipQuerier provides relationship queries for transitivity detection.
	RelationshipQuerier RelationshipQuerier

	// AnomalyStorage provides access to persisted anomalies (for dismissed pair checks).
	AnomalyStorage Storage

	// Logger for detector logging.
	Logger *slog.Logger
}

// CommunityInfo provides community membership information for detectors.
// This is a simple interface to avoid circular imports with the clustering package.
type CommunityInfo struct {
	// ID is the community identifier
	ID string
	// Members is the list of entity IDs in this community
	Members []string
	// Level is the hierarchy level (0 = base communities)
	Level int
}

// SimilarityFinder provides semantic similarity search functionality.
// This interface is satisfied by IndexManager.
type SimilarityFinder interface {
	// FindSimilar returns entity IDs semantically similar to the given entity.
	// threshold is the minimum similarity score (0.0-1.0).
	// limit is the maximum number of results.
	FindSimilar(ctx context.Context, entityID string, threshold float64, limit int) ([]SimilarityResult, error)
}

// SimilarityResult represents a similarity search result.
type SimilarityResult struct {
	EntityID   string  `json:"entity_id"`
	Similarity float64 `json:"similarity"`
}

// RelationshipQuerier provides relationship queries for detectors.
// This interface is satisfied by QueryManager.
type RelationshipQuerier interface {
	// GetOutgoingRelationships returns all outgoing relationships from an entity.
	GetOutgoingRelationships(ctx context.Context, entityID string) ([]RelationshipInfo, error)

	// GetIncomingRelationships returns all incoming relationships to an entity.
	GetIncomingRelationships(ctx context.Context, entityID string) ([]RelationshipInfo, error)
}

// RelationshipInfo represents a relationship for detector use.
type RelationshipInfo struct {
	FromEntityID string `json:"from_entity_id"`
	ToEntityID   string `json:"to_entity_id"`
	Predicate    string `json:"predicate"`
}

// Orchestrator coordinates running all enabled detectors.
type Orchestrator struct {
	mu sync.RWMutex

	// Configuration
	config Config

	// Storage for persisting anomalies
	storage Storage

	// Registered detectors
	detectors []Detector

	// Dependencies shared across detectors
	deps *DetectorDependencies

	// RelationshipApplier for auto-applying virtual edges
	applier RelationshipApplier

	// Logger
	logger *slog.Logger
}

// OrchestratorConfig holds configuration for the orchestrator.
type OrchestratorConfig struct {
	Config  Config
	Storage Storage
	Applier RelationshipApplier
	Logger  *slog.Logger
}

// NewOrchestrator creates a new detector orchestrator.
func NewOrchestrator(cfg OrchestratorConfig) (*Orchestrator, error) {
	if cfg.Storage == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "Orchestrator", "New", "storage is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Orchestrator{
		config:    cfg.Config,
		storage:   cfg.Storage,
		applier:   cfg.Applier,
		detectors: make([]Detector, 0),
		logger:    logger,
	}, nil
}

// SetDependencies sets the shared dependencies for all detectors.
// Must be called before RunDetection. Propagates dependencies to all registered detectors.
func (o *Orchestrator) SetDependencies(deps *DetectorDependencies) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.deps = deps

	// Propagate dependencies to all registered detectors
	for _, d := range o.detectors {
		d.SetDependencies(deps)
	}
}

// RegisterDetector adds a detector to the orchestrator.
func (o *Orchestrator) RegisterDetector(detector Detector) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.detectors = append(o.detectors, detector)
	o.logger.Debug("registered detector", "name", detector.Name())
}

// detectorOutput holds the result from a single detector run.
type detectorOutput struct {
	name      string
	anomalies []*StructuralAnomaly
	err       error
}

// RunDetection executes all registered detectors and persists results.
func (o *Orchestrator) RunDetection(ctx context.Context) (*Result, error) {
	o.mu.RLock()
	config := o.config
	detectors := o.detectors
	deps := o.deps
	o.mu.RUnlock()

	if !config.Enabled {
		return &Result{StartedAt: time.Now(), CompletedAt: time.Now()}, nil
	}

	if err := o.validateDependencies(deps); err != nil {
		return nil, err
	}

	result := &Result{StartedAt: time.Now(), Anomalies: make([]*StructuralAnomaly, 0)}

	o.logger.Debug("Starting detection with timeout", "timeout", config.DetectionTimeout)
	ctx, cancel := context.WithTimeout(ctx, config.DetectionTimeout)
	defer cancel()

	resultCh := o.runDetectorsConcurrently(ctx, config, detectors)
	detectorErrors := o.collectResults(ctx, config, result, resultCh)

	result.CompletedAt = time.Now()

	if len(detectorErrors) == len(detectors) && len(detectors) > 0 {
		errMsgs := make([]string, len(detectorErrors))
		for i, err := range detectorErrors {
			errMsgs[i] = err.Error()
		}
		aggregated := fmt.Errorf("all %d detectors failed: %s",
			len(detectors), strings.Join(errMsgs, "; "))
		return result, errs.WrapTransient(aggregated, "Orchestrator", "RunDetection",
			"all detectors failed")
	}

	// Apply virtual edges for high-confidence semantic gaps
	if err := o.applyVirtualEdges(ctx, config, result); err != nil {
		o.logger.Warn("virtual edge application failed", "error", err)
		// Don't fail the detection run, just log the warning
	}

	return result, nil
}

// validateDependencies checks that required dependencies are set.
func (o *Orchestrator) validateDependencies(deps *DetectorDependencies) error {
	if deps == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "Orchestrator", "RunDetection",
			"dependencies not set - call SetDependencies first")
	}
	if deps.StructuralIndices == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "Orchestrator", "RunDetection",
			"structural indices not available")
	}
	return nil
}

// runDetectorsConcurrently launches all enabled detectors and returns a channel of results.
func (o *Orchestrator) runDetectorsConcurrently(
	ctx context.Context,
	config Config,
	detectors []Detector,
) <-chan detectorOutput {
	resultCh := make(chan detectorOutput, len(detectors))
	var wg sync.WaitGroup

	for _, d := range detectors {
		if !config.IsDetectorEnabled(d.Name()) {
			o.logger.Debug("skipping disabled detector", "name", d.Name())
			continue
		}

		wg.Add(1)
		go func(detector Detector) {
			defer wg.Done()
			o.logger.Debug("running detector", "name", detector.Name())
			anomalies, err := detector.Detect(ctx)
			resultCh <- detectorOutput{name: detector.Name(), anomalies: anomalies, err: err}
		}(d)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	return resultCh
}

// collectResults gathers detector outputs and persists anomalies.
func (o *Orchestrator) collectResults(
	ctx context.Context,
	config Config,
	result *Result,
	resultCh <-chan detectorOutput,
) []error {
	var detectorErrors []error
	totalAnomalies := 0

	for res := range resultCh {
		if res.err != nil {
			o.logger.Error("detector failed", "name", res.name, "error", res.err)
			detectorErrors = append(detectorErrors, fmt.Errorf("%s: %w", res.name, res.err))
			continue
		}

		o.logger.Info("detector completed", "name", res.name, "anomalies", len(res.anomalies))
		totalAnomalies, result.Truncated = o.persistAnomalies(ctx, config, result, res.anomalies, totalAnomalies)

		if result.Truncated {
			break
		}
	}

	return detectorErrors
}

// persistAnomalies saves anomalies to storage respecting the max limit.
func (o *Orchestrator) persistAnomalies(
	ctx context.Context,
	config Config,
	result *Result,
	anomalies []*StructuralAnomaly,
	totalAnomalies int,
) (int, bool) {
	truncated := false

	for _, anomaly := range anomalies {
		if config.MaxAnomaliesPerRun > 0 && totalAnomalies >= config.MaxAnomaliesPerRun {
			truncated = true
			break
		}

		if err := o.storage.Save(ctx, anomaly); err != nil {
			o.logger.Error("failed to save anomaly", "id", anomaly.ID, "error", err)
			continue
		}

		result.Anomalies = append(result.Anomalies, anomaly)
		totalAnomalies++
	}

	return totalAnomalies, truncated
}

// UpdateConfig updates the orchestrator configuration.
func (o *Orchestrator) UpdateConfig(config Config) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if err := config.Validate(); err != nil {
		return err
	}

	o.config = config

	for _, d := range o.detectors {
		var detectorConfig interface{}
		switch d.Name() {
		case "semantic_gap":
			detectorConfig = config.SemanticGap
		case "core_anomaly":
			detectorConfig = config.CoreAnomaly
		case "transitivity":
			detectorConfig = config.Transitivity
		}

		if detectorConfig != nil {
			if err := d.Configure(detectorConfig); err != nil {
				o.logger.Warn("failed to configure detector", "name", d.Name(), "error", err)
			}
		}
	}

	return nil
}

// GetRegisteredDetectors returns the names of all registered detectors.
func (o *Orchestrator) GetRegisteredDetectors() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	names := make([]string, len(o.detectors))
	for i, d := range o.detectors {
		names[i] = d.Name()
	}
	return names
}

// SetApplier sets the relationship applier for virtual edge creation.
// This allows late binding when the applier depends on other components.
func (o *Orchestrator) SetApplier(applier RelationshipApplier) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.applier = applier
}

// applyVirtualEdges processes high-confidence semantic gaps and creates virtual edges.
// Only applies to semantic gap anomalies that meet the auto-apply threshold.
func (o *Orchestrator) applyVirtualEdges(ctx context.Context, config Config, result *Result) error {
	if !config.VirtualEdges.AutoApply.Enabled {
		return nil
	}

	if o.applier == nil {
		o.logger.Debug("virtual edge application skipped: no applier configured")
		return nil
	}

	var applied, queued int
	var lastErr error

	// Collect statistics for debugging
	var evaluated int
	var belowConfidenceThreshold int
	var confidenceSum float64

	for _, anomaly := range result.Anomalies {
		// Only process semantic gap anomalies
		if anomaly.Type != AnomalySemanticStructuralGap {
			continue
		}

		confidence := anomaly.Confidence
		evaluated++
		confidenceSum += confidence

		// Log threshold evaluation for debugging
		meetsConfidenceThreshold := confidence >= config.VirtualEdges.AutoApply.MinConfidence

		if !meetsConfidenceThreshold {
			belowConfidenceThreshold++
		}

		o.logger.Debug("evaluating anomaly for auto-apply",
			"entity_a", anomaly.EntityA,
			"entity_b", anomaly.EntityB,
			"confidence", confidence,
			"similarity", anomaly.Evidence.Similarity,
			"structural_distance", anomaly.Evidence.StructuralDistance,
			"min_confidence", config.VirtualEdges.AutoApply.MinConfidence,
			"meets_confidence", meetsConfidenceThreshold,
		)

		// Check if this should be auto-applied (based on confidence score)
		if config.VirtualEdges.AutoApply.ShouldAutoApply(confidence) {
			if err := o.autoApplyEdge(ctx, config, anomaly); err != nil {
				o.logger.Error("failed to auto-apply edge",
					"entity_a", anomaly.EntityA,
					"entity_b", anomaly.EntityB,
					"error", err)
				lastErr = err
				continue
			}
			applied++
			continue
		}

		// Check if this should go to review queue (based on confidence score)
		if config.VirtualEdges.ReviewQueue.ShouldQueue(confidence) {
			anomaly.Status = StatusHumanReview
			if err := o.storage.Save(ctx, anomaly); err != nil {
				o.logger.Error("failed to queue for review",
					"id", anomaly.ID,
					"error", err)
				lastErr = err
				continue
			}
			queued++
		}
	}

	// Calculate averages for diagnostic logging
	avgConfidence := 0.0
	if evaluated > 0 {
		avgConfidence = confidenceSum / float64(evaluated)
	}

	// Always log summary for visibility - including diagnostic info
	o.logger.Info("virtual edge processing complete",
		"semantic_gaps_evaluated", evaluated,
		"auto_applied", applied,
		"queued_for_review", queued,
		"below_confidence_threshold", belowConfidenceThreshold,
		"avg_confidence", avgConfidence,
		"min_confidence_threshold", config.VirtualEdges.AutoApply.MinConfidence)

	// Update result with virtual edge stats
	result.AutoApplied = applied
	result.QueuedForReview = queued

	return lastErr
}

// autoApplyEdge creates a virtual edge from a high-confidence semantic gap.
func (o *Orchestrator) autoApplyEdge(ctx context.Context, config Config, anomaly *StructuralAnomaly) error {
	predicate := config.VirtualEdges.AutoApply.BuildPredicate(anomaly.Confidence)

	suggestion := &RelationshipSuggestion{
		FromEntity: anomaly.EntityA,
		ToEntity:   anomaly.EntityB,
		Predicate:  predicate,
		Confidence: anomaly.Confidence,
		Reasoning: fmt.Sprintf("Auto-applied: confidence %.2f (similarity %.2f, structural distance %d)",
			anomaly.Confidence, anomaly.Evidence.Similarity, anomaly.Evidence.StructuralDistance),
	}

	if err := o.applier.ApplyRelationship(ctx, suggestion); err != nil {
		return err
	}

	// Update anomaly status to auto-applied
	now := time.Now()
	anomaly.Status = StatusAutoApplied
	anomaly.ReviewedAt = &now
	anomaly.ReviewedBy = "auto"
	anomaly.Suggestion = suggestion

	if err := o.storage.Save(ctx, anomaly); err != nil {
		return err
	}

	// Mark the pair as dismissed to prevent re-detection
	if natsStorage, ok := o.storage.(*NATSAnomalyStorage); ok {
		if err := natsStorage.MarkPairDismissed(ctx, anomaly.EntityA, anomaly.EntityB); err != nil {
			o.logger.Warn("failed to mark pair as dismissed",
				"entity_a", anomaly.EntityA,
				"entity_b", anomaly.EntityB,
				"error", err)
		}
	}

	return nil
}

// Result summarizes an inference detection run.
type Result struct {
	StartedAt       time.Time            `json:"started_at"`
	CompletedAt     time.Time            `json:"completed_at"`
	Anomalies       []*StructuralAnomaly `json:"anomalies"`
	Truncated       bool                 `json:"truncated"`         // Hit max anomalies limit
	AutoApplied     int                  `json:"auto_applied"`      // Virtual edges auto-applied
	QueuedForReview int                  `json:"queued_for_review"` // Anomalies sent to review queue
}

// Duration returns how long the detection run took.
func (r *Result) Duration() time.Duration {
	return r.CompletedAt.Sub(r.StartedAt)
}

// AnomalyCount returns the total number of anomalies detected.
func (r *Result) AnomalyCount() int {
	return len(r.Anomalies)
}

// CountByType returns anomaly counts grouped by type.
func (r *Result) CountByType() map[AnomalyType]int {
	counts := make(map[AnomalyType]int)
	for _, a := range r.Anomalies {
		counts[a.Type]++
	}
	return counts
}
