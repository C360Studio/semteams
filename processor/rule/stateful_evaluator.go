// Package rule - Stateful rule evaluation with state tracking
package rule

import (
	"context"
	"errors"
	"log/slog"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/processor/rule/expression"
)

// Compile-time interface check
var _ ActionExecutorInterface = (*ActionExecutor)(nil)

// StatefulEvaluator handles rule evaluation with state tracking.
// It manages state transitions (entered/exited/none) and executes
// the appropriate actions based on the transition type.
type StatefulEvaluator struct {
	stateTracker   *StateTracker
	actionExecutor ActionExecutorInterface
	exprEvaluator  *expression.Evaluator
	logger         *slog.Logger
}

// ActionExecutorInterface defines the interface for action execution.
// Actions receive an ExecutionContext with the full entity state and match state,
// replacing the previous (entityID, relatedID string) signature.
type ActionExecutorInterface interface {
	Execute(ctx context.Context, action Action, ec *ExecutionContext) error
}

// NewStatefulEvaluator creates a new stateful evaluator with the given dependencies.
// If logger is nil, uses the default logger.
func NewStatefulEvaluator(stateTracker *StateTracker, actionExecutor ActionExecutorInterface, logger *slog.Logger) *StatefulEvaluator {
	if logger == nil {
		logger = slog.Default()
	}
	return &StatefulEvaluator{
		stateTracker:   stateTracker,
		actionExecutor: actionExecutor,
		exprEvaluator:  expression.NewExpressionEvaluator(),
		logger:         logger,
	}
}

// EvaluateWithState evaluates a rule and fires appropriate actions based on state transitions.
// It:
//  1. Retrieves previous state from StateTracker (treats missing state as false)
//  2. Detects transition (entered/exited/none) by comparing previous and current match state
//  3. Fires appropriate actions based on transition:
//     - TransitionEntered: Execute all OnEnter actions (increments iteration)
//     - TransitionExited: Execute all OnExit actions (preserves iteration)
//     - TransitionNone + currentlyMatching: Execute all WhileTrue actions
//  4. Evaluates When clauses on each action before execution
//  5. Persists new state to StateTracker
//
// The entity and related parameters provide typed entity state for When clause evaluation
// and are passed through to actions via ExecutionContext. Pass nil for message-path rules.
//
// Returns the transition that occurred and any error encountered.
func (e *StatefulEvaluator) EvaluateWithState(
	ctx context.Context,
	ruleDef Definition,
	entityID string,
	relatedID string, // empty for single-entity rules
	currentlyMatching bool,
	entity *gtypes.EntityState, // nil for message-path rules
	related *gtypes.EntityState, // nil for single-entity or message-path rules
) (Transition, error) {
	// Build entity key (single entity or canonical pair)
	var entityKey string
	if relatedID == "" {
		entityKey = entityID
	} else {
		entityKey = buildPairKey(entityID, relatedID)
	}

	// Get previous state
	prevState, err := e.stateTracker.Get(ctx, ruleDef.ID, entityKey)
	wasMatching := false

	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			wasMatching = false
		} else {
			return TransitionNone, err
		}
	} else {
		wasMatching = prevState.IsMatching
	}

	// Detect transition
	transition := DetectTransition(wasMatching, currentlyMatching)

	// Build iteration count for new state
	iteration := prevState.Iteration
	if transition == TransitionEntered {
		iteration++
	}

	// Build match state for ExecutionContext (before persisting so actions see current state)
	matchState := &MatchState{
		RuleID:         ruleDef.ID,
		EntityKey:      entityKey,
		IsMatching:     currentlyMatching,
		LastTransition: string(transition),
		TransitionAt:   time.Now(),
		LastChecked:    time.Now(),
		Iteration:      iteration,
		MaxIterations:  ruleDef.MaxIterations,
	}

	// Build execution context for actions
	ec := &ExecutionContext{
		EntityID:  entityID,
		RelatedID: relatedID,
		Entity:    entity,
		Related:   related,
		State:     matchState,
	}

	// Build state fields for $state.* pseudo-field evaluation in When clauses
	stateFields := expression.StateFields{
		"$state.iteration":       iteration,
		"$state.max_iterations":  ruleDef.MaxIterations,
		"$state.last_transition": string(transition),
	}

	// Execute actions based on transition
	var actionsToExecute []Action

	switch transition {
	case TransitionEntered:
		actionsToExecute = ruleDef.OnEnter
		e.logger.Debug("Rule entered",
			"rule_id", ruleDef.ID,
			"entity_id", entityID,
			"related_id", relatedID,
			"iteration", iteration,
			"action_count", len(actionsToExecute))

	case TransitionExited:
		actionsToExecute = ruleDef.OnExit
		e.logger.Debug("Rule exited",
			"rule_id", ruleDef.ID,
			"entity_id", entityID,
			"related_id", relatedID,
			"action_count", len(actionsToExecute))

	case TransitionNone:
		if currentlyMatching {
			actionsToExecute = ruleDef.WhileTrue
			e.logger.Debug("Rule while true",
				"rule_id", ruleDef.ID,
				"entity_id", entityID,
				"related_id", relatedID,
				"action_count", len(actionsToExecute))
		}
	}

	// Execute all actions for this transition, evaluating When clauses
	for _, action := range actionsToExecute {
		// Evaluate When clause if present
		if len(action.When) > 0 {
			match, whenErr := e.evaluateWhen(action.When, entity, stateFields)
			if whenErr != nil {
				e.logger.Warn("When clause evaluation failed, skipping action",
					"rule_id", ruleDef.ID,
					"entity_id", entityID,
					"action_type", action.Type,
					"error", whenErr)
				continue
			}
			if !match {
				e.logger.Debug("Action skipped by When clause",
					"rule_id", ruleDef.ID,
					"entity_id", entityID,
					"action_type", action.Type)
				continue
			}
		}

		if err := e.actionExecutor.Execute(ctx, action, ec); err != nil {
			e.logger.Error("Failed to execute action",
				"rule_id", ruleDef.ID,
				"entity_id", entityID,
				"related_id", relatedID,
				"action_type", action.Type,
				"error", err)
			// Continue executing remaining actions despite error
		}
	}

	// Persist new state
	if err := e.stateTracker.Set(ctx, *matchState); err != nil {
		e.logger.Warn("Failed to persist rule state",
			"rule_id", ruleDef.ID,
			"entity_key", entityKey,
			"error", err)
		return transition, err
	}

	return transition, nil
}

// evaluateWhen evaluates a When clause (action-level guard conditions).
// When clauses use AND logic by default — all conditions must match for the action to execute.
// If entity is nil (message-path rules), the When clause is skipped and returns true.
func (e *StatefulEvaluator) evaluateWhen(
	conditions []expression.ConditionExpression,
	entity *gtypes.EntityState,
	stateFields expression.StateFields,
) (bool, error) {
	// For message-path rules without entity state, skip When evaluation
	// unless all conditions reference $state.* fields
	expr := expression.LogicalExpression{
		Conditions: conditions,
		Logic:      expression.LogicAnd,
	}
	return e.exprEvaluator.EvaluateWithStateFields(entity, stateFields, expr)
}
