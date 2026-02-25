package reactive

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestState is a test state struct that embeds ExecutionState.
type TestState struct {
	ExecutionState
	CustomField string `json:"custom_field"`
}

// GetExecutionState implements StateAccessor.
func (s *TestState) GetExecutionState() *ExecutionState {
	return &s.ExecutionState
}

func TestEvaluator_EvaluateRule_NoConditions(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID: "test-rule",
		Trigger: TriggerSource{
			WatchBucket:  "TEST",
			WatchPattern: "*",
		},
		Action: Action{
			Type:        ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error { return nil },
		},
	}

	ctx := &RuleContext{
		State: &TestState{
			ExecutionState: ExecutionState{
				ID:     "exec-1",
				Status: StatusRunning,
			},
		},
	}

	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")

	assert.True(t, result.ShouldFire)
	assert.Equal(t, "no conditions (always fires)", result.Reason)
	assert.Empty(t, result.ConditionResults)
}

func TestEvaluator_EvaluateRule_ANDLogic_AllPass(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:    "test-rule",
		Logic: "and",
		Conditions: []Condition{
			{Description: "always true 1", Evaluate: func(_ *RuleContext) bool { return true }},
			{Description: "always true 2", Evaluate: func(_ *RuleContext) bool { return true }},
			{Description: "always true 3", Evaluate: func(_ *RuleContext) bool { return true }},
		},
		Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")

	assert.True(t, result.ShouldFire)
	assert.Equal(t, "all conditions passed (AND logic)", result.Reason)
	assert.Len(t, result.ConditionResults, 3)
	for _, cr := range result.ConditionResults {
		assert.True(t, cr.Passed)
	}
}

func TestEvaluator_EvaluateRule_ANDLogic_OneFails(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:    "test-rule",
		Logic: "and",
		Conditions: []Condition{
			{Description: "passes", Evaluate: func(_ *RuleContext) bool { return true }},
			{Description: "fails", Evaluate: func(_ *RuleContext) bool { return false }},
			{Description: "passes", Evaluate: func(_ *RuleContext) bool { return true }},
		},
		Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")

	assert.False(t, result.ShouldFire)
	assert.Equal(t, "not all conditions passed (AND logic)", result.Reason)

	// Check individual results
	assert.True(t, result.ConditionResults[0].Passed)
	assert.False(t, result.ConditionResults[1].Passed)
	assert.True(t, result.ConditionResults[2].Passed)
}

func TestEvaluator_EvaluateRule_ORLogic_OnePass(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:    "test-rule",
		Logic: "or",
		Conditions: []Condition{
			{Description: "fails", Evaluate: func(_ *RuleContext) bool { return false }},
			{Description: "passes", Evaluate: func(_ *RuleContext) bool { return true }},
			{Description: "fails", Evaluate: func(_ *RuleContext) bool { return false }},
		},
		Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")

	assert.True(t, result.ShouldFire)
	assert.Equal(t, "at least one condition passed (OR logic)", result.Reason)
}

func TestEvaluator_EvaluateRule_ORLogic_AllFail(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:    "test-rule",
		Logic: "or",
		Conditions: []Condition{
			{Description: "fails 1", Evaluate: func(_ *RuleContext) bool { return false }},
			{Description: "fails 2", Evaluate: func(_ *RuleContext) bool { return false }},
		},
		Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")

	assert.False(t, result.ShouldFire)
	assert.Equal(t, "no conditions passed (OR logic)", result.Reason)
}

func TestEvaluator_EvaluateRule_DefaultsToAND(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:    "test-rule",
		Logic: "", // Empty should default to AND
		Conditions: []Condition{
			{Description: "passes", Evaluate: func(_ *RuleContext) bool { return true }},
			{Description: "fails", Evaluate: func(_ *RuleContext) bool { return false }},
		},
		Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")

	assert.False(t, result.ShouldFire)
	assert.Contains(t, result.Reason, "AND logic")
}

func TestEvaluator_Cooldown(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:       "test-rule",
		Cooldown: 100 * time.Millisecond,
		Trigger:  TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:   Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}

	// First evaluation should pass
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
	assert.True(t, result.ShouldFire)

	// Record firing
	e.RecordFiring("test-workflow", "exec-1", "test-rule")

	// Immediate re-evaluation should fail due to cooldown
	result = e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
	assert.False(t, result.ShouldFire)
	assert.Equal(t, "rule is on cooldown", result.Reason)

	// Wait for cooldown to expire
	time.Sleep(150 * time.Millisecond)

	// Should pass again
	result = e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
	assert.True(t, result.ShouldFire)
}

func TestEvaluator_MaxFirings(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:         "test-rule",
		MaxFirings: 2,
		Trigger:    TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:     Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}

	// First two should pass
	for i := 0; i < 2; i++ {
		result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
		assert.True(t, result.ShouldFire, "firing %d should pass", i+1)
		e.RecordFiring("test-workflow", "exec-1", "test-rule")
	}

	// Third should fail
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
	assert.False(t, result.ShouldFire)
	assert.Equal(t, "max firings reached", result.Reason)
}

func TestEvaluator_MaxFirings_PerExecution(t *testing.T) {
	e := NewEvaluator(slog.Default())

	rule := &RuleDef{
		ID:         "test-rule",
		MaxFirings: 1,
		Trigger:    TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
		Action:     Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
	}

	ctx := &RuleContext{State: &TestState{}}

	// Fire once for exec-1
	result := e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
	assert.True(t, result.ShouldFire)
	e.RecordFiring("test-workflow", "exec-1", "test-rule")

	// exec-1 should be blocked
	result = e.EvaluateRule(ctx, rule, "test-workflow", "exec-1")
	assert.False(t, result.ShouldFire)

	// exec-2 should still be able to fire (different execution)
	result = e.EvaluateRule(ctx, rule, "test-workflow", "exec-2")
	assert.True(t, result.ShouldFire)
}

func TestEvaluator_EvaluateRules_FirstMatchWins(t *testing.T) {
	e := NewEvaluator(slog.Default())

	def := &Definition{
		ID:           "test-workflow",
		StateBucket:  "TEST",
		StateFactory: func() any { return &TestState{} },
		Rules: []RuleDef{
			{
				ID: "rule-1",
				Conditions: []Condition{
					{Description: "fails", Evaluate: func(_ *RuleContext) bool { return false }},
				},
				Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
				Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
			},
			{
				ID: "rule-2",
				Conditions: []Condition{
					{Description: "passes", Evaluate: func(_ *RuleContext) bool { return true }},
				},
				Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
				Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
			},
			{
				ID: "rule-3",
				Conditions: []Condition{
					{Description: "passes", Evaluate: func(_ *RuleContext) bool { return true }},
				},
				Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
				Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
			},
		},
	}

	ctx := &RuleContext{State: &TestState{}}
	rule, result := e.EvaluateRules(ctx, def, "exec-1")

	require.NotNil(t, rule)
	assert.Equal(t, "rule-2", rule.ID) // First matching rule
	assert.True(t, result.ShouldFire)
}

func TestEvaluator_EvaluateRules_NoMatch(t *testing.T) {
	e := NewEvaluator(slog.Default())

	def := &Definition{
		ID:           "test-workflow",
		StateBucket:  "TEST",
		StateFactory: func() any { return &TestState{} },
		Rules: []RuleDef{
			{
				ID: "rule-1",
				Conditions: []Condition{
					{Description: "fails", Evaluate: func(_ *RuleContext) bool { return false }},
				},
				Trigger: TriggerSource{WatchBucket: "TEST", WatchPattern: "*"},
				Action:  Action{Type: ActionMutate, MutateState: func(_ *RuleContext, _ any) error { return nil }},
			},
		},
	}

	ctx := &RuleContext{State: &TestState{}}
	rule, result := e.EvaluateRules(ctx, def, "exec-1")

	assert.Nil(t, rule)
	assert.Nil(t, result)
}

func TestEvaluator_ClearExecutionState(t *testing.T) {
	e := NewEvaluator(slog.Default())

	// Record some firings
	e.RecordFiring("wf", "exec-1", "rule-1")
	e.RecordFiring("wf", "exec-1", "rule-2")
	e.RecordFiring("wf", "exec-2", "rule-1")

	// Verify counts
	assert.Equal(t, 1, e.getFiringCount("exec-1", "rule-1"))
	assert.Equal(t, 1, e.getFiringCount("exec-1", "rule-2"))
	assert.Equal(t, 1, e.getFiringCount("exec-2", "rule-1"))

	// Clear exec-1
	e.ClearExecutionState("exec-1")

	// exec-1 should be cleared
	assert.Equal(t, 0, e.getFiringCount("exec-1", "rule-1"))
	assert.Equal(t, 0, e.getFiringCount("exec-1", "rule-2"))

	// exec-2 should still exist
	assert.Equal(t, 1, e.getFiringCount("exec-2", "rule-1"))
}

// Test ConditionHelpers

func TestConditionHelpers_PhaseIs(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Phase: "pending"},
	}
	ctx := &RuleContext{State: state}

	assert.True(t, ConditionHelpers.PhaseIs("pending")(ctx))
	assert.False(t, ConditionHelpers.PhaseIs("running")(ctx))
	assert.False(t, ConditionHelpers.PhaseIs("pending")(&RuleContext{})) // nil state
}

func TestConditionHelpers_PhaseIn(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Phase: "pending"},
	}
	ctx := &RuleContext{State: state}

	assert.True(t, ConditionHelpers.PhaseIn("pending", "running")(ctx))
	assert.True(t, ConditionHelpers.PhaseIn("running", "pending")(ctx))
	assert.False(t, ConditionHelpers.PhaseIn("running", "completed")(ctx))
}

func TestConditionHelpers_StatusIs(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}
	ctx := &RuleContext{State: state}

	assert.True(t, ConditionHelpers.StatusIs(StatusRunning)(ctx))
	assert.False(t, ConditionHelpers.StatusIs(StatusCompleted)(ctx))
}

func TestConditionHelpers_IterationLessThan(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Iteration: 2},
	}
	ctx := &RuleContext{State: state}

	assert.True(t, ConditionHelpers.IterationLessThan(3)(ctx))
	assert.True(t, ConditionHelpers.IterationLessThan(5)(ctx))
	assert.False(t, ConditionHelpers.IterationLessThan(2)(ctx))
	assert.False(t, ConditionHelpers.IterationLessThan(1)(ctx))
}

func TestConditionHelpers_HasError(t *testing.T) {
	stateWithError := &TestState{
		ExecutionState: ExecutionState{Error: "something went wrong"},
	}
	stateNoError := &TestState{
		ExecutionState: ExecutionState{Error: ""},
	}

	assert.True(t, ConditionHelpers.HasError()(&RuleContext{State: stateWithError}))
	assert.False(t, ConditionHelpers.HasError()(&RuleContext{State: stateNoError}))
	assert.False(t, ConditionHelpers.HasError()(&RuleContext{})) // nil state
}

func TestConditionHelpers_NoError(t *testing.T) {
	stateWithError := &TestState{
		ExecutionState: ExecutionState{Error: "something went wrong"},
	}
	stateNoError := &TestState{
		ExecutionState: ExecutionState{Error: ""},
	}

	assert.False(t, ConditionHelpers.NoError()(&RuleContext{State: stateWithError}))
	assert.True(t, ConditionHelpers.NoError()(&RuleContext{State: stateNoError}))
	assert.True(t, ConditionHelpers.NoError()(&RuleContext{})) // nil state is no error
}

func TestConditionHelpers_IsWaiting(t *testing.T) {
	waitingState := &TestState{
		ExecutionState: ExecutionState{Status: StatusWaiting},
	}
	runningState := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}

	assert.True(t, ConditionHelpers.IsWaiting()(&RuleContext{State: waitingState}))
	assert.False(t, ConditionHelpers.IsWaiting()(&RuleContext{State: runningState}))
	assert.False(t, ConditionHelpers.NotWaiting()(&RuleContext{State: waitingState}))
	assert.True(t, ConditionHelpers.NotWaiting()(&RuleContext{State: runningState}))
}

// Test State Helpers

func TestExtractExecutionState_DirectPointer(t *testing.T) {
	es := &ExecutionState{ID: "test-123"}
	result := ExtractExecutionState(es)
	require.NotNil(t, result)
	assert.Equal(t, "test-123", result.ID)
}

func TestExtractExecutionState_ViaInterface(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{ID: "test-456"},
	}
	result := ExtractExecutionState(state)
	require.NotNil(t, result)
	assert.Equal(t, "test-456", result.ID)
}

func TestExtractExecutionState_ViaReflection(t *testing.T) {
	// Create a struct that embeds ExecutionState but doesn't implement StateAccessor
	type embeddedState struct {
		ExecutionState
		Extra string
	}

	state := &embeddedState{
		ExecutionState: ExecutionState{ID: "test-789"},
		Extra:          "data",
	}

	result := ExtractExecutionState(state)
	require.NotNil(t, result)
	assert.Equal(t, "test-789", result.ID)
}

func TestExtractExecutionState_Nil(t *testing.T) {
	assert.Nil(t, ExtractExecutionState(nil))
}

func TestExtractExecutionState_NonStruct(t *testing.T) {
	assert.Nil(t, ExtractExecutionState("not a struct"))
	assert.Nil(t, ExtractExecutionState(123))
}

func TestSetPhase(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{ID: "exec-1", Phase: "pending"},
	}

	SetPhase(state, "running", "fire-planner", TriggerStateOnly, "exec-1", "publish_async")

	assert.Equal(t, "running", state.Phase)
	assert.Len(t, state.Timeline, 1)
	assert.Equal(t, "fire-planner", state.Timeline[0].RuleID)
	assert.Equal(t, "running", state.Timeline[0].Phase)
	assert.Equal(t, "kv", state.Timeline[0].TriggerMode)
}

func TestSetStatus(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}

	SetStatus(state, StatusCompleted)

	assert.Equal(t, StatusCompleted, state.Status)
	assert.NotNil(t, state.CompletedAt)
}

func TestSetError(t *testing.T) {
	state := &TestState{}

	SetError(state, "something failed")
	assert.Equal(t, "something failed", state.Error)

	ClearError(state)
	assert.Empty(t, state.Error)
}

func TestIncrementIteration(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Iteration: 0},
	}

	assert.Equal(t, 1, IncrementIteration(state))
	assert.Equal(t, 2, IncrementIteration(state))
	assert.Equal(t, 3, IncrementIteration(state))
	assert.Equal(t, 3, state.Iteration)
}

func TestSetPendingTask(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}

	SetPendingTask(state, "task-123", "fire-planner")

	assert.Equal(t, "task-123", state.PendingTaskID)
	assert.Equal(t, "fire-planner", state.PendingRuleID)
	assert.Equal(t, StatusWaiting, state.Status)
}

func TestClearPendingTask(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{
			Status:        StatusWaiting,
			PendingTaskID: "task-123",
			PendingRuleID: "fire-planner",
		},
	}

	ClearPendingTask(state)

	assert.Empty(t, state.PendingTaskID)
	assert.Empty(t, state.PendingRuleID)
	assert.Equal(t, StatusRunning, state.Status)
}

func TestIsExpired(t *testing.T) {
	pastDeadline := time.Now().Add(-1 * time.Hour)
	futureDeadline := time.Now().Add(1 * time.Hour)

	expired := &TestState{
		ExecutionState: ExecutionState{Deadline: &pastDeadline},
	}
	notExpired := &TestState{
		ExecutionState: ExecutionState{Deadline: &futureDeadline},
	}
	noDeadline := &TestState{}

	assert.True(t, IsExpired(expired))
	assert.False(t, IsExpired(notExpired))
	assert.False(t, IsExpired(noDeadline))
}

func TestIsTerminalStatus(t *testing.T) {
	assert.True(t, IsTerminalStatus(StatusCompleted))
	assert.True(t, IsTerminalStatus(StatusFailed))
	assert.True(t, IsTerminalStatus(StatusEscalated))
	assert.True(t, IsTerminalStatus(StatusTimedOut))
	assert.False(t, IsTerminalStatus(StatusPending))
	assert.False(t, IsTerminalStatus(StatusRunning))
	assert.False(t, IsTerminalStatus(StatusWaiting))
}

func TestCanProceed(t *testing.T) {
	running := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}
	waiting := &TestState{
		ExecutionState: ExecutionState{Status: StatusWaiting},
	}
	completed := &TestState{
		ExecutionState: ExecutionState{Status: StatusCompleted},
	}

	pastDeadline := time.Now().Add(-1 * time.Hour)
	expired := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning, Deadline: &pastDeadline},
	}

	assert.True(t, CanProceed(running))
	assert.False(t, CanProceed(waiting))
	assert.False(t, CanProceed(completed))
	assert.False(t, CanProceed(expired))
}

func TestInitializeExecution(t *testing.T) {
	state := &TestState{}

	InitializeExecution(state, "exec-123", "plan-review", 30*time.Minute)

	assert.Equal(t, "exec-123", state.ID)
	assert.Equal(t, "plan-review", state.WorkflowID)
	assert.Equal(t, "pending", state.Phase)
	assert.Equal(t, 0, state.Iteration)
	assert.Equal(t, StatusRunning, state.Status)
	assert.NotNil(t, state.Deadline)
	assert.True(t, state.Deadline.After(time.Now()))
}

func TestCompleteExecution(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{
			Status:        StatusRunning,
			PendingTaskID: "task-123",
		},
	}

	CompleteExecution(state, "approved")

	assert.Equal(t, "approved", state.Phase)
	assert.Equal(t, StatusCompleted, state.Status)
	assert.NotNil(t, state.CompletedAt)
	assert.Empty(t, state.PendingTaskID)
}

func TestFailExecution(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}

	FailExecution(state, "validation failed")

	assert.Equal(t, StatusFailed, state.Status)
	assert.Equal(t, "validation failed", state.Error)
	assert.NotNil(t, state.CompletedAt)
}

func TestEscalateExecution(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}

	EscalateExecution(state, "max iterations exceeded")

	assert.Equal(t, StatusEscalated, state.Status)
	assert.Equal(t, "max iterations exceeded", state.Error)
}

func TestTimeoutExecution(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}

	TimeoutExecution(state)

	assert.Equal(t, StatusTimedOut, state.Status)
	assert.Equal(t, "execution timed out", state.Error)
}

func TestGetters(t *testing.T) {
	state := &TestState{
		ExecutionState: ExecutionState{
			ID:         "exec-123",
			WorkflowID: "plan-review",
			Phase:      "pending",
			Status:     StatusRunning,
			Iteration:  5,
		},
	}

	assert.Equal(t, "exec-123", GetID(state))
	assert.Equal(t, "plan-review", GetWorkflowID(state))
	assert.Equal(t, "pending", GetPhase(state))
	assert.Equal(t, StatusRunning, GetStatus(state))
	assert.Equal(t, 5, GetIteration(state))

	// Test with nil
	assert.Empty(t, GetID(nil))
	assert.Empty(t, GetWorkflowID(nil))
	assert.Empty(t, GetPhase(nil))
	assert.Equal(t, ExecutionStatus(""), GetStatus(nil))
	assert.Equal(t, 0, GetIteration(nil))
}
