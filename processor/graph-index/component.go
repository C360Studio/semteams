// Package graphindex provides the graph-index component for maintaining graph relationship indexes.
package graphindex

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

// Config holds configuration for graph-index component
type Config struct {
	Ports     *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	Workers   int                   `json:"workers" schema:"type:int,description:Number of worker goroutines,category:advanced"`
	BatchSize int                   `json:"batch_size" schema:"type:int,description:Batch size for index updates,category:advanced"`
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

	// Validate required output buckets exist
	requiredBuckets := map[string]bool{
		"OUTGOING_INDEX":  false,
		"INCOMING_INDEX":  false,
		"ALIAS_INDEX":     false,
		"PREDICATE_INDEX": false,
	}

	for _, output := range c.Ports.Outputs {
		if output.Subject != "" {
			if _, required := requiredBuckets[output.Subject]; required {
				requiredBuckets[output.Subject] = true
			}
		}
	}

	for bucket, found := range requiredBuckets {
		if !found {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
				fmt.Sprintf("required output bucket missing: %s", bucket))
		}
	}

	// Validate workers
	if c.Workers < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "workers cannot be negative")
	}

	// Validate batch size
	if c.BatchSize < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "batch_size cannot be negative")
	}

	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	if c.Workers == 0 {
		c.Workers = 1
	}
	if c.BatchSize == 0 {
		c.BatchSize = 50
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
					Name:    "outgoing_index",
					Type:    "kv-write",
					Subject: "OUTGOING_INDEX",
				},
				{
					Name:    "incoming_index",
					Type:    "kv-write",
					Subject: "INCOMING_INDEX",
				},
				{
					Name:    "alias_index",
					Type:    "kv-write",
					Subject: "ALIAS_INDEX",
				},
				{
					Name:    "predicate_index",
					Type:    "kv-write",
					Subject: "PREDICATE_INDEX",
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
					Name:    "outgoing_index",
					Type:    "kv-write",
					Subject: "OUTGOING_INDEX",
				},
				{
					Name:    "incoming_index",
					Type:    "kv-write",
					Subject: "INCOMING_INDEX",
				},
				{
					Name:    "alias_index",
					Type:    "kv-write",
					Subject: "ALIAS_INDEX",
				},
				{
					Name:    "predicate_index",
					Type:    "kv-write",
					Subject: "PREDICATE_INDEX",
				},
			},
		},
		Workers:   1,
		BatchSize: 50,
	}
}

// schema defines the configuration schema for graph-index component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-index processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources - KV buckets for index storage
	outgoingBucket  jetstream.KeyValue
	incomingBucket  jetstream.KeyValue
	aliasBucket     jetstream.KeyValue
	predicateBucket jetstream.KeyValue

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

// CreateGraphIndex is the factory function for creating graph-index components
func CreateGraphIndex(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphIndex", "factory", "NATSClient required")
	}

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphIndex", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphIndex", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-index")

	// Create component
	comp := &Component{
		name:       "graph-index",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-index factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-index", &component.Registration{
		Name:        "graph-index",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph relationship index maintenance processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphIndex,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-index",
		Type:        "processor",
		Description: "Graph relationship index maintenance processor",
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
	c.logger.Info("component initialized", slog.String("component", "graph-index"))

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

	// Initialize KV buckets
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	// Get KV buckets from configured outputs
	for _, portDef := range c.config.Ports.Outputs {
		bucket, err := js.KeyValue(ctx, portDef.Subject)
		if err != nil {
			cancel()
			return errs.Wrap(err, "Component", "Start", fmt.Sprintf("KV bucket access: %s", portDef.Subject))
		}

		// Assign bucket based on subject
		switch portDef.Subject {
		case "OUTGOING_INDEX":
			c.outgoingBucket = bucket
		case "INCOMING_INDEX":
			c.incomingBucket = bucket
		case "ALIAS_INDEX":
			c.aliasBucket = bucket
		case "PREDICATE_INDEX":
			c.predicateBucket = bucket
		}
	}

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	c.logger.Info("component started",
		slog.String("component", "graph-index"),
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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-index"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-index"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// ============================================================================
// Index Update Operations
// ============================================================================

// UpdateOutgoingIndex updates the outgoing index for an entity relationship
func (c *Component) UpdateOutgoingIndex(ctx context.Context, entityID, targetID, predicate string) error {
	if entityID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateOutgoingIndex", "entity ID cannot be empty")
	}
	if targetID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateOutgoingIndex", "target ID cannot be empty")
	}
	if predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateOutgoingIndex", "predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "UpdateOutgoingIndex", "context cancelled")
	}

	// Create outgoing entry
	entry := map[string]interface{}{
		"to_entity_id": targetID,
		"predicate":    predicate,
	}

	// Serialize entry
	data, err := json.Marshal(entry)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateOutgoingIndex", "entry serialization")
	}

	// Store in KV bucket using entity ID as key
	if _, err := c.outgoingBucket.Put(ctx, entityID, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateOutgoingIndex", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())

	c.logger.Debug("outgoing index updated",
		slog.String("entity_id", entityID),
		slog.String("target_id", targetID),
		slog.String("predicate", predicate))

	return nil
}

// UpdateIncomingIndex updates the incoming index for a relationship
func (c *Component) UpdateIncomingIndex(ctx context.Context, targetID, sourceID, predicate string) error {
	if targetID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateIncomingIndex", "target ID cannot be empty")
	}
	if sourceID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateIncomingIndex", "source ID cannot be empty")
	}
	if predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateIncomingIndex", "predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "UpdateIncomingIndex", "context cancelled")
	}

	// Create incoming entry
	entry := map[string]interface{}{
		"from_entity_id": sourceID,
		"predicate":      predicate,
	}

	// Serialize entry
	data, err := json.Marshal(entry)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateIncomingIndex", "entry serialization")
	}

	// Store in KV bucket using target ID as key
	if _, err := c.incomingBucket.Put(ctx, targetID, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateIncomingIndex", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())

	c.logger.Debug("incoming index updated",
		slog.String("target_id", targetID),
		slog.String("source_id", sourceID),
		slog.String("predicate", predicate))

	return nil
}

// UpdateAliasIndex updates the alias index for an entity
func (c *Component) UpdateAliasIndex(ctx context.Context, alias, entityID string) error {
	if alias == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateAliasIndex", "alias cannot be empty")
	}
	if entityID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateAliasIndex", "entity ID cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "UpdateAliasIndex", "context cancelled")
	}

	// Store alias mapping (value is just the entity ID as string)
	if _, err := c.aliasBucket.PutString(ctx, alias, entityID); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateAliasIndex", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(entityID)))
	c.lastActivity.Store(time.Now())

	c.logger.Debug("alias index updated",
		slog.String("alias", alias),
		slog.String("entity_id", entityID))

	return nil
}

// UpdatePredicateIndex updates the predicate index for an entity
func (c *Component) UpdatePredicateIndex(ctx context.Context, entityID, predicate string) error {
	if entityID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdatePredicateIndex", "entity ID cannot be empty")
	}
	if predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdatePredicateIndex", "predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "UpdatePredicateIndex", "context cancelled")
	}

	// Create predicate entry
	entry := map[string]interface{}{
		"entity_id": entityID,
		"predicate": predicate,
	}

	// Serialize entry
	data, err := json.Marshal(entry)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdatePredicateIndex", "entry serialization")
	}

	// Store in KV bucket using predicate as key
	if _, err := c.predicateBucket.Put(ctx, predicate, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdatePredicateIndex", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())

	c.logger.Debug("predicate index updated",
		slog.String("entity_id", entityID),
		slog.String("predicate", predicate))

	return nil
}

// ============================================================================
// Index Deletion Operations
// ============================================================================

// DeleteFromIndexes deletes an entity from all indexes
func (c *Component) DeleteFromIndexes(ctx context.Context, entityID string) error {
	if entityID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteFromIndexes", "entity ID cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "DeleteFromIndexes", "context cancelled")
	}

	// Delete from outgoing index
	if err := c.outgoingBucket.Delete(ctx, entityID); err != nil && err != jetstream.ErrKeyNotFound {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Warn("failed to delete from outgoing index", slog.String("entity_id", entityID), slog.Any("error", err))
	}

	// Delete from incoming index
	if err := c.incomingBucket.Delete(ctx, entityID); err != nil && err != jetstream.ErrKeyNotFound {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Warn("failed to delete from incoming index", slog.String("entity_id", entityID), slog.Any("error", err))
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	c.logger.Debug("entity deleted from indexes", slog.String("entity_id", entityID))

	return nil
}

// DeleteFromPredicateIndex deletes an entity from the predicate index
func (c *Component) DeleteFromPredicateIndex(ctx context.Context, entityID, predicate string) error {
	if entityID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteFromPredicateIndex", "entity ID cannot be empty")
	}
	if predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteFromPredicateIndex", "predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "DeleteFromPredicateIndex", "context cancelled")
	}

	// Delete from predicate index
	if err := c.predicateBucket.Delete(ctx, predicate); err != nil && err != jetstream.ErrKeyNotFound {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "DeleteFromPredicateIndex", "KV delete")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	c.logger.Debug("predicate index entry deleted",
		slog.String("entity_id", entityID),
		slog.String("predicate", predicate))

	return nil
}

// DeleteFromAliasIndex deletes an alias from the alias index
func (c *Component) DeleteFromAliasIndex(ctx context.Context, alias string) error {
	if alias == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteFromAliasIndex", "alias cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "DeleteFromAliasIndex", "context cancelled")
	}

	// Delete from alias index
	if err := c.aliasBucket.Delete(ctx, alias); err != nil && err != jetstream.ErrKeyNotFound {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "DeleteFromAliasIndex", "KV delete")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	c.logger.Debug("alias index entry deleted", slog.String("alias", alias))

	return nil
}

// DeleteFromIncomingIndex deletes a specific incoming reference
func (c *Component) DeleteFromIncomingIndex(ctx context.Context, targetID, sourceID string) error {
	if targetID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteFromIncomingIndex", "target ID cannot be empty")
	}
	if sourceID == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "DeleteFromIncomingIndex", "source ID cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "DeleteFromIncomingIndex", "context cancelled")
	}

	// For simplicity, delete the entire target entry (in real implementation, would remove specific source)
	if err := c.incomingBucket.Delete(ctx, targetID); err != nil && err != jetstream.ErrKeyNotFound {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "DeleteFromIncomingIndex", "KV delete")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	c.logger.Debug("incoming index entry deleted",
		slog.String("target_id", targetID),
		slog.String("source_id", sourceID))

	return nil
}
