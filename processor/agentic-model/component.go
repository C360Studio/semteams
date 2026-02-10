// Package agenticmodel provides an OpenAI-compatible agentic model processor component
// that routes agent requests to configured LLM endpoints with retry logic and tool calling support.
package agenticmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// agenticModelSchema defines the configuration schema
var agenticModelSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the agentic-model processor
type Component struct {
	name       string
	config     Config
	clients    map[string]*Client // endpoint name -> client
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Parsed timeout for message processing
	messageTimeout time.Duration

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Metrics
	requestsProcessed int64
	errors            int64
	lastActivity      time.Time
	metrics           *modelMetrics
}

// NewComponent creates a new agentic-model processor component
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Use default config if ports not set
	if config.Ports == nil {
		config = DefaultConfig()
		// Re-unmarshal to get user-provided values
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Parse timeout for message processing
	messageTimeout := 120 * time.Second // default
	if config.Timeout != "" {
		var err error
		messageTimeout, err = time.ParseDuration(config.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	// Create clients for each endpoint
	clients := make(map[string]*Client)
	for name, endpoint := range config.Endpoints {
		client, err := NewClient(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create client for endpoint %q: %w", name, err)
		}
		clients[name] = client
	}

	return &Component{
		name:           "agentic-model",
		config:         config,
		clients:        clients,
		natsClient:     deps.NATSClient,
		logger:         deps.GetLogger(),
		messageTimeout: messageTimeout,
		metrics:        getMetrics(deps.MetricsRegistry),
	}, nil
}

// Initialize prepares the component (no-op for this component)
func (c *Component) Initialize() error {
	return nil
}

// Start begins processing agent requests
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("component already running")
	}

	if c.natsClient == nil {
		return fmt.Errorf("NATS client required")
	}

	// Set up consumers for input ports
	for _, port := range c.config.Ports.Inputs {
		if port.Subject == "" {
			continue
		}

		if err := c.setupConsumer(ctx, port); err != nil {
			return fmt.Errorf("failed to setup consumer for %s: %w", port.Subject, err)
		}
	}

	c.running = true
	c.startTime = time.Now()

	return nil
}

// setupConsumer sets up a JetStream consumer for an input port
func (c *Component) setupConsumer(ctx context.Context, port component.PortDefinition) error {
	// Determine stream name
	streamName := port.StreamName
	if streamName == "" {
		streamName = c.config.StreamName
	}
	if streamName == "" {
		streamName = "AGENT"
	}

	// Wait for stream to be available
	if err := c.waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Create durable consumer name (with optional suffix for uniqueness in tests)
	consumerName := fmt.Sprintf("agentic-model-%s", sanitizeSubject(port.Subject))
	if c.config.ConsumerNameSuffix != "" {
		consumerName = consumerName + "-" + c.config.ConsumerNameSuffix
	}

	c.logger.Info("Setting up JetStream consumer",
		"stream", streamName,
		"consumer", consumerName,
		"filter_subject", port.Subject)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:     streamName,
		ConsumerName:   consumerName,
		FilterSubject:  port.Subject,
		DeliverPolicy:  "new", // Only process new requests, don't replay old ones
		AckPolicy:      "explicit",
		MaxDeliver:     3,
		AutoCreate:     false,
		MessageTimeout: c.messageTimeout, // Use configured timeout for LLM calls
	}

	err := c.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		c.handleRequest(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack JetStream message", "error", ackErr)
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}

	c.logger.Info("Subscribed to agent requests (JetStream)",
		"subject", port.Subject,
		"stream", streamName,
		"consumer", consumerName)
	return nil
}

// waitForStream waits for a JetStream stream to be available
func (c *Component) waitForStream(ctx context.Context, streamName string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}

	maxRetries := 30
	retryInterval := 100 * time.Millisecond
	maxInterval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		_, err := js.Stream(ctx, streamName)
		if err == nil {
			return nil
		}
		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				retryInterval = min(retryInterval*2, maxInterval)
			}
		}
	}

	return fmt.Errorf("stream %s not found after %d retries", streamName, maxRetries)
}

// sanitizeSubject converts a subject pattern to a valid consumer name suffix
func sanitizeSubject(subject string) string {
	s := strings.ReplaceAll(subject, ".", "-")
	s = strings.ReplaceAll(s, ">", "all")
	s = strings.ReplaceAll(s, "*", "any")
	return s
}

// Stop gracefully stops the component within the given timeout
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	// Create context with timeout for any cleanup operations
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// JetStream consumers are cleaned up automatically when their context is cancelled
	// The ConsumeStreamWithConfig uses the context passed to Start(), which is managed
	// by the flow runtime. No explicit unsubscribe needed for JetStream consumers.

	// Close all HTTP clients
	for name, client := range c.clients {
		if err := client.Close(); err != nil {
			c.logger.Warn("Failed to close client", "endpoint", name, "error", err)
		}
	}

	// Check if we completed within timeout
	select {
	case <-ctx.Done():
		c.logger.Warn("Stop timed out", "timeout", timeout)
	default:
		// Completed within timeout
	}

	c.running = false
	return nil
}

// handleRequest processes an agent request
func (c *Component) handleRequest(ctx context.Context, data []byte) {
	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()

	req, err := c.parseAgentRequest(data)
	if err != nil {
		c.logger.Error("Failed to parse agent request", "error", err)
		c.incrementErrors()
		return
	}

	c.logger.Info("Processing agent request",
		slog.String("request_id", req.RequestID),
		slog.String("model", req.Model),
		slog.String("role", req.Role))

	client, err := c.getClientForRequest(req)
	if err != nil {
		c.logger.Error("Failed to resolve endpoint", "error", err, "model", req.Model)
		c.publishErrorResponse(ctx, req.RequestID, err.Error())
		c.incrementErrors()
		return
	}

	startTime := time.Now()
	if c.metrics != nil {
		c.metrics.recordRequestStart(req.Model)
	}

	resp, err := c.executeRequest(ctx, client, req)
	duration := time.Since(startTime).Seconds()

	if err != nil || resp.Status == "error" {
		c.handleModelError(ctx, req, err, resp.Error, duration)
		return
	}

	c.handleModelSuccess(ctx, req, resp, duration)
}

// parseAgentRequest extracts an AgentRequest from raw message data
func (c *Component) parseAgentRequest(data []byte) (agentic.AgentRequest, error) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		return agentic.AgentRequest{}, fmt.Errorf("unmarshal BaseMessage: %w", err)
	}

	reqPtr, ok := baseMsg.Payload().(*agentic.AgentRequest)
	if !ok {
		return agentic.AgentRequest{}, fmt.Errorf("unexpected payload type: %T", baseMsg.Payload())
	}

	return *reqPtr, nil
}

// handleModelError processes and publishes error responses with metrics
func (c *Component) handleModelError(ctx context.Context, req agentic.AgentRequest, err error, respError string, duration float64) {
	errorMsg := respError
	if err != nil {
		errorMsg = err.Error()
	}

	c.logger.Error("Failed to complete chat", "error", errorMsg, "model", req.Model)

	errorType := classifyError(ctx, errorMsg)
	if c.metrics != nil {
		c.metrics.recordRequestError(req.Model, errorType, duration)
	}

	errorCtx, cancel := natsclient.DetachContextWithTrace(ctx, 5*time.Second)
	defer cancel()
	c.publishErrorResponse(errorCtx, req.RequestID, errorMsg)
	c.incrementErrors()
}

// classifyError determines the error type for metrics categorization
func classifyError(ctx context.Context, errorMsg string) string {
	if ctx.Err() != nil {
		return "timeout"
	}
	if strings.Contains(errorMsg, "connection") {
		return "connection"
	}
	if strings.Contains(errorMsg, "rate limit") {
		return "rate_limit"
	}
	return "unknown"
}

// handleModelSuccess processes successful responses with metrics and publishing
func (c *Component) handleModelSuccess(ctx context.Context, req agentic.AgentRequest, resp agentic.AgentResponse, duration float64) {
	toolCallCount := len(resp.Message.ToolCalls)

	if c.metrics != nil {
		c.metrics.recordRequestComplete(req.Model, duration, toolCallCount)
		if resp.TokenUsage.PromptTokens > 0 || resp.TokenUsage.CompletionTokens > 0 {
			c.metrics.recordTokenUsage(req.Model, resp.TokenUsage.PromptTokens, resp.TokenUsage.CompletionTokens)
		}
	}

	c.logger.Debug("Model request completed",
		slog.String("request_id", req.RequestID),
		slog.String("model", req.Model),
		slog.Float64("duration_seconds", duration),
		slog.Int("tool_calls", toolCallCount),
		slog.Int("prompt_tokens", resp.TokenUsage.PromptTokens),
		slog.Int("completion_tokens", resp.TokenUsage.CompletionTokens))

	if err := c.publishResponse(ctx, resp); err != nil {
		c.logger.Error("Failed to publish response", "error", err)
		c.incrementErrors()
		return
	}

	c.mu.Lock()
	c.requestsProcessed++
	c.mu.Unlock()
}

// getClientForRequest resolves endpoint and returns the appropriate client
func (c *Component) getClientForRequest(req agentic.AgentRequest) (*Client, error) {
	endpoint, err := c.ResolveEndpoint(req.Model)
	if err != nil {
		c.logger.Error("Failed to resolve endpoint", "model", req.Model, "error", err)
		return nil, err
	}

	// Get client for endpoint
	for name, ep := range c.config.Endpoints {
		if ep.URL == endpoint.URL && ep.Model == endpoint.Model {
			return c.clients[name], nil
		}
	}

	c.logger.Error("No client found for endpoint", "model", req.Model)
	return nil, fmt.Errorf("no client for model %s", req.Model)
}

// executeRequest executes the chat completion with timeout
func (c *Component) executeRequest(ctx context.Context, client *Client, req agentic.AgentRequest) (agentic.AgentResponse, error) {
	timeout := 120 * time.Second
	if c.config.Timeout != "" {
		if d, err := time.ParseDuration(c.config.Timeout); err == nil {
			timeout = d
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return client.ChatCompletion(reqCtx, req)
}

// incrementErrors safely increments the error counter
func (c *Component) incrementErrors() {
	c.mu.Lock()
	c.errors++
	c.mu.Unlock()
}

// ResolveEndpoint resolves a model name to an endpoint
func (c *Component) ResolveEndpoint(modelName string) (Endpoint, error) {
	// Resolution priority:
	// 1. Check if modelName is an alias -> resolve to alias target
	// 2. Check if (resolved or original) name is in Endpoints -> return endpoint
	// 3. Fall back to "default" endpoint if it exists
	// 4. Error if no resolution found

	resolvedName := modelName

	// Step 1: Check if modelName is an alias
	if c.config.ModelAliases != nil {
		if target, isAlias := c.config.ModelAliases[modelName]; isAlias {
			resolvedName = target
		}
	}

	// Step 2: Check if resolved name is an endpoint
	if endpoint, exists := c.config.Endpoints[resolvedName]; exists {
		return endpoint, nil
	}

	// Step 3: Try to find "default" endpoint
	if defaultEndpoint, exists := c.config.Endpoints["default"]; exists {
		return defaultEndpoint, nil
	}

	// Step 4: No resolution found
	return Endpoint{}, fmt.Errorf("no endpoint found for model %q", modelName)
}

// publishResponse publishes an agent response to JetStream
func (c *Component) publishResponse(ctx context.Context, resp agentic.AgentResponse) error {
	respMsg := message.NewBaseMessage(resp.Schema(), &resp, "agentic-model")
	data, err := json.Marshal(respMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// Publish to output subjects
	for _, port := range c.config.Ports.Outputs {
		if port.Subject == "" {
			continue
		}

		// Replace wildcard with request ID for specific routing
		subject := port.Subject
		if len(subject) > 0 && subject[len(subject)-1] == '*' {
			subject = subject[:len(subject)-1] + resp.RequestID
		}

		// Use JetStream for publishing to ensure delivery
		if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
			return fmt.Errorf("failed to publish to %s: %w", subject, err)
		}
	}

	return nil
}

// publishErrorResponse publishes an error response
func (c *Component) publishErrorResponse(ctx context.Context, requestID string, errMsg string) {
	resp := agentic.AgentResponse{
		RequestID: requestID,
		Status:    "error",
		Error:     errMsg,
	}

	if err := c.publishResponse(ctx, resp); err != nil {
		c.logger.Error("Failed to publish error response", "error", err)
	}
}

// Discoverable interface implementation

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "agentic-model",
		Type:        "processor",
		Description: "OpenAI-compatible agentic model processor with tool calling support",
		Version:     "0.1.0",
	}
}

// InputPorts returns configured input port definitions
func (c *Component) InputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Inputs))
	for i, portDef := range c.config.Ports.Inputs {
		ports[i] = component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionInput,
			Required:    portDef.Required,
			Description: portDef.Description,
			Config: component.JetStreamPort{
				StreamName: portDef.StreamName,
				Subjects:   []string{portDef.Subject},
			},
		}
	}
	return ports
}

// OutputPorts returns configured output port definitions
func (c *Component) OutputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		ports[i] = component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionOutput,
			Required:    portDef.Required,
			Description: portDef.Description,
			Config: component.JetStreamPort{
				StreamName: portDef.StreamName,
				Subjects:   []string{portDef.Subject},
			},
		}
	}
	return ports
}

// ConfigSchema returns the configuration schema
func (c *Component) ConfigSchema() component.ConfigSchema {
	return agenticModelSchema
}

// Health returns the current health status
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return component.HealthStatus{
		Healthy:    c.running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errors),
		Uptime:     time.Since(c.startTime),
		Status:     c.getStatus(),
	}
}

// getStatus returns a status string
func (c *Component) getStatus() string {
	if c.running {
		return "running"
	}
	return "stopped"
}

// DataFlow returns current data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errorRate float64
	total := c.requestsProcessed + c.errors
	if total > 0 {
		errorRate = float64(c.errors) / float64(total)
	}

	return component.FlowMetrics{
		MessagesPerSecond: 0, // TODO: Calculate rate
		BytesPerSecond:    0,
		ErrorRate:         errorRate,
		LastActivity:      c.lastActivity,
	}
}
