package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// schema is the configuration schema for reactive workflow, generated from Config struct tags.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the reactive workflow engine as a discoverable component.
type Component struct {
	config Config
	engine *Engine
	deps   component.Dependencies
	logger *slog.Logger

	// Lifecycle state
	mu        sync.RWMutex
	started   bool
	startTime time.Time

	// Ports (merged from config)
	inputPorts  []component.Port
	outputPorts []component.Port

	// Metrics
	metrics *EngineMetrics
}

// NewComponent creates a new reactive workflow engine component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Parse configuration
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "reactive-workflow", "NewComponent", "parse config")
	}

	// Apply defaults for empty fields
	defaultCfg := DefaultConfig()
	if config.StateBucket == "" {
		config.StateBucket = defaultCfg.StateBucket
	}
	if config.CallbackStreamName == "" {
		config.CallbackStreamName = defaultCfg.CallbackStreamName
	}
	if config.EventStreamName == "" {
		config.EventStreamName = defaultCfg.EventStreamName
	}
	if config.DefaultTimeout == "" {
		config.DefaultTimeout = defaultCfg.DefaultTimeout
	}
	if config.DefaultMaxIterations == 0 {
		config.DefaultMaxIterations = defaultCfg.DefaultMaxIterations
	}
	if config.CleanupRetention == "" {
		config.CleanupRetention = defaultCfg.CleanupRetention
	}
	if config.CleanupInterval == "" {
		config.CleanupInterval = defaultCfg.CleanupInterval
	}
	if config.TaskTimeoutDefault == "" {
		config.TaskTimeoutDefault = defaultCfg.TaskTimeoutDefault
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "reactive-workflow", "NewComponent", "validate config")
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

	logger := deps.GetLogger()

	// Get metrics if enabled
	var metrics *EngineMetrics
	if config.EnableMetrics {
		metrics = GetMetrics(deps.MetricsRegistry)
	}

	// Create engine with options
	engineOpts := []EngineOption{
		WithEngineLogger(logger),
	}
	if metrics != nil {
		engineOpts = append(engineOpts, WithEngineMetrics(metrics))
	}

	engine := NewEngine(config, deps.NATSClient, engineOpts...)

	comp := &Component{
		config:      config,
		engine:      engine,
		deps:        deps,
		logger:      logger,
		inputPorts:  inputPorts,
		outputPorts: outputPorts,
		metrics:     metrics,
	}

	return comp, nil
}

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "reactive-workflow",
		Type:        "processor",
		Description: "Reactive workflow engine using KV watch and subject triggers with typed Go conditions and actions",
		Version:     "1.0.0",
	}
}

// InputPorts returns input port definitions.
func (c *Component) InputPorts() []component.Port {
	return c.inputPorts
}

// OutputPorts returns output port definitions.
func (c *Component) OutputPorts() []component.Port {
	return c.outputPorts
}

// ConfigSchema returns the configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return schema
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	healthy := c.started && c.engine.IsRunning()
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

// DataFlow returns current data flow metrics.
// TODO: Integrate with EngineMetrics to return real-time flow data.
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
		LastActivity:      time.Now(),
	}
}

// Initialize prepares the component for starting.
func (c *Component) Initialize() error {
	ctx := context.Background()
	return c.engine.Initialize(ctx)
}

// Start starts the component.
func (c *Component) Start(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "reactive-workflow", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "reactive-workflow", "Start", "context already cancelled")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return errs.ErrAlreadyStarted
	}

	if err := c.engine.Start(ctx); err != nil {
		return errs.Wrap(err, "reactive-workflow", "Start", "start engine")
	}

	c.started = true
	c.startTime = time.Now()

	c.logger.Info("Reactive workflow component started",
		"workflows", c.engine.Registry().Count())

	return nil
}

// Stop stops the component within the given timeout.
// The Engine.Stop() method performs synchronous cleanup (cancels context,
// stops tickers, stops watchers/consumers), so the timeout parameter is
// reserved for future async cleanup operations if needed.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.engine.Stop()
	c.started = false

	c.logger.Info("Reactive workflow component stopped")

	return nil
}

// Engine returns the underlying workflow engine.
// This allows callers to register workflows and interact with the engine directly.
func (c *Component) Engine() *Engine {
	return c.engine
}

// RegisterWorkflow registers a workflow definition with the engine.
// Workflows can be registered before or after Start() is called.
// Registering before Start() is recommended so triggers are active from the beginning.
func (c *Component) RegisterWorkflow(def *Definition) error {
	return c.engine.RegisterWorkflow(def)
}

// DebugStatus returns extended debug information for the component.
func (c *Component) DebugStatus() any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	registry := c.engine.Registry()
	workflows := registry.GetAll()

	workflowInfo := make([]map[string]any, 0, len(workflows))
	for _, def := range workflows {
		ruleInfo := make([]map[string]any, 0, len(def.Rules))
		for _, rule := range def.Rules {
			ruleInfo = append(ruleInfo, map[string]any{
				"id":           rule.ID,
				"trigger_mode": rule.Trigger.Mode().String(),
				"action_type":  rule.Action.Type.String(),
			})
		}

		workflowInfo = append(workflowInfo, map[string]any{
			"id":          def.ID,
			"description": def.Description,
			"rule_count":  len(def.Rules),
			"rules":       ruleInfo,
		})
	}

	return map[string]any{
		"started":        c.started,
		"engine_running": c.engine.IsRunning(),
		"uptime_seconds": c.engine.Uptime().Seconds(),
		"workflow_count": registry.Count(),
		"workflows":      workflowInfo,
		"config": map[string]any{
			"state_bucket":           c.config.StateBucket,
			"callback_stream":        c.config.CallbackStreamName,
			"event_stream":           c.config.EventStreamName,
			"default_timeout":        c.config.DefaultTimeout,
			"default_max_iterations": c.config.DefaultMaxIterations,
		},
	}
}

// buildDefaultInputPorts creates default input ports.
func buildDefaultInputPorts() []component.Port {
	defaultCfg := DefaultConfig()
	if defaultCfg.Ports == nil {
		return nil
	}

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

// buildDefaultOutputPorts creates default output ports.
func buildDefaultOutputPorts() []component.Port {
	defaultCfg := DefaultConfig()
	if defaultCfg.Ports == nil {
		return nil
	}

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
