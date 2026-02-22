package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/workflow/actions"
)

func TestNewComponent(t *testing.T) {
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	if comp == nil {
		t.Fatal("component should not be nil")
	}
}

func TestNewComponentInvalidConfig(t *testing.T) {
	deps := component.Dependencies{}

	// Invalid JSON
	_, err := NewComponent([]byte("not json"), deps)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// Invalid config (missing required field)
	invalidConfig := Config{
		DefinitionsBucket: "",
		ExecutionsBucket:  "test",
		StreamName:        "test",
		DefaultTimeout:    "10m",
	}
	configJSON, _ := json.Marshal(invalidConfig)
	_, err = NewComponent(configJSON, deps)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestComponentMeta(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	meta := comp.Meta()

	if meta.Name != "workflow-processor" {
		t.Errorf("Name = %q, want 'workflow-processor'", meta.Name)
	}
	if meta.Type != "processor" {
		t.Errorf("Type = %q, want 'processor'", meta.Type)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q, want '1.0.0'", meta.Version)
	}
}

func TestComponentPorts(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	inputPorts := comp.InputPorts()
	outputPorts := comp.OutputPorts()

	if len(inputPorts) == 0 {
		t.Error("should have input ports")
	}
	if len(outputPorts) == 0 {
		t.Error("should have output ports")
	}

	// Check for expected input ports
	inputNames := make(map[string]bool)
	for _, p := range inputPorts {
		inputNames[p.Name] = true
	}

	expectedInputs := []string{"workflow.trigger", "workflow.step.complete", "workflow.step.result"}
	for _, name := range expectedInputs {
		if !inputNames[name] {
			t.Errorf("missing expected input port: %s", name)
		}
	}

	// Check for expected output ports
	outputNames := make(map[string]bool)
	for _, p := range outputPorts {
		outputNames[p.Name] = true
	}

	expectedOutputs := []string{"workflow.events", "agent.task"}
	for _, name := range expectedOutputs {
		if !outputNames[name] {
			t.Errorf("missing expected output port: %s", name)
		}
	}
}

func TestComponentConfigSchema(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	schema := comp.ConfigSchema()

	if schema.Properties == nil {
		t.Error("schema properties should not be nil")
	}

	// Check for expected properties
	expectedProps := []string{
		"definitions_bucket",
		"executions_bucket",
		"stream_name",
		"default_timeout",
		"default_max_iterations",
		"request_timeout",
		"ports",
	}

	for _, prop := range expectedProps {
		if _, ok := schema.Properties[prop]; !ok {
			t.Errorf("missing expected property: %s", prop)
		}
	}

	// Check default values
	defBucket, ok := schema.Properties["definitions_bucket"]
	if !ok {
		t.Fatal("definitions_bucket property not found")
	}
	if defBucket.Default != "WORKFLOW_DEFINITIONS" {
		t.Errorf("definitions_bucket default = %v, want 'WORKFLOW_DEFINITIONS'", defBucket.Default)
	}

	// Check min/max for max_iterations
	maxIterProp := schema.Properties["default_max_iterations"]
	if maxIterProp.Minimum == nil || *maxIterProp.Minimum != 1 {
		t.Errorf("default_max_iterations min should be 1")
	}
	if maxIterProp.Maximum == nil || *maxIterProp.Maximum != 100 {
		t.Errorf("default_max_iterations max should be 100")
	}
}

func TestComponentHealth(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	health := comp.Health()

	// Before start, should not be healthy
	if health.Healthy {
		t.Error("component should not be healthy before start")
	}
	if health.Status != "stopped" {
		t.Errorf("Status = %q, want 'stopped'", health.Status)
	}
}

func TestComponentDataFlow(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	flow := comp.DataFlow()

	// Initial data flow metrics
	if flow.MessagesPerSecond != 0 {
		t.Errorf("MessagesPerSecond = %v, want 0", flow.MessagesPerSecond)
	}
	if flow.BytesPerSecond != 0 {
		t.Errorf("BytesPerSecond = %v, want 0", flow.BytesPerSecond)
	}
	if flow.ErrorRate != 0 {
		t.Errorf("ErrorRate = %v, want 0", flow.ErrorRate)
	}
}

func TestConfigSchema(t *testing.T) {
	// Schema is auto-generated from Config struct tags

	// Should have required fields
	if len(schema.Required) == 0 {
		t.Error("schema should have required fields")
	}

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	expectedRequired := []string{"definitions_bucket", "executions_bucket", "stream_name"}
	for _, r := range expectedRequired {
		if !requiredSet[r] {
			t.Errorf("missing expected required field: %s", r)
		}
	}
}

func TestSanitizeSubject(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"workflow.trigger.test", "workflow-trigger-test"},
		{"agent.complete.>", "agent-complete-all"},
		{"agent.task.*", "agent-task-any"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeSubject(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeSubject(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultPorts(t *testing.T) {
	inputPorts := buildDefaultInputPorts()
	outputPorts := buildDefaultOutputPorts()

	// Verify input ports have correct structure
	for _, port := range inputPorts {
		if port.Direction != component.DirectionInput {
			t.Errorf("input port %s has wrong direction", port.Name)
		}
	}

	// Verify output ports have correct structure
	for _, port := range outputPorts {
		if port.Direction != component.DirectionOutput {
			t.Errorf("output port %s has wrong direction", port.Name)
		}
	}

	// Verify JetStreamPort config is properly set
	for _, port := range inputPorts {
		jsPort, ok := port.Config.(component.JetStreamPort)
		if !ok {
			t.Errorf("input port %s should have JetStreamPort config", port.Name)
			continue
		}
		if len(jsPort.Subjects) == 0 {
			t.Errorf("input port %s should have subjects", port.Name)
		}
	}
}

func TestInitializeWithoutNATSClient(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{} // No NATS client

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	// Initialize should succeed even without NATS
	if err := comp.(*Component).Initialize(); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}
}

func TestStartWithNilContext(t *testing.T) {
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	wfComp := comp.(*Component)
	if err := wfComp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Start with nil context should return error
	err = wfComp.Start(nil)
	if err == nil {
		t.Error("Start with nil context should return error")
	}
	if err != nil && err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

func TestHandleTriggerMessage_WarnsOnAsyncTaskPayload(t *testing.T) {
	// Setup: Create component with a log capture buffer
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	wfComp := comp.(*Component)

	// Replace logger with one that captures output
	var logBuf bytes.Buffer
	wfComp.logger = slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create an AsyncTaskPayload wrapped in BaseMessage
	asyncPayload := &actions.AsyncTaskPayload{
		TaskID:          "test-task-123",
		CallbackSubject: "workflow.step.result.exec-456",
		Data:            json.RawMessage(`{"key": "value"}`),
	}

	baseMsg := message.NewBaseMessage(
		message.Type{Domain: "workflow", Category: "async_task", Version: "v1"},
		asyncPayload,
		"test-source",
	)

	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	// Act: Call handler with AsyncTaskPayload
	ctx := context.Background()
	wfComp.handleTriggerMessage(ctx, data)

	// Assert: Should log WARNING, not ERROR
	logOutput := logBuf.String()

	if strings.Contains(logOutput, `"level":"ERROR"`) {
		t.Errorf("should not log ERROR for AsyncTaskPayload; log output: %s", logOutput)
	}

	if !strings.Contains(logOutput, `"level":"WARN"`) {
		t.Errorf("should log WARN for AsyncTaskPayload on trigger subject; log output: %s", logOutput)
	}

	if !strings.Contains(logOutput, "use workflow.async.* instead") {
		t.Errorf("warning should mention correct namespace; log output: %s", logOutput)
	}
}

func TestHandleTriggerMessage_SkipsOtherTypes(t *testing.T) {
	// Setup: Create component with a log capture buffer
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	wfComp := comp.(*Component)

	// Replace logger with one that captures output
	var logBuf bytes.Buffer
	wfComp.logger = slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create an AsyncStepResult (another workflow type, but not trigger or async_task)
	stepResult := &AsyncStepResult{
		TaskID: "task-123",
		Status: "success",
	}

	baseMsg := message.NewBaseMessage(
		message.Type{Domain: "workflow", Category: "step_result", Version: "v1"},
		stepResult,
		"test-source",
	)

	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	// Act: Call handler with non-trigger, non-async_task type
	ctx := context.Background()
	wfComp.handleTriggerMessage(ctx, data)

	// Assert: Should log DEBUG, not ERROR or WARN
	logOutput := logBuf.String()

	if strings.Contains(logOutput, `"level":"ERROR"`) {
		t.Errorf("should not log ERROR for step_result on trigger subject; log output: %s", logOutput)
	}

	if strings.Contains(logOutput, `"level":"WARN"`) {
		t.Errorf("should not log WARN for step_result (only async_task gets WARN); log output: %s", logOutput)
	}

	if !strings.Contains(logOutput, `"level":"DEBUG"`) {
		t.Errorf("should log DEBUG for non-trigger message; log output: %s", logOutput)
	}

	if !strings.Contains(logOutput, "Skipping non-trigger message") {
		t.Errorf("debug should mention skipping; log output: %s", logOutput)
	}
}

func TestHandleTriggerMessage_ProcessesTriggerPayload(t *testing.T) {
	// Setup: Create component with a log capture buffer
	config := DefaultConfig()
	configJSON, _ := json.Marshal(config)
	deps := component.Dependencies{}

	comp, err := NewComponent(configJSON, deps)
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	wfComp := comp.(*Component)

	// Initialize a minimal registry (with nil bucket - it will fail on Get but that's fine)
	wfComp.registry = &Registry{
		logger: slog.Default(),
	}

	// Replace logger with one that captures output
	var logBuf bytes.Buffer
	wfComp.logger = slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create a TriggerPayload (correct type for this handler)
	trigger := &TriggerPayload{
		WorkflowID: "test-workflow",
	}

	baseMsg := message.NewBaseMessage(
		message.Type{Domain: "workflow", Category: "trigger", Version: "v1"},
		trigger,
		"test-source",
	)

	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	// Act: Call handler with TriggerPayload
	ctx := context.Background()
	wfComp.handleTriggerMessage(ctx, data)

	// Assert: Should NOT log "Skipping" or WARN for async_task
	// The key point is that TriggerPayload passes the type filter.
	// It may error later (Workflow not found) but that's not what we're testing.
	logOutput := logBuf.String()

	if strings.Contains(logOutput, "Skipping non-trigger message") {
		t.Errorf("should not skip TriggerPayload; log output: %s", logOutput)
	}

	if strings.Contains(logOutput, "use workflow.async.* instead") {
		t.Errorf("should not warn about namespace for TriggerPayload; log output: %s", logOutput)
	}
}
