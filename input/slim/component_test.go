package slim

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
)

func TestNewComponent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SLIMEndpoint = "wss://test.slim.local"
	cfg.GroupIDs = []string{"test-group-1"}

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
	if meta.Name != "slim-bridge" {
		t.Errorf("expected name 'slim-bridge', got %q", meta.Name)
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
			name:      "missing ports with invalid duration",
			rawConfig: `{"key_ratchet_interval": "invalid"}`,
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

	slimComp := comp.(*Component)
	if err := slimComp.Initialize(); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	// Verify session manager was created
	if slimComp.sessionManager == nil {
		t.Error("session manager should be created during Initialize")
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

	slimComp := comp.(*Component)
	_ = slimComp.Initialize()

	ctx := context.Background()
	err = slimComp.Start(ctx)
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

	slimComp := comp.(*Component)
	_ = slimComp.Initialize()

	// nolint:staticcheck // Testing nil context behavior
	err = slimComp.Start(nil)
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

	slimComp := comp.(*Component)
	_ = slimComp.Initialize()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = slimComp.Start(ctx)
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

	slimComp := comp.(*Component)
	meta := slimComp.Meta()

	if meta.Name != "slim-bridge" {
		t.Errorf("expected name 'slim-bridge', got %q", meta.Name)
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

	slimComp := comp.(*Component)
	ports := slimComp.InputPorts()

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

	slimComp := comp.(*Component)
	ports := slimComp.OutputPorts()

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

	slimComp := comp.(*Component)

	// Before starting
	health := slimComp.Health()
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

	slimComp := comp.(*Component)
	flow := slimComp.DataFlow()

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

	slimComp := comp.(*Component)

	// Stop when not running should not error
	err = slimComp.Stop(5 * time.Second)
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

	slimComp := comp.(*Component)
	schema := slimComp.ConfigSchema()

	// Should have properties
	if len(schema.Properties) == 0 {
		t.Error("expected schema to have properties")
	}
}

func TestSanitizeSubject(t *testing.T) {
	tests := []struct {
		name    string
		groupID string
		want    string
	}{
		{
			name:    "no special chars",
			groupID: "group123",
			want:    "group123",
		},
		{
			name:    "with dots",
			groupID: "org.group.123",
			want:    "org-group-123",
		},
		{
			name:    "with colons (DID format)",
			groupID: "did:agntcy:group:tenant-123",
			want:    "did-agntcy-group-tenant-123",
		},
		{
			name:    "mixed special chars",
			groupID: "org:platform.group:123",
			want:    "org-platform-group-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSubject(tt.groupID)
			if got != tt.want {
				t.Errorf("sanitizeSubject(%q) = %q, want %q", tt.groupID, got, tt.want)
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
