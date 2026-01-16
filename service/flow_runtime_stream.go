package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/flowstore"
	"github.com/gorilla/websocket"
)

// StatusStreamEnvelope wraps all WebSocket status stream messages
type StatusStreamEnvelope struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp int64           `json:"timestamp"`
	FlowID    string          `json:"flow_id"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// SubscribeCommand represents a client subscription command
type SubscribeCommand struct {
	Command      string   `json:"command"`
	MessageTypes []string `json:"message_types,omitempty"`
	LogLevel     string   `json:"log_level,omitempty"`
	Sources      []string `json:"sources,omitempty"`
}

// ClientState represents client subscription state
type ClientState struct {
	messageTypes map[string]bool
	logLevel     string
	sources      map[string]bool
	mu           sync.RWMutex
}

// newClientState creates a new client state with default subscriptions
func newClientState() *ClientState {
	// Default: subscribe to all message types
	return &ClientState{
		messageTypes: map[string]bool{
			"flow_status":       true,
			"component_health":  true,
			"component_metrics": true,
			"log_entry":         true,
		},
		logLevel: "",
		sources:  make(map[string]bool),
	}
}

// IsSubscribed checks if the client is subscribed to a message type
func (cs *ClientState) IsSubscribed(messageType string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.messageTypes[messageType]
}

// UpdateSubscription updates the client's subscription filters
func (cs *ClientState) UpdateSubscription(messageTypes []string, logLevel string, sources []string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Update message types if provided
	if messageTypes != nil {
		cs.messageTypes = make(map[string]bool)
		for _, mt := range messageTypes {
			cs.messageTypes[mt] = true
		}
	}

	// Update log level
	cs.logLevel = logLevel

	// Update sources if provided
	if sources != nil {
		cs.sources = make(map[string]bool)
		for _, src := range sources {
			cs.sources[src] = true
		}
	}
}

// ShouldReceiveLogLevel checks if the client should receive logs at the given level
// Log level hierarchy: DEBUG=0 < INFO=1 < WARN=2 < ERROR=3
func (cs *ClientState) ShouldReceiveLogLevel(level string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// No filter = allow all
	if cs.logLevel == "" {
		return true
	}

	// Define level hierarchy
	levelOrder := map[string]int{
		"DEBUG": 0,
		"INFO":  1,
		"WARN":  2,
		"ERROR": 3,
	}

	minLevelValue, minExists := levelOrder[cs.logLevel]
	testLevelValue, testExists := levelOrder[level]

	if !minExists || !testExists {
		return true // Unknown levels pass through
	}

	return testLevelValue >= minLevelValue
}

// ShouldReceiveSource checks if the client should receive logs from the given source
func (cs *ClientState) ShouldReceiveSource(source string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// No filter = allow all
	if len(cs.sources) == 0 {
		return true
	}

	return cs.sources[source]
}

// handleStatusWebSocketImpl is the actual implementation of WebSocket status streaming
// This replaces the stub in flow_service.go
func (fs *FlowService) handleStatusWebSocketImpl(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get flowId from query parameter
	flowID := r.URL.Query().Get("flowId")

	// Validate flowId (required)
	if flowID == "" {
		fs.logger.Warn("WebSocket upgrade failed: missing flowId parameter")
		http.Error(w, "Missing flowId parameter", http.StatusBadRequest)
		return
	}

	// Validate flowId to prevent NATS injection (same as runtime logs endpoint)
	if strings.ContainsAny(flowID, ">*.") {
		fs.logger.Warn("WebSocket upgrade failed: invalid flowId (NATS injection attempt)", "flow_id", flowID)
		http.Error(w, "Invalid flowId", http.StatusBadRequest)
		return
	}

	// Verify flow exists
	_, err := fs.flowStore.Get(ctx, flowID)
	if err != nil {
		fs.logger.Error("WebSocket upgrade failed: flow not found", "flow_id", flowID, "error", err)
		http.Error(w, "Flow not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			// Allow connections from any origin for development
			// In production, this should be more restrictive
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fs.logger.Error("WebSocket upgrade failed", "flow_id", flowID, "error", err)
		return
	}

	fs.logger.Info("WebSocket client connected", "flow_id", flowID, "remote_addr", r.RemoteAddr)

	// Handle the WebSocket connection
	defer conn.Close()

	// Create client state for this connection
	clientState := newClientState()

	// Create context for worker goroutines
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	// Create channel for sending envelopes to the client
	envelopeChan := make(chan StatusStreamEnvelope, 100)

	// Start workers and goroutines
	var wg sync.WaitGroup
	fs.startWebSocketWorkers(workerCtx, clientState, flowID, envelopeChan, &wg)
	fs.startWebSocketWriter(workerCtx, conn, envelopeChan, &wg)
	fs.startWebSocketReader(workerCtx, conn, clientState, workerCancel, &wg)

	// Wait for workers to finish (on disconnect or error)
	wg.Wait()
	fs.logger.Info("WebSocket client disconnected", "flow_id", flowID)
}

// FlowStore interface for dependency injection in tests
type FlowStore interface {
	Get(ctx context.Context, id string) (*flowstore.Flow, error)
	List(ctx context.Context) ([]*flowstore.Flow, error)
	Create(ctx context.Context, flow *flowstore.Flow) error
	Update(ctx context.Context, flow *flowstore.Flow) error
	Delete(ctx context.Context, id string) error
}

// ComponentHealthProvider interface for dependency injection in tests
type ComponentHealthProvider interface {
	GetManagedComponents() map[string]*component.ManagedComponent
}

// natsSubscriber interface for NATS subscription with wildcard subjects
type natsSubscriber interface {
	Subscribe(ctx context.Context, subject string, handler func(context.Context, []byte)) error
}

// generateMessageID generates a unique message ID for envelopes
func generateMessageID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000")))
	}
	return hex.EncodeToString(bytes)
}

// healthTicker polls ComponentManager for health updates and sends envelopes
func healthTicker(
	ctx context.Context,
	clientState *ClientState,
	componentMgr ComponentHealthProvider,
	flowID string,
	sendFn func(StatusStreamEnvelope) error,
) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if client is subscribed to component_health
			if !clientState.IsSubscribed("component_health") {
				continue
			}

			// Get health from component manager
			components := componentMgr.GetManagedComponents()

			// Build health payload
			health := make(map[string]interface{})
			for name, mc := range components {
				if mc.Component != nil {
					health[name] = mc.Component.Health()
				}
			}

			// Marshal payload
			payload, err := json.Marshal(health)
			if err != nil {
				slog.Debug("Failed to marshal health payload", "error", err)
				continue
			}

			// Create envelope
			envelope := StatusStreamEnvelope{
				Type:      "component_health",
				ID:        generateMessageID(),
				Timestamp: time.Now().UnixMilli(),
				FlowID:    flowID,
				Payload:   json.RawMessage(payload),
			}

			// Send envelope
			if err := sendFn(envelope); err != nil {
				slog.Debug("Failed to send health envelope", "error", err)
			}
		}
	}
}

// flowWatcher polls FlowStore for state changes and sends envelopes
func flowWatcher(
	ctx context.Context,
	clientState *ClientState,
	flowStore FlowStore,
	flowID string,
	sendFn func(StatusStreamEnvelope) error,
) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var prevState flowstore.RuntimeState

	// Get initial state
	flow, err := flowStore.Get(ctx, flowID)
	if err == nil {
		prevState = flow.RuntimeState
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if client is subscribed to flow_status
			if !clientState.IsSubscribed("flow_status") {
				continue
			}

			// Get current flow state
			flow, err := flowStore.Get(ctx, flowID)
			if err != nil {
				slog.Debug("Failed to get flow state", "flow_id", flowID, "error", err)
				continue
			}

			// Only send if state changed
			if flow.RuntimeState == prevState {
				continue
			}

			// Build payload
			payload := map[string]interface{}{
				"state":      string(flow.RuntimeState),
				"prev_state": string(prevState),
				"timestamp":  time.Now().UnixMilli(),
			}

			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				slog.Debug("Failed to marshal flow status payload", "error", err)
				continue
			}

			// Create envelope
			envelope := StatusStreamEnvelope{
				Type:      "flow_status",
				ID:        generateMessageID(),
				Timestamp: time.Now().UnixMilli(),
				FlowID:    flowID,
				Payload:   json.RawMessage(payloadBytes),
			}

			// Send envelope
			if err := sendFn(envelope); err != nil {
				slog.Debug("Failed to send flow status envelope", "error", err)
			}

			// Update previous state
			prevState = flow.RuntimeState
		}
	}
}

// logStreamer subscribes to NATS logs.> and forwards log entries as envelopes
func logStreamer(
	ctx context.Context,
	clientState *ClientState,
	natsClient natsSubscriber,
	flowID string,
	sendFn func(StatusStreamEnvelope) error,
) {
	// Subscribe to logs.>
	err := natsClient.Subscribe(ctx, "logs.>", func(_ context.Context, data []byte) {
		// Check if client is subscribed to log_entry
		if !clientState.IsSubscribed("log_entry") {
			return
		}

		// Parse log entry
		var logEntry map[string]interface{}
		if err := json.Unmarshal(data, &logEntry); err != nil {
			slog.Debug("Failed to unmarshal log entry", "error", err)
			return
		}

		// Extract level and source
		level, _ := logEntry["level"].(string)
		source, _ := logEntry["source"].(string)

		// Check log level filter
		if !clientState.ShouldReceiveLogLevel(level) {
			return
		}

		// Check source filter
		if !clientState.ShouldReceiveSource(source) {
			return
		}

		// Create envelope
		envelope := StatusStreamEnvelope{
			Type:      "log_entry",
			ID:        generateMessageID(),
			Timestamp: time.Now().UnixMilli(),
			FlowID:    flowID,
			Payload:   json.RawMessage(data),
		}

		// Send envelope
		if err := sendFn(envelope); err != nil {
			slog.Debug("Failed to send log envelope", "error", err)
		}
	})

	if err != nil {
		slog.Error("Failed to subscribe to logs", "error", err)
		return
	}

	// Wait for context cancellation
	<-ctx.Done()
}

// metricsStreamer subscribes to NATS metrics.> and forwards metrics as envelopes
func metricsStreamer(
	ctx context.Context,
	clientState *ClientState,
	natsClient natsSubscriber,
	flowID string,
	sendFn func(StatusStreamEnvelope) error,
) {
	// Subscribe to metrics.>
	err := natsClient.Subscribe(ctx, "metrics.>", func(_ context.Context, data []byte) {
		// Check if client is subscribed to component_metrics
		if !clientState.IsSubscribed("component_metrics") {
			return
		}

		// Create envelope
		envelope := StatusStreamEnvelope{
			Type:      "component_metrics",
			ID:        generateMessageID(),
			Timestamp: time.Now().UnixMilli(),
			FlowID:    flowID,
			Payload:   json.RawMessage(data),
		}

		// Send envelope
		if err := sendFn(envelope); err != nil {
			slog.Debug("Failed to send metrics envelope", "error", err)
		}
	})

	if err != nil {
		slog.Error("Failed to subscribe to metrics", "error", err)
		return
	}

	// Wait for context cancellation
	<-ctx.Done()
}

// startWebSocketWorkers starts all worker goroutines for the WebSocket connection
func (fs *FlowService) startWebSocketWorkers(
	ctx context.Context,
	clientState *ClientState,
	flowID string,
	envelopeChan chan StatusStreamEnvelope,
	wg *sync.WaitGroup,
) {
	// Create send function that writes to the envelope channel
	sendFn := func(envelope StatusStreamEnvelope) error {
		select {
		case envelopeChan <- envelope:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Channel full, drop message to avoid blocking workers
			fs.logger.Debug("Envelope channel full, dropping message", "type", envelope.Type)
			return nil
		}
	}

	// Health ticker - polls ComponentManager every 5s
	if fs.componentManager != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			healthTicker(ctx, clientState, fs.componentManager, flowID, sendFn)
		}()
	}

	// Flow watcher - polls FlowStore for state changes every 1s
	wg.Add(1)
	go func() {
		defer wg.Done()
		flowWatcher(ctx, clientState, fs.flowStore, flowID, sendFn)
	}()

	// Log streamer - subscribes to NATS logs.>
	if fs.natsClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logStreamer(ctx, clientState, fs.natsClient, flowID, sendFn)
		}()
	}

	// Metrics streamer - subscribes to NATS metrics.>
	if fs.natsClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			metricsStreamer(ctx, clientState, fs.natsClient, flowID, sendFn)
		}()
	}
}

// startWebSocketWriter starts the goroutine that writes envelopes to the WebSocket
func (fs *FlowService) startWebSocketWriter(
	ctx context.Context,
	conn *websocket.Conn,
	envelopeChan chan StatusStreamEnvelope,
	wg *sync.WaitGroup,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case envelope := <-envelopeChan:
				// Marshal envelope to JSON
				data, err := json.Marshal(envelope)
				if err != nil {
					fs.logger.Debug("Failed to marshal envelope", "error", err)
					continue
				}

				// Write to WebSocket
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					fs.logger.Debug("Failed to write to WebSocket", "error", err)
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()
}

// startWebSocketReader starts the goroutine that reads from the WebSocket
func (fs *FlowService) startWebSocketReader(
	ctx context.Context,
	conn *websocket.Conn,
	clientState *ClientState,
	cancelFunc context.CancelFunc,
	wg *sync.WaitGroup,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Monitor context cancellation - close connection to unblock ReadMessage
		go func() {
			<-ctx.Done()
			conn.Close()
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				// Check if this was a context cancellation
				if ctx.Err() != nil {
					fs.logger.Debug("WebSocket reader stopped due to context cancellation")
					return
				}
				// Client disconnected or error
				fs.logger.Debug("WebSocket read error, closing connection", "error", err)
				cancelFunc()
				return
			}

			// Handle client command
			fs.handleWebSocketCommand(message, clientState)
		}
	}()
}

// handleWebSocketCommand processes a client command from the WebSocket
func (fs *FlowService) handleWebSocketCommand(message []byte, clientState *ClientState) {
	// Parse the command
	var cmd SubscribeCommand
	if err := json.Unmarshal(message, &cmd); err != nil {
		// Malformed JSON - log and ignore (don't crash)
		fs.logger.Debug("WebSocket received malformed JSON command", "error", err)
		return
	}

	// Handle subscribe command
	if cmd.Command == "subscribe" {
		clientState.UpdateSubscription(cmd.MessageTypes, cmd.LogLevel, cmd.Sources)
		fs.logger.Debug("WebSocket client updated subscription",
			"message_types", cmd.MessageTypes,
			"log_level", cmd.LogLevel,
			"sources", cmd.Sources)
		return
	}

	// Unknown command - log and ignore (don't crash)
	if cmd.Command != "" {
		fs.logger.Debug("WebSocket received unknown command", "command", cmd.Command)
	}
}
