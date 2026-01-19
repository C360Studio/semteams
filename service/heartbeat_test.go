package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// mockComponentHealthGetter implements componentHealthGetter for testing
type mockComponentHealthGetter struct {
	health map[string]bool
}

func (m *mockComponentHealthGetter) GetComponentHealth() map[string]bool {
	return m.health
}

func TestHeartbeatConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  HeartbeatConfig
		wantErr bool
	}{
		{
			name:    "empty interval uses default",
			config:  HeartbeatConfig{},
			wantErr: false,
		},
		{
			name:    "valid interval",
			config:  HeartbeatConfig{Interval: "30s"},
			wantErr: false,
		},
		{
			name:    "valid minute interval",
			config:  HeartbeatConfig{Interval: "1m"},
			wantErr: false,
		},
		{
			name:    "invalid duration format",
			config:  HeartbeatConfig{Interval: "invalid"},
			wantErr: true,
		},
		{
			name:    "negative interval",
			config:  HeartbeatConfig{Interval: "-1s"},
			wantErr: true,
		},
		{
			name:    "zero interval",
			config:  HeartbeatConfig{Interval: "0s"},
			wantErr: true,
		},
		{
			name:    "interval too short",
			config:  HeartbeatConfig{Interval: "500ms"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewHeartbeatService(t *testing.T) {
	tests := []struct {
		name      string
		rawConfig json.RawMessage
		wantErr   bool
	}{
		{
			name:      "nil config uses defaults",
			rawConfig: nil,
			wantErr:   false,
		},
		{
			name:      "empty config uses defaults",
			rawConfig: json.RawMessage(`{}`),
			wantErr:   false,
		},
		{
			name:      "valid config",
			rawConfig: json.RawMessage(`{"interval": "10s"}`),
			wantErr:   false,
		},
		{
			name:      "invalid json",
			rawConfig: json.RawMessage(`{invalid`),
			wantErr:   true,
		},
		{
			name:      "invalid interval",
			rawConfig: json.RawMessage(`{"interval": "invalid"}`),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewHeartbeatService(tt.rawConfig, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHeartbeatService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && svc == nil {
				t.Error("NewHeartbeatService() returned nil service without error")
			}
		})
	}
}

func TestHeartbeatService_StartStop(t *testing.T) {
	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "100ms"}, nil)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	ctx := context.Background()

	// Start service
	if err := hb.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if hb.Status() != StatusRunning {
		t.Errorf("Status() = %v, want %v", hb.Status(), StatusRunning)
	}

	// Verify start time was set
	if hb.startTime.IsZero() {
		t.Error("startTime should be set after Start()")
	}

	// Wait for at least one heartbeat tick
	time.Sleep(150 * time.Millisecond)

	// Stop service
	if err := hb.Stop(time.Second); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if hb.Status() != StatusStopped {
		t.Errorf("Status() = %v, want %v", hb.Status(), StatusStopped)
	}
}

func TestHeartbeatService_StartAlreadyRunning(t *testing.T) {
	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "1s"}, nil)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	ctx := context.Background()

	// Start service
	if err := hb.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer hb.Stop(time.Second)

	// Try to start again
	err = hb.Start(ctx)
	if err == nil {
		t.Error("Start() should return error when already running")
	}
}

func TestHeartbeatService_StopNotRunning(t *testing.T) {
	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "1s"}, nil)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	// Try to stop without starting
	err = hb.Stop(time.Second)
	if err == nil {
		t.Error("Stop() should return error when not running")
	}
}

func TestHeartbeatService_WithComponentManager(t *testing.T) {
	mockHealth := &mockComponentHealthGetter{
		health: map[string]bool{
			"component1": true,
			"component2": true,
			"component3": false,
		},
	}

	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "100ms"}, mockHealth)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	ctx := context.Background()

	if err := hb.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Let it emit a heartbeat
	time.Sleep(150 * time.Millisecond)

	if err := hb.Stop(time.Second); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// The test verifies no panics occur when component manager is present
	// Actual log output would need to be captured for verification
}

func TestHeartbeatService_ContextCancellation(t *testing.T) {
	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "1s"}, nil)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	if err := hb.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Cancel context - this will cause the heartbeat loop to exit
	cancel()

	// Give goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// After context cancellation, we need to clean up the ticker
	// The Stop() call may fail if status already changed, which is acceptable
	_ = hb.Stop(time.Second)

	// Verify the service is stopped (either by context or explicit stop)
	if hb.Status() != StatusStopped {
		t.Errorf("Status() = %v, want %v after context cancellation", hb.Status(), StatusStopped)
	}
}

func TestHeartbeatService_EmitHeartbeat(t *testing.T) {
	mockHealth := &mockComponentHealthGetter{
		health: map[string]bool{
			"comp1": true,
			"comp2": true,
			"comp3": false,
			"comp4": true,
		},
	}

	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "1s"}, mockHealth)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	hb.startTime = time.Now()

	// Call emitHeartbeat directly - should not panic
	hb.emitHeartbeat()

	// Test with nil component manager
	hb2, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "1s"}, nil)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}
	hb2.startTime = time.Now()
	hb2.emitHeartbeat() // Should not panic with nil componentManager
}

func TestHeartbeatService_Name(t *testing.T) {
	hb, err := newHeartbeatServiceForTest(&HeartbeatConfig{Interval: "1s"}, nil)
	if err != nil {
		t.Fatalf("newHeartbeatServiceForTest() error = %v", err)
	}

	if hb.Name() != "heartbeat" {
		t.Errorf("Name() = %v, want heartbeat", hb.Name())
	}
}
