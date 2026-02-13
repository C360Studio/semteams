package slim

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// componentSchema defines the configuration schema
var componentSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Component implements the SLIM bridge input component.
// It receives messages from SLIM groups and publishes them to NATS for agent processing.
type Component struct {
	name       string
	config     Config
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Session and message handling
	sessionManager *SessionManager
	messageMapper  *MessageMapper

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Message processing
	messageBuffer chan *Message

	// Context for background operations
	ctx    context.Context
	cancel context.CancelFunc

	// Metrics tracking
	messagesReceived int64
	messagesSent     int64
	errors           int64
	lastActivity     time.Time
}

// NewComponent creates a new SLIM bridge component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "unmarshal config")
	}

	// Use default config if ports not set
	if config.Ports == nil {
		config = DefaultConfig()
		// Re-unmarshal to get user-provided values
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.WrapInvalid(err, "Component", "NewComponent", "unmarshal config")
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "validate config")
	}

	bufferSize := config.MessageBufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	return &Component{
		name:          "slim-bridge",
		config:        config,
		natsClient:    deps.NATSClient,
		logger:        deps.GetLogger(),
		messageMapper: NewMessageMapper(),
		messageBuffer: make(chan *Message, bufferSize),
	}, nil
}

// Initialize prepares the component.
func (c *Component) Initialize() error {
	// Create session manager with stub client
	// In production, this would use the actual SLIM SDK client
	c.sessionManager = NewSessionManager(c.config, nil, c.logger)

	return nil
}

// Start begins receiving messages from SLIM groups.
func (c *Component) Start(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Component", "Start", "context already cancelled")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return errs.WrapFatal(errs.ErrAlreadyStarted, "Component", "Start", "check running state")
	}

	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "check NATS client")
	}

	// Create cancellable context for background operations
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Start session manager
	if err := c.sessionManager.Start(c.ctx); err != nil {
		c.cancel()
		return errs.Wrap(err, "Component", "Start", "start session manager")
	}

	// Start message processing loop
	go c.processMessageLoop(c.ctx)

	// Start response listener
	go c.listenForResponses(c.ctx)

	c.running = true
	c.startTime = time.Now()

	c.logger.Info("SLIM bridge started",
		slog.String("endpoint", c.config.SLIMEndpoint),
		slog.Int("groups", len(c.config.GroupIDs)))

	return nil
}

// processMessageLoop processes incoming SLIM messages.
func (c *Component) processMessageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.messageBuffer:
			if !ok {
				return
			}
			c.handleMessage(ctx, msg)
		}
	}
}

// handleMessage processes a single SLIM message.
func (c *Component) handleMessage(ctx context.Context, msg *Message) {
	c.mu.Lock()
	c.messagesReceived++
	c.lastActivity = time.Now()
	c.mu.Unlock()

	// Update session activity
	c.sessionManager.UpdateActivity(msg.GroupID)

	// Determine message type
	msgType, err := c.messageMapper.ParseMessageType(msg.Content)
	if err != nil {
		c.logger.Error("Failed to parse SLIM message type",
			slog.String("group_id", msg.GroupID),
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	switch msgType {
	case "user":
		c.handleUserMessage(ctx, msg)
	case "task":
		c.handleTaskMessage(ctx, msg)
	case "response":
		// Responses are typically sent outbound, not inbound
		c.logger.Debug("Received response message, ignoring",
			slog.String("group_id", msg.GroupID))
	default:
		c.logger.Warn("Unknown SLIM message type",
			slog.String("type", msgType),
			slog.String("group_id", msg.GroupID))
	}
}

// handleUserMessage processes a user message from SLIM.
func (c *Component) handleUserMessage(ctx context.Context, slimMsg *Message) {
	userMsg, err := c.messageMapper.ToUserMessage(slimMsg)
	if err != nil {
		c.logger.Error("Failed to map SLIM user message",
			slog.String("group_id", slimMsg.GroupID),
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	// Publish to NATS
	subject := "user.message.slim." + sanitizeSubject(slimMsg.GroupID)
	data, err := json.Marshal(userMsg)
	if err != nil {
		c.logger.Error("Failed to marshal user message",
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("Failed to publish user message",
			slog.String("subject", subject),
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	c.mu.Lock()
	c.messagesSent++
	c.mu.Unlock()

	c.logger.Debug("Published SLIM user message",
		slog.String("group_id", slimMsg.GroupID),
		slog.String("sender", slimMsg.SenderDID))
}

// handleTaskMessage processes a task delegation from SLIM.
func (c *Component) handleTaskMessage(ctx context.Context, slimMsg *Message) {
	taskMsg, err := c.messageMapper.ToTaskMessage(slimMsg)
	if err != nil {
		c.logger.Error("Failed to map SLIM task message",
			slog.String("group_id", slimMsg.GroupID),
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	// Publish to NATS
	subject := "agent.task.slim." + sanitizeSubject(slimMsg.GroupID)
	data, err := json.Marshal(taskMsg)
	if err != nil {
		c.logger.Error("Failed to marshal task message",
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("Failed to publish task message",
			slog.String("subject", subject),
			slog.Any("error", err))
		c.incrementErrors()
		return
	}

	c.mu.Lock()
	c.messagesSent++
	c.mu.Unlock()

	c.logger.Debug("Published SLIM task delegation",
		slog.String("group_id", slimMsg.GroupID),
		slog.String("task_id", taskMsg.TaskID))
}

// listenForResponses subscribes to response subjects and sends them to SLIM.
func (c *Component) listenForResponses(ctx context.Context) {
	// Subscribe to agent completion events
	// This is a simplified implementation - production would use proper stream consumers

	// For now, just log that we're ready to handle responses
	c.logger.Debug("SLIM response listener started")

	<-ctx.Done()
}

// SendResponse sends a response back to a SLIM group.
func (c *Component) SendResponse(ctx context.Context, groupID string, response *agentic.UserResponse) error {
	data, err := c.messageMapper.FromUserResponse(response)
	if err != nil {
		return errs.Wrap(err, "Component", "SendResponse", "map response")
	}

	if err := c.sessionManager.SendMessage(ctx, groupID, data); err != nil {
		return errs.Wrap(err, "Component", "SendResponse", "send to SLIM")
	}

	c.mu.Lock()
	c.messagesSent++
	c.mu.Unlock()

	return nil
}

// SendTaskResult sends a task result back to a SLIM group.
func (c *Component) SendTaskResult(ctx context.Context, groupID string, result *TaskResult) error {
	data, err := c.messageMapper.FromTaskResult(result)
	if err != nil {
		return errs.Wrap(err, "Component", "SendTaskResult", "map result")
	}

	if err := c.sessionManager.SendMessage(ctx, groupID, data); err != nil {
		return errs.Wrap(err, "Component", "SendTaskResult", "send to SLIM")
	}

	c.mu.Lock()
	c.messagesSent++
	c.mu.Unlock()

	return nil
}

// incrementErrors safely increments the error counter.
func (c *Component) incrementErrors() {
	c.mu.Lock()
	c.errors++
	c.mu.Unlock()
}

// Stop gracefully stops the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	// Cancel background context
	if c.cancel != nil {
		c.cancel()
	}

	// Stop session manager
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if c.sessionManager != nil {
		if err := c.sessionManager.Stop(ctx); err != nil {
			c.logger.Warn("Failed to stop session manager", slog.Any("error", err))
		}
	}

	// Close message buffer
	close(c.messageBuffer)

	c.running = false
	c.logger.Info("SLIM bridge stopped")

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "slim-bridge",
		Type:        "input",
		Description: "Receives messages from SLIM groups and publishes to NATS",
		Version:     "1.0.0",
	}
}

// InputPorts returns configured input port definitions.
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
			Config: component.NATSPort{
				Subject: portDef.Subject,
			},
		}
	}
	return ports
}

// OutputPorts returns configured output port definitions.
func (c *Component) OutputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		port := component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionOutput,
			Required:    portDef.Required,
			Description: portDef.Description,
		}
		if portDef.Type == "jetstream" {
			port.Config = component.JetStreamPort{
				StreamName: portDef.StreamName,
				Subjects:   []string{portDef.Subject},
			}
		} else {
			port.Config = component.NATSPort{
				Subject: portDef.Subject,
			}
		}
		ports[i] = port
	}
	return ports
}

// ConfigSchema returns the configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return componentSchema
}

// Health returns the current health status.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := "stopped"
	if c.running {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:    c.running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errors),
		Uptime:     time.Since(c.startTime),
		Status:     status,
	}
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errorRate float64
	total := c.messagesReceived + c.errors
	if total > 0 {
		errorRate = float64(c.errors) / float64(total)
	}

	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         errorRate,
		LastActivity:      c.lastActivity,
	}
}

// GetSessions returns all active SLIM sessions.
func (c *Component) GetSessions() []*GroupSession {
	if c.sessionManager == nil {
		return nil
	}
	return c.sessionManager.ListSessions()
}

// sanitizeSubject converts a group ID to a valid NATS subject component.
func sanitizeSubject(groupID string) string {
	result := make([]byte, len(groupID))
	for i := 0; i < len(groupID); i++ {
		if groupID[i] == '.' || groupID[i] == ':' {
			result[i] = '-'
		} else {
			result[i] = groupID[i]
		}
	}
	return string(result)
}
