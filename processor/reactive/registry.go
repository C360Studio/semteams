package reactive

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/c360studio/semstreams/message"
)

// WorkflowRegistry manages workflow definitions and their associated types.
// It provides thread-safe registration and lookup of workflows.
type WorkflowRegistry struct {
	logger *slog.Logger

	mu        sync.RWMutex
	workflows map[string]*Definition
	// resultTypes maps message type key -> factory function for callback result deserialization
	resultTypes map[string]func() message.Payload
}

// NewWorkflowRegistry creates a new workflow registry.
func NewWorkflowRegistry(logger *slog.Logger) *WorkflowRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &WorkflowRegistry{
		logger:      logger,
		workflows:   make(map[string]*Definition),
		resultTypes: make(map[string]func() message.Payload),
	}
}

// Register registers a workflow definition.
// Returns an error if the workflow ID is already registered.
func (r *WorkflowRegistry) Register(def *Definition) error {
	if def == nil {
		return &RegistryError{
			Op:      "register",
			Message: "definition is nil",
		}
	}

	if def.ID == "" {
		return &RegistryError{
			Op:      "register",
			Message: "workflow ID is required",
		}
	}

	if err := def.Validate(); err != nil {
		return &RegistryError{
			Op:         "register",
			WorkflowID: def.ID,
			Message:    "definition validation failed",
			Cause:      err,
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.workflows[def.ID]; exists {
		return &RegistryError{
			Op:         "register",
			WorkflowID: def.ID,
			Message:    "workflow already registered",
		}
	}

	r.workflows[def.ID] = def

	r.logger.Info("Registered workflow",
		"workflow_id", def.ID,
		"description", def.Description,
		"rules", len(def.Rules))

	return nil
}

// Unregister removes a workflow definition.
func (r *WorkflowRegistry) Unregister(workflowID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.workflows, workflowID)

	r.logger.Info("Unregistered workflow", "workflow_id", workflowID)
}

// Get returns a workflow definition by ID.
// Returns nil if not found.
func (r *WorkflowRegistry) Get(workflowID string) *Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.workflows[workflowID]
}

// List returns all registered workflow IDs.
func (r *WorkflowRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.workflows))
	for id := range r.workflows {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of registered workflows.
func (r *WorkflowRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.workflows)
}

// GetAll returns all registered workflow definitions.
func (r *WorkflowRegistry) GetAll() []*Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]*Definition, 0, len(r.workflows))
	for _, def := range r.workflows {
		defs = append(defs, def)
	}
	return defs
}

// RegisterResultType registers a factory function for deserializing callback results.
// The typeKey should be in format "domain.category.version".
func (r *WorkflowRegistry) RegisterResultType(typeKey string, factory func() message.Payload) error {
	if typeKey == "" {
		return &RegistryError{
			Op:      "register_result_type",
			Message: "type key is required",
		}
	}
	if factory == nil {
		return &RegistryError{
			Op:      "register_result_type",
			Message: "factory function is required",
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.resultTypes[typeKey] = factory

	r.logger.Debug("Registered result type", "type_key", typeKey)

	return nil
}

// GetResultTypeFactory returns the factory function for a result type.
// Returns nil if not found.
func (r *WorkflowRegistry) GetResultTypeFactory(typeKey string) func() message.Payload {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.resultTypes[typeKey]
}

// GetRulesForTrigger returns all rules across all workflows that match the given trigger.
// This is used for routing incoming events to the appropriate rules.
// Note: keyPattern is reserved for future pattern-based filtering.
func (r *WorkflowRegistry) GetRulesForTrigger(triggerMode TriggerMode, bucket, _ string) []*RuleWithWorkflow {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []*RuleWithWorkflow

	for _, def := range r.workflows {
		for i := range def.Rules {
			rule := &def.Rules[i]
			if rule.Trigger.Mode() != triggerMode {
				continue
			}

			// Check KV trigger match
			if triggerMode == TriggerStateOnly || triggerMode == TriggerMessageAndState {
				if rule.Trigger.WatchBucket != bucket {
					continue
				}
				// Pattern matching is handled by the trigger loop
			}

			matches = append(matches, &RuleWithWorkflow{
				Rule:       rule,
				Definition: def,
			})
		}
	}

	return matches
}

// GetRulesForSubject returns all rules that trigger on the given subject pattern.
func (r *WorkflowRegistry) GetRulesForSubject(subject string) []*RuleWithWorkflow {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []*RuleWithWorkflow

	for _, def := range r.workflows {
		for i := range def.Rules {
			rule := &def.Rules[i]
			mode := rule.Trigger.Mode()
			if mode != TriggerMessageOnly && mode != TriggerMessageAndState {
				continue
			}

			// Check if subject matches the rule's subject pattern
			if matchesSubjectPattern(subject, rule.Trigger.Subject) {
				matches = append(matches, &RuleWithWorkflow{
					Rule:       rule,
					Definition: def,
				})
			}
		}
	}

	return matches
}

// RuleWithWorkflow pairs a rule with its parent workflow definition.
type RuleWithWorkflow struct {
	Rule       *RuleDef
	Definition *Definition
}

// matchesSubjectPattern checks if a subject matches a NATS subject pattern.
// Supports * (single token) and > (multiple tokens) wildcards.
func matchesSubjectPattern(subject, pattern string) bool {
	if pattern == "" || subject == "" {
		return false
	}

	// Exact match
	if pattern == subject {
		return true
	}

	// Use the existing MatchesPattern helper
	return MatchesPattern(subject, pattern)
}

// RegistryError represents an error from the workflow registry.
type RegistryError struct {
	Op         string
	WorkflowID string
	Message    string
	Cause      error
}

// Error implements the error interface.
func (e *RegistryError) Error() string {
	if e.WorkflowID != "" {
		return fmt.Sprintf("registry %s [%s]: %s", e.Op, e.WorkflowID, e.Message)
	}
	return fmt.Sprintf("registry %s: %s", e.Op, e.Message)
}

// Unwrap returns the underlying error.
func (e *RegistryError) Unwrap() error {
	return e.Cause
}

// ValidationSummary returns a summary of all registered workflows for debugging.
func (r *WorkflowRegistry) ValidationSummary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.workflows) == 0 {
		return "No workflows registered"
	}

	summary := fmt.Sprintf("Registered workflows: %d\n", len(r.workflows))
	for id, def := range r.workflows {
		summary += fmt.Sprintf("  - %s (%s): %d rules\n", id, def.Description, len(def.Rules))
		for _, rule := range def.Rules {
			summary += fmt.Sprintf("      * %s [%s]\n", rule.ID, rule.Trigger.Mode())
		}
	}

	return summary
}

// Clear removes all registered workflows and result types.
// Used primarily for testing.
func (r *WorkflowRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.workflows = make(map[string]*Definition)
	r.resultTypes = make(map[string]func() message.Payload)

	r.logger.Debug("Cleared registry")
}
