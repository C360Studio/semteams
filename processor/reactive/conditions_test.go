package reactive

import (
	"testing"
	"time"
)

func TestPhaseIs(t *testing.T) {
	tests := []struct {
		name     string
		phase    string
		expected string
		want     bool
	}{
		{"match", "running", "running", true},
		{"no match", "running", "pending", false},
		{"empty phase", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BuilderTestState{
				ExecutionState: ExecutionState{Phase: tt.phase},
			}
			ctx := &RuleContext{State: state}

			cond := PhaseIs(tt.expected)
			if got := cond(ctx); got != tt.want {
				t.Errorf("PhaseIs(%q) = %v, want %v", tt.expected, got, tt.want)
			}
		})
	}

	t.Run("nil state", func(t *testing.T) {
		ctx := &RuleContext{State: nil}
		if PhaseIs("any")(ctx) {
			t.Error("Expected false for nil state")
		}
	})
}

func TestPhaseIsAny(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{Phase: "running"},
	}
	ctx := &RuleContext{State: state}

	t.Run("match one of many", func(t *testing.T) {
		cond := PhaseIsAny("pending", "running", "completed")
		if !cond(ctx) {
			t.Error("Expected true when phase matches one of the values")
		}
	})

	t.Run("no match", func(t *testing.T) {
		cond := PhaseIsAny("pending", "completed")
		if cond(ctx) {
			t.Error("Expected false when phase doesn't match any value")
		}
	})
}

func TestStatusIs(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{Status: StatusRunning},
	}
	ctx := &RuleContext{State: state}

	t.Run("match", func(t *testing.T) {
		if !StatusIs(StatusRunning)(ctx) {
			t.Error("Expected true for matching status")
		}
	})

	t.Run("no match", func(t *testing.T) {
		if StatusIs(StatusCompleted)(ctx) {
			t.Error("Expected false for non-matching status")
		}
	})
}

func TestStatusIsAny(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{Status: StatusWaiting},
	}
	ctx := &RuleContext{State: state}

	t.Run("match", func(t *testing.T) {
		cond := StatusIsAny(StatusRunning, StatusWaiting, StatusPending)
		if !cond(ctx) {
			t.Error("Expected true when status matches one of the values")
		}
	})
}

func TestStatusIsTerminal(t *testing.T) {
	tests := []struct {
		status ExecutionStatus
		want   bool
	}{
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusEscalated, true},
		{StatusTimedOut, true},
		{StatusRunning, false},
		{StatusPending, false},
		{StatusWaiting, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			state := &BuilderTestState{
				ExecutionState: ExecutionState{Status: tt.status},
			}
			ctx := &RuleContext{State: state}

			if got := StatusIsTerminal()(ctx); got != tt.want {
				t.Errorf("StatusIsTerminal() = %v, want %v for status %v", got, tt.want, tt.status)
			}
		})
	}
}

func TestStatusIsActive(t *testing.T) {
	tests := []struct {
		status ExecutionStatus
		want   bool
	}{
		{StatusPending, true},
		{StatusRunning, true},
		{StatusWaiting, true},
		{StatusCompleted, false},
		{StatusFailed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			state := &BuilderTestState{
				ExecutionState: ExecutionState{Status: tt.status},
			}
			ctx := &RuleContext{State: state}

			if got := StatusIsActive()(ctx); got != tt.want {
				t.Errorf("StatusIsActive() = %v, want %v for status %v", got, tt.want, tt.status)
			}
		})
	}
}

func TestIterationConditions(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{Iteration: 5},
	}
	ctx := &RuleContext{State: state}

	t.Run("IterationLessThan", func(t *testing.T) {
		if !IterationLessThan(10)(ctx) {
			t.Error("Expected true for iteration < 10")
		}
		if IterationLessThan(5)(ctx) {
			t.Error("Expected false for iteration < 5 (it's exactly 5)")
		}
		if IterationLessThan(3)(ctx) {
			t.Error("Expected false for iteration < 3")
		}
	})

	t.Run("IterationEquals", func(t *testing.T) {
		if !IterationEquals(5)(ctx) {
			t.Error("Expected true for iteration == 5")
		}
		if IterationEquals(3)(ctx) {
			t.Error("Expected false for iteration == 3")
		}
	})

	t.Run("IterationGreaterThan", func(t *testing.T) {
		if !IterationGreaterThan(3)(ctx) {
			t.Error("Expected true for iteration > 3")
		}
		if IterationGreaterThan(5)(ctx) {
			t.Error("Expected false for iteration > 5 (it's exactly 5)")
		}
		if IterationGreaterThan(10)(ctx) {
			t.Error("Expected false for iteration > 10")
		}
	})

	t.Run("nil state defaults", func(t *testing.T) {
		nilCtx := &RuleContext{State: nil}
		if !IterationLessThan(10)(nilCtx) {
			t.Error("Expected true for nil state with IterationLessThan")
		}
		if !IterationEquals(0)(nilCtx) {
			t.Error("Expected true for nil state with IterationEquals(0)")
		}
	})
}

func TestErrorConditions(t *testing.T) {
	t.Run("HasError", func(t *testing.T) {
		withError := &BuilderTestState{
			ExecutionState: ExecutionState{Error: "some error"},
		}
		withoutError := &BuilderTestState{
			ExecutionState: ExecutionState{},
		}

		if !HasError()(&RuleContext{State: withError}) {
			t.Error("Expected true for state with error")
		}
		if HasError()(&RuleContext{State: withoutError}) {
			t.Error("Expected false for state without error")
		}
	})

	t.Run("NoError", func(t *testing.T) {
		withError := &BuilderTestState{
			ExecutionState: ExecutionState{Error: "some error"},
		}
		withoutError := &BuilderTestState{
			ExecutionState: ExecutionState{},
		}

		if NoError()(&RuleContext{State: withError}) {
			t.Error("Expected false for state with error")
		}
		if !NoError()(&RuleContext{State: withoutError}) {
			t.Error("Expected true for state without error")
		}
	})
}

func TestPendingTaskConditions(t *testing.T) {
	withTask := &BuilderTestState{
		ExecutionState: ExecutionState{PendingTaskID: "task-123"},
	}
	withoutTask := &BuilderTestState{
		ExecutionState: ExecutionState{},
	}

	t.Run("HasPendingTask", func(t *testing.T) {
		if !HasPendingTask()(&RuleContext{State: withTask}) {
			t.Error("Expected true for state with pending task")
		}
		if HasPendingTask()(&RuleContext{State: withoutTask}) {
			t.Error("Expected false for state without pending task")
		}
	})

	t.Run("NoPendingTask", func(t *testing.T) {
		if NoPendingTask()(&RuleContext{State: withTask}) {
			t.Error("Expected false for state with pending task")
		}
		if !NoPendingTask()(&RuleContext{State: withoutTask}) {
			t.Error("Expected true for state without pending task")
		}
	})
}

func TestTimeoutConditions(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	expired := &BuilderTestState{
		ExecutionState: ExecutionState{Deadline: &past},
	}
	notExpired := &BuilderTestState{
		ExecutionState: ExecutionState{Deadline: &future},
	}
	noDeadline := &BuilderTestState{
		ExecutionState: ExecutionState{},
	}

	t.Run("IsTimedOut", func(t *testing.T) {
		if !IsTimedOut()(&RuleContext{State: expired}) {
			t.Error("Expected true for expired deadline")
		}
		if IsTimedOut()(&RuleContext{State: notExpired}) {
			t.Error("Expected false for future deadline")
		}
		if IsTimedOut()(&RuleContext{State: noDeadline}) {
			t.Error("Expected false for no deadline")
		}
	})

	t.Run("NotTimedOut", func(t *testing.T) {
		if NotTimedOut()(&RuleContext{State: expired}) {
			t.Error("Expected false for expired deadline")
		}
		if !NotTimedOut()(&RuleContext{State: notExpired}) {
			t.Error("Expected true for future deadline")
		}
		if !NotTimedOut()(&RuleContext{State: noDeadline}) {
			t.Error("Expected true for no deadline")
		}
	})
}

func TestStateFieldEquals(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{Phase: "running"},
		CustomField:    "custom-value",
		Counter:        42,
	}
	ctx := &RuleContext{State: state}

	t.Run("string field", func(t *testing.T) {
		getter := func(s any) string { return s.(*BuilderTestState).CustomField }
		if !StateFieldEquals(getter, "custom-value")(ctx) {
			t.Error("Expected true for matching custom field")
		}
		if StateFieldEquals(getter, "other")(ctx) {
			t.Error("Expected false for non-matching custom field")
		}
	})

	t.Run("int field", func(t *testing.T) {
		getter := func(s any) int { return s.(*BuilderTestState).Counter }
		if !StateFieldEquals(getter, 42)(ctx) {
			t.Error("Expected true for matching counter")
		}
		if StateFieldEquals(getter, 0)(ctx) {
			t.Error("Expected false for non-matching counter")
		}
	})
}

func TestStateFieldNotEquals(t *testing.T) {
	state := &BuilderTestState{
		CustomField: "value",
	}
	ctx := &RuleContext{State: state}

	getter := func(s any) string { return s.(*BuilderTestState).CustomField }

	if !StateFieldNotEquals(getter, "other")(ctx) {
		t.Error("Expected true when field != 'other'")
	}
	if StateFieldNotEquals(getter, "value")(ctx) {
		t.Error("Expected false when field == 'value'")
	}
}

func TestMessageFieldEquals(t *testing.T) {
	msg := &BuilderTestPayload{Value: "test-value"}
	ctx := &RuleContext{Message: msg}

	getter := func(m any) string { return m.(*BuilderTestPayload).Value }

	t.Run("match", func(t *testing.T) {
		if !MessageFieldEquals(getter, "test-value")(ctx) {
			t.Error("Expected true for matching message field")
		}
	})

	t.Run("no match", func(t *testing.T) {
		if MessageFieldEquals(getter, "other")(ctx) {
			t.Error("Expected false for non-matching message field")
		}
	})

	t.Run("nil message", func(t *testing.T) {
		nilCtx := &RuleContext{Message: nil}
		if MessageFieldEquals(getter, "any")(nilCtx) {
			t.Error("Expected false for nil message")
		}
	})
}

func TestHasStateAndHasMessage(t *testing.T) {
	state := &BuilderTestState{}
	msg := &BuilderTestPayload{}

	tests := []struct {
		name       string
		ctx        *RuleContext
		hasState   bool
		hasMessage bool
	}{
		{"both", &RuleContext{State: state, Message: msg}, true, true},
		{"state only", &RuleContext{State: state}, true, false},
		{"message only", &RuleContext{Message: msg}, false, true},
		{"neither", &RuleContext{}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasState()(tt.ctx); got != tt.hasState {
				t.Errorf("HasState() = %v, want %v", got, tt.hasState)
			}
			if got := HasMessage()(tt.ctx); got != tt.hasMessage {
				t.Errorf("HasMessage() = %v, want %v", got, tt.hasMessage)
			}
		})
	}
}

func TestLogicalCombinators(t *testing.T) {
	ctx := &RuleContext{State: &BuilderTestState{}}

	t.Run("And - all true", func(t *testing.T) {
		cond := And(Always(), Always(), Always())
		if !cond(ctx) {
			t.Error("Expected true when all conditions are true")
		}
	})

	t.Run("And - one false", func(t *testing.T) {
		cond := And(Always(), Never(), Always())
		if cond(ctx) {
			t.Error("Expected false when one condition is false")
		}
	})

	t.Run("Or - one true", func(t *testing.T) {
		cond := Or(Never(), Always(), Never())
		if !cond(ctx) {
			t.Error("Expected true when one condition is true")
		}
	})

	t.Run("Or - all false", func(t *testing.T) {
		cond := Or(Never(), Never(), Never())
		if cond(ctx) {
			t.Error("Expected false when all conditions are false")
		}
	})

	t.Run("Not", func(t *testing.T) {
		if Not(Always())(ctx) {
			t.Error("Expected Not(Always()) to be false")
		}
		if !Not(Never())(ctx) {
			t.Error("Expected Not(Never()) to be true")
		}
	})
}

func TestAlwaysAndNever(t *testing.T) {
	ctx := &RuleContext{}

	if !Always()(ctx) {
		t.Error("Always() should return true")
	}
	if Never()(ctx) {
		t.Error("Never() should return false")
	}
}

func TestCustomCondition(t *testing.T) {
	cond := Custom("custom check", func(ctx *RuleContext) bool {
		return ctx.State != nil
	})

	if cond.Description != "custom check" {
		t.Errorf("Expected description 'custom check', got %q", cond.Description)
	}

	if cond.Evaluate(&RuleContext{State: &BuilderTestState{}}) != true {
		t.Error("Expected true for non-nil state")
	}
	if cond.Evaluate(&RuleContext{}) != false {
		t.Error("Expected false for nil state")
	}
}
