// Package graphquery implements the query coordinator component for the graph subsystem.
// It orchestrates queries across graph-ingest and graph-index components and provides
// PathRAG traversal capabilities.
package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/resource"
	"github.com/nats-io/nats.go/jetstream"
)

// Note: jetstream import is used for KV types (jetstream.KeyValue, jetstream.KeyWatcher).
// This is the standard pattern across all processor components.
// natsclient wraps NATS operations but doesn't abstract jetstream types.

// natsRequester is a local interface for NATS request/reply and KV operations.
// *natsclient.Client satisfies this interface, and tests can provide mocks.
type natsRequester interface {
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
	SubscribeForRequests(ctx context.Context, subject string, handler func(ctx context.Context, data []byte) ([]byte, error)) error
	Status() natsclient.ConnectionStatus
	Connect(ctx context.Context) error
	WaitForConnection(ctx context.Context) error
	JetStream() (jetstream.JetStream, error)
	GetKeyValueBucket(ctx context.Context, name string) (jetstream.KeyValue, error)
}

// Config defines the configuration for the graph-query coordinator component
type Config struct {
	Ports        *component.PortConfig `json:"ports,omitempty"`
	QueryTimeout time.Duration         `json:"query_timeout,omitempty"`
	MaxDepth     int                   `json:"max_depth,omitempty"`

	// Resource startup settings for optional KV bucket dependencies (e.g., COMMUNITY_INDEX).
	// These control how long Start() waits for optional features to become available.
	// Production: use defaults (10 attempts × 500ms = 5s total).
	// Tests: use 1 attempt with 1ms interval for instant failure on missing buckets.
	StartupAttempts int           `json:"startup_attempts,omitempty"`
	StartupInterval time.Duration `json:"startup_interval,omitempty"`

	// RecheckInterval controls how often to check for bucket availability after startup timeout.
	// If bucket doesn't exist at startup, the component will recheck at this interval.
	// Default: 5s (allows recovery within reasonable time for distributed startup).
	RecheckInterval time.Duration `json:"recheck_interval,omitempty"`
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Ports == nil || len(c.Ports.Inputs) == 0 {
		return errors.New("ports configuration with at least one input port is required")
	}
	return nil
}

// ApplyDefaults applies default values to the configuration
func (c *Config) ApplyDefaults() {
	if c.QueryTimeout == 0 {
		c.QueryTimeout = 5 * time.Second
	}
	if c.MaxDepth == 0 {
		c.MaxDepth = 10
	}
	// Resource startup defaults match resource.DefaultConfig()
	if c.StartupAttempts == 0 {
		c.StartupAttempts = 10
	}
	if c.StartupInterval == 0 {
		c.StartupInterval = 500 * time.Millisecond
	}
	// Use shorter recheck interval than resource.DefaultConfig (60s) for faster recovery
	if c.RecheckInterval == 0 {
		c.RecheckInterval = 5 * time.Second
	}
}

// DefaultConfig returns a default configuration for the graph-query coordinator
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "query_entity", Type: "nats-request", Subject: "graph.query.entity"},
				{Name: "query_relationships", Type: "nats-request", Subject: "graph.query.relationships"},
				{Name: "query_path_search", Type: "nats-request", Subject: "graph.query.pathSearch"},
			},
			Outputs: []component.PortDefinition{},
		},
		QueryTimeout: 5 * time.Second,
		MaxDepth:     10,
	}
}

// Component implements the graph query coordinator
type Component struct {
	config       Config
	natsClient   natsRequester
	pathSearcher *PathSearcher
	router       *StaticRouter
	logger       *slog.Logger

	// Community cache for GraphRAG (consumer-owned, KV watch based)
	communityCache   *CommunityCache
	communityWatcher *resource.Watcher

	// Lifecycle state
	mu          sync.RWMutex
	wg          sync.WaitGroup
	initialized bool
	started     bool
	ctx         context.Context
	cancel      context.CancelFunc

	// Health tracking
	healthMu   sync.RWMutex
	errorCount int
	lastError  error

	// Metrics tracking
	metricsMu         sync.RWMutex
	messagesProcessed int64
	bytesProcessed    int64
	errors            int64
	lastMetricsReset  time.Time

	// Lifecycle reporting
	lifecycleReporter component.LifecycleReporter
}

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// CreateGraphQuery creates a new graph query coordinator component
func CreateGraphQuery(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Parse configuration
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply defaults
	config.ApplyDefaults()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Validate dependencies - deps.NATSClient is typed as *natsclient.Client
	// which satisfies our natsRequester interface
	if deps.NATSClient == nil {
		return nil, errors.New("NATSClient dependency is required")
	}

	logger := deps.GetLoggerWithComponent("graph-query")

	// Create component
	comp := &Component{
		config:           config,
		natsClient:       deps.NATSClient, // Assign to interface field
		pathSearcher:     NewPathSearcher(deps.NATSClient, config.QueryTimeout, config.MaxDepth, logger),
		logger:           logger,
		lastMetricsReset: time.Now(),
	}

	return comp, nil
}

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Type:        "processor",
		Name:        "graph-query",
		Description: "Query coordinator for graph subsystem - orchestrates queries across graph-ingest and graph-index",
		Version:     "1.0.0",
	}
}

// InputPorts returns the component's input ports
func (c *Component) InputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.config.Ports == nil {
		return []component.Port{}
	}

	// Build ports from configuration
	ports := make([]component.Port, 0, len(c.config.Ports.Inputs))
	for _, portDef := range c.config.Ports.Inputs {
		port := component.BuildPortFromDefinition(portDef, component.DirectionInput)
		ports = append(ports, port)
	}

	return ports
}

// OutputPorts returns the component's output ports (none for query coordinator)
func (c *Component) OutputPorts() []component.Port {
	// Query coordinator has no output ports - it returns data via request/reply
	return []component.Port{}
}

// ConfigSchema returns the JSON schema for the component's configuration
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"ports": {
				Type:        "object",
				Description: "Port configuration for input and output connections",
			},
			"query_timeout": {
				Type:        "string",
				Description: "Timeout for query operations (e.g., '5s', '10s')",
			},
			"max_depth": {
				Type:        "integer",
				Description: "Maximum traversal depth for path search queries",
			},
		},
		Required: []string{"ports"},
	}
}

// Health returns the component's health status
func (c *Component) Health() component.HealthStatus {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()

	c.mu.RLock()
	started := c.started
	c.mu.RUnlock()

	healthy := started && c.natsClient.Status() == natsclient.StatusConnected

	var lastErrorStr string
	if c.lastError != nil {
		lastErrorStr = c.lastError.Error()
	}

	return component.HealthStatus{
		Healthy:    healthy,
		ErrorCount: c.errorCount,
		LastError:  lastErrorStr,
		Status:     c.getHealthMessage(healthy),
	}
}

func (c *Component) getHealthMessage(healthy bool) string {
	if !healthy {
		if c.lastError != nil {
			return c.lastError.Error()
		}
		return "not started or NATS disconnected"
	}
	return "ok"
}

// DataFlow returns the component's data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	elapsed := time.Since(c.lastMetricsReset).Seconds()
	if elapsed == 0 {
		elapsed = 1
	}

	return component.FlowMetrics{
		MessagesPerSecond: float64(c.messagesProcessed) / elapsed,
		BytesPerSecond:    float64(c.bytesProcessed) / elapsed,
		ErrorRate:         float64(c.errors) / elapsed,
	}
}

// Initialize initializes the component
func (c *Component) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	// Validate configuration
	if err := c.config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	c.initialized = true
	c.logger.Info("graph-query coordinator initialized")
	return nil
}

// Start starts the component
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return errors.New("component not initialized")
	}

	if c.started {
		return nil // Already started - idempotent
	}

	// Create component context
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Wait for NATS connection
	if err := c.natsClient.WaitForConnection(c.ctx); err != nil {
		return fmt.Errorf("wait for NATS connection: %w", err)
	}

	// Initialize lifecycle reporter (throttled for high-throughput queries)
	js, err := c.natsClient.JetStream()
	if err != nil {
		c.logger.Warn("Failed to get JetStream, lifecycle reporting disabled", slog.Any("error", err))
		c.lifecycleReporter = component.NewNoOpLifecycleReporter()
	} else {
		statusBucket, err := js.CreateOrUpdateKeyValue(c.ctx, jetstream.KeyValueConfig{
			Bucket:      "COMPONENT_STATUS",
			Description: "Component lifecycle status tracking",
		})
		if err != nil {
			c.logger.Warn("Failed to create COMPONENT_STATUS bucket, lifecycle reporting disabled",
				slog.Any("error", err))
			c.lifecycleReporter = component.NewNoOpLifecycleReporter()
		} else {
			c.lifecycleReporter = component.NewLifecycleReporterFromConfig(component.LifecycleReporterConfig{
				KV:               statusBucket,
				ComponentName:    "graph-query",
				Logger:           c.logger,
				EnableThrottling: true,
			})
		}
	}

	// Create router for static routing
	c.router = NewStaticRouter(c.logger)

	// Subscribe to query subjects
	if err := c.setupQueryHandlers(); err != nil {
		return fmt.Errorf("subscribe to queries: %w", err)
	}

	// Initialize community cache for GraphRAG
	c.communityCache = NewCommunityCache(c.logger)

	// Set up resource watcher for COMMUNITY_INDEX bucket
	// This handles graceful startup and recovery if the bucket appears later
	watcherCfg := resource.DefaultConfig()
	watcherCfg.StartupAttempts = c.config.StartupAttempts
	watcherCfg.StartupInterval = c.config.StartupInterval
	watcherCfg.RecheckInterval = c.config.RecheckInterval
	watcherCfg.Logger = c.logger
	watcherCfg.OnAvailable = func() {
		c.enableGraphRAG()
	}
	watcherCfg.OnLost = func() {
		c.disableGraphRAG()
	}

	c.communityWatcher = resource.NewWatcher(
		"COMMUNITY_INDEX",
		func(ctx context.Context) error {
			_, err := c.natsClient.GetKeyValueBucket(ctx, graph.BucketCommunityIndex)
			return err
		},
		watcherCfg,
	)

	// Try to get bucket during startup
	if c.communityWatcher.WaitForStartup(c.ctx) {
		// Bucket available - enable GraphRAG immediately
		if err := c.startGraphRAGWatcher(); err != nil {
			return fmt.Errorf("start GraphRAG watcher: %w", err)
		}
	} else {
		// Bucket not available - start background checking
		c.logger.Info("COMMUNITY_INDEX bucket not available at startup, GraphRAG disabled (will retry)")
		c.communityWatcher.StartBackgroundCheck(c.ctx)
	}

	c.started = true

	// Report initial idle state
	_ = c.lifecycleReporter.ReportStage(c.ctx, "idle")

	c.logger.Info("graph-query coordinator started")
	return nil
}

// Stop stops the component
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil // Not started - safe to stop
	}

	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()

	// Wait for background goroutines with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown
	case <-time.After(timeout):
		c.logger.Warn("stop timeout waiting for goroutines")
	}

	// Stop community watcher (background check goroutine)
	if c.communityWatcher != nil {
		c.communityWatcher.Stop()
	}

	// Stop community cache watcher
	if c.communityCache != nil {
		c.communityCache.Stop()
	}

	c.mu.Lock()
	c.started = false
	c.mu.Unlock()

	c.logger.Info("graph-query coordinator stopped")
	return nil
}

// startGraphRAGWatcher initializes and starts the community cache KV watcher.
// Called when COMMUNITY_INDEX bucket is available at startup.
func (c *Component) startGraphRAGWatcher() error {
	communityBucket, err := c.natsClient.GetKeyValueBucket(c.ctx, graph.BucketCommunityIndex)
	if err != nil {
		return fmt.Errorf("get COMMUNITY_INDEX bucket: %w", err)
	}

	// Start community cache watcher in background
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.communityCache.WatchAndSync(c.ctx, communityBucket); err != nil {
			if c.ctx.Err() == nil {
				c.logger.Error("community cache watcher failed", "error", err)
			}
		}
	}()

	// Register GraphRAG handlers
	if err := c.setupGraphRAGHandlers(); err != nil {
		return fmt.Errorf("setup GraphRAG handlers: %w", err)
	}

	c.logger.Info("GraphRAG enabled")
	return nil
}

// enableGraphRAG is called when COMMUNITY_INDEX bucket becomes available after being unavailable.
// This is the OnAvailable callback for the resource watcher.
func (c *Component) enableGraphRAG() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ctx == nil || c.ctx.Err() != nil {
		return // Component shutting down
	}

	if err := c.startGraphRAGWatcher(); err != nil {
		c.logger.Error("failed to enable GraphRAG after bucket became available", "error", err)
	}
}

// disableGraphRAG is called when COMMUNITY_INDEX bucket is lost.
// This is the OnLost callback for the resource watcher.
func (c *Component) disableGraphRAG() {
	c.logger.Warn("COMMUNITY_INDEX bucket lost, GraphRAG queries will fail until recovered")
	// Note: We don't stop the community cache watcher here because:
	// 1. The watcher will handle the bucket disappearing gracefully
	// 2. When bucket returns, we'll get the OnAvailable callback
	// The communityCache.IsAvailable() check in handlers will prevent queries
}

// reportQuerying reports the querying stage (throttled to avoid KV spam)
func (c *Component) reportQuerying(ctx context.Context) {
	if c.lifecycleReporter != nil {
		_ = c.lifecycleReporter.ReportStage(ctx, "querying")
	}
}

// recordSuccess records successful query metrics
func (c *Component) recordSuccess(bytesIn, bytesOut int) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.messagesProcessed++
	c.bytesProcessed += int64(bytesIn + bytesOut)
}

// recordError records error metrics and updates health
func (c *Component) recordError(err error) {
	c.metricsMu.Lock()
	c.errors++
	c.metricsMu.Unlock()

	c.healthMu.Lock()
	c.errorCount++
	c.lastError = err
	c.healthMu.Unlock()

	c.logger.Error("query failed", "error", err)
}

// Register registers the graph-query component factory with the registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-query", &component.Registration{
		Name:        "graph-query",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Query coordinator for graph subsystem",
		Version:     "1.0.0",
		Factory:     CreateGraphQuery,
		Schema:      DefaultConfig().Schema(),
	})
}

// Schema returns the configuration schema for the component
func (c Config) Schema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"ports": {
				Type:        "object",
				Description: "Port configuration for input and output connections",
			},
			"query_timeout": {
				Type:        "string",
				Description: "Timeout for query operations (e.g., '5s', '10s')",
			},
			"max_depth": {
				Type:        "integer",
				Description: "Maximum traversal depth for path search queries",
			},
		},
		Required: []string{"ports"},
	}
}
