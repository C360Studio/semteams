package agenticdispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// Component implements the router processor
type Component struct {
	config      Config
	deps        component.Dependencies
	natsClient  *natsclient.Client
	logger      *slog.Logger
	loopTracker *LoopTracker
	registry    *CommandRegistry
	metrics     *routerMetrics

	// Lifecycle state
	mu        sync.RWMutex
	started   bool
	startTime time.Time

	// Ports
	inputPorts  []component.Port
	outputPorts []component.Port
}

// NewComponent creates a new router component
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Parse configuration
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults for empty values
	if config.DefaultRole == "" {
		config.DefaultRole = DefaultConfig().DefaultRole
	}
	if config.DefaultModel == "" {
		config.DefaultModel = DefaultConfig().DefaultModel
	}
	if config.StreamName == "" {
		config.StreamName = DefaultConfig().StreamName
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Build ports
	inputPorts := buildDefaultInputPorts()
	outputPorts := buildDefaultOutputPorts()

	if config.Ports != nil {
		if len(config.Ports.Inputs) > 0 {
			inputPorts = component.MergePortConfigs(inputPorts, config.Ports.Inputs, component.DirectionInput)
		}
		if len(config.Ports.Outputs) > 0 {
			outputPorts = component.MergePortConfigs(outputPorts, config.Ports.Outputs, component.DirectionOutput)
		}
	}

	logger := deps.GetLogger()
	comp := &Component{
		config:      config,
		deps:        deps,
		natsClient:  deps.NATSClient,
		logger:      logger,
		loopTracker: NewLoopTrackerWithLogger(logger),
		registry:    NewCommandRegistry(),
		metrics:     getMetrics(deps.MetricsRegistry),
		inputPorts:  inputPorts,
		outputPorts: outputPorts,
	}

	// Register built-in commands
	comp.registerBuiltinCommands()

	// Load globally registered commands
	comp.loadGlobalCommands()

	return comp, nil
}

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "router",
		Type:        "processor",
		Description: "Routes user messages to agentic loops with command parsing and permissions",
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

// Initialize prepares the component
func (c *Component) Initialize() error {
	return nil
}

// Start begins processing
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("router already started")
	}
	c.started = true
	c.startTime = time.Now()
	c.mu.Unlock()

	c.logger.Info("Starting router component")

	// Setup subscriptions
	if err := c.setupSubscriptions(ctx); err != nil {
		c.mu.Lock()
		c.started = false
		c.mu.Unlock()
		return fmt.Errorf("failed to setup subscriptions: %w", err)
	}

	c.logger.Info("Router component started",
		slog.Int("commands", c.registry.Count()))

	return nil
}

// Stop halts processing with graceful shutdown
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Create timeout context for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Wait for graceful shutdown or timeout
	select {
	case <-ctx.Done():
		c.logger.Warn("Router stop timed out", slog.Duration("timeout", timeout))
	default:
		// Immediate shutdown (no long-running operations to wait for)
	}

	c.mu.Lock()
	c.started = false
	c.mu.Unlock()

	c.logger.Info("Router component stopped")
	return nil
}

// setupSubscriptions sets up JetStream consumers for durable messaging
func (c *Component) setupSubscriptions(ctx context.Context) error {
	// Wait for streams to be available
	if err := c.waitForStream(ctx, c.config.StreamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", c.config.StreamName, err)
	}
	if err := c.waitForStream(ctx, "AGENT"); err != nil {
		return fmt.Errorf("stream AGENT not available: %w", err)
	}

	// Subscribe to user messages via JetStream
	// Use "last" policy to catch messages sent just before consumer starts
	userMsgCfg := natsclient.StreamConsumerConfig{
		StreamName:    c.config.StreamName,
		ConsumerName:  "agentic-dispatch-user-message",
		FilterSubject: "user.message.>",
		DeliverPolicy: "last",
		AckPolicy:     "explicit",
		MaxDeliver:    3,
		AutoCreate:    false,
	}
	err := c.natsClient.ConsumeStreamWithConfig(ctx, userMsgCfg, func(msgCtx context.Context, msg jetstream.Msg) {
		c.handleUserMessage(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack user message", slog.String("error", ackErr.Error()))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to user.message: %w", err)
	}

	// Subscribe to agent completions via JetStream
	agentCompleteCfg := natsclient.StreamConsumerConfig{
		StreamName:    "AGENT",
		ConsumerName:  "agentic-dispatch-agent-complete",
		FilterSubject: "agent.complete.*",
		DeliverPolicy: "new",
		AckPolicy:     "explicit",
		MaxDeliver:    3,
		AutoCreate:    false,
	}
	err = c.natsClient.ConsumeStreamWithConfig(ctx, agentCompleteCfg, func(msgCtx context.Context, msg jetstream.Msg) {
		c.handleAgentComplete(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack agent complete message", slog.String("error", ackErr.Error()))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to agent.complete: %w", err)
	}

	// Subscribe to loop created events for workflow context sync
	agentCreatedCfg := natsclient.StreamConsumerConfig{
		StreamName:    "AGENT",
		ConsumerName:  "agentic-dispatch-agent-created",
		FilterSubject: "agent.created.*",
		DeliverPolicy: "new",
		AckPolicy:     "explicit",
		MaxDeliver:    3,
		AutoCreate:    false,
	}
	err = c.natsClient.ConsumeStreamWithConfig(ctx, agentCreatedCfg, func(msgCtx context.Context, msg jetstream.Msg) {
		c.handleAgentCreated(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack agent created message", slog.String("error", ackErr.Error()))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to agent.created: %w", err)
	}

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

// handleUserMessage processes incoming user messages
func (c *Component) handleUserMessage(ctx context.Context, data []byte) {
	startTime := time.Now()

	var msg agentic.UserMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Error("Failed to unmarshal user message", slog.String("error", err.Error()))
		return
	}

	// Record message received
	c.metrics.recordMessageReceived(msg.ChannelType)

	c.logger.Debug("Received user message",
		slog.String("message_id", msg.MessageID),
		slog.String("user_id", msg.UserID),
		slog.String("channel", msg.ChannelType))

	// Check if it's a command
	if strings.HasPrefix(msg.Content, "/") {
		c.handleCommand(ctx, msg)
	} else {
		// It's a task submission
		c.handleTaskSubmission(ctx, msg)
	}

	// Record routing duration
	duration := time.Since(startTime).Seconds()
	c.metrics.recordRoutingDuration(duration)
}

// handleCommand processes command messages
func (c *Component) handleCommand(ctx context.Context, msg agentic.UserMessage) {
	name, cmd, args, found := c.registry.Match(msg.Content)
	if !found {
		c.sendResponse(ctx, agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Unknown command. Type /help for available commands.",
			Timestamp:   time.Now(),
		})
		return
	}

	// Check permission
	if cmd.Config.Permission != "" && !c.hasPermission(msg.UserID, cmd.Config.Permission) {
		c.sendResponse(ctx, agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     fmt.Sprintf("Permission denied: requires '%s'", cmd.Config.Permission),
			Timestamp:   time.Now(),
		})
		return
	}

	// Resolve loop ID
	loopID := ""
	if len(args) > 0 && args[0] != "" {
		loopID = args[0]
	} else if c.config.AutoContinue {
		loopID = c.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
	}

	// Check if loop is required
	if cmd.Config.RequireLoop && loopID == "" {
		c.sendResponse(ctx, agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "No active loop. Specify a loop_id or start a task first.",
			Timestamp:   time.Now(),
		})
		return
	}

	// Execute handler
	resp, err := cmd.Handler(ctx, msg, args, loopID)
	if err != nil {
		c.sendResponse(ctx, agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     fmt.Sprintf("Command failed: %s", err.Error()),
			Timestamp:   time.Now(),
		})
		return
	}

	c.sendResponse(ctx, resp)

	// Record command executed
	c.metrics.recordCommandExecuted(name)

	c.logger.Info("Command executed",
		slog.String("command", name),
		slog.String("user_id", msg.UserID))
}

// handleTaskSubmission creates a new agent task
func (c *Component) handleTaskSubmission(ctx context.Context, msg agentic.UserMessage) {
	// Check submit permission
	if !c.hasPermission(msg.UserID, "submit_task") {
		c.sendResponse(ctx, agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Permission denied: cannot submit tasks",
			Timestamp:   time.Now(),
		})
		return
	}

	// Determine loop ID (continue existing or create new)
	loopID := ""
	if msg.ReplyTo != "" {
		loopID = msg.ReplyTo
	} else if c.config.AutoContinue {
		loopID = c.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
	}

	// Create new loop if needed
	if loopID == "" {
		loopID = "loop_" + uuid.New().String()[:8]
	}

	taskID := uuid.New().String()

	// Create task message
	task := agentic.TaskMessage{
		LoopID: loopID,
		TaskID: taskID,
		Role:   c.config.DefaultRole,
		Model:  c.config.DefaultModel,
		Prompt: msg.Content,
	}

	// Track the loop
	c.loopTracker.Track(&LoopInfo{
		LoopID:        loopID,
		TaskID:        taskID,
		UserID:        msg.UserID,
		ChannelType:   msg.ChannelType,
		ChannelID:     msg.ChannelID,
		State:         "pending",
		MaxIterations: 20,
		CreatedAt:     time.Now(),
	})

	// Record loop started
	c.metrics.recordLoopStarted()

	// Publish task
	taskData, err := json.Marshal(task)
	if err != nil {
		c.logger.Error("Failed to marshal task", slog.String("error", err.Error()))
		return
	}

	subject := fmt.Sprintf("agent.task.%s", taskID)
	if err := c.natsClient.PublishToStream(ctx, subject, taskData); err != nil {
		c.logger.Error("Failed to publish task", slog.String("error", err.Error()))
		c.sendResponse(ctx, agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Failed to submit task. Please try again.",
			Timestamp:   time.Now(),
		})
		return
	}

	// Record task submitted
	c.metrics.recordTaskSubmitted()

	// Send acknowledgment
	c.sendResponse(ctx, agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		InReplyTo:   loopID,
		Type:        agentic.ResponseTypeStatus,
		Content:     fmt.Sprintf("Task submitted. Loop: %s", loopID),
		Timestamp:   time.Now(),
	})

	c.logger.Info("Task submitted",
		slog.String("loop_id", loopID),
		slog.String("task_id", taskID),
		slog.String("user_id", msg.UserID))
}

// handleAgentComplete processes agent completion events
func (c *Component) handleAgentComplete(ctx context.Context, data []byte) {
	var completion struct {
		LoopID  string `json:"loop_id"`
		TaskID  string `json:"task_id"`
		Outcome string `json:"outcome"`
		Result  string `json:"result,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	if err := json.Unmarshal(data, &completion); err != nil {
		c.logger.Error("Failed to unmarshal completion", slog.String("error", err.Error()))
		return
	}

	// Get loop info
	loopInfo := c.loopTracker.Get(completion.LoopID)
	if loopInfo == nil {
		c.logger.Warn("Completion for unknown loop", slog.String("loop_id", completion.LoopID))
		return
	}

	// Update loop state
	c.loopTracker.UpdateState(completion.LoopID, completion.Outcome)

	// Record loop ended
	c.metrics.recordLoopEnded()

	// Record completion received
	c.metrics.recordCompletionReceived(completion.Outcome)

	// Build response content
	var content string
	var respType string
	switch completion.Outcome {
	case "complete":
		respType = agentic.ResponseTypeResult
		content = fmt.Sprintf("Loop %s completed.", completion.LoopID)
		if completion.Result != "" {
			content = completion.Result
		}
	case "cancelled":
		respType = agentic.ResponseTypeStatus
		content = fmt.Sprintf("Loop %s cancelled.", completion.LoopID)
	case "failed":
		respType = agentic.ResponseTypeError
		content = fmt.Sprintf("Loop %s failed: %s", completion.LoopID, completion.Error)
	default:
		respType = agentic.ResponseTypeStatus
		content = fmt.Sprintf("Loop %s: %s", completion.LoopID, completion.Outcome)
	}

	// Send response to user
	c.sendResponse(ctx, agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: loopInfo.ChannelType,
		ChannelID:   loopInfo.ChannelID,
		UserID:      loopInfo.UserID,
		InReplyTo:   completion.LoopID,
		Type:        respType,
		Content:     content,
		Timestamp:   time.Now(),
	})

	c.logger.Info("Loop completed",
		slog.String("loop_id", completion.LoopID),
		slog.String("outcome", completion.Outcome))
}

// handleAgentCreated processes loop creation events for workflow context sync
func (c *Component) handleAgentCreated(_ context.Context, data []byte) {
	var created struct {
		LoopID        string `json:"loop_id"`
		TaskID        string `json:"task_id"`
		Role          string `json:"role"`
		Model         string `json:"model"`
		WorkflowSlug  string `json:"workflow_slug"`
		WorkflowStep  string `json:"workflow_step"`
		MaxIterations int    `json:"max_iterations"`
		CreatedAt     string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		c.logger.Error("Failed to unmarshal agent created", slog.String("error", err.Error()))
		return
	}

	// Check if we already track this loop (we originated it)
	if existing := c.loopTracker.Get(created.LoopID); existing != nil {
		// Atomically update workflow context if missing
		c.loopTracker.UpdateWorkflowContext(created.LoopID, created.WorkflowSlug, created.WorkflowStep)
		return
	}

	// New loop we didn't originate - track it
	createdAt, err := time.Parse(time.RFC3339, created.CreatedAt)
	if err != nil {
		c.logger.Warn("Invalid created_at timestamp, using current time",
			slog.String("loop_id", created.LoopID),
			slog.String("raw_value", created.CreatedAt),
			slog.String("error", err.Error()))
		createdAt = time.Now()
	}
	c.loopTracker.Track(&LoopInfo{
		LoopID:        created.LoopID,
		TaskID:        created.TaskID,
		State:         "executing",
		MaxIterations: created.MaxIterations,
		WorkflowSlug:  created.WorkflowSlug,
		WorkflowStep:  created.WorkflowStep,
		CreatedAt:     createdAt,
	})

	// Record external loop for metrics (will be decremented by handleAgentComplete)
	c.metrics.recordLoopStarted()

	c.logger.Debug("Tracked external loop",
		slog.String("loop_id", created.LoopID),
		slog.String("workflow_slug", created.WorkflowSlug),
		slog.String("workflow_step", created.WorkflowStep))
}

// sendResponse publishes a response to the user's channel
func (c *Component) sendResponse(ctx context.Context, resp agentic.UserResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		c.logger.Error("Failed to marshal response", slog.String("error", err.Error()))
		return
	}

	subject := fmt.Sprintf("user.response.%s.%s", resp.ChannelType, resp.ChannelID)
	if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("Failed to publish response", slog.String("error", err.Error()))
	}
}

// hasPermission checks if a user has a specific permission
func (c *Component) hasPermission(userID, permission string) bool {
	switch permission {
	case "view":
		return c.inList(userID, c.config.Permissions.View)
	case "submit_task":
		return c.inList(userID, c.config.Permissions.SubmitTask)
	case "cancel_own":
		return c.config.Permissions.CancelOwn
	case "cancel_any":
		return c.inList(userID, c.config.Permissions.CancelAny)
	case "approve":
		return c.inList(userID, c.config.Permissions.Approve)
	default:
		return false
	}
}

// inList checks if a user is in a permission list
func (c *Component) inList(userID string, list []string) bool {
	for _, allowed := range list {
		if allowed == "*" || allowed == userID {
			return true
		}
	}
	return false
}

// canUserControlLoop checks if a user can control a specific loop
func (c *Component) canUserControlLoop(userID, loopID string) bool {
	// Can always control if has cancel_any
	if c.inList(userID, c.config.Permissions.CancelAny) {
		return true
	}

	// Check if user owns the loop
	loopInfo := c.loopTracker.Get(loopID)
	if loopInfo == nil {
		return false
	}

	return loopInfo.UserID == userID && c.config.Permissions.CancelOwn
}

// CommandRegistry returns the command registry for external registration
func (c *Component) CommandRegistry() *CommandRegistry {
	return c.registry
}

// LoopTracker returns the loop tracker
func (c *Component) LoopTracker() *LoopTracker {
	return c.loopTracker
}

// loadGlobalCommands loads globally registered commands into the component
func (c *Component) loadGlobalCommands() {
	cmdCtx := &CommandContext{
		NATSClient:    c.natsClient,
		LoopTracker:   c.loopTracker,
		Logger:        c.logger,
		HasPermission: c.hasPermission,
	}

	for name, executor := range ListRegisteredCommands() {
		config := executor.Config()

		// Wrap executor in handler function
		handler := func(exec CommandExecutor) CommandHandler {
			return func(ctx context.Context, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
				return exec.Execute(ctx, cmdCtx, msg, args, loopID)
			}
		}(executor)

		if err := c.registry.Register(name, config, handler); err != nil {
			c.logger.Warn("Failed to register global command",
				slog.String("command", name),
				slog.String("error", err.Error()))
		}
	}
}
