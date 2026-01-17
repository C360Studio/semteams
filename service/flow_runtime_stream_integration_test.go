//go:build integration

package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/flowstore"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/logging"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock types for ComponentHealth testing ---

// mockComponentHealthProvider implements ComponentHealthProvider for testing
type mockComponentHealthProvider struct {
	components map[string]*component.ManagedComponent
}

func (m *mockComponentHealthProvider) GetManagedComponents() map[string]*component.ManagedComponent {
	return m.components
}

// mockHealthComponent implements component.Discoverable for health testing
type mockHealthComponent struct {
	name   string
	health component.HealthStatus
}

func (m *mockHealthComponent) Meta() component.Metadata {
	return component.Metadata{
		Name:        m.name,
		Type:        "input",
		Description: "Mock component for testing",
		Version:     "1.0.0",
	}
}

func (m *mockHealthComponent) InputPorts() []component.Port {
	return nil
}

func (m *mockHealthComponent) OutputPorts() []component.Port {
	return []component.Port{{Name: "output", Direction: component.DirectionOutput}}
}

func (m *mockHealthComponent) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{}
}

func (m *mockHealthComponent) Health() component.HealthStatus {
	return m.health
}

func (m *mockHealthComponent) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{}
}

// TestWebSocketStatusStream_ReceivesMetrics verifies metrics flow through WebSocket
func TestWebSocketStatusStream_ReceivesMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get shared NATS client
	natsClient := getSharedNATSClient(t)

	// Create flow store and test flow
	flowStore, err := flowstore.NewStore(natsClient)
	require.NoError(t, err)

	flowID := createTestFlowForStream(t, ctx, flowStore)
	defer func() { _ = flowStore.Delete(ctx, flowID) }()

	// Create FlowService
	fs := createTestFlowServiceForStream(t, natsClient, flowStore)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/flowbuilder/status/stream", fs.handleStatusWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect WebSocket
	conn := connectTestWebSocket(t, server, flowID)
	defer conn.Close()

	// Give the server-side goroutines time to set up NATS subscriptions
	time.Sleep(100 * time.Millisecond)

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Publish a metric directly to NATS
	metricData := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
		"name":      "test_counter",
		"component": "test-component",
		"type":      "counter",
		"value":     42.0,
		"labels":    map[string]string{"status": "success"},
	}
	metricBytes, err := json.Marshal(metricData)
	require.NoError(t, err)

	err = natsClient.Publish(ctx, "metrics.test-component.test_counter", metricBytes)
	require.NoError(t, err)

	// Wait for metrics envelope
	envelope := waitForEnvelopeType(t, conn, "component_metrics", 5*time.Second)
	require.NotNil(t, envelope, "Should receive component_metrics envelope")

	assert.Equal(t, "component_metrics", envelope.Type)
	assert.Equal(t, flowID, envelope.FlowID)
	assert.NotEmpty(t, envelope.ID)
	assert.Greater(t, envelope.Timestamp, int64(0))
}

// TestWebSocketStatusStream_ReceivesLogs verifies logs flow through WebSocket via NATSLogHandler
func TestWebSocketStatusStream_ReceivesLogs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get shared NATS client
	natsClient := getSharedNATSClient(t)

	// Create flow store and test flow
	flowStore, err := flowstore.NewStore(natsClient)
	require.NoError(t, err)

	flowID := createTestFlowForStream(t, ctx, flowStore)
	defer func() { _ = flowStore.Delete(ctx, flowID) }()

	// Create NATSLogHandler for publishing logs to NATS
	natsHandler := logging.NewNATSLogHandler(natsClient, logging.NATSLogHandlerConfig{
		MinLevel:       slog.LevelDebug,
		ExcludeSources: nil,
	})

	// Create FlowService
	fs := createTestFlowServiceForStream(t, natsClient, flowStore)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/flowbuilder/status/stream", fs.handleStatusWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect WebSocket
	conn := connectTestWebSocket(t, server, flowID)
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Generate a log record and send it directly through NATSLogHandler
	// This simulates what happens when slog is called with NATSLogHandler in the handler chain
	testRecord := slog.NewRecord(time.Now(), slog.LevelInfo, "Integration test log message", 0)
	testRecord.AddAttrs(
		slog.String("component", "integration-test"),
		slog.String("test_id", "log-flow-test"),
	)
	err = natsHandler.Handle(ctx, testRecord)
	require.NoError(t, err)

	// Wait for our specific log message
	envelope := waitForLogMessage(t, conn, "Integration test log message", 5*time.Second)
	require.NotNil(t, envelope, "Should receive log_entry envelope with our message")

	assert.Equal(t, "log_entry", envelope.Type)
	assert.Equal(t, flowID, envelope.FlowID)

	// Verify payload contains our log message
	var payload map[string]interface{}
	err = json.Unmarshal(envelope.Payload, &payload)
	require.NoError(t, err)

	assert.Equal(t, "INFO", payload["level"])
	assert.Contains(t, payload["message"], "Integration test log message")
}

// TestWebSocketStatusStream_ReceivesFlowStatus verifies flow state changes are sent
func TestWebSocketStatusStream_ReceivesFlowStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get shared NATS client
	natsClient := getSharedNATSClient(t)

	// Create flow store
	flowStore, err := flowstore.NewStore(natsClient)
	require.NoError(t, err)

	// Create test flow in initial state
	flowID := createTestFlowForStream(t, ctx, flowStore)
	defer func() { _ = flowStore.Delete(ctx, flowID) }()

	// Create FlowService
	fs := createTestFlowServiceForStream(t, natsClient, flowStore)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/flowbuilder/status/stream", fs.handleStatusWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect WebSocket
	conn := connectTestWebSocket(t, server, flowID)
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Update flow state - this should trigger flow_status message
	flow, err := flowStore.Get(ctx, flowID)
	require.NoError(t, err)

	flow.RuntimeState = flowstore.StateRunning
	err = flowStore.Update(ctx, flow)
	require.NoError(t, err)

	// Wait for flow_status envelope (flowWatcher polls every 1 second)
	envelope := waitForEnvelopeType(t, conn, "flow_status", 3*time.Second)
	require.NotNil(t, envelope, "Should receive flow_status envelope")

	assert.Equal(t, "flow_status", envelope.Type)
	assert.Equal(t, flowID, envelope.FlowID)

	// Verify payload
	var payload map[string]interface{}
	err = json.Unmarshal(envelope.Payload, &payload)
	require.NoError(t, err)

	assert.Equal(t, string(flowstore.StateRunning), payload["state"])
}

// TestWebSocketStatusStream_ReceivesComponentHealth verifies component health flows through WebSocket
func TestWebSocketStatusStream_ReceivesComponentHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get shared NATS client
	natsClient := getSharedNATSClient(t)

	// Create flow store and test flow
	flowStore, err := flowstore.NewStore(natsClient)
	require.NoError(t, err)

	flowID := createTestFlowForStream(t, ctx, flowStore)
	defer func() { _ = flowStore.Delete(ctx, flowID) }()

	// Create mock ComponentHealthProvider with test components
	mockProvider := &mockComponentHealthProvider{
		components: map[string]*component.ManagedComponent{
			"test-input": {
				Component: &mockHealthComponent{
					name: "test-input",
					health: component.HealthStatus{
						Healthy:    true,
						Status:     "running",
						ErrorCount: 0,
					},
				},
			},
			"test-processor": {
				Component: &mockHealthComponent{
					name: "test-processor",
					health: component.HealthStatus{
						Healthy:    true,
						Status:     "processing",
						ErrorCount: 2,
					},
				},
			},
		},
	}

	// Create FlowService with componentManager set
	fs := createTestFlowServiceForStream(t, natsClient, flowStore)
	fs.componentManager = mockProvider

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/flowbuilder/status/stream", fs.handleStatusWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect WebSocket
	conn := connectTestWebSocket(t, server, flowID)
	defer conn.Close()

	// Wait for component_health envelope (healthTicker runs every 5s, but first tick is immediate)
	// We need to wait up to 6 seconds to catch the first tick
	envelope := waitForEnvelopeType(t, conn, "component_health", 6*time.Second)
	require.NotNil(t, envelope, "Should receive component_health envelope")

	assert.Equal(t, "component_health", envelope.Type)
	assert.Equal(t, flowID, envelope.FlowID)
	assert.NotEmpty(t, envelope.ID)
	assert.Greater(t, envelope.Timestamp, int64(0))

	// Verify payload contains health data for our test components
	var payload map[string]interface{}
	err = json.Unmarshal(envelope.Payload, &payload)
	require.NoError(t, err)

	// Check test-input health
	testInput, ok := payload["test-input"].(map[string]interface{})
	require.True(t, ok, "Should have test-input in health payload")
	assert.Equal(t, true, testInput["healthy"])
	assert.Equal(t, "running", testInput["status"])

	// Check test-processor health
	testProcessor, ok := payload["test-processor"].(map[string]interface{})
	require.True(t, ok, "Should have test-processor in health payload")
	assert.Equal(t, true, testProcessor["healthy"])
	assert.Equal(t, "processing", testProcessor["status"])
}

// TestWebSocketStatusStream_LogsNotReceivedWhenLogForwarderDisabled verifies no logs without LogForwarder
func TestWebSocketStatusStream_LogsNotReceivedWhenLogForwarderDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get shared NATS client
	natsClient := getSharedNATSClient(t)

	// Create flow store and test flow
	flowStore, err := flowstore.NewStore(natsClient)
	require.NoError(t, err)

	flowID := createTestFlowForStream(t, ctx, flowStore)
	defer func() { _ = flowStore.Delete(ctx, flowID) }()

	// Create FlowService WITHOUT LogForwarder wired to slog
	fs := createTestFlowServiceForStream(t, natsClient, flowStore)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/flowbuilder/status/stream", fs.handleStatusWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect WebSocket
	conn := connectTestWebSocket(t, server, flowID)
	defer conn.Close()

	// Generate a log message - without LogForwarder, this won't reach NATS
	slog.Info("This log should NOT reach WebSocket", "component", "test")

	// Try to receive - should timeout because no LogForwarder is publishing to NATS
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	envelope := waitForEnvelopeType(t, conn, "log_entry", 2*time.Second)

	// We should NOT receive a log_entry (might receive metrics though)
	if envelope != nil && envelope.Type == "log_entry" {
		// Check if it's our message
		var payload map[string]interface{}
		_ = json.Unmarshal(envelope.Payload, &payload)
		if msg, ok := payload["message"].(string); ok {
			assert.NotContains(t, msg, "This log should NOT reach WebSocket",
				"Log should not reach WebSocket without LogForwarder")
		}
	}
}

// TestWebSocketStatusStream_ExcludeSourcesFiltering verifies excluded sources don't publish
func TestWebSocketStatusStream_ExcludeSourcesFiltering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get shared NATS client
	natsClient := getSharedNATSClient(t)

	// Create flow store and test flow
	flowStore, err := flowstore.NewStore(natsClient)
	require.NoError(t, err)

	flowID := createTestFlowForStream(t, ctx, flowStore)
	defer func() { _ = flowStore.Delete(ctx, flowID) }()

	// Create NATSLogHandler with exclude_sources
	natsHandler := logging.NewNATSLogHandler(natsClient, logging.NATSLogHandlerConfig{
		MinLevel:       slog.LevelDebug,
		ExcludeSources: []string{"excluded-source"},
	})

	// Create FlowService
	fs := createTestFlowServiceForStream(t, natsClient, flowStore)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/flowbuilder/status/stream", fs.handleStatusWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect WebSocket
	conn := connectTestWebSocket(t, server, flowID)
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Send log records directly through NATSLogHandler
	// Log from excluded source - should NOT be forwarded to NATS
	excludedRecord := slog.NewRecord(time.Now(), slog.LevelInfo, "Excluded log message", 0)
	excludedRecord.AddAttrs(slog.String("source", "excluded-source"))
	_ = natsHandler.Handle(ctx, excludedRecord)

	// Log from allowed source - should be forwarded
	allowedRecord := slog.NewRecord(time.Now(), slog.LevelInfo, "Allowed log message", 0)
	allowedRecord.AddAttrs(slog.String("source", "allowed-source"))
	_ = natsHandler.Handle(ctx, allowedRecord)

	// Collect envelopes for a short time
	receivedLogs := collectEnvelopesOfType(t, conn, "log_entry", 3*time.Second)

	// Should receive the allowed log but NOT the excluded one
	var foundAllowed, foundExcluded bool
	for _, env := range receivedLogs {
		var payload map[string]interface{}
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			continue
		}
		if msg, ok := payload["message"].(string); ok {
			if strings.Contains(msg, "Allowed log message") {
				foundAllowed = true
			}
			if strings.Contains(msg, "Excluded log message") {
				foundExcluded = true
			}
		}
	}

	assert.True(t, foundAllowed, "Should receive log from allowed source")
	assert.False(t, foundExcluded, "Should NOT receive log from excluded source")
}

// --- Test Helpers ---

// createTestFlowForStream creates a test flow in the flow store
func createTestFlowForStream(t *testing.T, ctx context.Context, store *flowstore.Store) string {
	t.Helper()

	flowID := "test-flow-" + time.Now().Format("20060102150405")
	flow := &flowstore.Flow{
		ID:           flowID,
		Name:         "Test Flow",
		Version:      1,
		RuntimeState: flowstore.StateDeployedStopped,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := store.Create(ctx, flow)
	require.NoError(t, err)

	return flowID
}

// createTestFlowServiceForStream creates a FlowService for integration tests
func createTestFlowServiceForStream(t *testing.T, natsClient *natsclient.Client, flowStore *flowstore.Store) *FlowService {
	t.Helper()

	baseService := NewBaseServiceWithOptions(
		"test-flow-service",
		nil,
		WithNATS(natsClient),
		WithLogger(slog.Default()),
	)

	return &FlowService{
		BaseService: baseService,
		flowStore:   flowStore,
		natsClient:  natsClient,
		config: FlowServiceConfig{
			LogStreamBufferSize: 100,
		},
	}
}

// connectTestWebSocket connects to the WebSocket status stream endpoint
func connectTestWebSocket(t *testing.T, server *httptest.Server, flowID string) *websocket.Conn {
	t.Helper()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/flowbuilder/status/stream?flowId=" + flowID

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "WebSocket connection should succeed")
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	return conn
}

// waitForEnvelopeType waits for an envelope of a specific type
func waitForEnvelopeType(t *testing.T, conn *websocket.Conn, msgType string, timeout time.Duration) *StatusStreamEnvelope {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)

		_, data, err := conn.ReadMessage()
		if err != nil {
			// Timeout or connection closed
			return nil
		}

		var envelope StatusStreamEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}

		if envelope.Type == msgType {
			return &envelope
		}
	}

	return nil
}

// collectEnvelopesOfType collects all envelopes of a specific type within timeout
func collectEnvelopesOfType(t *testing.T, conn *websocket.Conn, msgType string, timeout time.Duration) (envelopes []StatusStreamEnvelope) {
	t.Helper()

	// Recover from panics caused by reading from failed websocket connections
	defer func() {
		if r := recover(); r != nil {
			// Connection was in a failed state, return what we have
			t.Logf("WebSocket read recovered from panic: %v", r)
		}
	}()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			// Connection already closed
			break
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			// Check for any kind of close or permanent error
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			// Check if it's a network timeout (expected, continue trying)
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}
			// Check for "use of closed network connection" type errors
			if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "failed") {
				break
			}
			// Other transient error, continue trying until deadline
			continue
		}

		var envelope StatusStreamEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}

		if envelope.Type == msgType {
			envelopes = append(envelopes, envelope)
		}
	}

	return envelopes
}

// waitForLogMessage waits for a log_entry envelope containing a specific message
func waitForLogMessage(t *testing.T, conn *websocket.Conn, messageSubstr string, timeout time.Duration) *StatusStreamEnvelope {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)

		_, data, err := conn.ReadMessage()
		if err != nil {
			// Timeout or connection closed
			return nil
		}

		var envelope StatusStreamEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}

		if envelope.Type == "log_entry" {
			var payload map[string]interface{}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				continue
			}
			if msg, ok := payload["message"].(string); ok {
				if strings.Contains(msg, messageSubstr) {
					return &envelope
				}
			}
		}
	}

	return nil
}
