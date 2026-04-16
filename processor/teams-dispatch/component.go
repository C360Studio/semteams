package teamsdispatch

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
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/types"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	operatingmodel "github.com/c360studio/semteams/operating-model"
)

// Component implements the router processor
type Component struct {
	config        Config
	deps          component.Dependencies
	natsClient    *natsclient.Client
	logger        *slog.Logger
	loopTracker   *LoopTracker
	registry      *CommandRegistry
	metrics       *routerMetrics
	modelRegistry model.RegistryReader // Unified model registry for model selection

	// Optional LLM intent classifier (nil when disabled)
	intentClassifier IntentClassifier

	// Lifecycle state
	mu        sync.RWMutex
	started   bool
	startTime time.Time

	// Ports
	inputPorts  []component.Port
	outputPorts []component.Port

	// Track consumers for cleanup
	consumerInfos []consumerInfo

	// profileReader reads the user's operating-model from the ENTITY_STATES
	// KV bucket so handleTaskSubmission can embed a preamble in task.Context
	// for the first LLM call (issue #22 timing fix). Nil when graph-ingest
	// hasn't started or the bucket isn't available.
	profileReader operatingmodel.ProfileReader

	// sendResponseFn is a test hook only; production leaves this nil. When
	// non-nil it replaces the NATS-publishing behavior of sendResponse, so unit
	// tests can capture outgoing responses without a live NATS connection.
	sendResponseFn func(agentic.UserResponse)
}

// consumerInfo tracks JetStream consumer details for cleanup
type consumerInfo struct {
	streamName   string
	consumerName string
}

// NewComponent creates a new router component
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Parse configuration
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "parse config")
	}

	// Require model registry
	if deps.ModelRegistry == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "Component", "NewComponent", "deps.ModelRegistry is required")
	}

	// Apply defaults for all empty values. The component manager's schema
	// filtering may strip nested objects (like permissions) from the raw
	// config JSON — so we must apply defaults for any field that was not
	// parsed, not just top-level strings.
	defaults := DefaultConfig()
	if config.DefaultRole == "" {
		config.DefaultRole = defaults.DefaultRole
	}
	if config.StreamName == "" {
		config.StreamName = defaults.StreamName
	}
	// Apply default permissions if the schema filter stripped them
	// from the raw config (nested objects may not survive the
	// component manager's schema validation pass).
	if config.Permissions.SubmitTask == nil {
		config.Permissions = defaults.Permissions
	}
	// Ensure port config is populated so outputSubject/inputSubject helpers
	// always have a base to work from. An explicit empty Ports object in the
	// config JSON is treated as "use defaults".
	if config.Ports == nil {
		config.Ports = defaults.Ports
	}

	slog.Info("teams-dispatch initialized",
		"default_role", config.DefaultRole,
		"submit_task", config.Permissions.SubmitTask,
		"auto_continue", config.AutoContinue)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "validate config")
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
		config:        config,
		deps:          deps,
		natsClient:    deps.NATSClient,
		logger:        logger,
		loopTracker:   NewLoopTrackerWithLogger(logger),
		registry:      NewCommandRegistry(),
		metrics:       getMetrics(deps.MetricsRegistry),
		modelRegistry: deps.ModelRegistry,
		inputPorts:    inputPorts,
		outputPorts:   outputPorts,
	}

	// Register built-in commands
	comp.registerBuiltinCommands()

	// Load globally registered commands
	comp.loadGlobalCommands()

	// Initialize intent classifier if enabled
	if config.EnableIntentClassification {
		comp.intentClassifier = NewLLMIntentClassifier(
			deps.ModelRegistry,
			config.ClassificationModel,
			logger,
		)
		logger.Info("Intent classification enabled", slog.String("model", config.ClassificationModel))
	}

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

// Initialize prepares the component
func (c *Component) Initialize() error {
	return nil
}

// Start begins processing
func (c *Component) Start(ctx context.Context) error {
	// Validate context
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Component", "Start", "context already cancelled")
	}

	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return errs.ErrAlreadyStarted
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
		return errs.Wrap(err, "Component", "Start", "setup subscriptions")
	}

	// Wire the graph-backed ProfileReader for first-iteration context injection
	// (issue #22). Falls back to nil if the KV bucket isn't available.
	if c.natsClient != nil {
		c.initProfileReader(ctx)
	}

	c.logger.Info("Router component started",
		slog.Int("commands", c.registry.Count()))

	return nil
}

// initProfileReader opens the ENTITY_STATES KV bucket and stores a
// GraphProfileReader on the component. Graceful: logs and continues if the
// bucket isn't available (graph-ingest hasn't started).
func (c *Component) initProfileReader(ctx context.Context) {
	reader, err := operatingmodel.NewGraphProfileReader(ctx, c.natsClient, "ENTITY_STATES", c.logger)
	if err != nil {
		c.logger.Debug("ENTITY_STATES bucket not available; profile preamble injection disabled",
			"error", err)
		return
	}
	c.profileReader = reader
	c.logger.Info("Profile preamble reader wired for first-iteration injection")
}

// buildProfileContext queries the user's operating model and returns a
// ConstructedContext suitable for embedding in TaskMessage.Context. Returns
// nil when the user has no profile or the read fails — callers treat nil as
// "no context to inject."
func (c *Component) buildProfileContext(ctx context.Context, userID string) *types.ConstructedContext {
	result, err := c.profileReader.ReadOperatingModel(ctx, c.deps.Platform.Org, c.deps.Platform.Platform, userID)
	if err != nil {
		c.logger.Debug("Profile read failed; skipping first-iteration injection",
			"user_id", userID, "error", err)
		return nil
	}
	if result == nil || len(result.Entries) == 0 {
		return nil
	}

	pc := &operatingmodel.ProfileContext{
		UserID:         userID,
		LoopID:         "pre-task",
		ProfileVersion: result.Version,
		OperatingModel: operatingmodel.ProfileContextSlice{
			Content:    renderEntriesForPreamble(result.Entries),
			TokenCount: 0, // populated below
			EntryCount: len(result.Entries),
		},
		TokenBudget: 800,
	}
	pc.OperatingModel.TokenCount = (len(pc.OperatingModel.Content) + 3) / 4

	preamble := pc.SystemPromptPreamble()
	if preamble == "" {
		return nil
	}
	return &types.ConstructedContext{
		Content:    preamble,
		TokenCount: (len(preamble) + 3) / 4,
	}
}

// renderEntriesForPreamble produces a bullet list from entries for embedding
// in the system-prompt preamble.
func renderEntriesForPreamble(entries []operatingmodel.Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString("- ")
		b.WriteString(e.Title)
		if e.Cadence != "" {
			b.WriteString(" (")
			b.WriteString(e.Cadence)
			b.WriteString(")")
		}
		b.WriteString(": ")
		b.WriteString(e.Summary)
		b.WriteString("\n")
	}
	return b.String()
}

// Stop halts processing with graceful shutdown
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}

	// Stop all JetStream consumers
	for _, info := range c.consumerInfos {
		if c.config.DeleteConsumerOnStop {
			// Delete consumer from server (for test cleanup)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			if err := c.natsClient.StopAndDeleteConsumer(ctx, info.streamName, info.consumerName); err != nil {
				c.logger.Debug("Failed to delete consumer", "stream", info.streamName, "consumer", info.consumerName, "error", err)
			} else {
				c.logger.Debug("Stopped and deleted consumer", "stream", info.streamName, "consumer", info.consumerName)
			}
			cancel()
		} else {
			// Just stop local consumption (keep durable consumer for resume)
			c.natsClient.StopConsumer(info.streamName, info.consumerName)
			c.logger.Debug("Stopped consumer", "stream", info.streamName, "consumer", info.consumerName)
		}
	}
	c.consumerInfos = nil

	c.started = false
	c.mu.Unlock()

	c.logger.Info("Router component stopped")
	return nil
}

// consumerConfig holds the per-port consumer configuration used by
// setupSubscriptions when iterating input ports.
type consumerConfig struct {
	// consumerName is the durable JetStream consumer name.
	consumerName string
	// deliverPolicy is "last" or "new"; user messages use "last" to
	// catch messages published just before the consumer starts.
	deliverPolicy string
	// handler is invoked for each message on this port.
	handler func(context.Context, []byte)
}

// portConsumerConfigs maps input port names to their consumer settings.
// Non-optional ports (user_messages, complete) use "new"; user_messages
// uses "last" so messages published just before Start are not lost.
var portConsumerConfigs = map[string]consumerConfig{
	"user_messages": {
		consumerName:  "teams-dispatch-user-message",
		deliverPolicy: "last",
	},
	"complete": {
		consumerName:  "teams-dispatch-complete",
		deliverPolicy: "new",
	},
	"created": {
		consumerName:  "teams-dispatch-created",
		deliverPolicy: "new",
	},
	"failed": {
		consumerName:  "teams-dispatch-failed",
		deliverPolicy: "new",
	},
}

// setupSubscriptions iterates the configured input ports and creates one
// durable JetStream consumer per port. Subjects and stream names are read
// from the port configuration, so the component is subject-namespace-agnostic.
func (c *Component) setupSubscriptions(ctx context.Context) error {
	// Collect the unique set of stream names we need to wait for.
	seen := make(map[string]struct{})
	if c.config.Ports != nil {
		for _, port := range c.config.Ports.Inputs {
			if port.StreamName != "" {
				seen[port.StreamName] = struct{}{}
			}
		}
	}
	// Always wait for the component's primary stream too.
	seen[c.config.StreamName] = struct{}{}

	for streamName := range seen {
		if err := c.waitForStream(ctx, streamName); err != nil {
			return errs.WrapTransient(err, "Component", "setupSubscriptions",
				fmt.Sprintf("wait for stream %s", streamName))
		}
	}

	if c.config.Ports == nil {
		return nil
	}

	for _, portDef := range c.config.Ports.Inputs {
		portDef := portDef // capture loop variable

		cfg, known := portConsumerConfigs[portDef.Name]
		if !known {
			c.logger.Warn("Unknown input port — skipping subscription",
				slog.String("port", portDef.Name))
			continue
		}
		if portDef.Subject == "" {
			c.logger.Warn("Input port has no subject — skipping subscription",
				slog.String("port", portDef.Name))
			continue
		}

		streamName := portDef.StreamName
		if streamName == "" {
			streamName = c.config.StreamName
		}

		// Apply optional consumer name suffix for test isolation.
		consumerName := cfg.consumerName
		if c.config.ConsumerNameSuffix != "" {
			consumerName = consumerName + "-" + c.config.ConsumerNameSuffix
		}

		var dataHandler func(context.Context, []byte)
		switch portDef.Name {
		case "user_messages":
			dataHandler = c.handleUserMessage
		case "complete":
			dataHandler = c.handleAgentComplete
		case "created":
			dataHandler = c.handleAgentCreated
		case "failed":
			dataHandler = c.handleAgentFailed
		default:
			c.logger.Warn("No handler for input port — skipping subscription",
				slog.String("port", portDef.Name))
			continue
		}

		handler := c.ackingHandler(dataHandler, portDef.Name)

		consumerCfg := natsclient.StreamConsumerConfig{
			StreamName:    streamName,
			ConsumerName:  consumerName,
			FilterSubject: portDef.Subject,
			DeliverPolicy: cfg.deliverPolicy,
			AckPolicy:     "explicit",
			MaxDeliver:    3,
			AutoCreate:    false,
		}
		if err := c.natsClient.ConsumeStreamWithConfig(ctx, consumerCfg, handler); err != nil {
			return errs.WrapTransient(err, "Component", "setupSubscriptions",
				fmt.Sprintf("subscribe to port %s (%s)", portDef.Name, portDef.Subject))
		}
		c.consumerInfos = append(c.consumerInfos, consumerInfo{
			streamName:   streamName,
			consumerName: consumerName,
		})

		c.logger.Debug("Subscribed to input port",
			slog.String("port", portDef.Name),
			slog.String("subject", portDef.Subject),
			slog.String("stream", streamName),
			slog.String("consumer", consumerName))
	}

	return nil
}

func (c *Component) ackingHandler(fn func(context.Context, []byte), portName string) func(context.Context, jetstream.Msg) {
	return func(msgCtx context.Context, msg jetstream.Msg) {
		fn(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack message", slog.String("port", portName), slog.String("error", ackErr.Error()))
		}
	}
}

// waitForStream waits for a JetStream stream to be available
func (c *Component) waitForStream(ctx context.Context, streamName string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return errs.WrapTransient(err, "Component", "waitForStream", "get JetStream context")
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

	return errs.WrapTransient(fmt.Errorf("stream %s not found after %d retries", streamName, maxRetries), "Component", "waitForStream", "find stream")
}

// handleUserMessage processes incoming user messages
func (c *Component) handleUserMessage(ctx context.Context, data []byte) {
	startTime := time.Now()

	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", slog.String("error", err.Error()))
		return
	}

	userMsg, ok := baseMsg.Payload().(*agentic.UserMessage)
	if !ok {
		c.logger.Error("Unexpected payload type", slog.String("type", fmt.Sprintf("%T", baseMsg.Payload())))
		return
	}
	msg := *userMsg

	// Record message received
	c.metrics.recordMessageReceived(msg.ChannelType)

	c.logger.Debug("Received user message",
		slog.String("message_id", msg.MessageID),
		slog.String("user_id", msg.UserID),
		slog.String("channel", msg.ChannelType))

	// Four-way routing:
	// 1. Explicit commands (starts with "/") — always command
	// 2. Onboarding interview turn — when the user has an active onboarding loop
	//    on this channel, free-text goes to the interview handler, not to the
	//    generic task-submission path
	// 3. Intent classification (when enabled) — LLM classifies ambiguous messages
	// 4. Fallback — treat as task submission
	switch {
	case strings.HasPrefix(msg.Content, "/"):
		c.handleCommand(ctx, msg)
	case c.isOnboardingInFlight(msg.UserID, msg.ChannelID):
		c.handleOnboardingTurn(ctx, msg)
	case c.intentClassifier != nil:
		c.handleClassifiedMessage(ctx, msg)
	default:
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

	c.logger.Debug("Command executed",
		slog.String("command", name),
		slog.String("user_id", msg.UserID))
}

// handleClassifiedMessage uses the intent classifier to route ambiguous messages.
func (c *Component) handleClassifiedMessage(ctx context.Context, msg agentic.UserMessage) {
	activeLoops := c.loopTracker.GetUserLoops(msg.UserID)
	intent, err := c.intentClassifier.Classify(ctx, msg, activeLoops)
	if err != nil {
		c.logger.Error("Intent classification failed, falling back to task submission",
			slog.Any("error", err))
		c.handleTaskSubmission(ctx, msg)
		return
	}

	c.logger.Debug("Intent classified",
		slog.String("type", string(intent.Type)),
		slog.String("loop_id", intent.LoopID),
		slog.Float64("confidence", intent.Confidence))

	switch intent.Type {
	case IntentContinue:
		// Continue an existing loop — treat as task submission to that loop
		if intent.LoopID != "" {
			msg.ReplyTo = intent.LoopID
		}
		c.handleTaskSubmission(ctx, msg)

	case IntentSignal:
		// Validate signal type before acting on LLM output
		if intent.LoopID == "" && c.config.AutoContinue {
			intent.LoopID = c.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
		}
		if intent.LoopID != "" && intent.SignalType != "" && isKnownSignalType(intent.SignalType) {
			// Route through the command handler to get permission/existence checks
			handler := c.makeSignalCommand(intent.SignalType)
			resp, err := handler(ctx, msg, []string{intent.LoopID}, intent.LoopID)
			if err != nil {
				c.logger.Error("Signal command failed via classification", slog.Any("error", err))
				return
			}
			c.sendResponse(ctx, resp)
		} else {
			// Can't determine loop or signal type — ask user to be explicit
			c.sendResponse(ctx, agentic.UserResponse{
				ResponseID:  uuid.New().String(),
				ChannelType: msg.ChannelType,
				ChannelID:   msg.ChannelID,
				UserID:      msg.UserID,
				Type:        agentic.ResponseTypeStatus,
				Content:     "I understood that as a control signal but couldn't determine the target loop. Try: /approve, /reject, /pause, or /resume [loop_id]",
				Timestamp:   time.Now(),
			})
		}

	case IntentQuestion:
		// Route to status — find the relevant loop and show status
		if intent.LoopID == "" && c.config.AutoContinue {
			intent.LoopID = c.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
		}
		resp, err := c.handleStatusCommand(ctx, msg, []string{intent.LoopID}, intent.LoopID)
		if err != nil {
			c.logger.Error("Status command failed via classification", slog.Any("error", err))
			return
		}
		c.sendResponse(ctx, resp)

	case IntentMeta:
		// Route to help for now
		resp, err := c.handleHelpCommand(ctx, msg, nil, "")
		if err != nil {
			c.logger.Error("Help command failed via classification", slog.Any("error", err))
			return
		}
		c.sendResponse(ctx, resp)

	case IntentNewTask:
		// New task — standard submission
		c.handleTaskSubmission(ctx, msg)

	default:
		// Unknown intent — fall back to task submission
		c.handleTaskSubmission(ctx, msg)
	}
}

// isKnownSignalType checks if a signal type string is one of the known constants.
// This guards against arbitrary signal types from LLM classification output.
func isKnownSignalType(st string) bool {
	switch st {
	case agentic.SignalCancel, agentic.SignalPause, agentic.SignalResume,
		agentic.SignalApprove, agentic.SignalReject, agentic.SignalFeedback, agentic.SignalRetry:
		return true
	default:
		return false
	}
}

// resolveModel returns the default model from the model registry.
func (c *Component) resolveModel() string {
	return c.modelRegistry.GetDefault()
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

	// Create task message. Metadata carries the originating user_id so
	// downstream teams-memory can hydrate a per-user profile context when the
	// loop is created (see processor/teams-memory/profile_context.go).
	task := agentic.TaskMessage{
		LoopID:           loopID,
		TaskID:           taskID,
		Role:             c.config.DefaultRole,
		Model:            c.resolveModel(),
		Prompt:           msg.Content,
		ContextRequestID: msg.ContextRequestID,
		Metadata: map[string]any{
			"user_id": msg.UserID,
		},
	}

	// Embed the user's operating-model preamble so the first LLM iteration
	// always sees it (issue #22 timing fix). The async agent.context.profile
	// path through teams-memory refreshes it for subsequent iterations.
	if c.profileReader != nil {
		task.Context = c.buildProfileContext(ctx, msg.UserID)
	}

	// Track the loop
	c.loopTracker.Track(&LoopInfo{
		LoopID:           loopID,
		TaskID:           taskID,
		UserID:           msg.UserID,
		ChannelType:      msg.ChannelType,
		ChannelID:        msg.ChannelID,
		State:            "pending",
		MaxIterations:    20,
		ContextRequestID: msg.ContextRequestID,
		CreatedAt:        time.Now(),
	})

	// Record loop started
	c.metrics.recordLoopStarted()

	// Wrap task in BaseMessage envelope (required by agentic-loop)
	baseMsg := message.NewBaseMessage(task.Schema(), &task, "teams-dispatch")
	taskData, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Error("Failed to marshal task", slog.String("error", err.Error()))
		return
	}

	taskSubject := c.outputSubject("tasks", taskID)
	if taskSubject == "" {
		c.logger.Error("tasks output port not configured; cannot publish task")
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
	if err := c.natsClient.PublishToStream(ctx, taskSubject, taskData); err != nil {
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

	c.logger.Debug("Task submitted",
		slog.String("loop_id", loopID),
		slog.String("task_id", taskID),
		slog.String("user_id", msg.UserID))
}

// handleAgentComplete processes agent completion events
func (c *Component) handleAgentComplete(ctx context.Context, data []byte) {
	// Parse BaseMessage envelope
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", slog.String("error", err.Error()))
		return
	}

	// Extract LoopCompletedEvent from payload
	completionPtr, ok := baseMsg.Payload().(*agentic.LoopCompletedEvent)
	if !ok {
		c.logger.Error("Unexpected payload type", slog.String("type", fmt.Sprintf("%T", baseMsg.Payload())))
		return
	}
	completion := *completionPtr

	// Get loop info
	loopInfo := c.loopTracker.Get(completion.LoopID)
	if loopInfo == nil {
		c.logger.Warn("Completion for unknown loop", slog.String("loop_id", completion.LoopID))
		return
	}

	// Update loop state (map outcome → display state)
	c.loopTracker.UpdateState(completion.LoopID, outcomeToState(completion.Outcome))

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
		// Note: Failed loops are handled by handleAgentFailed, but handle gracefully
		respType = agentic.ResponseTypeError
		content = fmt.Sprintf("Loop %s failed", completion.LoopID)
	default:
		respType = agentic.ResponseTypeStatus
		content = fmt.Sprintf("Loop %s: %s", completion.LoopID, completion.Outcome)
	}

	// Send response to user (skipped for workflow-initiated loops without user routing)
	c.sendUserResponseForLoop(ctx, loopInfo, respType, content)

	c.logger.Info("Loop completed",
		slog.String("loop_id", completion.LoopID),
		slog.String("outcome", completion.Outcome))
}

// handleAgentCreated processes loop creation events for workflow context sync
func (c *Component) handleAgentCreated(_ context.Context, data []byte) {
	// Parse BaseMessage envelope
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", slog.String("error", err.Error()))
		return
	}

	// Extract LoopCreatedEvent from payload
	createdPtr, ok := baseMsg.Payload().(*agentic.LoopCreatedEvent)
	if !ok {
		c.logger.Error("Unexpected payload type", slog.String("type", fmt.Sprintf("%T", baseMsg.Payload())))
		return
	}
	created := *createdPtr

	// Check if we already track this loop (we originated it)
	if existing := c.loopTracker.Get(created.LoopID); existing != nil {
		// Atomically update workflow context if missing
		c.loopTracker.UpdateWorkflowContext(created.LoopID, created.WorkflowSlug, created.WorkflowStep)
		// Atomically update context request ID if missing
		c.loopTracker.UpdateContextRequestID(created.LoopID, created.ContextRequestID)
		return
	}

	// New loop we didn't originate - track it
	c.loopTracker.Track(&LoopInfo{
		LoopID:           created.LoopID,
		TaskID:           created.TaskID,
		State:            "executing",
		MaxIterations:    created.MaxIterations,
		WorkflowSlug:     created.WorkflowSlug,
		WorkflowStep:     created.WorkflowStep,
		ContextRequestID: created.ContextRequestID,
		Metadata:         created.Metadata,
		CreatedAt:        created.CreatedAt,
	})

	// Record external loop for metrics (will be decremented by handleAgentComplete)
	c.metrics.recordLoopStarted()

	c.logger.Debug("Tracked external loop",
		slog.String("loop_id", created.LoopID),
		slog.String("workflow_slug", created.WorkflowSlug),
		slog.String("workflow_step", created.WorkflowStep))
}

// handleAgentFailed processes loop failure events
func (c *Component) handleAgentFailed(ctx context.Context, data []byte) {
	// Parse BaseMessage envelope
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", slog.String("error", err.Error()))
		return
	}

	// Extract LoopFailedEvent from payload
	failurePtr, ok := baseMsg.Payload().(*agentic.LoopFailedEvent)
	if !ok {
		c.logger.Error("Unexpected payload type", slog.String("type", fmt.Sprintf("%T", baseMsg.Payload())))
		return
	}
	failure := *failurePtr

	// Update loop state
	c.loopTracker.UpdateState(failure.LoopID, "failed")

	// Record metrics
	c.metrics.recordLoopEnded()
	c.metrics.recordCompletionReceived("failed")

	// Get loop info for response routing
	loopInfo := c.loopTracker.Get(failure.LoopID)
	if loopInfo == nil {
		c.logger.Warn("Failure for unknown loop", slog.String("loop_id", failure.LoopID))
		return
	}

	// Send error response to user (skipped for workflow-initiated loops without user routing)
	errorContent := fmt.Sprintf("Loop %s failed: %s", failure.LoopID, failure.Error)
	c.sendUserResponseForLoop(ctx, loopInfo, agentic.ResponseTypeError, errorContent)

	c.logger.Info("Loop failed",
		slog.String("loop_id", failure.LoopID),
		slog.String("reason", failure.Reason),
		slog.String("error", failure.Error))
}

// sendResponse publishes a response to the user's channel
func (c *Component) sendResponse(ctx context.Context, resp agentic.UserResponse) {
	if c.sendResponseFn != nil {
		c.sendResponseFn(resp)
		return
	}
	respMsg := message.NewBaseMessage(resp.Schema(), &resp, "teams-dispatch")
	data, err := json.Marshal(respMsg)
	if err != nil {
		c.logger.Error("Failed to marshal response", slog.String("error", err.Error()))
		return
	}

	responseSubject := c.outputSubject("user_response", resp.ChannelType+"."+resp.ChannelID)
	if responseSubject == "" {
		c.logger.Error("user_response output port not configured; cannot publish response")
		return
	}
	if err := c.natsClient.PublishToStream(ctx, responseSubject, data); err != nil {
		c.logger.Error("Failed to publish response", slog.String("error", err.Error()))
	}
}

// sendUserResponseForLoop sends a response only if the loop has a user channel.
// This prevents invalid NATS subjects (e.g. a bare prefix with no channel) for
// loops without user routing. Workflow-initiated loops are silently skipped.
func (c *Component) sendUserResponseForLoop(ctx context.Context, loopInfo *LoopInfo, respType, content string) {
	if loopInfo.ChannelType == "" || loopInfo.ChannelID == "" {
		c.logger.Debug("Skipping user response for loop without user routing",
			slog.String("loop_id", loopInfo.LoopID),
			slog.String("channel_type", loopInfo.ChannelType),
			slog.String("channel_id", loopInfo.ChannelID),
			slog.String("workflow_slug", loopInfo.WorkflowSlug))
		return
	}

	c.sendResponse(ctx, agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: loopInfo.ChannelType,
		ChannelID:   loopInfo.ChannelID,
		UserID:      loopInfo.UserID,
		InReplyTo:   loopInfo.LoopID,
		Type:        respType,
		Content:     content,
		Timestamp:   time.Now(),
	})
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

// outputSubject resolves the NATS publish subject for a named output port.
// It strips trailing wildcard tokens ("*", ">") from the configured subject
// and appends suffix, enabling callers to build per-message subjects like
// "teams.task.<taskID>" without knowing the subject namespace.
// Returns "" if the port is not found or has no subject configured — callers
// must treat "" as a misconfiguration and log accordingly.
func (c *Component) outputSubject(portName, suffix string) string {
	if c.config.Ports != nil {
		for _, port := range c.config.Ports.Outputs {
			if port.Name == portName && port.Subject != "" {
				base := strings.TrimSuffix(port.Subject, "*")
				base = strings.TrimSuffix(base, ">")
				return base + suffix
			}
		}
	}
	return ""
}

// inputSubject resolves the NATS filter subject for a named input port.
// Returns "" if the port is not found or has no subject configured.
func (c *Component) inputSubject(portName string) string {
	if c.config.Ports != nil {
		for _, port := range c.config.Ports.Inputs {
			if port.Name == portName && port.Subject != "" {
				return port.Subject
			}
		}
	}
	return ""
}

// inputStreamName resolves the stream name for a named input port.
// Returns the component's configured StreamName as fallback.
func (c *Component) inputStreamName(portName string) string {
	if c.config.Ports != nil {
		for _, port := range c.config.Ports.Inputs {
			if port.Name == portName && port.StreamName != "" {
				return port.StreamName
			}
		}
	}
	return c.config.StreamName
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
