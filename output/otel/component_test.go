package otel

import (
	"context"
	"encoding/json"
	"sync"
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
	if meta.Name != "otel-exporter" {
		t.Errorf("expected name 'otel-exporter', got %q", meta.Name)
	}

	if meta.Type != "output" {
		t.Errorf("expected type 'output', got %q", meta.Type)
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
			name:      "invalid protocol",
			rawConfig: `{"ports": {}, "protocol": "websocket"}`,
			wantErr:   true,
		},
		{
			name:      "invalid sampling rate",
			rawConfig: `{"ports": {}, "sampling_rate": 2.0}`,
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

	otelComp := comp.(*Component)
	if err := otelComp.Initialize(); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	// Verify span collector was created
	if otelComp.spanCollector == nil {
		t.Error("span collector should be created during Initialize")
	}

	// Verify metric mapper was created
	if otelComp.metricMapper == nil {
		t.Error("metric mapper should be created during Initialize")
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

	otelComp := comp.(*Component)
	_ = otelComp.Initialize()

	ctx := context.Background()
	err = otelComp.Start(ctx)
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

	otelComp := comp.(*Component)
	_ = otelComp.Initialize()

	// nolint:staticcheck // Testing nil context behavior
	err = otelComp.Start(nil)
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

	otelComp := comp.(*Component)
	_ = otelComp.Initialize()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = otelComp.Start(ctx)
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

	otelComp := comp.(*Component)
	meta := otelComp.Meta()

	if meta.Name != "otel-exporter" {
		t.Errorf("expected name 'otel-exporter', got %q", meta.Name)
	}
	if meta.Type != "output" {
		t.Errorf("expected type 'output', got %q", meta.Type)
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

	otelComp := comp.(*Component)
	ports := otelComp.InputPorts()

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

	otelComp := comp.(*Component)
	ports := otelComp.OutputPorts()

	// OTEL exporter has no NATS output ports
	if len(ports) != 0 {
		t.Errorf("expected 0 output ports, got %d", len(ports))
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

	otelComp := comp.(*Component)

	// Before starting
	health := otelComp.Health()
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

	otelComp := comp.(*Component)
	flow := otelComp.DataFlow()

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

	otelComp := comp.(*Component)

	// Stop when not running should not error
	err = otelComp.Stop(5 * time.Second)
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

	otelComp := comp.(*Component)
	schema := otelComp.ConfigSchema()

	// Should have properties
	if len(schema.Properties) == 0 {
		t.Error("expected schema to have properties")
	}
}

func TestComponentSetExporter(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	otelComp := comp.(*Component)
	_ = otelComp.Initialize()

	// Set mock exporter
	mockExp := &MockExporter{}
	otelComp.SetExporter(mockExp)

	if otelComp.exporter != mockExp {
		t.Error("expected exporter to be set")
	}
}

func TestComponentGetSpanCollector(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	otelComp := comp.(*Component)
	_ = otelComp.Initialize()

	sc := otelComp.GetSpanCollector()
	if sc == nil {
		t.Error("expected span collector, got nil")
	}
}

func TestComponentGetMetricMapper(t *testing.T) {
	cfg := DefaultConfig()
	rawConfig, _ := json.Marshal(cfg)

	deps := component.Dependencies{}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	otelComp := comp.(*Component)
	_ = otelComp.Initialize()

	mm := otelComp.GetMetricMapper()
	if mm == nil {
		t.Error("expected metric mapper, got nil")
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

// MockExporter implements Exporter for testing.
type MockExporter struct {
	mu sync.Mutex

	SpansExported   []*SpanData
	MetricsExported []*MetricData

	ExportSpansErr   error
	ExportMetricsErr error
	ShutdownErr      error

	shutdownCalled bool
}

func (m *MockExporter) ExportSpans(_ context.Context, spans []*SpanData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ExportSpansErr != nil {
		return m.ExportSpansErr
	}

	m.SpansExported = append(m.SpansExported, spans...)
	return nil
}

func (m *MockExporter) ExportMetrics(_ context.Context, metrics []*MetricData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ExportMetricsErr != nil {
		return m.ExportMetricsErr
	}

	m.MetricsExported = append(m.MetricsExported, metrics...)
	return nil
}

func (m *MockExporter) Shutdown(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shutdownCalled = true
	return m.ShutdownErr
}

func (m *MockExporter) GetExportedSpans() []*SpanData {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*SpanData, len(m.SpansExported))
	copy(result, m.SpansExported)
	return result
}

func (m *MockExporter) GetExportedMetrics() []*MetricData {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*MetricData, len(m.MetricsExported))
	copy(result, m.MetricsExported)
	return result
}

func (m *MockExporter) WasShutdownCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCalled
}

// mockRegistry implements RegistryInterface for testing.
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
