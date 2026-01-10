// Package graphgateway provides the graph-gateway component for exposing graph operations via HTTP.
package graphgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/gateway"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// natsRequester is a local interface for NATS request/reply operations.
// *natsclient.Client satisfies this interface, and tests can provide mocks.
type natsRequester interface {
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
	Status() natsclient.ConnectionStatus
}

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
	QueryTimeout     time.Duration         `json:"query_timeout" schema:"type:duration,description:Query timeout duration,category:basic"`
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
	if c.QueryTimeout == 0 {
		c.QueryTimeout = 30 * time.Second
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
		QueryTimeout:     30 * time.Second,
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
	natsClient    *natsclient.Client
	natsRequester natsRequester // Interface for NATS request/reply (mockable)
	logger        *slog.Logger

	// Lifecycle reporting
	lifecycleReporter component.LifecycleReporter

	// HTTP server for GraphQL endpoint
	httpServer *http.Server
	httpMux    *http.ServeMux

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
	natsClient := deps.NATSClient

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
		name:          "graph-gateway",
		config:        config,
		natsClient:    natsClient,
		natsRequester: natsClient, // Assign to interface field for mockability
		logger:        logger,
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

	// Initialize lifecycle reporter (throttled for high-throughput serving)
	statusBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
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
			ComponentName:    "graph-gateway",
			Logger:           c.logger,
			EnableThrottling: true,
		})
	}

	// Create HTTP server mux and register handlers
	c.httpMux = http.NewServeMux()
	c.RegisterHTTPHandlers("", c.httpMux)

	// Create HTTP server
	c.httpServer = &http.Server{
		Addr:    c.config.BindAddress,
		Handler: c.httpMux,
	}

	// Start HTTP server in background
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.logger.Info("starting HTTP server",
			slog.String("bind_address", c.config.BindAddress))

		if err := c.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("HTTP server error",
				slog.Any("error", err))
		}
	}()

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	c.logger.Info("component started",
		slog.String("component", "graph-gateway"),
		slog.String("bind_address", c.config.BindAddress),
		slog.Time("start_time", c.startTime))

	// Report initial idle state
	if c.lifecycleReporter != nil {
		if err := c.lifecycleReporter.ReportStage(ctx, "idle"); err != nil {
			c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "idle"), slog.Any("error", err))
		}
	}

	return nil
}

// Stop gracefully shuts down the component
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return nil // Already stopped
	}

	// Shutdown HTTP server gracefully
	if c.httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), timeout)
		defer shutdownCancel()

		c.logger.Info("shutting down HTTP server")
		if err := c.httpServer.Shutdown(shutdownCtx); err != nil {
			c.logger.Warn("HTTP server shutdown error", slog.Any("error", err))
		}
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

// mapGraphQLQueryToNATSSubject maps a GraphQL query to a NATS subject
func (c *Component) mapGraphQLQueryToNATSSubject(query string) string {
	query = strings.ToLower(query)

	// IMPORTANT: Check specific patterns BEFORE generic ones
	// "entityidhierarchy" and "entitybyalias" contain "entity" - must check first

	// Most specific patterns first
	if strings.Contains(query, "entityidhierarchy") {
		return "graph.query.hierarchyStats"
	}
	if strings.Contains(query, "entitybyalias") {
		return "graph.query.entityByAlias"
	}
	if strings.Contains(query, "entitiesbyprefix") {
		return "graph.query.prefix"
	}
	if strings.Contains(query, "pathsearch") {
		return "graph.query.pathSearch"
	}
	if strings.Contains(query, "spatialsearch") {
		return "graph.query.spatial"
	}
	if strings.Contains(query, "temporalsearch") {
		return "graph.query.temporal"
	}
	if strings.Contains(query, "semanticsearch") || strings.Contains(query, "textsearch") || strings.Contains(query, "similaritysearch") {
		return "graph.query.semantic"
	}
	if strings.Contains(query, "findsimilar") || strings.Contains(query, "similarentities") {
		return "graph.query.similar"
	}
	if strings.Contains(query, "relationships") {
		return "graph.query.relationships"
	}
	if strings.Contains(query, "capabilities") {
		return "graph.query.capabilities"
	}

	// GraphRAG search patterns - must come before generic "entity" check
	if strings.Contains(query, "localsearch") {
		return "graph.query.localSearch"
	}
	if strings.Contains(query, "globalsearch") {
		return "graph.query.globalSearch"
	}

	// Generic "entity" check MUST come last
	if strings.Contains(query, "entity") {
		return "graph.query.entity"
	}

	return "graph.query.unknown"
}

// subjectToGraphQLField maps a NATS subject to the GraphQL response field name
func (c *Component) subjectToGraphQLField(subject string) string {
	switch subject {
	case "graph.query.pathSearch":
		return "pathSearch"
	case "graph.query.entity":
		return "entity"
	case "graph.query.entityByAlias":
		return "entityByAlias"
	case "graph.query.relationships":
		return "relationships"
	case "graph.query.capabilities":
		return "capabilities"
	case "graph.query.hierarchyStats":
		return "entityIdHierarchy"
	case "graph.query.prefix":
		return "entitiesByPrefix"
	case "graph.query.spatial":
		return "spatialSearch"
	case "graph.query.temporal":
		return "temporalSearch"
	case "graph.query.semantic":
		return "similaritySearch"
	case "graph.query.similar":
		return "findSimilar"
	case "graph.query.localSearch":
		return "localSearch"
	case "graph.query.globalSearch":
		return "globalSearch"
	default:
		return ""
	}
}

// transformVariablesToNATSPayload transforms GraphQL variables to NATS payload format
func (c *Component) transformVariablesToNATSPayload(variables map[string]interface{}, subject string) map[string]interface{} {
	if variables == nil {
		return map[string]interface{}{}
	}

	payload := make(map[string]interface{})

	// Transform based on query type
	switch subject {
	case "graph.query.pathSearch":
		// Transform GraphQL variable names to NATS format
		if start, ok := variables["start"]; ok {
			payload["start_entity"] = start
		}
		if startEntity, ok := variables["startEntity"]; ok {
			payload["start_entity"] = startEntity
		}
		if startEntityVal, ok := variables["start_entity"]; ok {
			payload["start_entity"] = startEntityVal
		}

		if depth, ok := variables["depth"]; ok {
			payload["max_depth"] = depth
		}
		if maxDepth, ok := variables["maxDepth"]; ok {
			payload["max_depth"] = maxDepth
		}
		if maxDepthVal, ok := variables["max_depth"]; ok {
			payload["max_depth"] = maxDepthVal
		}

		// maxNodes parameter for limiting traversal
		if nodes, ok := variables["nodes"]; ok {
			payload["max_nodes"] = nodes
		}
		if maxNodes, ok := variables["maxNodes"]; ok {
			payload["max_nodes"] = maxNodes
		}
		if maxNodesVal, ok := variables["max_nodes"]; ok {
			payload["max_nodes"] = maxNodesVal
		}

	case "graph.query.entity":
		// Pass through id field
		if id, ok := variables["id"]; ok {
			payload["id"] = id
		}

	case "graph.query.relationships":
		// Pass through entity_id field
		if entityID, ok := variables["entityId"]; ok {
			payload["entity_id"] = entityID
		}
		if entityIDVal, ok := variables["entity_id"]; ok {
			payload["entity_id"] = entityIDVal
		}
		// Pass through direction field (convert GraphQL enum INCOMING/OUTGOING to lowercase)
		if direction, ok := variables["direction"].(string); ok {
			payload["direction"] = strings.ToLower(direction)
		}

	case "graph.query.hierarchyStats", "graph.query.prefix":
		// Pass through prefix field
		if prefix, ok := variables["prefix"]; ok {
			payload["prefix"] = prefix
		}
		if limit, ok := variables["limit"]; ok {
			payload["limit"] = limit
		}

	case "graph.query.spatial":
		// Pass through bounding box parameters
		if north, ok := variables["north"]; ok {
			payload["north"] = north
		}
		if south, ok := variables["south"]; ok {
			payload["south"] = south
		}
		if east, ok := variables["east"]; ok {
			payload["east"] = east
		}
		if west, ok := variables["west"]; ok {
			payload["west"] = west
		}
		if limit, ok := variables["limit"]; ok {
			payload["limit"] = limit
		}

	case "graph.query.temporal":
		// Pass through time range parameters
		if startTime, ok := variables["startTime"]; ok {
			payload["startTime"] = startTime
		}
		if endTime, ok := variables["endTime"]; ok {
			payload["endTime"] = endTime
		}
		if limit, ok := variables["limit"]; ok {
			payload["limit"] = limit
		}

	case "graph.query.semantic":
		// Pass through semantic search parameters
		if query, ok := variables["query"]; ok {
			payload["query"] = query
		}
		if limit, ok := variables["limit"]; ok {
			payload["limit"] = limit
		}

	case "graph.query.similar":
		// Pass through similar entity search parameters
		if entityID, ok := variables["entityId"]; ok {
			payload["entity_id"] = entityID
		}
		if entityIDVal, ok := variables["entity_id"]; ok {
			payload["entity_id"] = entityIDVal
		}
		if limit, ok := variables["limit"]; ok {
			payload["limit"] = limit
		}

	case "graph.query.localSearch":
		// GraphRAG local search - transform camelCase to snake_case
		if entityID, ok := variables["entityId"]; ok {
			payload["entity_id"] = entityID
		}
		if query, ok := variables["query"]; ok {
			payload["query"] = query
		}
		if level, ok := variables["level"]; ok {
			payload["level"] = level
		}

	case "graph.query.globalSearch":
		// GraphRAG global search - transform camelCase to snake_case
		if query, ok := variables["query"]; ok {
			payload["query"] = query
		}
		if level, ok := variables["level"]; ok {
			payload["level"] = level
		}
		if maxCommunities, ok := variables["maxCommunities"]; ok {
			payload["max_communities"] = maxCommunities
		}

	default:
		// For unknown subjects, pass through as-is
		return variables
	}

	return payload
}

// handleGraphQL handles GraphQL requests
func (c *Component) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	// Report serving stage (throttled)
	c.reportServing(r.Context())

	// Check HTTP method - only POST allowed
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		response := map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "method not allowed"},
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Use request context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), c.config.QueryTimeout)
	defer cancel()

	// Parse GraphQL request
	var gqlReq struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&gqlReq); err != nil {
		atomic.AddInt64(&c.errors, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		response := map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "invalid request"},
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Validate query field
	if gqlReq.Query == "" {
		atomic.AddInt64(&c.errors, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		response := map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "invalid request"},
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Map to NATS subject
	subject := c.mapGraphQLQueryToNATSSubject(gqlReq.Query)

	// Transform variables to NATS payload format
	payload := c.transformVariablesToNATSPayload(gqlReq.Variables, subject)
	payloadBytes, _ := json.Marshal(payload)
	resp, err := c.natsRequester.Request(ctx, subject, payloadBytes, c.config.QueryTimeout)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		w.Header().Set("Content-Type", "application/json")

		// Check if error is due to timeout or context cancellation
		if err == context.DeadlineExceeded || ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			w.WriteHeader(http.StatusGatewayTimeout)
			response := map[string]interface{}{
				"errors": []map[string]interface{}{
					{"message": "request timeout"},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Other errors (e.g., component unavailable)
		w.WriteHeader(http.StatusInternalServerError)
		response := map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "query failed"},
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check if response is a plain-text error from NATS handler (format: "error: <message>")
	respStr := string(resp)
	if strings.HasPrefix(respStr, "error:") {
		atomic.AddInt64(&c.errors, 1)
		errorMsg := strings.TrimPrefix(respStr, "error: ")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // GraphQL convention: 200 with errors
		response := map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": strings.TrimSpace(errorMsg)},
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check if response contains GraphQL errors
	var respData map[string]interface{}
	if err := json.Unmarshal(resp, &respData); err == nil {
		if errors, ok := respData["errors"]; ok && errors != nil {
			// Response contains GraphQL errors - return 200 with errors (GraphQL convention)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(respData)
			return
		}
	}

	// Success - wrap in GraphQL format with field name from subject
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Wrap response with GraphQL field name based on subject
	fieldName := c.subjectToGraphQLField(subject)
	var dataPayload interface{}
	if fieldName != "" {
		dataPayload = map[string]json.RawMessage{
			fieldName: resp,
		}
	} else {
		dataPayload = json.RawMessage(resp)
	}
	response := map[string]interface{}{
		"data": dataPayload,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("failed to encode response", slog.Any("error", err))
	}
}

// handleMCP handles MCP requests
func (c *Component) handleMCP(w http.ResponseWriter, r *http.Request) {
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	// Report serving stage (throttled)
	c.reportServing(r.Context())

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
func (c *Component) handlePlayground(w http.ResponseWriter, r *http.Request) {
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	// Report serving stage (throttled)
	c.reportServing(r.Context())

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

// reportServing reports the serving stage (throttled to avoid KV spam)
func (c *Component) reportServing(ctx context.Context) {
	if c.lifecycleReporter != nil {
		if err := c.lifecycleReporter.ReportStage(ctx, "serving"); err != nil {
			c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "serving"), slog.Any("error", err))
		}
	}
}
