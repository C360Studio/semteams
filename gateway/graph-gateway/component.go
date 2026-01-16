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
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/inference"
	"github.com/c360/semstreams/graph/query"
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
	Ports              *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	GraphQLPath        string                `json:"graphql_path" schema:"type:string,description:GraphQL endpoint path,category:basic"`
	MCPPath            string                `json:"mcp_path" schema:"type:string,description:MCP endpoint path,category:basic"`
	BindAddress        string                `json:"bind_address" schema:"type:string,description:HTTP server bind address,category:basic"`
	EnablePlayground   bool                  `json:"enable_playground" schema:"type:bool,description:Enable GraphQL playground,category:basic"`
	EnableInferenceAPI bool                  `json:"enable_inference_api" schema:"type:bool,description:Enable inference API for anomaly review,category:basic"`
	QueryTimeout       time.Duration         `json:"query_timeout" schema:"type:duration,description:Query timeout duration,category:basic"`
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

	// Query classification (T0: keywords, T1/T2: embedding similarity)
	classifier *query.ClassifierChain

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
	ctx         context.Context // Stored context from Start() for use in handler registration
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

	// Create query classifier chain (T0: keywords only for now)
	// T1/T2 embedding classifier can be added later when domain examples are loaded
	keywordClassifier := query.NewKeywordClassifier()
	classifier := query.NewClassifierChain(keywordClassifier, nil)

	// Create component
	comp := &Component{
		name:          "graph-gateway",
		config:        config,
		natsClient:    natsClient,
		natsRequester: natsClient, // Assign to interface field for mockability
		logger:        logger,
		classifier:    classifier,
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

	// Create cancellable context and store for handler registration
	ctx, cancel := context.WithCancel(ctx)
	c.ctx = ctx
	c.cancel = cancel

	// Initialize lifecycle reporter (throttled for high-throughput serving)
	if c.natsClient == nil {
		c.lifecycleReporter = component.NewNoOpLifecycleReporter()
	} else {
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

	// Register inference API handlers if enabled
	inferenceEnabled := false
	if c.config.EnableInferenceAPI {
		if err := c.registerInferenceHandlers(prefix, mux); err != nil {
			c.logger.Warn("failed to register inference handlers",
				slog.Any("error", err))
		} else {
			inferenceEnabled = true
		}
	}

	c.logger.Info("HTTP handlers registered",
		slog.String("graphql_path", graphqlPath),
		slog.String("mcp_path", mcpPath),
		slog.Bool("playground_enabled", c.config.EnablePlayground),
		slog.Bool("inference_enabled", inferenceEnabled))
}

// registerInferenceHandlers registers inference API handlers for anomaly review.
// The gateway creates its own read-only connection to ANOMALY_INDEX bucket.
func (c *Component) registerInferenceHandlers(prefix string, mux *http.ServeMux) error {
	// Get JetStream to access the ANOMALY_INDEX bucket
	js, err := c.natsClient.JetStream()
	if err != nil {
		return errs.Wrap(err, "Component", "registerInferenceHandlers", "get JetStream")
	}

	// Get the ANOMALY_INDEX bucket (created by graph-clustering)
	// Use stored context to allow cancellation during shutdown
	anomalyBucket, err := js.KeyValue(c.ctx, graph.BucketAnomalyIndex)
	if err != nil {
		// Bucket may not exist if graph-clustering hasn't started yet
		// This is not a fatal error - just skip inference API
		return errs.Wrap(err, "Component", "registerInferenceHandlers", "get anomaly bucket")
	}

	// Create read-only storage for listing/viewing anomalies
	storage := inference.NewNATSAnomalyStorage(anomalyBucket, c.logger)

	// Create relationship applier for approved anomalies
	applier := inference.NewNATSRelationshipApplier(js, "graph.events.relationship.create", c.logger)

	// Create and register inference HTTP handler
	handler := inference.NewHTTPHandler(storage, applier, c.logger)
	inferencePath := prefix + "/inference"
	if inferencePath[0] != '/' {
		inferencePath = "/" + inferencePath
	}
	// Clean double slashes
	for i := 0; i < len(inferencePath)-1; i++ {
		if inferencePath[i] == '/' && inferencePath[i+1] == '/' {
			inferencePath = inferencePath[:i] + inferencePath[i+1:]
			i--
		}
	}
	handler.RegisterHTTPHandlers(inferencePath, mux)

	c.logger.Info("inference API handlers registered",
		slog.String("inference_path", inferencePath))

	return nil
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

	switch subject {
	case "graph.query.pathSearch":
		return c.transformPathSearchVars(variables)
	case "graph.query.entity":
		return extractVars(variables, "id")
	case "graph.query.relationships":
		return c.transformRelationshipVars(variables)
	case "graph.query.hierarchyStats", "graph.query.prefix":
		return extractVars(variables, "prefix", "limit")
	case "graph.query.spatial":
		return extractVars(variables, "north", "south", "east", "west", "limit")
	case "graph.query.temporal":
		return extractVars(variables, "startTime", "endTime", "limit")
	case "graph.query.semantic":
		return extractVars(variables, "query", "limit")
	case "graph.query.similar":
		return c.transformSimilarVars(variables)
	case "graph.query.localSearch":
		return c.transformLocalSearchVars(variables)
	case "graph.query.globalSearch":
		return c.transformGlobalSearchVars(variables)
	default:
		return variables
	}
}

// extractVars extracts specified keys from variables into a new map.
func extractVars(variables map[string]interface{}, keys ...string) map[string]interface{} {
	payload := make(map[string]interface{})
	for _, key := range keys {
		if val, ok := variables[key]; ok {
			payload[key] = val
		}
	}
	return payload
}

// transformPathSearchVars transforms path search variables.
func (c *Component) transformPathSearchVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	// Handle multiple possible names for start_entity
	for _, key := range []string{"start", "startEntity", "start_entity"} {
		if val, ok := variables[key]; ok {
			payload["start_entity"] = val
		}
	}
	// Handle multiple possible names for max_depth
	for _, key := range []string{"depth", "maxDepth", "max_depth"} {
		if val, ok := variables[key]; ok {
			payload["max_depth"] = val
		}
	}
	// Handle multiple possible names for max_nodes
	for _, key := range []string{"nodes", "maxNodes", "max_nodes"} {
		if val, ok := variables[key]; ok {
			payload["max_nodes"] = val
		}
	}
	return payload
}

// transformRelationshipVars transforms relationship query variables.
func (c *Component) transformRelationshipVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	for _, key := range []string{"entityId", "entity_id"} {
		if val, ok := variables[key]; ok {
			payload["entity_id"] = val
		}
	}
	if direction, ok := variables["direction"].(string); ok {
		payload["direction"] = strings.ToLower(direction)
	}
	return payload
}

// transformSimilarVars transforms similar entity search variables.
func (c *Component) transformSimilarVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	for _, key := range []string{"entityId", "entity_id"} {
		if val, ok := variables[key]; ok {
			payload["entity_id"] = val
		}
	}
	if limit, ok := variables["limit"]; ok {
		payload["limit"] = limit
	}
	return payload
}

// transformLocalSearchVars transforms GraphRAG local search variables.
func (c *Component) transformLocalSearchVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	if entityID, ok := variables["entityId"]; ok {
		payload["entity_id"] = entityID
	}
	if query, ok := variables["query"]; ok {
		payload["query"] = query
	}
	if level, ok := variables["level"]; ok {
		payload["level"] = level
	}
	return payload
}

// transformGlobalSearchVars transforms GraphRAG global search variables.
func (c *Component) transformGlobalSearchVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	if query, ok := variables["query"]; ok {
		payload["query"] = query
	}
	if level, ok := variables["level"]; ok {
		payload["level"] = level
	}
	if maxCommunities, ok := variables["maxCommunities"]; ok {
		payload["max_communities"] = maxCommunities
	}
	return payload
}

// mergeClassificationOptions merges query classification results into the NATS payload.
// This allows the backend to receive extracted temporal, spatial, and other hints
// from natural language queries.
func (c *Component) mergeClassificationOptions(payload map[string]interface{}, result *query.ClassificationResult) {
	if result == nil || result.Options == nil {
		return
	}

	// Add classification metadata
	payload["classification_tier"] = result.Tier
	payload["classification_confidence"] = result.Confidence
	if result.Intent != "" {
		payload["classification_intent"] = result.Intent
	}

	// Merge extracted options (temporal, spatial, similarity hints)
	for key, value := range result.Options {
		// Don't overwrite existing payload values
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
}

// writeGraphQLError writes a GraphQL error response with the given status code and message.
func (c *Component) writeGraphQLError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]interface{}{
		"errors": []map[string]interface{}{
			{"message": message},
		},
	}
	json.NewEncoder(w).Encode(response)
}

// writeGraphQLSuccess writes a successful GraphQL response wrapping data with the field name.
func (c *Component) writeGraphQLSuccess(w http.ResponseWriter, subject string, resp []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fieldName := c.subjectToGraphQLField(subject)
	var dataPayload interface{}
	if fieldName != "" {
		dataPayload = map[string]json.RawMessage{fieldName: resp}
	} else {
		dataPayload = json.RawMessage(resp)
	}

	response := map[string]interface{}{"data": dataPayload}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("failed to encode response", slog.Any("error", err))
	}
}

// handleNATSResponse processes the NATS response and writes appropriate GraphQL response.
func (c *Component) handleNATSResponse(w http.ResponseWriter, subject string, resp []byte) {
	// Check if response is a plain-text error from NATS handler (format: "error: <message>")
	respStr := string(resp)
	if strings.HasPrefix(respStr, "error:") {
		atomic.AddInt64(&c.errors, 1)
		errorMsg := strings.TrimSpace(strings.TrimPrefix(respStr, "error: "))
		c.writeGraphQLError(w, http.StatusOK, errorMsg) // GraphQL convention: 200 with errors
		return
	}

	// Check if response contains GraphQL errors
	var respData map[string]interface{}
	if err := json.Unmarshal(resp, &respData); err == nil {
		if errors, ok := respData["errors"]; ok && errors != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(respData)
			return
		}
	}

	c.writeGraphQLSuccess(w, subject, resp)
}

// handleGraphQL handles GraphQL requests
func (c *Component) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())
	c.reportServing(r.Context())

	if r.Method != http.MethodPost {
		c.writeGraphQLError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), c.config.QueryTimeout)
	defer cancel()

	var gqlReq struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&gqlReq); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.writeGraphQLError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if gqlReq.Query == "" {
		atomic.AddInt64(&c.errors, 1)
		c.writeGraphQLError(w, http.StatusBadRequest, "invalid request")
		return
	}

	subject := c.mapGraphQLQueryToNATSSubject(gqlReq.Query)
	payload := c.transformVariablesToNATSPayload(gqlReq.Variables, subject)

	// For search queries, classify the query text and merge extracted options
	if c.classifier != nil && (subject == "graph.query.globalSearch" || subject == "graph.query.semantic") {
		if queryText, ok := payload["query"].(string); ok && queryText != "" {
			result := c.classifier.ClassifyQuery(ctx, queryText)
			if result != nil {
				// Merge classification options into payload
				c.mergeClassificationOptions(payload, result)
			}
		}
	}

	payloadBytes, _ := json.Marshal(payload)

	resp, err := c.natsRequester.Request(ctx, subject, payloadBytes, c.config.QueryTimeout)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		if err == context.DeadlineExceeded || ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			c.writeGraphQLError(w, http.StatusGatewayTimeout, "request timeout")
			return
		}
		c.writeGraphQLError(w, http.StatusInternalServerError, "query failed")
		return
	}

	c.handleNATSResponse(w, subject, resp)
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
