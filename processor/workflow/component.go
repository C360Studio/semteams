package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// schema is the configuration schema for workflow-processor, generated from Config struct tags
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the workflow processor
type Component struct {
	config     Config
	deps       component.Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Core components
	registry  *Registry
	execStore *ExecutionStore
	executor  *Executor

	// KV buckets
	definitionsBucket jetstream.KeyValue
	executionsBucket  jetstream.KeyValue

	// Lifecycle state
	mu        sync.RWMutex
	started   bool
	startTime time.Time

	// Ports (merged from config)
	inputPorts  []component.Port
	outputPorts []component.Port

	// Metrics
	metrics *workflowMetrics

	// Active executions for agent.complete mapping
	activeExecutions sync.Map // exec_id -> *Execution
}

// NewComponent creates a new workflow processor component
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Start with defaults
	config := DefaultConfig()

	// Parse configuration
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

	comp := &Component{
		config:      config,
		deps:        deps,
		natsClient:  deps.NATSClient,
		logger:      deps.GetLogger(),
		inputPorts:  inputPorts,
		outputPorts: outputPorts,
		metrics:     getMetrics(deps.MetricsRegistry),
	}

	return comp, nil
}

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "workflow-processor",
		Type:        "processor",
		Description: "Orchestrates multi-step agentic workflows with loops, limits, and timeouts",
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

// Start starts the component
func (c *Component) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	// Initialize KV buckets
	if c.natsClient != nil {
		if err := c.initializeKVBuckets(ctx); err != nil {
			return fmt.Errorf("failed to initialize KV buckets: %w", err)
		}

		// Create registry
		c.registry = NewRegistry(c.definitionsBucket, c.logger)

		// Load workflow definitions from files (if configured)
		if len(c.config.WorkflowFiles) > 0 {
			fileDefinitions, err := c.loadWorkflowDefinitionsFromFiles()
			if err != nil {
				return fmt.Errorf("failed to load workflow files: %w", err)
			}

			for i := range fileDefinitions {
				def := &fileDefinitions[i]
				if err := c.registry.Register(ctx, def); err != nil {
					c.logger.Warn("Failed to register workflow from file",
						slog.String("id", def.ID),
						slog.String("error", err.Error()))
				} else {
					c.logger.Info("Loaded workflow from file",
						slog.String("id", def.ID),
						slog.String("name", def.Name))
				}
			}
		}

		// Load additional workflow definitions from KV bucket
		if err := c.registry.Load(ctx); err != nil {
			c.logger.Warn("Failed to load workflow definitions from KV", "error", err)
		}

		// Create execution store
		c.execStore = NewExecutionStore(c.executionsBucket)

		// Create executor
		c.executor = NewExecutor(
			c.natsClient,
			c.execStore,
			c.logger,
			c.config,
			c.metrics,
			c.publishEvent,
		)

		// Start watching for definition changes
		if err := c.registry.Watch(ctx); err != nil {
			c.logger.Warn("Failed to start registry watcher", "error", err)
		}

		// Set up NATS subscriptions
		if err := c.setupSubscriptions(ctx); err != nil {
			return fmt.Errorf("failed to setup subscriptions: %w", err)
		}
	}

	c.started = true
	c.startTime = time.Now()

	c.logger.Info("Workflow processor started",
		slog.Int("workflow_count", len(c.registry.ListEnabled())))

	return nil
}

// Stop stops the component
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	// Stop the registry watcher goroutine
	if c.registry != nil {
		c.registry.StopWatch()
	}

	// Clear active executions to prevent memory leaks
	c.activeExecutions.Range(func(key, _ any) bool {
		c.activeExecutions.Delete(key)
		return true
	})

	c.started = false
	c.logger.Info("Workflow processor stopped")
	return nil
}

// initializeKVBuckets initializes the KV buckets
func (c *Component) initializeKVBuckets(ctx context.Context) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream: %w", err)
	}

	// Initialize definitions bucket
	definitionsBucket, err := js.KeyValue(ctx, c.config.DefinitionsBucket)
	if err != nil {
		definitionsBucket, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: c.config.DefinitionsBucket,
		})
		if err != nil {
			return fmt.Errorf("failed to create definitions bucket: %w", err)
		}
	}
	c.definitionsBucket = definitionsBucket

	// Initialize executions bucket with TTL
	executionsBucket, err := js.KeyValue(ctx, c.config.ExecutionsBucket)
	if err != nil {
		executionsBucket, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: c.config.ExecutionsBucket,
			TTL:    7 * 24 * time.Hour, // 7 days
		})
		if err != nil {
			return fmt.Errorf("failed to create executions bucket: %w", err)
		}
	}
	c.executionsBucket = executionsBucket

	return nil
}

// setupSubscriptions sets up JetStream consumers for input ports
func (c *Component) setupSubscriptions(ctx context.Context) error {
	for _, port := range c.inputPorts {
		var subjects []string

		switch p := port.Config.(type) {
		case component.JetStreamPort:
			subjects = p.Subjects
		case component.NATSPort:
			subjects = []string{p.Subject}
		}

		if len(subjects) == 0 {
			continue
		}

		var handler func(context.Context, []byte)

		switch port.Name {
		case "workflow.trigger":
			handler = c.handleTriggerMessage
		case "workflow.step.complete":
			handler = c.handleStepCompleteMessage
		case "agent.complete":
			handler = c.handleAgentCompleteMessage
		default:
			c.logger.Warn("Unknown input port", "port", port.Name)
			continue
		}

		for _, subject := range subjects {
			if err := c.setupConsumer(ctx, port.Name, subject, handler); err != nil {
				return fmt.Errorf("failed to setup consumer for %s: %w", subject, err)
			}
		}
	}

	return nil
}

// setupConsumer sets up a JetStream consumer for an input port
func (c *Component) setupConsumer(ctx context.Context, portName, subject string, handler func(context.Context, []byte)) error {
	// Determine stream name based on subject
	streamName := c.config.StreamName
	if strings.HasPrefix(subject, "agent.") {
		streamName = "AGENT"
	}

	// Wait for stream to be available
	if err := c.waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Create durable consumer name
	consumerName := fmt.Sprintf("workflow-%s", sanitizeSubject(subject))
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
		DeliverPolicy: "new",
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

// handleTriggerMessage processes workflow trigger messages
func (c *Component) handleTriggerMessage(ctx context.Context, data []byte) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", "error", err)
		return
	}

	trigger, ok := baseMsg.Payload().(*TriggerPayload)
	if !ok {
		c.logger.Error("Unexpected payload type", "type", fmt.Sprintf("%T", baseMsg.Payload()))
		return
	}

	workflowID := trigger.WorkflowID
	triggerPayload := trigger.Data

	// Get workflow definition
	workflow, ok := c.registry.Get(workflowID)
	if !ok {
		c.logger.Error("Workflow not found", "workflow_id", workflowID)
		return
	}

	if !workflow.Enabled {
		c.logger.Warn("Workflow is disabled", "workflow_id", workflowID)
		return
	}

	// Parse timeout
	timeout, err := time.ParseDuration(workflow.Timeout)
	if err != nil || timeout == 0 {
		timeout, _ = time.ParseDuration(c.config.DefaultTimeout)
	}

	// Create trigger context
	triggerCtx := TriggerContext{
		Subject:   fmt.Sprintf("workflow.trigger.%s", workflowID),
		Payload:   triggerPayload,
		Timestamp: time.Now(),
	}

	// Create execution
	exec := NewExecution(workflow.ID, workflow.Name, triggerCtx, timeout)

	// Record metrics
	if c.metrics != nil {
		c.metrics.recordWorkflowStarted(workflowID)
	}

	c.logger.Info("Starting workflow execution",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", workflowID),
		slog.String("workflow_name", workflow.Name))

	// Store in active executions
	c.activeExecutions.Store(exec.ID, exec)

	// Start execution
	if err := c.executor.StartExecution(ctx, workflow, exec); err != nil {
		c.logger.Error("Failed to start execution", "error", err)
		c.activeExecutions.Delete(exec.ID)
	}
}

// handleStepCompleteMessage processes step completion messages
func (c *Component) handleStepCompleteMessage(ctx context.Context, data []byte) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("Failed to unmarshal BaseMessage", "error", err)
		return
	}

	msg, ok := baseMsg.Payload().(*StepCompleteMessage)
	if !ok {
		c.logger.Error("Unexpected payload type", "type", fmt.Sprintf("%T", baseMsg.Payload()))
		return
	}

	c.logger.Debug("Received step complete",
		slog.String("execution_id", msg.ExecutionID),
		slog.String("step_name", msg.StepName),
		slog.String("status", msg.Status))

	// Get execution
	exec, err := c.execStore.Get(ctx, msg.ExecutionID)
	if err != nil {
		c.logger.Error("Execution not found for step complete", "error", err, "execution_id", msg.ExecutionID)
		return
	}

	if exec.State.IsTerminal() {
		c.logger.Debug("Execution already terminal", "execution_id", msg.ExecutionID)
		return
	}

	// Get workflow
	workflow, ok := c.registry.Get(exec.WorkflowID)
	if !ok {
		c.logger.Error("Workflow not found", "workflow_id", exec.WorkflowID)
		return
	}

	// Build step result
	stepResult := StepResult{
		StepName:    msg.StepName,
		Status:      msg.Status,
		Output:      msg.Output,
		Error:       msg.Error,
		StartedAt:   exec.UpdatedAt,
		CompletedAt: time.Now(),
		Duration:    time.Since(exec.UpdatedAt),
		Iteration:   exec.Iteration,
	}

	// Continue execution
	if err := c.executor.ContinueExecution(ctx, workflow, exec, stepResult); err != nil {
		c.logger.Error("Failed to continue execution", "error", err)
	}

	// Check if execution is now terminal
	updatedExec, err := c.execStore.Get(ctx, msg.ExecutionID)
	if err == nil && updatedExec.State.IsTerminal() {
		c.activeExecutions.Delete(msg.ExecutionID)
	}
}

// handleAgentCompleteMessage processes agent.complete messages
func (c *Component) handleAgentCompleteMessage(ctx context.Context, data []byte) {
	var msg struct {
		LoopID     string          `json:"loop_id"`
		TaskID     string          `json:"task_id"`
		Outcome    string          `json:"outcome"`
		Output     json.RawMessage `json:"output,omitempty"`
		Error      string          `json:"error,omitempty"`
		Role       string          `json:"role,omitempty"`
		WorkflowID string          `json:"workflow_id,omitempty"`
		ExecID     string          `json:"execution_id,omitempty"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Error("Failed to unmarshal agent complete message", "error", err)
		return
	}

	// Try to find execution ID from message or by task correlation
	execID := msg.ExecID
	if execID == "" {
		// Try to find by task ID correlation
		// This would need a task_id -> exec_id mapping in practice
		c.logger.Debug("No execution ID in agent complete, skipping", "loop_id", msg.LoopID)
		return
	}

	c.logger.Debug("Received agent complete for workflow",
		slog.String("execution_id", execID),
		slog.String("loop_id", msg.LoopID),
		slog.String("outcome", msg.Outcome))

	var agentError string
	if msg.Outcome != "complete" {
		agentError = msg.Error
		if agentError == "" {
			agentError = fmt.Sprintf("agent outcome: %s", msg.Outcome)
		}
	}

	if err := c.executor.HandleAgentComplete(ctx, c.registry, execID, msg.Output, agentError); err != nil {
		c.logger.Error("Failed to handle agent complete", "error", err)
	}
}

// publishEvent publishes a workflow event
func (c *Component) publishEvent(ctx context.Context, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	subject := "workflow.events"
	if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

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

// sanitizeSubject converts a subject pattern to a valid consumer name suffix
func sanitizeSubject(subject string) string {
	s := strings.ReplaceAll(subject, ".", "-")
	s = strings.ReplaceAll(s, ">", "all")
	s = strings.ReplaceAll(s, "*", "any")
	return s
}
