// Package graphclustering provides the graph-clustering component for community detection.
package graphclustering

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

// Config holds configuration for graph-clustering component
type Config struct {
	Ports             *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	DetectionInterval time.Duration         `json:"detection_interval" schema:"type:string,description:Interval between community detection runs,category:basic"`
	BatchSize         int                   `json:"batch_size" schema:"type:int,description:Event count threshold for triggering detection,category:basic"`
	EnableLLM         bool                  `json:"enable_llm" schema:"type:bool,description:Enable LLM-based community summarization,category:advanced"`
	LLMEndpoint       string                `json:"llm_endpoint" schema:"type:string,description:URL for LLM endpoint (required if enable_llm is true),category:advanced"`
	MinCommunitySize  int                   `json:"min_community_size" schema:"type:int,description:Minimum number of entities to form a community,category:advanced"`
	MaxIterations     int                   `json:"max_iterations" schema:"type:int,description:Maximum iterations for LPA algorithm,category:advanced"`
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

	// Validate COMMUNITY_INDEX output exists
	hasCommunityIndex := false
	for _, output := range c.Ports.Outputs {
		if output.Subject == "COMMUNITY_INDEX" {
			hasCommunityIndex = true
			break
		}
	}
	if !hasCommunityIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "COMMUNITY_INDEX output required")
	}

	// Validate detection interval
	if c.DetectionInterval <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "detection_interval must be greater than 0")
	}

	// If LLM is enabled, endpoint is required
	if c.EnableLLM && c.LLMEndpoint == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "llm_endpoint required when enable_llm is true")
	}

	// Validate min community size
	if c.MinCommunitySize <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "min_community_size must be greater than 0")
	}

	// Validate max iterations
	if c.MaxIterations <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "max_iterations must be greater than 0")
	}

	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	if c.DetectionInterval == 0 {
		c.DetectionInterval = 30 * time.Second
	}
	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
	// EnableLLM defaults to false (zero value)
	if c.MinCommunitySize == 0 {
		c.MinCommunitySize = 3
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = 100
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
					Name:    "communities",
					Type:    "kv-write",
					Subject: "COMMUNITY_INDEX",
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
					Name:    "communities",
					Type:    "kv-write",
					Subject: "COMMUNITY_INDEX",
				},
			},
		},
		DetectionInterval: 30 * time.Second,
		BatchSize:         100,
		EnableLLM:         false,
		MinCommunitySize:  3,
		MaxIterations:     100,
	}
}

// schema defines the configuration schema for graph-clustering component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-clustering processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	communityBucket jetstream.KeyValue

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

// CreateGraphClustering is the factory function for creating graph-clustering components
func CreateGraphClustering(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphClustering", "factory", "NATSClient required")
	}

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphClustering", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphClustering", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-clustering")

	// Create component
	comp := &Component{
		name:       "graph-clustering",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-clustering factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-clustering", &component.Registration{
		Name:        "graph-clustering",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph community detection and clustering processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphClustering,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-clustering",
		Type:        "processor",
		Description: "Graph community detection and clustering processor",
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
	c.logger.Info("component initialized", slog.String("component", "graph-clustering"))

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

	// Get COMMUNITY_INDEX bucket
	communityBucket, err := js.KeyValue(ctx, "COMMUNITY_INDEX")
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket access: COMMUNITY_INDEX")
	}
	c.communityBucket = communityBucket

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	c.logger.Info("component started",
		slog.String("component", "graph-clustering"),
		slog.Time("start_time", c.startTime),
		slog.Duration("detection_interval", c.config.DetectionInterval),
		slog.Bool("enable_llm", c.config.EnableLLM))

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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-clustering"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-clustering"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}
