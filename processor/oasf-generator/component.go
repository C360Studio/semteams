package oasfgenerator

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// componentSchema defines the configuration schema
var componentSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Component implements the OASF generator processor.
type Component struct {
	name       string
	config     Config
	natsClient *natsclient.Client
	logger     *slog.Logger
	mapper     *Mapper
	generator  *Generator
	metrics    *Metrics

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// KV watcher
	kvWatcher jetstream.KeyWatcher

	// Metrics tracking
	recordsGenerated int64
	errors           int64
	lastActivity     time.Time

	// Context for background operations
	ctx    context.Context
	cancel context.CancelFunc
}

// NewComponent creates a new OASF generator component.
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

	// Create mapper with config values
	mapper := NewMapper(
		config.DefaultAgentVersion,
		config.DefaultAuthors,
		config.IncludeExtensions,
	)

	return &Component{
		name:       "oasf-generator",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		mapper:     mapper,
		metrics:    newMetrics(deps.MetricsRegistry),
	}, nil
}

// Initialize prepares the component.
func (c *Component) Initialize() error {
	// Create generator (depends on mapper and NATS client)
	c.generator = NewGenerator(c.mapper, c.natsClient, c.config, c.logger)
	return nil
}

// Start begins watching for entity changes and generating OASF records.
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

	// Initialize generator (sets up KV stores)
	if err := c.generator.Initialize(c.ctx); err != nil {
		c.cancel()
		return errs.Wrap(err, "Component", "Start", "initialize generator")
	}

	// Start KV watcher
	if err := c.startKVWatcher(c.ctx); err != nil {
		c.cancel()
		return errs.Wrap(err, "Component", "Start", "start KV watcher")
	}

	c.running = true
	c.startTime = time.Now()

	c.logger.Info("OASF generator started",
		slog.String("entity_kv_bucket", c.config.EntityKVBucket),
		slog.String("oasf_kv_bucket", c.config.OASFKVBucket),
		slog.String("watch_pattern", c.config.WatchPattern))

	return nil
}

// startKVWatcher starts watching the entity KV bucket for changes.
func (c *Component) startKVWatcher(ctx context.Context) error {
	// Get the entity KV bucket
	kv, err := c.natsClient.GetKeyValueBucket(ctx, c.config.EntityKVBucket)
	if err != nil {
		return errs.Wrap(err, "Component", "startKVWatcher", "get entity KV bucket")
	}

	// Create watcher with pattern
	pattern := c.config.WatchPattern
	if pattern == "" {
		pattern = ">"
	}

	watcher, err := kv.Watch(ctx, pattern, jetstream.IgnoreDeletes())
	if err != nil {
		return errs.Wrap(err, "Component", "startKVWatcher", "create KV watcher")
	}
	c.kvWatcher = watcher

	// Start background goroutine to process updates
	go c.watchLoop(ctx)

	return nil
}

// watchLoop processes KV updates in a background goroutine.
func (c *Component) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-c.kvWatcher.Updates():
			if !ok {
				// Watcher closed
				return
			}
			if entry == nil {
				// Initial values complete
				continue
			}

			c.handleEntityChange(entry)
		}
	}
}

// handleEntityChange processes a single entity change from KV.
func (c *Component) handleEntityChange(entry jetstream.KeyValueEntry) {
	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()

	if c.metrics != nil {
		c.metrics.EntityChanged()
	}

	entityID := entry.Key()
	c.logger.Debug("Entity changed, queuing OASF generation",
		slog.String("entity_id", entityID))

	// Queue for generation (with debouncing)
	c.generator.QueueGeneration(entityID)
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

	// Stop KV watcher
	if c.kvWatcher != nil {
		if err := c.kvWatcher.Stop(); err != nil {
			c.logger.Warn("Failed to stop KV watcher", slog.Any("error", err))
		}
		c.kvWatcher = nil
	}

	// Stop generator
	if c.generator != nil {
		c.generator.Stop()
	}

	c.running = false
	c.logger.Info("OASF generator stopped")

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "oasf-generator",
		Type:        "processor",
		Description: "Generates OASF records from agent entity capabilities",
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
	total := c.recordsGenerated + c.errors
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

// GenerateForEntity manually triggers OASF generation for an entity.
// This is useful for testing and on-demand generation.
func (c *Component) GenerateForEntity(ctx context.Context, entityID string) error {
	return c.generator.GenerateForEntity(ctx, entityID)
}
