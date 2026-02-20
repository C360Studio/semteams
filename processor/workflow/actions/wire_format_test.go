package actions

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/message"
)

func TestParsePayloadToMap(t *testing.T) {
	tests := []struct {
		name     string
		payload  json.RawMessage
		wantKeys []string
		wantRaw  bool // expect "raw" key
		wantVal  bool // expect "value" key
		checkVal any  // expected value under "value" key (if wantVal)
	}{
		{
			name:     "json object",
			payload:  json.RawMessage(`{"key": "value", "num": 42}`),
			wantKeys: []string{"key", "num"},
		},
		{
			name:     "empty object",
			payload:  json.RawMessage(`{}`),
			wantKeys: []string{},
		},
		{
			name:     "nil payload",
			payload:  nil,
			wantKeys: []string{},
		},
		{
			name:     "empty payload",
			payload:  json.RawMessage(``),
			wantKeys: []string{},
		},
		{
			name:     "json string",
			payload:  json.RawMessage(`"hello world"`),
			wantVal:  true,
			checkVal: "hello world",
		},
		{
			name:     "json number",
			payload:  json.RawMessage(`42`),
			wantVal:  true,
			checkVal: float64(42), // JSON numbers unmarshal as float64
		},
		{
			name:     "json boolean",
			payload:  json.RawMessage(`true`),
			wantVal:  true,
			checkVal: true,
		},
		{
			name:     "json null",
			payload:  json.RawMessage(`null`),
			wantVal:  true,
			checkVal: nil,
		},
		{
			name:    "json array",
			payload: json.RawMessage(`[1, 2, 3]`),
			wantVal: true,
			// Array check is more complex, just verify key exists
		},
		{
			name:    "invalid json",
			payload: json.RawMessage(`not valid json`),
			wantRaw: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePayloadToMap(tt.payload)

			if tt.wantRaw {
				if _, ok := result["raw"]; !ok {
					t.Error("expected 'raw' key in result")
				}
				return
			}

			if tt.wantVal {
				val, ok := result["value"]
				if !ok {
					t.Error("expected 'value' key in result")
					return
				}
				if tt.checkVal != nil && val != tt.checkVal {
					t.Errorf("value = %v (%T), want %v (%T)", val, val, tt.checkVal, tt.checkVal)
				}
				return
			}

			// For object payloads, verify expected keys exist
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q in result", key)
				}
			}
		})
	}
}

func TestAsyncTaskPayloadWireFormat(t *testing.T) {
	// Create an AsyncTaskPayload
	asyncTask := &AsyncTaskPayload{
		TaskID:          "test-task-123",
		CallbackSubject: "workflow.step.result.exec-456",
		Data:            json.RawMessage(`{"original": "data"}`),
	}

	// Wrap in BaseMessage
	baseMsg := message.NewBaseMessage(asyncTask.Schema(), asyncTask, "workflow")

	// Marshal to JSON
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify wire format structure
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal wire format failed: %v", err)
	}

	// Check type field
	msgType, ok := wire["type"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'type' field")
	}
	if msgType["domain"] != "workflow" {
		t.Errorf("type.domain = %v, want 'workflow'", msgType["domain"])
	}
	if msgType["category"] != "async_task" {
		t.Errorf("type.category = %v, want 'async_task'", msgType["category"])
	}
	if msgType["version"] != "v1" {
		t.Errorf("type.version = %v, want 'v1'", msgType["version"])
	}

	// Check payload field
	payload, ok := wire["payload"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'payload' field")
	}
	if payload["task_id"] != "test-task-123" {
		t.Errorf("payload.task_id = %v, want 'test-task-123'", payload["task_id"])
	}
	if payload["callback_subject"] != "workflow.step.result.exec-456" {
		t.Errorf("payload.callback_subject = %v, want 'workflow.step.result.exec-456'", payload["callback_subject"])
	}

	// Check meta field exists
	if _, ok := wire["meta"]; !ok {
		t.Error("missing 'meta' field")
	}
}

func TestAsyncTaskPayloadRoundTrip(t *testing.T) {
	// Create original payload
	original := &AsyncTaskPayload{
		TaskID:          "round-trip-task",
		CallbackSubject: "workflow.step.result.exec-789",
		Data:            json.RawMessage(`{"key": "value", "nested": {"a": 1}}`),
	}

	// Wrap in BaseMessage and marshal
	baseMsg := message.NewBaseMessage(original.Schema(), original, "test-source")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal back to BaseMessage
	var restored message.BaseMessage
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify type
	if restored.Type().Domain != "workflow" {
		t.Errorf("restored type.domain = %v, want 'workflow'", restored.Type().Domain)
	}
	if restored.Type().Category != "async_task" {
		t.Errorf("restored type.category = %v, want 'async_task'", restored.Type().Category)
	}

	// Verify payload
	restoredPayload, ok := restored.Payload().(*AsyncTaskPayload)
	if !ok {
		t.Fatalf("restored payload type = %T, want *AsyncTaskPayload", restored.Payload())
	}
	if restoredPayload.TaskID != original.TaskID {
		t.Errorf("restored task_id = %v, want %v", restoredPayload.TaskID, original.TaskID)
	}
	if restoredPayload.CallbackSubject != original.CallbackSubject {
		t.Errorf("restored callback_subject = %v, want %v", restoredPayload.CallbackSubject, original.CallbackSubject)
	}

	// Compare Data by unmarshaling both to maps (avoids whitespace differences)
	var originalData, restoredData map[string]any
	if err := json.Unmarshal(original.Data, &originalData); err != nil {
		t.Fatalf("unmarshal original data failed: %v", err)
	}
	if err := json.Unmarshal(restoredPayload.Data, &restoredData); err != nil {
		t.Fatalf("unmarshal restored data failed: %v", err)
	}
	if restoredData["key"] != originalData["key"] {
		t.Errorf("restored data.key = %v, want %v", restoredData["key"], originalData["key"])
	}
}

func TestGenericJSONPayloadWireFormat(t *testing.T) {
	// This tests the format used by publish and call actions
	dataMap := map[string]any{
		"field1": "value1",
		"field2": float64(42),
	}

	genericPayload := message.NewGenericJSON(dataMap)
	baseMsg := message.NewBaseMessage(genericPayload.Schema(), genericPayload, "workflow")

	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify wire format structure
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal wire format failed: %v", err)
	}

	// Check type field
	msgType, ok := wire["type"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'type' field")
	}
	if msgType["domain"] != "core" {
		t.Errorf("type.domain = %v, want 'core'", msgType["domain"])
	}
	if msgType["category"] != "json" {
		t.Errorf("type.category = %v, want 'json'", msgType["category"])
	}
	if msgType["version"] != "v1" {
		t.Errorf("type.version = %v, want 'v1'", msgType["version"])
	}

	// Check payload.data field
	payload, ok := wire["payload"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'payload' field")
	}
	payloadData, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'payload.data' field")
	}
	if payloadData["field1"] != "value1" {
		t.Errorf("payload.data.field1 = %v, want 'value1'", payloadData["field1"])
	}
}

func TestGenericJSONPayloadRoundTrip(t *testing.T) {
	dataMap := map[string]any{
		"string":  "hello",
		"number":  float64(3.14),
		"boolean": true,
		"nested": map[string]any{
			"inner": "value",
		},
	}

	original := message.NewGenericJSON(dataMap)
	baseMsg := message.NewBaseMessage(original.Schema(), original, "test-source")

	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored message.BaseMessage
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify type
	if restored.Type().String() != "core.json.v1" {
		t.Errorf("restored type = %v, want 'core.json.v1'", restored.Type().String())
	}

	// Verify payload
	restoredPayload, ok := restored.Payload().(*message.GenericJSONPayload)
	if !ok {
		t.Fatalf("restored payload type = %T, want *message.GenericJSONPayload", restored.Payload())
	}
	if restoredPayload.Data["string"] != "hello" {
		t.Errorf("restored data.string = %v, want 'hello'", restoredPayload.Data["string"])
	}
}

func TestNonObjectPayloadWrapping(t *testing.T) {
	tests := []struct {
		name       string
		payload    json.RawMessage
		wantKey    string // "value" for valid non-object JSON, "raw" for invalid
		wantString string // expected string representation under the key
	}{
		{
			name:    "string payload",
			payload: json.RawMessage(`"hello"`),
			wantKey: "value",
		},
		{
			name:    "array payload",
			payload: json.RawMessage(`[1, 2, 3]`),
			wantKey: "value",
		},
		{
			name:    "number payload",
			payload: json.RawMessage(`42`),
			wantKey: "value",
		},
		{
			name:       "invalid json",
			payload:    json.RawMessage(`not json`),
			wantKey:    "raw",
			wantString: "not json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataMap := parsePayloadToMap(tt.payload)

			// Wrap in GenericJSONPayload
			genericPayload := message.NewGenericJSON(dataMap)
			baseMsg := message.NewBaseMessage(genericPayload.Schema(), genericPayload, "workflow")

			data, err := json.Marshal(baseMsg)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// Verify structure
			var wire map[string]any
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			payload := wire["payload"].(map[string]any)
			payloadData := payload["data"].(map[string]any)

			if _, ok := payloadData[tt.wantKey]; !ok {
				t.Errorf("expected key %q in payload.data, got keys: %v", tt.wantKey, payloadData)
			}

			if tt.wantString != "" {
				if payloadData[tt.wantKey] != tt.wantString {
					t.Errorf("payload.data.%s = %v, want %q", tt.wantKey, payloadData[tt.wantKey], tt.wantString)
				}
			}
		})
	}
}
