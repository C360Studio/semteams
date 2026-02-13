package a2a

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
)

func TestNewComponent(t *testing.T) {
	cfg := DefaultConfig()

	rawConfig, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	if comp == nil {
		t.Fatal("expected component, got nil")
	}

	// Verify it implements Discoverable
	discoverable, ok := comp.(component.Discoverable)
	if !ok {
		t.Fatal("component does not implement Discoverable")
	}

	meta := discoverable.Meta()
	if meta.Name != "a2a-adapter" {
		t.Errorf("expected name 'a2a-adapter', got %q", meta.Name)
	}

	if meta.Type != "input" {
		t.Errorf("expected type 'input', got %q", meta.Type)
	}
}

func TestNewComponentInvalidConfig(t *testing.T) {
	tests := []struct {
		name      string
		rawConfig string
		wantErr   bool
	}{
		{
			name:      "invalid json",
			rawConfig: `{not valid json}`,
			wantErr:   true,
		},
		{
			name:      "invalid transport",
			rawConfig: `{"ports": {}, "transport": "grpc"}`,
			wantErr:   true,
		},
		{
			name:      "slim without group",
			rawConfig: `{"ports": {}, "transport": "slim"}`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := component.Dependencies{}

			_, err := NewComponent([]byte(tt.rawConfig), deps)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestComponentInitialize(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	if err := a2aComp.Initialize(); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	// Verify card generator was created
	if a2aComp.cardGenerator == nil {
		t.Error("card generator should be created during Initialize")
	}
}

func TestComponentStartWithoutNATSClient(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{
		// NATSClient is nil
	}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	_ = a2aComp.Initialize()

	ctx := context.Background()
	err = a2aComp.Start(ctx)
	if err == nil {
		t.Error("expected error when starting without NATS client")
	}
}

func TestComponentStartNilContext(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	_ = a2aComp.Initialize()

	// nolint:staticcheck // Testing nil context behavior
	err = a2aComp.Start(nil)
	if err == nil {
		t.Error("expected error when starting with nil context")
	}
}

func TestComponentStartCancelledContext(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	_ = a2aComp.Initialize()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = a2aComp.Start(ctx)
	if err == nil {
		t.Error("expected error when starting with cancelled context")
	}
}

func TestComponentMeta(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	meta := a2aComp.Meta()

	if meta.Name != "a2a-adapter" {
		t.Errorf("expected name 'a2a-adapter', got %q", meta.Name)
	}
	if meta.Type != "input" {
		t.Errorf("expected type 'input', got %q", meta.Type)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", meta.Version)
	}
}

func TestComponentInputPorts(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	ports := a2aComp.InputPorts()

	if len(ports) == 0 {
		t.Error("expected at least one input port")
	}

	// Verify first port
	if len(ports) > 0 {
		port := ports[0]
		if port.Direction != component.DirectionInput {
			t.Errorf("expected input direction, got %v", port.Direction)
		}
	}
}

func TestComponentOutputPorts(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	ports := a2aComp.OutputPorts()

	if len(ports) < 2 {
		t.Errorf("expected at least 2 output ports, got %d", len(ports))
	}

	// Verify output direction
	for _, port := range ports {
		if port.Direction != component.DirectionOutput {
			t.Errorf("expected output direction for port %q, got %v", port.Name, port.Direction)
		}
	}
}

func TestComponentHealth(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)

	// Before starting
	health := a2aComp.Health()
	if health.Healthy {
		t.Error("expected unhealthy before start")
	}
	if health.Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", health.Status)
	}
}

func TestComponentDataFlow(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	flow := a2aComp.DataFlow()

	// No activity yet
	if flow.ErrorRate != 0 {
		t.Errorf("expected error rate 0, got %f", flow.ErrorRate)
	}
}

func TestComponentStopWhenNotRunning(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)

	// Stop when not running should not error
	err = a2aComp.Stop(5 * time.Second)
	if err != nil {
		t.Errorf("Stop should not error when not running: %v", err)
	}
}

func TestComponentConfigSchema(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)
	schema := a2aComp.ConfigSchema()

	// Should have properties
	if len(schema.Properties) == 0 {
		t.Error("expected schema to have properties")
	}
}

func TestComponentUpdateAgentCard(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	a2aComp := comp.(*Component)

	card := &AgentCard{
		Name:        "Test Agent",
		Description: "Updated agent card",
	}

	a2aComp.UpdateAgentCard(card)

	// Verify card was stored
	a2aComp.cardMu.RLock()
	storedCard := a2aComp.agentCard
	a2aComp.cardMu.RUnlock()

	if storedCard == nil {
		t.Fatal("expected stored card, got nil")
	}

	if storedCard.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got %q", storedCard.Name)
	}
}

func TestSanitizeSubject(t *testing.T) {
	tests := []struct {
		name   string
		taskID string
		want   string
	}{
		{
			name:   "no special chars",
			taskID: "task123",
			want:   "task123",
		},
		{
			name:   "with dots",
			taskID: "org.task.123",
			want:   "org-task-123",
		},
		{
			name:   "with colons",
			taskID: "task:123:abc",
			want:   "task-123-abc",
		},
		{
			name:   "with slashes",
			taskID: "task/123/abc",
			want:   "task-123-abc",
		},
		{
			name:   "mixed special chars",
			taskID: "org:task.123/abc",
			want:   "org-task-123-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSubject(tt.taskID)
			if got != tt.want {
				t.Errorf("sanitizeSubject(%q) = %q, want %q", tt.taskID, got, tt.want)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name     string
		registry RegistryInterface
		wantErr  bool
	}{
		{
			name:     "nil registry",
			registry: nil,
			wantErr:  true,
		},
		{
			name:     "valid registry",
			registry: &mockRegistry{},
			wantErr:  false,
		},
		{
			name:     "registry returns error",
			registry: &mockRegistry{err: errMock},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Register(tt.registry)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// mockRegistry implements RegistryInterface for testing
type mockRegistry struct {
	err error
}

var errMock = &mockError{msg: "mock error"}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func (m *mockRegistry) RegisterWithConfig(_ component.RegistrationConfig) error {
	return m.err
}
