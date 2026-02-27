package agenticloop

import (
	"testing"
)

func TestBoidHandler_ExtractEntitiesFromToolResult(t *testing.T) {
	handler := NewBoidHandler(nil, nil)

	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "no entity IDs",
			content:  "Just some regular text without entity IDs",
			expected: nil,
		},
		{
			name:    "single entity ID",
			content: "Found entity: acme.ops.robotics.gcs.drone.001",
			expected: []string{
				"acme.ops.robotics.gcs.drone.001",
			},
		},
		{
			name:    "multiple entity IDs",
			content: "Entities: acme.ops.robotics.gcs.drone.001 and acme.ops.robotics.gcs.sensor.temp-01",
			expected: []string{
				"acme.ops.robotics.gcs.drone.001",
				"acme.ops.robotics.gcs.sensor.temp-01",
			},
		},
		{
			name:    "duplicate entity IDs",
			content: "Entity acme.ops.robotics.gcs.drone.001 mentioned again acme.ops.robotics.gcs.drone.001",
			expected: []string{
				"acme.ops.robotics.gcs.drone.001",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.ExtractEntitiesFromToolResult(tt.content)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entities, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("entity %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestBoidHandler_ExtractEntitiesFromContext(t *testing.T) {
	handler := NewBoidHandler(nil, nil)

	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "no entity markers",
			content:  "Regular context without entity markers",
			expected: nil,
		},
		{
			name:    "single entity marker",
			content: "[Entity: acme.ops.robotics.gcs.drone.001] Some context",
			expected: []string{
				"acme.ops.robotics.gcs.drone.001",
			},
		},
		{
			name:    "multiple entity markers",
			content: "[Entity: acme.ops.robotics.gcs.drone.001] First context [Entity: acme.ops.robotics.gcs.sensor.temp-01] Second context",
			expected: []string{
				"acme.ops.robotics.gcs.drone.001",
				"acme.ops.robotics.gcs.sensor.temp-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.ExtractEntitiesFromContext(tt.content)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entities, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("entity %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestBoidHandler_CalculateVelocity(t *testing.T) {
	handler := NewBoidHandler(nil, nil)

	tests := []struct {
		name      string
		oldFocus  []string
		newFocus  []string
		wantRange [2]float64 // [min, max]
	}{
		{
			name:      "no change",
			oldFocus:  []string{"a", "b", "c"},
			newFocus:  []string{"a", "b", "c"},
			wantRange: [2]float64{0.0, 0.1},
		},
		{
			name:      "complete change",
			oldFocus:  []string{"a", "b", "c"},
			newFocus:  []string{"d", "e", "f"},
			wantRange: [2]float64{0.9, 1.0},
		},
		{
			name:      "partial change",
			oldFocus:  []string{"a", "b", "c"},
			newFocus:  []string{"a", "d", "e"},
			wantRange: [2]float64{0.3, 0.7},
		},
		{
			name:      "empty old",
			oldFocus:  []string{},
			newFocus:  []string{"a", "b"},
			wantRange: [2]float64{0.9, 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			velocity := handler.CalculateVelocity(tt.oldFocus, tt.newFocus)

			if velocity < tt.wantRange[0] || velocity > tt.wantRange[1] {
				t.Errorf("velocity %v not in expected range [%v, %v]",
					velocity, tt.wantRange[0], tt.wantRange[1])
			}
		})
	}
}

func TestMergeEntities(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		newEnts  []string
		maxCount int
		expected []string
	}{
		{
			name:     "empty both",
			existing: nil,
			newEnts:  nil,
			maxCount: 10,
			expected: []string{},
		},
		{
			name:     "new entities only",
			existing: nil,
			newEnts:  []string{"a", "b"},
			maxCount: 10,
			expected: []string{"a", "b"},
		},
		{
			name:     "existing entities only",
			existing: []string{"a", "b"},
			newEnts:  nil,
			maxCount: 10,
			expected: []string{"a", "b"},
		},
		{
			name:     "merge without duplicates",
			existing: []string{"a", "b"},
			newEnts:  []string{"c", "d"},
			maxCount: 10,
			expected: []string{"c", "d", "a", "b"},
		},
		{
			name:     "merge with duplicates",
			existing: []string{"a", "b"},
			newEnts:  []string{"b", "c"},
			maxCount: 10,
			expected: []string{"b", "c", "a"},
		},
		{
			name:     "limit exceeded",
			existing: []string{"a", "b", "c"},
			newEnts:  []string{"d", "e"},
			maxCount: 3,
			expected: []string{"d", "e", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeEntities(tt.existing, tt.newEnts, tt.maxCount)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entities, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("entity %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}
