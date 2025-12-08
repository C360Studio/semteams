package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/gateway"
	gql "github.com/c360/semstreams/gateway/graphql"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/processor/graph/querymanager"
)

// mcpGatewaySchema defines the configuration schema
var mcpGatewaySchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// MetricsRecorder records request metrics for performance monitoring.
type MetricsRecorder interface {
	RecordRequest(ctx context.Context, success bool, duration time.Duration)
}

// Gateway implements the MCP gateway component.
// It exposes a single "graphql" MCP tool for AI agent integration,
// executing queries in-process against the BaseResolver.
type Gateway struct {
	name       string
	config     Config
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Components
	resolver *gql.BaseResolver // Reuses GraphQL gateway's resolver
	executor *Executor         // In-process GraphQL execution
	server   *Server           // MCP server with SSE transport

	// Lifecycle state
	running atomic.Bool

	// Protects metrics and startTime
	mu        sync.RWMutex
	startTime time.Time
	shutdown  chan struct{}
	done      chan struct{}

	// Metrics
	requestsTotal   atomic.Uint64
	requestsSuccess atomic.Uint64
	requestsFailed  atomic.Uint64
	lastActivity    atomic.Value // stores time.Time
}

// NewMCPGateway creates a new MCP gateway from configuration.
func NewMCPGateway(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var config Config
	if err := component.SafeUnmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "MCPGateway", "NewMCPGateway", "config unmarshal")
	}

	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "MCPGateway", "NewMCPGateway", "config validation")
	}

	logger := deps.GetLogger().With("component", "mcp-gateway")

	// Create base resolver with QueryManager backend (preferred) or NATS fallback
	var resolver *gql.BaseResolver
	if deps.QueryManager != nil {
		if qm, ok := deps.QueryManager.(querymanager.Querier); ok {
			resolver = gql.NewBaseResolver(qm, nil)
			logger.Info("MCP gateway using QueryManager backend")
		} else {
			logger.Warn("QueryManager type assertion failed, MCP gateway may have limited functionality")
		}
	}

	// If QueryManager not available, try NATS fallback (limited functionality)
	if resolver == nil {
		if deps.NATSClient != nil {
			// Create NATS client wrapper for fallback
			// Note: Some operations (LocalSearch, GlobalSearch) require QueryManager
			natsSubjects := gql.NATSSubjectsConfig{
				EntityQuery:       "graph.query.entity",
				EntitiesQuery:     "graph.query.entities",
				TypeQuery:         "graph.query.type",
				RelationshipQuery: "graph.query.relationships",
				SemanticSearch:    "graph.search.semantic",
			}
			natsWrapper := gql.NewNATSClient(deps.NATSClient, natsSubjects, config.Timeout())
			resolver = gql.NewBaseResolverWithNATS(natsWrapper, nil)
			logger.Info("MCP gateway using NATS backend (limited functionality)")
		} else {
			return nil, errs.WrapFatal(errs.ErrMissingConfig, "MCPGateway", "NewMCPGateway",
				"QueryManager or NATS client required")
		}
	}

	// Create in-process GraphQL executor
	executor, err := NewExecutor(resolver, logger)
	if err != nil {
		return nil, errs.WrapFatal(err, "MCPGateway", "NewMCPGateway", "create executor")
	}

	// Create gateway first so it can be passed as metrics recorder to server
	gw := &Gateway{
		name:       "mcp-gateway",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     logger,
		resolver:   resolver,
		executor:   executor,
		shutdown:   make(chan struct{}),
		done:       make(chan struct{}),
	}

	// Create MCP server with gateway as metrics recorder
	server, err := NewServer(config, executor, gw, logger)
	if err != nil {
		return nil, errs.WrapFatal(err, "MCPGateway", "NewMCPGateway", "create server")
	}
	gw.server = server

	return gw, nil
}

// Initialize prepares the MCP gateway.
func (g *Gateway) Initialize() error {
	g.logger.Info("Initializing MCP gateway")

	if err := g.server.Setup(); err != nil {
		return errs.WrapFatal(err, "MCPGateway", "Initialize", "server setup")
	}

	g.logger.Info("MCP gateway initialized",
		"address", g.config.BindAddress,
		"path", g.config.Path)

	return nil
}

// Start begins the MCP gateway operation.
func (g *Gateway) Start(ctx context.Context) error {
	if g.running.Load() {
		return errs.WrapFatal(errs.ErrAlreadyStarted, "MCPGateway", "Start",
			"gateway already running")
	}

	g.mu.Lock()
	g.running.Store(true)
	g.startTime = time.Now()
	g.mu.Unlock()

	g.logger.Info("MCP gateway starting")

	ready := make(chan struct{})
	errChan := make(chan error, 1)

	go func() {
		if err := g.server.Start(ctx, ready); err != nil {
			select {
			case errChan <- err:
			default:
			}
			g.mu.Lock()
			g.running.Store(false)
			g.mu.Unlock()
		}
	}()

	select {
	case <-ready:
		g.logger.Info("MCP gateway started successfully",
			"address", g.config.BindAddress,
			"path", g.config.Path)
	case err := <-errChan:
		return err
	case <-time.After(5 * time.Second):
		return errs.WrapFatal(errs.ErrConnectionTimeout, "MCPGateway", "Start",
			"server failed to start within timeout")
	}

	<-ctx.Done()
	g.logger.Info("MCP gateway context cancelled")

	return g.Stop(30 * time.Second)
}

// Stop gracefully stops the MCP gateway.
func (g *Gateway) Stop(timeout time.Duration) error {
	if !g.running.Load() {
		return nil
	}

	g.logger.Info("MCP gateway stopping")

	close(g.shutdown)

	if err := g.server.Stop(timeout); err != nil {
		g.logger.Error("Failed to stop server", "error", err)
		return err
	}

	g.mu.Lock()
	g.running.Store(false)
	close(g.done)
	g.mu.Unlock()

	g.logger.Info("MCP gateway stopped")

	return nil
}

// RegisterHTTPHandlers registers gateway routes with an external HTTP mux.
// The MCP gateway runs its own HTTP server, so this is a no-op.
func (g *Gateway) RegisterHTTPHandlers(prefix string, _ *http.ServeMux) {
	g.logger.Info("RegisterHTTPHandlers called",
		"prefix", prefix,
		"note", "MCP gateway runs its own HTTP server")
}

// Component metadata implementation

// Meta returns component metadata.
func (g *Gateway) Meta() component.Metadata {
	return component.Metadata{
		Name:        g.name,
		Type:        "gateway",
		Description: "MCP gateway for AI agent integration via GraphQL",
		Version:     "0.1.0",
	}
}

// InputPorts returns no input ports (gateway is request-driven).
func (g *Gateway) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts returns no output ports (gateway uses request/reply).
func (g *Gateway) OutputPorts() []component.Port {
	return []component.Port{}
}

// ConfigSchema returns the configuration schema.
func (g *Gateway) ConfigSchema() component.ConfigSchema {
	return mcpGatewaySchema
}

// Health returns the current health status.
func (g *Gateway) Health() component.HealthStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	healthy := g.running.Load() && g.server.IsRunning()

	return component.HealthStatus{
		Healthy:    healthy,
		LastCheck:  time.Now(),
		ErrorCount: int(g.requestsFailed.Load()),
		Uptime:     time.Since(g.startTime),
	}
}

// DataFlow returns current data flow metrics.
func (g *Gateway) DataFlow() component.FlowMetrics {
	total := g.requestsTotal.Load()
	failed := g.requestsFailed.Load()

	var errorRate float64
	if total > 0 {
		errorRate = float64(failed) / float64(total)
	}

	// Load lastActivity atomically
	var lastAct time.Time
	if v := g.lastActivity.Load(); v != nil {
		lastAct = v.(time.Time)
	}

	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         errorRate,
		LastActivity:      lastAct,
	}
}

// RecordRequest records request metrics including latency.
// This is used by the server to track query performance for resource tuning.
func (g *Gateway) RecordRequest(ctx context.Context, success bool, duration time.Duration) {
	g.requestsTotal.Add(1)
	if success {
		g.requestsSuccess.Add(1)
	} else {
		g.requestsFailed.Add(1)
	}
	g.lastActivity.Store(time.Now())

	// Use context-aware logging to propagate trace IDs and request metadata
	g.logger.DebugContext(ctx, "GraphQL query completed",
		"success", success,
		"duration_ms", duration.Milliseconds())
}

// Ensure Gateway implements required interfaces
var (
	_ component.Discoverable = (*Gateway)(nil)
	_ gateway.Gateway        = (*Gateway)(nil)
)
