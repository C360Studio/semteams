package querymanager

import (
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
