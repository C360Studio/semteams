// Package config provides configuration for SemStreams E2E tests
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultValidationConfig(t *testing.T) {
	cfg := DefaultValidationConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, 0.80, cfg.MinStorageRate)
	assert.Equal(t, 5*time.Second, cfg.ValidationTimeout)
	assert.NotEmpty(t, cfg.RequiredIndexes)
	assert.Contains(t, cfg.RequiredIndexes, "PREDICATE_INDEX")
	assert.Contains(t, cfg.RequiredIndexes, "SPATIAL_INDEX")
	assert.Contains(t, cfg.RequiredIndexes, "ALIAS_INDEX")
}

func TestValidationResult_StorageRate(t *testing.T) {
	tests := []struct {
		name     string
		sent     int
		stored   int
		expected float64
	}{
		{
			name:     "all stored",
			sent:     10,
			stored:   10,
			expected: 1.0,
		},
		{
			name:     "80% stored",
			sent:     10,
			stored:   8,
			expected: 0.8,
		},
		{
			name:     "none stored",
			sent:     10,
			stored:   0,
			expected: 0.0,
		},
		{
			name:     "zero sent",
			sent:     0,
			stored:   0,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewValidationResult(tt.sent)
			result.EntitiesStored = tt.stored
			result.CalculateStorageRate()

			assert.Equal(t, tt.expected, result.StorageRate)
		})
	}
}

func TestValidationResult_MeetsThreshold(t *testing.T) {
	cfg := DefaultValidationConfig()

	tests := []struct {
		name     string
		rate     float64
		expected bool
	}{
		{
			name:     "above threshold",
			rate:     0.90,
			expected: true,
		},
		{
			name:     "at threshold",
			rate:     0.80,
			expected: true,
		},
		{
			name:     "below threshold",
			rate:     0.70,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{StorageRate: tt.rate}
			assert.Equal(t, tt.expected, result.MeetsThreshold(cfg.MinStorageRate))
		})
	}
}
