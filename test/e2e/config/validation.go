// Package config provides configuration for SemStreams E2E tests
package config

import "time"

// ValidationConfig holds configurable validation thresholds for E2E tests
type ValidationConfig struct {
	// MinStorageRate is the minimum acceptable ratio of stored to sent entities (default: 0.80)
	MinStorageRate float64

	// RequiredIndexes lists index buckets that must have entries after processing
	RequiredIndexes []string

	// RequiredMetrics lists Prometheus metrics that must be present after processing
	RequiredMetrics []string

	// ValidationTimeout is the maximum time to wait for processing before validation
	ValidationTimeout time.Duration
}

// ValidationResult contains the outcomes of NATS KV validation
type ValidationResult struct {
	// EntitiesSent is the count of entities sent through the pipeline
	EntitiesSent int

	// EntitiesStored is the count of entities found in NATS KV
	EntitiesStored int

	// StorageRate is the ratio of stored to sent entities (0.0-1.0)
	StorageRate float64

	// IndexesChecked lists which indexes were validated
	IndexesChecked []string

	// IndexesPopulated is the count of indexes with at least one entry
	IndexesPopulated int

	// MetricsVerified lists Prometheus metrics found
	MetricsVerified []string

	// MetricsMissing lists expected metrics that were not found
	MetricsMissing []string
}

// DefaultValidationConfig returns a ValidationConfig with sensible defaults
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MinStorageRate: 0.80,
		RequiredIndexes: []string{
			"PREDICATE_INDEX",
			"INCOMING_INDEX",
			"ALIAS_INDEX",
			"SPATIAL_INDEX",
			"TEMPORAL_INDEX",
		},
		RequiredMetrics: []string{
			"indexengine_events_processed_total",
			"indexengine_index_updates_total",
			"semstreams_cache_hits_total",
			"semstreams_cache_misses_total",
		},
		ValidationTimeout: 5 * time.Second,
	}
}

// NewValidationResult creates a new ValidationResult with the given sent count
func NewValidationResult(entitiesSent int) *ValidationResult {
	return &ValidationResult{
		EntitiesSent:    entitiesSent,
		IndexesChecked:  []string{},
		MetricsVerified: []string{},
		MetricsMissing:  []string{},
	}
}

// CalculateStorageRate computes the storage rate from sent and stored counts
func (r *ValidationResult) CalculateStorageRate() {
	if r.EntitiesSent == 0 {
		r.StorageRate = 0.0
		return
	}
	r.StorageRate = float64(r.EntitiesStored) / float64(r.EntitiesSent)
}

// MeetsThreshold returns true if the storage rate meets or exceeds the given threshold
func (r *ValidationResult) MeetsThreshold(threshold float64) bool {
	return r.StorageRate >= threshold
}
