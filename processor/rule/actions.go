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

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
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
	// ActionTypePublishAgent triggers an agentic loop by publishing a TaskMessage
	ActionTypePublishAgent = "publish_agent"
	// ActionTypeTriggerWorkflow triggers a reactive workflow by publishing to workflow.trigger.<workflow_id>
	ActionTypeTriggerWorkflow = "trigger_workflow"
	// ActionTypePublishBoidSignal publishes a Boid steering signal for agent coordination
	ActionTypePublishBoidSignal = "publish_boid_signal"
)

// Action represents an action to execute when a rule fires.
// Actions are triggered by state transitions (OnEnter, OnExit) or
// while a condition remains true (WhileTrue).
type Action struct {
	// Type specifies the action type (publish, add_triple, remove_triple, update_triple, publish_agent)
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

	// Role is the agent role for publish_agent actions (e.g., "general", "architect", "editor")
	Role string `json:"role,omitempty"`

	// Model is the model endpoint name for publish_agent actions
	Model string `json:"model,omitempty"`

	// Prompt is the task prompt template for publish_agent actions
	// Supports variable substitution: $entity.id, $related.id
	Prompt string `json:"prompt,omitempty"`

	// WorkflowID is the workflow identifier for trigger_workflow actions
	WorkflowID string `json:"workflow_id,omitempty"`

	// ContextData provides additional context passed to the workflow
	ContextData map[string]any `json:"context_data,omitempty"`

	// BoidSignalType specifies the type of boid signal: separation, cohesion, or alignment
	BoidSignalType string `json:"boid_signal_type,omitempty"`

	// BoidStrength specifies the steering strength for boid signals (0.0-1.0)
	BoidStrength float64 `json:"boid_strength,omitempty"`
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
	case ActionTypePublishAgent:
		return e.executePublishAgent(ctx, action, entityID, relatedID)
	case ActionTypeTriggerWorkflow:
		return e.executeTriggerWorkflow(ctx, action, entityID, relatedID)
	case ActionTypePublishBoidSignal:
		return e.executePublishBoidSignal(ctx, action, entityID, relatedID)
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

// EntityContext provides entity data for variable substitution in rules.
// This is used for rules-based workflow orchestration where completion
// state from agentic loops is used to trigger follow-up actions.
type EntityContext struct {
	ID         string // The entity/loop ID
	Role       string // Agent role (architect, editor, general, etc.)
	Result     string // The result/output content
	Model      string // Model used for the agent
	TaskID     string // Task identifier
	ParentLoop string // Parent loop ID (for chained workflows)
	Iterations int    // Number of iterations completed
}

// substituteVariablesWithContext replaces template variables with entity context values.
// Supports all basic variables plus extended context fields:
//   - $entity.id: The entity ID
//   - $related.id: The related entity ID (for pair rules)
//   - $entity.role: The agent role
//   - $entity.result: The agent output/result
//   - $entity.model: The model used
//   - $entity.task_id: The task identifier
//   - $entity.parent_loop: The parent loop ID
//   - $entity.iterations: The iteration count
func substituteVariablesWithContext(template string, entity EntityContext, relatedID string) string {
	result := template
	result = strings.ReplaceAll(result, "$entity.id", entity.ID)
	result = strings.ReplaceAll(result, "$related.id", relatedID)
	result = strings.ReplaceAll(result, "$entity.role", entity.Role)
	result = strings.ReplaceAll(result, "$entity.result", entity.Result)
	result = strings.ReplaceAll(result, "$entity.model", entity.Model)
	result = strings.ReplaceAll(result, "$entity.task_id", entity.TaskID)
	result = strings.ReplaceAll(result, "$entity.parent_loop", entity.ParentLoop)
	result = strings.ReplaceAll(result, "$entity.iterations", fmt.Sprintf("%d", entity.Iterations))
	return result
}

// executePublishAgent executes a publish_agent action, triggering an agentic loop.
// It publishes a TaskMessage to the specified NATS subject.
func (e *ActionExecutor) executePublishAgent(ctx context.Context, action Action, entityID, relatedID string) error {
	// Validate required fields
	if action.Subject == "" {
		return errors.New("subject is required for publish_agent action")
	}
	if action.Role == "" {
		return errors.New("role is required for publish_agent action")
	}
	if action.Model == "" {
		return errors.New("model is required for publish_agent action")
	}
	if action.Prompt == "" {
		return errors.New("prompt is required for publish_agent action")
	}

	// Validate role
	validRoles := map[string]bool{
		"general":   true,
		"architect": true,
		"editor":    true,
		"reviewer":  true,
		"fixer":     true,
	}
	if !validRoles[action.Role] {
		return fmt.Errorf("invalid role %q: must be one of: general, architect, editor, reviewer, fixer", action.Role)
	}

	// Substitute variables in subject and prompt
	subject := substituteVariables(action.Subject, entityID, relatedID)
	prompt := substituteVariables(action.Prompt, entityID, relatedID)

	// Generate a unique task ID
	taskID := fmt.Sprintf("rule-%s-%d", entityID, time.Now().UnixNano())

	// Build the TaskMessage
	task := agentic.TaskMessage{
		TaskID: taskID,
		Role:   action.Role,
		Model:  action.Model,
		Prompt: prompt,
	}

	if e.logger != nil {
		e.logger.Debug("Triggering agent task",
			"subject", subject,
			"task_id", taskID,
			"role", action.Role,
			"model", action.Model,
			"entity_id", entityID)
	}

	// Publish via NATS if publisher is configured
	if e.publisher != nil {
		// Wrap task in BaseMessage envelope (required by agentic-loop)
		baseMsg := message.NewBaseMessage(task.Schema(), &task, "rule-engine")
		data, err := json.Marshal(baseMsg)
		if err != nil {
			return fmt.Errorf("marshal task message: %w", err)
		}

		if err := e.publisher.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("publish agent task to %s: %w", subject, err)
		}

		if e.logger != nil {
			e.logger.Debug("Agent task published",
				"subject", subject,
				"task_id", taskID,
				"size", len(data))
		}
	} else if e.logger != nil {
		e.logger.Debug("Agent task not published (no publisher configured)",
			"subject", subject,
			"task_id", taskID)
	}

	return nil
}

// executeTriggerWorkflow triggers a reactive workflow by publishing to workflow.trigger.<workflow_id>.
// This enables rules to initiate complex orchestration workflows while keeping rules simple.
// The payload is wrapped in a BaseMessage for proper deserialization by the reactive workflow engine.
func (e *ActionExecutor) executeTriggerWorkflow(ctx context.Context, action Action, entityID, relatedID string) error {
	if action.WorkflowID == "" {
		return errors.New("workflow_id is required for trigger_workflow action")
	}

	// Build typed trigger payload (implements message.Payload)
	payload := &WorkflowTriggerPayload{
		WorkflowID:  action.WorkflowID,
		EntityID:    entityID,
		TriggeredAt: time.Now().UTC(),
		RelatedID:   relatedID,
		Context:     action.ContextData,
	}

	subject := fmt.Sprintf("workflow.trigger.%s", action.WorkflowID)

	if e.logger != nil {
		e.logger.Debug("Triggering workflow",
			"workflow_id", action.WorkflowID,
			"subject", subject,
			"entity_id", entityID)
	}

	// Publish via NATS if publisher is configured
	if e.publisher != nil {
		// Create BaseMessage with proper type info for deserialization
		msgType := message.Type{
			Domain:   WorkflowTriggerDomain,
			Category: WorkflowTriggerCategory,
			Version:  WorkflowTriggerVersion,
		}
		baseMsg := message.NewBaseMessage(msgType, payload, "rule-processor")

		// BaseMessage.MarshalJSON handles the wire format
		data, err := json.Marshal(baseMsg)
		if err != nil {
			return fmt.Errorf("marshal workflow trigger message: %w", err)
		}

		if err := e.publisher.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("publish workflow trigger to %s: %w", subject, err)
		}

		if e.logger != nil {
			e.logger.Debug("Workflow trigger published",
				"workflow_id", action.WorkflowID,
				"subject", subject,
				"entity_id", entityID,
				"size", len(data))
		}
	} else if e.logger != nil {
		e.logger.Debug("Workflow trigger not published (no publisher configured)",
			"workflow_id", action.WorkflowID,
			"subject", subject,
			"entity_id", entityID)
	}

	return nil
}

// executePublishBoidSignal executes a publish_boid_signal action.
// It publishes a BoidSteeringSignal to the agent.boid.<loopID> subject
// for consumption by the agentic-loop for coordination.
func (e *ActionExecutor) executePublishBoidSignal(ctx context.Context, action Action, entityID, relatedID string) error {
	if action.Subject == "" {
		return errors.New("subject is required for publish_boid_signal action")
	}
	if action.BoidSignalType == "" {
		return errors.New("boid_signal_type is required for publish_boid_signal action")
	}

	// Validate signal type
	validSignalTypes := map[string]bool{
		"separation": true,
		"cohesion":   true,
		"alignment":  true,
	}
	if !validSignalTypes[action.BoidSignalType] {
		return fmt.Errorf("invalid boid_signal_type %q: must be one of: separation, cohesion, alignment", action.BoidSignalType)
	}

	// Substitute variables in subject
	subject := substituteVariables(action.Subject, entityID, relatedID)

	// Build signal payload
	strength := action.BoidStrength
	if strength <= 0 || strength > 1 {
		strength = 0.5 // Default strength
	}

	signal := map[string]any{
		"loop_id":     entityID,
		"signal_type": action.BoidSignalType,
		"strength":    strength,
		"source_rule": "rule_engine",
		"timestamp":   time.Now().Format(time.RFC3339Nano),
	}

	// Add related entity as context
	if relatedID != "" {
		signal["related_id"] = relatedID
	}

	// Include any custom properties
	if action.Properties != nil {
		for k, v := range action.Properties {
			signal[k] = v
		}
	}

	if e.logger != nil {
		e.logger.Debug("Publishing boid signal",
			"subject", subject,
			"signal_type", action.BoidSignalType,
			"entity_id", entityID,
			"strength", strength)
	}

	// Publish via NATS if publisher is configured
	if e.publisher != nil {
		// Wrap in BaseMessage for proper deserialization
		msgType := message.Type{
			Domain:   "boid",
			Category: "signal",
			Version:  "v1",
		}

		// Create a generic payload wrapper
		payload := &message.GenericJSONPayload{Data: signal}
		baseMsg := message.NewBaseMessage(msgType, payload, "rule-processor")

		data, err := json.Marshal(baseMsg)
		if err != nil {
			return fmt.Errorf("marshal boid signal message: %w", err)
		}

		if err := e.publisher.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("publish boid signal to %s: %w", subject, err)
		}

		if e.logger != nil {
			e.logger.Debug("Boid signal published",
				"subject", subject,
				"signal_type", action.BoidSignalType,
				"entity_id", entityID,
				"size", len(data))
		}
	} else if e.logger != nil {
		e.logger.Debug("Boid signal not published (no publisher configured)",
			"subject", subject,
			"signal_type", action.BoidSignalType,
			"entity_id", entityID)
	}

	return nil
}
