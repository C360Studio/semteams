package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go/jetstream"
)

// CallbackHandler manages async callback processing for reactive workflows.
// It consumes callback messages, correlates them to pending executions,
// deserializes typed results, and triggers state mutations.
//
// Thread Safety: All task registration operations are protected by a single
// mutex to ensure atomicity. A callback cannot arrive between partial
// registration states.
type CallbackHandler struct {
	logger     *slog.Logger
	consumer   *SubjectConsumer
	dispatcher *Dispatcher
	store      StateStore

	// taskData holds all task-related data protected by a single mutex.
	// This ensures atomic registration/lookup/unregistration operations.
	taskData struct {
		sync.RWMutex
		// tasks maps taskID -> task registration for callback correlation
		tasks map[string]*TaskRegistration
		// rules maps taskID -> rule definition for callback processing
		rules map[string]*RuleDef
		// defs maps taskID -> workflow definition
		defs map[string]*Definition
	}

	// activeConsumers tracks which callback subjects we're consuming.
	activeConsumers map[string]bool
	consumerMu      sync.Mutex

	// shutdown signals goroutines to stop
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// TaskRegistration holds the information needed to process a callback.
type TaskRegistration struct {
	// TaskID is the unique task identifier.
	TaskID string

	// ExecutionKey is the KV key where execution state is stored.
	ExecutionKey string

	// ExecutionID is the execution identifier.
	ExecutionID string

	// WorkflowID is the workflow definition ID.
	WorkflowID string

	// RuleID is the rule that initiated the async action.
	RuleID string

	// ExpectedResultType is the message type for deserializing the result.
	// Format: "domain.category.version"
	ExpectedResultType string

	// RegisteredAt is when the task was registered.
	RegisteredAt time.Time

	// Timeout is when this task registration expires (optional).
	Timeout *time.Time
}

// NewCallbackHandler creates a new callback handler.
func NewCallbackHandler(
	logger *slog.Logger,
	consumer *SubjectConsumer,
	dispatcher *Dispatcher,
	store StateStore,
) *CallbackHandler {
	if logger == nil {
		logger = slog.Default()
	}

	h := &CallbackHandler{
		logger:          logger,
		consumer:        consumer,
		dispatcher:      dispatcher,
		store:           store,
		activeConsumers: make(map[string]bool),
		shutdown:        make(chan struct{}),
	}
	h.taskData.tasks = make(map[string]*TaskRegistration)
	h.taskData.rules = make(map[string]*RuleDef)
	h.taskData.defs = make(map[string]*Definition)
	return h
}

// RegisterTask registers a pending async task for callback correlation.
// Call this after dispatching an async action to enable callback processing.
// This operation is atomic - a callback cannot arrive during registration.
func (h *CallbackHandler) RegisterTask(reg *TaskRegistration, rule *RuleDef, def *Definition) {
	h.taskData.Lock()
	h.taskData.tasks[reg.TaskID] = reg
	h.taskData.rules[reg.TaskID] = rule
	h.taskData.defs[reg.TaskID] = def
	h.taskData.Unlock()

	h.logger.Debug("Registered task for callback",
		"task_id", reg.TaskID,
		"execution_key", reg.ExecutionKey,
		"rule_id", reg.RuleID,
		"expected_type", reg.ExpectedResultType)
}

// UnregisterTask removes a task registration (e.g., after callback received or timeout).
func (h *CallbackHandler) UnregisterTask(taskID string) {
	h.taskData.Lock()
	delete(h.taskData.tasks, taskID)
	delete(h.taskData.rules, taskID)
	delete(h.taskData.defs, taskID)
	h.taskData.Unlock()

	h.logger.Debug("Unregistered task", "task_id", taskID)
}

// GetTaskRegistration returns the registration for a task ID, or nil if not found.
func (h *CallbackHandler) GetTaskRegistration(taskID string) *TaskRegistration {
	h.taskData.RLock()
	defer h.taskData.RUnlock()
	return h.taskData.tasks[taskID]
}

// getTaskRuleAndDef returns the rule and definition for a task ID.
// Returns nil, nil if the task is not found.
func (h *CallbackHandler) getTaskRuleAndDef(taskID string) (*RuleDef, *Definition) {
	h.taskData.RLock()
	defer h.taskData.RUnlock()
	return h.taskData.rules[taskID], h.taskData.defs[taskID]
}

// StartCallbackConsumer starts consuming callbacks for a workflow.
// The subject pattern should match the callback subject used in async actions.
func (h *CallbackHandler) StartCallbackConsumer(
	ctx context.Context,
	js jetstream.JetStream,
	streamName string,
	subjectPattern string,
	consumerName string,
) error {
	h.consumerMu.Lock()
	if h.activeConsumers[subjectPattern] {
		h.consumerMu.Unlock()
		return nil // Already consuming
	}
	h.activeConsumers[subjectPattern] = true
	h.consumerMu.Unlock()

	return h.consumer.StartConsumer(ctx, js, streamName, subjectPattern, consumerName,
		func(ctx context.Context, event SubjectMessageEvent, msg jetstream.Msg) {
			h.handleCallbackMessage(ctx, event, msg)
		})
}

// StopCallbackConsumer stops consuming callbacks for a specific subject.
func (h *CallbackHandler) StopCallbackConsumer(streamName, consumerName string) {
	h.consumer.StopConsumer(streamName, consumerName)

	// Note: We don't track which subject maps to which consumer here,
	// so we can't remove from activeConsumers precisely.
	// In practice, Stop is called during shutdown.
}

// Stop shuts down the callback handler.
func (h *CallbackHandler) Stop() {
	h.shutdownOnce.Do(func() {
		close(h.shutdown)
	})

	h.consumer.StopAll()

	h.logger.Info("Callback handler stopped")
}

// handleCallbackMessage processes an incoming callback message.
func (h *CallbackHandler) handleCallbackMessage(
	ctx context.Context,
	event SubjectMessageEvent,
	msg jetstream.Msg,
) {
	// Parse the callback message to extract task ID
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(event.Data, &baseMsg); err != nil {
		h.logger.Error("Failed to unmarshal callback message",
			"subject", event.Subject,
			"error", err)
		// Nak to allow retry
		_ = msg.Nak()
		return
	}

	// Try to extract task ID from the payload
	taskID, err := h.extractTaskID(baseMsg.Payload())
	if err != nil {
		h.logger.Error("Failed to extract task ID from callback",
			"subject", event.Subject,
			"error", err)
		// Term the message - can't process without task ID
		_ = msg.Term()
		return
	}

	// Look up the task registration
	reg := h.GetTaskRegistration(taskID)
	if reg == nil {
		h.logger.Warn("Received callback for unknown task",
			"task_id", taskID,
			"subject", event.Subject)
		// Ack anyway - the task may have been processed already or timed out
		_ = msg.Ack()
		return
	}

	// Get the rule and definition (atomically with reg lookup)
	rule, def := h.getTaskRuleAndDef(taskID)
	if rule == nil || def == nil {
		h.logger.Error("Missing rule or definition for callback",
			"task_id", taskID)
		_ = msg.Term()
		h.UnregisterTask(taskID)
		return
	}

	// Load the current execution state from KV
	entry, err := h.store.Get(ctx, reg.ExecutionKey)
	if err != nil {
		h.logger.Error("Failed to load execution state for callback",
			"task_id", taskID,
			"execution_key", reg.ExecutionKey,
			"error", err)
		// Nak to retry - state might be temporarily unavailable
		_ = msg.Nak()
		return
	}

	// Deserialize the state
	state := def.StateFactory()
	if err := json.Unmarshal(entry.Value(), state); err != nil {
		h.logger.Error("Failed to unmarshal execution state",
			"task_id", taskID,
			"execution_key", reg.ExecutionKey,
			"error", err)
		_ = msg.Term()
		h.UnregisterTask(taskID)
		return
	}

	// Build the rule context
	ruleCtx := &RuleContext{
		State:      state,
		Message:    baseMsg.Payload(),
		KVRevision: entry.Revision(),
		Subject:    event.Subject,
		KVKey:      reg.ExecutionKey,
	}

	// Check if this is a failure result
	if asyncResult, ok := baseMsg.Payload().(*AsyncStepResult); ok {
		if asyncResult.Status == "failed" {
			h.handleFailedCallback(ctx, ruleCtx, rule, def, asyncResult, msg, taskID)
			return
		}
	}

	// Process the successful callback via dispatcher
	result, err := h.dispatcher.HandleCallback(ctx, ruleCtx, rule, baseMsg.Payload(), def)
	if err != nil {
		h.logger.Error("Failed to process callback",
			"task_id", taskID,
			"error", err)
		// Don't retry callback processing failures - they're likely permanent
		_ = msg.Term()
		h.UnregisterTask(taskID)
		return
	}

	// Log appropriately based on result
	if result.PartialFailure {
		h.logger.Warn("Callback processed with partial failure",
			"task_id", taskID,
			"execution_key", reg.ExecutionKey,
			"new_revision", result.NewRevision,
			"partial_error", result.PartialError)
	} else {
		h.logger.Info("Callback processed successfully",
			"task_id", taskID,
			"execution_key", reg.ExecutionKey,
			"new_revision", result.NewRevision)
	}

	// Acknowledge the message
	_ = msg.Ack()

	// Unregister the task
	h.UnregisterTask(taskID)
}

// handleFailedCallback processes a callback that indicates the async operation failed.
// Note: rule is reserved for future rule-specific error handling.
func (h *CallbackHandler) handleFailedCallback(
	ctx context.Context,
	ruleCtx *RuleContext,
	_ *RuleDef,
	def *Definition,
	asyncResult *AsyncStepResult,
	msg jetstream.Msg,
	taskID string,
) {
	errMsg := asyncResult.Error
	if errMsg == "" {
		errMsg = "async operation failed"
	}

	// Use the dispatcher to handle the failure
	_, err := h.dispatcher.HandleFailure(ctx, ruleCtx, errMsg, def)
	if err != nil {
		h.logger.Error("Failed to record async failure",
			"task_id", taskID,
			"error", err)
	}

	_ = msg.Ack()
	h.UnregisterTask(taskID)
}

// extractTaskID extracts the task ID from a callback payload.
// It tries multiple approaches to find the task ID.
func (h *CallbackHandler) extractTaskID(payload message.Payload) (string, error) {
	// Try direct type assertion to AsyncStepResult
	if result, ok := payload.(*AsyncStepResult); ok {
		if result.TaskID != "" {
			return result.TaskID, nil
		}
	}

	// Try the TaskIDExtractor interface
	if extractor, ok := payload.(TaskIDExtractor); ok {
		if taskID := extractor.GetTaskID(); taskID != "" {
			return taskID, nil
		}
	}

	// Try to find task_id in the JSON representation
	data, err := payload.MarshalJSON()
	if err != nil {
		return "", &CallbackError{
			TaskID:  "",
			Message: "failed to marshal payload for task ID extraction",
			Cause:   err,
		}
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return "", &CallbackError{
			TaskID:  "",
			Message: "failed to unmarshal payload fields",
			Cause:   err,
		}
	}

	// Look for common task ID field names
	for _, fieldName := range []string{"task_id", "taskId", "TaskID", "id"} {
		if val, ok := fields[fieldName]; ok {
			if strVal, ok := val.(string); ok && strVal != "" {
				return strVal, nil
			}
		}
	}

	return "", &CallbackError{
		TaskID:  "",
		Message: "no task ID found in callback payload",
	}
}

// TaskIDExtractor is implemented by payloads that provide a task ID.
type TaskIDExtractor interface {
	GetTaskID() string
}

// CleanupExpiredTasks removes task registrations that have exceeded their timeout.
// Call this periodically to prevent memory leaks from orphaned tasks.
func (h *CallbackHandler) CleanupExpiredTasks() int {
	now := time.Now()
	var expired []string

	h.taskData.RLock()
	for taskID, reg := range h.taskData.tasks {
		if reg.Timeout != nil && now.After(*reg.Timeout) {
			expired = append(expired, taskID)
		}
	}
	h.taskData.RUnlock()

	for _, taskID := range expired {
		h.UnregisterTask(taskID)
		h.logger.Warn("Cleaned up expired task registration", "task_id", taskID)
	}

	return len(expired)
}

// PendingTaskCount returns the number of pending task registrations.
func (h *CallbackHandler) PendingTaskCount() int {
	h.taskData.RLock()
	defer h.taskData.RUnlock()
	return len(h.taskData.tasks)
}

// CallbackError represents an error during callback processing.
type CallbackError struct {
	TaskID  string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *CallbackError) Error() string {
	if e.TaskID != "" {
		return "callback error for task " + e.TaskID + ": " + e.Message
	}
	return "callback error: " + e.Message
}

// Unwrap returns the underlying error.
func (e *CallbackError) Unwrap() error {
	return e.Cause
}

// BuildCallbackSubjectPattern creates a wildcard subject pattern for callbacks.
// This is used to subscribe to all callbacks for a workflow.
func BuildCallbackSubjectPattern(workflowID string) string {
	return "workflow.callback." + workflowID + ".>"
}

// ParseCallbackSubject extracts the workflow ID and execution ID from a callback subject.
// Subject format: workflow.callback.{workflowID}.{executionID}
func ParseCallbackSubject(subject string) (workflowID, executionID string, err error) {
	parts := strings.Split(subject, ".")
	if len(parts) < 4 {
		return "", "", &CallbackError{
			Message: "invalid callback subject format: " + subject,
		}
	}

	if parts[0] != "workflow" || parts[1] != "callback" {
		return "", "", &CallbackError{
			Message: "subject does not match callback pattern: " + subject,
		}
	}

	return parts[2], parts[3], nil
}

// CallbackConsumerConfig holds configuration for callback consumers.
type CallbackConsumerConfig struct {
	// StreamName is the JetStream stream to consume from.
	StreamName string

	// ConsumerName is the durable consumer name.
	ConsumerName string

	// SubjectPattern is the NATS subject pattern for callbacks.
	SubjectPattern string
}

// DefaultCallbackConsumerConfig returns default configuration for a workflow.
func DefaultCallbackConsumerConfig(workflowID string) CallbackConsumerConfig {
	return CallbackConsumerConfig{
		StreamName:     "WORKFLOW_CALLBACKS",
		ConsumerName:   "callback-" + workflowID,
		SubjectPattern: BuildCallbackSubjectPattern(workflowID),
	}
}

// CallbackMetrics provides metrics about callback processing.
type CallbackMetrics struct {
	// PendingTasks is the current number of pending task registrations.
	PendingTasks int

	// ProcessedCount is the total number of callbacks processed.
	ProcessedCount int64

	// FailedCount is the total number of failed callbacks.
	FailedCount int64

	// ExpiredCount is the total number of expired task registrations.
	ExpiredCount int64
}

// GetMetrics returns current callback metrics.
// Note: Processed/Failed/Expired counts require adding tracking to the handler.
func (h *CallbackHandler) GetMetrics() CallbackMetrics {
	return CallbackMetrics{
		PendingTasks: h.PendingTaskCount(),
	}
}
