package agenticloop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360/semstreams/agentic"
	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// Component implements the agentic-loop processor
type Component struct {
	config     Config
	handler    *MessageHandler
	deps       component.Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Lifecycle state
	mu        sync.RWMutex
	started   bool
	startTime time.Time

	// KV buckets
	loopsBucket        jetstream.KeyValue
	trajectoriesBucket jetstream.KeyValue

	// Ports (merged from config)
	inputPorts  []component.Port
	outputPorts []component.Port
}

// NewComponent creates a new agentic-loop component
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Parse configuration
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Merge ports with defaults
	var inputPorts []component.Port
	var outputPorts []component.Port

	if config.Ports != nil && len(config.Ports.Inputs) > 0 {
		inputPorts = component.MergePortConfigs(
			buildDefaultInputPorts(),
			config.Ports.Inputs,
			component.DirectionInput,
		)
	} else {
		inputPorts = buildDefaultInputPorts()
	}

	if config.Ports != nil && len(config.Ports.Outputs) > 0 {
		outputPorts = component.MergePortConfigs(
			buildDefaultOutputPorts(),
			config.Ports.Outputs,
			component.DirectionOutput,
		)
	} else {
		outputPorts = buildDefaultOutputPorts()
	}

	// Create handler
	handler := NewMessageHandler(config)

	comp := &Component{
		config:      config,
		handler:     handler,
		deps:        deps,
		natsClient:  deps.NATSClient,
		logger:      deps.GetLogger(),
		inputPorts:  inputPorts,
		outputPorts: outputPorts,
	}

	return comp, nil
}

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "agentic-loop",
		Type:        "processor",
		Description: "Orchestrates agentic loops with tool calls and trajectory tracking",
		Version:     "1.0.0",
	}
}

// InputPorts returns input port definitions
func (c *Component) InputPorts() []component.Port {
	return c.inputPorts
}

// OutputPorts returns output port definitions
func (c *Component) OutputPorts() []component.Port {
	return c.outputPorts
}

// ConfigSchema returns the configuration schema
func (c *Component) ConfigSchema() component.ConfigSchema {
	return buildConfigSchema()
}

// Health returns current health status
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	healthy := c.started
	uptime := time.Duration(0)
	if c.started {
		uptime = time.Since(c.startTime)
	}

	status := "stopped"
	if healthy {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:   healthy,
		LastCheck: time.Now(),
		Uptime:    uptime,
		Status:    status,
	}
}

// DataFlow returns current data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
		LastActivity:      time.Now(),
	}
}

// Initialize prepares the component (no-op for this component)
func (c *Component) Initialize() error {
	return nil
}

// Start starts the component.
// The context is used for cancellation during startup operations.
func (c *Component) Start(ctx context.Context) error {
	// Check for cancellation before acquiring lock
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	// Initialize KV buckets if NATS client available
	if c.natsClient != nil {
		if err := c.initializeKVBuckets(ctx); err != nil {
			return fmt.Errorf("failed to initialize KV buckets: %w", err)
		}

		// Set up NATS subscriptions for input ports
		if err := c.setupSubscriptions(ctx); err != nil {
			return fmt.Errorf("failed to setup subscriptions: %w", err)
		}
	}

	c.started = true
	c.startTime = time.Now()

	return nil
}

// Stop stops the component within the given timeout.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	// JetStream consumers are cleaned up automatically when their context is cancelled
	// The ConsumeStreamWithConfig uses the context passed to Start(), which is managed
	// by the flow runtime. No explicit unsubscribe needed for JetStream consumers.

	c.started = false
	return nil
}

// buildDefaultInputPorts creates default input ports
func buildDefaultInputPorts() []component.Port {
	defaultCfg := DefaultConfig()
	ports := make([]component.Port, len(defaultCfg.Ports.Inputs))
	for i, portDef := range defaultCfg.Ports.Inputs {
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

// buildDefaultOutputPorts creates default output ports
func buildDefaultOutputPorts() []component.Port {
	defaultCfg := DefaultConfig()
	ports := make([]component.Port, len(defaultCfg.Ports.Outputs))
	for i, portDef := range defaultCfg.Ports.Outputs {
		ports[i] = component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: portDef.Description,
			Config: component.JetStreamPort{
				StreamName: portDef.StreamName,
				Subjects:   []string{portDef.Subject},
			},
		}
	}
	return ports
}

// buildConfigSchema builds the configuration schema
func buildConfigSchema() component.ConfigSchema {
	minIterPtr := new(int)
	*minIterPtr = 1

	maxIterPtr := new(int)
	*maxIterPtr = 1000

	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"max_iterations": {
				Type:        "integer",
				Description: "Maximum number of iterations before loop terminates",
				Default:     20,
				Minimum:     minIterPtr,
				Maximum:     maxIterPtr,
				Category:    "basic",
			},
			"timeout": {
				Type:        "string",
				Description: "Timeout duration for loop execution (e.g., 120s, 5m)",
				Default:     "120s",
				Category:    "basic",
			},
			"loops_bucket": {
				Type:        "string",
				Description: "NATS KV bucket name for storing loop state",
				Default:     "AGENT_LOOPS",
				Category:    "advanced",
			},
			"trajectories_bucket": {
				Type:        "string",
				Description: "NATS KV bucket name for storing trajectories",
				Default:     "AGENT_TRAJECTORIES",
				Category:    "advanced",
			},
			"ports": {
				Type:        "ports",
				Description: "Port configuration for inputs and outputs",
				Category:    "advanced",
			},
		},
		Required: []string{"max_iterations", "timeout", "loops_bucket", "trajectories_bucket"},
	}
}

// initializeKVBuckets initializes the KV buckets for loop and trajectory storage
func (c *Component) initializeKVBuckets(ctx context.Context) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream: %w", err)
	}

	// Initialize loops bucket
	loopsBucket, err := js.KeyValue(ctx, c.config.LoopsBucket)
	if err != nil {
		// Bucket doesn't exist, try to create it
		loopsBucket, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: c.config.LoopsBucket,
		})
		if err != nil {
			return fmt.Errorf("failed to create loops bucket: %w", err)
		}
	}
	c.loopsBucket = loopsBucket

	// Initialize trajectories bucket
	trajectoriesBucket, err := js.KeyValue(ctx, c.config.TrajectoriesBucket)
	if err != nil {
		// Bucket doesn't exist, try to create it
		trajectoriesBucket, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: c.config.TrajectoriesBucket,
		})
		if err != nil {
			return fmt.Errorf("failed to create trajectories bucket: %w", err)
		}
	}
	c.trajectoriesBucket = trajectoriesBucket

	return nil
}

// setupSubscriptions sets up JetStream consumers for input ports
func (c *Component) setupSubscriptions(ctx context.Context) error {
	for _, port := range c.inputPorts {
		var subject string

		// Handle both JetStreamPort and NATSPort for backward compatibility
		switch p := port.Config.(type) {
		case component.JetStreamPort:
			if len(p.Subjects) > 0 {
				subject = p.Subjects[0]
			}
		case component.NATSPort:
			subject = p.Subject
		}

		if subject == "" {
			continue
		}

		var handler func(context.Context, []byte)

		// Route to appropriate handler based on port name
		switch port.Name {
		case "agent.task":
			handler = c.handleTaskMessage
		case "agent.response":
			handler = c.handleResponseMessage
		case "tool.result":
			handler = c.handleToolResultMessage
		default:
			c.logger.Warn("Unknown input port", "port", port.Name)
			continue
		}

		if err := c.setupConsumer(ctx, port.Name, subject, handler); err != nil {
			return fmt.Errorf("failed to setup consumer for %s: %w", subject, err)
		}
	}

	return nil
}

// setupConsumer sets up a JetStream consumer for an input port
func (c *Component) setupConsumer(ctx context.Context, portName, subject string, handler func(context.Context, []byte)) error {
	// Determine stream name
	streamName := c.config.StreamName
	if streamName == "" {
		streamName = "AGENT"
	}

	// Wait for stream to be available
	if err := c.waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Create durable consumer name
	consumerName := fmt.Sprintf("agentic-loop-%s", sanitizeSubject(subject))
	if c.config.ConsumerNameSuffix != "" {
		consumerName = consumerName + "-" + c.config.ConsumerNameSuffix
	}

	c.logger.Info("Setting up JetStream consumer",
		"stream", streamName,
		"consumer", consumerName,
		"filter_subject", subject,
		"port", portName)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: subject,
		DeliverPolicy: "new", // Only process new messages, don't replay old ones
		AckPolicy:     "explicit",
		MaxDeliver:    3,
		AutoCreate:    false,
	}

	err := c.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		handler(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack JetStream message", "error", ackErr)
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}

	c.logger.Info("Subscribed (JetStream)",
		"subject", subject,
		"stream", streamName,
		"consumer", consumerName,
		"port", portName)
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

// handleTaskMessage processes incoming task messages
func (c *Component) handleTaskMessage(ctx context.Context, data []byte) {
	var task TaskMessage
	if err := json.Unmarshal(data, &task); err != nil {
		c.logger.Error("Failed to unmarshal task message", "error", err)
		return
	}

	// Handle the task using the message handler
	result, err := c.handler.HandleTask(ctx, task)
	if err != nil {
		c.logger.Error("Failed to handle task", "error", err, "task_id", task.TaskID)
		return
	}

	// Publish output messages
	c.publishResults(ctx, result)

	// Persist loop state to KV
	c.persistLoopState(ctx, result.LoopID)

	// Persist trajectory steps
	c.persistTrajectorySteps(ctx, result.LoopID, result.TrajectorySteps)
}

// handleResponseMessage processes incoming agent response messages
func (c *Component) handleResponseMessage(ctx context.Context, data []byte) {
	var response agentic.AgentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		c.logger.Error("Failed to unmarshal response message", "error", err)
		return
	}

	// Extract loop ID from request ID (we need to track this mapping)
	// For now, we'll try to extract it from the response or look it up
	// The integration test publishes to "agent.response.{requestID}"
	// We need to get the loop ID from somewhere - check the handler

	// Try to find loop ID by checking all loops (not ideal but works for integration test)
	loopID := c.findLoopIDForRequest(response.RequestID)
	if loopID == "" {
		c.logger.Warn("No loop found for request", "request_id", response.RequestID)
		return
	}

	// Handle the response using the message handler
	result, err := c.handler.HandleModelResponse(ctx, loopID, response)
	if err != nil {
		c.logger.Error("Failed to handle model response", "error", err, "loop_id", loopID)
		return
	}

	// Publish output messages
	c.publishResults(ctx, result)

	// Persist loop state to KV
	c.persistLoopState(ctx, result.LoopID)

	// Persist trajectory steps
	c.persistTrajectorySteps(ctx, result.LoopID, result.TrajectorySteps)

	// If loop completed, finalize trajectory
	if result.State == agentic.LoopStateComplete || result.State == agentic.LoopStateFailed {
		c.finalizeTrajectory(ctx, result.LoopID, result.State)
	}
}

// handleToolResultMessage processes incoming tool result messages
func (c *Component) handleToolResultMessage(ctx context.Context, data []byte) {
	var toolResult agentic.ToolResult
	if err := json.Unmarshal(data, &toolResult); err != nil {
		c.logger.Error("Failed to unmarshal tool result message", "error", err)
		return
	}

	// Find loop ID for this tool call
	loopID := c.findLoopIDForToolCall(toolResult.CallID)
	if loopID == "" {
		c.logger.Warn("No loop found for tool call", "call_id", toolResult.CallID)
		return
	}

	// Handle the tool result using the message handler
	result, err := c.handler.HandleToolResult(ctx, loopID, toolResult)
	if err != nil {
		c.logger.Error("Failed to handle tool result", "error", err, "loop_id", loopID)
		return
	}

	// Publish output messages
	c.publishResults(ctx, result)

	// Persist loop state to KV
	c.persistLoopState(ctx, result.LoopID)

	// Persist trajectory steps
	c.persistTrajectorySteps(ctx, result.LoopID, result.TrajectorySteps)
}

// publishResults publishes all output messages from a handler result using JetStream
func (c *Component) publishResults(ctx context.Context, result HandlerResult) {
	for _, msg := range result.PublishedMessages {
		// Use JetStream for publishing to ensure delivery
		if err := c.natsClient.PublishToStream(ctx, msg.Subject, msg.Data); err != nil {
			c.logger.Error("Failed to publish message", "error", err, "subject", msg.Subject)
		}
	}
}

// persistLoopState persists the loop state to KV
func (c *Component) persistLoopState(ctx context.Context, loopID string) {
	if c.loopsBucket == nil {
		return
	}

	entity, err := c.handler.GetLoop(loopID)
	if err != nil {
		c.logger.Error("Failed to get loop for persistence", "error", err, "loop_id", loopID)
		return
	}

	data, err := json.Marshal(entity)
	if err != nil {
		c.logger.Error("Failed to marshal loop entity", "error", err, "loop_id", loopID)
		return
	}

	if _, err := c.loopsBucket.Put(ctx, loopID, data); err != nil {
		c.logger.Error("Failed to persist loop state", "error", err, "loop_id", loopID)
	}
}

// persistTrajectorySteps persists trajectory steps to KV
func (c *Component) persistTrajectorySteps(ctx context.Context, loopID string, steps []agentic.TrajectoryStep) {
	if c.trajectoriesBucket == nil || len(steps) == 0 {
		return
	}

	// Get or create trajectory
	var trajectory agentic.Trajectory
	entry, err := c.trajectoriesBucket.Get(ctx, loopID)
	if err == nil {
		// Trajectory exists, unmarshal it
		if err := json.Unmarshal(entry.Value(), &trajectory); err != nil {
			c.logger.Error("Failed to unmarshal existing trajectory", "error", err, "loop_id", loopID)
			return
		}
	} else {
		// Create new trajectory
		trajectory = agentic.Trajectory{
			LoopID:    loopID,
			StartTime: time.Now(),
			Steps:     []agentic.TrajectoryStep{},
		}
	}

	// Append new steps
	trajectory.Steps = append(trajectory.Steps, steps...)

	// Persist updated trajectory
	data, err := json.Marshal(trajectory)
	if err != nil {
		c.logger.Error("Failed to marshal trajectory", "error", err, "loop_id", loopID)
		return
	}

	if _, err := c.trajectoriesBucket.Put(ctx, loopID, data); err != nil {
		c.logger.Error("Failed to persist trajectory", "error", err, "loop_id", loopID)
	}
}

// finalizeTrajectory marks a trajectory as complete
func (c *Component) finalizeTrajectory(ctx context.Context, loopID string, state agentic.LoopState) {
	if c.trajectoriesBucket == nil {
		return
	}

	// Get trajectory
	entry, err := c.trajectoriesBucket.Get(ctx, loopID)
	if err != nil {
		c.logger.Error("Failed to get trajectory for finalization", "error", err, "loop_id", loopID)
		return
	}

	var trajectory agentic.Trajectory
	if err := json.Unmarshal(entry.Value(), &trajectory); err != nil {
		c.logger.Error("Failed to unmarshal trajectory", "error", err, "loop_id", loopID)
		return
	}

	// Set end time and outcome
	now := time.Now()
	trajectory.EndTime = &now
	if state == agentic.LoopStateComplete {
		trajectory.Outcome = "complete"
	} else {
		trajectory.Outcome = "failed"
	}

	// Persist finalized trajectory
	data, err := json.Marshal(trajectory)
	if err != nil {
		c.logger.Error("Failed to marshal finalized trajectory", "error", err, "loop_id", loopID)
		return
	}

	if _, err := c.trajectoriesBucket.Put(ctx, loopID, data); err != nil {
		c.logger.Error("Failed to persist finalized trajectory", "error", err, "loop_id", loopID)
	}
}

// findLoopIDForRequest finds the loop ID associated with a request ID
func (c *Component) findLoopIDForRequest(requestID string) string {
	loopID, exists := c.handler.loopManager.GetLoopForRequest(requestID)
	if !exists {
		return ""
	}
	return loopID
}

// findLoopIDForToolCall finds the loop ID associated with a tool call ID
func (c *Component) findLoopIDForToolCall(callID string) string {
	loopID, exists := c.handler.loopManager.GetLoopForToolCall(callID)
	if !exists {
		return ""
	}
	return loopID
}
