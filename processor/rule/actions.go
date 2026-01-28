// Package rule - Action execution for ECA rules
package rule

import (
	"context"
	"encoding/json"
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

// TripleMutator handles triple mutations via NATS request/response.
// The returned uint64 is the KV revision after the write, used for feedback loop prevention.
type TripleMutator interface {
	// AddTriple adds a triple via NATS request/response and returns the KV revision
	AddTriple(ctx context.Context, triple message.Triple) (uint64, error)
	// RemoveTriple removes a triple via NATS request/response and returns the KV revision
	RemoveTriple(ctx context.Context, subject, predicate string) (uint64, error)
}

// Publisher handles publishing messages to NATS subjects.
// It abstracts the decision between core NATS and JetStream publishing.
type Publisher interface {
	// Publish sends a message to a NATS subject.
	// The implementation determines whether to use core NATS or JetStream
	// based on port configuration.
	Publish(ctx context.Context, subject string, data []byte) error
}

// ActionExecutor executes actions for rules.
// It handles triple mutations, NATS publishing, and other action types.
type ActionExecutor struct {
	logger        *slog.Logger
	tripleMutator TripleMutator // Optional: if nil, triple mutations are logged but not persisted
	publisher     Publisher     // Optional: if nil, publish actions are logged but not sent
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

// NewActionExecutorWithMutator creates a new ActionExecutor with triple mutation support.
// The mutator enables actual persistence of triple operations via NATS request/response.
func NewActionExecutorWithMutator(logger *slog.Logger, mutator TripleMutator) *ActionExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &ActionExecutor{
		logger:        logger,
		tripleMutator: mutator,
	}
}

// NewActionExecutorFull creates a new ActionExecutor with full functionality.
// The mutator enables triple persistence, and the publisher enables NATS publishing.
func NewActionExecutorFull(logger *slog.Logger, mutator TripleMutator, publisher Publisher) *ActionExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &ActionExecutor{
		logger:        logger,
		tripleMutator: mutator,
		publisher:     publisher,
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
// If a TripleMutator is configured, the triple is persisted via NATS request/response.
func (e *ActionExecutor) ExecuteAddTriple(ctx context.Context, action Action, entityID, relatedID string) (message.Triple, error) {
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
		Source:     "rule_engine",
		Timestamp:  time.Now(),
		Confidence: 1.0,
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

	// Persist triple via NATS request/response if mutator is configured
	if e.tripleMutator != nil {
		revision, err := e.tripleMutator.AddTriple(ctx, triple)
		if err != nil {
			return message.Triple{}, fmt.Errorf("persist triple: %w", err)
		}
		if e.logger != nil {
			e.logger.Debug("Triple persisted",
				"entity_id", entityID,
				"predicate", predicate,
				"kv_revision", revision)
		}
	} else if e.logger != nil {
		e.logger.Debug("Triple not persisted (no mutator configured)",
			"entity_id", entityID,
			"predicate", predicate)
	}

	return triple, nil
}

// ExecuteRemoveTriple executes a remove_triple action, removing a semantic triple.
// If a TripleMutator is configured, the triple is removed via NATS request/response.
func (e *ActionExecutor) ExecuteRemoveTriple(ctx context.Context, action Action, entityID, relatedID string) error {
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

	// Remove triple via NATS request/response if mutator is configured
	if e.tripleMutator != nil {
		revision, err := e.tripleMutator.RemoveTriple(ctx, entityID, predicate)
		if err != nil {
			return fmt.Errorf("remove triple: %w", err)
		}
		if e.logger != nil {
			e.logger.Debug("Triple removed",
				"entity_id", entityID,
				"predicate", predicate,
				"kv_revision", revision)
		}
	} else if e.logger != nil {
		e.logger.Debug("Triple not removed (no mutator configured)",
			"entity_id", entityID,
			"predicate", predicate)
	}

	return nil
}

// executePublish executes a publish action, sending a message to a NATS subject.
func (e *ActionExecutor) executePublish(ctx context.Context, action Action, entityID, relatedID string) error {
	// Validate subject is present
	if action.Subject == "" {
		return errors.New("subject is required for publish action")
	}

	subject := substituteVariables(action.Subject, entityID, relatedID)

	// Build the message payload
	payload := map[string]any{
		"entity_id":  entityID,
		"subject":    subject,
		"timestamp":  time.Now().Format(time.RFC3339Nano),
		"source":     "rule_engine",
		"properties": action.Properties,
	}
	if relatedID != "" {
		payload["related_id"] = relatedID
	}

	if e.logger != nil {
		e.logger.Debug("Publishing message",
			"subject", subject,
			"entity_id", entityID,
			"related_id", relatedID,
			"properties", action.Properties)
	}

	// Publish via NATS if publisher is configured
	if e.publisher != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal publish payload: %w", err)
		}

		if err := e.publisher.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("publish to %s: %w", subject, err)
		}

		if e.logger != nil {
			e.logger.Debug("Message published",
				"subject", subject,
				"entity_id", entityID,
				"size", len(data))
		}
	} else if e.logger != nil {
		e.logger.Debug("Message not published (no publisher configured)",
			"subject", subject,
			"entity_id", entityID)
	}

	return nil
}

// executeUpdateTriple executes an update_triple action by removing the existing triple
// and adding a new one with the updated values. This is the only way to "update" a triple
// since triples are identified by (subject, predicate, object) - changing any of those
// creates a different triple.
func (e *ActionExecutor) executeUpdateTriple(ctx context.Context, action Action, entityID, relatedID string) error {
	// Validate predicate is present
	if action.Predicate == "" {
		return errors.New("predicate is required for update_triple action")
	}

	predicate := substituteVariables(action.Predicate, entityID, relatedID)
	object := substituteVariables(action.Object, entityID, relatedID)

	if e.logger != nil {
		e.logger.Debug("Updating triple (remove + add)",
			"entity_id", entityID,
			"predicate", predicate,
			"object", object,
			"properties", action.Properties)
	}

	// Step 1: Remove existing triple with this predicate
	if e.tripleMutator != nil {
		_, err := e.tripleMutator.RemoveTriple(ctx, entityID, predicate)
		if err != nil {
			// Log but continue - triple may not exist, which is fine for update
			if e.logger != nil {
				e.logger.Debug("No existing triple to remove (or error)",
					"entity_id", entityID,
					"predicate", predicate,
					"error", err)
			}
		}
	}

	// Step 2: Add the new triple with updated values
	// Parse TTL
	ttl, err := action.ParseTTL()
	if err != nil {
		return fmt.Errorf("parse TTL: %w", err)
	}

	var expiresAt *time.Time
	if ttl > 0 {
		expTime := time.Now().Add(ttl)
		expiresAt = &expTime
	}

	triple := message.Triple{
		Subject:    entityID,
		Predicate:  predicate,
		Object:     object,
		Source:     "rule_engine",
		Timestamp:  time.Now(),
		Confidence: 1.0,
		ExpiresAt:  expiresAt,
	}

	if e.tripleMutator != nil {
		revision, err := e.tripleMutator.AddTriple(ctx, triple)
		if err != nil {
			return fmt.Errorf("add updated triple: %w", err)
		}
		if e.logger != nil {
			e.logger.Debug("Triple updated",
				"entity_id", entityID,
				"predicate", predicate,
				"object", object,
				"kv_revision", revision)
		}
	} else if e.logger != nil {
		e.logger.Debug("Triple not updated (no mutator configured)",
			"entity_id", entityID,
			"predicate", predicate)
	}

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
