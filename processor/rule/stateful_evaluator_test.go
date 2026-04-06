// Package rule - Tests for stateful rule evaluation
package rule

import (
	"context"
	"log/slog"
	"testing"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/rule/expression"
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
				OnExit: []Action{
					{Type: ActionTypePublish, Subject: "test.exited"},
				},
				WhileTrue: []Action{
					{Type: ActionTypePublish, Subject: "test.while-true"},
				},
			}

			entityID := "entity-123"
			entityKey := entityID

			if tt.previousMatching {
				prevState := MatchState{
					RuleID:         ruleDef.ID,
					EntityKey:      entityKey,
					IsMatching:     true,
					LastTransition: string(TransitionEntered),
					TransitionAt:   time.Now().Add(-1 * time.Minute),
					LastChecked:    time.Now(),
				}
				if err := stateTracker.Set(ctx, prevState); err != nil {
					t.Fatalf("Failed to set previous state: %v", err)
				}
			}

			transition, err := evaluator.EvaluateWithState(
				ctx, ruleDef, entityID, "", tt.currentlyMatching, nil, nil,
			)

			if err != nil {
				t.Errorf("EvaluateWithState() error = %v, want nil", err)
			}
			if transition != tt.wantTransition {
				t.Errorf("EvaluateWithState() transition = %v, want %v", transition, tt.wantTransition)
			}
			if actionExecutor.onEnterCalls != tt.wantOnEnterCalls {
				t.Errorf("OnEnter actions called %d times, want %d", actionExecutor.onEnterCalls, tt.wantOnEnterCalls)
			}
			if actionExecutor.onExitCalls != tt.wantOnExitCalls {
				t.Errorf("OnExit actions called %d times, want %d", actionExecutor.onExitCalls, tt.wantOnExitCalls)
			}
			if actionExecutor.whileTrueCalls != tt.wantWhileTrueCalls {
				t.Errorf("WhileTrue actions called %d times, want %d", actionExecutor.whileTrueCalls, tt.wantWhileTrueCalls)
			}

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

// TestStatefulEvaluator_NoInitialState tests handling when no previous state exists
func TestStatefulEvaluator_NoInitialState(t *testing.T) {
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

	transition, err := evaluator.EvaluateWithState(ctx, ruleDef, "entity-new", "", true, nil, nil)
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

// TestStatefulEvaluator_PairRule tests pair rules with two entity IDs
func TestStatefulEvaluator_PairRule(t *testing.T) {
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

	transition1, err := evaluator.EvaluateWithState(ctx, ruleDef, entity1, entity2, true, nil, nil)
	if err != nil {
		t.Fatalf("First evaluation error: %v", err)
	}
	if transition1 != TransitionEntered {
		t.Errorf("First transition = %v, want TransitionEntered", transition1)
	}
	if actionExecutor.onEnterCalls != 1 {
		t.Errorf("OnEnter calls = %d, want 1", actionExecutor.onEnterCalls)
	}

	expectedKey := buildPairKey(entity1, entity2)
	state, err := stateTracker.Get(ctx, ruleDef.ID, expectedKey)
	if err != nil {
		t.Fatalf("Failed to get state: %v", err)
	}
	if state.EntityKey != expectedKey {
		t.Errorf("State EntityKey = %v, want %v", state.EntityKey, expectedKey)
	}
}

// TestStatefulEvaluator_MultipleActions tests executing multiple actions
func TestStatefulEvaluator_MultipleActions(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

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

	transition, err := evaluator.EvaluateWithState(ctx, ruleDef, "entity-multi", "", true, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateWithState() error = %v", err)
	}
	if transition != TransitionEntered {
		t.Errorf("Expected TransitionEntered, got %v", transition)
	}
	if actionExecutor.executeCallCount != 3 {
		t.Errorf("Expected 3 action executions, got %d", actionExecutor.executeCallCount)
	}
}

// TestStatefulEvaluator_Iteration tests iteration tracking across transitions
func TestStatefulEvaluator_Iteration(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	ruleDef := Definition{
		ID:            "iter-rule",
		Type:          "expression",
		Name:          "Iteration Rule",
		MaxIterations: 3,
		OnEnter: []Action{
			{Type: ActionTypePublish, Subject: "test.entered"},
		},
	}

	entityID := "entity-iter"

	// First entry: iteration should be 1
	_, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true, nil, nil)
	if err != nil {
		t.Fatalf("First entry error: %v", err)
	}
	state, _ := stateTracker.Get(ctx, ruleDef.ID, entityID)
	if state.Iteration != 1 {
		t.Errorf("After first entry: iteration = %d, want 1", state.Iteration)
	}
	if state.MaxIterations != 3 {
		t.Errorf("MaxIterations = %d, want 3", state.MaxIterations)
	}

	// Exit: iteration preserved
	_, err = evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", false, nil, nil)
	if err != nil {
		t.Fatalf("Exit error: %v", err)
	}
	state, _ = stateTracker.Get(ctx, ruleDef.ID, entityID)
	if state.Iteration != 1 {
		t.Errorf("After exit: iteration = %d, want 1 (preserved)", state.Iteration)
	}

	// Second entry: iteration should be 2
	_, err = evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true, nil, nil)
	if err != nil {
		t.Fatalf("Second entry error: %v", err)
	}
	state, _ = stateTracker.Get(ctx, ruleDef.ID, entityID)
	if state.Iteration != 2 {
		t.Errorf("After second entry: iteration = %d, want 2", state.Iteration)
	}

	// Third entry: iteration should be 3
	_, _ = evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", false, nil, nil)
	_, err = evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true, nil, nil)
	if err != nil {
		t.Fatalf("Third entry error: %v", err)
	}
	state, _ = stateTracker.Get(ctx, ruleDef.ID, entityID)
	if state.Iteration != 3 {
		t.Errorf("After third entry: iteration = %d, want 3", state.Iteration)
	}
}

// TestStatefulEvaluator_WhenClause tests conditional action execution
func TestStatefulEvaluator_WhenClause(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	entity := &gtypes.EntityState{
		ID: "entity-when",
		Triples: []message.Triple{
			{Subject: "entity-when", Predicate: "review.verdict", Object: "approved"},
			{Subject: "entity-when", Predicate: "status", Object: "complete"},
		},
	}

	ruleDef := Definition{
		ID:   "when-rule",
		Type: "expression",
		Name: "When Rule",
		OnEnter: []Action{
			{
				Type:    ActionTypePublish,
				Subject: "test.approved",
				When: []expression.ConditionExpression{
					{Field: "review.verdict", Operator: "eq", Value: "approved"},
				},
			},
			{
				Type:    ActionTypePublish,
				Subject: "test.rejected",
				When: []expression.ConditionExpression{
					{Field: "review.verdict", Operator: "eq", Value: "rejected"},
				},
			},
			{
				Type:    ActionTypePublish,
				Subject: "test.always",
				// No When clause — always executes
			},
		},
	}

	_, err := evaluator.EvaluateWithState(ctx, ruleDef, entity.ID, "", true, entity, nil)
	if err != nil {
		t.Fatalf("EvaluateWithState() error = %v", err)
	}

	// Should execute: test.approved (matches) and test.always (no When)
	// Should skip: test.rejected (doesn't match)
	if actionExecutor.executeCallCount != 2 {
		t.Errorf("Expected 2 actions executed, got %d", actionExecutor.executeCallCount)
	}
}

// TestStatefulEvaluator_WhenWithStateFields tests $state.* fields in When clauses
func TestStatefulEvaluator_WhenWithStateFields(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	ruleDef := Definition{
		ID:            "budget-rule",
		Type:          "expression",
		Name:          "Budget Rule",
		MaxIterations: 3,
		OnEnter: []Action{
			{
				Type:    ActionTypePublish,
				Subject: "test.retry",
				When: []expression.ConditionExpression{
					{Field: "$state.iteration", Operator: "lte", Value: float64(3)},
				},
			},
			{
				Type:    ActionTypePublish,
				Subject: "test.escalate",
				When: []expression.ConditionExpression{
					{Field: "$state.iteration", Operator: "gt", Value: float64(3)},
				},
			},
		},
	}

	entityID := "entity-budget"

	// First 3 entries: retry action fires, escalate doesn't
	for i := range 3 {
		actionExecutor.executeCallCount = 0
		// Exit then enter to trigger TransitionEntered each time
		if i > 0 {
			_, _ = evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", false, nil, nil)
		}
		_, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true, nil, nil)
		if err != nil {
			t.Fatalf("Entry %d error: %v", i+1, err)
		}
		if actionExecutor.executeCallCount != 1 {
			t.Errorf("Entry %d: expected 1 action (retry), got %d", i+1, actionExecutor.executeCallCount)
		}
	}

	// Fourth entry: escalate action fires, retry doesn't
	_, _ = evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", false, nil, nil)
	actionExecutor.executeCallCount = 0
	_, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true, nil, nil)
	if err != nil {
		t.Fatalf("Fourth entry error: %v", err)
	}
	if actionExecutor.executeCallCount != 1 {
		t.Errorf("Fourth entry: expected 1 action (escalate), got %d", actionExecutor.executeCallCount)
	}
}

// TestStatefulEvaluator_WhenNilEntity tests When clause behavior with nil entity
func TestStatefulEvaluator_WhenNilEntity(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	ruleDef := Definition{
		ID:   "nil-entity-rule",
		Type: "expression",
		Name: "Nil Entity Rule",
		OnEnter: []Action{
			{
				Type:    ActionTypePublish,
				Subject: "test.guarded",
				When: []expression.ConditionExpression{
					// This references a triple field but entity is nil
					{Field: "some.field", Operator: "eq", Value: "x"},
				},
			},
			{
				Type:    ActionTypePublish,
				Subject: "test.state-guarded",
				When: []expression.ConditionExpression{
					// $state.* fields work even without entity
					{Field: "$state.iteration", Operator: "eq", Value: float64(1)},
				},
			},
		},
	}

	_, err := evaluator.EvaluateWithState(ctx, ruleDef, "entity-nil", "", true, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateWithState() error = %v", err)
	}

	// guarded: entity is nil, triple field eval fails → skipped
	// state-guarded: $state.iteration == 1 → executes
	if actionExecutor.executeCallCount != 1 {
		t.Errorf("Expected 1 action executed (state-guarded only), got %d", actionExecutor.executeCallCount)
	}
}

// mockActionExecutor tracks action execution calls for testing
type mockActionExecutor struct {
	onEnterCalls     int
	onExitCalls      int
	whileTrueCalls   int
	executeCallCount int
}

func (m *mockActionExecutor) Execute(_ context.Context, action Action, _ *ExecutionContext) error {
	m.executeCallCount++

	switch action.Subject {
	case "test.entered":
		m.onEnterCalls++
	case "test.exited":
		m.onExitCalls++
	case "test.while-true":
		m.whileTrueCalls++
	case "test.action1", "test.action2":
		m.onEnterCalls++
	}

	if action.Type == ActionTypeAddTriple {
		m.onEnterCalls++
	}

	return nil
}

// TestStatefulEvaluator_TransitionFieldTracking verifies that FieldValues are
// persisted across evaluations and used by the transition operator.
func TestStatefulEvaluator_TransitionFieldTracking(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	ruleDef := Definition{
		ID:   "transition-rule",
		Type: "expression",
		Name: "Test Transition Rule",
		Conditions: []expression.ConditionExpression{
			{
				Field:    "workflow.plan.status",
				Operator: expression.OpTransition,
				Value:    "drafting",
				From:     []interface{}{"created", "rejected"},
			},
		},
		OnEnter: []Action{
			{Type: ActionTypePublish, Subject: "test.entered"},
		},
	}

	entityID := "plan.001"
	entityCreated := &gtypes.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{Subject: entityID, Predicate: "workflow.plan.status", Object: "created"},
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}
	entityDrafting := &gtypes.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{Subject: entityID, Predicate: "workflow.plan.status", Object: "drafting"},
		},
		Version:   2,
		UpdatedAt: time.Now(),
	}

	// First evaluation: entity has status "created"
	// The transition condition checks "is status transitioning to drafting?" — no, it's "created"
	// This should NOT match, but it should capture "created" in FieldValues
	transition1, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", false, entityCreated, nil)
	if err != nil {
		t.Fatalf("First evaluation error: %v", err)
	}
	if transition1 != TransitionNone {
		t.Errorf("First evaluation: expected TransitionNone, got %v", transition1)
	}

	// Verify FieldValues were captured
	state1, err := stateTracker.Get(ctx, ruleDef.ID, entityID)
	if err != nil {
		t.Fatalf("Failed to get state after first eval: %v", err)
	}
	if state1.FieldValues == nil {
		t.Fatal("FieldValues should be captured after first evaluation")
	}
	if state1.FieldValues["workflow.plan.status"] != "created" {
		t.Errorf("Expected FieldValues[workflow.plan.status] = 'created', got %q", state1.FieldValues["workflow.plan.status"])
	}

	// Second evaluation: entity now has status "drafting"
	// The transition condition checks: current = "drafting" (matches Value),
	// previous = "created" (in From set) → MATCH
	transition2, err := evaluator.EvaluateWithState(ctx, ruleDef, entityID, "", true, entityDrafting, nil)
	if err != nil {
		t.Fatalf("Second evaluation error: %v", err)
	}
	if transition2 != TransitionEntered {
		t.Errorf("Second evaluation: expected TransitionEntered, got %v", transition2)
	}
	if actionExecutor.onEnterCalls != 1 {
		t.Errorf("Expected 1 OnEnter call, got %d", actionExecutor.onEnterCalls)
	}

	// Verify FieldValues updated to "drafting"
	state2, err := stateTracker.Get(ctx, ruleDef.ID, entityID)
	if err != nil {
		t.Fatalf("Failed to get state after second eval: %v", err)
	}
	if state2.FieldValues["workflow.plan.status"] != "drafting" {
		t.Errorf("Expected FieldValues[workflow.plan.status] = 'drafting', got %q", state2.FieldValues["workflow.plan.status"])
	}
}

// TestStatefulEvaluator_FieldValuesBackwardCompat verifies that existing MatchState
// without FieldValues still works (backward compatibility with persisted state).
func TestStatefulEvaluator_FieldValuesBackwardCompat(t *testing.T) {
	ctx := context.Background()
	bucket := newMockKVBucket()
	logger := slog.Default()
	stateTracker := NewStateTracker(bucket, logger)
	actionExecutor := &mockActionExecutor{}
	evaluator := NewStatefulEvaluator(stateTracker, actionExecutor, logger)

	// Simulate old persisted state without FieldValues
	oldState := MatchState{
		RuleID:         "test-rule",
		EntityKey:      "entity-old",
		IsMatching:     true,
		LastTransition: "entered",
		TransitionAt:   time.Now().Add(-1 * time.Minute),
		LastChecked:    time.Now(),
		Iteration:      1,
		// FieldValues intentionally nil (old state format)
	}
	if err := stateTracker.Set(ctx, oldState); err != nil {
		t.Fatalf("Failed to set old state: %v", err)
	}

	ruleDef := Definition{
		ID:   "test-rule",
		Type: "expression",
		Name: "Backward Compat Rule",
		WhileTrue: []Action{
			{Type: ActionTypePublish, Subject: "test.while-true"},
		},
	}

	// Evaluate with existing state — should work fine with nil FieldValues
	transition, err := evaluator.EvaluateWithState(ctx, ruleDef, "entity-old", "", true, nil, nil)
	if err != nil {
		t.Fatalf("Evaluation with old state error: %v", err)
	}
	if transition != TransitionNone {
		t.Errorf("Expected TransitionNone (true→true), got %v", transition)
	}
	if actionExecutor.whileTrueCalls != 1 {
		t.Errorf("Expected 1 WhileTrue call, got %d", actionExecutor.whileTrueCalls)
	}
}
