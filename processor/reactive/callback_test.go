package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Register test payload types for unmarshaling
	_ = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "test",
		Category:    "callback",
		Version:     "v1",
		Description: "Test callback payload",
		Factory:     func() any { return &TestCallbackPayload{} },
	})

	_ = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "async-result",
		Version:     "v1",
		Description: "Async step result",
		Factory:     func() any { return &AsyncStepResult{} },
	})

	_ = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "test",
		Category:    "payload",
		Version:     "v1",
		Description: "Test payload",
		Factory:     func() any { return &TestPayload{} },
	})
}

// mockJetStreamMsg implements jetstream.Msg for testing.
type mockJetStreamMsg struct {
	subject string
	data    []byte
	acked   bool
	naked   bool
	termed  bool
	mu      sync.Mutex
}

func (m *mockJetStreamMsg) Subject() string { return m.subject }
func (m *mockJetStreamMsg) Reply() string   { return "" }
func (m *mockJetStreamMsg) Data() []byte    { return m.data }
func (m *mockJetStreamMsg) Headers() nats.Header {
	return make(nats.Header)
}
func (m *mockJetStreamMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{
		Sequence: jetstream.SequencePair{
			Stream:   1,
			Consumer: 1,
		},
		NumDelivered: 1,
	}, nil
}

func (m *mockJetStreamMsg) Ack() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked = true
	return nil
}

func (m *mockJetStreamMsg) DoubleAck(ctx context.Context) error {
	return m.Ack()
}

func (m *mockJetStreamMsg) Nak() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.naked = true
	return nil
}

func (m *mockJetStreamMsg) NakWithDelay(delay time.Duration) error {
	return m.Nak()
}

func (m *mockJetStreamMsg) InProgress() error {
	return nil
}

func (m *mockJetStreamMsg) Term() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.termed = true
	return nil
}

func (m *mockJetStreamMsg) TermWithReason(reason string) error {
	return m.Term()
}

func (m *mockJetStreamMsg) wasAcked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acked
}

func (m *mockJetStreamMsg) wasNaked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.naked
}

func (m *mockJetStreamMsg) wasTermed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.termed
}

// TestCallbackPayload implements message.Payload for testing.
type TestCallbackPayload struct {
	TaskID  string `json:"task_id"`
	Content string `json:"content"`
}

func (p *TestCallbackPayload) Schema() message.Type {
	return message.Type{
		Domain:   "test",
		Category: "callback",
		Version:  "v1",
	}
}

func (p *TestCallbackPayload) Validate() error {
	return nil
}

func (p *TestCallbackPayload) MarshalJSON() ([]byte, error) {
	type Alias TestCallbackPayload
	return json.Marshal((*Alias)(p))
}

func (p *TestCallbackPayload) UnmarshalJSON(data []byte) error {
	type Alias TestCallbackPayload
	return json.Unmarshal(data, (*Alias)(p))
}

func (p *TestCallbackPayload) GetTaskID() string {
	return p.TaskID
}

func newTestCallbackHandler() (*CallbackHandler, *mockPublisher, *mockStateStore) {
	logger := slog.Default()
	publisher := &mockPublisher{}
	store := newMockStateStore()
	consumer := NewSubjectConsumer(logger)
	dispatcher := NewDispatcher(logger,
		WithPublisher(publisher),
		WithStateStore(store),
	)

	handler := NewCallbackHandler(logger, consumer, dispatcher, store)
	return handler, publisher, store
}

func TestCallbackHandler_RegisterAndUnregisterTask(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	reg := &TaskRegistration{
		TaskID:             "task-123",
		ExecutionKey:       "exec-abc",
		ExecutionID:        "exec-abc",
		WorkflowID:         "workflow-test",
		RuleID:             "rule-1",
		ExpectedResultType: "test.result.v1",
		RegisteredAt:       time.Now(),
	}

	rule := &RuleDef{ID: "rule-1"}
	def := newTestDefinition()

	// Register
	handler.RegisterTask(reg, rule, def)

	// Verify registration
	retrieved := handler.GetTaskRegistration("task-123")
	require.NotNil(t, retrieved)
	assert.Equal(t, "task-123", retrieved.TaskID)
	assert.Equal(t, "exec-abc", retrieved.ExecutionKey)
	assert.Equal(t, 1, handler.PendingTaskCount())

	// Unregister
	handler.UnregisterTask("task-123")

	// Verify unregistration
	retrieved = handler.GetTaskRegistration("task-123")
	assert.Nil(t, retrieved)
	assert.Equal(t, 0, handler.PendingTaskCount())
}

func TestCallbackHandler_GetTaskRegistration_NotFound(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	retrieved := handler.GetTaskRegistration("nonexistent")
	assert.Nil(t, retrieved)
}

func TestCallbackHandler_CleanupExpiredTasks(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	// Register expired task
	handler.RegisterTask(&TaskRegistration{
		TaskID:       "expired-task",
		ExecutionKey: "exec-1",
		Timeout:      &past,
	}, &RuleDef{ID: "rule-1"}, newTestDefinition())

	// Register active task
	handler.RegisterTask(&TaskRegistration{
		TaskID:       "active-task",
		ExecutionKey: "exec-2",
		Timeout:      &future,
	}, &RuleDef{ID: "rule-2"}, newTestDefinition())

	// Register task without timeout
	handler.RegisterTask(&TaskRegistration{
		TaskID:       "no-timeout-task",
		ExecutionKey: "exec-3",
	}, &RuleDef{ID: "rule-3"}, newTestDefinition())

	assert.Equal(t, 3, handler.PendingTaskCount())

	// Cleanup expired
	cleaned := handler.CleanupExpiredTasks()
	assert.Equal(t, 1, cleaned)
	assert.Equal(t, 2, handler.PendingTaskCount())

	// Verify correct tasks remain
	assert.Nil(t, handler.GetTaskRegistration("expired-task"))
	assert.NotNil(t, handler.GetTaskRegistration("active-task"))
	assert.NotNil(t, handler.GetTaskRegistration("no-timeout-task"))
}

func TestCallbackHandler_ExtractTaskID_FromAsyncStepResult(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	payload := &AsyncStepResult{
		TaskID: "task-from-result",
		Status: "success",
	}

	taskID, err := handler.extractTaskID(payload)
	require.NoError(t, err)
	assert.Equal(t, "task-from-result", taskID)
}

func TestCallbackHandler_ExtractTaskID_FromTaskIDExtractor(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	payload := &TestCallbackPayload{
		TaskID:  "task-from-extractor",
		Content: "test",
	}

	taskID, err := handler.extractTaskID(payload)
	require.NoError(t, err)
	assert.Equal(t, "task-from-extractor", taskID)
}

func TestCallbackHandler_ExtractTaskID_FromJSONField(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	// A payload that doesn't implement TaskIDExtractor but has task_id field
	payload := &TestPayload{
		TaskID:  "task-from-json",
		Content: "test",
	}

	taskID, err := handler.extractTaskID(payload)
	require.NoError(t, err)
	assert.Equal(t, "task-from-json", taskID)
}

func TestCallbackHandler_ExtractTaskID_NotFound(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	// A payload without any task ID field
	payload := &WorkflowCompletionEvent{
		ExecutionID: "exec-1",
		WorkflowID:  "workflow-1",
	}

	_, err := handler.extractTaskID(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no task ID found")
}

func TestCallbackHandler_HandleCallbackMessage_Success(t *testing.T) {
	handler, _, store := newTestCallbackHandler()
	defer handler.Stop()

	// Set up execution state in store
	state := newTestDispatcherState()
	state.Status = StatusWaiting
	state.PendingTaskID = "task-123"
	stateData, _ := json.Marshal(state)
	rev, _ := store.Put(context.Background(), state.ID, stateData)

	// Register the task
	handler.RegisterTask(&TaskRegistration{
		TaskID:             "task-123",
		ExecutionKey:       state.ID,
		ExecutionID:        state.ID,
		WorkflowID:         "workflow-test",
		RuleID:             "async-rule",
		ExpectedResultType: "test.callback.v1",
	}, &RuleDef{
		ID: "async-rule",
		Action: Action{
			Type:               ActionPublishAsync,
			ExpectedResultType: "test.callback.v1",
			MutateState: func(ctx *RuleContext, result any) error {
				s := ctx.State.(*TestDispatcherState)
				s.Phase = "callback-received"
				s.CustomField = "updated-by-callback"
				return nil
			},
		},
	}, newTestDefinition())

	// Create callback message
	callbackPayload := &TestCallbackPayload{
		TaskID:  "task-123",
		Content: "callback-data",
	}
	baseMsg := message.NewBaseMessage(callbackPayload.Schema(), callbackPayload, "test")
	msgData, _ := json.Marshal(baseMsg)

	event := SubjectMessageEvent{
		Subject:   "workflow.callback.workflow-test." + state.ID,
		Data:      msgData,
		Timestamp: time.Now(),
	}

	msg := &mockJetStreamMsg{
		subject: event.Subject,
		data:    msgData,
	}

	// Process the callback
	handler.handleCallbackMessage(context.Background(), event, msg)

	// Verify message was acked
	assert.True(t, msg.wasAcked())

	// Verify task was unregistered
	assert.Nil(t, handler.GetTaskRegistration("task-123"))

	// Verify state was updated
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusRunning, storedState.Status) // Cleared from waiting
	assert.Equal(t, "callback-received", storedState.Phase)
	assert.Equal(t, "updated-by-callback", storedState.CustomField)

	// Verify revision increased
	entry, _ := store.Get(context.Background(), state.ID)
	assert.Greater(t, entry.Revision(), rev)
}

func TestCallbackHandler_HandleCallbackMessage_UnknownTask(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	// Don't register any task

	callbackPayload := &TestCallbackPayload{
		TaskID:  "unknown-task",
		Content: "data",
	}
	baseMsg := message.NewBaseMessage(callbackPayload.Schema(), callbackPayload, "test")
	msgData, _ := json.Marshal(baseMsg)

	event := SubjectMessageEvent{
		Subject:   "workflow.callback.workflow-test.exec-1",
		Data:      msgData,
		Timestamp: time.Now(),
	}

	msg := &mockJetStreamMsg{
		subject: event.Subject,
		data:    msgData,
	}

	// Process the callback - should ack (task may have already been processed)
	handler.handleCallbackMessage(context.Background(), event, msg)

	assert.True(t, msg.wasAcked())
}

func TestCallbackHandler_HandleCallbackMessage_InvalidJSON(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	event := SubjectMessageEvent{
		Subject:   "workflow.callback.workflow-test.exec-1",
		Data:      []byte("invalid json"),
		Timestamp: time.Now(),
	}

	msg := &mockJetStreamMsg{
		subject: event.Subject,
		data:    event.Data,
	}

	// Process the callback - should nak
	handler.handleCallbackMessage(context.Background(), event, msg)

	assert.True(t, msg.wasNaked())
}

func TestCallbackHandler_HandleCallbackMessage_FailedCallback(t *testing.T) {
	handler, _, store := newTestCallbackHandler()
	defer handler.Stop()

	// Set up execution state in store
	state := newTestDispatcherState()
	state.Status = StatusWaiting
	state.PendingTaskID = "task-fail"
	stateData, _ := json.Marshal(state)
	store.Put(context.Background(), state.ID, stateData)

	def := newTestDefinition()
	def.Events.OnFail = "test.workflow.failed"

	// Register the task
	handler.RegisterTask(&TaskRegistration{
		TaskID:       "task-fail",
		ExecutionKey: state.ID,
		ExecutionID:  state.ID,
		WorkflowID:   "workflow-test",
		RuleID:       "async-rule",
	}, &RuleDef{
		ID: "async-rule",
		Action: Action{
			Type: ActionPublishAsync,
		},
	}, def)

	// Create failed callback using AsyncStepResult
	callbackPayload := &AsyncStepResult{
		TaskID: "task-fail",
		Status: "failed",
		Error:  "external service error",
	}
	baseMsg := message.NewBaseMessage(callbackPayload.Schema(), callbackPayload, "test")
	msgData, _ := json.Marshal(baseMsg)

	event := SubjectMessageEvent{
		Subject:   "workflow.callback.workflow-test." + state.ID,
		Data:      msgData,
		Timestamp: time.Now(),
	}

	msg := &mockJetStreamMsg{
		subject: event.Subject,
		data:    msgData,
	}

	// Process the callback
	handler.handleCallbackMessage(context.Background(), event, msg)

	// Verify message was acked
	assert.True(t, msg.wasAcked())

	// Verify task was unregistered
	assert.Nil(t, handler.GetTaskRegistration("task-fail"))

	// Verify state shows failure
	storedData, _ := store.getData(state.ID)
	var storedState TestDispatcherState
	json.Unmarshal(storedData, &storedState)
	assert.Equal(t, StatusFailed, storedState.Status)
	assert.Equal(t, "external service error", storedState.Error)
}

func TestCallbackHandler_Stop(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()

	// Register some tasks
	handler.RegisterTask(&TaskRegistration{
		TaskID:       "task-1",
		ExecutionKey: "exec-1",
	}, &RuleDef{ID: "rule-1"}, newTestDefinition())

	// Stop should not panic
	handler.Stop()

	// Multiple stops should be safe
	handler.Stop()
	handler.Stop()
}

func TestCallbackHandler_GetMetrics(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	// Initially no tasks
	metrics := handler.GetMetrics()
	assert.Equal(t, 0, metrics.PendingTasks)

	// Add some tasks
	handler.RegisterTask(&TaskRegistration{
		TaskID:       "task-1",
		ExecutionKey: "exec-1",
	}, &RuleDef{ID: "rule-1"}, newTestDefinition())

	handler.RegisterTask(&TaskRegistration{
		TaskID:       "task-2",
		ExecutionKey: "exec-2",
	}, &RuleDef{ID: "rule-2"}, newTestDefinition())

	metrics = handler.GetMetrics()
	assert.Equal(t, 2, metrics.PendingTasks)
}

func TestBuildCallbackSubjectPattern(t *testing.T) {
	pattern := BuildCallbackSubjectPattern("plan-review")
	assert.Equal(t, "workflow.callback.plan-review.>", pattern)
}

func TestParseCallbackSubject(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		wantWfID    string
		wantExecID  string
		expectError bool
	}{
		{
			name:       "valid subject",
			subject:    "workflow.callback.plan-review.exec-123",
			wantWfID:   "plan-review",
			wantExecID: "exec-123",
		},
		{
			name:       "valid subject with extra parts",
			subject:    "workflow.callback.my-workflow.exec.extra",
			wantWfID:   "my-workflow",
			wantExecID: "exec",
		},
		{
			name:        "too short",
			subject:     "workflow.callback",
			expectError: true,
		},
		{
			name:        "wrong prefix",
			subject:     "events.callback.workflow.exec",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wfID, execID, err := ParseCallbackSubject(tt.subject)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantWfID, wfID)
				assert.Equal(t, tt.wantExecID, execID)
			}
		})
	}
}

func TestDefaultCallbackConsumerConfig(t *testing.T) {
	config := DefaultCallbackConsumerConfig("plan-review")

	assert.Equal(t, "WORKFLOW_CALLBACKS", config.StreamName)
	assert.Equal(t, "callback-plan-review", config.ConsumerName)
	assert.Equal(t, "workflow.callback.plan-review.>", config.SubjectPattern)
}

func TestCallbackError(t *testing.T) {
	err := &CallbackError{
		TaskID:  "task-123",
		Message: "processing failed",
	}
	assert.Equal(t, "callback error for task task-123: processing failed", err.Error())

	errNoTask := &CallbackError{
		Message: "no task ID",
	}
	assert.Equal(t, "callback error: no task ID", errNoTask.Error())
}

func TestCallbackError_Unwrap(t *testing.T) {
	cause := &ValidationError{Field: "test", Message: "invalid"}
	err := &CallbackError{
		TaskID:  "task-123",
		Message: "failed",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, cause, unwrapped)
}

func TestAsyncStepResult_Schema(t *testing.T) {
	result := &AsyncStepResult{
		TaskID: "task-1",
		Status: "success",
	}

	schema := result.Schema()
	assert.Equal(t, "workflow", schema.Domain)
	assert.Equal(t, "async-result", schema.Category)
	assert.Equal(t, "v1", schema.Version)
}

func TestAsyncStepResult_GetTaskID(t *testing.T) {
	result := &AsyncStepResult{
		TaskID: "task-abc",
		Status: "success",
	}

	assert.Equal(t, "task-abc", result.GetTaskID())
}

func TestAsyncStepResult_Validate(t *testing.T) {
	// Valid
	valid := &AsyncStepResult{
		TaskID: "task-1",
		Status: "success",
	}
	assert.NoError(t, valid.Validate())

	// Missing task ID
	noTaskID := &AsyncStepResult{
		Status: "success",
	}
	assert.Error(t, noTaskID.Validate())

	// Missing status
	noStatus := &AsyncStepResult{
		TaskID: "task-1",
	}
	assert.Error(t, noStatus.Validate())
}

func TestAsyncStepResult_JSONRoundTrip(t *testing.T) {
	original := &AsyncStepResult{
		TaskID:      "task-123",
		ExecutionID: "exec-456",
		Status:      "success",
		Output:      json.RawMessage(`{"key": "value"}`),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded AsyncStepResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.TaskID, decoded.TaskID)
	assert.Equal(t, original.ExecutionID, decoded.ExecutionID)
	assert.Equal(t, original.Status, decoded.Status)
	assert.JSONEq(t, `{"key": "value"}`, string(decoded.Output))
}

func TestCallbackHandler_ConcurrentTaskOperations(t *testing.T) {
	handler, _, _ := newTestCallbackHandler()
	defer handler.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrently register and unregister tasks
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			taskID := "task-" + string(rune('a'+idx%26))

			// Register
			handler.RegisterTask(&TaskRegistration{
				TaskID:       taskID,
				ExecutionKey: "exec-" + taskID,
			}, &RuleDef{ID: "rule-" + taskID}, newTestDefinition())

			// Read
			_ = handler.GetTaskRegistration(taskID)

			// Unregister
			handler.UnregisterTask(taskID)
		}(i)
	}

	wg.Wait()

	// Should not panic and tasks should be cleaned up
	// (some may remain if unregister happened before register)
}
