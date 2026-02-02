package clustering

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semstreams/graph"
)

func TestStatisticalSummarizer_SummarizeCommunity(t *testing.T) {
	summarizer := NewStatisticalSummarizer()
	ctx := context.Background()

	// Create test entities with robotics theme
	// Using proper 6-part entity ID format: org.platform.domain.system.type.instance
	entities := []*gtypes.EntityState{
		{
			ID: "c360.platform.robotics.system.drone.1",
		},
		{
			ID: "c360.platform.robotics.system.drone.2",
		},
		{
			ID: "c360.platform.robotics.system.sensor.1",
		},
		{
			ID: "c360.platform.robotics.system.battery.1",
		},
	}

	community := &Community{
		ID:      "comm-0-test",
		Level:   0,
		Members: []string{"c360.platform.robotics.system.drone.1", "c360.platform.robotics.system.drone.2", "c360.platform.robotics.system.sensor.1", "c360.platform.robotics.system.battery.1"},
	}

	// Summarize community
	result, err := summarizer.SummarizeCommunity(ctx, community, entities)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify summary fields are populated
	assert.NotEmpty(t, result.StatisticalSummary, "StatisticalSummary should not be empty")
	assert.NotEmpty(t, result.Keywords, "Keywords should not be empty")
	assert.NotEmpty(t, result.RepEntities, "RepEntities should not be empty")
	assert.Equal(t, "statistical", result.SummaryStatus)

	// Verify keywords contain relevant terms
	keywordSet := make(map[string]bool)
	for _, kw := range result.Keywords {
		keywordSet[kw] = true
	}
	// Should contain terms from types (drone, sensor, battery)
	assert.True(t, keywordSet["drone"] || keywordSet["sensor"] || keywordSet["battery"],
		"Keywords should contain type-related terms")

	// Verify representative entities
	assert.LessOrEqual(t, len(result.RepEntities), summarizer.MaxRepEntities,
		"Should not exceed max representative entities")

	// Verify summary mentions entity count
	assert.Contains(t, result.StatisticalSummary, "4 entities", "Summary should mention entity count")

	t.Logf("Statistical Summary: %s", result.StatisticalSummary)
	t.Logf("Keywords: %v", result.Keywords)
	t.Logf("RepEntities: %v", result.RepEntities)
}

func TestStatisticalSummarizer_KeywordExtraction(t *testing.T) {
	summarizer := NewStatisticalSummarizer()
	summarizer.MaxKeywords = 5

	// Using proper 6-part entity ID format with navigation types
	entities := []*gtypes.EntityState{
		{
			ID: "c360.platform.robotics.system.navigation.1",
		},
		{
			ID: "c360.platform.robotics.system.navigation.2",
		},
		{
			ID: "c360.platform.robotics.system.navigation.3",
		},
	}

	keywords := summarizer.extractKeywords(entities)

	assert.LessOrEqual(t, len(keywords), summarizer.MaxKeywords)
	assert.NotEmpty(t, keywords)

	// Should extract terms from types
	keywordSet := make(map[string]bool)
	for _, kw := range keywords {
		keywordSet[kw] = true
	}

	// "navigation" should be highly ranked (appears in all types)
	assert.True(t, keywordSet["navigation"], "Should extract 'navigation' from types")

	t.Logf("Extracted keywords: %v", keywords)
}

func TestStatisticalSummarizer_RepresentativeEntities(t *testing.T) {
	summarizer := NewStatisticalSummarizer()
	summarizer.MaxRepEntities = 3
	ctx := context.Background()

	// Create a graph where "hub" is central (pointed to by others via triples)
	// This tests PageRank behavior: entities with many incoming links are important
	// Using proper 6-part entity ID format
	entities := []*gtypes.EntityState{
		{
			ID: "c360.platform.robotics.system.hub.1",
		},
		{
			ID: "c360.platform.robotics.system.sensor.1",
		},
		{
			ID: "c360.platform.robotics.system.sensor.2",
		},
		{
			ID: "c360.platform.robotics.system.actuator.1",
		},
	}

	repEntities := summarizer.findRepresentativeEntities(ctx, entities)

	assert.LessOrEqual(t, len(repEntities), summarizer.MaxRepEntities)
	assert.NotEmpty(t, repEntities)

	// With no edges/triples, entities are ranked by type frequency
	// "sensor" type appears twice, so sensor entities should rank high
	t.Logf("Representative entities: %v", repEntities)
}

func TestStatisticalSummarizer_SummaryGeneration(t *testing.T) {
	summarizer := NewStatisticalSummarizer()

	// Using proper 6-part entity ID format
	entities := []*gtypes.EntityState{
		{ID: "c360.platform.robotics.system.drone.1"},
		{ID: "c360.platform.robotics.system.drone.2"},
		{ID: "c360.platform.robotics.system.drone.3"},
		{ID: "c360.platform.robotics.system.sensor.1"},
		{ID: "c360.platform.robotics.system.sensor.2"},
	}

	keywords := []string{"robotics", "autonomous", "navigation"}

	summary := summarizer.generateSummary(entities, keywords)

	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "5 entities", "Should mention entity count")
	assert.Contains(t, summary, "drone", "Should mention most common type")
	assert.Contains(t, summary, "sensor", "Should mention second most common type")

	t.Logf("Generated summary: %s", summary)
}

func TestStatisticalSummarizer_EmptyEntities(t *testing.T) {
	summarizer := NewStatisticalSummarizer()
	ctx := context.Background()

	community := &Community{
		ID:      "comm-0-empty",
		Level:   0,
		Members: []string{},
	}

	// Empty entities should return error
	_, err := summarizer.SummarizeCommunity(ctx, community, []*gtypes.EntityState{})
	assert.Error(t, err, "Should error on empty entities")
}

func TestStatisticalSummarizer_NilCommunity(t *testing.T) {
	summarizer := NewStatisticalSummarizer()
	ctx := context.Background()

	entities := []*gtypes.EntityState{
		{ID: "c360.platform.robotics.system.test.1"},
	}

	// Nil community should return error
	_, err := summarizer.SummarizeCommunity(ctx, nil, entities)
	assert.Error(t, err, "Should error on nil community")
}

func TestStatisticalSummarizer_ContextCancellation(t *testing.T) {
	summarizer := NewStatisticalSummarizer()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	entityID := "c360.platform.robotics.system.test.1"
	entities := []*gtypes.EntityState{
		{ID: entityID},
	}

	community := &Community{
		ID:      "comm-0-test",
		Level:   0,
		Members: []string{entityID},
	}

	// Should return context error
	_, err := summarizer.SummarizeCommunity(ctx, community, entities)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestExtractTerms(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple text",
			input:    "autonomous navigation system",
			expected: []string{"autonomous", "navigation", "system"},
		},
		{
			name:     "With stop words",
			input:    "the drone is flying",
			expected: []string{"drone", "flying"},
		},
		{
			name:     "Hyphenated terms",
			input:    "path-planning algorithm",
			expected: []string{"path", "planning", "algorithm"},
		},
		{
			name:     "Short terms filtered",
			input:    "a big AI system",
			expected: []string{"big", "system"}, // "AI" is too short
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTerms(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}
