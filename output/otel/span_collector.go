package otel

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// SpanData represents collected span information.
type SpanData struct {
	// TraceID is the trace identifier.
	TraceID string `json:"trace_id"`

	// SpanID is the span identifier.
	SpanID string `json:"span_id"`

	// ParentSpanID is the parent span identifier.
	ParentSpanID string `json:"parent_span_id,omitempty"`

	// Name is the span name.
	Name string `json:"name"`

	// Kind is the span kind (client, server, internal, producer, consumer).
	Kind string `json:"kind"`

	// StartTime is when the span started.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the span ended.
	EndTime time.Time `json:"end_time,omitempty"`

	// Status indicates the span status.
	Status SpanStatus `json:"status"`

	// Attributes are span attributes.
	Attributes map[string]any `json:"attributes,omitempty"`

	// Events are span events.
	Events []SpanEvent `json:"events,omitempty"`

	// Links are span links.
	Links []SpanLink `json:"links,omitempty"`
}

// SpanStatus represents the status of a span.
type SpanStatus struct {
	// Code is the status code (unset, ok, error).
	Code string `json:"code"`

	// Message is an optional status message.
	Message string `json:"message,omitempty"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	// Name is the event name.
	Name string `json:"name"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Attributes are event attributes.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanLink represents a link to another span.
type SpanLink struct {
	// TraceID is the linked trace ID.
	TraceID string `json:"trace_id"`

	// SpanID is the linked span ID.
	SpanID string `json:"span_id"`

	// Attributes are link attributes.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// AgentEvent represents an agent lifecycle event from NATS.
type AgentEvent struct {
	// Type is the event type (loop.created, loop.completed, loop.failed, etc.)
	Type string `json:"type"`

	// LoopID is the agent loop identifier.
	LoopID string `json:"loop_id"`

	// TaskID is the task identifier (for task events).
	TaskID string `json:"task_id,omitempty"`

	// ToolName is the tool name (for tool events).
	ToolName string `json:"tool_name,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// EntityID is the agent's entity ID.
	EntityID string `json:"entity_id,omitempty"`

	// Role is the agent's role.
	Role string `json:"role,omitempty"`

	// Error is the error message for failure events.
	Error string `json:"error,omitempty"`

	// Duration is the operation duration (for completion events).
	Duration time.Duration `json:"duration,omitempty"`

	// Metadata contains additional event metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SpanCollector collects spans from agent events.
type SpanCollector struct {
	mu sync.RWMutex

	// Active spans indexed by loop/task ID
	activeSpans map[string]*SpanData

	// Completed spans ready for export
	completedSpans []*SpanData

	// Service information
	serviceName    string
	serviceVersion string

	// Sampling
	samplingRate float64

	// Counters
	spansCreated   int64
	spansCompleted int64
	spansDropped   int64
}

// NewSpanCollector creates a new span collector.
func NewSpanCollector(serviceName, serviceVersion string, samplingRate float64) *SpanCollector {
	return &SpanCollector{
		activeSpans:    make(map[string]*SpanData),
		completedSpans: make([]*SpanData, 0),
		serviceName:    serviceName,
		serviceVersion: serviceVersion,
		samplingRate:   samplingRate,
	}
}

// ProcessEvent processes an agent event and creates/updates spans.
func (sc *SpanCollector) ProcessEvent(_ context.Context, data []byte) error {
	var event AgentEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}

	switch event.Type {
	case "loop.created":
		sc.startLoopSpan(&event)
	case "loop.completed":
		sc.endLoopSpan(&event, "ok", "")
	case "loop.failed":
		sc.endLoopSpan(&event, "error", event.Error)
	case "task.started":
		sc.startTaskSpan(&event)
	case "task.completed":
		sc.endTaskSpan(&event, "ok", "")
	case "task.failed":
		sc.endTaskSpan(&event, "error", event.Error)
	case "tool.started":
		sc.startToolSpan(&event)
	case "tool.completed":
		sc.endToolSpan(&event, "ok", "")
	case "tool.failed":
		sc.endToolSpan(&event, "error", event.Error)
	default:
		// Add as event to parent span
		sc.addEventToSpan(&event)
	}

	return nil
}

// startLoopSpan creates a new root span for an agent loop.
func (sc *SpanCollector) startLoopSpan(event *AgentEvent) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	span := &SpanData{
		TraceID:   generateTraceID(event.LoopID),
		SpanID:    generateSpanID(event.LoopID),
		Name:      "agent.loop",
		Kind:      "server",
		StartTime: event.Timestamp,
		Status:    SpanStatus{Code: "unset"},
		Attributes: map[string]any{
			"agent.loop_id":   event.LoopID,
			"agent.entity_id": event.EntityID,
			"agent.role":      event.Role,
			"service.name":    sc.serviceName,
			"service.version": sc.serviceVersion,
		},
	}

	// Add metadata as attributes
	for k, v := range event.Metadata {
		span.Attributes["agent."+k] = v
	}

	sc.activeSpans[event.LoopID] = span
	sc.spansCreated++
}

// endLoopSpan completes a loop span.
func (sc *SpanCollector) endLoopSpan(event *AgentEvent, statusCode, statusMsg string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	span, ok := sc.activeSpans[event.LoopID]
	if !ok {
		return
	}

	span.EndTime = event.Timestamp
	span.Status = SpanStatus{Code: statusCode, Message: statusMsg}

	if event.Duration > 0 {
		span.Attributes["agent.duration_ms"] = event.Duration.Milliseconds()
	}

	delete(sc.activeSpans, event.LoopID)
	sc.completedSpans = append(sc.completedSpans, span)
	sc.spansCompleted++
}

// startTaskSpan creates a child span for a task.
func (sc *SpanCollector) startTaskSpan(event *AgentEvent) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Find parent loop span
	parentSpan, ok := sc.activeSpans[event.LoopID]
	if !ok {
		return
	}

	spanKey := event.LoopID + ":" + event.TaskID
	span := &SpanData{
		TraceID:      parentSpan.TraceID,
		SpanID:       generateSpanID(spanKey),
		ParentSpanID: parentSpan.SpanID,
		Name:         "agent.task",
		Kind:         "internal",
		StartTime:    event.Timestamp,
		Status:       SpanStatus{Code: "unset"},
		Attributes: map[string]any{
			"agent.loop_id": event.LoopID,
			"agent.task_id": event.TaskID,
		},
	}

	for k, v := range event.Metadata {
		span.Attributes["task."+k] = v
	}

	sc.activeSpans[spanKey] = span
	sc.spansCreated++
}

// endTaskSpan completes a task span.
func (sc *SpanCollector) endTaskSpan(event *AgentEvent, statusCode, statusMsg string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	spanKey := event.LoopID + ":" + event.TaskID
	span, ok := sc.activeSpans[spanKey]
	if !ok {
		return
	}

	span.EndTime = event.Timestamp
	span.Status = SpanStatus{Code: statusCode, Message: statusMsg}

	if event.Duration > 0 {
		span.Attributes["task.duration_ms"] = event.Duration.Milliseconds()
	}

	delete(sc.activeSpans, spanKey)
	sc.completedSpans = append(sc.completedSpans, span)
	sc.spansCompleted++
}

// startToolSpan creates a child span for a tool execution.
func (sc *SpanCollector) startToolSpan(event *AgentEvent) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Find parent task or loop span
	parentKey := event.LoopID
	if event.TaskID != "" {
		parentKey = event.LoopID + ":" + event.TaskID
	}

	parentSpan, ok := sc.activeSpans[parentKey]
	if !ok {
		// Try loop span as parent
		parentSpan, ok = sc.activeSpans[event.LoopID]
		if !ok {
			return
		}
	}

	spanKey := event.LoopID + ":tool:" + event.ToolName
	span := &SpanData{
		TraceID:      parentSpan.TraceID,
		SpanID:       generateSpanID(spanKey),
		ParentSpanID: parentSpan.SpanID,
		Name:         "agent.tool." + event.ToolName,
		Kind:         "client",
		StartTime:    event.Timestamp,
		Status:       SpanStatus{Code: "unset"},
		Attributes: map[string]any{
			"agent.loop_id":  event.LoopID,
			"agent.task_id":  event.TaskID,
			"tool.name":      event.ToolName,
			"tool.timestamp": event.Timestamp.Format(time.RFC3339),
		},
	}

	for k, v := range event.Metadata {
		span.Attributes["tool."+k] = v
	}

	sc.activeSpans[spanKey] = span
	sc.spansCreated++
}

// endToolSpan completes a tool span.
func (sc *SpanCollector) endToolSpan(event *AgentEvent, statusCode, statusMsg string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	spanKey := event.LoopID + ":tool:" + event.ToolName
	span, ok := sc.activeSpans[spanKey]
	if !ok {
		return
	}

	span.EndTime = event.Timestamp
	span.Status = SpanStatus{Code: statusCode, Message: statusMsg}

	if event.Duration > 0 {
		span.Attributes["tool.duration_ms"] = event.Duration.Milliseconds()
	}

	delete(sc.activeSpans, spanKey)
	sc.completedSpans = append(sc.completedSpans, span)
	sc.spansCompleted++
}

// addEventToSpan adds an event to an active span.
func (sc *SpanCollector) addEventToSpan(event *AgentEvent) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Find parent span
	var span *SpanData
	if event.TaskID != "" {
		spanKey := event.LoopID + ":" + event.TaskID
		span = sc.activeSpans[spanKey]
	}
	if span == nil {
		span = sc.activeSpans[event.LoopID]
	}
	if span == nil {
		return
	}

	spanEvent := SpanEvent{
		Name:       event.Type,
		Timestamp:  event.Timestamp,
		Attributes: event.Metadata,
	}
	span.Events = append(span.Events, spanEvent)
}

// FlushCompleted returns and clears completed spans.
func (sc *SpanCollector) FlushCompleted() []*SpanData {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	spans := sc.completedSpans
	sc.completedSpans = make([]*SpanData, 0)
	return spans
}

// Stats returns collector statistics.
func (sc *SpanCollector) Stats() map[string]int64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return map[string]int64{
		"spans_created":   sc.spansCreated,
		"spans_completed": sc.spansCompleted,
		"spans_dropped":   sc.spansDropped,
		"active_spans":    int64(len(sc.activeSpans)),
		"pending_spans":   int64(len(sc.completedSpans)),
	}
}

// generateTraceID generates a trace ID from a loop ID.
func generateTraceID(loopID string) string {
	// Use a deterministic hash for trace ID based on loop ID
	// In production, this would use proper trace ID generation
	return hashToHex(loopID, 32)
}

// generateSpanID generates a span ID from a key.
func generateSpanID(key string) string {
	// Use a deterministic hash for span ID
	return hashToHex(key, 16)
}

// hashToHex creates a hex string of specified length from a key.
func hashToHex(key string, length int) string {
	// Simple deterministic hash for testing
	// In production, use crypto/rand or proper OTEL SDK
	h := uint64(0)
	for _, c := range key {
		h = h*31 + uint64(c)
	}

	hex := make([]byte, length)
	for i := 0; i < length; i++ {
		nibble := (h >> (uint(i) * 4)) & 0xf
		if nibble < 10 {
			hex[length-1-i] = byte('0' + nibble)
		} else {
			hex[length-1-i] = byte('a' + nibble - 10)
		}
	}
	return string(hex)
}
