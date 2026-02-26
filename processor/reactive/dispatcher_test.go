package reactive

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPublisher captures published messages for testing.
type mockPublisher struct {
	mu       sync.Mutex
	messages []publishedMessage
	err      error
}

type publishedMessage struct {
	Subject string
	Data    []byte
}

func (p *mockPublisher) Publish(_ context.Context, subject string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.err != nil {
		return p.err
	}

	p.messages = append(p.messages, publishedMessage{
		Subject: subject,
		Data:    data,
	})
	return nil
}

func (p *mockPublisher) getMessages() []publishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]publishedMessage, len(p.messages))
	copy(result, p.messages)
	return result
}

// mockStateStore provides an in-memory KV store for testing.
type mockStateStore struct {
	mu        sync.RWMutex
	data      map[string][]byte
	revisions map[string]uint64
	nextRev   uint64
	getErr    error
	putErr    error
	updateErr error
}

func newMockStateStore() *mockStateStore {
	return &mockStateStore{
		data:      make(map[string][]byte),
		revisions: make(map[string]uint64),
		nextRev:   1,
	}
}

func (s *mockStateStore) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.getErr != nil {
		return nil, s.getErr
	}

	value, exists := s.data[key]
	if !exists {
		return nil, jetstream.ErrKeyNotFound
	}

	return &mockKVEntry{
		key:      key,
		value:    value,
		revision: s.revisions[key],
	}, nil
}

func (s *mockStateStore) Put(_ context.Context, key string, value []byte) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.putErr != nil {
		return 0, s.putErr
	}

	s.data[key] = value
	s.revisions[key] = s.nextRev
	rev := s.nextRev
	s.nextRev++
	return rev, nil
}

func (s *mockStateStore) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.updateErr != nil {
		return 0, s.updateErr
	}

	currentRev, exists := s.revisions[key]
	if !exists {
		return 0, errors.New("key not found")
	}
	if currentRev != revision {
		return 0, errors.New("revision mismatch")
	}

	s.data[key] = value
	s.revisions[key] = s.nextRev
	rev := s.nextRev
	s.nextRev++
	return rev, nil
}

func (s *mockStateStore) getData(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.data[key]
	return data, ok
}

// mockKVEntry implements jetstream.KeyValueEntry for testing.
type mockKVEntry struct {
	key      string
	value    []byte
	revision uint64
}

func (e *mockKVEntry) Bucket() string                  { return "test-bucket" }
func (e *mockKVEntry) Key() string                     { return e.key }
func (e *mockKVEntry) Value() []byte                   { return e.value }
func (e *mockKVEntry) Revision() uint64                { return e.revision }
func (e *mockKVEntry) Created() time.Time              { return time.Now() }
func (e *mockKVEntry) Delta() uint64                   { return 0 }
func (e *mockKVEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }

// TestPayload is a simple payload type for testing.
type TestPayload struct {
	TaskID   string          `json:"task_id"`
	Content  string          `json:"content"`
	Callback *CallbackFields `json:"callback,omitempty"`
}

func (p *TestPayload) Schema() message.Type {
	return message.Type{
		Domain:   "test",
		Category: "payload",
		Version:  "v1",
	}
}

func (p *TestPayload) Validate() error {
	return nil
}

func (p *TestPayload) MarshalJSON() ([]byte, error) {
	type Alias TestPayload
	return json.Marshal((*Alias)(p))
}

func (p *TestPayload) UnmarshalJSON(data []byte) error {
	type Alias TestPayload
	return json.Unmarshal(data, (*Alias)(p))
}

func (p *TestPayload) InjectCallback(fields CallbackFields) {
	p.Callback = &fields
}

// TestDispatcherState embeds ExecutionState for testing.
type TestDispatcherState struct {
	ExecutionState
	CustomField string `json:"custom_field"`
}

func (s *TestDispatcherState) GetExecutionState() *ExecutionState {
	return &s.ExecutionState
}

func newTestDispatcherState() *TestDispatcherState {
	state := &TestDispatcherState{
		CustomField: "test-value",
	}
	InitializeExecution(state, "exec-123", "workflow-test", 5*time.Minute)
	return state
}

func newTestDefinition() *Definition {
	return &Definition{
		ID:          "workflow-test",
		StateBucket: "test-bucket",
		StateFactory: func() any {
			return &TestDispatcherState{}
		},
	}
}

func TestDispatcher_DispatchPublish(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
		WithSource("test-dispatcher"),
	)

	state := newTestDispatcherState()
	state.Phase = "ready"

	// Pre-populate store with initial state
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "publish-rule",
		Action: Action{
			Type:           ActionPublish,
			PublishSubject: "test.output",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return &TestPayload{
					Content: "test-content",
				}, nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err)

	assert.True(t, result.Published)
	assert.False(t, result.StateUpdated) // No MutateState defined

	// Verify message was published
	messages := publisher.getMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "test.output", messages[0].Subject)

	// Verify message content
	var baseMsg map[string]any
	err = json.Unmarshal(messages[0].Data, &baseMsg)
	require.NoError(t, err)
	assert.Equal(t, "test", baseMsg["type"].(map[string]any)["domain"])
}

func TestDispatcher_DispatchPublish_WithMutation(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Phase = "initial"

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "publish-rule",
		Action: Action{
			Type:           ActionPublish,
			PublishSubject: "test.output",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return &TestPayload{Content: "content"}, nil
			},
			MutateState: func(ctx *RuleContext, _ any) error {
				state := ctx.State.(*TestDispatcherState)
				state.Phase = "published"
				return nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err)

	assert.True(t, result.Published)
	assert.True(t, result.StateUpdated)
	assert.Greater(t, result.NewRevision, rev)

	// Verify state was written to store
	storedData, ok := store.getData(state.ID)
	require.True(t, ok)

	var storedState TestDispatcherState
	err = json.Unmarshal(storedData, &storedState)
	require.NoError(t, err)
	assert.Equal(t, "published", storedState.Phase)
}

func TestDispatcher_DispatchPublishAsync(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Phase = "pending"

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "async-rule",
		Action: Action{
			Type:               ActionPublishAsync,
			PublishSubject:     "test.async.input",
			ExpectedResultType: "test.result.v1",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return &TestPayload{
					Content: "async-task",
				}, nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err)

	// Verify result
	assert.True(t, result.Published)
	assert.True(t, result.StateUpdated)
	assert.NotEmpty(t, result.TaskID)

	// Verify message was published with callback info
	messages := publisher.getMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "test.async.input", messages[0].Subject)

	// Verify payload has callback fields injected
	var baseMsg struct {
		Payload TestPayload `json:"payload"`
	}
	err = json.Unmarshal(messages[0].Data, &baseMsg)
	require.NoError(t, err)
	require.NotNil(t, baseMsg.Payload.Callback)
	assert.Equal(t, result.TaskID, baseMsg.Payload.Callback.TaskID)
	assert.Contains(t, baseMsg.Payload.Callback.CallbackSubject, "workflow.callback")

	// Verify state is now waiting
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusWaiting, storedState.Status)
	assert.Equal(t, result.TaskID, storedState.PendingTaskID)
}

func TestDispatcher_DispatchMutate(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Phase = "initial"
	state.CustomField = "original"

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type: ActionMutate,
			MutateState: func(ctx *RuleContext, _ any) error {
				state := ctx.State.(*TestDispatcherState)
				state.Phase = "mutated"
				state.CustomField = "updated"
				return nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err)

	assert.False(t, result.Published)
	assert.True(t, result.StateUpdated)
	assert.Greater(t, result.NewRevision, rev)

	// Verify state was mutated and stored
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, "mutated", storedState.Phase)
	assert.Equal(t, "updated", storedState.CustomField)
}

func TestDispatcher_DispatchComplete(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Phase = "final"
	state.Status = StatusRunning

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "complete-rule",
		Action: Action{
			Type: ActionComplete,
		},
	}

	def := newTestDefinition()
	def.Events.OnComplete = "test.workflow.completed"

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err)

	assert.True(t, result.Published)
	assert.True(t, result.StateUpdated)

	// Verify state is completed
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusCompleted, storedState.Status)
	assert.NotNil(t, storedState.CompletedAt)

	// Verify completion event was published
	messages := publisher.getMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "test.workflow.completed", messages[0].Subject)
}

func TestDispatcher_HandleCallback(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Status = StatusWaiting
	state.PendingTaskID = "task-123"
	state.PendingRuleID = "async-rule"
	state.Phase = "waiting"

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	callbackResult := map[string]any{
		"status": "success",
		"output": "callback-result",
	}

	rule := &RuleDef{
		ID: "async-rule",
		Action: Action{
			Type:               ActionPublishAsync,
			ExpectedResultType: "test.result.v1",
			MutateState: func(ctx *RuleContext, _ any) error {
				state := ctx.State.(*TestDispatcherState)
				state.Phase = "callback-processed"
				state.CustomField = "callback-value"
				return nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.HandleCallback(context.Background(), ruleCtx, rule, callbackResult, def)
	require.NoError(t, err)

	assert.True(t, result.StateUpdated)

	// Verify state was updated
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusRunning, storedState.Status) // Cleared from waiting
	assert.Empty(t, storedState.PendingTaskID)
	assert.Equal(t, "callback-processed", storedState.Phase)
	assert.Equal(t, "callback-value", storedState.CustomField)
}

func TestDispatcher_HandleFailure(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Status = StatusRunning

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	def := newTestDefinition()
	def.Events.OnFail = "test.workflow.failed"

	result, err := dispatcher.HandleFailure(context.Background(), ruleCtx, "something went wrong", def)
	require.NoError(t, err)

	assert.True(t, result.Published)
	assert.True(t, result.StateUpdated)

	// Verify state is failed
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusFailed, storedState.Status)
	assert.Equal(t, "something went wrong", storedState.Error)

	// Verify failure event was published
	messages := publisher.getMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "test.workflow.failed", messages[0].Subject)
}

func TestDispatcher_HandleEscalation(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Status = StatusRunning
	state.Iteration = 10

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	def := newTestDefinition()
	def.Events.OnEscalate = "test.workflow.escalated"

	result, err := dispatcher.HandleEscalation(context.Background(), ruleCtx, "max iterations exceeded", def)
	require.NoError(t, err)

	assert.True(t, result.Published)
	assert.True(t, result.StateUpdated)

	// Verify state is escalated
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusEscalated, storedState.Status)
	assert.Equal(t, "max iterations exceeded", storedState.Error)

	// Verify escalation event was published
	messages := publisher.getMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "test.workflow.escalated", messages[0].Subject)
}

func TestDispatcher_NoPublisher(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
		// No publisher
	)

	state := newTestDispatcherState()

	ruleCtx := &RuleContext{
		State: state,
		KVKey: state.ID,
	}

	rule := &RuleDef{
		ID: "publish-rule",
		Action: Action{
			Type:           ActionPublish,
			PublishSubject: "test.output",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return &TestPayload{Content: "content"}, nil
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no publisher configured")
}

func TestDispatcher_NoStateStore(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		// No state store
	)

	state := newTestDispatcherState()

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: 1,
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type: ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error {
				return nil
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no state store configured")
}

func TestDispatcher_BuildPayloadError(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()

	ruleCtx := &RuleContext{
		State: state,
		KVKey: state.ID,
	}

	rule := &RuleDef{
		ID: "publish-rule",
		Action: Action{
			Type:           ActionPublish,
			PublishSubject: "test.output",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return nil, errors.New("payload build failed")
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload build failed")
}

func TestDispatcher_PublishError(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{err: errors.New("publish failed")}
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()

	ruleCtx := &RuleContext{
		State: state,
		KVKey: state.ID,
	}

	rule := &RuleDef{
		ID: "publish-rule",
		Action: Action{
			Type:           ActionPublish,
			PublishSubject: "test.output",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return &TestPayload{Content: "content"}, nil
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish failed")
}

func TestDispatcher_OptimisticConcurrencyFailure(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()
	store.updateErr = errors.New("revision mismatch")

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	state := newTestDispatcherState()

	// Put initial state
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type: ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error {
				return nil
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revision mismatch")
}

func TestDispatcher_StateMutationError(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type: ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error {
				return errors.New("mutation failed")
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutation failed")
}

func TestDispatcher_WithKVWatcher(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()
	kvWatcher := NewKVWatcher(logger)

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
		WithKVWatcher(kvWatcher),
	)

	state := newTestDispatcherState()

	// Pre-populate store with initial state (simulates existing state)
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev, // Set revision to indicate this is an update, not initial creation
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type: ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error {
				return nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err)
	assert.True(t, result.StateUpdated)

	// Verify own revision is NOT recorded - we intentionally allow subsequent rules
	// to fire on engine writes. Workflow rules prevent self-triggering through phase
	// transitions (e.g., dispatch-generator fires on phase="generating" and writes
	// phase="dispatched", so it won't re-trigger on its own write).
	kvWatcher.revisionMu.Lock()
	_, recorded := kvWatcher.ownRevisions[state.ID]
	kvWatcher.revisionMu.Unlock()
	assert.False(t, recorded, "own revision should NOT be recorded to allow workflow rule chaining")
}

func TestDispatcher_NoStateForMutate(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	ruleCtx := &RuleContext{
		State: nil, // No state
		KVKey: "some-key",
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type: ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error {
				return nil
			},
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no state available")
}

func TestWorkflowEvents_Schema(t *testing.T) {
	completionEvent := &WorkflowCompletionEvent{
		ExecutionID: "exec-1",
		WorkflowID:  "workflow-1",
		Status:      "completed",
		Phase:       "final",
		CompletedAt: time.Now(),
	}

	failureEvent := &WorkflowFailureEvent{
		ExecutionID: "exec-1",
		WorkflowID:  "workflow-1",
		Phase:       "running",
		Error:       "something failed",
		FailedAt:    time.Now(),
	}

	escalationEvent := &WorkflowEscalationEvent{
		ExecutionID: "exec-1",
		WorkflowID:  "workflow-1",
		Phase:       "loop",
		Reason:      "max iterations",
		EscalatedAt: time.Now(),
	}

	// Test schemas
	assert.Equal(t, "workflow", completionEvent.Schema().Domain)
	assert.Equal(t, "completion", completionEvent.Schema().Category)

	assert.Equal(t, "workflow", failureEvent.Schema().Domain)
	assert.Equal(t, "failure", failureEvent.Schema().Category)

	assert.Equal(t, "workflow", escalationEvent.Schema().Domain)
	assert.Equal(t, "escalation", escalationEvent.Schema().Category)

	// Test validation
	assert.NoError(t, completionEvent.Validate())
	assert.NoError(t, failureEvent.Validate())
	assert.NoError(t, escalationEvent.Validate())

	// Test validation fails without execution ID
	emptyCompletion := &WorkflowCompletionEvent{}
	assert.Error(t, emptyCompletion.Validate())
}

func TestBuildCallbackSubject(t *testing.T) {
	subject := buildCallbackSubject("plan-review", "exec-123")
	assert.Equal(t, "workflow.callback.plan-review.exec-123", subject)
}

func TestDispatchError(t *testing.T) {
	err := &DispatchError{
		Action:  "publish",
		Subject: "test.subject",
		Message: "connection failed",
	}
	assert.Equal(t, "dispatch publish to test.subject: connection failed", err.Error())

	errNoSubject := &DispatchError{
		Action:  "mutate",
		Message: "no state",
	}
	assert.Equal(t, "dispatch mutate: no state", errNoSubject.Error())
}

func TestDispatchError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &DispatchError{
		Action:  "publish",
		Message: "failed",
		Cause:   cause,
	}

	assert.True(t, errors.Is(err, cause))
	assert.Equal(t, cause, errors.Unwrap(err))
}

func TestDispatcher_PartialFailure_AsyncPublish(t *testing.T) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()
	store.updateErr = errors.New("state write failed")

	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Phase = "pending"

	// Pre-populate with initial state
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "async-rule",
		Action: Action{
			Type:               ActionPublishAsync,
			PublishSubject:     "test.async.input",
			ExpectedResultType: "test.result.v1",
			BuildPayload: func(_ *RuleContext) (message.Payload, error) {
				return &TestPayload{Content: "async-task"}, nil
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.NoError(t, err) // No error - partial failure reported in result

	// Message was published successfully
	assert.True(t, result.Published)
	assert.NotEmpty(t, result.TaskID)

	// But state write failed
	assert.False(t, result.StateUpdated)
	assert.True(t, result.PartialFailure)
	assert.Contains(t, result.PartialError, "state write failed")
}

func TestDispatcher_Mutate_NilMutateState(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "mutate-rule",
		Action: Action{
			Type:        ActionMutate,
			MutateState: nil, // Intentionally nil
		},
	}

	def := newTestDefinition()

	_, err := dispatcher.DispatchAction(context.Background(), ruleCtx, rule, def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no MutateState function")
}

func TestDispatcher_HandleCallback_PartialSuccess(t *testing.T) {
	logger := slog.Default()
	store := newMockStateStore()

	dispatcher := NewDispatcher(logger,
		WithStateStore(store),
	)

	state := newTestDispatcherState()
	state.Status = StatusWaiting
	state.PendingTaskID = "task-123"

	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	ruleCtx := &RuleContext{
		State:      state,
		KVKey:      state.ID,
		KVRevision: rev,
	}

	rule := &RuleDef{
		ID: "async-rule",
		Action: Action{
			Type:               ActionPublishAsync,
			ExpectedResultType: "test.result.v1",
			MutateState: func(_ *RuleContext, _ any) error {
				return errors.New("mutation failed")
			},
		},
	}

	def := newTestDefinition()

	result, err := dispatcher.HandleCallback(context.Background(), ruleCtx, rule, nil, def)
	require.NoError(t, err) // No error - partial failure in result

	// State was written (with failed status)
	assert.True(t, result.StateUpdated)
	assert.True(t, result.PartialFailure)
	assert.Contains(t, result.PartialError, "mutation failed")

	// Verify state shows failed status
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusFailed, storedState.Status)
}
