// Package rule - Action execution for ECA rules
package rule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"github.com/c360/semstreams/message"
)

// Action type constants define the supported action types for rule execution.
const (
	// ActionTypePublish publishes a message to a NATS subject
	ActionTypePublish = "publish"
	// ActionTypeAddTriple creates a relationship triple in the graph
	ActionTypeAddTriple = "add_triple"
	// ActionTypeRemoveTriple removes a relationship triple from the graph
	ActionTypeRemoveTriple = "remove_triple"
	// ActionTypeUpdateTriple updates metadata on an existing triple
	ActionTypeUpdateTriple = "update_triple"
)

// Action represents an action to execute when a rule fires.
// Actions are triggered by state transitions (OnEnter, OnExit) or
// while a condition remains true (WhileTrue).
type Action struct {
	// Type specifies the action type (publish, add_triple, remove_triple, update_triple)
	Type string `json:"type"`

	// Subject is the NATS subject for publish actions
	Subject string `json:"subject,omitempty"`

	// Predicate is the relationship type for triple actions
	Predicate string `json:"predicate,omitempty"`

	// Object is the target entity or value for triple actions
	Object string `json:"object,omitempty"`

	// TTL specifies optional expiration time for triples (e.g., "5m", "1h")
	TTL string `json:"ttl,omitempty"`

	// Properties contains additional metadata for the action
	Properties map[string]any `json:"properties,omitempty"`
}

// ParseTTL parses the TTL string into a duration.
// Returns 0 duration if TTL is empty (no expiration).
// Returns an error if the TTL format is invalid or negative.
func (a Action) ParseTTL() (time.Duration, error) {
	if a.TTL == "" {
		return 0, nil
	}

	duration, err := time.ParseDuration(a.TTL)
	if err != nil {
		return 0, fmt.Errorf("invalid TTL format: %w", err)
	}

	// Reject negative durations
	if duration < 0 {
		return 0, errors.New("TTL cannot be negative")
	}

	return duration, nil
}

// ActionExecutor executes actions for rules.
// It handles triple mutations, NATS publishing, and other action types.
type ActionExecutor struct {
	logger *slog.Logger
	// Future: Add dependencies for triple mutation API, NATS publisher
}

// NewActionExecutor creates a new ActionExecutor with the given logger.
// If logger is nil, uses the default logger.
func NewActionExecutor(logger *slog.Logger) *ActionExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &ActionExecutor{
		logger: logger,
	}
}

// Execute runs the given action in the context of an entity.
// The entityID is the subject entity, and relatedID is an optional related entity
// (used for pair rules and relationship triples).
func (e *ActionExecutor) Execute(ctx context.Context, action Action, entityID string, relatedID string) error {
	switch action.Type {
	case ActionTypeAddTriple:
		_, err := e.ExecuteAddTriple(ctx, action, entityID, relatedID)
		return err
	case ActionTypeRemoveTriple:
		return e.ExecuteRemoveTriple(ctx, action, entityID, relatedID)
	case ActionTypePublish:
		return e.executePublish(ctx, action, entityID, relatedID)
	case ActionTypeUpdateTriple:
		return e.executeUpdateTriple(ctx, action, entityID, relatedID)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// ExecuteAddTriple executes an add_triple action, creating a new semantic triple.
// Returns the created triple and any error that occurred.
func (e *ActionExecutor) ExecuteAddTriple(_ context.Context, action Action, entityID, relatedID string) (message.Triple, error) {
	// Validate predicate is present
	if action.Predicate == "" {
		return message.Triple{}, errors.New("predicate is required for add_triple action")
	}

	// Substitute variables in predicate and object
	predicate := substituteVariables(action.Predicate, entityID, relatedID)
	object := substituteVariables(action.Object, entityID, relatedID)

	// Parse TTL
	ttl, err := action.ParseTTL()
	if err != nil {
		return message.Triple{}, fmt.Errorf("parse TTL: %w", err)
	}

	// Calculate expiration time if TTL is set
	var expiresAt *time.Time
	if ttl > 0 {
		expTime := time.Now().Add(ttl)
		expiresAt = &expTime
	}

	// Create the triple
	triple := message.Triple{
		Subject:    entityID,
		Predicate:  predicate,
		Object:     object,
		Source:     "rule_engine", // TODO: Make configurable
		Timestamp:  time.Now(),
		Confidence: 1.0, // TODO: Make configurable
		ExpiresAt:  expiresAt,
	}

	if e.logger != nil {
		e.logger.Debug("Adding triple",
			"entity_id", entityID,
			"predicate", predicate,
			"object", object,
			"ttl", ttl,
			"expires_at", expiresAt)
	}

	// TODO: Call triple mutation API to persist the triple
	// This will be integrated with the graph processor in a future task

	return triple, nil
}

// ExecuteRemoveTriple executes a remove_triple action, removing a semantic triple.
func (e *ActionExecutor) ExecuteRemoveTriple(_ context.Context, action Action, entityID, relatedID string) error {
	// Validate predicate is present
	if action.Predicate == "" {
		return errors.New("predicate is required for remove_triple action")
	}

	predicate := substituteVariables(action.Predicate, entityID, relatedID)
	object := substituteVariables(action.Object, entityID, relatedID)

	if e.logger != nil {
		e.logger.Debug("Removing triple",
			"entity_id", entityID,
			"predicate", predicate,
			"object", object)
	}

	// TODO: Call triple mutation API to remove the triple
	// This will be integrated with the graph processor in a future task

	return nil
}

// executePublish executes a publish action, sending a message to a NATS subject.
func (e *ActionExecutor) executePublish(_ context.Context, action Action, entityID, relatedID string) error {
	subject := substituteVariables(action.Subject, entityID, relatedID)

	if e.logger != nil {
		e.logger.Debug("Publishing message",
			"subject", subject,
			"entity_id", entityID,
			"properties", action.Properties)
	}

	// TODO: Publish to NATS subject
	// This will be integrated with NATS client in a future task

	return nil
}

// executeUpdateTriple executes an update_triple action, updating triple metadata.
func (e *ActionExecutor) executeUpdateTriple(_ context.Context, action Action, entityID, relatedID string) error {
	predicate := substituteVariables(action.Predicate, entityID, relatedID)
	object := substituteVariables(action.Object, entityID, relatedID)

	if e.logger != nil {
		e.logger.Debug("Updating triple",
			"entity_id", entityID,
			"predicate", predicate,
			"object", object,
			"properties", action.Properties)
	}

	// TODO: Update triple metadata via graph processor API
	// This will be integrated with the graph processor in a future task

	return nil
}

// substituteVariables replaces template variables with actual entity IDs.
// Supported variables:
//   - $entity.id: The primary entity ID
//   - $related.id: The related entity ID (for pair rules)
func substituteVariables(template, entityID, relatedID string) string {
	result := template
	result = strings.ReplaceAll(result, "$entity.id", entityID)
	result = strings.ReplaceAll(result, "$related.id", relatedID)
	return result
}
