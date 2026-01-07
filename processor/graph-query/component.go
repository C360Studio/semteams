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
	"github.com/c360/semstreams/natsclient"
)

// natsRequester is a local interface for NATS request/reply operations.
// *natsclient.Client satisfies this interface, and tests can provide mocks.
type natsRequester interface {
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
	SubscribeForRequests(ctx context.Context, subject string, handler func(ctx context.Context, data []byte) ([]byte, error)) error
	Status() natsclient.ConnectionStatus
	Connect(ctx context.Context) error
	WaitForConnection(ctx context.Context) error
}

// Config defines the configuration for the graph-query coordinator component
type Config struct {
	Ports        *component.PortConfig `json:"ports,omitempty"`
	QueryTimeout time.Duration         `json:"query_timeout,omitempty"`
	MaxDepth     int                   `json:"max_depth,omitempty"`
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
}

// DefaultConfig returns a default configuration for the graph-query coordinator
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "query_entity", Type: "nats-request", Subject: "graph.query.entity"},
				{Name: "query_relationships", Type: "nats-request", Subject: "graph.query.relationships"},
				{Name: "query_path_search", Type: "nats-request", Subject: "graph.query.pathSearch"},
				{Name: "query_capabilities", Type: "nats-request", Subject: "graph.query.capabilities"},
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
	logger       *slog.Logger

	// Lifecycle state
	mu          sync.RWMutex
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

	// Subscribe to query subjects
	if err := c.setupQueryHandlers(); err != nil {
		return fmt.Errorf("subscribe to queries: %w", err)
	}

	c.started = true
	c.logger.Info("graph-query coordinator started")
	return nil
}

// Stop stops the component
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil // Not started - safe to stop
	}

	if c.cancel != nil {
		c.cancel()
	}

	c.started = false
	c.logger.Info("graph-query coordinator stopped")
	return nil
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
