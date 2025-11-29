// Package rule - Tests for Stateful Rules (TDD - RED Phase)
package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// T036: Test StatefulRule OnEnter - fires exactly once when condition transitions false→true
func TestStatefulRule_OnEnter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		initialMatch    bool
		subsequentMatch bool
		expectOnEnter   bool
	}{
		{
			name:            "false to true triggers on_enter",
			initialMatch:    false,
			subsequentMatch: true,
			expectOnEnter:   true,
		},
		{
			name:            "true to true does NOT trigger on_enter",
			initialMatch:    true,
			subsequentMatch: true,
			expectOnEnter:   false,
		},
		{
			name:            "false to false does NOT trigger on_enter",
			initialMatch:    false,
			subsequentMatch: false,
			expectOnEnter:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Detect transition (pure function, no KV store needed)
			transition := DetectTransition(tt.initialMatch, tt.subsequentMatch)

			// Verify on_enter should fire
			if tt.expectOnEnter {
				assert.Equal(t, TransitionEntered, transition)
			} else {
				assert.NotEqual(t, TransitionEntered, transition)
			}
		})
	}
}

// T037: Test StatefulRule OnExit - fires exactly once when condition transitions true→false
func TestStatefulRule_OnExit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		initialMatch    bool
		currentMatch    bool
		expectOnExit    bool
		expectOnEnter   bool
		expectWhileTrue bool
	}{
		{
			name:            "true to false triggers on_exit",
			initialMatch:    true,
			currentMatch:    false,
			expectOnExit:    true,
			expectOnEnter:   false,
			expectWhileTrue: false,
		},
		{
			name:            "false to false does NOT trigger on_exit",
			initialMatch:    false,
			currentMatch:    false,
			expectOnExit:    false,
			expectOnEnter:   false,
			expectWhileTrue: false,
		},
		{
			name:            "true to true does NOT trigger on_exit",
			initialMatch:    true,
			currentMatch:    true,
			expectOnExit:    false,
			expectOnEnter:   false,
			expectWhileTrue: true, // Should trigger while_true instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Detect transition (pure function, no KV store needed)
			transition := DetectTransition(tt.initialMatch, tt.currentMatch)

			// Verify on_exit should fire
			if tt.expectOnExit {
				assert.Equal(t, TransitionExited, transition)
			}

			// Verify on_enter should NOT fire
			if tt.expectOnEnter {
				assert.Equal(t, TransitionEntered, transition)
			} else {
				assert.NotEqual(t, TransitionEntered, transition)
			}

			// Verify while_true conditions
			if tt.expectWhileTrue {
				assert.Equal(t, TransitionNone, transition)
				assert.True(t, tt.currentMatch, "while_true requires current match to be true")
			}
		})
	}
}

// T038: Test StatefulRule NoDuplicateOnEnter - on_enter does NOT fire repeatedly when condition stays true
func TestStatefulRule_NoDuplicateOnEnter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		evaluations      []bool // Sequence of match results
		expectEnterCount int
		expectExitCount  int
	}{
		{
			name:             "single false→true transition",
			evaluations:      []bool{false, true, true, true},
			expectEnterCount: 1, // Only first transition
			expectExitCount:  0,
		},
		{
			name:             "multiple transitions",
			evaluations:      []bool{false, true, true, false, true, true},
			expectEnterCount: 2, // Two false→true transitions
			expectExitCount:  1, // One true→false transition
		},
		{
			name:             "always false",
			evaluations:      []bool{false, false, false},
			expectEnterCount: 0,
			expectExitCount:  0,
		},
		{
			name:             "always true (after initial transition)",
			evaluations:      []bool{true, true, true}, // Starts true
			expectEnterCount: 0,                        // No transition from initial state
			expectExitCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enterCount := 0
			exitCount := 0

			// Simulate evaluations over time (pure function test, no KV store needed)
			var previousMatch bool
			for i, currentMatch := range tt.evaluations {
				// On first iteration, there's no previous state
				if i == 0 {
					previousMatch = currentMatch
					continue
				}

				transition := DetectTransition(previousMatch, currentMatch)

				switch transition {
				case TransitionEntered:
					enterCount++
				case TransitionExited:
					exitCount++
				}

				previousMatch = currentMatch
			}

			assert.Equal(t, tt.expectEnterCount, enterCount, "on_enter should fire exactly %d times", tt.expectEnterCount)
			assert.Equal(t, tt.expectExitCount, exitCount, "on_exit should fire exactly %d times", tt.expectExitCount)
		})
	}
}

// T038a: Test StatefulRule WhileTrue - fires on every update while condition is true
func TestStatefulRule_WhileTrue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		previousMatch   bool
		currentMatch    bool
		expectWhileTrue bool
	}{
		{
			name:            "true to true triggers while_true",
			previousMatch:   true,
			currentMatch:    true,
			expectWhileTrue: true,
		},
		{
			name:            "false to true does NOT trigger while_true (on_enter instead)",
			previousMatch:   false,
			currentMatch:    true,
			expectWhileTrue: false,
		},
		{
			name:            "true to false does NOT trigger while_true",
			previousMatch:   true,
			currentMatch:    false,
			expectWhileTrue: false,
		},
		{
			name:            "false to false does NOT trigger while_true",
			previousMatch:   false,
			currentMatch:    false,
			expectWhileTrue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transition := DetectTransition(tt.previousMatch, tt.currentMatch)

			// while_true should fire when:
			// 1. No transition occurred (TransitionNone)
			// 2. Current match is true
			shouldFireWhileTrue := (transition == TransitionNone) && tt.currentMatch

			assert.Equal(t, tt.expectWhileTrue, shouldFireWhileTrue)
		})
	}
}

// T038b: Test StateTracker key generation for entity pairs
func TestStateTracker_EntityPairKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entity1 string
		entity2 string
		wantKey string
	}{
		{
			name:    "alphabetical order",
			entity1: "c360.platform1.robotics.mav1.drone.001",
			entity2: "c360.platform1.robotics.mav1.drone.002",
			wantKey: "c360.platform1.robotics.mav1.drone.001_c360.platform1.robotics.mav1.drone.002",
		},
		{
			name:    "reverse alphabetical should sort",
			entity1: "c360.platform1.robotics.mav1.drone.002",
			entity2: "c360.platform1.robotics.mav1.drone.001",
			wantKey: "c360.platform1.robotics.mav1.drone.001_c360.platform1.robotics.mav1.drone.002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function doesn't exist yet - will fail
			key := buildPairKey(tt.entity1, tt.entity2)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}
