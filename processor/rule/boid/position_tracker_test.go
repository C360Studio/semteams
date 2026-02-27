package boid

import (
	"testing"
)

func TestCalculateVelocity(t *testing.T) {
	tests := []struct {
		name     string
		oldFocus []string
		newFocus []string
		expected float64
	}{
		{
			name:     "no change",
			oldFocus: []string{"a", "b", "c"},
			newFocus: []string{"a", "b", "c"},
			expected: 0.0,
		},
		{
			name:     "complete change",
			oldFocus: []string{"a", "b"},
			newFocus: []string{"c", "d"},
			expected: 1.0,
		},
		{
			name:     "partial change",
			oldFocus: []string{"a", "b"},
			newFocus: []string{"b", "c"},
			expected: 0.5, // 2 changes out of 4 total
		},
		{
			name:     "empty old",
			oldFocus: []string{},
			newFocus: []string{"a", "b"},
			expected: 1.0,
		},
		{
			name:     "empty new",
			oldFocus: []string{"a", "b"},
			newFocus: []string{},
			expected: 1.0,
		},
		{
			name:     "both empty",
			oldFocus: []string{},
			newFocus: []string{},
			expected: 0.0,
		},
		{
			name:     "addition only",
			oldFocus: []string{"a"},
			newFocus: []string{"a", "b"},
			expected: 1.0 / 3.0, // 1 addition out of 3 total
		},
		{
			name:     "removal only",
			oldFocus: []string{"a", "b"},
			newFocus: []string{"a"},
			expected: 1.0 / 3.0, // 1 removal out of 3 total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateVelocity(tt.oldFocus, tt.newFocus)
			tolerance := 0.01
			if result < tt.expected-tolerance || result > tt.expected+tolerance {
				t.Errorf("got %f, want %f (±%f)", result, tt.expected, tolerance)
			}
		})
	}
}
