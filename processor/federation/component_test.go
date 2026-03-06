package federation_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	proc "github.com/c360studio/semstreams/processor/federation"
)

func TestNewComponent_DefaultConfig(t *testing.T) {
	deps := component.Dependencies{
		Logger: nil, // will use slog.Default
	}

	comp, err := proc.NewComponent(json.RawMessage(`{}`), deps)
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}
	if comp == nil {
		t.Fatal("NewComponent() returned nil")
	}

	meta := comp.Meta()
	if meta.Name != "federation-processor" {
		t.Errorf("Meta().Name = %q, want %q", meta.Name, "federation-processor")
	}
	if meta.Type != "processor" {
		t.Errorf("Meta().Type = %q, want %q", meta.Type, "processor")
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Meta().Version = %q, want %q", meta.Version, "1.0.0")
	}
}

func TestNewComponent_CustomConfig(t *testing.T) {
	cfg := proc.Config{
		LocalNamespace: "acme",
		MergePolicy:    proc.MergePolicyStandard,
		InputSubject:   "semsource.graph.events",
		OutputSubject:  "semsource.graph.merged",
		InputStream:    "GRAPH_EVENTS",
		OutputStream:   "GRAPH_MERGED",
	}

	rawConfig, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal config: %v", err)
	}

	deps := component.Dependencies{}
	comp, err := proc.NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}
	if comp == nil {
		t.Fatal("NewComponent() returned nil")
	}

	// Verify ports reflect custom subjects
	inputs := comp.InputPorts()
	if len(inputs) != 1 {
		t.Fatalf("InputPorts() len = %d, want 1", len(inputs))
	}
	jsPort, ok := inputs[0].Config.(component.JetStreamPort)
	if !ok {
		t.Fatalf("InputPorts()[0].Config type = %T, want JetStreamPort", inputs[0].Config)
	}
	if jsPort.StreamName != "GRAPH_EVENTS" {
		t.Errorf("InputPorts()[0].StreamName = %q, want %q", jsPort.StreamName, "GRAPH_EVENTS")
	}
	if len(jsPort.Subjects) != 1 || jsPort.Subjects[0] != "semsource.graph.events" {
		t.Errorf("InputPorts()[0].Subjects = %v, want [semsource.graph.events]", jsPort.Subjects)
	}

	outputs := comp.OutputPorts()
	if len(outputs) != 1 {
		t.Fatalf("OutputPorts() len = %d, want 1", len(outputs))
	}
	jsOutPort, ok := outputs[0].Config.(component.JetStreamPort)
	if !ok {
		t.Fatalf("OutputPorts()[0].Config type = %T, want JetStreamPort", outputs[0].Config)
	}
	if jsOutPort.StreamName != "GRAPH_MERGED" {
		t.Errorf("OutputPorts()[0].StreamName = %q, want %q", jsOutPort.StreamName, "GRAPH_MERGED")
	}
}

func TestNewComponent_InvalidConfig(t *testing.T) {
	deps := component.Dependencies{}

	// Missing local_namespace
	rawConfig := json.RawMessage(`{"local_namespace":"","merge_policy":"standard"}`)
	_, err := proc.NewComponent(rawConfig, deps)
	if err == nil {
		t.Error("NewComponent() expected error for empty local_namespace")
	}
}

func TestNewComponent_MalformedJSON(t *testing.T) {
	deps := component.Dependencies{}
	_, err := proc.NewComponent(json.RawMessage(`{not json`), deps)
	if err == nil {
		t.Error("NewComponent() expected error for malformed JSON")
	}
}

func TestComponent_Health_Stopped(t *testing.T) {
	deps := component.Dependencies{}
	comp, err := proc.NewComponent(json.RawMessage(`{}`), deps)
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}

	health := comp.Health()
	if health.Healthy {
		t.Error("Health().Healthy should be false when stopped")
	}
	if health.Status != "stopped" {
		t.Errorf("Health().Status = %q, want %q", health.Status, "stopped")
	}
}

func TestComponent_DataFlow_Stopped(t *testing.T) {
	deps := component.Dependencies{}
	comp, err := proc.NewComponent(json.RawMessage(`{}`), deps)
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}

	flow := comp.DataFlow()
	if flow.MessagesPerSecond != 0 {
		t.Errorf("DataFlow().MessagesPerSecond = %f, want 0", flow.MessagesPerSecond)
	}
	if flow.BytesPerSecond != 0 {
		t.Errorf("DataFlow().BytesPerSecond = %f, want 0", flow.BytesPerSecond)
	}
	if flow.ErrorRate != 0 {
		t.Errorf("DataFlow().ErrorRate = %f, want 0", flow.ErrorRate)
	}
	if !flow.LastActivity.IsZero() {
		t.Errorf("DataFlow().LastActivity should be zero when stopped, got %v", flow.LastActivity)
	}
}

func TestComponent_StartRequiresNATSClient(t *testing.T) {
	deps := component.Dependencies{}
	comp, err := proc.NewComponent(json.RawMessage(`{}`), deps)
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}

	// Cast to access Start
	lc, ok := comp.(component.LifecycleComponent)
	if !ok {
		t.Fatal("Component does not implement LifecycleComponent")
	}

	err = lc.Start(t.Context())
	if err == nil {
		t.Error("Start() should error without NATS client")
	}
}

func TestRegister_NilRegistry(t *testing.T) {
	err := proc.Register(nil)
	if err == nil {
		t.Error("Register(nil) should return error")
	}
}

func TestConfigSchema_Generated(t *testing.T) {
	deps := component.Dependencies{}
	comp, err := proc.NewComponent(json.RawMessage(`{}`), deps)
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}

	schema := comp.ConfigSchema()
	if schema.Properties == nil {
		t.Error("ConfigSchema().Properties should not be nil")
	}
	// Should have at least local_namespace and merge_policy
	if _, ok := schema.Properties["local_namespace"]; !ok {
		t.Error("ConfigSchema() missing local_namespace property")
	}
}
