package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ExecutionStore manages workflow execution state persistence in NATS KV.
// It provides typed access to execution state with optimistic concurrency control.
type ExecutionStore struct {
	logger *slog.Logger
	bucket jetstream.KeyValue

	// stateFactory creates typed state instances for unmarshaling.
	// Each workflow has its own state type that embeds ExecutionState.
	stateFactory func() any

	// taskIndex maps taskID -> executionKey for callback correlation.
	// This is a secondary index maintained in memory.
	taskIndex   map[string]string
	taskIndexMu sync.RWMutex

	// keyPrefix is prepended to all execution keys (e.g., "plan-review.")
	keyPrefix string
}

// ExecutionStoreOption configures an ExecutionStore.
type ExecutionStoreOption func(*ExecutionStore)

// WithKeyPrefix sets the key prefix for execution keys.
func WithKeyPrefix(prefix string) ExecutionStoreOption {
	return func(s *ExecutionStore) {
		s.keyPrefix = prefix
	}
}

// WithStoreStateFactory sets the state factory for the store.
func WithStoreStateFactory(factory func() any) ExecutionStoreOption {
	return func(s *ExecutionStore) {
		s.stateFactory = factory
	}
}

// NewExecutionStore creates a new execution store.
func NewExecutionStore(
	logger *slog.Logger,
	bucket jetstream.KeyValue,
	opts ...ExecutionStoreOption,
) *ExecutionStore {
	if logger == nil {
		logger = slog.Default()
	}

	s := &ExecutionStore{
		logger:    logger,
		bucket:    bucket,
		taskIndex: make(map[string]string),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ExecutionEntry represents a loaded execution with its KV metadata.
type ExecutionEntry struct {
	// State is the typed execution state.
	State any

	// Key is the KV key.
	Key string

	// Revision is the KV revision for optimistic concurrency.
	Revision uint64

	// Created is when the KV entry was created.
	Created time.Time
}

// CreateExecution initializes and stores a new workflow execution.
// Returns the created entry with its initial revision.
func (s *ExecutionStore) CreateExecution(
	ctx context.Context,
	executionID string,
	workflowID string,
	timeout time.Duration,
) (*ExecutionEntry, error) {
	if s.stateFactory == nil {
		return nil, &StoreError{
			Op:      "create",
			Key:     executionID,
			Message: "no state factory configured",
		}
	}

	// Create typed state instance
	state := s.stateFactory()

	// Initialize the execution state
	InitializeExecution(state, executionID, workflowID, timeout)

	// Build the key
	key := s.buildKey(executionID)

	// Serialize to JSON
	data, err := json.Marshal(state)
	if err != nil {
		return nil, &StoreError{
			Op:      "create",
			Key:     key,
			Message: "failed to marshal state",
			Cause:   err,
		}
	}

	// Create the entry (fails if key already exists)
	rev, err := s.bucket.Create(ctx, key, data)
	if err != nil {
		if isKeyExistsError(err) {
			return nil, &StoreError{
				Op:      "create",
				Key:     key,
				Message: "execution already exists",
				Cause:   err,
			}
		}
		return nil, &StoreError{
			Op:      "create",
			Key:     key,
			Message: "failed to create entry",
			Cause:   err,
		}
	}

	s.logger.Info("Created execution",
		"execution_id", executionID,
		"workflow_id", workflowID,
		"key", key,
		"revision", rev)

	return &ExecutionEntry{
		State:    state,
		Key:      key,
		Revision: rev,
		Created:  time.Now(),
	}, nil
}

// LoadExecution loads an execution by ID.
// Returns nil, nil if the execution doesn't exist.
func (s *ExecutionStore) LoadExecution(ctx context.Context, executionID string) (*ExecutionEntry, error) {
	key := s.buildKey(executionID)
	return s.LoadExecutionByKey(ctx, key)
}

// LoadExecutionByKey loads an execution by its full KV key.
func (s *ExecutionStore) LoadExecutionByKey(ctx context.Context, key string) (*ExecutionEntry, error) {
	entry, err := s.bucket.Get(ctx, key)
	if err != nil {
		if isKeyNotFoundError(err) {
			return nil, nil // Not found is not an error
		}
		return nil, &StoreError{
			Op:      "load",
			Key:     key,
			Message: "failed to get entry",
			Cause:   err,
		}
	}

	if s.stateFactory == nil {
		return nil, &StoreError{
			Op:      "load",
			Key:     key,
			Message: "no state factory configured",
		}
	}

	// Deserialize into typed state
	state := s.stateFactory()
	if err := json.Unmarshal(entry.Value(), state); err != nil {
		return nil, &StoreError{
			Op:      "load",
			Key:     key,
			Message: "failed to unmarshal state",
			Cause:   err,
		}
	}

	return &ExecutionEntry{
		State:    state,
		Key:      key,
		Revision: entry.Revision(),
		Created:  entry.Created(),
	}, nil
}

// SaveExecution saves the execution state with optimistic concurrency.
// The revision must match the current KV revision.
func (s *ExecutionStore) SaveExecution(
	ctx context.Context,
	key string,
	state any,
	revision uint64,
) (uint64, error) {
	// Serialize to JSON
	data, err := json.Marshal(state)
	if err != nil {
		return 0, &StoreError{
			Op:      "save",
			Key:     key,
			Message: "failed to marshal state",
			Cause:   err,
		}
	}

	// Update with optimistic concurrency
	newRev, err := s.bucket.Update(ctx, key, data, revision)
	if err != nil {
		if isRevisionMismatchError(err) {
			return 0, &StoreError{
				Op:      "save",
				Key:     key,
				Message: "revision mismatch - concurrent modification",
				Cause:   err,
			}
		}
		return 0, &StoreError{
			Op:      "save",
			Key:     key,
			Message: "failed to update entry",
			Cause:   err,
		}
	}

	s.logger.Debug("Saved execution",
		"key", key,
		"old_revision", revision,
		"new_revision", newRev)

	return newRev, nil
}

// DeleteExecution removes an execution from the store.
func (s *ExecutionStore) DeleteExecution(ctx context.Context, executionID string) error {
	key := s.buildKey(executionID)

	if err := s.bucket.Delete(ctx, key); err != nil {
		if isKeyNotFoundError(err) {
			return nil // Already deleted
		}
		return &StoreError{
			Op:      "delete",
			Key:     key,
			Message: "failed to delete entry",
			Cause:   err,
		}
	}

	// Clean up task index
	s.removeFromTaskIndex(executionID)

	s.logger.Info("Deleted execution", "execution_id", executionID, "key", key)
	return nil
}

// ListExecutions returns all executions matching the filter.
// If filter is nil, returns all executions.
func (s *ExecutionStore) ListExecutions(
	ctx context.Context,
	filter *ExecutionFilter,
) ([]*ExecutionEntry, error) {
	keys, err := s.bucket.Keys(ctx)
	if err != nil {
		// ErrNoKeysFound is not an error - just means empty bucket
		if err == jetstream.ErrNoKeysFound {
			return nil, nil
		}
		return nil, &StoreError{
			Op:      "list",
			Message: "failed to list keys",
			Cause:   err,
		}
	}

	var entries []*ExecutionEntry
	for _, key := range keys {
		// Filter by prefix
		if s.keyPrefix != "" && !strings.HasPrefix(key, s.keyPrefix) {
			continue
		}

		entry, err := s.LoadExecutionByKey(ctx, key)
		if err != nil {
			s.logger.Warn("Failed to load execution during list",
				"key", key,
				"error", err)
			continue
		}
		if entry == nil {
			continue
		}

		// Apply filter
		if filter != nil && !s.matchesFilter(entry, filter) {
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// ExecutionFilter specifies criteria for filtering executions.
type ExecutionFilter struct {
	// Status filters by execution status.
	Status *ExecutionStatus

	// WorkflowID filters by workflow ID.
	WorkflowID string

	// Phase filters by current phase.
	Phase string

	// ActiveOnly excludes terminal states (completed, failed, escalated, timed_out).
	ActiveOnly bool

	// ExpiredOnly includes only executions past their deadline.
	ExpiredOnly bool
}

// matchesFilter checks if an entry matches the filter criteria.
func (s *ExecutionStore) matchesFilter(entry *ExecutionEntry, filter *ExecutionFilter) bool {
	es := ExtractExecutionState(entry.State)
	if es == nil {
		return false
	}

	if filter.Status != nil && es.Status != *filter.Status {
		return false
	}

	if filter.WorkflowID != "" && es.WorkflowID != filter.WorkflowID {
		return false
	}

	if filter.Phase != "" && es.Phase != filter.Phase {
		return false
	}

	if filter.ActiveOnly && IsTerminalStatus(es.Status) {
		return false
	}

	if filter.ExpiredOnly {
		if es.Deadline == nil || !time.Now().After(*es.Deadline) {
			return false
		}
	}

	return true
}

// RegisterTaskIndex registers a task ID -> execution key mapping.
func (s *ExecutionStore) RegisterTaskIndex(taskID, executionKey string) {
	s.taskIndexMu.Lock()
	s.taskIndex[taskID] = executionKey
	s.taskIndexMu.Unlock()

	s.logger.Debug("Registered task index",
		"task_id", taskID,
		"execution_key", executionKey)
}

// LookupExecutionByTaskID finds an execution key by task ID.
func (s *ExecutionStore) LookupExecutionByTaskID(taskID string) (string, bool) {
	s.taskIndexMu.RLock()
	defer s.taskIndexMu.RUnlock()
	key, ok := s.taskIndex[taskID]
	return key, ok
}

// UnregisterTaskIndex removes a task ID from the index.
func (s *ExecutionStore) UnregisterTaskIndex(taskID string) {
	s.taskIndexMu.Lock()
	delete(s.taskIndex, taskID)
	s.taskIndexMu.Unlock()
}

// removeFromTaskIndex removes all task mappings for an execution by execution ID.
func (s *ExecutionStore) removeFromTaskIndex(executionID string) {
	key := s.buildKey(executionID)
	s.removeFromTaskIndexByKey(key)
}

// removeFromTaskIndexByKey removes all task mappings for an execution by full KV key.
func (s *ExecutionStore) removeFromTaskIndexByKey(key string) {
	s.taskIndexMu.Lock()
	defer s.taskIndexMu.Unlock()

	// Find and remove any tasks mapping to this execution
	for taskID, execKey := range s.taskIndex {
		if execKey == key {
			delete(s.taskIndex, taskID)
		}
	}
}

// CheckTimeout checks if an execution has exceeded its deadline.
// If expired, it updates the state to StatusTimedOut.
// Returns true if the execution was timed out.
func (s *ExecutionStore) CheckTimeout(ctx context.Context, entry *ExecutionEntry) (bool, error) {
	es := ExtractExecutionState(entry.State)
	if es == nil {
		return false, nil
	}

	// Already in terminal state
	if IsTerminalStatus(es.Status) {
		return false, nil
	}

	// Check deadline
	if es.Deadline == nil || !time.Now().After(*es.Deadline) {
		return false, nil
	}

	// Mark as timed out
	TimeoutExecution(entry.State)

	// Save the updated state
	newRev, err := s.SaveExecution(ctx, entry.Key, entry.State, entry.Revision)
	if err != nil {
		return false, err
	}

	entry.Revision = newRev

	s.logger.Warn("Execution timed out",
		"execution_id", es.ID,
		"key", entry.Key)

	return true, nil
}

// CheckIterationLimit checks if an execution has exceeded the max iterations.
// If exceeded, it updates the state to StatusEscalated.
// Returns true if the execution was escalated.
func (s *ExecutionStore) CheckIterationLimit(
	ctx context.Context,
	entry *ExecutionEntry,
	maxIterations int,
) (bool, error) {
	if maxIterations <= 0 {
		return false, nil // No limit
	}

	es := ExtractExecutionState(entry.State)
	if es == nil {
		return false, nil
	}

	// Already in terminal state
	if IsTerminalStatus(es.Status) {
		return false, nil
	}

	// Check iteration count
	if es.Iteration < maxIterations {
		return false, nil
	}

	// Escalate
	EscalateExecution(entry.State, "max iterations exceeded")

	// Save the updated state
	newRev, err := s.SaveExecution(ctx, entry.Key, entry.State, entry.Revision)
	if err != nil {
		return false, err
	}

	entry.Revision = newRev

	s.logger.Warn("Execution escalated due to iteration limit",
		"execution_id", es.ID,
		"key", entry.Key,
		"iteration", es.Iteration,
		"max", maxIterations)

	return true, nil
}

// CleanupCompletedExecutions deletes executions that have been in a terminal
// state for longer than the specified retention period.
func (s *ExecutionStore) CleanupCompletedExecutions(
	ctx context.Context,
	retention time.Duration,
) (int, error) {
	filter := &ExecutionFilter{ActiveOnly: false}
	entries, err := s.ListExecutions(ctx, filter)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-retention)
	var cleaned int

	for _, entry := range entries {
		es := ExtractExecutionState(entry.State)
		if es == nil {
			continue
		}

		// Only clean up terminal states
		if !IsTerminalStatus(es.Status) {
			continue
		}

		// Check if completed before cutoff
		if es.CompletedAt == nil || es.CompletedAt.After(cutoff) {
			continue
		}

		// Delete the execution
		if err := s.bucket.Delete(ctx, entry.Key); err != nil {
			if !isKeyNotFoundError(err) {
				s.logger.Warn("Failed to cleanup execution",
					"key", entry.Key,
					"error", err)
			}
			continue
		}

		// Clean up task index for this execution
		s.removeFromTaskIndexByKey(entry.Key)

		cleaned++
		s.logger.Debug("Cleaned up completed execution",
			"key", entry.Key,
			"status", es.Status,
			"completed_at", es.CompletedAt)
	}

	if cleaned > 0 {
		s.logger.Info("Cleaned up completed executions",
			"count", cleaned,
			"retention", retention)
	}

	return cleaned, nil
}

// WatchExecutions starts watching for execution state changes.
// The handler is called for each state change.
func (s *ExecutionStore) WatchExecutions(
	ctx context.Context,
	handler func(ctx context.Context, entry *ExecutionEntry, op KVOperation),
) error {
	pattern := s.keyPrefix + "*"
	if s.keyPrefix == "" {
		pattern = ">"
	}

	watcher, err := s.bucket.Watch(ctx, pattern)
	if err != nil {
		return &StoreError{
			Op:      "watch",
			Message: "failed to start watcher",
			Cause:   err,
		}
	}

	go func() {
		defer func() {
			if err := watcher.Stop(); err != nil {
				s.logger.Warn("Error stopping watcher", "error", err)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case kvEntry, ok := <-watcher.Updates():
				if !ok {
					return
				}
				if kvEntry == nil {
					continue // Initial state complete
				}

				op := KVOperationPut
				if kvEntry.Operation() == jetstream.KeyValueDelete {
					op = KVOperationDelete
				}

				var entry *ExecutionEntry
				if op == KVOperationPut && s.stateFactory != nil {
					state := s.stateFactory()
					if err := json.Unmarshal(kvEntry.Value(), state); err != nil {
						s.logger.Warn("Failed to unmarshal watched entry",
							"key", kvEntry.Key(),
							"error", err)
						continue // Skip malformed entries
					}
					entry = &ExecutionEntry{
						State:    state,
						Key:      kvEntry.Key(),
						Revision: kvEntry.Revision(),
						Created:  kvEntry.Created(),
					}
				} else {
					entry = &ExecutionEntry{
						Key:      kvEntry.Key(),
						Revision: kvEntry.Revision(),
						Created:  kvEntry.Created(),
					}
				}

				handler(ctx, entry, op)
			}
		}
	}()

	return nil
}

// buildKey constructs the full KV key for an execution ID.
func (s *ExecutionStore) buildKey(executionID string) string {
	if s.keyPrefix == "" {
		return executionID
	}
	return s.keyPrefix + executionID
}

// StoreError represents an error from the execution store.
type StoreError struct {
	Op      string
	Key     string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *StoreError) Error() string {
	if e.Key != "" {
		return "store " + e.Op + " " + e.Key + ": " + e.Message
	}
	return "store " + e.Op + ": " + e.Message
}

// Unwrap returns the underlying error.
func (e *StoreError) Unwrap() error {
	return e.Cause
}

// IsNotFound returns true if the error indicates the key was not found.
func (e *StoreError) IsNotFound() bool {
	return e.Cause != nil && isKeyNotFoundError(e.Cause)
}

// IsConflict returns true if the error indicates a revision conflict.
func (e *StoreError) IsConflict() bool {
	return e.Cause != nil && isRevisionMismatchError(e.Cause)
}

// Helper functions for error detection

func isKeyNotFoundError(err error) bool {
	return err == jetstream.ErrKeyNotFound
}

func isKeyExistsError(err error) bool {
	// JetStream returns ErrKeyExists when Create is called on existing key
	return err == jetstream.ErrKeyExists
}

func isRevisionMismatchError(err error) bool {
	if err == nil {
		return false
	}
	// JetStream returns ErrKeyExists when Update revision doesn't match
	if err == jetstream.ErrKeyExists {
		return true
	}
	// Fallback: check error message for revision mismatch indication.
	// Note: This is version-dependent and may need updates with NATS library changes.
	errMsg := err.Error()
	return strings.Contains(errMsg, "wrong last sequence")
}
