package natsclient

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestEvent is a simple event type for testing
type TestEvent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Value     int       `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

func TestJSONCodec_Marshal(t *testing.T) {
	codec := JSONCodec[TestEvent]{}
	event := TestEvent{
		ID:        "test-1",
		Name:      "test event",
		Value:     42,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify JSON structure
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal verification failed: %v", err)
	}

	if parsed["id"] != "test-1" {
		t.Errorf("Expected id 'test-1', got %v", parsed["id"])
	}
	if parsed["name"] != "test event" {
		t.Errorf("Expected name 'test event', got %v", parsed["name"])
	}
	if int(parsed["value"].(float64)) != 42 {
		t.Errorf("Expected value 42, got %v", parsed["value"])
	}
}

func TestJSONCodec_Unmarshal(t *testing.T) {
	codec := JSONCodec[TestEvent]{}
	data := []byte(`{"id":"test-2","name":"another event","value":100,"timestamp":"2024-01-02T00:00:00Z"}`)

	var event TestEvent
	if err := codec.Unmarshal(data, &event); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if event.ID != "test-2" {
		t.Errorf("Expected id 'test-2', got %s", event.ID)
	}
	if event.Name != "another event" {
		t.Errorf("Expected name 'another event', got %s", event.Name)
	}
	if event.Value != 100 {
		t.Errorf("Expected value 100, got %d", event.Value)
	}
}

func TestJSONCodec_RoundTrip(t *testing.T) {
	codec := JSONCodec[TestEvent]{}
	original := TestEvent{
		ID:        "round-trip",
		Name:      "test",
		Value:     999,
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}

	data, err := codec.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded TestEvent
	if err := codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: expected %s, got %s", original.ID, decoded.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: expected %s, got %s", original.Name, decoded.Name)
	}
	if decoded.Value != original.Value {
		t.Errorf("Value mismatch: expected %d, got %d", original.Value, decoded.Value)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp mismatch: expected %v, got %v", original.Timestamp, decoded.Timestamp)
	}
}

func TestNewSubject(t *testing.T) {
	subject := NewSubject[TestEvent]("test.events.created")

	if subject.Pattern != "test.events.created" {
		t.Errorf("Expected pattern 'test.events.created', got %s", subject.Pattern)
	}

	// Verify codec is a JSONCodec
	if _, ok := subject.Codec.(JSONCodec[TestEvent]); !ok {
		t.Errorf("Expected JSONCodec, got %T", subject.Codec)
	}
}

func TestSubject_TypeSafety(t *testing.T) {
	// This test verifies the type system - compile-time checks
	// Creating subjects with specific types
	type EventA struct {
		TypeA string `json:"type_a"`
	}
	type EventB struct {
		TypeB int `json:"type_b"`
	}

	subjectA := NewSubject[EventA]("events.a")
	subjectB := NewSubject[EventB]("events.b")

	// Verify each subject only works with its specific type
	// These would fail at compile time if we tried to mix types:
	// subjectA.Publish(ctx, client, EventB{}) // Won't compile
	// subjectB.Publish(ctx, client, EventA{}) // Won't compile

	// Verify codec types match
	_, okA := subjectA.Codec.(JSONCodec[EventA])
	_, okB := subjectB.Codec.(JSONCodec[EventB])

	if !okA {
		t.Error("Subject A should have JSONCodec[EventA]")
	}
	if !okB {
		t.Error("Subject B should have JSONCodec[EventB]")
	}
}

// CustomCodec demonstrates implementing a custom codec
type CustomCodec[T any] struct {
	prefix []byte
}

func (c CustomCodec[T]) Marshal(v T) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(c.prefix, data...), nil
}

func (c CustomCodec[T]) Unmarshal(data []byte, v *T) error {
	// Skip prefix
	return json.Unmarshal(data[len(c.prefix):], v)
}

func TestNewSubjectWithCodec(t *testing.T) {
	customCodec := CustomCodec[TestEvent]{prefix: []byte("PREFIX:")}
	subject := NewSubjectWithCodec("test.custom", customCodec)

	event := TestEvent{ID: "custom-1", Name: "custom", Value: 1}

	data, err := subject.Codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify prefix is present
	if string(data[:7]) != "PREFIX:" {
		t.Errorf("Expected prefix 'PREFIX:', got %s", string(data[:7]))
	}

	var decoded TestEvent
	if err := subject.Codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != "custom-1" {
		t.Errorf("Expected ID 'custom-1', got %s", decoded.ID)
	}
}

// MockClient provides a test double for natsclient.Client
type MockClient struct {
	mu          sync.Mutex
	published   []PublishedMessage
	subscribers map[string][]func(context.Context, []byte)
}

// PublishedMessage records a message that was published through MockClient.
type PublishedMessage struct {
	Subject string
	Data    []byte
}

func NewMockClient() *MockClient {
	return &MockClient{
		subscribers: make(map[string][]func(context.Context, []byte)),
	}
}

func (m *MockClient) Publish(_ context.Context, subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, PublishedMessage{Subject: subject, Data: data})
	return nil
}

func (m *MockClient) GetPublished() []PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]PublishedMessage, len(m.published))
	copy(result, m.published)
	return result
}

func TestSubject_Publish_Pattern(t *testing.T) {
	// Test that Subject creates correct data structure
	subject := NewSubject[TestEvent]("workflow.events.started")
	event := TestEvent{
		ID:    "exec-123",
		Name:  "workflow started",
		Value: 1,
	}

	// Serialize using subject's codec
	data, err := subject.Codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify the data is valid JSON
	var decoded TestEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != "exec-123" {
		t.Errorf("Expected ID 'exec-123', got %s", decoded.ID)
	}
}

// ComplexEvent tests nested structures
type ComplexEvent struct {
	ID       string         `json:"id"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Items    []Item         `json:"items,omitempty"`
}

type Item struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

func TestJSONCodec_ComplexTypes(t *testing.T) {
	codec := JSONCodec[ComplexEvent]{}
	event := ComplexEvent{
		ID: "complex-1",
		Metadata: map[string]any{
			"source":  "test",
			"version": 1,
		},
		Items: []Item{
			{Name: "item1", Value: "string"},
			{Name: "item2", Value: 42},
		},
	}

	data, err := codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ComplexEvent
	if err := codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != "complex-1" {
		t.Errorf("Expected ID 'complex-1', got %s", decoded.ID)
	}
	if decoded.Metadata["source"] != "test" {
		t.Errorf("Expected metadata.source 'test', got %v", decoded.Metadata["source"])
	}
	if len(decoded.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(decoded.Items))
	}
}

func TestSubject_DeclarativePattern(t *testing.T) {
	// Test the declarative pattern for subject definitions
	// This mirrors how subjects would be defined in subjects/workflow.go

	type WorkflowStartedEvent struct {
		ExecutionID string    `json:"execution_id"`
		WorkflowID  string    `json:"workflow_id"`
		StartedAt   time.Time `json:"started_at"`
	}

	type WorkflowCompletedEvent struct {
		ExecutionID string    `json:"execution_id"`
		WorkflowID  string    `json:"workflow_id"`
		CompletedAt time.Time `json:"completed_at"`
		Iterations  int       `json:"iterations"`
	}

	// Define subjects declaratively (simulating package-level vars)
	WorkflowStarted := NewSubject[WorkflowStartedEvent]("workflow.events.started")
	WorkflowCompleted := NewSubject[WorkflowCompletedEvent]("workflow.events.completed")

	// Verify patterns
	if WorkflowStarted.Pattern != "workflow.events.started" {
		t.Errorf("Unexpected pattern: %s", WorkflowStarted.Pattern)
	}
	if WorkflowCompleted.Pattern != "workflow.events.completed" {
		t.Errorf("Unexpected pattern: %s", WorkflowCompleted.Pattern)
	}

	// Verify type-specific encoding
	startedData, err := WorkflowStarted.Codec.Marshal(WorkflowStartedEvent{
		ExecutionID: "exec-1",
		WorkflowID:  "wf-1",
		StartedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Marshal WorkflowStartedEvent failed: %v", err)
	}

	completedData, err := WorkflowCompleted.Codec.Marshal(WorkflowCompletedEvent{
		ExecutionID: "exec-1",
		WorkflowID:  "wf-1",
		CompletedAt: time.Now(),
		Iterations:  3,
	})
	if err != nil {
		t.Fatalf("Marshal WorkflowCompletedEvent failed: %v", err)
	}

	// Verify each has different structure
	var startedMap, completedMap map[string]any
	json.Unmarshal(startedData, &startedMap)
	json.Unmarshal(completedData, &completedMap)

	if _, ok := startedMap["started_at"]; !ok {
		t.Error("WorkflowStartedEvent should have started_at")
	}
	if _, ok := startedMap["iterations"]; ok {
		t.Error("WorkflowStartedEvent should NOT have iterations")
	}

	if _, ok := completedMap["completed_at"]; !ok {
		t.Error("WorkflowCompletedEvent should have completed_at")
	}
	if _, ok := completedMap["iterations"]; !ok {
		t.Error("WorkflowCompletedEvent should have iterations")
	}
}
