package inference

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutoApplyConfig_ShouldAutoApply(t *testing.T) {
	tests := []struct {
		name           string
		config         AutoApplyConfig
		similarity     float64
		structuralDist int
		want           bool
	}{
		{
			name:           "meets both thresholds",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.90,
			structuralDist: 5,
			want:           true,
		},
		{
			name:           "similarity too low",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.80,
			structuralDist: 5,
			want:           false,
		},
		{
			name:           "structural distance too low",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.90,
			structuralDist: 3,
			want:           false,
		},
		{
			name:           "disabled config",
			config:         AutoApplyConfig{Enabled: false, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.95,
			structuralDist: 10,
			want:           false,
		},
		{
			name:           "exactly at threshold - both",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.85,
			structuralDist: 4,
			want:           true,
		},
		{
			name:           "exactly at similarity threshold only",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.85,
			structuralDist: 3,
			want:           false,
		},
		{
			name:           "exactly at distance threshold only",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.84,
			structuralDist: 4,
			want:           false,
		},
		{
			name:           "both below threshold",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.75,
			structuralDist: 3,
			want:           false,
		},
		{
			name:           "high confidence gap",
			config:         AutoApplyConfig{Enabled: true, MinSimilarity: 0.85, MinStructuralDistance: 4},
			similarity:     0.95,
			structuralDist: 10,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldAutoApply(tt.similarity, tt.structuralDist)
			assert.Equal(t, tt.want, got, "ShouldAutoApply(%v, %d) = %v, want %v",
				tt.similarity, tt.structuralDist, got, tt.want)
		})
	}
}

func TestAutoApplyConfig_BuildPredicate(t *testing.T) {
	config := AutoApplyConfig{PredicateTemplate: "inferred.semantic.{band}"}

	tests := []struct {
		similarity float64
		want       string
	}{
		{0.95, "inferred.semantic.high"},
		{0.92, "inferred.semantic.high"},
		{0.90, "inferred.semantic.high"},
		{0.89, "inferred.semantic.medium"},
		{0.87, "inferred.semantic.medium"},
		{0.85, "inferred.semantic.medium"},
		{0.84, "inferred.semantic.related"},
		{0.80, "inferred.semantic.related"},
		{0.70, "inferred.semantic.related"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("similarity_%.2f", tt.similarity), func(t *testing.T) {
			got := config.BuildPredicate(tt.similarity)
			assert.Equal(t, tt.want, got, "BuildPredicate(%.2f) = %s, want %s",
				tt.similarity, got, tt.want)
		})
	}
}

func TestAutoApplyConfig_BuildPredicate_CustomTemplate(t *testing.T) {
	tests := []struct {
		template   string
		similarity float64
		want       string
	}{
		{"custom.{band}.edge", 0.95, "custom.high.edge"},
		{"prefix.{band}", 0.87, "prefix.medium"},
		{"{band}", 0.75, "related"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			config := AutoApplyConfig{PredicateTemplate: tt.template}
			got := config.BuildPredicate(tt.similarity)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReviewQueueConfig_ShouldQueue(t *testing.T) {
	tests := []struct {
		name       string
		config     ReviewQueueConfig
		similarity float64
		want       bool
	}{
		{
			name:       "within range",
			config:     ReviewQueueConfig{Enabled: true, MinSimilarity: 0.70, MaxSimilarity: 0.85},
			similarity: 0.78,
			want:       true,
		},
		{
			name:       "at min threshold",
			config:     ReviewQueueConfig{Enabled: true, MinSimilarity: 0.70, MaxSimilarity: 0.85},
			similarity: 0.70,
			want:       true,
		},
		{
			name:       "at max threshold",
			config:     ReviewQueueConfig{Enabled: true, MinSimilarity: 0.70, MaxSimilarity: 0.85},
			similarity: 0.85,
			want:       false, // Should NOT queue at max (auto-apply takes over)
		},
		{
			name:       "below min",
			config:     ReviewQueueConfig{Enabled: true, MinSimilarity: 0.70, MaxSimilarity: 0.85},
			similarity: 0.65,
			want:       false,
		},
		{
			name:       "above max",
			config:     ReviewQueueConfig{Enabled: true, MinSimilarity: 0.70, MaxSimilarity: 0.85},
			similarity: 0.90,
			want:       false,
		},
		{
			name:       "disabled",
			config:     ReviewQueueConfig{Enabled: false, MinSimilarity: 0.70, MaxSimilarity: 0.85},
			similarity: 0.78,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldQueue(tt.similarity)
			assert.Equal(t, tt.want, got, "ShouldQueue(%.2f) = %v, want %v",
				tt.similarity, got, tt.want)
		})
	}
}
