package inference

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/processor/graph/structuralindex"
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
}

// DetectorDependencies provides access to shared resources needed by detectors.
type DetectorDependencies struct {
	// StructuralIndices provides access to k-core and pivot indices.
	StructuralIndices *structuralindex.StructuralIndices

	// PreviousKCore is the k-core index from the previous computation (for demotion detection).
	PreviousKCore *structuralindex.KCoreIndex

	// SimilarityFinder provides semantic similarity queries.
	SimilarityFinder SimilarityFinder

	// RelationshipQuerier provides relationship queries for transitivity detection.
	RelationshipQuerier RelationshipQuerier

	// Logger for detector logging.
	Logger *slog.Logger
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

	// Logger
	logger *slog.Logger
}

// OrchestratorConfig holds configuration for the orchestrator.
type OrchestratorConfig struct {
	Config  Config
	Storage Storage
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
		detectors: make([]Detector, 0),
		logger:    logger,
	}, nil
}

// SetDependencies sets the shared dependencies for all detectors.
// Must be called before RunDetection.
func (o *Orchestrator) SetDependencies(deps *DetectorDependencies) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.deps = deps
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

// Result summarizes an inference detection run.
type Result struct {
	StartedAt   time.Time            `json:"started_at"`
	CompletedAt time.Time            `json:"completed_at"`
	Anomalies   []*StructuralAnomaly `json:"anomalies"`
	Truncated   bool                 `json:"truncated"` // Hit max anomalies limit
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
