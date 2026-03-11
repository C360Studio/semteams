package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// Publisher handles publishing messages to NATS subjects.
type Publisher interface {
	// Publish sends a message to a NATS subject.
	Publish(ctx context.Context, subject string, data []byte) error
}

// StateStore provides read/write access to execution state in KV.
type StateStore interface {
	// Get retrieves an entry by key.
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)

	// Put stores a value and returns the new revision.
	Put(ctx context.Context, key string, value []byte) (uint64, error)

	// Update performs a CAS update with explicit revision.
	// Returns ErrKVRevisionMismatch on conflict.
	Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error)
}

// Dispatcher executes actions for reactive workflow rules.
// It handles NATS publishing, KV state mutations, and async callback tracking.
type Dispatcher struct {
	logger    *slog.Logger
	publisher Publisher
	store     StateStore

	// kvWatcher is optional - used to mark own revisions to prevent feedback loops
	kvWatcher *KVWatcher

	// source identifies this dispatcher in published messages
	source string
}

// DispatcherOption configures a Dispatcher.
type DispatcherOption func(*Dispatcher)

// WithPublisher sets the NATS publisher for the dispatcher.
func WithPublisher(p Publisher) DispatcherOption {
	return func(d *Dispatcher) {
		d.publisher = p
	}
}

// WithStateStore sets the KV state store for the dispatcher.
func WithStateStore(s StateStore) DispatcherOption {
	return func(d *Dispatcher) {
		d.store = s
	}
}

// WithKVWatcher sets the KV watcher for feedback loop prevention.
func WithKVWatcher(w *KVWatcher) DispatcherOption {
	return func(d *Dispatcher) {
		d.kvWatcher = w
	}
}

// WithSource sets the source identifier for published messages.
func WithSource(source string) DispatcherOption {
	return func(d *Dispatcher) {
		d.source = source
	}
}

// NewDispatcher creates a new action dispatcher.
func NewDispatcher(logger *slog.Logger, opts ...DispatcherOption) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}

	d := &Dispatcher{
		logger: logger,
		source: "reactive-workflow",
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// DispatchResult contains the result of dispatching an action.
type DispatchResult struct {
	// TaskID is set for async actions to correlate callbacks.
	TaskID string

	// NewRevision is the KV revision after state mutation.
	NewRevision uint64

	// Published indicates if a message was published.
	Published bool

	// StateUpdated indicates if state was written to KV.
	StateUpdated bool

	// PartialFailure indicates the action partially succeeded but had errors.
	// For example, publish succeeded but state write failed.
	PartialFailure bool

	// PartialError contains the error message for partial failures.
	PartialError string
}

// DispatchAction executes the action defined in a rule.
// The ctx contains both the triggering data and the current state.
// Returns the dispatch result and any error that occurred.
func (d *Dispatcher) DispatchAction(
	ctx context.Context,
	ruleCtx *RuleContext,
	rule *RuleDef,
	def *Definition,
) (*DispatchResult, error) {
	action := &rule.Action

	switch action.Type {
	case ActionPublishAsync:
		return d.dispatchPublishAsync(ctx, ruleCtx, action, def)
	case ActionPublish:
		return d.dispatchPublish(ctx, ruleCtx, action, def)
	case ActionMutate:
		return d.dispatchMutate(ctx, ruleCtx, action, def)
	case ActionComplete:
		return d.dispatchComplete(ctx, ruleCtx, action, def)
	default:
		return nil, &DispatchError{
			Action:  action.Type.String(),
			Message: "unknown action type",
		}
	}
}

// dispatchPublishAsync publishes a message and parks the execution waiting for callback.
func (d *Dispatcher) dispatchPublishAsync(
	ctx context.Context,
	ruleCtx *RuleContext,
	action *Action,
	def *Definition,
) (*DispatchResult, error) {
	if d.publisher == nil {
		return nil, &DispatchError{
			Action:  "publish_async",
			Message: "no publisher configured",
		}
	}

	// Generate task ID for callback correlation
	taskID := uuid.New().String()

	// Build the payload
	payload, err := action.BuildPayload(ruleCtx)
	if err != nil {
		return nil, &DispatchError{
			Action:  "publish_async",
			Message: "failed to build payload: " + err.Error(),
		}
	}

	// Get execution ID from state
	execID := GetID(ruleCtx.State)

	// Inject callback fields into the payload if it supports it
	if injectable, ok := payload.(CallbackInjectable); ok {
		injectable.InjectCallback(CallbackFields{
			TaskID:          taskID,
			CallbackSubject: buildCallbackSubject(def.ID, execID),
			ExecutionID:     execID,
		})
	}

	// Build message type from payload
	msgType := payload.Schema()

	// Create BaseMessage wrapper
	baseMsg := message.NewBaseMessage(msgType, payload, d.source)

	// Marshal to JSON
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return nil, &DispatchError{
			Action:  "publish_async",
			Message: "failed to marshal message: " + err.Error(),
		}
	}

	// Publish to NATS
	if err := d.publisher.Publish(ctx, action.PublishSubject, data); err != nil {
		return nil, &DispatchError{
			Action:  "publish_async",
			Subject: action.PublishSubject,
			Message: "failed to publish: " + err.Error(),
		}
	}

	d.logger.Debug("Published async action",
		"subject", action.PublishSubject,
		"task_id", taskID,
		"execution_id", execID)

	// Update state to mark as waiting for callback
	result := &DispatchResult{
		TaskID:    taskID,
		Published: true,
	}

	// Park the execution - set pending task and status to waiting
	if ruleCtx.State != nil {
		SetPendingTask(ruleCtx.State, taskID, action.ExpectedResultType)
		SetStatus(ruleCtx.State, StatusWaiting)

		// Write state to KV
		newRev, err := d.writeState(ctx, ruleCtx, def)
		if err != nil {
			// Mark as partial failure - message was published but state write failed
			// The caller should handle this case (e.g., retry state write, alert)
			d.logger.Error("Failed to update state after async publish",
				"error", err,
				"task_id", taskID)
			result.PartialFailure = true
			result.PartialError = "state write failed after publish: " + err.Error()
		} else {
			result.NewRevision = newRev
			result.StateUpdated = true
		}
	}

	return result, nil
}

// dispatchPublish writes state first (if mutator configured), then publishes message.
// State is written BEFORE publish to prevent race conditions where downstream components
// process the message and update state before the engine can claim the correct phase.
func (d *Dispatcher) dispatchPublish(
	ctx context.Context,
	ruleCtx *RuleContext,
	action *Action,
	def *Definition,
) (*DispatchResult, error) {
	if d.publisher == nil {
		return nil, &DispatchError{
			Action:  "publish",
			Message: "no publisher configured",
		}
	}

	result := &DispatchResult{}

	// CRITICAL: Write state BEFORE publishing to prevent race conditions.
	// If we publish first, downstream components may process the message and update
	// state before the engine can write its state mutation. This causes "wrong last
	// sequence" errors when the engine tries to update with a stale revision.
	if action.MutateState != nil && ruleCtx.State != nil {
		if err := action.MutateState(ruleCtx, nil); err != nil {
			return nil, &DispatchError{
				Action:  "publish",
				Message: "state mutation failed: " + err.Error(),
			}
		}

		newRev, err := d.writeState(ctx, ruleCtx, def)
		if err != nil {
			return nil, &DispatchError{
				Action:  "publish",
				Message: "failed to write state: " + err.Error(),
			}
		}
		result.NewRevision = newRev
		result.StateUpdated = true
	}

	// Build the payload (after state mutation so payload reflects updated state)
	payload, err := action.BuildPayload(ruleCtx)
	if err != nil {
		return nil, &DispatchError{
			Action:  "publish",
			Message: "failed to build payload: " + err.Error(),
		}
	}

	// Build message type from payload
	msgType := payload.Schema()

	// Create BaseMessage wrapper
	baseMsg := message.NewBaseMessage(msgType, payload, d.source)

	// Marshal to JSON
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return nil, &DispatchError{
			Action:  "publish",
			Message: "failed to marshal message: " + err.Error(),
		}
	}

	// Publish to NATS (state already claimed the correct phase)
	if err := d.publisher.Publish(ctx, action.PublishSubject, data); err != nil {
		return nil, &DispatchError{
			Action:  "publish",
			Subject: action.PublishSubject,
			Message: "failed to publish: " + err.Error(),
		}
	}

	d.logger.Debug("Published action",
		"subject", action.PublishSubject,
		"execution_id", GetID(ruleCtx.State))

	result.Published = true
	return result, nil
}

// dispatchMutate updates KV state without publishing.
func (d *Dispatcher) dispatchMutate(
	ctx context.Context,
	ruleCtx *RuleContext,
	action *Action,
	def *Definition,
) (*DispatchResult, error) {
	if ruleCtx.State == nil {
		return nil, &DispatchError{
			Action:  "mutate",
			Message: "no state available to mutate",
		}
	}

	// Defensive nil check - MutateState is required for ActionMutate
	// but we check here in case validation was bypassed
	if action.MutateState == nil {
		return nil, &DispatchError{
			Action:  "mutate",
			Message: "no MutateState function configured",
		}
	}

	// Apply the state mutation
	if err := action.MutateState(ruleCtx, nil); err != nil {
		return nil, &DispatchError{
			Action:  "mutate",
			Message: "state mutation failed: " + err.Error(),
			Cause:   err,
		}
	}

	// Write state to KV
	newRev, err := d.writeState(ctx, ruleCtx, def)
	if err != nil {
		return nil, &DispatchError{
			Action:  "mutate",
			Message: "failed to write state: " + err.Error(),
		}
	}

	d.logger.Debug("State mutated",
		"key", ruleCtx.KVKey,
		"revision", newRev,
		"execution_id", GetID(ruleCtx.State))

	return &DispatchResult{
		NewRevision:  newRev,
		StateUpdated: true,
	}, nil
}

// dispatchComplete marks the execution as completed and publishes terminal events.
func (d *Dispatcher) dispatchComplete(
	ctx context.Context,
	ruleCtx *RuleContext,
	action *Action,
	def *Definition,
) (*DispatchResult, error) {
	result := &DispatchResult{}

	// Apply optional state mutation first
	if action.MutateState != nil && ruleCtx.State != nil {
		if err := action.MutateState(ruleCtx, nil); err != nil {
			return nil, &DispatchError{
				Action:  "complete",
				Message: "state mutation failed: " + err.Error(),
			}
		}
	}

	// Mark execution as completed
	if ruleCtx.State != nil {
		CompleteExecution(ruleCtx.State, GetPhase(ruleCtx.State))

		// Write final state to KV
		newRev, err := d.writeState(ctx, ruleCtx, def)
		if err != nil {
			return nil, &DispatchError{
				Action:  "complete",
				Message: "failed to write final state: " + err.Error(),
			}
		}
		result.NewRevision = newRev
		result.StateUpdated = true
	}

	// Publish rule-specific completion event (from CompleteWithEvent builder)
	if action.PublishSubject != "" && action.BuildPayload != nil && d.publisher != nil {
		payload, err := action.BuildPayload(ruleCtx)
		if err != nil {
			return nil, &DispatchError{
				Action:  "complete",
				Message: "failed to build completion event payload: " + err.Error(),
			}
		}

		// Build message with proper wrapping (same pattern as dispatchPublish)
		msgType := payload.Schema()
		baseMsg := message.NewBaseMessage(msgType, payload, d.source)
		data, err := json.Marshal(baseMsg)
		if err != nil {
			return nil, &DispatchError{
				Action:  "complete",
				Message: "failed to marshal completion event: " + err.Error(),
			}
		}

		if err := d.publisher.Publish(ctx, action.PublishSubject, data); err != nil {
			d.logger.Error("Failed to publish completion event",
				"error", err,
				"subject", action.PublishSubject)
			// Don't fail the action - state was already updated
		} else {
			result.Published = true
			d.logger.Debug("Published completion event",
				"subject", action.PublishSubject,
				"execution_id", GetID(ruleCtx.State))
		}
	}

	// Publish workflow-level completion event if configured (different from rule event)
	if def.Events.OnComplete != "" && d.publisher != nil {
		if err := d.publishCompletionEvent(ctx, ruleCtx, def.Events.OnComplete); err != nil {
			d.logger.Error("Failed to publish workflow completion event",
				"error", err,
				"subject", def.Events.OnComplete)
			// Don't fail the action - state was already updated
		} else if !result.Published {
			result.Published = true
		}
	}

	d.logger.Info("Execution completed",
		"execution_id", GetID(ruleCtx.State),
		"workflow_id", def.ID)

	return result, nil
}

// writeState serializes and writes the state to KV with optimistic concurrency.
// Note: def is reserved for future workflow-specific write options.
func (d *Dispatcher) writeState(
	ctx context.Context,
	ruleCtx *RuleContext,
	_ *Definition,
) (uint64, error) {
	if d.store == nil {
		return 0, &DispatchError{
			Action:  "write_state",
			Message: "no state store configured",
		}
	}

	// Serialize state to JSON
	data, err := json.Marshal(ruleCtx.State)
	if err != nil {
		return 0, err
	}

	// Determine the key
	key := ruleCtx.KVKey
	if key == "" {
		key = GetID(ruleCtx.State)
	}

	var newRev uint64

	if ruleCtx.KVRevision > 0 {
		// Use optimistic concurrency control
		newRev, err = d.store.Update(ctx, key, data, ruleCtx.KVRevision)
	} else {
		// First write - use Put
		newRev, err = d.store.Put(ctx, key, data)
	}

	if err != nil {
		return 0, err
	}

	// NOTE: We intentionally DO NOT record own revisions here.
	//
	// Previously, we called d.kvWatcher.RecordOwnRevision(key, newRev) to prevent
	// feedback loops. However, this blocked ALL rules from firing on engine writes,
	// breaking workflow progression (e.g., generator-completed -> dispatch-reviewer).
	//
	// Workflow rules naturally prevent self-triggering through phase transitions:
	// - dispatch-generator fires on phase="generating", writes phase="dispatched"
	// - The new phase doesn't match "generating", so dispatch-generator won't re-fire
	//
	// If a workflow rule could re-trigger on its own write, that's a workflow design
	// issue to be fixed in the rule conditions, not masked by skipping all handlers.

	// Update the revision in context for subsequent operations
	ruleCtx.KVRevision = newRev

	return newRev, nil
}

// publishCompletionEvent publishes a workflow completion event.
func (d *Dispatcher) publishCompletionEvent(
	ctx context.Context,
	ruleCtx *RuleContext,
	subject string,
) error {
	event := &WorkflowCompletionEvent{
		ExecutionID: GetID(ruleCtx.State),
		WorkflowID:  GetWorkflowID(ruleCtx.State),
		Status:      string(GetStatus(ruleCtx.State)),
		Phase:       GetPhase(ruleCtx.State),
		CompletedAt: time.Now(),
	}

	// Create message with well-known type
	msgType := message.Type{
		Domain:   "workflow",
		Category: "completion",
		Version:  "v1",
	}

	baseMsg := message.NewBaseMessage(msgType, event, d.source)

	data, err := json.Marshal(baseMsg)
	if err != nil {
		return err
	}

	return d.publisher.Publish(ctx, subject, data)
}

// HandleCallback processes an async callback result.
// It looks up the execution by task ID, deserializes the result,
// invokes the state mutator, and writes the updated state to KV.
func (d *Dispatcher) HandleCallback(
	ctx context.Context,
	ruleCtx *RuleContext,
	rule *RuleDef,
	result any,
	def *Definition,
) (*DispatchResult, error) {
	if ruleCtx.State == nil {
		return nil, &DispatchError{
			Action:  "callback",
			Message: "no state available",
		}
	}

	// Clear the pending task
	ClearPendingTask(ruleCtx.State)

	// Apply the state mutation with the result
	if rule.Action.MutateState != nil {
		if err := rule.Action.MutateState(ruleCtx, result); err != nil {
			// Set error on state and fail execution
			FailExecution(ruleCtx.State, "callback mutation failed: "+err.Error())

			// Still write the failed state - this is important for observability
			newRev, writeErr := d.writeState(ctx, ruleCtx, def)
			if writeErr != nil {
				d.logger.Error("Failed to write failed state",
					"error", writeErr,
					"original_error", err)
				return nil, &DispatchError{
					Action:  "callback",
					Message: "state mutation failed and state write failed: " + err.Error(),
					Cause:   writeErr,
				}
			}

			// Return partial success - state was written with failed status
			return &DispatchResult{
				NewRevision:    newRev,
				StateUpdated:   true,
				PartialFailure: true,
				PartialError:   "callback mutation failed: " + err.Error(),
			}, nil
		}
	}

	// Write updated state to KV
	newRev, err := d.writeState(ctx, ruleCtx, def)
	if err != nil {
		return nil, &DispatchError{
			Action:  "callback",
			Message: "failed to write state: " + err.Error(),
		}
	}

	d.logger.Debug("Callback processed",
		"key", ruleCtx.KVKey,
		"revision", newRev,
		"execution_id", GetID(ruleCtx.State))

	return &DispatchResult{
		NewRevision:  newRev,
		StateUpdated: true,
	}, nil
}

// HandleFailure records a failure and optionally publishes failure event.
func (d *Dispatcher) HandleFailure(
	ctx context.Context,
	ruleCtx *RuleContext,
	errMsg string,
	def *Definition,
) (*DispatchResult, error) {
	result := &DispatchResult{}

	if ruleCtx.State != nil {
		FailExecution(ruleCtx.State, errMsg)

		newRev, err := d.writeState(ctx, ruleCtx, def)
		if err != nil {
			d.logger.Error("Failed to write failure state",
				"error", err,
				"original_error", errMsg)
		} else {
			result.NewRevision = newRev
			result.StateUpdated = true
		}
	}

	// Publish failure event if configured
	if def.Events.OnFail != "" && d.publisher != nil {
		if err := d.publishFailureEvent(ctx, ruleCtx, def.Events.OnFail, errMsg); err != nil {
			d.logger.Error("Failed to publish failure event",
				"error", err,
				"subject", def.Events.OnFail)
		} else {
			result.Published = true
		}
	}

	d.logger.Error("Execution failed",
		"execution_id", GetID(ruleCtx.State),
		"error", errMsg)

	return result, nil
}

// publishFailureEvent publishes a workflow failure event.
func (d *Dispatcher) publishFailureEvent(
	ctx context.Context,
	ruleCtx *RuleContext,
	subject string,
	errMsg string,
) error {
	event := &WorkflowFailureEvent{
		ExecutionID: GetID(ruleCtx.State),
		WorkflowID:  GetWorkflowID(ruleCtx.State),
		Phase:       GetPhase(ruleCtx.State),
		Error:       errMsg,
		FailedAt:    time.Now(),
	}

	msgType := message.Type{
		Domain:   "workflow",
		Category: "failure",
		Version:  "v1",
	}

	baseMsg := message.NewBaseMessage(msgType, event, d.source)

	data, err := json.Marshal(baseMsg)
	if err != nil {
		return err
	}

	return d.publisher.Publish(ctx, subject, data)
}

// HandleEscalation records an escalation and optionally publishes escalation event.
func (d *Dispatcher) HandleEscalation(
	ctx context.Context,
	ruleCtx *RuleContext,
	reason string,
	def *Definition,
) (*DispatchResult, error) {
	result := &DispatchResult{}

	if ruleCtx.State != nil {
		EscalateExecution(ruleCtx.State, reason)

		newRev, err := d.writeState(ctx, ruleCtx, def)
		if err != nil {
			d.logger.Error("Failed to write escalation state",
				"error", err,
				"reason", reason)
		} else {
			result.NewRevision = newRev
			result.StateUpdated = true
		}
	}

	// Publish escalation event if configured
	if def.Events.OnEscalate != "" && d.publisher != nil {
		if err := d.publishEscalationEvent(ctx, ruleCtx, def.Events.OnEscalate, reason); err != nil {
			d.logger.Error("Failed to publish escalation event",
				"error", err,
				"subject", def.Events.OnEscalate)
		} else {
			result.Published = true
		}
	}

	d.logger.Warn("Execution escalated",
		"execution_id", GetID(ruleCtx.State),
		"reason", reason)

	return result, nil
}

// publishEscalationEvent publishes a workflow escalation event.
func (d *Dispatcher) publishEscalationEvent(
	ctx context.Context,
	ruleCtx *RuleContext,
	subject string,
	reason string,
) error {
	event := &WorkflowEscalationEvent{
		ExecutionID: GetID(ruleCtx.State),
		WorkflowID:  GetWorkflowID(ruleCtx.State),
		Phase:       GetPhase(ruleCtx.State),
		Reason:      reason,
		EscalatedAt: time.Now(),
	}

	msgType := message.Type{
		Domain:   "workflow",
		Category: "escalation",
		Version:  "v1",
	}

	baseMsg := message.NewBaseMessage(msgType, event, d.source)

	data, err := json.Marshal(baseMsg)
	if err != nil {
		return err
	}

	return d.publisher.Publish(ctx, subject, data)
}

// buildCallbackSubject creates the callback subject for async actions.
func buildCallbackSubject(workflowID, executionID string) string {
	return "workflow.callback." + workflowID + "." + executionID
}

// CallbackInjectable is implemented by payloads that can receive callback fields.
type CallbackInjectable interface {
	InjectCallback(fields CallbackFields)
}

// WorkflowCompletionEvent is published when a workflow completes successfully.
type WorkflowCompletionEvent struct {
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	Status      string    `json:"status"`
	Phase       string    `json:"final_phase"`
	CompletedAt time.Time `json:"completed_at"`
}

// Schema returns the message type for WorkflowCompletionEvent.
func (e *WorkflowCompletionEvent) Schema() message.Type {
	return message.Type{
		Domain:   "workflow",
		Category: "completion",
		Version:  "v1",
	}
}

// Validate validates the completion event.
func (e *WorkflowCompletionEvent) Validate() error {
	if e.ExecutionID == "" {
		return &ValidationError{Field: "execution_id", Message: "required"}
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (e *WorkflowCompletionEvent) MarshalJSON() ([]byte, error) {
	type Alias WorkflowCompletionEvent
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *WorkflowCompletionEvent) UnmarshalJSON(data []byte) error {
	type Alias WorkflowCompletionEvent
	return json.Unmarshal(data, (*Alias)(e))
}

// WorkflowFailureEvent is published when a workflow fails.
type WorkflowFailureEvent struct {
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	Phase       string    `json:"phase"`
	Error       string    `json:"error"`
	FailedAt    time.Time `json:"failed_at"`
}

// Schema returns the message type for WorkflowFailureEvent.
func (e *WorkflowFailureEvent) Schema() message.Type {
	return message.Type{
		Domain:   "workflow",
		Category: "failure",
		Version:  "v1",
	}
}

// Validate validates the failure event.
func (e *WorkflowFailureEvent) Validate() error {
	if e.ExecutionID == "" {
		return &ValidationError{Field: "execution_id", Message: "required"}
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (e *WorkflowFailureEvent) MarshalJSON() ([]byte, error) {
	type Alias WorkflowFailureEvent
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *WorkflowFailureEvent) UnmarshalJSON(data []byte) error {
	type Alias WorkflowFailureEvent
	return json.Unmarshal(data, (*Alias)(e))
}

// WorkflowEscalationEvent is published when a workflow is escalated.
type WorkflowEscalationEvent struct {
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	Phase       string    `json:"phase"`
	Reason      string    `json:"reason"`
	EscalatedAt time.Time `json:"escalated_at"`
}

// Schema returns the message type for WorkflowEscalationEvent.
func (e *WorkflowEscalationEvent) Schema() message.Type {
	return message.Type{
		Domain:   "workflow",
		Category: "escalation",
		Version:  "v1",
	}
}

// Validate validates the escalation event.
func (e *WorkflowEscalationEvent) Validate() error {
	if e.ExecutionID == "" {
		return &ValidationError{Field: "execution_id", Message: "required"}
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (e *WorkflowEscalationEvent) MarshalJSON() ([]byte, error) {
	type Alias WorkflowEscalationEvent
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *WorkflowEscalationEvent) UnmarshalJSON(data []byte) error {
	type Alias WorkflowEscalationEvent
	return json.Unmarshal(data, (*Alias)(e))
}

// DispatchError represents an error during action dispatch.
type DispatchError struct {
	Action  string
	Subject string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *DispatchError) Error() string {
	if e.Subject != "" {
		return "dispatch " + e.Action + " to " + e.Subject + ": " + e.Message
	}
	return "dispatch " + e.Action + ": " + e.Message
}

// Unwrap returns the underlying error.
func (e *DispatchError) Unwrap() error {
	return e.Cause
}
