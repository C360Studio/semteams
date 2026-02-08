package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

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
		return fmt.Errorf("failed to list workflow keys: %w", err)
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

// Register adds or updates a workflow definition
func (r *Registry) Register(ctx context.Context, workflow *wfschema.Definition) error {
	if err := workflow.Validate(); err != nil {
		return fmt.Errorf("invalid workflow: %w", err)
	}

	data, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	if _, err := r.bucket.Put(ctx, workflow.ID, data); err != nil {
		return fmt.Errorf("failed to save workflow: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old trigger mapping if it exists
	if existing, ok := r.workflows[workflow.ID]; ok {
		delete(r.byTrigger, existing.Trigger.Subject)
	}

	r.workflows[workflow.ID] = workflow
	r.byTrigger[workflow.Trigger.Subject] = workflow.ID

	r.logger.Info("Registered workflow",
		slog.String("id", workflow.ID),
		slog.String("name", workflow.Name),
		slog.String("trigger", workflow.Trigger.Subject))

	return nil
}

// Unregister removes a workflow definition
func (r *Registry) Unregister(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	workflow, ok := r.workflows[id]
	if !ok {
		return fmt.Errorf("workflow not found: %s", id)
	}

	if err := r.bucket.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
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
		return fmt.Errorf("failed to create watcher: %w", err)
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
