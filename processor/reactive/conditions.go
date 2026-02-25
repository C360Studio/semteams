package reactive

// This file provides condition helper functions for building reactive workflow rules.
// These helpers create ConditionFunc instances that can be used with RuleBuilder.When().

// PhaseIs returns a condition that checks if the execution phase matches the expected value.
func PhaseIs(expectedPhase string) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return es.Phase == expectedPhase
	}
}

// PhaseIsAny returns a condition that checks if the execution phase matches any of the expected values.
func PhaseIsAny(phases ...string) ConditionFunc {
	phaseSet := make(map[string]bool, len(phases))
	for _, p := range phases {
		phaseSet[p] = true
	}
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return phaseSet[es.Phase]
	}
}

// StatusIs returns a condition that checks if the execution status matches the expected value.
func StatusIs(expectedStatus ExecutionStatus) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return es.Status == expectedStatus
	}
}

// StatusIsAny returns a condition that checks if the execution status matches any of the expected values.
func StatusIsAny(statuses ...ExecutionStatus) ConditionFunc {
	statusSet := make(map[ExecutionStatus]bool, len(statuses))
	for _, s := range statuses {
		statusSet[s] = true
	}
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return statusSet[es.Status]
	}
}

// StatusIsTerminal returns a condition that checks if the execution has reached a terminal state.
func StatusIsTerminal() ConditionFunc {
	return StatusIsAny(StatusCompleted, StatusFailed, StatusEscalated, StatusTimedOut)
}

// StatusIsActive returns a condition that checks if the execution is still active.
func StatusIsActive() ConditionFunc {
	return StatusIsAny(StatusPending, StatusRunning, StatusWaiting)
}

// IterationLessThan returns a condition that checks if the iteration count is below the limit.
func IterationLessThan(limit int) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return true // Default to true if no state (first iteration)
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return true
		}
		return es.Iteration < limit
	}
}

// IterationEquals returns a condition that checks if the iteration count equals the expected value.
func IterationEquals(expected int) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return expected == 0
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return expected == 0
		}
		return es.Iteration == expected
	}
}

// IterationGreaterThan returns a condition that checks if the iteration count exceeds the threshold.
func IterationGreaterThan(threshold int) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return es.Iteration > threshold
	}
}

// HasError returns a condition that checks if the execution has an error set.
func HasError() ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return es.Error != ""
	}
}

// NoError returns a condition that checks if the execution has no error.
func NoError() ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return true
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return true
		}
		return es.Error == ""
	}
}

// HasPendingTask returns a condition that checks if there's a pending async task.
func HasPendingTask() ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return false
		}
		return es.PendingTaskID != ""
	}
}

// NoPendingTask returns a condition that checks if there's no pending async task.
func NoPendingTask() ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return true
		}
		es := ExtractExecutionState(ctx.State)
		if es == nil {
			return true
		}
		return es.PendingTaskID == ""
	}
}

// IsTimedOut returns a condition that checks if the execution has exceeded its deadline.
func IsTimedOut() ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		return IsExpired(ctx.State)
	}
}

// NotTimedOut returns a condition that checks if the execution has not exceeded its deadline.
func NotTimedOut() ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return true
		}
		return !IsExpired(ctx.State)
	}
}

// StateFieldEquals creates a generic condition that checks if a field in the state equals an expected value.
// The getter function extracts the field value from the typed state.
func StateFieldEquals[T comparable](getter func(state any) T, expected T) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		return getter(ctx.State) == expected
	}
}

// StateFieldNotEquals creates a condition that checks if a field does not equal a value.
func StateFieldNotEquals[T comparable](getter func(state any) T, notExpected T) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.State == nil {
			return false
		}
		return getter(ctx.State) != notExpected
	}
}

// MessageFieldEquals creates a condition that checks if a field in the message equals an expected value.
func MessageFieldEquals[T comparable](getter func(msg any) T, expected T) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.Message == nil {
			return false
		}
		return getter(ctx.Message) == expected
	}
}

// MessageFieldNotEquals creates a condition that checks if a field in the message does not equal a value.
func MessageFieldNotEquals[T comparable](getter func(msg any) T, notExpected T) ConditionFunc {
	return func(ctx *RuleContext) bool {
		if ctx.Message == nil {
			return false
		}
		return getter(ctx.Message) != notExpected
	}
}

// HasState returns a condition that checks if the rule context has state available.
func HasState() ConditionFunc {
	return func(ctx *RuleContext) bool {
		return ctx.State != nil
	}
}

// HasMessage returns a condition that checks if the rule context has a message available.
func HasMessage() ConditionFunc {
	return func(ctx *RuleContext) bool {
		return ctx.Message != nil
	}
}

// Custom creates a Condition struct from a custom function.
// This is useful when you need both a description and condition for logging/debugging.
// For simple usage with RuleBuilder.When(), use the ConditionFunc directly instead.
func Custom(description string, fn ConditionFunc) Condition {
	return Condition{
		Description: description,
		Evaluate:    fn,
	}
}

// CustomFunc creates a ConditionFunc from a custom function with a closure over the description.
// This provides API consistency with other condition helpers while still allowing custom logic.
// The description is only used for debugging/logging purposes if needed.
func CustomFunc(fn ConditionFunc) ConditionFunc {
	return fn
}

// And combines multiple conditions with AND logic.
// All conditions must be true for the combined condition to be true.
func And(conditions ...ConditionFunc) ConditionFunc {
	return func(ctx *RuleContext) bool {
		for _, c := range conditions {
			if !c(ctx) {
				return false
			}
		}
		return true
	}
}

// Or combines multiple conditions with OR logic.
// At least one condition must be true for the combined condition to be true.
func Or(conditions ...ConditionFunc) ConditionFunc {
	return func(ctx *RuleContext) bool {
		for _, c := range conditions {
			if c(ctx) {
				return true
			}
		}
		return false
	}
}

// Not negates a condition.
func Not(condition ConditionFunc) ConditionFunc {
	return func(ctx *RuleContext) bool {
		return !condition(ctx)
	}
}

// Always returns a condition that always evaluates to true.
// Useful for rules that should always fire when triggered.
func Always() ConditionFunc {
	return func(_ *RuleContext) bool {
		return true
	}
}

// Never returns a condition that always evaluates to false.
// Useful for temporarily disabling rules.
func Never() ConditionFunc {
	return func(_ *RuleContext) bool {
		return false
	}
}
