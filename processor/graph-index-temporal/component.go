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

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/resource"
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

	// Dependency startup configuration
	StartupAttempts int `json:"startup_attempts,omitempty" schema:"type:int,description:Max attempts to wait for dependencies at startup,category:advanced"`
	StartupInterval int `json:"startup_interval_ms,omitempty" schema:"type:int,description:Interval between startup attempts in milliseconds,category:advanced"`
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
		if output.Subject == graph.BucketTemporalIndex {
			hasTemporalIndex = true
			break
		}
	}
	if !hasTemporalIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf("%s output required", graph.BucketTemporalIndex))
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

	// Dependency startup defaults
	if c.StartupAttempts == 0 {
		c.StartupAttempts = 30 // ~15 seconds with 500ms interval
	}
	if c.StartupInterval == 0 {
		c.StartupInterval = 500 // milliseconds
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
					Subject: graph.BucketEntityStates,
				},
			}
		}
		if len(c.Ports.Outputs) == 0 {
			c.Ports.Outputs = []component.PortDefinition{
				{
					Name:    "temporal_index",
					Type:    "kv-write",
					Subject: graph.BucketTemporalIndex,
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
					Subject: graph.BucketEntityStates,
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "temporal_index",
					Type:    "kv-write",
					Subject: graph.BucketTemporalIndex,
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

	// Lifecycle reporting
	lifecycleReporter component.LifecycleReporter

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
	natsClient := deps.NATSClient

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
		natsClient: natsClient,
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

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "context cancelled")
	}

	// Create TEMPORAL_INDEX bucket (we are the WRITER)
	temporalBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketTemporalIndex,
		Description: "Temporal index for time-based queries",
	})
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled during bucket creation")
		}
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("KV bucket creation: %s", graph.BucketTemporalIndex))
	}
	c.temporalBucket = temporalBucket

	// Initialize lifecycle reporter early for dependency waiting visibility
	c.initLifecycleReporter(ctx)

	// Set up query handlers
	if err := c.setupQueryHandlers(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "setup query handlers")
	}

	// Get JetStream for bucket access
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	// Report waiting stage before dependency check
	if err := c.lifecycleReporter.ReportStage(ctx, "waiting_for_"+graph.BucketEntityStates); err != nil {
		c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "waiting_for_"+graph.BucketEntityStates), slog.Any("error", err))
	}

	// Configure resource watcher for bounded startup attempts
	watcherCfg := resource.DefaultConfig()
	watcherCfg.StartupAttempts = c.config.StartupAttempts
	watcherCfg.StartupInterval = time.Duration(c.config.StartupInterval) * time.Millisecond
	watcherCfg.Logger = c.logger

	entityWatcher := resource.NewWatcher(
		graph.BucketEntityStates,
		func(checkCtx context.Context) error {
			_, err := js.KeyValue(checkCtx, graph.BucketEntityStates)
			return err
		},
		watcherCfg,
	)

	if !entityWatcher.WaitForStartup(ctx) {
		cancel()
		return errs.WrapTransient(
			fmt.Errorf("bucket %s not available after %d attempts", graph.BucketEntityStates, c.config.StartupAttempts),
			"Component", "Start", "dependency not available",
		)
	}

	entityBucket, err := js.KeyValue(ctx, graph.BucketEntityStates)
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("get %s bucket", graph.BucketEntityStates))
	}

	// Start entity watcher goroutine
	c.wg.Add(1)
	go c.watchEntityStates(ctx, entityBucket)

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	// Report initial idle state
	if err := c.lifecycleReporter.ReportStage(ctx, "idle"); err != nil {
		c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "idle"), slog.Any("error", err))
	}

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
		return errs.WrapTransient(fmt.Errorf("timeout after %v", timeout), "Component", "Stop", "graceful shutdown timeout")
	}
}

// initLifecycleReporter initializes the lifecycle reporter for component status tracking.
func (c *Component) initLifecycleReporter(ctx context.Context) {
	statusBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketComponentStatus,
		Description: "Component lifecycle status tracking",
	})
	if err != nil {
		c.logger.Warn("Failed to create COMPONENT_STATUS bucket, lifecycle reporting disabled",
			slog.Any("error", err))
		c.lifecycleReporter = component.NewNoOpLifecycleReporter()
		return
	}
	c.lifecycleReporter = component.NewLifecycleReporterFromConfig(component.LifecycleReporterConfig{
		KV:               statusBucket,
		ComponentName:    "graph-index-temporal",
		Logger:           c.logger,
		EnableThrottling: true,
	})
}

// ============================================================================
// Entity State Watcher
// ============================================================================

// watchEntityStates watches the ENTITY_STATES KV bucket and indexes entities with temporal data
func (c *Component) watchEntityStates(ctx context.Context, bucket jetstream.KeyValue) {
	defer c.wg.Done()

	watcher, err := bucket.WatchAll(ctx)
	if err != nil {
		c.logger.Error("failed to start entity watcher",
			slog.String("bucket", graph.BucketEntityStates),
			slog.Any("error", err))
		return
	}
	// NOTE: watcher.Stop() is called explicitly before each return, not via defer.
	// This avoids a race condition in nats.go where Stop() can race with the
	// internal message handler goroutine when using defer.

	c.logger.Info("entity watcher started", slog.String("bucket", graph.BucketEntityStates))

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("entity watcher stopping", slog.String("reason", "context cancelled"))
			watcher.Stop()
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				// Channel closed, watcher stopped externally
				watcher.Stop()
				return
			}
			if entry == nil {
				// nil entry indicates initial state enumeration complete
				c.logger.Debug("entity watcher initial sync complete")
				continue
			}

			if entry.Operation() == jetstream.KeyValueDelete {
				c.handleEntityDelete(ctx, entry.Key())
				continue
			}

			c.processEntityUpdate(ctx, entry)
		}
	}
}

// processEntityUpdate indexes an entity's temporal data if it has timestamps
func (c *Component) processEntityUpdate(ctx context.Context, entry jetstream.KeyValueEntry) {
	// Report indexing stage (throttled to avoid KV spam)
	if err := c.lifecycleReporter.ReportStage(ctx, "indexing"); err != nil {
		c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "indexing"), slog.Any("error", err))
	}

	var state graph.EntityState
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		c.logger.Warn("failed to unmarshal entity state",
			slog.String("entity", entry.Key()),
			slog.Any("error", err))
		return
	}

	// Determine entity ID
	entityID := state.ID
	if entityID == "" {
		entityID = entry.Key()
	}
	if entityID == "" && len(state.Triples) > 0 {
		entityID = state.Triples[0].Subject
	}

	// Use entity's UpdatedAt timestamp (matching indexmanager/indexes.go pattern)
	// The UpdatedAt field is always set when the entity is stored, providing
	// consistent temporal indexing without relying on triple predicates
	ts := state.UpdatedAt
	if ts.IsZero() {
		// Fallback to triple-based extraction if UpdatedAt is not set
		if extracted := c.extractTimestamp(state.Triples); extracted != nil {
			ts = *extracted
		} else {
			// Skip if no timestamp available
			return
		}
	}

	// Calculate time bucket based on resolution
	timeBucket := c.calculateTimeBucket(ts)

	// Update temporal index
	if err := c.updateTemporalIndex(ctx, timeBucket, entityID, ts); err != nil {
		c.logger.Warn("failed to update temporal index",
			slog.String("entity", entityID),
			slog.String("bucket", timeBucket),
			slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
		return
	}

	c.logger.Debug("indexed entity temporal data",
		slog.String("entity", entityID),
		slog.String("bucket", timeBucket),
		slog.Time("timestamp", ts))

	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())
}

// extractTimestamp extracts a timestamp from entity triples
func (c *Component) extractTimestamp(triples []message.Triple) *time.Time {
	for _, triple := range triples {
		switch triple.Predicate {
		case "core.time.timestamp", "timestamp", "time.timestamp", "time.observation.recorded", "created_at", "updated_at":
			// Try to parse the object as various time formats
			switch v := triple.Object.(type) {
			case time.Time:
				return &v
			case string:
				// Try RFC3339 first
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					return &t
				}
				// Try RFC3339Nano
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					return &t
				}
				// Try common ISO format
				if t, err := time.Parse("2006-01-02T15:04:05Z", v); err == nil {
					return &t
				}
			case float64:
				// Unix timestamp
				t := time.Unix(int64(v), 0)
				return &t
			case int64:
				t := time.Unix(v, 0)
				return &t
			}
		}
	}
	return nil
}

// calculateTimeBucket calculates the time bucket key based on configured resolution
// Uses dot-separated format to match indexmanager/manager.go:1518 QueryTemporal expectations
func (c *Component) calculateTimeBucket(ts time.Time) string {
	t := ts.UTC()
	switch c.config.TimeResolution {
	case "minute":
		return fmt.Sprintf("%04d.%02d.%02d.%02d.%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute())
	case "hour":
		return fmt.Sprintf("%04d.%02d.%02d.%02d", t.Year(), t.Month(), t.Day(), t.Hour())
	case "day":
		return fmt.Sprintf("%04d.%02d.%02d", t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("%04d.%02d.%02d.%02d", t.Year(), t.Month(), t.Day(), t.Hour()) // Default to hour
	}
}

// updateTemporalIndex updates the temporal index bucket for a time bucket
// Uses events array format to match indexmanager/indexes.go:1520-1552 and QueryTemporal expectations
func (c *Component) updateTemporalIndex(ctx context.Context, timeBucket, entityID string, ts time.Time) error {
	// Get current data for this time bucket
	entry, err := c.temporalBucket.Get(ctx, timeBucket)

	var temporalData map[string]interface{}
	if err == nil {
		if err := json.Unmarshal(entry.Value(), &temporalData); err != nil {
			temporalData = map[string]interface{}{
				"events":       []interface{}{},
				"entity_count": 0,
			}
		}
	} else {
		temporalData = map[string]interface{}{
			"events":       []interface{}{},
			"entity_count": 0,
		}
	}

	// Get or create events array
	events, _ := temporalData["events"].([]interface{})

	// Append new event (accumulate all events, matching indexmanager pattern)
	newEvent := map[string]interface{}{
		"entity":    entityID,
		"type":      "update",
		"timestamp": ts.Format(time.RFC3339),
	}
	events = append(events, newEvent)
	temporalData["events"] = events

	// Track unique entity count
	uniqueEntities := make(map[string]bool)
	for _, evt := range events {
		if eventMap, ok := evt.(map[string]interface{}); ok {
			if entity, ok := eventMap["entity"].(string); ok {
				uniqueEntities[entity] = true
			}
		}
	}
	temporalData["entity_count"] = len(uniqueEntities)

	// Serialize and write
	data, err := json.Marshal(temporalData)
	if err != nil {
		return errs.Wrap(err, "Component", "updateTemporalIndex", "marshal temporal data")
	}

	if entry != nil {
		_, err = c.temporalBucket.Update(ctx, timeBucket, data, entry.Revision())
	} else {
		_, err = c.temporalBucket.Create(ctx, timeBucket, data)
	}

	if err != nil {
		return errs.Wrap(err, "Component", "updateTemporalIndex", "write temporal data")
	}

	return nil
}

// handleEntityDelete handles entity deletion from the temporal index
func (c *Component) handleEntityDelete(_ context.Context, entityID string) {
	c.logger.Debug("entity deleted - temporal cleanup not fully implemented",
		slog.String("entity", entityID))
}
