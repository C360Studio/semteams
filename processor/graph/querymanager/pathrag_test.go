package querymanager

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
)

func TestValidatePathPattern(t *testing.T) {
	m := &Manager{
		config: Config{
			Query: QueryConfig{
				MaxPathLength: 5,
			},
		},
	}

	tests := []struct {
		name    string
		pattern PathPattern
		wantErr bool
	}{
		{
			name:    "Valid pattern",
			pattern: PathPattern{MaxDepth: 3, DecayFactor: 0.8},
			wantErr: false,
		},
		{
			name:    "Zero max depth",
			pattern: PathPattern{MaxDepth: 0},
			wantErr: true,
		},
		{
			name:    "Negative max depth",
			pattern: PathPattern{MaxDepth: -1},
			wantErr: true,
		},
		{
			name:    "Exceeds max path length",
			pattern: PathPattern{MaxDepth: 10},
			wantErr: true,
		},
		{
			name:    "Decay factor too high",
			pattern: PathPattern{MaxDepth: 3, DecayFactor: 1.5},
			wantErr: true,
		},
		{
			name:    "Negative decay factor",
			pattern: PathPattern{MaxDepth: 3, DecayFactor: -0.5},
			wantErr: true,
		},
		{
			name:    "Valid zero decay factor (no decay)",
			pattern: PathPattern{MaxDepth: 3, DecayFactor: 0},
			wantErr: false,
		},
		{
			name:    "Valid decay factor at boundary",
			pattern: PathPattern{MaxDepth: 3, DecayFactor: 1.0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validatePathPattern(tt.pattern)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMatchesEdgeTypes(t *testing.T) {
	m := &Manager{}

	tests := []struct {
		name         string
		edgeType     string
		allowedTypes []string
		want         bool
	}{
		{
			name:         "Empty allowed types matches all",
			edgeType:     "any_type",
			allowedTypes: []string{},
			want:         true,
		},
		{
			name:         "Nil allowed types matches all",
			edgeType:     "any_type",
			allowedTypes: nil,
			want:         true,
		},
		{
			name:         "Exact match",
			edgeType:     "connects_to",
			allowedTypes: []string{"connects_to", "relates_to"},
			want:         true,
		},
		{
			name:         "No match",
			edgeType:     "unknown_type",
			allowedTypes: []string{"connects_to", "relates_to"},
			want:         false,
		},
		{
			name:         "Single allowed type match",
			edgeType:     "depends_on",
			allowedTypes: []string{"depends_on"},
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.matchesEdgeTypes(tt.edgeType, tt.allowedTypes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchesNodeTypes(t *testing.T) {
	m := &Manager{}

	tests := []struct {
		name         string
		entityID     string
		allowedTypes []string
		want         bool
	}{
		{
			name:         "Empty allowed types matches all",
			entityID:     "org.platform.domain.system.sensor.sensor-01",
			allowedTypes: []string{},
			want:         true,
		},
		{
			name:         "Nil allowed types matches all",
			entityID:     "org.platform.domain.system.sensor.sensor-01",
			allowedTypes: nil,
			want:         true,
		},
		{
			name:         "Type matches",
			entityID:     "org.platform.domain.system.sensor.sensor-01",
			allowedTypes: []string{"sensor", "zone"},
			want:         true,
		},
		{
			name:         "Type does not match",
			entityID:     "org.platform.domain.system.sensor.sensor-01",
			allowedTypes: []string{"zone", "building"},
			want:         false,
		},
		{
			name:         "Invalid entity ID format",
			entityID:     "invalid-id",
			allowedTypes: []string{"sensor"},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.matchesNodeTypes(tt.entityID, tt.allowedTypes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildPathResult(t *testing.T) {
	m := &Manager{}

	entity1 := &gtypes.EntityState{ID: "entity-1"}
	entity2 := &gtypes.EntityState{ID: "entity-2"}
	entity3 := &gtypes.EntityState{ID: "entity-3"}

	t.Run("Build result with paths", func(t *testing.T) {
		state := &pathTraversalState{
			entities: map[string]*gtypes.EntityState{
				"entity-1": entity1,
				"entity-2": entity2,
				"entity-3": entity3,
			},
			pathTo: map[string][]string{
				"entity-1": {"entity-1"},
				"entity-2": {"entity-1", "entity-2"},
				"entity-3": {"entity-1", "entity-2", "entity-3"},
			},
			edgesTo: map[string][]GraphEdge{
				"entity-1": {},
				"entity-2": {{From: "entity-1", To: "entity-2", Weight: 1.0}},
				"entity-3": {
					{From: "entity-1", To: "entity-2", Weight: 1.0},
					{From: "entity-2", To: "entity-3", Weight: 0.8},
				},
			},
		}

		pattern := PathPattern{IncludeSelf: false}
		result := m.buildPathResult(state, pattern)

		assert.Equal(t, 3, result.Count)
		assert.Len(t, result.Entities, 3)
		// Should have 2 paths (excluding start-only path when IncludeSelf=false)
		assert.Len(t, result.Paths, 2)
	})

	t.Run("Include self path", func(t *testing.T) {
		state := &pathTraversalState{
			entities: map[string]*gtypes.EntityState{
				"entity-1": entity1,
			},
			pathTo: map[string][]string{
				"entity-1": {"entity-1"},
			},
			edgesTo: map[string][]GraphEdge{
				"entity-1": {},
			},
		}

		pattern := PathPattern{IncludeSelf: true}
		result := m.buildPathResult(state, pattern)

		assert.Equal(t, 1, result.Count)
		assert.Len(t, result.Paths, 1)
		assert.Equal(t, 0, result.Paths[0].Length)
	})

	t.Run("Calculate path weight", func(t *testing.T) {
		state := &pathTraversalState{
			entities: map[string]*gtypes.EntityState{
				"entity-1": entity1,
				"entity-2": entity2,
			},
			pathTo: map[string][]string{
				"entity-1": {"entity-1"},
				"entity-2": {"entity-1", "entity-2"},
			},
			edgesTo: map[string][]GraphEdge{
				"entity-1": {},
				"entity-2": {{From: "entity-1", To: "entity-2", Weight: 0.5}},
			},
		}

		pattern := PathPattern{IncludeSelf: false}
		result := m.buildPathResult(state, pattern)

		require.Len(t, result.Paths, 1)
		assert.Equal(t, 0.5, result.Paths[0].Weight)
	})
}

func TestUpdateTraversalState(t *testing.T) {
	m := &Manager{}

	t.Run("Update state with decay", func(t *testing.T) {
		state := &pathTraversalState{
			pathTo: map[string][]string{
				"entity-1": {"entity-1"},
			},
			edgesTo: map[string][]GraphEdge{
				"entity-1": {},
			},
			scores: map[string]float64{
				"entity-1": 1.0,
			},
		}

		rel := &Relationship{
			FromEntityID: "entity-1",
			ToEntityID:   "entity-2",
			EdgeType:     "connects",
			Weight:       1.0,
		}

		pattern := PathPattern{DecayFactor: 0.5}
		m.updateTraversalState(state, "entity-1", "entity-2", rel, pattern, 0)

		assert.Equal(t, []string{"entity-1", "entity-2"}, state.pathTo["entity-2"])
		assert.Len(t, state.edgesTo["entity-2"], 1)
		assert.Equal(t, 0.5, state.scores["entity-2"])
	})

	t.Run("Update state without decay", func(t *testing.T) {
		state := &pathTraversalState{
			pathTo: map[string][]string{
				"entity-1": {"entity-1"},
			},
			edgesTo: map[string][]GraphEdge{
				"entity-1": {},
			},
			scores: map[string]float64{
				"entity-1": 1.0,
			},
		}

		rel := &Relationship{
			FromEntityID: "entity-1",
			ToEntityID:   "entity-2",
			EdgeType:     "connects",
			Weight:       1.0,
		}

		pattern := PathPattern{DecayFactor: 0} // No decay
		m.updateTraversalState(state, "entity-1", "entity-2", rel, pattern, 0)

		// Score should be parent * 1.0 (default decay when 0)
		assert.Equal(t, 1.0, state.scores["entity-2"])
	})

	t.Run("Handle zero weight relationship", func(t *testing.T) {
		state := &pathTraversalState{
			pathTo: map[string][]string{
				"entity-1": {"entity-1"},
			},
			edgesTo: map[string][]GraphEdge{
				"entity-1": {},
			},
			scores: map[string]float64{
				"entity-1": 1.0,
			},
		}

		rel := &Relationship{
			FromEntityID: "entity-1",
			ToEntityID:   "entity-2",
			EdgeType:     "connects",
			Weight:       0, // Zero weight should default to 1.0
		}

		pattern := PathPattern{DecayFactor: 0.5}
		m.updateTraversalState(state, "entity-1", "entity-2", rel, pattern, 0)

		// Edge weight should be 1.0 (default) * decay^(depth+1)
		require.Len(t, state.edgesTo["entity-2"], 1)
		assert.Equal(t, 0.5, state.edgesTo["entity-2"][0].Weight)
	})
}

func TestGeneratePathQueryKey(t *testing.T) {
	m := &Manager{}

	t.Run("Generate cache key", func(t *testing.T) {
		pattern := PathPattern{
			MaxDepth:  3,
			EdgeTypes: []string{"connects", "depends"},
			Direction: DirectionOutgoing,
		}

		key := m.generatePathQueryKey("entity-1", pattern)
		assert.Contains(t, key, "path:")
		assert.Contains(t, key, "entity-1")
		assert.Contains(t, key, "3")
	})

	t.Run("Different patterns produce different keys", func(t *testing.T) {
		pattern1 := PathPattern{MaxDepth: 3, Direction: DirectionOutgoing}
		pattern2 := PathPattern{MaxDepth: 3, Direction: DirectionIncoming}

		key1 := m.generatePathQueryKey("entity-1", pattern1)
		key2 := m.generatePathQueryKey("entity-1", pattern2)

		assert.NotEqual(t, key1, key2)
	})
}

func TestPathTraversalState(t *testing.T) {
	t.Run("Initialize state correctly", func(t *testing.T) {
		state := &pathTraversalState{
			visited:      make(map[string]bool),
			entities:     make(map[string]*gtypes.EntityState),
			pathTo:       make(map[string][]string),
			edgesTo:      make(map[string][]GraphEdge),
			scores:       make(map[string]float64),
			nodesVisited: 0,
			truncated:    false,
		}

		assert.Empty(t, state.visited)
		assert.Empty(t, state.entities)
		assert.Equal(t, 0, state.nodesVisited)
		assert.False(t, state.truncated)
	})

	t.Run("Track visited nodes", func(t *testing.T) {
		state := &pathTraversalState{
			visited: make(map[string]bool),
		}

		state.visited["node-1"] = true
		state.visited["node-2"] = true

		assert.True(t, state.visited["node-1"])
		assert.True(t, state.visited["node-2"])
		assert.False(t, state.visited["node-3"])
	})
}

func TestProcessNeighborFiltering(t *testing.T) {
	// Test the filtering logic without needing full Manager dependencies
	m := &Manager{}

	t.Run("EdgeTypes filter - match", func(t *testing.T) {
		result := m.matchesEdgeTypes("located_in", []string{"located_in", "connects_to"})
		assert.True(t, result)
	})

	t.Run("EdgeTypes filter - no match", func(t *testing.T) {
		result := m.matchesEdgeTypes("depends_on", []string{"located_in", "connects_to"})
		assert.False(t, result)
	})

	t.Run("NodeTypes filter - valid entity ID match", func(t *testing.T) {
		result := m.matchesNodeTypes("org.platform.domain.system.sensor.temp-01", []string{"sensor"})
		assert.True(t, result)
	})

	t.Run("NodeTypes filter - valid entity ID no match", func(t *testing.T) {
		result := m.matchesNodeTypes("org.platform.domain.system.sensor.temp-01", []string{"zone"})
		assert.False(t, result)
	})
}

// TestScoreMapKeyConsistency verifies that scores map keys match entity IDs.
// This test reproduces a bug where using a different key (e.g., input parameter)
// than entity.ID causes score lookup failures in the resolver.
func TestGetSiblingRelationships(t *testing.T) {
	t.Run("Valid EntityID returns siblings from mock reader", func(t *testing.T) {
		// Create a mock entity reader that returns sibling entity IDs
		mockReader := &mockEntityReaderWithPrefix{
			prefixResults: map[string][]string{
				"c360.logistics.environmental.sensor.temperature": {
					"c360.logistics.environmental.sensor.temperature.cold-storage-01",
					"c360.logistics.environmental.sensor.temperature.cold-storage-02",
					"c360.logistics.environmental.sensor.temperature.warehouse-a",
				},
			},
		}

		m := &Manager{
			entityReader: mockReader,
		}

		entityID := "c360.logistics.environmental.sensor.temperature.cold-storage-01"
		siblings, err := m.getSiblingRelationships(nil, entityID)

		require.NoError(t, err)
		require.Len(t, siblings, 2, "Should find 2 siblings (excluding self)")

		// Verify sibling properties
		for _, sibling := range siblings {
			assert.Equal(t, entityID, sibling.FromEntityID)
			assert.NotEqual(t, entityID, sibling.ToEntityID, "Should not include self")
			assert.Equal(t, "graph.rel.sibling", sibling.EdgeType)
			assert.Equal(t, 0.7, sibling.Weight, "Sibling weight should be 0.7")
			assert.True(t, sibling.Properties["inferred"].(bool), "Should be marked as inferred")
			assert.Equal(t, "type_prefix", sibling.Properties["inference"])
		}
	})

	t.Run("Invalid EntityID returns nil", func(t *testing.T) {
		m := &Manager{}

		// Invalid EntityID (not 6 parts)
		siblings, err := m.getSiblingRelationships(nil, "invalid-entity-id")

		assert.NoError(t, err, "Should not error for invalid EntityID, just return nil")
		assert.Nil(t, siblings, "Should return nil for invalid EntityID")
	})

	t.Run("Entity with no siblings returns empty slice", func(t *testing.T) {
		mockReader := &mockEntityReaderWithPrefix{
			prefixResults: map[string][]string{
				"c360.logistics.environmental.sensor.temperature": {
					"c360.logistics.environmental.sensor.temperature.only-one", // Just self
				},
			},
		}

		m := &Manager{
			entityReader: mockReader,
		}

		entityID := "c360.logistics.environmental.sensor.temperature.only-one"
		siblings, err := m.getSiblingRelationships(nil, entityID)

		require.NoError(t, err)
		assert.Empty(t, siblings, "Should return empty slice when no siblings exist")
	})
}

// mockEntityReaderWithPrefix is a mock implementation of EntityReader for testing
type mockEntityReaderWithPrefix struct {
	prefixResults map[string][]string
}

func (m *mockEntityReaderWithPrefix) GetEntity(_ context.Context, _ string) (*gtypes.EntityState, error) {
	return nil, nil
}

func (m *mockEntityReaderWithPrefix) ExistsEntity(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockEntityReaderWithPrefix) BatchGet(_ context.Context, _ []string) ([]*gtypes.EntityState, error) {
	return nil, nil
}

func (m *mockEntityReaderWithPrefix) ListWithPrefix(_ context.Context, prefix string) ([]string, error) {
	if results, ok := m.prefixResults[prefix]; ok {
		return results, nil
	}
	return []string{}, nil
}

func TestScoreMapKeyConsistency(t *testing.T) {
	m := &Manager{}

	t.Run("Scores map keys must match entity.ID for correct lookup", func(t *testing.T) {
		// Simulate the scenario where map key differs from entity.ID
		// This is the suspected root cause of the decay scoring bug
		startEntityID := "c360.logistics.content.document.operations.doc-ops-001"

		entity1 := &gtypes.EntityState{ID: startEntityID}
		entity2 := &gtypes.EntityState{ID: "c360.logistics.content.document.operations.doc-ops-002"}
		entity3 := &gtypes.EntityState{ID: "c360.logistics.content.document.operations.doc-ops-003"}

		state := &pathTraversalState{
			entities: map[string]*gtypes.EntityState{
				startEntityID: entity1,
				entity2.ID:    entity2,
				entity3.ID:    entity3,
			},
			scores: map[string]float64{
				startEntityID: 1.0,
				entity2.ID:    0.8,
				entity3.ID:    0.64,
			},
			pathTo: map[string][]string{
				startEntityID: {startEntityID},
				entity2.ID:    {startEntityID, entity2.ID},
				entity3.ID:    {startEntityID, entity2.ID, entity3.ID},
			},
			edgesTo: map[string][]GraphEdge{
				startEntityID: {},
				entity2.ID:    {{From: startEntityID, To: entity2.ID, Weight: 0.8}},
				entity3.ID:    {{From: entity2.ID, To: entity3.ID, Weight: 0.64}},
			},
		}

		pattern := PathPattern{IncludeSelf: true}
		result := m.buildPathResult(state, pattern)

		// Verify all entities are present
		require.Equal(t, 3, result.Count)
		require.Len(t, result.Entities, 3)

		// CRITICAL: Verify that scores map keys match entity IDs
		// This is the invariant that must hold for correct score lookup
		for _, entity := range result.Entities {
			score, found := result.Scores[entity.ID]
			assert.True(t, found, "Score not found for entity %s - map key mismatch!", entity.ID)
			assert.Greater(t, score, 0.0, "Entity %s has zero score, likely lookup failure", entity.ID)
		}

		// Verify start entity has score 1.0
		assert.Equal(t, 1.0, result.Scores[startEntityID], "Start entity should have score 1.0")
	})

	t.Run("Documents ID mismatch behavior for regression prevention", func(t *testing.T) {
		// This test documents what happens if map keys don't match entity.ID.
		// The fix in ExecutePath prevents this from happening by always using
		// entity.ID as the map key. This test ensures we understand the behavior
		// and have debug logging to catch any future regressions.

		inputParam := "start-entity-input"         // Key used in state maps
		actualEntityID := "start-entity-actual-id" // Entity's actual ID field (different!)

		entity1 := &gtypes.EntityState{ID: actualEntityID} // Note: ID differs from map key!
		entity2 := &gtypes.EntityState{ID: "entity-2"}

		state := &pathTraversalState{
			entities: map[string]*gtypes.EntityState{
				inputParam: entity1, // Using inputParam as key, but entity.ID is different
				entity2.ID: entity2,
			},
			scores: map[string]float64{
				inputParam: 1.0, // Score stored under inputParam
				entity2.ID: 0.8,
			},
			pathTo: map[string][]string{
				inputParam: {inputParam},
				entity2.ID: {inputParam, entity2.ID},
			},
			edgesTo: map[string][]GraphEdge{
				inputParam: {},
				entity2.ID: {{From: inputParam, To: entity2.ID, Weight: 0.8}},
			},
		}

		pattern := PathPattern{IncludeSelf: true}
		result := m.buildPathResult(state, pattern)

		// Document the expected behavior: score lookup fails when keys don't match
		// This is caught by debug logging in buildPathResult
		_, foundByActualID := result.Scores[entity1.ID]
		_, foundByMapKey := result.Scores[inputParam]

		// Score is stored under inputParam, not actualEntityID
		assert.False(t, foundByActualID, "Score should NOT be found by entity.ID when keys mismatch")
		assert.True(t, foundByMapKey, "Score should be found by original map key")

		// The fix in ExecutePath ensures this mismatch never occurs in practice
		// by always using entity.ID as the map key
	})
}
