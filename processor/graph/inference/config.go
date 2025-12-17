// Package inference provides structural anomaly detection and inference
// for identifying potential missing relationships in the knowledge graph.
package inference

import (
	"fmt"
	"time"

	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/processor/graph/llm"
)

// Config configures the structural inference system
type Config struct {
	// Enabled activates inference detection
	Enabled bool `json:"enabled"`

	// RunWithCommunityDetection triggers inference after community detection completes
	RunWithCommunityDetection bool `json:"run_with_community_detection"`

	// MaxAnomaliesPerRun limits total anomalies detected per run to prevent runaway detection
	MaxAnomaliesPerRun int `json:"max_anomalies_per_run"`

	// Detector-specific configurations
	SemanticGap  SemanticGapConfig  `json:"semantic_gap"`
	CoreAnomaly  CoreAnomalyConfig  `json:"core_anomaly"`
	Transitivity TransitivityConfig `json:"transitivity"`

	// Review configuration for LLM-assisted and human review
	Review ReviewConfig `json:"review"`

	// Storage configuration
	Storage StorageConfig `json:"storage"`

	// Operation timeouts
	DetectionTimeout time.Duration `json:"detection_timeout"`
}

// SemanticGapConfig configures the semantic-structural gap detector
type SemanticGapConfig struct {
	// Enabled activates semantic-structural gap detection
	Enabled bool `json:"enabled"`

	// MinSemanticSimilarity is the minimum embedding similarity to consider (0.0-1.0)
	MinSemanticSimilarity float64 `json:"min_semantic_similarity"`

	// MinStructuralDistance is the minimum graph distance for flagging (hops)
	MinStructuralDistance int `json:"min_structural_distance"`

	// MaxGapsPerEntity limits gaps detected per entity to prevent noise
	MaxGapsPerEntity int `json:"max_gaps_per_entity"`

	// MaxCandidatesPerEntity limits semantic search candidates per entity
	MaxCandidatesPerEntity int `json:"max_candidates_per_entity"`
}

// CoreAnomalyConfig configures the k-core based anomaly detectors
type CoreAnomalyConfig struct {
	// Enabled activates core-based anomaly detection (isolation + demotion)
	Enabled bool `json:"enabled"`

	// MinCoreForHubAnalysis sets minimum k-core level for hub isolation analysis
	MinCoreForHubAnalysis int `json:"min_core_for_hub_analysis"`

	// HubIsolationThreshold is the peer connectivity ratio below which hub is "isolated"
	// Value between 0.0-1.0: ratio of actual same-core connections to expected
	HubIsolationThreshold float64 `json:"hub_isolation_threshold"`

	// TrackCoreDemotions enables detection of entities that dropped k-core level
	TrackCoreDemotions bool `json:"track_core_demotions"`

	// MinDemotionDelta is the minimum core level drop to flag (e.g., 2 means core 5->3 flagged)
	MinDemotionDelta int `json:"min_demotion_delta"`
}

// TransitivityConfig configures the transitivity gap detector
type TransitivityConfig struct {
	// Enabled activates transitivity gap detection
	Enabled bool `json:"enabled"`

	// MaxIntermediateHops is the maximum hops in A->...->B->...->C chains to analyze
	MaxIntermediateHops int `json:"max_intermediate_hops"`

	// MinExpectedTransitivity is the maximum expected A-C distance when A->B->C exists
	// Gaps where actual distance > this are flagged
	MinExpectedTransitivity int `json:"min_expected_transitivity"`

	// TransitivePredicates lists predicates that should be transitive
	// e.g., ["member_of", "part_of", "located_in"]
	TransitivePredicates []string `json:"transitive_predicates"`
}

// ReviewConfig configures the LLM-assisted review pipeline
type ReviewConfig struct {
	// Enabled activates the review worker
	Enabled bool `json:"enabled"`

	// Workers is the number of concurrent review workers
	Workers int `json:"workers"`

	// AutoApproveThreshold: LLM can auto-approve anomalies at or above this confidence
	AutoApproveThreshold float64 `json:"auto_approve_threshold"`

	// AutoRejectThreshold: LLM can auto-reject anomalies at or below this confidence
	AutoRejectThreshold float64 `json:"auto_reject_threshold"`

	// FallbackToHuman escalates uncertain cases (between thresholds) to human review
	FallbackToHuman bool `json:"fallback_to_human"`

	// BatchSize is the number of anomalies to process in each review batch
	BatchSize int `json:"batch_size"`

	// ReviewTimeout is the timeout for individual LLM review calls
	ReviewTimeout time.Duration `json:"review_timeout"`

	// LLM configuration for the review model
	LLM llm.Config `json:"llm"`
}

// StorageConfig configures anomaly storage
type StorageConfig struct {
	// BucketName is the NATS KV bucket for storing anomalies
	BucketName string `json:"bucket_name"`

	// RetentionDays is how long to keep resolved anomalies (applied/rejected)
	RetentionDays int `json:"retention_days"`

	// CleanupInterval is how often to run cleanup of old anomalies
	CleanupInterval time.Duration `json:"cleanup_interval"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() Config {
	return Config{
		Enabled:                   false, // Opt-in feature
		RunWithCommunityDetection: true,
		MaxAnomaliesPerRun:        100,
		SemanticGap: SemanticGapConfig{
			Enabled:                true,
			MinSemanticSimilarity:  0.7,
			MinStructuralDistance:  3,
			MaxGapsPerEntity:       5,
			MaxCandidatesPerEntity: 50,
		},
		CoreAnomaly: CoreAnomalyConfig{
			Enabled:               true,
			MinCoreForHubAnalysis: 2,
			HubIsolationThreshold: 0.3,
			TrackCoreDemotions:    true,
			MinDemotionDelta:      1,
		},
		Transitivity: TransitivityConfig{
			Enabled:                 true,
			MaxIntermediateHops:     2,
			MinExpectedTransitivity: 3,
			TransitivePredicates:    []string{"member_of", "part_of", "located_in", "belongs_to"},
		},
		Review: ReviewConfig{
			Enabled:              false, // Requires LLM setup
			Workers:              2,
			AutoApproveThreshold: 0.9,
			AutoRejectThreshold:  0.3,
			FallbackToHuman:      true,
			BatchSize:            10,
			ReviewTimeout:        30 * time.Second,
		},
		Storage: StorageConfig{
			BucketName:      "ANOMALY_INDEX",
			RetentionDays:   30,
			CleanupInterval: 24 * time.Hour,
		},
		DetectionTimeout: 5 * time.Minute,
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if disabled
	}

	if err := c.validateGlobalSettings(); err != nil {
		return err
	}

	if err := c.validateSemanticGap(); err != nil {
		return err
	}

	if err := c.validateCoreAnomaly(); err != nil {
		return err
	}

	if err := c.validateTransitivity(); err != nil {
		return err
	}

	if err := c.validateReview(); err != nil {
		return err
	}

	return c.validateStorage()
}

// validateGlobalSettings validates top-level configuration
func (c *Config) validateGlobalSettings() error {
	if c.MaxAnomaliesPerRun <= 0 {
		msg := fmt.Sprintf("max_anomalies_per_run must be positive, got %d", c.MaxAnomaliesPerRun)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.DetectionTimeout <= 0 {
		msg := fmt.Sprintf("detection_timeout must be positive, got %v", c.DetectionTimeout)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	// At least one detector must be enabled
	if !c.SemanticGap.Enabled && !c.CoreAnomaly.Enabled && !c.Transitivity.Enabled {
		msg := "at least one detector must be enabled when inference is enabled"
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	return nil
}

// validateSemanticGap validates semantic gap detector configuration
func (c *Config) validateSemanticGap() error {
	if !c.SemanticGap.Enabled {
		return nil
	}

	if c.SemanticGap.MinSemanticSimilarity < 0 || c.SemanticGap.MinSemanticSimilarity > 1 {
		msg := fmt.Sprintf(
			"semantic_gap.min_semantic_similarity must be between 0 and 1, got %f",
			c.SemanticGap.MinSemanticSimilarity,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.SemanticGap.MinStructuralDistance <= 0 {
		msg := fmt.Sprintf(
			"semantic_gap.min_structural_distance must be positive, got %d",
			c.SemanticGap.MinStructuralDistance,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.SemanticGap.MaxGapsPerEntity <= 0 {
		msg := fmt.Sprintf(
			"semantic_gap.max_gaps_per_entity must be positive, got %d",
			c.SemanticGap.MaxGapsPerEntity,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.SemanticGap.MaxCandidatesPerEntity <= 0 {
		msg := fmt.Sprintf(
			"semantic_gap.max_candidates_per_entity must be positive, got %d",
			c.SemanticGap.MaxCandidatesPerEntity,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	return nil
}

// validateCoreAnomaly validates core anomaly detector configuration
func (c *Config) validateCoreAnomaly() error {
	if !c.CoreAnomaly.Enabled {
		return nil
	}

	if c.CoreAnomaly.MinCoreForHubAnalysis < 1 {
		msg := fmt.Sprintf(
			"core_anomaly.min_core_for_hub_analysis must be at least 1, got %d",
			c.CoreAnomaly.MinCoreForHubAnalysis,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.CoreAnomaly.HubIsolationThreshold < 0 || c.CoreAnomaly.HubIsolationThreshold > 1 {
		msg := fmt.Sprintf(
			"core_anomaly.hub_isolation_threshold must be between 0 and 1, got %f",
			c.CoreAnomaly.HubIsolationThreshold,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.CoreAnomaly.TrackCoreDemotions && c.CoreAnomaly.MinDemotionDelta <= 0 {
		msg := fmt.Sprintf(
			"core_anomaly.min_demotion_delta must be positive when tracking demotions, got %d",
			c.CoreAnomaly.MinDemotionDelta,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	return nil
}

// validateTransitivity validates transitivity gap detector configuration
func (c *Config) validateTransitivity() error {
	if !c.Transitivity.Enabled {
		return nil
	}

	if c.Transitivity.MaxIntermediateHops <= 0 {
		msg := fmt.Sprintf(
			"transitivity.max_intermediate_hops must be positive, got %d",
			c.Transitivity.MaxIntermediateHops,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Transitivity.MinExpectedTransitivity <= 0 {
		msg := fmt.Sprintf(
			"transitivity.min_expected_transitivity must be positive, got %d",
			c.Transitivity.MinExpectedTransitivity,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	return nil
}

// validateReview validates review pipeline configuration
func (c *Config) validateReview() error {
	if !c.Review.Enabled {
		return nil
	}

	if c.Review.Workers <= 0 {
		msg := fmt.Sprintf("review.workers must be positive, got %d", c.Review.Workers)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Review.Workers > 10 {
		msg := fmt.Sprintf(
			"review.workers should not exceed 10 for resource management, got %d",
			c.Review.Workers,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Review.AutoApproveThreshold < 0 || c.Review.AutoApproveThreshold > 1 {
		msg := fmt.Sprintf(
			"review.auto_approve_threshold must be between 0 and 1, got %f",
			c.Review.AutoApproveThreshold,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Review.AutoRejectThreshold < 0 || c.Review.AutoRejectThreshold > 1 {
		msg := fmt.Sprintf(
			"review.auto_reject_threshold must be between 0 and 1, got %f",
			c.Review.AutoRejectThreshold,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Review.AutoRejectThreshold >= c.Review.AutoApproveThreshold {
		msg := fmt.Sprintf(
			"review.auto_reject_threshold (%f) must be less than auto_approve_threshold (%f)",
			c.Review.AutoRejectThreshold,
			c.Review.AutoApproveThreshold,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Review.BatchSize <= 0 {
		msg := fmt.Sprintf("review.batch_size must be positive, got %d", c.Review.BatchSize)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Review.ReviewTimeout <= 0 {
		msg := fmt.Sprintf("review.review_timeout must be positive, got %v", c.Review.ReviewTimeout)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	return nil
}

// validateStorage validates storage configuration
func (c *Config) validateStorage() error {
	if c.Storage.BucketName == "" {
		msg := "storage.bucket_name cannot be empty"
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if len(c.Storage.BucketName) > 64 {
		msg := fmt.Sprintf(
			"storage.bucket_name is too long (max 64 chars): %s",
			c.Storage.BucketName,
		)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Storage.RetentionDays <= 0 {
		msg := fmt.Sprintf("storage.retention_days must be positive, got %d", c.Storage.RetentionDays)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	if c.Storage.CleanupInterval <= 0 {
		msg := fmt.Sprintf("storage.cleanup_interval must be positive, got %v", c.Storage.CleanupInterval)
		return errs.WrapInvalid(errs.ErrInvalidConfig, "inference", "Validate", msg)
	}

	return nil
}

// ApplyDefaults fills in any zero values with defaults
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()

	c.applyGlobalDefaults(defaults)
	c.applySemanticGapDefaults(defaults)
	c.applyCoreAnomalyDefaults(defaults)
	c.applyTransitivityDefaults(defaults)
	c.applyReviewDefaults(defaults)
	c.applyStorageDefaults(defaults)
}

func (c *Config) applyGlobalDefaults(defaults Config) {
	if c.MaxAnomaliesPerRun == 0 {
		c.MaxAnomaliesPerRun = defaults.MaxAnomaliesPerRun
	}
	if c.DetectionTimeout == 0 {
		c.DetectionTimeout = defaults.DetectionTimeout
	}
}

func (c *Config) applySemanticGapDefaults(defaults Config) {
	if c.SemanticGap.MinSemanticSimilarity == 0 {
		c.SemanticGap.MinSemanticSimilarity = defaults.SemanticGap.MinSemanticSimilarity
	}
	if c.SemanticGap.MinStructuralDistance == 0 {
		c.SemanticGap.MinStructuralDistance = defaults.SemanticGap.MinStructuralDistance
	}
	if c.SemanticGap.MaxGapsPerEntity == 0 {
		c.SemanticGap.MaxGapsPerEntity = defaults.SemanticGap.MaxGapsPerEntity
	}
	if c.SemanticGap.MaxCandidatesPerEntity == 0 {
		c.SemanticGap.MaxCandidatesPerEntity = defaults.SemanticGap.MaxCandidatesPerEntity
	}
}

func (c *Config) applyCoreAnomalyDefaults(defaults Config) {
	if c.CoreAnomaly.MinCoreForHubAnalysis == 0 {
		c.CoreAnomaly.MinCoreForHubAnalysis = defaults.CoreAnomaly.MinCoreForHubAnalysis
	}
	if c.CoreAnomaly.HubIsolationThreshold == 0 {
		c.CoreAnomaly.HubIsolationThreshold = defaults.CoreAnomaly.HubIsolationThreshold
	}
	if c.CoreAnomaly.MinDemotionDelta == 0 {
		c.CoreAnomaly.MinDemotionDelta = defaults.CoreAnomaly.MinDemotionDelta
	}
}

func (c *Config) applyTransitivityDefaults(defaults Config) {
	if c.Transitivity.MaxIntermediateHops == 0 {
		c.Transitivity.MaxIntermediateHops = defaults.Transitivity.MaxIntermediateHops
	}
	if c.Transitivity.MinExpectedTransitivity == 0 {
		c.Transitivity.MinExpectedTransitivity = defaults.Transitivity.MinExpectedTransitivity
	}
	if len(c.Transitivity.TransitivePredicates) == 0 {
		c.Transitivity.TransitivePredicates = defaults.Transitivity.TransitivePredicates
	}
}

func (c *Config) applyReviewDefaults(defaults Config) {
	if c.Review.Workers == 0 {
		c.Review.Workers = defaults.Review.Workers
	}
	if c.Review.AutoApproveThreshold == 0 {
		c.Review.AutoApproveThreshold = defaults.Review.AutoApproveThreshold
	}
	if c.Review.AutoRejectThreshold == 0 {
		c.Review.AutoRejectThreshold = defaults.Review.AutoRejectThreshold
	}
	if c.Review.BatchSize == 0 {
		c.Review.BatchSize = defaults.Review.BatchSize
	}
	if c.Review.ReviewTimeout == 0 {
		c.Review.ReviewTimeout = defaults.Review.ReviewTimeout
	}
}

func (c *Config) applyStorageDefaults(defaults Config) {
	if c.Storage.BucketName == "" {
		c.Storage.BucketName = defaults.Storage.BucketName
	}
	if c.Storage.RetentionDays == 0 {
		c.Storage.RetentionDays = defaults.Storage.RetentionDays
	}
	if c.Storage.CleanupInterval == 0 {
		c.Storage.CleanupInterval = defaults.Storage.CleanupInterval
	}
}

// GetEnabledDetectors returns a list of enabled detector names
func (c *Config) GetEnabledDetectors() []string {
	var enabled []string
	if c.SemanticGap.Enabled {
		enabled = append(enabled, "semantic_gap")
	}
	if c.CoreAnomaly.Enabled {
		enabled = append(enabled, "core_anomaly")
	}
	if c.Transitivity.Enabled {
		enabled = append(enabled, "transitivity")
	}
	return enabled
}

// IsDetectorEnabled checks if a specific detector is enabled
func (c *Config) IsDetectorEnabled(detector string) bool {
	switch detector {
	case "semantic_gap":
		return c.SemanticGap.Enabled
	case "core_anomaly":
		return c.CoreAnomaly.Enabled
	case "transitivity":
		return c.Transitivity.Enabled
	default:
		return false
	}
}
