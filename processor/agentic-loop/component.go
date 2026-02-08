package agenticloop

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

// schema is the configuration schema for agentic-loop, generated from Config struct tags
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the agentic-loop processor
type Component struct {
	config     Config
	handler    *MessageHandler
	deps       component.Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Parsed timeout for message processing
	messageTimeout time.Duration

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

	// Metrics
	metrics *loopMetrics
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

	// Parse timeout for message processing
	messageTimeout, err := time.ParseDuration(config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout format: %w", err)
	}

	// Create handler
	handler := NewMessageHandler(config)

	comp := &Component{
		config:         config,
		handler:        handler,
		deps:           deps,
		natsClient:     deps.NATSClient,
		logger:         deps.GetLogger(),
		messageTimeout: messageTimeout,
		inputPorts:     inputPorts,
		outputPorts:    outputPorts,
		metrics:        getMetrics(deps.MetricsRegistry),
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
	return schema
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
		case "agent.signal":
			handler = c.handleSignalMessage
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
		StreamName:     streamName,
		ConsumerName:   consumerName,
		FilterSubject:  subject,
		DeliverPolicy:  "new", // Only process new messages, don't replay old ones
		AckPolicy:      "explicit",
		MaxDeliver:     3,
		AutoCreate:     false,
		MessageTimeout: c.messageTimeout, // Use configured timeout for LLM calls
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
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", "error", err)
		return
	}

	task, ok := baseMsg.Payload().(*agentic.TaskMessage)
	if !ok {
		c.logger.Error("Unexpected payload type", "type", fmt.Sprintf("%T", baseMsg.Payload()))
		return
	}

	c.logger.Info("Processing task message",
		slog.String("task_id", task.TaskID),
		slog.String("role", task.Role),
		slog.String("model", task.Model))

	// Handle the task using the message handler
	result, err := c.handler.HandleTask(ctx, *task)
	if err != nil {
		c.logger.Error("Failed to handle task", "error", err, "task_id", task.TaskID)
		return
	}

	// Record loop creation
	if c.metrics != nil {
		c.metrics.recordLoopCreated()
	}

	c.logger.Debug("Loop created",
		slog.String("loop_id", result.LoopID),
		slog.String("task_id", task.TaskID))

	// Publish output messages
	c.publishResults(ctx, result)

	// Persist loop state to KV
	c.persistLoopState(ctx, result.LoopID)

	// Persist trajectory steps
	c.persistTrajectorySteps(ctx, result.LoopID, result.TrajectorySteps)
}

// handleResponseMessage processes incoming agent response messages
func (c *Component) handleResponseMessage(ctx context.Context, data []byte) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", "error", err)
		return
	}

	responsePtr, ok := baseMsg.Payload().(*agentic.AgentResponse)
	if !ok {
		c.logger.Error("Unexpected payload type", "type", fmt.Sprintf("%T", baseMsg.Payload()))
		return
	}
	response := *responsePtr

	// Try to find loop ID by checking all loops
	loopID := c.findLoopIDForRequest(response.RequestID)
	if loopID == "" {
		c.logger.Warn("No loop found for request", "request_id", response.RequestID)
		return
	}

	c.logger.Debug("Processing model response",
		slog.String("loop_id", loopID),
		slog.String("request_id", response.RequestID),
		slog.String("status", response.Status))

	// Get loop entity to track iterations and duration
	entity, err := c.handler.GetLoop(loopID)
	if err != nil {
		c.logger.Error("Failed to get loop for metrics", "error", err, "loop_id", loopID)
	}
	startTime := entity.StartedAt

	// Handle the response using the message handler
	result, err := c.handler.HandleModelResponse(ctx, loopID, response)
	if err != nil {
		c.logger.Error("Failed to handle model response", "error", err, "loop_id", loopID)

		// Record failed loop metrics
		if c.metrics != nil && entity.ID != "" {
			duration := time.Since(startTime).Seconds()
			c.metrics.recordLoopFailed("handler_error", entity.Iterations, duration)
		}
		return
	}

	// Record iteration
	if c.metrics != nil {
		c.metrics.recordIteration()
		c.metrics.recordTrajectoryStep("model_call")
	}

	// Record tool calls if any
	if response.Status == "tool_call" && c.metrics != nil {
		for _, toolCall := range response.Message.ToolCalls {
			c.metrics.recordToolCallDispatched(toolCall.Name)
			c.logger.Debug("Tool call dispatched",
				slog.String("loop_id", loopID),
				slog.String("tool", toolCall.Name),
				slog.String("call_id", toolCall.ID))
		}
	}

	// Record completion or failure
	if result.State == agentic.LoopStateComplete {
		if c.metrics != nil && entity.ID != "" {
			duration := time.Since(startTime).Seconds()
			c.metrics.recordLoopCompleted(entity.Iterations, duration)
		}
		c.logger.Info("Loop completed",
			slog.String("loop_id", loopID),
			slog.Int("iterations", entity.Iterations))
	} else if result.State == agentic.LoopStateFailed {
		if c.metrics != nil && entity.ID != "" {
			duration := time.Since(startTime).Seconds()
			reason := "unknown"
			if response.Status == "error" {
				reason = "model_error"
			}
			c.metrics.recordLoopFailed(reason, entity.Iterations, duration)
		}
		c.logger.Warn("Loop failed",
			slog.String("loop_id", loopID),
			slog.Int("iterations", entity.Iterations))
	}

	// Publish output messages
	c.publishResults(ctx, result)

	// Persist loop state to KV
	c.persistLoopState(ctx, result.LoopID)

	// Persist trajectory steps
	c.persistTrajectorySteps(ctx, result.LoopID, result.TrajectorySteps)

	// If loop completed, finalize trajectory and persist completion state
	if result.State == agentic.LoopStateComplete || result.State == agentic.LoopStateFailed {
		c.finalizeTrajectory(ctx, result.LoopID, result.State)

		// Persist enriched completion state for rules engine
		if result.CompletionState != nil {
			c.persistCompletionState(ctx, result.LoopID, result.CompletionState)
		}
	}
}

// handleToolResultMessage processes incoming tool result messages
func (c *Component) handleToolResultMessage(ctx context.Context, data []byte) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", "error", err)
		return
	}

	toolResultPtr, ok := baseMsg.Payload().(*agentic.ToolResult)
	if !ok {
		c.logger.Error("Unexpected payload type", "type", fmt.Sprintf("%T", baseMsg.Payload()))
		return
	}
	toolResult := *toolResultPtr

	// Find loop ID for this tool call
	loopID := c.findLoopIDForToolCall(toolResult.CallID)
	if loopID == "" {
		c.logger.Warn("No loop found for tool call", "call_id", toolResult.CallID)
		return
	}

	hasError := toolResult.Error != ""

	c.logger.Debug("Processing tool result",
		slog.String("loop_id", loopID),
		slog.String("call_id", toolResult.CallID),
		slog.Bool("has_error", hasError))

	// Record tool result received
	if c.metrics != nil {
		c.metrics.recordToolResultReceived(hasError)
		c.metrics.recordTrajectoryStep("tool_call")
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

	// Publish context events for agentic-memory to consume
	for _, event := range result.ContextEvents {
		c.publishContextEvent(ctx, event)
	}
}

// publishContextEvent publishes a context management event
func (c *Component) publishContextEvent(ctx context.Context, event agentic.ContextEvent) {
	eventMsg := message.NewBaseMessage(event.Schema(), &event, "agentic-loop")
	data, err := json.Marshal(eventMsg)
	if err != nil {
		c.logger.Error("Failed to marshal context event", "error", err, "type", event.Type)
		return
	}

	subject := fmt.Sprintf("agent.context.compaction.%s", event.LoopID)
	if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("Failed to publish context event", "error", err, "subject", subject)
	}
}

// persistCompletionState persists the enriched completion state to KV.
// Key pattern: COMPLETE_{loopID} for rules engine to watch.
// The rules engine can then trigger follow-up actions based on completion data.
func (c *Component) persistCompletionState(ctx context.Context, loopID string, completion *agentic.LoopCompletedEvent) {
	if c.loopsBucket == nil || completion == nil {
		return
	}

	data, err := json.Marshal(completion)
	if err != nil {
		c.logger.Error("Failed to marshal completion state", "error", err, "loop_id", loopID)
		return
	}

	// Key pattern: COMPLETE_{loopID} for rules engine to watch
	key := fmt.Sprintf("COMPLETE_%s", loopID)
	if _, err := c.loopsBucket.Put(ctx, key, data); err != nil {
		c.logger.Error("Failed to persist completion state", "error", err, "loop_id", loopID)
		return
	}

	c.logger.Debug("Persisted completion state",
		slog.String("loop_id", loopID),
		slog.String("key", key),
		slog.String("role", completion.Role))
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

// handleSignalMessage processes incoming signal messages (cancel, pause, etc.)
func (c *Component) handleSignalMessage(ctx context.Context, data []byte) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", "error", err)
		return
	}

	signalPtr, ok := baseMsg.Payload().(*agentic.UserSignal)
	if !ok {
		c.logger.Error("Unexpected payload type", "type", fmt.Sprintf("%T", baseMsg.Payload()))
		return
	}
	signal := *signalPtr

	c.logger.Info("Processing signal message",
		slog.String("signal_id", signal.SignalID),
		slog.String("type", signal.Type),
		slog.String("loop_id", signal.LoopID),
		slog.String("user_id", signal.UserID))

	// Handle based on signal type
	switch signal.Type {
	case agentic.SignalCancel:
		c.handleCancelSignal(ctx, signal)
	case agentic.SignalPause:
		c.handlePauseSignal(ctx, signal)
	case agentic.SignalResume:
		c.handleResumeSignal(ctx, signal)
	default:
		c.logger.Warn("Unsupported signal type",
			slog.String("type", signal.Type),
			slog.String("loop_id", signal.LoopID))
	}
}

// handleCancelSignal handles a cancel signal for a loop
func (c *Component) handleCancelSignal(ctx context.Context, signal agentic.UserSignal) {
	loopID := signal.LoopID

	// Get current loop state
	entity, err := c.handler.GetLoop(loopID)
	if err != nil {
		c.logger.Error("Failed to get loop for cancel",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Check if loop is already terminal
	if entity.State.IsTerminal() {
		c.logger.Warn("Cannot cancel terminal loop",
			slog.String("loop_id", loopID),
			slog.String("state", string(entity.State)))
		return
	}

	// Update loop state to cancelled
	now := time.Now()
	entity.State = agentic.LoopStateCancelled
	entity.CancelledBy = signal.UserID
	entity.CancelledAt = now

	// Update in handler
	if err := c.handler.UpdateLoop(entity); err != nil {
		c.logger.Error("Failed to update loop state",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Persist loop state to KV
	c.persistLoopState(ctx, loopID)

	// Record metrics
	if c.metrics != nil {
		duration := time.Since(entity.StartedAt).Seconds()
		c.metrics.recordLoopFailed("cancelled", entity.Iterations, duration)
	}

	// Publish completion event
	completion := agentic.LoopCancelledEvent{
		LoopID:      loopID,
		TaskID:      entity.TaskID,
		Outcome:     agentic.OutcomeCancelled,
		CancelledBy: signal.UserID,
		CancelledAt: now,
	}

	completionMsg := message.NewBaseMessage(completion.Schema(), &completion, "agentic-loop")
	completionData, err := json.Marshal(completionMsg)
	if err != nil {
		c.logger.Error("Failed to marshal completion",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	subject := fmt.Sprintf("agent.complete.%s", loopID)
	if err := c.natsClient.PublishToStream(ctx, subject, completionData); err != nil {
		c.logger.Error("Failed to publish completion",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Finalize trajectory
	c.finalizeTrajectory(ctx, loopID, agentic.LoopStateCancelled)

	c.logger.Info("Loop cancelled",
		slog.String("loop_id", loopID),
		slog.String("cancelled_by", signal.UserID))
}

// handlePauseSignal handles a pause signal for a loop
func (c *Component) handlePauseSignal(ctx context.Context, signal agentic.UserSignal) {
	loopID := signal.LoopID

	// Get current loop state
	entity, err := c.handler.GetLoop(loopID)
	if err != nil {
		c.logger.Error("Failed to get loop for pause",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Check if loop can be paused
	if entity.State.IsTerminal() || entity.State == agentic.LoopStatePaused {
		c.logger.Warn("Cannot pause loop",
			slog.String("loop_id", loopID),
			slog.String("state", string(entity.State)))
		return
	}

	// Set pause requested flag
	entity.PauseRequested = true

	// Update in handler
	if err := c.handler.UpdateLoop(entity); err != nil {
		c.logger.Error("Failed to update loop state",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Persist loop state to KV
	c.persistLoopState(ctx, loopID)

	c.logger.Info("Pause requested for loop",
		slog.String("loop_id", loopID),
		slog.String("requested_by", signal.UserID))
}

// handleResumeSignal handles a resume signal for a paused loop
func (c *Component) handleResumeSignal(ctx context.Context, signal agentic.UserSignal) {
	loopID := signal.LoopID

	// Get current loop state
	entity, err := c.handler.GetLoop(loopID)
	if err != nil {
		c.logger.Error("Failed to get loop for resume",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Check if loop can be resumed
	if entity.State != agentic.LoopStatePaused {
		c.logger.Warn("Cannot resume non-paused loop",
			slog.String("loop_id", loopID),
			slog.String("state", string(entity.State)))
		return
	}

	// Clear pause state and restore to executing
	entity.State = agentic.LoopStateExecuting
	entity.PauseRequested = false

	// Update in handler
	if err := c.handler.UpdateLoop(entity); err != nil {
		c.logger.Error("Failed to update loop state",
			slog.String("error", err.Error()),
			slog.String("loop_id", loopID))
		return
	}

	// Persist loop state to KV
	c.persistLoopState(ctx, loopID)

	c.logger.Info("Loop resumed",
		slog.String("loop_id", loopID),
		slog.String("resumed_by", signal.UserID))
}
