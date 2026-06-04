package main

import (
	"os"
	"testing"
	"time"
)

func TestParseHeartbeat(t *testing.T) {
	// Read the testdata/heartbeat.bin file.
	frameData, err := os.ReadFile("testdata/heartbeat.bin")
	if err != nil {
		t.Fatalf("failed to read testdata/heartbeat.bin: %v", err)
	}

	r := NewReceiver()
	err = r.ParseHeartbeat(frameData)
	if err != nil {
		t.Fatalf("ParseHeartbeat failed: %v", err)
	}

	hb := r.GetHeartbeat()
	if hb == nil {
		t.Fatal("heartbeat is nil after parsing")
	}

	// Verify the parsed fields match what we expect.
	if hb.SystemID != 1 {
		t.Errorf("expected system_id=1, got %d", hb.SystemID)
	}
	if hb.ComponentID != 1 {
		t.Errorf("expected component_id=1, got %d", hb.ComponentID)
	}
	if hb.AutopilotType != 3 {
		t.Errorf("expected autopilot_type=3, got %d", hb.AutopilotType)
	}
	if hb.BaseMode != 81 {
		t.Errorf("expected base_mode=81, got %d", hb.BaseMode)
	}
	if hb.ReceivedAt.IsZero() {
		t.Errorf("received_at should not be zero")
	}
}

func TestGetHeartbeat(t *testing.T) {
	r := NewReceiver()
	
	// Initially, no heartbeat should exist.
	if r.GetHeartbeat() != nil {
		t.Fatal("expected nil heartbeat initially")
	}

	// Parse a heartbeat and verify it's returned.
	frameData, err := os.ReadFile("testdata/heartbeat.bin")
	if err != nil {
		t.Fatalf("failed to read testdata/heartbeat.bin: %v", err)
	}

	err = r.ParseHeartbeat(frameData)
	if err != nil {
		t.Fatalf("ParseHeartbeat failed: %v", err)
	}

	hb := r.GetHeartbeat()
	if hb == nil {
		t.Fatal("expected non-nil heartbeat after parsing")
	}

	// Verify the copy is independent.
	originalTime := hb.ReceivedAt
	time.Sleep(10 * time.Millisecond)

	hb2 := r.GetHeartbeat()
	if hb2.ReceivedAt != originalTime {
		t.Errorf("ReceivedAt should be consistent, got %v vs %v", originalTime, hb2.ReceivedAt)
	}
}
