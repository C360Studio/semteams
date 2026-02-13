package otel

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewSpanCollector(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)

	if sc == nil {
		t.Fatal("expected span collector, got nil")
	}

	if sc.serviceName != "test-service" {
		t.Errorf("expected service name 'test-service', got %q", sc.serviceName)
	}

	if sc.serviceVersion != "1.0.0" {
		t.Errorf("expected service version '1.0.0', got %q", sc.serviceVersion)
	}

	if sc.samplingRate != 1.0 {
		t.Errorf("expected sampling rate 1.0, got %f", sc.samplingRate)
	}
}

func TestSpanCollectorProcessLoopEvents(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()
	now := time.Now()

	// Create loop
	createEvent := AgentEvent{
		Type:      "loop.created",
		LoopID:    "loop-001",
		Timestamp: now,
		EntityID:  "agent.test.001",
		Role:      "architect",
	}
	data, _ := json.Marshal(createEvent)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Verify active span
	stats := sc.Stats()
	if stats["active_spans"] != 1 {
		t.Errorf("expected 1 active span, got %d", stats["active_spans"])
	}
	if stats["spans_created"] != 1 {
		t.Errorf("expected 1 span created, got %d", stats["spans_created"])
	}

	// Complete loop
	completeEvent := AgentEvent{
		Type:      "loop.completed",
		LoopID:    "loop-001",
		Timestamp: now.Add(5 * time.Second),
		Duration:  5 * time.Second,
	}
	data, _ = json.Marshal(completeEvent)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Verify span completed
	stats = sc.Stats()
	if stats["active_spans"] != 0 {
		t.Errorf("expected 0 active spans, got %d", stats["active_spans"])
	}
	if stats["spans_completed"] != 1 {
		t.Errorf("expected 1 span completed, got %d", stats["spans_completed"])
	}
	if stats["pending_spans"] != 1 {
		t.Errorf("expected 1 pending span, got %d", stats["pending_spans"])
	}

	// Flush and verify span data
	spans := sc.FlushCompleted()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name != "agent.loop" {
		t.Errorf("expected span name 'agent.loop', got %q", span.Name)
	}
	if span.Status.Code != "ok" {
		t.Errorf("expected status 'ok', got %q", span.Status.Code)
	}
	if span.Attributes["agent.loop_id"] != "loop-001" {
		t.Errorf("expected loop_id 'loop-001', got %v", span.Attributes["agent.loop_id"])
	}
}

func TestSpanCollectorProcessTaskEvents(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()
	now := time.Now()

	// Create parent loop first
	createLoop := AgentEvent{
		Type:      "loop.created",
		LoopID:    "loop-002",
		Timestamp: now,
	}
	data, _ := json.Marshal(createLoop)
	_ = sc.ProcessEvent(ctx, data)

	// Start task
	startTask := AgentEvent{
		Type:      "task.started",
		LoopID:    "loop-002",
		TaskID:    "task-001",
		Timestamp: now.Add(time.Second),
	}
	data, _ = json.Marshal(startTask)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Verify task span created
	stats := sc.Stats()
	if stats["active_spans"] != 2 {
		t.Errorf("expected 2 active spans, got %d", stats["active_spans"])
	}

	// Complete task
	completeTask := AgentEvent{
		Type:      "task.completed",
		LoopID:    "loop-002",
		TaskID:    "task-001",
		Timestamp: now.Add(2 * time.Second),
		Duration:  time.Second,
	}
	data, _ = json.Marshal(completeTask)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Complete loop
	completeLoop := AgentEvent{
		Type:      "loop.completed",
		LoopID:    "loop-002",
		Timestamp: now.Add(3 * time.Second),
	}
	data, _ = json.Marshal(completeLoop)
	_ = sc.ProcessEvent(ctx, data)

	// Verify spans
	spans := sc.FlushCompleted()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Find task span
	var taskSpan *SpanData
	for _, s := range spans {
		if s.Name == "agent.task" {
			taskSpan = s
			break
		}
	}

	if taskSpan == nil {
		t.Fatal("task span not found")
	}

	if taskSpan.ParentSpanID == "" {
		t.Error("expected task span to have parent")
	}
	if taskSpan.Attributes["agent.task_id"] != "task-001" {
		t.Errorf("expected task_id 'task-001', got %v", taskSpan.Attributes["agent.task_id"])
	}
}

func TestSpanCollectorProcessToolEvents(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()
	now := time.Now()

	// Create parent loop
	createLoop := AgentEvent{
		Type:      "loop.created",
		LoopID:    "loop-003",
		Timestamp: now,
	}
	data, _ := json.Marshal(createLoop)
	_ = sc.ProcessEvent(ctx, data)

	// Start tool
	startTool := AgentEvent{
		Type:      "tool.started",
		LoopID:    "loop-003",
		ToolName:  "code_search",
		Timestamp: now.Add(time.Second),
	}
	data, _ = json.Marshal(startTool)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Complete tool
	completeTool := AgentEvent{
		Type:      "tool.completed",
		LoopID:    "loop-003",
		ToolName:  "code_search",
		Timestamp: now.Add(2 * time.Second),
		Duration:  time.Second,
	}
	data, _ = json.Marshal(completeTool)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Complete loop
	completeLoop := AgentEvent{
		Type:      "loop.completed",
		LoopID:    "loop-003",
		Timestamp: now.Add(3 * time.Second),
	}
	data, _ = json.Marshal(completeLoop)
	_ = sc.ProcessEvent(ctx, data)

	// Verify spans
	spans := sc.FlushCompleted()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Find tool span
	var toolSpan *SpanData
	for _, s := range spans {
		if s.Name == "agent.tool.code_search" {
			toolSpan = s
			break
		}
	}

	if toolSpan == nil {
		t.Fatal("tool span not found")
	}

	if toolSpan.Kind != "client" {
		t.Errorf("expected tool span kind 'client', got %q", toolSpan.Kind)
	}
	if toolSpan.Attributes["tool.name"] != "code_search" {
		t.Errorf("expected tool.name 'code_search', got %v", toolSpan.Attributes["tool.name"])
	}
}

func TestSpanCollectorFailedSpans(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()
	now := time.Now()

	// Create loop
	createLoop := AgentEvent{
		Type:      "loop.created",
		LoopID:    "loop-004",
		Timestamp: now,
	}
	data, _ := json.Marshal(createLoop)
	_ = sc.ProcessEvent(ctx, data)

	// Fail loop
	failLoop := AgentEvent{
		Type:      "loop.failed",
		LoopID:    "loop-004",
		Timestamp: now.Add(time.Second),
		Error:     "LLM timeout",
	}
	data, _ = json.Marshal(failLoop)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	spans := sc.FlushCompleted()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status.Code != "error" {
		t.Errorf("expected status 'error', got %q", span.Status.Code)
	}
	if span.Status.Message != "LLM timeout" {
		t.Errorf("expected status message 'LLM timeout', got %q", span.Status.Message)
	}
}

func TestSpanCollectorAddEventToSpan(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()
	now := time.Now()

	// Create loop
	createLoop := AgentEvent{
		Type:      "loop.created",
		LoopID:    "loop-005",
		Timestamp: now,
	}
	data, _ := json.Marshal(createLoop)
	_ = sc.ProcessEvent(ctx, data)

	// Add unknown event (should be added as span event)
	unknownEvent := AgentEvent{
		Type:      "model.response.received",
		LoopID:    "loop-005",
		Timestamp: now.Add(time.Second),
		Metadata: map[string]any{
			"tokens": 150,
		},
	}
	data, _ = json.Marshal(unknownEvent)
	if err := sc.ProcessEvent(ctx, data); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Complete loop
	completeLoop := AgentEvent{
		Type:      "loop.completed",
		LoopID:    "loop-005",
		Timestamp: now.Add(2 * time.Second),
	}
	data, _ = json.Marshal(completeLoop)
	_ = sc.ProcessEvent(ctx, data)

	spans := sc.FlushCompleted()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if len(span.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(span.Events))
	}

	event := span.Events[0]
	if event.Name != "model.response.received" {
		t.Errorf("expected event name 'model.response.received', got %q", event.Name)
	}
}

func TestSpanCollectorFlushCompleted(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()
	now := time.Now()

	// Create and complete multiple loops
	for i := 0; i < 3; i++ {
		loopID := "loop-batch-" + string(rune('a'+i))
		create := AgentEvent{
			Type:      "loop.created",
			LoopID:    loopID,
			Timestamp: now,
		}
		data, _ := json.Marshal(create)
		_ = sc.ProcessEvent(ctx, data)

		complete := AgentEvent{
			Type:      "loop.completed",
			LoopID:    loopID,
			Timestamp: now.Add(time.Second),
		}
		data, _ = json.Marshal(complete)
		_ = sc.ProcessEvent(ctx, data)
	}

	// First flush
	spans := sc.FlushCompleted()
	if len(spans) != 3 {
		t.Errorf("expected 3 spans, got %d", len(spans))
	}

	// Second flush should be empty
	spans = sc.FlushCompleted()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans after second flush, got %d", len(spans))
	}
}

func TestGenerateTraceID(t *testing.T) {
	traceID := generateTraceID("loop-001")

	if len(traceID) != 32 {
		t.Errorf("expected trace ID length 32, got %d", len(traceID))
	}

	// Should be deterministic
	traceID2 := generateTraceID("loop-001")
	if traceID != traceID2 {
		t.Errorf("expected deterministic trace ID, got %q and %q", traceID, traceID2)
	}

	// Different input should give different ID
	traceID3 := generateTraceID("loop-002")
	if traceID == traceID3 {
		t.Error("expected different trace IDs for different inputs")
	}
}

func TestGenerateSpanID(t *testing.T) {
	spanID := generateSpanID("span-001")

	if len(spanID) != 16 {
		t.Errorf("expected span ID length 16, got %d", len(spanID))
	}

	// Should be deterministic
	spanID2 := generateSpanID("span-001")
	if spanID != spanID2 {
		t.Errorf("expected deterministic span ID, got %q and %q", spanID, spanID2)
	}
}

func TestSpanCollectorInvalidJSON(t *testing.T) {
	sc := NewSpanCollector("test-service", "1.0.0", 1.0)
	ctx := context.Background()

	err := sc.ProcessEvent(ctx, []byte("not valid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
