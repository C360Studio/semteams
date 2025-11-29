// Package rule - Stateful rule evaluation with state tracking
package rule

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Compile-time interface check
var _ ActionExecutorInterface = (*ActionExecutor)(nil)

// StatefulEvaluator handles rule evaluation with state tracking.
// It manages state transitions (entered/exited/none) and executes
// the appropriate actions based on the transition type.
type StatefulEvaluator struct {
	stateTracker   *StateTracker
	actionExecutor ActionExecutorInterface
	logger         *slog.Logger
}

// ActionExecutorInterface defines the interface for action execution.
// This allows for easy mocking in tests.
type ActionExecutorInterface interface {
	Execute(ctx context.Context, action Action, entityID string, relatedID string) error
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
		logger:         logger,
	}
}

// EvaluateWithState evaluates a rule and fires appropriate actions based on state transitions.
// It:
//  1. Retrieves previous state from StateTracker (treats missing state as false)
//  2. Detects transition (entered/exited/none) by comparing previous and current match state
//  3. Fires appropriate actions based on transition:
//     - TransitionEntered: Execute all OnEnter actions
//     - TransitionExited: Execute all OnExit actions
//     - TransitionNone + currentlyMatching: Execute all WhileTrue actions
//  4. Persists new state to StateTracker
//
// Returns the transition that occurred and any error encountered.
func (e *StatefulEvaluator) EvaluateWithState(
	ctx context.Context,
	ruleDef Definition,
	entityID string,
	relatedID string, // empty for single-entity rules
	currentlyMatching bool,
) (Transition, error) {
	// Build entity key (single entity or canonical pair)
	var entityKey string
	if relatedID == "" {
		// Single entity rule
		entityKey = entityID
	} else {
		// Pair rule - use canonical sorted key
		entityKey = buildPairKey(entityID, relatedID)
	}

	// Get previous state
	prevState, err := e.stateTracker.Get(ctx, ruleDef.ID, entityKey)
	wasMatching := false

	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			// No previous state - treat as wasMatching = false
			wasMatching = false
		} else {
			// Real error - return it
			return TransitionNone, err
		}
	} else {
		wasMatching = prevState.IsMatching
	}

	// Detect transition
	transition := DetectTransition(wasMatching, currentlyMatching)

	// Execute actions based on transition
	var actionsToExecute []Action

	switch transition {
	case TransitionEntered:
		actionsToExecute = ruleDef.OnEnter
		e.logger.Debug("Rule entered",
			"rule_id", ruleDef.ID,
			"entity_id", entityID,
			"related_id", relatedID,
			"action_count", len(actionsToExecute))

	case TransitionExited:
		actionsToExecute = ruleDef.OnExit
		e.logger.Debug("Rule exited",
			"rule_id", ruleDef.ID,
			"entity_id", entityID,
			"related_id", relatedID,
			"action_count", len(actionsToExecute))

	case TransitionNone:
		// No transition - check if we should fire WhileTrue actions
		if currentlyMatching {
			actionsToExecute = ruleDef.WhileTrue
			e.logger.Debug("Rule while true",
				"rule_id", ruleDef.ID,
				"entity_id", entityID,
				"related_id", relatedID,
				"action_count", len(actionsToExecute))
		}
	}

	// Execute all actions for this transition
	for _, action := range actionsToExecute {
		if err := e.actionExecutor.Execute(ctx, action, entityID, relatedID); err != nil {
			e.logger.Error("Failed to execute action",
				"rule_id", ruleDef.ID,
				"entity_id", entityID,
				"related_id", relatedID,
				"action_type", action.Type,
				"error", err)
			// Continue executing remaining actions despite error
		}
	}

	// Build and persist new state
	newState := MatchState{
		RuleID:         ruleDef.ID,
		EntityKey:      entityKey,
		IsMatching:     currentlyMatching,
		LastTransition: string(transition),
		TransitionAt:   time.Now(),
		LastChecked:    time.Now(),
	}

	if err := e.stateTracker.Set(ctx, newState); err != nil {
		e.logger.Warn("Failed to persist rule state",
			"rule_id", ruleDef.ID,
			"entity_key", entityKey,
			"error", err)
		return transition, err
	}

	return transition, nil
}
