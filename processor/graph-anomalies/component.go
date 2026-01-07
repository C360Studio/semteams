// Package graphanomalies provides the graph-anomalies component for cluster/community anomaly detection.
package graphanomalies

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
	"github.com/c360/semstreams/graph/structural"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/pkg/retry"
	"github.com/nats-io/nats.go/jetstream"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Config holds configuration for graph-anomalies component
type Config struct {
	Ports              *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	ComputeIntervalStr string                `json:"compute_interval" schema:"type:string,description:Interval between structural index computations (e.g. 1h or 30m),category:basic"`
	PivotCount         int                   `json:"pivot_count" schema:"type:int,description:Number of pivot nodes for distance indexing,category:basic"`
	MaxHopDistance     int                   `json:"max_hop_distance" schema:"type:int,description:Maximum hop distance for BFS traversal,category:basic"`
	ComputeOnStartup   bool                  `json:"compute_on_startup" schema:"type:bool,description:Compute indices immediately on startup,category:basic"`

	// Parsed duration (set by ApplyDefaults)
	computeInterval time.Duration
}

// ComputeInterval returns the parsed compute interval duration
func (c *Config) ComputeInterval() time.Duration {
	return c.computeInterval
}

// Validate implements component.Validatable interface
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Inputs) < 2 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "two input ports required (OUTGOING_INDEX and INCOMING_INDEX)")
	}
	if len(c.Ports.Outputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one output port required")
	}

	// Validate OUTGOING_INDEX and INCOMING_INDEX inputs
	hasOutgoingIndex := false
	hasIncomingIndex := false
	for _, input := range c.Ports.Inputs {
		if input.Subject == graph.BucketOutgoingIndex {
			hasOutgoingIndex = true
		}
		if input.Subject == graph.BucketIncomingIndex {
			hasIncomingIndex = true
		}
	}
	if !hasOutgoingIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf("%s input required", graph.BucketOutgoingIndex))
	}
	if !hasIncomingIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf("%s input required", graph.BucketIncomingIndex))
	}

	// Validate STRUCTURAL_INDEX output exists
	hasStructuralIndex := false
	for _, output := range c.Ports.Outputs {
		if output.Subject == graph.BucketStructuralIndex {
			hasStructuralIndex = true
			break
		}
	}
	if !hasStructuralIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf("%s output required", graph.BucketStructuralIndex))
	}

	// Validate compute interval (parsed duration must be positive)
	if c.computeInterval <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "compute_interval must be greater than 0")
	}

	// Validate pivot count
	if c.PivotCount <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "pivot_count must be greater than 0")
	}

	// Validate max hop distance
	if c.MaxHopDistance <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "max_hop_distance must be greater than 0")
	}

	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	// Parse compute interval from string
	if c.ComputeIntervalStr != "" {
		if d, err := time.ParseDuration(c.ComputeIntervalStr); err == nil {
			c.computeInterval = d
		}
	}
	if c.computeInterval == 0 {
		c.computeInterval = 1 * time.Hour
	}
	if c.PivotCount == 0 {
		c.PivotCount = 16
	}
	if c.MaxHopDistance == 0 {
		c.MaxHopDistance = 10
	}
	// ComputeOnStartup defaults to true when not explicitly set
	// The challenge: bool zero value is false, so we can't distinguish "not set" from "explicitly false"
	// Solution: Always default to true. Users wanting false must set it AFTER ApplyDefaults,
	// or the factory must handle it specially
	if !c.ComputeOnStartup {
		c.ComputeOnStartup = true
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
					Name:    "outgoing_watch",
					Type:    "kv-watch",
					Subject: graph.BucketOutgoingIndex,
				},
				{
					Name:    "incoming_watch",
					Type:    "kv-watch",
					Subject: graph.BucketIncomingIndex,
				},
			}
		}
		if len(c.Ports.Outputs) == 0 {
			c.Ports.Outputs = []component.PortDefinition{
				{
					Name:    "structural_index",
					Type:    "kv-write",
					Subject: graph.BucketStructuralIndex,
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
					Name:    "outgoing_watch",
					Type:    "kv-watch",
					Subject: graph.BucketOutgoingIndex,
				},
				{
					Name:    "incoming_watch",
					Type:    "kv-watch",
					Subject: graph.BucketIncomingIndex,
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "structural_index",
					Type:    "kv-write",
					Subject: graph.BucketStructuralIndex,
				},
			},
		},
		computeInterval:  1 * time.Hour,
		PivotCount:       16,
		MaxHopDistance:   10,
		ComputeOnStartup: true,
	}
}

// schema defines the configuration schema for graph-anomalies component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-anomalies processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	storage          structural.Storage
	kcoreComputer    *structural.KCoreComputer
	pivotComputer    *structural.PivotComputer
	graphProvider    structural.GraphProvider
	structuralBucket jetstream.KeyValue

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

// CreateGraphAnomalies is the factory function for creating graph-anomalies components
func CreateGraphAnomalies(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphAnomalies", "factory", "NATSClient required")
	}
	natsClient := deps.NATSClient

	// Parse configuration
	var config Config
	var userSetComputeOnStartup bool
	if len(rawConfig) > 0 {
		// First unmarshal to detect if ComputeOnStartup was explicitly set
		var rawMap map[string]interface{}
		if err := json.Unmarshal(rawConfig, &rawMap); err != nil {
			return nil, errs.Wrap(err, "CreateGraphAnomalies", "factory", "config unmarshal")
		}
		_, userSetComputeOnStartup = rawMap["compute_on_startup"]

		// Now unmarshal into config struct
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphAnomalies", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Preserve user's explicit ComputeOnStartup choice if provided
	computeOnStartupValue := config.ComputeOnStartup

	// Apply defaults and validate
	config.ApplyDefaults()

	// Restore user's explicit choice for ComputeOnStartup if they set it
	if userSetComputeOnStartup {
		config.ComputeOnStartup = computeOnStartupValue
	}

	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphAnomalies", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-anomalies")

	// Create component
	comp := &Component{
		name:       "graph-anomalies",
		config:     config,
		natsClient: natsClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-anomalies factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-anomalies", &component.Registration{
		Name:        "graph-anomalies",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph cluster/community anomaly detection processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphAnomalies,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-anomalies",
		Type:        "processor",
		Description: "Graph cluster/community anomaly detection processor",
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
	c.logger.Info("component initialized", slog.String("component", "graph-anomalies"))

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

	// Initialize JetStream for input bucket retries
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	// Create STRUCTURAL_INDEX bucket (we are the WRITER)
	structuralBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketStructuralIndex,
		Description: "Structural index for anomaly detection",
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "create output bucket: "+graph.BucketStructuralIndex)
	}
	c.structuralBucket = structuralBucket

	// Create storage
	c.storage = structural.NewNATSStructuralIndexStorage(structuralBucket)

	// Wait for OUTGOING_INDEX bucket with retries (we are the READER)
	outgoingBucket, err := retry.DoWithResult(ctx, retry.Persistent(), func() (jetstream.KeyValue, error) {
		bucket, err := js.KeyValue(ctx, graph.BucketOutgoingIndex)
		if err != nil {
			c.logger.Debug("waiting for input bucket", slog.String("bucket", graph.BucketOutgoingIndex), slog.Any("error", err))
		}
		return bucket, err
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "input bucket not available: "+graph.BucketOutgoingIndex)
	}

	// Wait for INCOMING_INDEX bucket with retries (we are the READER)
	incomingBucket, err := retry.DoWithResult(ctx, retry.Persistent(), func() (jetstream.KeyValue, error) {
		bucket, err := js.KeyValue(ctx, graph.BucketIncomingIndex)
		if err != nil {
			c.logger.Debug("waiting for input bucket", slog.String("bucket", graph.BucketIncomingIndex), slog.Any("error", err))
		}
		return bucket, err
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "input bucket not available: "+graph.BucketIncomingIndex)
	}

	// Create graph provider
	c.graphProvider = newKVGraphProvider(outgoingBucket, incomingBucket, c.logger)

	// Create computers
	c.kcoreComputer = structural.NewKCoreComputer(c.graphProvider, c.logger)
	c.pivotComputer = structural.NewPivotComputer(c.graphProvider, c.config.PivotCount, c.logger)

	// Set up query handlers
	if err := c.setupQueryHandlers(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "setup query handlers")
	}

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	// Start compute loop goroutine
	c.wg.Add(1)
	go c.runComputeLoop(ctx)

	// Compute on startup if configured
	if c.config.ComputeOnStartup {
		go func() {
			// Small delay to allow other components to start
			time.Sleep(5 * time.Second)
			c.computeStructuralIndices(ctx)
		}()
	}

	c.logger.Info("component started",
		slog.String("component", "graph-anomalies"),
		slog.Time("start_time", c.startTime),
		slog.Duration("compute_interval", c.config.ComputeInterval()),
		slog.Int("pivot_count", c.config.PivotCount),
		slog.Int("max_hop_distance", c.config.MaxHopDistance),
		slog.Bool("compute_on_startup", c.config.ComputeOnStartup))

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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-anomalies"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-anomalies"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// ============================================================================
// Graph Provider Implementation
// ============================================================================

// kvGraphProvider implements structural.GraphProvider using NATS KV buckets
type kvGraphProvider struct {
	outgoingBucket jetstream.KeyValue
	incomingBucket jetstream.KeyValue
	logger         *slog.Logger
}

// newKVGraphProvider creates a new KV-backed graph provider
func newKVGraphProvider(outgoing, incoming jetstream.KeyValue, logger *slog.Logger) *kvGraphProvider {
	return &kvGraphProvider{
		outgoingBucket: outgoing,
		incomingBucket: incoming,
		logger:         logger,
	}
}

// GetAllEntityIDs returns all entity IDs by scanning both index buckets
func (p *kvGraphProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	// Use a map to deduplicate entity IDs from both buckets
	entitySet := make(map[string]bool)

	// Scan OUTGOING_INDEX
	if p.outgoingBucket != nil {
		keys, err := p.outgoingBucket.Keys(ctx)
		if err == nil {
			for _, key := range keys {
				entitySet[key] = true
			}
		}
	}

	// Scan INCOMING_INDEX
	if p.incomingBucket != nil {
		keys, err := p.incomingBucket.Keys(ctx)
		if err == nil {
			for _, key := range keys {
				entitySet[key] = true
			}
		}
	}

	// Convert to slice
	entityIDs := make([]string, 0, len(entitySet))
	for id := range entitySet {
		entityIDs = append(entityIDs, id)
	}

	return entityIDs, nil
}

// GetNeighbors returns neighbors based on direction
func (p *kvGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	switch direction {
	case "outgoing":
		return p.getNeighborsFromBucket(ctx, p.outgoingBucket, entityID)
	case "incoming":
		return p.getNeighborsFromBucket(ctx, p.incomingBucket, entityID)
	case "both":
		// Combine outgoing and incoming neighbors
		outgoing, _ := p.getNeighborsFromBucket(ctx, p.outgoingBucket, entityID)
		incoming, _ := p.getNeighborsFromBucket(ctx, p.incomingBucket, entityID)

		// Deduplicate
		neighborSet := make(map[string]bool)
		for _, n := range outgoing {
			neighborSet[n] = true
		}
		for _, n := range incoming {
			neighborSet[n] = true
		}

		neighbors := make([]string, 0, len(neighborSet))
		for n := range neighborSet {
			neighbors = append(neighbors, n)
		}
		return neighbors, nil
	default:
		return nil, fmt.Errorf("invalid direction: %s", direction)
	}
}

// getNeighborsFromBucket retrieves neighbors from a specific bucket
func (p *kvGraphProvider) getNeighborsFromBucket(ctx context.Context, bucket jetstream.KeyValue, entityID string) ([]string, error) {
	if bucket == nil {
		return []string{}, nil
	}

	entry, err := bucket.Get(ctx, entityID)
	if err != nil {
		return []string{}, nil // Entity has no neighbors in this bucket
	}

	var neighbors []string
	if err := json.Unmarshal(entry.Value(), &neighbors); err != nil {
		return []string{}, fmt.Errorf("unmarshal neighbors: %w", err)
	}

	return neighbors, nil
}

// GetEdgeWeight returns edge weight (simplified - returns 1.0 if edge exists)
func (p *kvGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	neighbors, err := p.GetNeighbors(ctx, fromID, "outgoing")
	if err != nil {
		return 0.0, err
	}

	for _, n := range neighbors {
		if n == toID {
			return 1.0, nil
		}
	}

	return 0.0, nil
}

// ============================================================================
// Compute Loop
// ============================================================================

// runComputeLoop runs structural index computations on a timer
func (c *Component) runComputeLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.ComputeInterval())
	defer ticker.Stop()

	c.logger.Info("compute loop started",
		slog.Duration("interval", c.config.ComputeInterval()))

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("compute loop stopping")
			return
		case <-ticker.C:
			c.computeStructuralIndices(ctx)
		}
	}
}

// computeStructuralIndices runs k-core and pivot distance computations
func (c *Component) computeStructuralIndices(ctx context.Context) {
	c.logger.Info("computing structural indices")
	start := time.Now()

	// Track total entities processed
	totalEntities := 0

	// Compute k-core decomposition
	kcoreIndex, err := c.kcoreComputer.Compute(ctx)
	if err != nil {
		c.logger.Error("k-core computation failed", slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
	} else if kcoreIndex != nil {
		// Store k-core index
		if err := c.storage.SaveKCoreIndex(ctx, kcoreIndex); err != nil {
			c.logger.Error("failed to save k-core index", slog.Any("error", err))
			atomic.AddInt64(&c.errors, 1)
		} else {
			totalEntities = kcoreIndex.EntityCount
			c.logger.Info("k-core computation complete",
				slog.Int("entities", kcoreIndex.EntityCount),
				slog.Int("max_core", kcoreIndex.MaxCore))
		}
	}

	// Compute pivot distances
	pivotIndex, err := c.pivotComputer.Compute(ctx)
	if err != nil {
		c.logger.Error("pivot computation failed", slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
	} else if pivotIndex != nil {
		// Store pivot index
		if err := c.storage.SavePivotIndex(ctx, pivotIndex); err != nil {
			c.logger.Error("failed to save pivot index", slog.Any("error", err))
			atomic.AddInt64(&c.errors, 1)
		} else {
			if pivotIndex.EntityCount > totalEntities {
				totalEntities = pivotIndex.EntityCount
			}
			c.logger.Info("pivot computation complete",
				slog.Int("entities", pivotIndex.EntityCount),
				slog.Int("pivots", len(pivotIndex.Pivots)))
		}
	}

	if totalEntities > 0 {
		atomic.AddInt64(&c.messagesProcessed, int64(totalEntities))
		c.lastActivity.Store(time.Now())
	}

	c.logger.Info("structural indices computed",
		slog.Int("entities_processed", totalEntities),
		slog.Duration("duration", time.Since(start)))
}
