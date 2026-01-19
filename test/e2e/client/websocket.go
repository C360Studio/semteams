// Package client provides HTTP and WebSocket clients for SemStreams E2E tests
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketClient handles WebSocket connections for status streaming
type WebSocketClient struct {
	baseURL string
	dialer  *websocket.Dialer
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(baseURL string) *WebSocketClient {
	return &WebSocketClient{
		baseURL: baseURL,
		dialer:  websocket.DefaultDialer,
	}
}

// StatusStreamEnvelope matches service.StatusStreamEnvelope
type StatusStreamEnvelope struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp int64           `json:"timestamp"`
	FlowID    string          `json:"flow_id"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// SubscribeCommand matches service.SubscribeCommand
type SubscribeCommand struct {
	Command      string   `json:"command"`
	MessageTypes []string `json:"message_types,omitempty"`
	LogLevel     string   `json:"log_level,omitempty"`
	Sources      []string `json:"sources,omitempty"`
}

// SubscribeAck matches service.SubscribeAck (Server → Client acknowledgment)
type SubscribeAck struct {
	Type       string   `json:"type"`       // Always "subscribe_ack"
	Subscribed []string `json:"subscribed"` // Message types now subscribed
	LogLevel   string   `json:"log_level"`  // Current log level filter
	Sources    []string `json:"sources"`    // Current source filters
}

// WatchStatusStreamOpts configures status stream watching
type WatchStatusStreamOpts struct {
	Timeout       time.Duration
	MessageTypes  []string      // Filter: flow_status, component_health, component_metrics, log_entry
	LogLevel      string        // Minimum: DEBUG, INFO, WARN, ERROR
	DrainDuration time.Duration // Continue collecting after condition met to capture burst
}

// DefaultWatchStatusStreamOpts returns sensible defaults
func DefaultWatchStatusStreamOpts() WatchStatusStreamOpts {
	return WatchStatusStreamOpts{
		Timeout:      60 * time.Second,
		MessageTypes: []string{"flow_status", "component_health", "component_metrics", "log_entry"},
		LogLevel:     "DEBUG",
	}
}

// WatchStatusStreamCondition evaluates received envelopes
type WatchStatusStreamCondition func(envelopes []StatusStreamEnvelope) (satisfied bool, err error)

// WatchStatusStream connects to WebSocket and collects envelopes until condition is met
func (c *WebSocketClient) WatchStatusStream(
	ctx context.Context,
	flowID string,
	condition WatchStatusStreamCondition,
	opts WatchStatusStreamOpts,
) ([]StatusStreamEnvelope, error) {
	// Apply defaults
	if opts.Timeout == 0 {
		opts.Timeout = DefaultWatchStatusStreamOpts().Timeout
	}

	// Build WebSocket URL
	wsURL, err := c.buildWSURL(flowID)
	if err != nil {
		return nil, err
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Connect
	conn, _, err := c.dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	// Send subscribe command if filtering
	if len(opts.MessageTypes) > 0 || opts.LogLevel != "" {
		cmd := SubscribeCommand{
			Command:      "subscribe",
			MessageTypes: opts.MessageTypes,
			LogLevel:     opts.LogLevel,
		}
		if err := conn.WriteJSON(cmd); err != nil {
			return nil, fmt.Errorf("failed to send subscribe command: %w", err)
		}
	}

	// Read envelopes until condition satisfied
	return c.readUntilCondition(ctx, conn, condition, opts)
}

// Health checks if WebSocket endpoint is available by attempting a connection
func (c *WebSocketClient) Health(ctx context.Context, flowID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	wsURL, err := c.buildWSURL(flowID)
	if err != nil {
		return err
	}

	conn, _, err := c.dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket health check failed: %w", err)
	}
	conn.Close()

	return nil
}

func (c *WebSocketClient) buildWSURL(flowID string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Convert scheme: http -> ws, https -> wss
	scheme := "ws"
	if base.Scheme == "https" {
		scheme = "wss"
	}

	return fmt.Sprintf("%s://%s/flowbuilder/status/stream?flowId=%s",
		scheme, base.Host, url.QueryEscape(flowID)), nil
}

func (c *WebSocketClient) readUntilCondition(
	ctx context.Context,
	conn *websocket.Conn,
	condition WatchStatusStreamCondition,
	opts WatchStatusStreamOpts,
) ([]StatusStreamEnvelope, error) {
	var collected []StatusStreamEnvelope
	var drainDeadline time.Time
	draining := false

	for {
		select {
		case <-ctx.Done():
			// If we're draining and context expired, return what we have (success)
			if draining {
				return collected, nil
			}
			return collected, ctx.Err()
		default:
		}

		// Set read deadline - use drain deadline if draining, otherwise context deadline
		var deadline time.Time
		if draining {
			deadline = drainDeadline
		} else if ctxDeadline, ok := ctx.Deadline(); ok {
			deadline = ctxDeadline
		}

		if !deadline.IsZero() {
			if err := conn.SetReadDeadline(deadline); err != nil {
				return collected, fmt.Errorf("failed to set read deadline: %w", err)
			}
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			// If draining and we hit the deadline, that's success - return collected
			if draining {
				return collected, nil
			}
			// Check if this is a context cancellation (timeout)
			if ctx.Err() != nil {
				return collected, ctx.Err()
			}
			return collected, fmt.Errorf("websocket read failed: %w", err)
		}

		var envelope StatusStreamEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue // Skip malformed messages
		}

		// Skip subscribe_ack messages - they're control messages, not status envelopes
		if envelope.Type == "subscribe_ack" {
			continue
		}

		collected = append(collected, envelope)

		// Check condition (only if not already draining)
		if !draining {
			satisfied, err := condition(collected)
			if err != nil {
				return collected, err
			}
			if satisfied {
				// If no drain duration, return immediately
				if opts.DrainDuration == 0 {
					return collected, nil
				}
				// Start drain period
				draining = true
				drainDeadline = time.Now().Add(opts.DrainDuration)
			}
		}

		// Check if drain period has expired
		if draining && time.Now().After(drainDeadline) {
			return collected, nil
		}
	}
}

// --- Pre-built Condition Functions ---

// HasMessageType returns condition satisfied when message type appears
func HasMessageType(msgType string) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		for _, e := range envelopes {
			if e.Type == msgType {
				return true, nil
			}
		}
		return false, nil
	}
}

// HasAllMessageTypes returns condition satisfied when all types appear
func HasAllMessageTypes(types []string) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		found := make(map[string]bool)
		for _, e := range envelopes {
			found[e.Type] = true
		}
		for _, t := range types {
			if !found[t] {
				return false, nil
			}
		}
		return true, nil
	}
}

// MessageTypeCountReaches returns condition when count of type >= target
func MessageTypeCountReaches(msgType string, target int) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		count := 0
		for _, e := range envelopes {
			if e.Type == msgType {
				count++
			}
		}
		return count >= target, nil
	}
}

// LogMessageContains returns condition when log contains substring
func LogMessageContains(substr string) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		for _, e := range envelopes {
			if e.Type != "log_entry" {
				continue
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			if msg, ok := payload["message"].(string); ok {
				if strings.Contains(msg, substr) {
					return true, nil
				}
			}
		}
		return false, nil
	}
}

// EnvelopeCountReaches returns condition when total envelope count >= target
func EnvelopeCountReaches(target int) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		return len(envelopes) >= target, nil
	}
}

// CombineWSConditionsAnd returns condition satisfied when all conditions are satisfied
func CombineWSConditionsAnd(conditions ...WatchStatusStreamCondition) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		for _, cond := range conditions {
			satisfied, err := cond(envelopes)
			if err != nil {
				return false, err
			}
			if !satisfied {
				return false, nil
			}
		}
		return true, nil
	}
}

// CombineWSConditionsOr returns condition satisfied when any condition is satisfied
func CombineWSConditionsOr(conditions ...WatchStatusStreamCondition) WatchStatusStreamCondition {
	return func(envelopes []StatusStreamEnvelope) (bool, error) {
		for _, cond := range conditions {
			satisfied, err := cond(envelopes)
			if err != nil {
				return false, err
			}
			if satisfied {
				return true, nil
			}
		}
		return false, nil
	}
}

// CountMessageTypes returns a count of each message type in the envelopes
func CountMessageTypes(envelopes []StatusStreamEnvelope) map[string]int {
	counts := make(map[string]int)
	for _, e := range envelopes {
		counts[e.Type]++
	}
	return counts
}
