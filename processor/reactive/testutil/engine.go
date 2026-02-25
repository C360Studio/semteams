package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/reactive"
)

// TestEngine wraps a reactive workflow engine with test helpers.
type TestEngine struct {
	t        *testing.T
	kv       *InMemoryKV
	bus      *InMemoryBus
	registry *reactive.WorkflowRegistry
}

// NewTestEngine creates a new test engine with in-memory KV and bus.
func NewTestEngine(t *testing.T) *TestEngine {
	t.Helper()

	return &TestEngine{
		t:        t,
		kv:       NewInMemoryKV("TEST_WORKFLOW_STATE"),
		bus:      NewInMemoryBus(),
		registry: reactive.NewWorkflowRegistry(nil),
	}
}

// KV returns the in-memory KV store.
func (e *TestEngine) KV() *InMemoryKV {
	return e.kv
}

// Bus returns the in-memory message bus.
func (e *TestEngine) Bus() *InMemoryBus {
	return e.bus
}

// Registry returns the workflow registry.
func (e *TestEngine) Registry() *reactive.WorkflowRegistry {
	return e.registry
}

// RegisterWorkflow registers a workflow definition.
func (e *TestEngine) RegisterWorkflow(def *reactive.Definition) error {
	return e.registry.Register(def)
}

// TriggerKV simulates a KV state change and triggers any matching rules.
func (e *TestEngine) TriggerKV(ctx context.Context, key string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	_, err = e.kv.Put(ctx, key, data)
	return err
}

// TriggerKVUpdate simulates a KV update with revision check.
func (e *TestEngine) TriggerKVUpdate(ctx context.Context, key string, state any, revision uint64) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	_, err = e.kv.Update(ctx, key, data, revision)
	return err
}

// TriggerMessage publishes a message to trigger subject-based rules.
func (e *TestEngine) TriggerMessage(ctx context.Context, subject string, payload message.Payload, source string) error {
	baseMsg := message.NewBaseMessage(payload.Schema(), payload, source)
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	return e.bus.Publish(ctx, subject, data)
}

// HandleCallback simulates receiving an async callback result.
func (e *TestEngine) HandleCallback(ctx context.Context, key string, result *reactive.AsyncStepResult) error {
	// Get the current state
	entry, err := e.kv.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}

	// Deserialize to get execution ID
	var rawState map[string]any
	if err := json.Unmarshal(entry.Value(), &rawState); err != nil {
		return fmt.Errorf("unmarshal state: %w", err)
	}

	workflowID, _ := rawState["workflow_id"].(string)

	// Build callback subject
	subject := fmt.Sprintf("workflow.callback.%s.%s", workflowID, result.TaskID)

	// Publish the callback result
	baseMsg := message.NewBaseMessage(result.Schema(), result, "test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("marshal callback: %w", err)
	}

	return e.bus.Publish(ctx, subject, data)
}

// AssertPhase verifies the execution phase matches the expected value.
func (e *TestEngine) AssertPhase(key, expectedPhase string) {
	e.t.Helper()

	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		e.t.Fatalf("AssertPhase: failed to get state for key %q: %v", key, err)
	}

	var state struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		e.t.Fatalf("AssertPhase: failed to unmarshal state: %v", err)
	}

	if state.Phase != expectedPhase {
		e.t.Errorf("AssertPhase: expected phase %q, got %q", expectedPhase, state.Phase)
	}
}

// AssertStatus verifies the execution status matches the expected value.
func (e *TestEngine) AssertStatus(key string, expectedStatus reactive.ExecutionStatus) {
	e.t.Helper()

	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		e.t.Fatalf("AssertStatus: failed to get state for key %q: %v", key, err)
	}

	var state struct {
		Status reactive.ExecutionStatus `json:"status"`
	}
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		e.t.Fatalf("AssertStatus: failed to unmarshal state: %v", err)
	}

	if state.Status != expectedStatus {
		e.t.Errorf("AssertStatus: expected status %q, got %q", expectedStatus, state.Status)
	}
}

// AssertIteration verifies the iteration count matches the expected value.
func (e *TestEngine) AssertIteration(key string, expectedIteration int) {
	e.t.Helper()

	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		e.t.Fatalf("AssertIteration: failed to get state for key %q: %v", key, err)
	}

	var state struct {
		Iteration int `json:"iteration"`
	}
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		e.t.Fatalf("AssertIteration: failed to unmarshal state: %v", err)
	}

	if state.Iteration != expectedIteration {
		e.t.Errorf("AssertIteration: expected iteration %d, got %d", expectedIteration, state.Iteration)
	}
}

// AssertState verifies the state using a custom assertion function.
func (e *TestEngine) AssertState(key string, assertFn func(t *testing.T, data []byte)) {
	e.t.Helper()

	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		e.t.Fatalf("AssertState: failed to get state for key %q: %v", key, err)
	}

	assertFn(e.t, entry.Value())
}

// AssertStateAs unmarshals state into the given type and runs assertions.
func (e *TestEngine) AssertStateAs(key string, state any, assertFn func(t *testing.T, state any)) {
	e.t.Helper()

	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		e.t.Fatalf("AssertStateAs: failed to get state for key %q: %v", key, err)
	}

	if err := json.Unmarshal(entry.Value(), state); err != nil {
		e.t.Fatalf("AssertStateAs: failed to unmarshal state: %v", err)
	}

	assertFn(e.t, state)
}

// AssertPublished verifies that a message was published to a subject pattern.
func (e *TestEngine) AssertPublished(subjectPattern string) {
	e.t.Helper()

	if !e.bus.HasMessage(subjectPattern) {
		e.t.Errorf("AssertPublished: no message found for pattern %q", subjectPattern)
	}
}

// AssertPublishedCount verifies the number of messages published to a subject pattern.
func (e *TestEngine) AssertPublishedCount(subjectPattern string, expectedCount int) {
	e.t.Helper()

	actualCount := e.bus.CountForSubject(subjectPattern)
	if actualCount != expectedCount {
		e.t.Errorf("AssertPublishedCount: expected %d messages for %q, got %d", expectedCount, subjectPattern, actualCount)
	}
}

// AssertPublishedWithType verifies that a message with specific type was published.
func (e *TestEngine) AssertPublishedWithType(subjectPattern string, msgType message.Type) {
	e.t.Helper()

	if !e.bus.HasMessageWithType(subjectPattern, msgType) {
		e.t.Errorf("AssertPublishedWithType: no message with type %v found for pattern %q", msgType, subjectPattern)
	}
}

// AssertNoPublished verifies that no messages were published to a subject pattern.
func (e *TestEngine) AssertNoPublished(subjectPattern string) {
	e.t.Helper()

	if e.bus.HasMessage(subjectPattern) {
		e.t.Errorf("AssertNoPublished: unexpected message found for pattern %q", subjectPattern)
	}
}

// AssertPayload verifies the payload of the last message to a subject pattern.
func (e *TestEngine) AssertPayload(subjectPattern string, assertFn func(t *testing.T, payload any)) {
	e.t.Helper()

	msg := e.bus.LastMessageForSubject(subjectPattern)
	if msg == nil {
		e.t.Fatalf("AssertPayload: no message found for pattern %q", subjectPattern)
	}

	var baseMsg message.BaseMessage
	if err := json.Unmarshal(msg.Data, &baseMsg); err != nil {
		e.t.Fatalf("AssertPayload: failed to unmarshal message: %v", err)
	}

	assertFn(e.t, baseMsg.Payload())
}

// GetState retrieves the current state for a key.
func (e *TestEngine) GetState(key string) ([]byte, error) {
	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return entry.Value(), nil
}

// GetStateAs retrieves and unmarshals the state into the given type.
func (e *TestEngine) GetStateAs(key string, state any) error {
	data, err := e.GetState(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, state)
}

// WaitForPhase waits for the execution to reach a specific phase.
func (e *TestEngine) WaitForPhase(key, expectedPhase string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entry, err := e.kv.Get(context.Background(), key)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var state struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(entry.Value(), &state); err != nil {
			return fmt.Errorf("unmarshal state: %w", err)
		}

		if state.Phase == expectedPhase {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for phase %q", expectedPhase)
}

// WaitForStatus waits for the execution to reach a specific status.
func (e *TestEngine) WaitForStatus(key string, expectedStatus reactive.ExecutionStatus, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entry, err := e.kv.Get(context.Background(), key)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var state struct {
			Status reactive.ExecutionStatus `json:"status"`
		}
		if err := json.Unmarshal(entry.Value(), &state); err != nil {
			return fmt.Errorf("unmarshal state: %w", err)
		}

		if state.Status == expectedStatus {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for status %q", expectedStatus)
}

// WaitForPublished waits for a message to be published to a subject pattern.
func (e *TestEngine) WaitForPublished(subjectPattern string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.bus.HasMessage(subjectPattern) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for message on %q", subjectPattern)
}

// Clear resets the test engine state.
func (e *TestEngine) Clear() {
	e.kv.Clear()
	e.bus.Clear()
}

// PrintState prints the current state for debugging.
func (e *TestEngine) PrintState(key string) {
	e.t.Helper()

	entry, err := e.kv.Get(context.Background(), key)
	if err != nil {
		e.t.Logf("State for %q: (not found)", key)
		return
	}

	var state map[string]any
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		e.t.Logf("State for %q: %s", key, entry.Value())
		return
	}

	formatted, _ := json.MarshalIndent(state, "", "  ")
	e.t.Logf("State for %q:\n%s", key, formatted)
}

// PrintMessages prints all published messages for debugging.
func (e *TestEngine) PrintMessages() {
	e.t.Helper()

	msgs := e.bus.Messages()
	e.t.Logf("Published messages (%d total):", len(msgs))
	for i, msg := range msgs {
		e.t.Logf("  [%d] %s: %s", i, msg.Subject, msg.Data)
	}
}
