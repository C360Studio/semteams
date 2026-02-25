package reactive

import (
	"log/slog"
	"sync"
	"time"
)

// Evaluator evaluates rules against RuleContext to determine which rules should fire.
// It tracks cooldowns and firing counts to enforce rule constraints.
type Evaluator struct {
	logger *slog.Logger

	// cooldowns tracks the last time each rule fired for cooldown enforcement.
	// Key is workflowID:ruleID, value is the last fire time.
	cooldowns  map[string]time.Time
	cooldownMu sync.RWMutex

	// firingCounts tracks how many times each rule has fired per execution.
	// Key is executionID:ruleID, value is the count.
	firingCounts map[string]int
	firingMu     sync.RWMutex
}

// NewEvaluator creates a new rule evaluator.
func NewEvaluator(logger *slog.Logger) *Evaluator {
	return &Evaluator{
		logger:       logger,
		cooldowns:    make(map[string]time.Time),
		firingCounts: make(map[string]int),
	}
}

// EvaluationResult contains the result of evaluating a rule.
type EvaluationResult struct {
	// RuleID is the ID of the rule that was evaluated.
	RuleID string

	// ShouldFire indicates whether the rule's conditions are met and it should fire.
	ShouldFire bool

	// Reason explains why the rule should or should not fire.
	Reason string

	// ConditionResults contains the result of each condition evaluation.
	ConditionResults []ConditionResult
}

// ConditionResult contains the result of evaluating a single condition.
type ConditionResult struct {
	// Description is the condition's description.
	Description string

	// Passed indicates whether the condition evaluated to true.
	Passed bool
}

// EvaluateRule evaluates a rule against a RuleContext and returns whether it should fire.
// This checks:
// 1. All conditions (using AND/OR logic based on rule.Logic)
// 2. Cooldown constraints
// 3. Max firings constraints
func (e *Evaluator) EvaluateRule(
	ctx *RuleContext,
	rule *RuleDef,
	workflowID string,
	executionID string,
) EvaluationResult {
	result := EvaluationResult{
		RuleID:           rule.ID,
		ConditionResults: make([]ConditionResult, 0, len(rule.Conditions)),
	}

	// Check cooldown first (cheap check)
	if rule.Cooldown > 0 {
		if e.isOnCooldown(workflowID, rule.ID, rule.Cooldown) {
			result.ShouldFire = false
			result.Reason = "rule is on cooldown"
			e.logger.Debug("Rule on cooldown",
				"workflow", workflowID,
				"rule", rule.ID,
				"cooldown", rule.Cooldown)
			return result
		}
	}

	// Check max firings
	if rule.MaxFirings > 0 {
		count := e.getFiringCount(executionID, rule.ID)
		if count >= rule.MaxFirings {
			result.ShouldFire = false
			result.Reason = "max firings reached"
			e.logger.Debug("Rule max firings reached",
				"workflow", workflowID,
				"rule", rule.ID,
				"count", count,
				"max", rule.MaxFirings)
			return result
		}
	}

	// Evaluate conditions
	if len(rule.Conditions) == 0 {
		// No conditions means the rule always fires (subject to cooldown/max firings)
		result.ShouldFire = true
		result.Reason = "no conditions (always fires)"
		e.logger.Debug("Rule has no conditions, will fire",
			"workflow", workflowID,
			"rule", rule.ID)
		return result
	}

	// Evaluate each condition
	logic := rule.Logic
	if logic == "" {
		logic = "and" // Default to AND
	}

	passedCount := 0
	for _, cond := range rule.Conditions {
		passed := cond.Evaluate(ctx)
		result.ConditionResults = append(result.ConditionResults, ConditionResult{
			Description: cond.Description,
			Passed:      passed,
		})
		if passed {
			passedCount++
		}

		e.logger.Debug("Condition evaluated",
			"workflow", workflowID,
			"rule", rule.ID,
			"condition", cond.Description,
			"passed", passed)
	}

	// Apply logic
	switch logic {
	case "or":
		result.ShouldFire = passedCount > 0
		if result.ShouldFire {
			result.Reason = "at least one condition passed (OR logic)"
		} else {
			result.Reason = "no conditions passed (OR logic)"
		}
	default: // "and"
		result.ShouldFire = passedCount == len(rule.Conditions)
		if result.ShouldFire {
			result.Reason = "all conditions passed (AND logic)"
		} else {
			result.Reason = "not all conditions passed (AND logic)"
		}
	}

	e.logger.Debug("Rule evaluation complete",
		"workflow", workflowID,
		"rule", rule.ID,
		"shouldFire", result.ShouldFire,
		"reason", result.Reason,
		"passedCount", passedCount,
		"totalConditions", len(rule.Conditions))

	return result
}

// EvaluateRules evaluates all rules in a workflow definition against a RuleContext.
// Returns the first rule that should fire, or nil if no rule matches.
// Rules are evaluated in definition order for deterministic behavior.
func (e *Evaluator) EvaluateRules(
	ctx *RuleContext,
	def *Definition,
	executionID string,
) (*RuleDef, *EvaluationResult) {
	for i := range def.Rules {
		rule := &def.Rules[i]
		result := e.EvaluateRule(ctx, rule, def.ID, executionID)
		if result.ShouldFire {
			return rule, &result
		}
	}
	return nil, nil
}

// RecordFiring records that a rule has fired, updating cooldown and firing count.
func (e *Evaluator) RecordFiring(workflowID, executionID, ruleID string) {
	// Update cooldown
	e.cooldownMu.Lock()
	e.cooldowns[cooldownKey(workflowID, ruleID)] = time.Now()
	e.cooldownMu.Unlock()

	// Update firing count
	e.firingMu.Lock()
	key := firingKey(executionID, ruleID)
	e.firingCounts[key]++
	e.firingMu.Unlock()

	e.logger.Debug("Recorded rule firing",
		"workflow", workflowID,
		"execution", executionID,
		"rule", ruleID)
}

// ClearExecutionState clears firing counts for a completed execution.
// Call this when an execution completes to free memory.
func (e *Evaluator) ClearExecutionState(executionID string) {
	e.firingMu.Lock()
	defer e.firingMu.Unlock()

	// Remove all entries for this execution
	for key := range e.firingCounts {
		if hasPrefix(key, executionID+":") {
			delete(e.firingCounts, key)
		}
	}
}

// isOnCooldown checks if a rule is currently on cooldown.
func (e *Evaluator) isOnCooldown(workflowID, ruleID string, cooldown time.Duration) bool {
	e.cooldownMu.RLock()
	lastFire, exists := e.cooldowns[cooldownKey(workflowID, ruleID)]
	e.cooldownMu.RUnlock()

	if !exists {
		return false
	}

	return time.Since(lastFire) < cooldown
}

// getFiringCount returns how many times a rule has fired for an execution.
func (e *Evaluator) getFiringCount(executionID, ruleID string) int {
	e.firingMu.RLock()
	defer e.firingMu.RUnlock()
	return e.firingCounts[firingKey(executionID, ruleID)]
}

// cooldownKey creates a key for the cooldowns map.
func cooldownKey(workflowID, ruleID string) string {
	return workflowID + ":" + ruleID
}

// firingKey creates a key for the firingCounts map.
func firingKey(executionID, ruleID string) string {
	return executionID + ":" + ruleID
}

// hasPrefix is a simple string prefix check (avoiding strings import for this small use).
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// ConditionHelpers provides common condition functions for use in rule definitions.
var ConditionHelpers = struct {
	// PhaseIs returns a condition that checks if the execution phase matches.
	PhaseIs func(phase string) ConditionFunc

	// PhaseIn returns a condition that checks if the execution phase is in the list.
	PhaseIn func(phases ...string) ConditionFunc

	// StatusIs returns a condition that checks if the execution status matches.
	StatusIs func(status ExecutionStatus) ConditionFunc

	// IterationLessThan returns a condition that checks if iteration is below limit.
	IterationLessThan func(max int) ConditionFunc

	// IterationEquals returns a condition that checks if iteration equals value.
	IterationEquals func(n int) ConditionFunc

	// HasError returns a condition that checks if execution has an error.
	HasError func() ConditionFunc

	// NoError returns a condition that checks if execution has no error.
	NoError func() ConditionFunc

	// IsWaiting returns a condition that checks if execution is waiting for callback.
	IsWaiting func() ConditionFunc

	// NotWaiting returns a condition that checks if execution is not waiting.
	NotWaiting func() ConditionFunc
}{
	PhaseIs: func(phase string) ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			return base != nil && base.Phase == phase
		}
	},

	PhaseIn: func(phases ...string) ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			if base == nil {
				return false
			}
			for _, p := range phases {
				if base.Phase == p {
					return true
				}
			}
			return false
		}
	},

	StatusIs: func(status ExecutionStatus) ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			return base != nil && base.Status == status
		}
	},

	IterationLessThan: func(max int) ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			return base != nil && base.Iteration < max
		}
	},

	IterationEquals: func(n int) ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			return base != nil && base.Iteration == n
		}
	},

	HasError: func() ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			return base != nil && base.Error != ""
		}
	},

	NoError: func() ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return true // No state means no error
			}
			base := GetExecutionState(ctx.State)
			return base == nil || base.Error == ""
		}
	},

	IsWaiting: func() ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return false
			}
			base := GetExecutionState(ctx.State)
			return base != nil && base.Status == StatusWaiting
		}
	},

	NotWaiting: func() ConditionFunc {
		return func(ctx *RuleContext) bool {
			if ctx.State == nil {
				return true
			}
			base := GetExecutionState(ctx.State)
			return base == nil || base.Status != StatusWaiting
		}
	},
}

// GetExecutionState extracts the embedded ExecutionState from a typed state struct.
// Returns nil if the state is nil or does not contain an ExecutionState.
func GetExecutionState(state any) *ExecutionState {
	if state == nil {
		return nil
	}

	// Try direct type assertion first
	if es, ok := state.(*ExecutionState); ok {
		return es
	}

	// Try to get ExecutionState field via interface
	// This works for structs that embed ExecutionState
	type execStateGetter interface {
		GetExecutionState() *ExecutionState
	}
	if getter, ok := state.(execStateGetter); ok {
		return getter.GetExecutionState()
	}

	// Try reflection as last resort for embedded structs
	return getExecutionStateViaReflection(state)
}

// getExecutionStateViaReflection uses reflection to find embedded ExecutionState.
// This is a fallback for structs that embed ExecutionState without implementing the interface.
func getExecutionStateViaReflection(state any) *ExecutionState {
	// Import reflect only when needed
	// For now, we'll require explicit interface implementation
	// This can be enhanced later if needed
	return nil
}
