package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestServeHeartbeat_Success(t *testing.T) {
	// Set up receiver with heartbeat data from testdata
	receiver := NewReceiver()
	frameData, err := os.ReadFile("testdata/heartbeat.bin")
	if err != nil {
		t.Fatalf("failed to read testdata/heartbeat.bin: %v", err)
	}

	err = receiver.ParseHeartbeat(frameData)
	if err != nil {
		t.Fatalf("ParseHeartbeat failed: %v", err)
	}

	server := NewServer(receiver)

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/heartbeat", nil)
	w := httptest.NewRecorder()

	server.ServeHeartbeat(w, req)

	// Check response status
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Check Content-Type
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %s", w.Header().Get("Content-Type"))
	}

	// Parse response body
	body := w.Body.String()
	var response HeartbeatResponse
	err = json.Unmarshal([]byte(body), &response)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %v, body: %s", err, body)
	}

	// Verify fields
	if response.SystemID != 1 {
		t.Errorf("expected system_id=1, got %d", response.SystemID)
	}
	if response.ComponentID != 1 {
		t.Errorf("expected component_id=1, got %d", response.ComponentID)
	}
	if response.AutopilotType != 3 {
		t.Errorf("expected autopilot_type=3, got %d", response.AutopilotType)
	}
	if response.BaseMode != 81 {
		t.Errorf("expected base_mode=81, got %d", response.BaseMode)
	}
	if response.ReceivedAt == "" {
		t.Error("expected non-empty received_at")
	}
}

func TestServeHeartbeat_NoData(t *testing.T) {
	receiver := NewReceiver()
	server := NewServer(receiver)

	req := httptest.NewRequest(http.MethodGet, "/heartbeat", nil)
	w := httptest.NewRecorder()

	server.ServeHeartbeat(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

func TestServeHeartbeat_MethodNotAllowed(t *testing.T) {
	receiver := NewReceiver()
	server := NewServer(receiver)

	req := httptest.NewRequest(http.MethodPost, "/heartbeat", nil)
	w := httptest.NewRecorder()

	server.ServeHeartbeat(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHeartbeatResponseJSON(t *testing.T) {
	// Set up receiver with heartbeat data
	receiver := NewReceiver()
	frameData, err := os.ReadFile("testdata/heartbeat.bin")
	if err != nil {
		t.Fatalf("failed to read testdata/heartbeat.bin: %v", err)
	}

	err = receiver.ParseHeartbeat(frameData)
	if err != nil {
		t.Fatalf("ParseHeartbeat failed: %v", err)
	}

	server := NewServer(receiver)

	// Use http.Server and a test client to ensure proper JSON encoding
	ts := httptest.NewServer(http.HandlerFunc(server.ServeHeartbeat))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var response HeartbeatResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %v, body: %s", err, string(body))
	}

	// Verify all required fields are present
	if response.SystemID != 1 {
		t.Errorf("expected system_id=1, got %d", response.SystemID)
	}
	if response.ComponentID != 1 {
		t.Errorf("expected component_id=1, got %d", response.ComponentID)
	}
	if response.AutopilotType != 3 {
		t.Errorf("expected autopilot_type=3, got %d", response.AutopilotType)
	}
	if response.BaseMode != 81 {
		t.Errorf("expected base_mode=81, got %d", response.BaseMode)
	}
	if response.ReceivedAt == "" {
		t.Error("expected non-empty received_at")
	}
}
