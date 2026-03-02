// Package graphgateway provides the graph-gateway component for exposing graph operations via HTTP.
package graphgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/gateway"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/inference"
	"github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
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
	// Validate context
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Component", "Start", "context already cancelled")
	}

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
		Addr:         c.config.BindAddress,
		Handler:      c.httpMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
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

	// Agentic query patterns
	if strings.Contains(query, "trajectory") {
		return "agentic.query.trajectory"
	}

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

	// Predicate queries - must come before generic "entity" check
	if strings.Contains(query, "compoundpredicatequery") || strings.Contains(query, "compoundpredicate") {
		return "graph.index.query.predicateCompound"
	}
	if strings.Contains(query, "predicatestats") {
		return "graph.index.query.predicateStats"
	}
	if strings.Contains(query, "predicates") && !strings.Contains(query, "entitiesbypredicate") {
		return "graph.index.query.predicateList"
	}
	if strings.Contains(query, "entitiesbypredicate") {
		return "graph.index.query.predicate"
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
	case "graph.index.query.predicate":
		return "entitiesByPredicate"
	case "graph.index.query.predicateList":
		return "predicates"
	case "graph.index.query.predicateStats":
		return "predicateStats"
	case "graph.index.query.predicateCompound":
		return "compoundPredicateQuery"
	case "agentic.query.trajectory":
		return "trajectory"
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
	case "graph.index.query.predicate":
		return extractVars(variables, "predicate", "limit")
	case "graph.index.query.predicateList":
		return map[string]interface{}{}
	case "graph.index.query.predicateStats":
		return c.transformPredicateStatsVars(variables)
	case "graph.index.query.predicateCompound":
		return c.transformCompoundPredicateVars(variables)
	case "agentic.query.trajectory":
		return extractVars(variables, "loopId", "limit")
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
	// Handle direction — normalize to lowercase
	if direction, ok := variables["direction"].(string); ok {
		payload["direction"] = strings.ToLower(direction)
	}
	// Handle predicates — pass through as-is (array via JSON variables)
	if predicates, ok := variables["predicates"]; ok {
		payload["predicates"] = predicates
	}
	// Handle timeout — accept camelCase and snake_case
	for _, key := range []string{"timeout", "timeoutDuration"} {
		if val, ok := variables[key].(string); ok {
			payload["timeout"] = val
			break
		}
	}
	// Handle maxPaths — accept camelCase and snake_case
	for _, key := range []string{"maxPaths", "max_paths"} {
		if val, ok := variables[key]; ok {
			payload["max_paths"] = val
			break
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

// transformPredicateStatsVars transforms predicate stats query variables.
func (c *Component) transformPredicateStatsVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	if predicate, ok := variables["predicate"]; ok {
		payload["predicate"] = predicate
	}
	// Handle multiple possible names for sample_limit
	for _, key := range []string{"sampleLimit", "sample_limit"} {
		if val, ok := variables[key]; ok {
			payload["sample_limit"] = val
		}
	}
	return payload
}

// transformCompoundPredicateVars transforms compound predicate query variables.
func (c *Component) transformCompoundPredicateVars(variables map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	if predicates, ok := variables["predicates"]; ok {
		payload["predicates"] = predicates
	}
	if operator, ok := variables["operator"]; ok {
		payload["operator"] = operator
	}
	if limit, ok := variables["limit"]; ok {
		payload["limit"] = limit
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

// extractInlineArguments parses inline arguments from a GraphQL query string.
// For example, `entity(id: "test")` returns {"id": "test"}.
// Variable references ($varName) are skipped since they come from the variables field.
// If the query has a variable declaration block (e.g., `query Q($id: String!)`),
// it is skipped and the field arguments are parsed instead.
func extractInlineArguments(query string) map[string]interface{} {
	result := make(map[string]interface{})

	// Find the first argument block
	openIdx := strings.IndexByte(query, '(')
	if openIdx < 0 {
		return result
	}

	closeIdx := findMatchingParen(query, openIdx)
	if closeIdx < 0 {
		return result
	}

	argStr := query[openIdx+1 : closeIdx]

	// Check if this is a variable declaration block (starts with $)
	trimmed := strings.TrimLeft(argStr, " \t\n")
	if len(trimmed) > 0 && trimmed[0] == '$' {
		// This is a variable declaration block — skip it and find the next paren block
		search := query[closeIdx+1:]
		nextOpen := strings.IndexByte(search, '(')
		if nextOpen < 0 {
			return result
		}
		absOpen := closeIdx + 1 + nextOpen
		nextClose := findMatchingParen(query, absOpen)
		if nextClose < 0 {
			return result
		}
		argStr = query[absOpen+1 : nextClose]
	}

	parseInlineArgs(argStr, result)
	return result
}

// findMatchingParen finds the closing paren matching the open paren at openIdx.
func findMatchingParen(s string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseInlineArgs parses key: value pairs from an argument string.
func parseInlineArgs(argStr string, result map[string]interface{}) {
	i := 0
	for i < len(argStr) {
		// Skip whitespace and commas
		for i < len(argStr) && (argStr[i] == ' ' || argStr[i] == '\t' || argStr[i] == '\n' || argStr[i] == ',') {
			i++
		}
		if i >= len(argStr) {
			break
		}

		// Read key
		keyStart := i
		for i < len(argStr) && argStr[i] != ':' && argStr[i] != ' ' && argStr[i] != '\t' {
			i++
		}
		key := strings.TrimSpace(argStr[keyStart:i])
		if key == "" {
			break
		}

		// Skip to colon
		for i < len(argStr) && argStr[i] != ':' {
			i++
		}
		if i >= len(argStr) {
			break
		}
		i++ // skip ':'

		// Skip whitespace after colon
		for i < len(argStr) && (argStr[i] == ' ' || argStr[i] == '\t') {
			i++
		}
		if i >= len(argStr) {
			break
		}

		// Parse value and advance position
		val, newPos := parseInlineValue(argStr, i)
		i = newPos
		if val != nil {
			result[key] = val
		}
	}
}

// parseInlineValue parses a single value starting at position i in argStr.
// Returns the parsed value (or nil for variable references) and the new position.
func parseInlineValue(argStr string, i int) (interface{}, int) {
	switch {
	case argStr[i] == '"':
		return parseStringValue(argStr, i)

	case argStr[i] == '$':
		// Variable reference - skip, these come from the variables field
		for i < len(argStr) && argStr[i] != ',' && argStr[i] != ')' && argStr[i] != ' ' {
			i++
		}
		return nil, i

	default:
		return parseLiteralValue(argStr, i)
	}
}

// parseStringValue parses a quoted string value starting at position i (the opening quote).
func parseStringValue(argStr string, i int) (interface{}, int) {
	i++ // skip opening quote
	var sb strings.Builder
	for i < len(argStr) {
		if argStr[i] == '\\' && i+1 < len(argStr) {
			sb.WriteByte(argStr[i+1])
			i += 2
			continue
		}
		if argStr[i] == '"' {
			i++ // skip closing quote
			break
		}
		sb.WriteByte(argStr[i])
		i++
	}
	return sb.String(), i
}

// parseLiteralValue parses a boolean, numeric, null, or enum value starting at position i.
func parseLiteralValue(argStr string, i int) (interface{}, int) {
	valStart := i
	for i < len(argStr) && argStr[i] != ',' && argStr[i] != ')' && argStr[i] != ' ' {
		i++
	}
	val := strings.TrimSpace(argStr[valStart:i])
	if val == "" {
		return nil, i
	}
	switch val {
	case "true":
		return true, i
	case "false":
		return false, i
	case "null":
		return nil, i
	}
	// Try integer
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n, i
	}
	// Try float
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f, i
	}
	// Treat as enum/identifier value (e.g., OUTGOING, ASC)
	return val, i
}

// mergeVariables merges inline arguments with explicit variables.
// Explicit variables take precedence over inline arguments.
func mergeVariables(inline, explicit map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(inline)+len(explicit))
	for k, v := range inline {
		merged[k] = v
	}
	for k, v := range explicit {
		merged[k] = v
	}
	return merged
}

// isPubAckResponse detects JetStream PubAck responses that indicate
// a stream/subject overlap configuration issue. PubAck responses have
// the shape: {"stream":"NAME","seq":N} with optional "domain" and "duplicate" fields.
func isPubAckResponse(data []byte) bool {
	// PubAck responses are always small; real query responses are larger
	if len(data) > 256 {
		return false
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return false
	}

	// Must have "stream" (string) and "seq" (number)
	stream, hasStream := obj["stream"]
	seq, hasSeq := obj["seq"]
	if !hasStream || !hasSeq {
		return false
	}
	if _, ok := stream.(string); !ok {
		return false
	}
	if _, ok := seq.(float64); !ok {
		return false
	}

	// Only allow known PubAck fields
	for key := range obj {
		switch key {
		case "stream", "seq", "domain", "duplicate":
			// known PubAck fields
		default:
			return false
		}
	}

	return true
}

// isIntrospectionQuery checks if a GraphQL query's first field is an introspection field.
// It looks for __schema or __type as the first field selector in the selection set,
// avoiding false positives from entity IDs or comments containing these strings.
func isIntrospectionQuery(query string) bool {
	q := strings.TrimSpace(query)

	// Strip operation keyword and name: "query MyQuery" or "query"
	if strings.HasPrefix(q, "query") || strings.HasPrefix(q, "mutation") {
		if braceIdx := strings.IndexByte(q, '{'); braceIdx >= 0 {
			q = q[braceIdx:]
		}
	}

	// Strip opening brace and whitespace to get the first field selector
	q = strings.TrimLeft(q, "{ \t\n\r")
	return strings.HasPrefix(q, "__schema") || strings.HasPrefix(q, "__type")
}

// handleIntrospection returns a hardcoded schema for GraphQL introspection queries.
// Handles both __schema and __type queries.
func (c *Component) handleIntrospection(w http.ResponseWriter, queryStr string) {
	schema := buildIntrospectionSchema()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var data map[string]interface{}
	if strings.Contains(queryStr, "__type") && !strings.Contains(queryStr, "__schema") {
		// __type query - extract requested type name and return matching type
		data = map[string]interface{}{"__type": findTypeByName(schema, queryStr)}
	} else {
		data = map[string]interface{}{"__schema": schema}
	}

	response := map[string]interface{}{"data": data}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("failed to encode introspection response", slog.Any("error", err))
	}
}

// findTypeByName extracts the type name from a __type query and returns the matching type.
func findTypeByName(schema map[string]interface{}, queryStr string) interface{} {
	// Extract type name from __type(name: "TypeName")
	args := extractInlineArguments(queryStr)
	name, _ := args["name"].(string)
	if name == "" {
		return nil
	}

	types, _ := schema["types"].([]map[string]interface{})
	for _, t := range types {
		if t["name"] == name {
			return t
		}
	}
	return nil
}

// buildIntrospectionSchema builds a minimal introspection schema describing
// the supported query fields and their argument/return types.
func buildIntrospectionSchema() map[string]interface{} {
	return map[string]interface{}{
		"queryType":        map[string]interface{}{"name": "Query"},
		"mutationType":     nil,
		"subscriptionType": nil,
		"types": []map[string]interface{}{
			{
				"kind": "OBJECT",
				"name": "Query",
				"fields": []map[string]interface{}{
					fieldDef("entity", "Entity", argDef("id", "String!")),
					fieldDef("entitiesByPrefix", "[Entity]", argDef("prefix", "String!"), argDef("limit", "Int")),
					fieldDef("entityByAlias", "Entity", argDef("alias", "String!")),
					fieldDef("relationships", "[Relationship]", argDef("entityId", "String!"), argDef("direction", "String")),
					fieldDef("entityIdHierarchy", "HierarchyResult", argDef("prefix", "String!"), argDef("limit", "Int")),
					fieldDef("pathSearch", "PathSearchResult", argDef("startEntity", "String!"), argDef("maxDepth", "Int"), argDef("maxNodes", "Int"),
						argDef("direction", "String"), argDef("predicates", "[String]"),
						argDef("timeout", "String"), argDef("maxPaths", "Int")),
					fieldDef("spatialSearch", "[Entity]", argDef("north", "Float!"), argDef("south", "Float!"), argDef("east", "Float!"), argDef("west", "Float!"), argDef("limit", "Int")),
					fieldDef("temporalSearch", "[Entity]", argDef("startTime", "String!"), argDef("endTime", "String!"), argDef("limit", "Int")),
					fieldDef("semanticSearch", "[Entity]", argDef("query", "String!"), argDef("limit", "Int")),
					fieldDef("findSimilar", "[Entity]", argDef("entityId", "String!"), argDef("limit", "Int")),
					fieldDef("localSearch", "SearchResult", argDef("entityId", "String!"), argDef("query", "String"), argDef("level", "Int")),
					fieldDef("globalSearch", "SearchResult", argDef("query", "String!"), argDef("level", "Int"), argDef("maxCommunities", "Int")),
					fieldDef("capabilities", "Capabilities"),
					// Agentic queries
					fieldDef("trajectory", "Trajectory", argDef("loopId", "String!"), argDef("limit", "Int")),
					// Predicate queries
					fieldDef("entitiesByPredicate", "[String]", argDef("predicate", "String!"), argDef("limit", "Int")),
					fieldDef("predicates", "PredicateListResult"),
					fieldDef("predicateStats", "PredicateStatsResult", argDef("predicate", "String!"), argDef("sampleLimit", "Int")),
					fieldDef("compoundPredicateQuery", "CompoundPredicateResult", argDef("predicates", "[String!]!"), argDef("operator", "String!"), argDef("limit", "Int")),
				},
			},
			typeDef("OBJECT", "Entity", "id", "triples"),
			typeDef("OBJECT", "Triple", "subject", "predicate", "object"),
			typeDef("OBJECT", "Relationship", "from", "to", "predicate"),
			typeDef("OBJECT", "HierarchyResult", "prefix", "children", "count"),
			typeDef("OBJECT", "PathSearchResult", "entities", "edges", "paths"),
			typeDef("OBJECT", "SearchResult", "results", "score"),
			typeDef("OBJECT", "Capabilities", "queries", "mutations"),
			// Predicate types
			typeDef("OBJECT", "PredicateSummary", "predicate", "entityCount"),
			typeDef("OBJECT", "PredicateListResult", "predicates", "total"),
			typeDef("OBJECT", "PredicateStatsResult", "predicate", "entityCount", "sampleEntities"),
			typeDef("OBJECT", "CompoundPredicateResult", "entities", "operator", "matched"),
			// Agentic types
			typeDef("OBJECT", "Trajectory", "loopId", "startTime", "endTime", "steps", "outcome", "totalTokensIn", "totalTokensOut", "duration"),
			typeDef("OBJECT", "TrajectoryStep", "timestamp", "stepType", "requestId", "prompt", "response", "tokensIn", "tokensOut", "toolName", "toolResult", "duration"),
			typeDef("SCALAR", "String"),
			typeDef("SCALAR", "Int"),
			typeDef("SCALAR", "Float"),
			typeDef("SCALAR", "Boolean"),
		},
	}
}

// fieldDef builds a field definition for introspection.
func fieldDef(name, typeName string, args ...map[string]interface{}) map[string]interface{} {
	field := map[string]interface{}{
		"name": name,
		"type": map[string]interface{}{"name": typeName},
		"args": args,
	}
	if len(args) == 0 {
		field["args"] = []map[string]interface{}{}
	}
	return field
}

// argDef builds an argument definition for introspection.
func argDef(name, typeName string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"type": map[string]interface{}{"name": typeName},
	}
}

// typeDef builds a type definition for introspection.
func typeDef(kind, name string, fieldNames ...string) map[string]interface{} {
	td := map[string]interface{}{
		"kind": kind,
		"name": name,
	}
	if len(fieldNames) > 0 {
		fields := make([]map[string]interface{}, len(fieldNames))
		for i, fn := range fieldNames {
			fields[i] = map[string]interface{}{
				"name": fn,
				"type": map[string]interface{}{"name": "String"},
			}
		}
		td["fields"] = fields
	}
	return td
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

	// Detect JetStream PubAck responses (indicates stream/subject overlap)
	if isPubAckResponse(resp) {
		atomic.AddInt64(&c.errors, 1)
		c.writeGraphQLError(w, http.StatusBadGateway,
			"received stream acknowledgment instead of query response")
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

	// Unwrap QueryResponse envelope for graph.index.query.* subjects
	// These handlers return QueryResponse[T] with {data: T, error: string, timestamp: time}
	// We need to extract just the data field for the GraphQL response
	if strings.HasPrefix(subject, "graph.index.query.") {
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error string          `json:"error,omitempty"`
		}
		if err := json.Unmarshal(resp, &envelope); err == nil {
			if envelope.Error != "" {
				c.writeGraphQLError(w, http.StatusOK, envelope.Error)
				return
			}
			if len(envelope.Data) > 0 {
				resp = envelope.Data
			}
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

	// Handle introspection queries locally
	if isIntrospectionQuery(gqlReq.Query) {
		c.handleIntrospection(w, gqlReq.Query)
		return
	}

	subject := c.mapGraphQLQueryToNATSSubject(gqlReq.Query)

	// Reject unrecognized queries immediately instead of dispatching to NATS
	if subject == "graph.query.unknown" {
		atomic.AddInt64(&c.errors, 1)
		c.writeGraphQLError(w, http.StatusBadRequest, "unrecognized query")
		return
	}

	// Extract inline arguments from query string and merge with explicit variables
	inlineArgs := extractInlineArguments(gqlReq.Query)
	mergedVars := mergeVariables(inlineArgs, gqlReq.Variables)

	payload := c.transformVariablesToNATSPayload(mergedVars, subject)

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
