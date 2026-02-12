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
	"github.com/c360studio/semstreams/pkg/errs"
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
		return nil, errs.WrapInvalid(err, "workflow-processor", "NewComponent", "parse config")
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "workflow-processor", "NewComponent", "validate config")
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
	// Validate context
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "workflow-processor", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "workflow-processor", "Start", "context already cancelled")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return errs.ErrAlreadyStarted
	}

	// Initialize KV buckets
	if c.natsClient != nil {
		if err := c.initializeKVBuckets(ctx); err != nil {
			return errs.WrapTransient(err, "workflow-processor", "Start", "initialize KV buckets")
		}

		// Create registry
		c.registry = NewRegistry(c.definitionsBucket, c.logger)

		// Load workflow definitions from files (if configured)
		if len(c.config.WorkflowFiles) > 0 {
			fileDefinitions, err := c.loadWorkflowDefinitionsFromFiles()
			if err != nil {
				return errs.WrapInvalid(err, "workflow-processor", "Start", "load workflow files")
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
			c.persistCompletionState,
		)

		// Start watching for definition changes
		if err := c.registry.Watch(ctx); err != nil {
			c.logger.Warn("Failed to start registry watcher", "error", err)
		}

		// Set up NATS subscriptions
		if err := c.setupSubscriptions(ctx); err != nil {
			return errs.WrapTransient(err, "workflow-processor", "Start", "setup subscriptions")
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
		return errs.WrapTransient(err, "workflow-processor", "initializeKVBuckets", "get JetStream")
	}

	// Initialize definitions bucket
	definitionsBucket, err := js.KeyValue(ctx, c.config.DefinitionsBucket)
	if err != nil {
		definitionsBucket, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: c.config.DefinitionsBucket,
		})
		if err != nil {
			return errs.WrapTransient(err, "workflow-processor", "initializeKVBuckets", "create definitions bucket")
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
			return errs.WrapTransient(err, "workflow-processor", "initializeKVBuckets", "create executions bucket")
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
			if err := c.setupConsumer(ctx, port, subject, handler); err != nil {
				return errs.WrapTransient(err, "workflow-processor", "setupSubscriptions", fmt.Sprintf("setup consumer for %s", subject))
			}
		}
	}

	return nil
}

// setupConsumer sets up a JetStream consumer for an input port
func (c *Component) setupConsumer(ctx context.Context, port component.Port, subject string, handler func(context.Context, []byte)) error {
	// Determine stream name based on subject
	streamName := c.config.StreamName
	if strings.HasPrefix(subject, "agent.") {
		streamName = "AGENT"
	}

	// Wait for stream to be available
	if err := c.waitForStream(ctx, streamName); err != nil {
		return errs.WrapTransient(err, "workflow-processor", "setupConsumer", fmt.Sprintf("wait for stream %s", streamName))
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
		"port", port.Name)

	// Get consumer config from port (allows user configuration)
	// Defaults to "new" - only process new messages, don't replay old ones
	consumerCfg := component.GetConsumerConfig(port)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: subject,
		DeliverPolicy: consumerCfg.DeliverPolicy,
		AckPolicy:     consumerCfg.AckPolicy,
		MaxDeliver:    consumerCfg.MaxDeliver,
		AutoCreate:    false,
	}

	err := c.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		handler(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack JetStream message", "error", ackErr)
		}
	})
	if err != nil {
		return errs.WrapTransient(err, "workflow-processor", "setupConsumer", fmt.Sprintf("setup consumer for stream %s", streamName))
	}

	c.logger.Info("Subscribed (JetStream)",
		"subject", subject,
		"stream", streamName,
		"consumer", consumerName,
		"port", port.Name)
	return nil
}

// waitForStream waits for a JetStream stream to be available
func (c *Component) waitForStream(ctx context.Context, streamName string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return errs.WrapTransient(err, "workflow-processor", "waitForStream", "get JetStream context")
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

	return errs.WrapTransient(fmt.Errorf("stream %s not found after %d retries", streamName, maxRetries), "workflow-processor", "waitForStream", "wait for stream availability")
}

// buildMergedPayload creates a JSON blob with struct fields merged with Data.
// Struct fields take precedence over Data fields with the same name.
func buildMergedPayload(trigger *TriggerPayload) (json.RawMessage, error) {
	result := make(map[string]any)

	// First, parse Data blob (base layer)
	if trigger.Data != nil && len(trigger.Data) > 0 {
		if err := json.Unmarshal(trigger.Data, &result); err != nil {
			// Data might not be a JSON object - store it as-is under "_data"
			result["_data"] = string(trigger.Data)
		}
	}

	// Then, overlay struct fields (takes precedence over Data)
	if trigger.WorkflowID != "" {
		result["workflow_id"] = trigger.WorkflowID
	}
	if trigger.Role != "" {
		result["role"] = trigger.Role
	}
	if trigger.Model != "" {
		result["model"] = trigger.Model
	}
	if trigger.Prompt != "" {
		result["prompt"] = trigger.Prompt
	}
	if trigger.UserID != "" {
		result["user_id"] = trigger.UserID
	}
	if trigger.ChannelType != "" {
		result["channel_type"] = trigger.ChannelType
	}
	if trigger.ChannelID != "" {
		result["channel_id"] = trigger.ChannelID
	}
	if trigger.RequestID != "" {
		result["request_id"] = trigger.RequestID
	}

	return json.Marshal(result)
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

	// Build merged payload for TriggerContext (struct fields + Data)
	mergedPayload, err := buildMergedPayload(trigger)
	if err != nil {
		c.logger.Error("Failed to build merged payload", "error", err)
		return
	}

	// Create trigger context with merged payload
	triggerCtx := TriggerContext{
		Subject:   fmt.Sprintf("workflow.trigger.%s", workflowID),
		Payload:   mergedPayload,
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

	// Parse duration from message
	var duration time.Duration
	if msg.Duration != "" {
		var err error
		duration, err = time.ParseDuration(msg.Duration)
		if err != nil {
			c.logger.Warn("Invalid duration in step complete message",
				"duration", msg.Duration,
				"error", err)
		}
	}

	// Build step result from message fields
	StepResult := StepResult{
		StepName:    msg.StepName,
		Status:      msg.Status,
		Output:      msg.Output,
		Error:       msg.Error,
		StartedAt:   msg.StartedAt,
		CompletedAt: msg.CompletedAt,
		Duration:    duration,
		Iteration:   msg.Iteration,
	}

	// Continue execution
	if err := c.executor.ContinueExecution(ctx, workflow, exec, StepResult); err != nil {
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
	if execID == "" && msg.TaskID != "" {
		// Look up execution by task_id using secondary index
		exec, err := c.executor.execStore.GetByTaskID(ctx, msg.TaskID)
		if err != nil {
			c.logger.Debug("No execution found for task completion",
				slog.String("task_id", msg.TaskID),
				slog.String("loop_id", msg.LoopID))
			return
		}
		execID = exec.ID
		c.logger.Debug("Found execution by task_id correlation",
			slog.String("execution_id", execID),
			slog.String("task_id", msg.TaskID))
	} else if execID == "" {
		c.logger.Debug("No execution ID or task ID in agent complete, skipping",
			slog.String("loop_id", msg.LoopID))
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
func (c *Component) publishEvent(ctx context.Context, ev event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return errs.WrapInvalid(err, "workflow-processor", "publishEvent", "marshal event")
	}

	subject := "workflow.events"
	if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
		return errs.WrapTransient(err, "workflow-processor", "publishEvent", "publish to stream")
	}

	return nil
}

// persistCompletionState writes completion state for rules engine observability.
// Key pattern: COMPLETE_{executionID} in WORKFLOW_EXECUTIONS bucket.
// Rules can watch this bucket to trigger follow-up actions based on workflow outcomes.
func (c *Component) persistCompletionState(ctx context.Context, exec *Execution, state string) {
	if c.executionsBucket == nil {
		return
	}

	snapshot := exec.Clone()
	completion := map[string]any{
		"workflow_id":   snapshot.WorkflowID,
		"workflow_name": snapshot.WorkflowName,
		"execution_id":  snapshot.ID,
		"state":         state,
		"iteration":     snapshot.Iteration,
		"step_results":  snapshot.StepResults,
		"error":         snapshot.Error,
		"started_at":    snapshot.StartedAt,
		"completed_at":  time.Now(),
	}

	data, err := json.Marshal(completion)
	if err != nil {
		c.logger.Error("Failed to marshal completion state", "error", err, "execution_id", snapshot.ID)
		return
	}

	// Key pattern: COMPLETE_{executionID} for rules engine to watch
	key := fmt.Sprintf("COMPLETE_%s", snapshot.ID)
	if _, err := c.executionsBucket.Put(ctx, key, data); err != nil {
		c.logger.Error("Failed to persist completion state", "error", err, "execution_id", snapshot.ID)
		return
	}

	c.logger.Debug("Persisted completion state",
		slog.String("execution_id", snapshot.ID),
		slog.String("key", key),
		slog.String("state", state))
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
