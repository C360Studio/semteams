package inference

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutoApplyConfig_ShouldAutoApply(t *testing.T) {
	tests := []struct {
		name       string
		config     AutoApplyConfig
		confidence float64
		want       bool
	}{
		{
			name:       "meets confidence threshold",
			config:     AutoApplyConfig{Enabled: true, MinConfidence: 0.95},
			confidence: 0.97,
			want:       true,
		},
		{
			name:       "confidence too low",
			config:     AutoApplyConfig{Enabled: true, MinConfidence: 0.95},
			confidence: 0.90,
			want:       false,
		},
		{
			name:       "disabled config",
			config:     AutoApplyConfig{Enabled: false, MinConfidence: 0.95},
			confidence: 1.0,
			want:       false,
		},
		{
			name:       "exactly at threshold",
			config:     AutoApplyConfig{Enabled: true, MinConfidence: 0.95},
			confidence: 0.95,
			want:       true,
		},
		{
			name:       "just below threshold",
			config:     AutoApplyConfig{Enabled: true, MinConfidence: 0.95},
			confidence: 0.94,
			want:       false,
		},
		{
			name:       "high confidence gap",
			config:     AutoApplyConfig{Enabled: true, MinConfidence: 0.95},
			confidence: 1.0,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldAutoApply(tt.confidence)
			assert.Equal(t, tt.want, got, "ShouldAutoApply(%v) = %v, want %v",
				tt.confidence, got, tt.want)
		})
	}
}

func TestAutoApplyConfig_BuildPredicate(t *testing.T) {
	config := AutoApplyConfig{PredicateTemplate: "inferred.semantic.{band}"}

	tests := []struct {
		confidence float64
		want       string
	}{
		{1.00, "inferred.semantic.high"},
		{0.97, "inferred.semantic.high"},
		{0.95, "inferred.semantic.high"},
		{0.94, "inferred.semantic.medium"},
		{0.90, "inferred.semantic.medium"},
		{0.85, "inferred.semantic.medium"},
		{0.84, "inferred.semantic.related"},
		{0.80, "inferred.semantic.related"},
		{0.70, "inferred.semantic.related"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("confidence_%.2f", tt.confidence), func(t *testing.T) {
			got := config.BuildPredicate(tt.confidence)
			assert.Equal(t, tt.want, got, "BuildPredicate(%.2f) = %s, want %s",
				tt.confidence, got, tt.want)
		})
	}
}

func TestAutoApplyConfig_BuildPredicate_CustomTemplate(t *testing.T) {
	tests := []struct {
		template   string
		confidence float64
		want       string
	}{
		{"custom.{band}.edge", 0.95, "custom.high.edge"},
		{"prefix.{band}", 0.90, "prefix.medium"},
		{"{band}", 0.75, "related"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			config := AutoApplyConfig{PredicateTemplate: tt.template}
			got := config.BuildPredicate(tt.confidence)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReviewQueueConfig_ShouldQueue(t *testing.T) {
	tests := []struct {
		name       string
		config     ReviewQueueConfig
		confidence float64
		want       bool
	}{
		{
			name:       "within range",
			config:     ReviewQueueConfig{Enabled: true, MinConfidence: 0.70, MaxConfidence: 0.95},
			confidence: 0.85,
			want:       true,
		},
		{
			name:       "at min threshold",
			config:     ReviewQueueConfig{Enabled: true, MinConfidence: 0.70, MaxConfidence: 0.95},
			confidence: 0.70,
			want:       true,
		},
		{
			name:       "at max threshold",
			config:     ReviewQueueConfig{Enabled: true, MinConfidence: 0.70, MaxConfidence: 0.95},
			confidence: 0.95,
			want:       false, // Should NOT queue at max (auto-apply takes over)
		},
		{
			name:       "below min",
			config:     ReviewQueueConfig{Enabled: true, MinConfidence: 0.70, MaxConfidence: 0.95},
			confidence: 0.65,
			want:       false,
		},
		{
			name:       "above max",
			config:     ReviewQueueConfig{Enabled: true, MinConfidence: 0.70, MaxConfidence: 0.95},
			confidence: 0.98,
			want:       false,
		},
		{
			name:       "disabled",
			config:     ReviewQueueConfig{Enabled: false, MinConfidence: 0.70, MaxConfidence: 0.95},
			confidence: 0.85,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldQueue(tt.confidence)
			assert.Equal(t, tt.want, got, "ShouldQueue(%.2f) = %v, want %v",
				tt.confidence, got, tt.want)
		})
	}
}
