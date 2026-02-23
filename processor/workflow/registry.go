package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
	"github.com/nats-io/nats.go/jetstream"
)

// Registry manages workflow definitions
type Registry struct {
	bucket jetstream.KeyValue
	logger *slog.Logger

	mu        sync.RWMutex
	workflows map[string]*wfschema.Definition
	byTrigger map[string]string // subject -> workflow ID

	// Watch lifecycle management
	watchCancel context.CancelFunc
	watchDone   chan struct{}
}

// NewRegistry creates a new workflow registry
func NewRegistry(bucket jetstream.KeyValue, logger *slog.Logger) *Registry {
	return &Registry{
		bucket:    bucket,
		logger:    logger,
		workflows: make(map[string]*wfschema.Definition),
		byTrigger: make(map[string]string),
	}
}

// Load loads all workflow definitions from the KV bucket
func (r *Registry) Load(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing
	r.workflows = make(map[string]*wfschema.Definition)
	r.byTrigger = make(map[string]string)

	// List all keys
	keys, err := r.bucket.Keys(ctx)
	if err != nil {
		// Check for empty bucket error using proper error types
		if errors.Is(err, jetstream.ErrNoKeysFound) || isNoKeysFoundError(err) {
			r.logger.Info("No workflow definitions found in bucket")
			return nil
		}
		return errs.WrapTransient(err, "workflow-registry", "Load", "list workflow keys")
	}

	for _, key := range keys {
		entry, err := r.bucket.Get(ctx, key)
		if err != nil {
			r.logger.Warn("Failed to get workflow definition", "key", key, "error", err)
			continue
		}

		var workflow wfschema.Definition
		if err := json.Unmarshal(entry.Value(), &workflow); err != nil {
			r.logger.Warn("Failed to unmarshal workflow definition", "key", key, "error", err)
			continue
		}

		if err := workflow.Validate(); err != nil {
			r.logger.Warn("Invalid workflow definition", "key", key, "error", err)
			continue
		}

		// Validate workflow types against the global payload registry
		// This is a non-blocking validation to allow gradual adoption
		warnings := validateWorkflowTypes(&workflow, component.GlobalPayloadRegistry())
		for _, warning := range warnings {
			r.logger.Warn(warning,
				slog.String("workflow_id", workflow.ID),
				slog.String("workflow_name", workflow.Name))
		}

		r.workflows[workflow.ID] = &workflow
		r.byTrigger[workflow.Trigger.Subject] = workflow.ID

		r.logger.Info("Loaded workflow definition",
			slog.String("id", workflow.ID),
			slog.String("name", workflow.Name),
			slog.String("trigger", workflow.Trigger.Subject),
			slog.Bool("enabled", workflow.Enabled))
	}

	r.logger.Info("Workflow registry loaded", slog.Int("count", len(r.workflows)))
	return nil
}

// isNoKeysFoundError checks if the error indicates no keys were found
// This handles both the sentinel error and string-based errors from different NATS versions
func isNoKeysFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no keys found") || strings.Contains(errStr, "no keys")
}

// Get retrieves a workflow by ID
func (r *Registry) Get(id string) (*wfschema.Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workflow, ok := r.workflows[id]
	return workflow, ok
}

// GetByTrigger retrieves a workflow by trigger subject
func (r *Registry) GetByTrigger(subject string) (*wfschema.Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byTrigger[subject]
	if !ok {
		return nil, false
	}

	workflow, ok := r.workflows[id]
	return workflow, ok
}

// List returns all workflow definitions
func (r *Registry) List() []*wfschema.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workflows := make([]*wfschema.Definition, 0, len(r.workflows))
	for _, w := range r.workflows {
		workflows = append(workflows, w)
	}
	return workflows
}

// ListEnabled returns all enabled workflow definitions
func (r *Registry) ListEnabled() []*wfschema.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workflows := make([]*wfschema.Definition, 0)
	for _, w := range r.workflows {
		if w.Enabled {
			workflows = append(workflows, w)
		}
	}
	return workflows
}

// TriggerSubjects returns all trigger subjects from enabled workflows
func (r *Registry) TriggerSubjects() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subjects := make([]string, 0)
	for subject, id := range r.byTrigger {
		if w, ok := r.workflows[id]; ok && w.Enabled {
			subjects = append(subjects, subject)
		}
	}
	return subjects
}

// Register adds or updates a workflow definition.
// If the workflow already exists in KV, it only updates if the new version is newer.
// This ensures file-based definitions don't overwrite newer versions that may have
// been updated at runtime via KV.
func (r *Registry) Register(ctx context.Context, workflow *wfschema.Definition) error {
	if err := workflow.Validate(); err != nil {
		return errs.WrapInvalid(err, "workflow-registry", "Register", "validate workflow")
	}

	// Validate workflow types against the global payload registry
	// This is a non-blocking validation to allow gradual adoption
	warnings := validateWorkflowTypes(workflow, component.GlobalPayloadRegistry())
	for _, warning := range warnings {
		r.logger.Warn(warning,
			slog.String("workflow_id", workflow.ID),
			slog.String("workflow_name", workflow.Name))
	}

	// Check if workflow already exists in KV
	existing, err := r.bucket.Get(ctx, workflow.ID)
	if err == nil {
		// Entry exists - compare versions
		var existingDef wfschema.Definition
		if unmarshalErr := json.Unmarshal(existing.Value(), &existingDef); unmarshalErr != nil {
			r.logger.Warn("Failed to unmarshal existing workflow definition, will overwrite",
				slog.String("workflow_id", workflow.ID),
				slog.String("error", unmarshalErr.Error()))
			// Fall through to Put()
		} else {
			if !IsNewerVersion(workflow.Version, existingDef.Version) {
				r.logger.Debug("Skipping registration - existing version is same or newer",
					slog.String("workflow_id", workflow.ID),
					slog.String("existing_version", existingDef.Version),
					slog.String("new_version", workflow.Version))
				// Update in-memory map from existing KV data (don't overwrite KV)
				r.updateInMemory(&existingDef)
				return nil
			}
			r.logger.Info("Updating workflow to newer version",
				slog.String("workflow_id", workflow.ID),
				slog.String("old_version", existingDef.Version),
				slog.String("new_version", workflow.Version))
		}
	}
	// If Get() failed with key not found, proceed with registration

	data, err := json.Marshal(workflow)
	if err != nil {
		return errs.WrapInvalid(err, "workflow-registry", "Register", "marshal workflow")
	}

	if _, err := r.bucket.Put(ctx, workflow.ID, data); err != nil {
		return errs.WrapTransient(err, "workflow-registry", "Register", "save workflow to KV bucket")
	}

	r.updateInMemory(workflow)

	r.logger.Info("Registered workflow",
		slog.String("id", workflow.ID),
		slog.String("name", workflow.Name),
		slog.String("version", workflow.Version),
		slog.String("trigger", workflow.Trigger.Subject))

	return nil
}

// updateInMemory updates the in-memory workflow maps.
// Caller must not hold the mutex.
func (r *Registry) updateInMemory(workflow *wfschema.Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old trigger mapping if it exists
	if existing, ok := r.workflows[workflow.ID]; ok {
		delete(r.byTrigger, existing.Trigger.Subject)
	}

	r.workflows[workflow.ID] = workflow
	r.byTrigger[workflow.Trigger.Subject] = workflow.ID
}

// Unregister removes a workflow definition
func (r *Registry) Unregister(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	workflow, ok := r.workflows[id]
	if !ok {
		return errs.WrapInvalid(fmt.Errorf("workflow not found: %s", id), "workflow-registry", "Unregister", "find workflow")
	}

	if err := r.bucket.Delete(ctx, id); err != nil {
		return errs.WrapTransient(err, "workflow-registry", "Unregister", "delete from KV bucket")
	}

	delete(r.workflows, id)
	delete(r.byTrigger, workflow.Trigger.Subject)

	r.logger.Info("Unregistered workflow", slog.String("id", id))
	return nil
}

// Watch watches for workflow definition changes.
// The watcher runs until the context is cancelled or StopWatch is called.
func (r *Registry) Watch(ctx context.Context) error {
	watcher, err := r.bucket.Watch(ctx, ">")
	if err != nil {
		return errs.WrapTransient(err, "workflow-registry", "Watch", "create KV watcher")
	}

	// Create a cancellable context for the watch goroutine
	watchCtx, cancel := context.WithCancel(ctx)
	r.watchCancel = cancel
	r.watchDone = make(chan struct{})

	go func() {
		defer close(r.watchDone)
		defer func() {
			if err := watcher.Stop(); err != nil {
				r.logger.Warn("Failed to stop watcher", "error", err)
			}
		}()

		for {
			select {
			case <-watchCtx.Done():
				return
			case entry := <-watcher.Updates():
				if entry == nil {
					continue
				}

				r.handleWatchUpdate(entry)
			}
		}
	}()

	return nil
}

// StopWatch stops the workflow definition watcher and waits for it to complete
func (r *Registry) StopWatch() {
	if r.watchCancel != nil {
		r.watchCancel()
	}
	if r.watchDone != nil {
		<-r.watchDone
	}
}

// handleWatchUpdate processes a KV watch update
func (r *Registry) handleWatchUpdate(entry jetstream.KeyValueEntry) {
	key := entry.Key()

	if entry.Operation() == jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge {
		r.mu.Lock()
		if workflow, ok := r.workflows[key]; ok {
			delete(r.byTrigger, workflow.Trigger.Subject)
			delete(r.workflows, key)
			r.logger.Info("Workflow deleted via watch", slog.String("id", key))
		}
		r.mu.Unlock()
		return
	}

	var workflow wfschema.Definition
	if err := json.Unmarshal(entry.Value(), &workflow); err != nil {
		r.logger.Warn("Failed to unmarshal workflow from watch", "key", key, "error", err)
		return
	}

	if err := workflow.Validate(); err != nil {
		r.logger.Warn("Invalid workflow from watch", "key", key, "error", err)
		return
	}

	// Validate workflow types against the global payload registry
	// This is a non-blocking validation to allow gradual adoption
	warnings := validateWorkflowTypes(&workflow, component.GlobalPayloadRegistry())
	for _, warning := range warnings {
		r.logger.Warn(warning,
			slog.String("workflow_id", workflow.ID),
			slog.String("workflow_name", workflow.Name))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old trigger mapping if it exists
	if existing, ok := r.workflows[workflow.ID]; ok {
		delete(r.byTrigger, existing.Trigger.Subject)
	}

	r.workflows[workflow.ID] = &workflow
	r.byTrigger[workflow.Trigger.Subject] = workflow.ID

	r.logger.Info("Workflow updated via watch",
		slog.String("id", workflow.ID),
		slog.String("name", workflow.Name))
}

// validateWorkflowTypes checks workflow type annotations and payload mappings at load time.
// This includes:
// 1. Validating input_type and output_type are registered in the payload registry
// 2. Validating payload_mapping paths have valid format (root.field.subfield...)
//
// Returns a slice of warning messages for any issues found.
// This is non-blocking validation to allow gradual adoption of typed payloads.
func validateWorkflowTypes(def *wfschema.Definition, payloadRegistry *component.PayloadRegistry) []string {
	var warnings []string

	// Validate types for each step
	for _, step := range def.Steps {
		warnings = append(warnings, validateStepTypes(&step, def, payloadRegistry)...)
	}

	return warnings
}

// validateStepTypes validates inputs and outputs for a single step
// and recursively validates nested parallel steps.
func validateStepTypes(step *wfschema.StepDef, def *wfschema.Definition, payloadRegistry *component.PayloadRegistry) []string {
	var warnings []string

	// Validate input interfaces
	for inputName, inputRef := range step.Inputs {
		if inputRef.Interface != "" {
			domain, category, version, err := ParseTypeString(inputRef.Interface)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("step %q input %q has invalid interface format: %v", step.Name, inputName, err))
			} else if payload := payloadRegistry.CreatePayload(domain, category, version); payload == nil {
				warnings = append(warnings, fmt.Sprintf("step %q input %q declares interface %q which is not registered in payload registry", step.Name, inputName, inputRef.Interface))
			}
		}

		// Validate 'from' reference
		fromWarnings := validateFromReference(inputRef.From, step.Name, inputName, def)
		warnings = append(warnings, fromWarnings...)
	}

	// Validate output interfaces
	for outputName, outputDef := range step.Outputs {
		if outputDef.Interface != "" {
			domain, category, version, err := ParseTypeString(outputDef.Interface)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("step %q output %q has invalid interface format: %v", step.Name, outputName, err))
			} else if payload := payloadRegistry.CreatePayload(domain, category, version); payload == nil {
				warnings = append(warnings, fmt.Sprintf("step %q output %q declares interface %q which is not registered in payload registry", step.Name, outputName, outputDef.Interface))
			}
		}
	}

	// Recursively validate nested parallel steps
	if step.Type == "parallel" {
		for _, nested := range step.Steps {
			warnings = append(warnings, validateStepTypes(&nested, def, payloadRegistry)...)
		}
	}

	return warnings
}

// validateFromReference validates a 'from' reference in an input declaration.
// It checks:
// 1. Reference format is valid (root.field.subfield...)
// 2. Root segment is valid (execution, trigger, steps)
// 3. For steps.* paths, the referenced step exists
// 4. For steps.* paths with outputs declared, the referenced output exists
func validateFromReference(fromRef, stepName, inputName string, def *wfschema.Definition) []string {
	var warnings []string

	// Empty reference is invalid
	if fromRef == "" {
		warnings = append(warnings, fmt.Sprintf("step %q input %q has empty 'from' reference", stepName, inputName))
		return warnings
	}

	// Split reference into parts
	parts := strings.Split(fromRef, ".")
	if len(parts) == 0 {
		warnings = append(warnings, fmt.Sprintf("step %q input %q has malformed 'from' reference: %q", stepName, inputName, fromRef))
		return warnings
	}

	// Validate root segment
	root := parts[0]
	validRoots := map[string]bool{
		"execution": true,
		"trigger":   true,
		"steps":     true,
	}

	// Handle shorthand step references (e.g., "fetch.result" instead of "steps.fetch.output.result")
	isShorthand := !validRoots[root] && len(parts) >= 2
	if isShorthand {
		// Treat as shorthand step reference
		root = "steps"
		// Expand: "fetch.result" -> ["steps", "fetch", "output", "result"]
		parts = append([]string{"steps", parts[0], "output"}, parts[1:]...)
	}

	if !validRoots[root] {
		warnings = append(warnings, fmt.Sprintf("step %q input %q has invalid 'from' root %q (valid: execution, trigger, steps, or step shorthand)", stepName, inputName, root))
		return warnings
	}

	// For steps.* paths, validate the step name exists and output is declared
	if root == "steps" {
		if len(parts) < 2 {
			warnings = append(warnings, fmt.Sprintf("step %q input %q has incomplete steps reference %q (requires step name)", stepName, inputName, fromRef))
			return warnings
		}

		refStepName := parts[1]
		refStep := findStepByName(refStepName, def)
		if refStep == nil {
			warnings = append(warnings, fmt.Sprintf("step %q input %q references non-existent step %q", stepName, inputName, refStepName))
			return warnings
		}

		// If the referenced step has outputs declared, validate the output field
		if len(refStep.Outputs) > 0 && len(parts) >= 4 {
			// parts[2] should be "output", parts[3] is the output field name
			// "steps.fetch.output.result" -> outputName = "result"
			// "steps.fetch.output.data.nested" -> outputName = "data"
			outputName := parts[3]
			if _, declared := refStep.Outputs[outputName]; !declared {
				declaredOutputs := make([]string, 0, len(refStep.Outputs))
				for name := range refStep.Outputs {
					declaredOutputs = append(declaredOutputs, name)
				}
				warnings = append(warnings, fmt.Sprintf(
					"step %q input %q references output %q from step %q, but step only declares: %v",
					stepName, inputName, outputName, refStepName, declaredOutputs))
			}
		}
	}

	return warnings
}

// stepExistsInWorkflow checks if a step with the given name exists in the workflow definition.
func stepExistsInWorkflow(stepName string, def *wfschema.Definition) bool {
	return findStepByName(stepName, def) != nil
}

// findStepByName returns the step definition for a given step name, or nil if not found.
// It searches both top-level steps and nested parallel steps.
func findStepByName(stepName string, def *wfschema.Definition) *wfschema.StepDef {
	for i := range def.Steps {
		step := &def.Steps[i]
		if step.Name == stepName {
			return step
		}
		// Check nested parallel steps
		if step.Type == "parallel" {
			for j := range step.Steps {
				nested := &step.Steps[j]
				if nested.Name == stepName {
					return nested
				}
			}
		}
	}
	return nil
}
