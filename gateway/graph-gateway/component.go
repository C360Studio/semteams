// Package graphgateway provides the graph-gateway component for exposing graph operations via HTTP.
package graphgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/gateway"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
	_ gateway.Gateway              = (*Component)(nil)
)

// Config holds configuration for graph-gateway component
type Config struct {
	Ports            *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	GraphQLPath      string                `json:"graphql_path" schema:"type:string,description:GraphQL endpoint path,category:basic"`
	MCPPath          string                `json:"mcp_path" schema:"type:string,description:MCP endpoint path,category:basic"`
	BindAddress      string                `json:"bind_address" schema:"type:string,description:HTTP server bind address,category:basic"`
	EnablePlayground bool                  `json:"enable_playground" schema:"type:bool,description:Enable GraphQL playground,category:basic"`
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
	if c.GraphQLPath == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "graphql_path cannot be empty")
	}
	if c.MCPPath == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "mcp_path cannot be empty")
	}
	if c.BindAddress == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "bind_address cannot be empty")
	}
	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	if c.GraphQLPath == "" {
		c.GraphQLPath = "/graphql"
	}
	if c.MCPPath == "" {
		c.MCPPath = "/mcp"
	}
	if c.BindAddress == "" {
		c.BindAddress = "localhost:8080"
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
					Name:    "http",
					Type:    "http",
					Subject: "/graphql",
				},
			}
		}
		if len(c.Ports.Outputs) == 0 {
			c.Ports.Outputs = []component.PortDefinition{
				{
					Name:    "mutations",
					Type:    "nats-request",
					Subject: "graph.mutation.*",
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
					Name:    "http",
					Type:    "http",
					Subject: "/graphql",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "mutations",
					Type:    "nats-request",
					Subject: "graph.mutation.*",
				},
			},
		},
		GraphQLPath:      "/graphql",
		MCPPath:          "/mcp",
		BindAddress:      "localhost:8080",
		EnablePlayground: false,
	}
}

// schema defines the configuration schema for graph-gateway component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-gateway gateway
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

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

// CreateGraphGateway is the factory function for creating graph-gateway components
func CreateGraphGateway(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphGateway", "factory", "NATSClient required")
	}

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphGateway", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphGateway", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-gateway")

	// Create component
	comp := &Component{
		name:       "graph-gateway",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-gateway factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-gateway", &component.Registration{
		Name:        "graph-gateway",
		Type:        "gateway",
		Protocol:    "http",
		Domain:      "graph",
		Description: "Graph operations HTTP gateway",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphGateway,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-gateway",
		Type:        "gateway",
		Description: "Graph operations HTTP gateway",
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
	c.logger.Info("component initialized", slog.String("component", "graph-gateway"))

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

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	c.logger.Info("component started",
		slog.String("component", "graph-gateway"),
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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-gateway"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-gateway"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// ============================================================================
// Gateway Interface (1 method)
// ============================================================================

// RegisterHTTPHandlers registers HTTP handlers with the provided mux
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	// Ensure prefix ends with slash for proper path joining
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}

	// Register GraphQL handler
	graphqlPath := prefix + c.config.GraphQLPath
	if graphqlPath[0] != '/' {
		graphqlPath = "/" + graphqlPath
	}
	// Clean double slashes
	for i := 0; i < len(graphqlPath)-1; i++ {
		if graphqlPath[i] == '/' && graphqlPath[i+1] == '/' {
			graphqlPath = graphqlPath[:i] + graphqlPath[i+1:]
			i--
		}
	}
	mux.HandleFunc(graphqlPath, c.handleGraphQL)

	// Register MCP handler
	mcpPath := prefix + c.config.MCPPath
	if mcpPath[0] != '/' {
		mcpPath = "/" + mcpPath
	}
	// Clean double slashes
	for i := 0; i < len(mcpPath)-1; i++ {
		if mcpPath[i] == '/' && mcpPath[i+1] == '/' {
			mcpPath = mcpPath[:i] + mcpPath[i+1:]
			i--
		}
	}
	mux.HandleFunc(mcpPath, c.handleMCP)

	// Register playground if enabled
	if c.config.EnablePlayground {
		playgroundPath := prefix
		if playgroundPath == "" {
			playgroundPath = "/"
		}
		// Ensure trailing slash
		if playgroundPath[len(playgroundPath)-1] != '/' {
			playgroundPath = playgroundPath + "/"
		}
		mux.HandleFunc(playgroundPath, c.handlePlayground)
	}

	c.logger.Info("HTTP handlers registered",
		slog.String("graphql_path", graphqlPath),
		slog.String("mcp_path", mcpPath),
		slog.Bool("playground_enabled", c.config.EnablePlayground))
}

// ============================================================================
// HTTP Handlers
// ============================================================================

// handleGraphQL handles GraphQL requests
func (c *Component) handleGraphQL(w http.ResponseWriter, _ *http.Request) {
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	// For now, return a simple response
	// In real implementation, this would handle GraphQL queries
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"data": map[string]interface{}{
			"message": "GraphQL endpoint",
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("failed to encode response", slog.Any("error", err))
	}
}

// handleMCP handles MCP requests
func (c *Component) handleMCP(w http.ResponseWriter, _ *http.Request) {
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	// For now, return a simple response
	// In real implementation, this would handle MCP protocol
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"message": "MCP endpoint",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("failed to encode response", slog.Any("error", err))
	}
}

// handlePlayground handles GraphQL playground requests
func (c *Component) handlePlayground(w http.ResponseWriter, _ *http.Request) {
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	// Return simple HTML playground
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	html := `<!DOCTYPE html>
<html>
<head>
    <title>GraphQL Playground</title>
</head>
<body>
    <h1>GraphQL Playground</h1>
    <p>GraphQL endpoint: ` + c.config.GraphQLPath + `</p>
</body>
</html>`
	if _, err := w.Write([]byte(html)); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("failed to write playground response", slog.Any("error", err))
	}
}
