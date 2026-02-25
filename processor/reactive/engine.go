package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// Engine orchestrates the reactive workflow engine.
// It manages all sub-components: triggers, evaluator, dispatcher, callback handler, and store.
type Engine struct {
	logger     *slog.Logger
	config     Config
	natsClient *natsclient.Client

	// Core components
	registry        *WorkflowRegistry
	evaluator       *Evaluator
	dispatcher      *Dispatcher
	callbackHandler *CallbackHandler
	subjectConsumer *SubjectConsumer
	kvWatcher       *KVWatcher

	// KV bucket for execution state
	stateBucket jetstream.KeyValue

	// Execution store
	store *ExecutionStore

	// Lifecycle
	mu        sync.RWMutex
	started   bool
	startTime time.Time
	ctx       context.Context
	cancel    context.CancelFunc

	// Cleanup ticker
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}

	// Metrics (optional, set via WithEngineMetrics)
	metrics MetricsRecorder
}

// MetricsRecorder records engine metrics.
type MetricsRecorder interface {
	RecordRuleEvaluation(workflowID, ruleID string, fired bool)
	RecordActionDispatch(workflowID, ruleID, actionType string)
	RecordExecutionCreated(workflowID string)
}

// EngineOption configures an Engine.
type EngineOption func(*Engine)

// WithEngineLogger sets the logger for the engine.
func WithEngineLogger(logger *slog.Logger) EngineOption {
	return func(e *Engine) {
		e.logger = logger
	}
}

// WithEngineMetrics sets the metrics recorder for the engine.
func WithEngineMetrics(metrics MetricsRecorder) EngineOption {
	return func(e *Engine) {
		e.metrics = metrics
	}
}

// NewEngine creates a new reactive workflow engine.
func NewEngine(
	config Config,
	natsClient *natsclient.Client,
	opts ...EngineOption,
) *Engine {
	e := &Engine{
		config:     config,
		natsClient: natsClient,
		logger:     slog.Default(),
		registry:   NewWorkflowRegistry(nil),
	}

	for _, opt := range opts {
		opt(e)
	}

	// Update registry logger
	e.registry = NewWorkflowRegistry(e.logger)

	return e
}

// Registry returns the workflow registry.
func (e *Engine) Registry() *WorkflowRegistry {
	return e.registry
}

// RegisterWorkflow registers a workflow definition.
func (e *Engine) RegisterWorkflow(def *Definition) error {
	return e.registry.Register(def)
}

// Initialize prepares the engine for starting.
// Call this after registering workflows but before Start.
func (e *Engine) Initialize(ctx context.Context) error {
	if e.natsClient == nil {
		return &EngineError{Op: "initialize", Message: "NATS client is required"}
	}

	js, err := e.natsClient.JetStream()
	if err != nil {
		return &EngineError{Op: "initialize", Message: "failed to get JetStream", Cause: err}
	}

	// Get or create the state bucket
	e.stateBucket, err = js.KeyValue(ctx, e.config.StateBucket)
	if err != nil {
		// Try to create the bucket
		e.stateBucket, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      e.config.StateBucket,
			Description: "Reactive workflow execution state",
			TTL:         0, // No automatic expiration
		})
		if err != nil {
			return &EngineError{Op: "initialize", Message: "failed to get/create state bucket", Cause: err}
		}
	}

	// Create core components
	e.subjectConsumer = NewSubjectConsumer(e.logger)
	e.kvWatcher = NewKVWatcher(e.logger)
	e.evaluator = NewEvaluator(e.logger)

	// Create dispatcher with state store
	stateStore := &engineStateStore{
		bucket: e.stateBucket,
		logger: e.logger,
	}
	publisher := &natsPublisher{client: e.natsClient}
	e.dispatcher = NewDispatcher(e.logger,
		WithPublisher(publisher),
		WithStateStore(stateStore),
		WithKVWatcher(e.kvWatcher),
		WithSource("reactive-workflow-engine"),
	)

	// Create callback handler
	e.callbackHandler = NewCallbackHandler(e.logger, e.subjectConsumer, e.dispatcher, stateStore)

	e.logger.Info("Engine initialized",
		"state_bucket", e.config.StateBucket,
		"workflows", e.registry.Count())

	return nil
}

// Start starts the engine and all trigger loops.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return &EngineError{Op: "start", Message: "already started"}
	}

	// Create cancellable context
	e.ctx, e.cancel = context.WithCancel(ctx)

	// Start KV watch triggers for all registered workflows
	if err := e.startKVTriggers(e.ctx); err != nil {
		e.cancel()
		return &EngineError{Op: "start", Message: "failed to start KV triggers", Cause: err}
	}

	// Start subject triggers for all registered workflows
	if err := e.startSubjectTriggers(e.ctx); err != nil {
		e.cancel()
		return &EngineError{Op: "start", Message: "failed to start subject triggers", Cause: err}
	}

	// Start cleanup goroutine
	e.startCleanup()

	e.started = true
	e.startTime = time.Now()

	e.logger.Info("Engine started",
		"workflows", e.registry.Count())

	return nil
}

// Stop stops the engine and all trigger loops.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started {
		return
	}

	// Cancel context to stop all goroutines
	if e.cancel != nil {
		e.cancel()
	}

	// Stop cleanup
	if e.cleanupTicker != nil {
		e.cleanupTicker.Stop()
		close(e.cleanupDone)
	}

	// Stop callback handler
	if e.callbackHandler != nil {
		e.callbackHandler.Stop()
	}

	// Stop KV watcher
	if e.kvWatcher != nil {
		e.kvWatcher.StopAll()
	}

	// Stop subject consumer
	if e.subjectConsumer != nil {
		e.subjectConsumer.StopAll()
	}

	e.started = false

	e.logger.Info("Engine stopped")
}

// IsRunning returns whether the engine is running.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.started
}

// Uptime returns how long the engine has been running.
func (e *Engine) Uptime() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return 0
	}
	return time.Since(e.startTime)
}

// startKVTriggers starts KV watch loops for all workflows with KV triggers.
func (e *Engine) startKVTriggers(ctx context.Context) error {
	js, err := e.natsClient.JetStream()
	if err != nil {
		return err
	}

	for _, def := range e.registry.GetAll() {
		for i := range def.Rules {
			rule := &def.Rules[i]
			mode := rule.Trigger.Mode()

			if mode != TriggerStateOnly && mode != TriggerMessageAndState {
				continue
			}

			// Get or create the watch bucket
			watchBucket, err := js.KeyValue(ctx, rule.Trigger.WatchBucket)
			if err != nil {
				return &EngineError{
					Op:      "start_kv_trigger",
					Message: "failed to get watch bucket: " + rule.Trigger.WatchBucket,
					Cause:   err,
				}
			}

			// Capture rule and def for the closure
			capturedRule := rule
			capturedDef := def

			// Start watching (no watcher name - use bucket+pattern as key internally)
			err = e.kvWatcher.StartWatch(
				ctx,
				watchBucket,
				rule.Trigger.WatchPattern,
				func(ctx context.Context, event KVWatchEvent) {
					e.handleKVEvent(ctx, event, capturedRule, capturedDef)
				},
			)
			if err != nil {
				return &EngineError{
					Op:      "start_kv_trigger",
					Message: "failed to start KV watch for rule " + rule.ID,
					Cause:   err,
				}
			}

			e.logger.Debug("Started KV trigger",
				"workflow", def.ID,
				"rule", rule.ID,
				"bucket", rule.Trigger.WatchBucket,
				"pattern", rule.Trigger.WatchPattern)
		}
	}

	return nil
}

// startSubjectTriggers starts subject consumer loops for all workflows with subject triggers.
func (e *Engine) startSubjectTriggers(ctx context.Context) error {
	js, err := e.natsClient.JetStream()
	if err != nil {
		return err
	}

	for _, def := range e.registry.GetAll() {
		for i := range def.Rules {
			rule := &def.Rules[i]
			mode := rule.Trigger.Mode()

			if mode != TriggerMessageOnly && mode != TriggerMessageAndState {
				continue
			}

			// Build consumer name
			consumerName := e.config.ConsumerNamePrefix + def.ID + "-" + rule.ID

			// Capture rule and def for the closure
			capturedRule := rule
			capturedDef := def

			// Start consuming
			err = e.subjectConsumer.StartConsumer(
				ctx,
				js,
				rule.Trigger.StreamName,
				rule.Trigger.Subject,
				consumerName,
				func(ctx context.Context, event SubjectMessageEvent, msg jetstream.Msg) {
					e.handleSubjectEvent(ctx, event, msg, capturedRule, capturedDef)
				},
			)
			if err != nil {
				return &EngineError{
					Op:      "start_subject_trigger",
					Message: "failed to start subject consumer for rule " + rule.ID,
					Cause:   err,
				}
			}

			e.logger.Debug("Started subject trigger",
				"workflow", def.ID,
				"rule", rule.ID,
				"stream", rule.Trigger.StreamName,
				"subject", rule.Trigger.Subject)
		}
	}

	return nil
}

// handleKVEvent processes a KV watch event.
func (e *Engine) handleKVEvent(ctx context.Context, event KVWatchEvent, rule *RuleDef, def *Definition) {
	// Build rule context from KV event
	ruleCtx, err := BuildRuleContextFromKV(event, def.StateFactory)
	if err != nil {
		e.logger.Error("Failed to build rule context from KV event",
			"rule", rule.ID,
			"key", event.Key,
			"error", err)
		return
	}

	// Set KV metadata
	ruleCtx.KVKey = event.Key
	ruleCtx.KVRevision = event.Revision

	// Evaluate and potentially fire the rule
	e.evaluateAndFire(ctx, ruleCtx, rule, def)
}

// handleSubjectEvent processes a subject message event.
func (e *Engine) handleSubjectEvent(
	ctx context.Context,
	event SubjectMessageEvent,
	msg jetstream.Msg,
	rule *RuleDef,
	def *Definition,
) {
	// Build rule context from message
	ruleCtx, err := e.buildRuleContextFromMessage(ctx, event, rule, def)
	if err != nil {
		e.logger.Error("Failed to build rule context from message",
			"rule", rule.ID,
			"subject", event.Subject,
			"error", err)
		// Nak the message to allow retry
		_ = msg.Nak()
		return
	}

	ruleCtx.Subject = event.Subject

	// Evaluate and potentially fire the rule
	e.evaluateAndFire(ctx, ruleCtx, rule, def)

	// Always acknowledge the message after evaluation (don't retry just because conditions didn't match)
	_ = msg.Ack()
}

// buildRuleContextFromMessage builds a RuleContext from a subject message event.
func (e *Engine) buildRuleContextFromMessage(
	ctx context.Context,
	event SubjectMessageEvent,
	rule *RuleDef,
	def *Definition,
) (*RuleContext, error) {
	// Deserialize the message payload
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(event.Data, &baseMsg); err != nil {
		return nil, &MessageDeserializeError{
			Subject: event.Subject,
			Cause:   err,
		}
	}

	ruleCtx := &RuleContext{
		Message: baseMsg.Payload(),
		Subject: event.Subject,
	}

	// If this is a combined trigger, load state
	if rule.Trigger.Mode() == TriggerMessageAndState {
		// Get the state key from the message or use a key function
		stateKey := ""
		if rule.Trigger.StateKeyFunc != nil {
			stateKey = rule.Trigger.StateKeyFunc(baseMsg.Payload())
		}

		if stateKey != "" && rule.Trigger.StateBucket != "" {
			js, err := e.natsClient.JetStream()
			if err != nil {
				return nil, err
			}

			bucket, err := js.KeyValue(ctx, rule.Trigger.StateBucket)
			if err != nil {
				return nil, &StateLoadError{
					Key:   stateKey,
					Cause: err,
				}
			}

			entry, err := bucket.Get(ctx, stateKey)
			if err != nil {
				// Key not found is not an error for combined triggers
				if err != jetstream.ErrKeyNotFound {
					return nil, &StateLoadError{
						Key:   stateKey,
						Cause: err,
					}
				}
			} else {
				// Deserialize state
				state := def.StateFactory()
				if err := json.Unmarshal(entry.Value(), state); err != nil {
					return nil, &StateLoadError{
						Key:   stateKey,
						Cause: err,
					}
				}
				ruleCtx.State = state
				ruleCtx.KVKey = stateKey
				ruleCtx.KVRevision = entry.Revision()
			}
		}
	}

	return ruleCtx, nil
}

// evaluateAndFire evaluates a rule and fires it if conditions are met.
func (e *Engine) evaluateAndFire(ctx context.Context, ruleCtx *RuleContext, rule *RuleDef, def *Definition) bool {
	// Extract execution ID from state if available
	executionID := ""
	if ruleCtx.State != nil {
		es := ExtractExecutionState(ruleCtx.State)
		if es != nil {
			executionID = es.ID
		}
	}

	// Evaluate the rule
	result := e.evaluator.EvaluateRule(ruleCtx, rule, def.ID, executionID)

	if e.metrics != nil {
		e.metrics.RecordRuleEvaluation(def.ID, rule.ID, result.ShouldFire)
	}

	if !result.ShouldFire {
		e.logger.Debug("Rule conditions not met",
			"workflow", def.ID,
			"rule", rule.ID,
			"reason", result.Reason)
		return false
	}

	// Fire the rule
	e.logger.Debug("Firing rule",
		"workflow", def.ID,
		"rule", rule.ID,
		"action", rule.Action.Type)

	dispatchResult, err := e.dispatcher.DispatchAction(ctx, ruleCtx, rule, def)
	if err != nil {
		e.logger.Error("Failed to dispatch rule action",
			"workflow", def.ID,
			"rule", rule.ID,
			"error", err)
		return false
	}

	if e.metrics != nil {
		e.metrics.RecordActionDispatch(def.ID, rule.ID, rule.Action.Type.String())
	}

	// If this was an async action, register for callback
	if rule.Action.Type == ActionPublishAsync && dispatchResult.TaskID != "" {
		timeout := e.config.GetTaskTimeoutDefault()
		timeoutAt := time.Now().Add(timeout)

		reg := &TaskRegistration{
			TaskID:             dispatchResult.TaskID,
			ExecutionKey:       ruleCtx.KVKey,
			ExecutionID:        executionID,
			WorkflowID:         def.ID,
			RuleID:             rule.ID,
			ExpectedResultType: rule.Action.ExpectedResultType,
			RegisteredAt:       time.Now(),
			Timeout:            &timeoutAt,
		}
		e.callbackHandler.RegisterTask(reg, rule, def)
	}

	e.logger.Info("Rule fired",
		"workflow", def.ID,
		"rule", rule.ID,
		"action", rule.Action.Type,
		"task_id", dispatchResult.TaskID)

	return true
}

// startCleanup starts the periodic cleanup goroutine.
func (e *Engine) startCleanup() {
	e.cleanupDone = make(chan struct{})
	e.cleanupTicker = time.NewTicker(e.config.GetCleanupInterval())

	go func() {
		for {
			select {
			case <-e.cleanupTicker.C:
				e.runCleanup()
			case <-e.cleanupDone:
				return
			}
		}
	}()
}

// runCleanup performs periodic cleanup tasks.
func (e *Engine) runCleanup() {
	// Cleanup expired task registrations
	if e.callbackHandler != nil {
		expired := e.callbackHandler.CleanupExpiredTasks()
		if expired > 0 {
			e.logger.Info("Cleaned up expired task registrations", "count", expired)
		}
	}

	// Note: ExecutionStore cleanup would go here if we had an active store instance
}

// CreateExecution creates a new workflow execution.
func (e *Engine) CreateExecution(ctx context.Context, workflowID, executionID string) (*ExecutionEntry, error) {
	def := e.registry.Get(workflowID)
	if def == nil {
		return nil, &EngineError{
			Op:      "create_execution",
			Message: "workflow not found: " + workflowID,
		}
	}

	// Create state using the workflow's factory
	state := def.StateFactory()

	// Initialize the execution
	timeout := def.Timeout
	if timeout == 0 {
		timeout = e.config.GetDefaultTimeout()
	}
	InitializeExecution(state, executionID, workflowID, timeout)

	// Build the key
	key := workflowID + "." + executionID

	// Serialize and store
	data, err := json.Marshal(state)
	if err != nil {
		return nil, &EngineError{Op: "create_execution", Message: "failed to marshal state", Cause: err}
	}

	rev, err := e.stateBucket.Create(ctx, key, data)
	if err != nil {
		return nil, &EngineError{Op: "create_execution", Message: "failed to create execution", Cause: err}
	}

	if e.metrics != nil {
		e.metrics.RecordExecutionCreated(workflowID)
	}

	e.logger.Info("Created execution",
		"workflow", workflowID,
		"execution", executionID,
		"key", key)

	return &ExecutionEntry{
		State:    state,
		Key:      key,
		Revision: rev,
		Created:  time.Now(),
	}, nil
}

// EngineError represents an error from the engine.
type EngineError struct {
	Op      string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *EngineError) Error() string {
	if e.Cause != nil {
		return "engine " + e.Op + ": " + e.Message + ": " + e.Cause.Error()
	}
	return "engine " + e.Op + ": " + e.Message
}

// Unwrap returns the underlying error.
func (e *EngineError) Unwrap() error {
	return e.Cause
}

// natsPublisher implements Publisher using natsclient.
type natsPublisher struct {
	client *natsclient.Client
}

func (p *natsPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	return p.client.Publish(ctx, subject, data)
}

// engineStateStore implements StateStore for the dispatcher.
type engineStateStore struct {
	bucket jetstream.KeyValue
	logger *slog.Logger
}

func (s *engineStateStore) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	return s.bucket.Get(ctx, key)
}

func (s *engineStateStore) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	return s.bucket.Put(ctx, key, value)
}

func (s *engineStateStore) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	return s.bucket.Update(ctx, key, value, revision)
}
