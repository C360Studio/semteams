// Package graphindextemporal provides the graph-index-temporal component for temporal indexing.
package graphindextemporal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Config holds configuration for graph-index-temporal component
type Config struct {
	Ports          *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	TimeResolution string                `json:"time_resolution" schema:"type:string,description:Time resolution (minute hour day),category:basic"`
	Workers        int                   `json:"workers" schema:"type:int,description:Number of worker goroutines,category:advanced"`
	BatchSize      int                   `json:"batch_size" schema:"type:int,description:Batch size for processing,category:advanced"`
}

// Validate implements component.Validatable interface
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Inputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one input port required")
	}
	if len(c.Ports.Outputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one output port required")
	}

	// Validate TEMPORAL_INDEX output exists
	hasTemporalIndex := false
	for _, output := range c.Ports.Outputs {
		if output.Subject == "TEMPORAL_INDEX" {
			hasTemporalIndex = true
			break
		}
	}
	if !hasTemporalIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "TEMPORAL_INDEX output required")
	}

	// Validate time resolution
	if c.TimeResolution == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "time_resolution required")
	}
	if c.TimeResolution != "minute" && c.TimeResolution != "hour" && c.TimeResolution != "day" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "time_resolution must be 'minute', 'hour', or 'day'")
	}

	// Validate workers
	if c.Workers <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "workers must be greater than 0")
	}

	// Validate batch size
	if c.BatchSize <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "batch_size must be greater than 0")
	}

	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	if c.TimeResolution == "" {
		c.TimeResolution = "hour"
	}
	if c.Workers == 0 {
		c.Workers = 4
	}
	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
	if c.Ports == nil {
		// Apply full default port config
		defaultConf := DefaultConfig()
		c.Ports = defaultConf.Ports
	} else {
		// If ports exist but are empty, populate with defaults
		if len(c.Ports.Inputs) == 0 {
			c.Ports.Inputs = []component.PortDefinition{
				{
					Name:    "entity_watch",
					Type:    "kv-watch",
					Subject: "ENTITY_STATES",
				},
			}
		}
		if len(c.Ports.Outputs) == 0 {
			c.Ports.Outputs = []component.PortDefinition{
				{
					Name:    "temporal_index",
					Type:    "kv-write",
					Subject: "TEMPORAL_INDEX",
				},
			}
		}
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:    "entity_watch",
					Type:    "kv-watch",
					Subject: "ENTITY_STATES",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "temporal_index",
					Type:    "kv-write",
					Subject: "TEMPORAL_INDEX",
				},
			},
		},
		TimeResolution: "hour",
		Workers:        4,
		BatchSize:      100,
	}
}

// schema defines the configuration schema for graph-index-temporal component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-index-temporal processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	temporalBucket jetstream.KeyValue

	// Lifecycle state
	mu          sync.RWMutex
	running     bool
	initialized bool
	startTime   time.Time
	wg          sync.WaitGroup
	cancel      context.CancelFunc

	// Metrics (atomic)
	messagesProcessed int64
	bytesProcessed    int64
	errors            int64
	lastActivity      atomic.Value // stores time.Time

	// Port definitions
	inputPorts  []component.Port
	outputPorts []component.Port
}

// CreateGraphIndexTemporal is the factory function for creating graph-index-temporal components
func CreateGraphIndexTemporal(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphIndexTemporal", "factory", "NATSClient required")
	}

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphIndexTemporal", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphIndexTemporal", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-index-temporal")

	// Create component
	comp := &Component{
		name:       "graph-index-temporal",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-index-temporal factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-index-temporal", &component.Registration{
		Name:        "graph-index-temporal",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph temporal indexing processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphIndexTemporal,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-index-temporal",
		Type:        "processor",
		Description: "Graph temporal indexing processor",
		Version:     "1.0.0",
	}
}

// InputPorts returns input port definitions
func (c *Component) InputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inputPorts
}

// OutputPorts returns output port definitions
func (c *Component) OutputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()
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

	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}

	errorCount := int(atomic.LoadInt64(&c.errors))
	lastErr := ""
	status := "stopped"

	if c.running {
		status = "running"
		if errorCount > 0 {
			lastErr = "errors occurred during processing"
		}
	}

	return component.HealthStatus{
		Healthy:    c.running && errorCount == 0,
		LastCheck:  time.Now(),
		ErrorCount: errorCount,
		LastError:  lastErr,
		Uptime:     uptime,
		Status:     status,
	}
}

// DataFlow returns current data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	messages := atomic.LoadInt64(&c.messagesProcessed)
	bytes := atomic.LoadInt64(&c.bytesProcessed)
	errorCount := atomic.LoadInt64(&c.errors)

	c.mu.RLock()
	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}
	c.mu.RUnlock()

	// Calculate rates
	var messagesPerSec, bytesPerSec, errorRate float64
	if uptime > 0 {
		seconds := uptime.Seconds()
		messagesPerSec = float64(messages) / seconds
		bytesPerSec = float64(bytes) / seconds
		if messages > 0 {
			errorRate = float64(errorCount) / float64(messages)
		}
	}

	lastAct := time.Now()
	if stored := c.lastActivity.Load(); stored != nil {
		if t, ok := stored.(time.Time); ok {
			lastAct = t
		}
	}

	return component.FlowMetrics{
		MessagesPerSecond: messagesPerSec,
		BytesPerSecond:    bytesPerSec,
		ErrorRate:         errorRate,
		LastActivity:      lastAct,
	}
}

// ============================================================================
// LifecycleComponent Interface (3 methods)
// ============================================================================

// Initialize validates configuration and sets up ports (no I/O)
func (c *Component) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil // Idempotent
	}

	// Validate configuration
	if err := c.config.Validate(); err != nil {
		return errs.Wrap(err, "Component", "Initialize", "config validation")
	}

	// Build input ports from config
	c.inputPorts = make([]component.Port, len(c.config.Ports.Inputs))
	for i, portDef := range c.config.Ports.Inputs {
		c.inputPorts[i] = component.BuildPortFromDefinition(portDef, component.DirectionInput)
	}

	// Build output ports from config
	c.outputPorts = make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		c.outputPorts[i] = component.BuildPortFromDefinition(portDef, component.DirectionOutput)
	}

	c.initialized = true
	c.logger.Info("component initialized", slog.String("component", "graph-index-temporal"))

	return nil
}

// Start begins processing (must be initialized first)
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check initialization
	if !c.initialized {
		return errs.WrapFatal(fmt.Errorf("component not initialized"), "Component", "Start", "initialization check")
	}

	// Idempotent - already running
	if c.running {
		return nil
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	// Initialize JetStream
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	// Get TEMPORAL_INDEX bucket
	temporalBucket, err := js.KeyValue(ctx, "TEMPORAL_INDEX")
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket access: TEMPORAL_INDEX")
	}
	c.temporalBucket = temporalBucket

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	c.logger.Info("component started",
		slog.String("component", "graph-index-temporal"),
		slog.Time("start_time", c.startTime),
		slog.String("time_resolution", c.config.TimeResolution),
		slog.Int("workers", c.config.Workers))

	return nil
}

// Stop gracefully shuts down the component
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return nil // Already stopped
	}

	// Cancel context
	if c.cancel != nil {
		c.cancel()
	}

	c.running = false
	c.mu.Unlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-index-temporal"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-index-temporal"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}
