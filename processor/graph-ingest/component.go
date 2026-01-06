// Package graphingest provides the graph-ingest component for entity and triple ingestion.
package graphingest

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
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Config holds configuration for graph-ingest component
type Config struct {
	Ports           *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	EnableHierarchy bool                  `json:"enable_hierarchy" schema:"type:bool,description:Enable hierarchy inference,category:advanced"`
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
	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	// EnableHierarchy defaults to false
	if c.Ports == nil {
		c.Ports = &component.PortConfig{}
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:    "entity_stream",
					Type:    "jetstream",
					Subject: "entity.>",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "entity_states",
					Type:    "kv-write",
					Subject: "ENTITY_STATES",
				},
			},
		},
		EnableHierarchy: false,
	}
}

// schema defines the configuration schema for graph-ingest component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-ingest processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	entityBucket jetstream.KeyValue

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

// CreateGraphIngest is the factory function for creating graph-ingest components
func CreateGraphIngest(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphIngest", "factory", "NATSClient required")
	}

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphIngest", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphIngest", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-ingest")

	// Create component
	comp := &Component{
		name:       "graph-ingest",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-ingest factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-ingest", &component.Registration{
		Name:        "graph-ingest",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Entity and triple ingestion processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphIngest,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-ingest",
		Type:        "processor",
		Description: "Entity and triple ingestion processor for graph system",
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
	c.logger.Info("component initialized", slog.String("component", "graph-ingest"))

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

	// Initialize KV bucket
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	bucket, err := js.KeyValue(ctx, "ENTITY_STATES")
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket access")
	}
	c.entityBucket = bucket

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	c.logger.Info("component started",
		slog.String("component", "graph-ingest"),
		slog.Time("start_time", c.startTime))

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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-ingest"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-ingest"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// ============================================================================
// Entity Operations
// ============================================================================

// CreateEntity creates a new entity in the graph
func (c *Component) CreateEntity(ctx context.Context, entity *graph.EntityState) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "CreateEntity", "entity cannot be nil")
	}
	if entity.ID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "CreateEntity", "entity ID cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "CreateEntity", "context cancelled")
	}

	// Serialize entity
	data, err := json.Marshal(entity)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "CreateEntity", "entity serialization")
	}

	// Store in KV bucket
	if _, err := c.entityBucket.Put(ctx, entity.ID, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "CreateEntity", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())

	c.logger.Debug("entity created",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))

	return nil
}

// UpdateEntity updates an existing entity
func (c *Component) UpdateEntity(ctx context.Context, entity *graph.EntityState) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateEntity", "entity cannot be nil")
	}
	if entity.ID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateEntity", "entity ID cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "UpdateEntity", "context cancelled")
	}

	// Serialize entity
	data, err := json.Marshal(entity)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateEntity", "entity serialization")
	}

	// Update in KV bucket
	if _, err := c.entityBucket.Put(ctx, entity.ID, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateEntity", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())

	c.logger.Debug("entity updated",
		slog.String("entity_id", entity.ID),
		slog.Uint64("version", entity.Version))

	return nil
}

// DeleteEntity removes an entity from the graph
func (c *Component) DeleteEntity(ctx context.Context, entityID string) error {
	if entityID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteEntity", "entity ID cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "DeleteEntity", "context cancelled")
	}

	// Delete from KV bucket
	if err := c.entityBucket.Delete(ctx, entityID); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "DeleteEntity", "KV delete")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	c.logger.Debug("entity deleted", slog.String("entity_id", entityID))

	return nil
}

// ============================================================================
// Triple Operations
// ============================================================================

// AddTriple adds a triple to an entity
func (c *Component) AddTriple(ctx context.Context, triple message.Triple) error {
	if triple.Subject == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "AddTriple", "triple subject cannot be empty")
	}
	if triple.Predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "AddTriple", "triple predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "AddTriple", "context cancelled")
	}

	// Get existing entity
	entry, err := c.entityBucket.Get(ctx, triple.Subject)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Create new entity with this triple
			entity := &graph.EntityState{
				ID:        triple.Subject,
				Triples:   []message.Triple{triple},
				Version:   1,
				UpdatedAt: time.Now(),
			}
			return c.CreateEntity(ctx, entity)
		}
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "AddTriple", "entity lookup")
	}

	// Deserialize existing entity
	var entity graph.EntityState
	if err := json.Unmarshal(entry.Value(), &entity); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "AddTriple", "entity deserialization")
	}

	// Add triple
	entity.Triples = append(entity.Triples, triple)
	entity.Version++
	entity.UpdatedAt = time.Now()

	// Update entity
	return c.UpdateEntity(ctx, &entity)
}

// RemoveTriple removes a triple from an entity
func (c *Component) RemoveTriple(ctx context.Context, subject, predicate string) error {
	if subject == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "RemoveTriple", "subject cannot be empty")
	}
	if predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "RemoveTriple", "predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "RemoveTriple", "context cancelled")
	}

	// Get existing entity
	entry, err := c.entityBucket.Get(ctx, subject)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil // Entity doesn't exist, nothing to remove
		}
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "RemoveTriple", "entity lookup")
	}

	// Deserialize existing entity
	var entity graph.EntityState
	if err := json.Unmarshal(entry.Value(), &entity); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "RemoveTriple", "entity deserialization")
	}

	// Remove matching triples
	filtered := make([]message.Triple, 0, len(entity.Triples))
	for _, t := range entity.Triples {
		if t.Predicate != predicate {
			filtered = append(filtered, t)
		}
	}

	entity.Triples = filtered
	entity.Version++
	entity.UpdatedAt = time.Now()

	// Update entity
	return c.UpdateEntity(ctx, &entity)
}
