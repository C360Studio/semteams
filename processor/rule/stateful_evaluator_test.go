// Package rule - Tests for stateful rule evaluation
package rule

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestStatefulEvaluator_EvaluateWithState tests state-based action firing
func TestStatefulEvaluator_EvaluateWithState(t *testing.T) {
	tests := []struct {
		name               string
		previousMatching   bool
		currentlyMatching  bool
		wantTransition     Transition
		wantOnEnterCalls   int
		wantOnExitCalls    int
		wantWhileTrueCalls int
	}{
		{
			name:              "false to true fires on_enter",
			previousMatching:  false,
			currentlyMatching: true,
			wantTransition:    TransitionEntered,
			wantOnEnterCalls:  1,
		},
		{
			name:              "true to false fires on_exit",
			previousMatching:  true,
			currentlyMatching: false,
			wantTransition:    TransitionExited,
			wantOnExitCalls:   1,
		},
		{
			name:               "true to true fires while_true",
			previousMatching:   true,
			currentlyMatching:  true,
			wantTransition:     TransitionNone,
			wantWhileTrueCalls: 1,
		},
		{
			name:              "false to false fires nothing",
			previousMatching:  false,
			currentlyMatching: false,
			wantTransition:    TransitionNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock KV bucket
			bucket := newMockKVBucket()
			logger := slog.Default()

			// Create StateTracker
			stateTracker := NewStateTracker(bucket, logger)

			// Create mock action executor to count action executions
			actionExecutor := &mockActionExecutor{
				onEnterCalls:   0,
				onExitCalls:    0,
				whileTrueCalls: 0,
			}

			// Create StatefulEvaluator
			evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

			// Create rule definition with different action sets
			ruleDef := Definition{
				ID:   "test-rule",
				Type: "expression",
				Name: "Test Rule",
				OnEnter: []Action{
					{Type: ActionTypePublish, Subject: "test.entered"},
				},
				OnExit: []Action{
					{Type: ActionTypePublish, Subject: "test.exited"},
				},
				WhileTrue: []Action{
					{Type: ActionTypePublish, Subject: "test.while-true"},
				},
			}

			entityID := "entity-123"
			entityKey := entityID

			// Set up previous state if needed
			if tt.previousMatching {
				prevState := MatchState{
					RuleID:         ruleDef.ID,
					EntityKey:      entityKey,
					IsMatching:     true,
					LastTransition: string(TransitionEntered),
					TransitionAt:   time.Now().Add(-1 * time.Minute),
					LastChecked:    time.Now(),
				}
				err := stateTracker.Set(ctx, prevState)
				if err != nil {
					t.Fatalf("Failed to set previous state: %v", err)
				}
			}

			// Execute evaluation
			transition, err := evaluator.EvaluateWithState(
				ctx,
				ruleDef,
				entityID,
				"", // No related entity for single-entity rule
				tt.currentlyMatching,
			)

			// Verify no error
			if err != nil {
				t.Errorf("EvaluateWithState() error = %v, want nil", err)
			}

			// Verify transition type
			if transition != tt.wantTransition {
				t.Errorf("EvaluateWithState() transition = %v, want %v", transition, tt.wantTransition)
			}

			// Verify action execution counts
			if actionExecutor.onEnterCalls != tt.wantOnEnterCalls {
				t.Errorf("OnEnter actions called %d times, want %d", actionExecutor.onEnterCalls, tt.wantOnEnterCalls)
			}

			if actionExecutor.onExitCalls != tt.wantOnExitCalls {
				t.Errorf("OnExit actions called %d times, want %d", actionExecutor.onExitCalls, tt.wantOnExitCalls)
			}

			if actionExecutor.whileTrueCalls != tt.wantWhileTrueCalls {
				t.Errorf("WhileTrue actions called %d times, want %d", actionExecutor.whileTrueCalls, tt.wantWhileTrueCalls)
			}

			// Verify state was persisted correctly
			newState, err := stateTracker.Get(ctx, ruleDef.ID, entityKey)
			if err != nil {
				t.Errorf("Failed to get new state: %v", err)
			}

			if newState.IsMatching != tt.currentlyMatching {
				t.Errorf("Persisted state IsMatching = %v, want %v", newState.IsMatching, tt.currentlyMatching)
			}

			if newState.LastTransition != string(transition) {
				t.Errorf("Persisted state LastTransition = %v, want %v", newState.LastTransition, string(transition))
			}
		})
	}
}

// TestStatefulEvaluator_EvaluateWithState_NoInitialState tests handling when no previous state exists
func TestStatefulEvaluator_EvaluateWithState_NoInitialState(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()

	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}

	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	ruleDef := Definition{
		ID:   "test-rule",
		Type: "expression",
		Name: "Test Rule",
		OnEnter: []Action{
			{Type: ActionTypePublish, Subject: "test.entered"},
		},
	}

	entityID := "entity-new"

	// Evaluate with currentlyMatching=true and no previous state
	// Should treat as false→true transition
	transition, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true)

	if err != nil {
		t.Fatalf("EvaluateWithState() error = %v", err)
	}

	if transition != TransitionEntered {
		t.Errorf("Expected TransitionEntered, got %v", transition)
	}

	if actionExecutor.onEnterCalls != 1 {
		t.Errorf("Expected OnEnter to be called once, got %d calls", actionExecutor.onEnterCalls)
	}
}

// TestStatefulEvaluator_EvaluateWithState_PairRule tests pair rules with two entity IDs
func TestStatefulEvaluator_EvaluateWithState_PairRule(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()

	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}

	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	ruleDef := Definition{
		ID:   "pair-rule",
		Type: "pair",
		Name: "Pair Rule",
		OnEnter: []Action{
			{Type: ActionTypeAddTriple, Predicate: "related_to", Object: "$related.id"},
		},
	}

	entity1 := "entity-a"
	entity2 := "entity-b"

	// First evaluation: false→true
	transition1, err := evaluator.EvaluateWithState(ctx, ruleDef, entity1, entity2, true)
	if err != nil {
		t.Fatalf("First evaluation error: %v", err)
	}

	if transition1 != TransitionEntered {
		t.Errorf("First transition = %v, want TransitionEntered", transition1)
	}

	if actionExecutor.onEnterCalls != 1 {
		t.Errorf("OnEnter calls = %d, want 1", actionExecutor.onEnterCalls)
	}

	// Verify state key is canonical (sorted)
	expectedKey := buildPairKey(entity1, entity2)
	state, err := stateTracker.Get(ctx, ruleDef.ID, expectedKey)
	if err != nil {
		t.Fatalf("Failed to get state: %v", err)
	}

	if state.EntityKey != expectedKey {
		t.Errorf("State EntityKey = %v, want %v", state.EntityKey, expectedKey)
	}
}

// TestStatefulEvaluator_EvaluateWithState_MultipleActions tests executing multiple actions
func TestStatefulEvaluator_EvaluateWithState_MultipleActions(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()

	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}

	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	// Rule with multiple OnEnter actions
	ruleDef := Definition{
		ID:   "multi-action-rule",
		Type: "expression",
		Name: "Multi-Action Rule",
		OnEnter: []Action{
			{Type: ActionTypePublish, Subject: "test.action1"},
			{Type: ActionTypePublish, Subject: "test.action2"},
			{Type: ActionTypeAddTriple, Predicate: "status", Object: "active"},
		},
	}

	entityID := "entity-multi"

	transition, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true)
	if err != nil {
		t.Fatalf("EvaluateWithState() error = %v", err)
	}

	if transition != TransitionEntered {
		t.Errorf("Expected TransitionEntered, got %v", transition)
	}

	// Should execute all 3 OnEnter actions
	if actionExecutor.executeCallCount != 3 {
		t.Errorf("Expected 3 action executions, got %d", actionExecutor.executeCallCount)
	}
}

// mockActionExecutor tracks action execution calls for testing
type mockActionExecutor struct {
	onEnterCalls     int
	onExitCalls      int
	whileTrueCalls   int
	executeCallCount int
}

func (m *mockActionExecutor) Execute(_ context.Context, action Action, _ string, _ string) error {
	m.executeCallCount++

	// Track which action set was executed based on subject patterns
	if action.Subject == "test.entered" {
		m.onEnterCalls++
	} else if action.Subject == "test.exited" {
		m.onExitCalls++
	} else if action.Subject == "test.while-true" {
		m.whileTrueCalls++
	}

	// Handle multi-action test
	if action.Subject == "test.action1" || action.Subject == "test.action2" {
		m.onEnterCalls++
	}

	if action.Type == ActionTypeAddTriple {
		m.onEnterCalls++
	}

	return nil
}
