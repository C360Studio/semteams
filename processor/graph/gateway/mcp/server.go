package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/c360/semstreams/pkg/errs"
	gql "github.com/c360/semstreams/processor/graph/gateway/graphql"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/time/rate"
)

// maxResponseSize limits GraphQL response size to prevent memory exhaustion.
const maxResponseSize = 10 * 1024 * 1024 // 10MB

// MetricsRecorder records request metrics for performance monitoring.
type MetricsRecorder interface {
	RecordRequest(ctx context.Context, success bool, duration time.Duration)
}

// Server manages the MCP server with SSE transport.
// This server is designed to be started as an output port of the graph processor,
// with direct in-process access to the GraphQL executor (which uses QueryManager).
type Server struct {
	config      Config
	executor    *gql.Executor
	logger      *slog.Logger
	mcpServer   *server.MCPServer
	httpServer  *http.Server
	mux         *http.ServeMux
	rateLimiter *rate.Limiter
	metrics     MetricsRecorder

	running  bool
	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once
}

// NewServer creates a new MCP server.
// The executor must have a valid Resolver with QueryManager for query execution.
func NewServer(config Config, executor *gql.Executor, metrics MetricsRecorder, logger *slog.Logger) (*Server, error) {
	if executor == nil {
		return nil, errs.WrapFatal(fmt.Errorf("executor is nil"), "Server", "NewServer",
			"executor is required")
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Rate limiter: 10 requests/second with burst of 20
	rateLimiter := rate.NewLimiter(rate.Limit(10), 20)

	return &Server{
		config:      config,
		executor:    executor,
		logger:      logger,
		mux:         http.NewServeMux(),
		rateLimiter: rateLimiter,
		metrics:     metrics,
		stopChan:    make(chan struct{}),
	}, nil
}

// Setup configures the MCP server and HTTP routes.
func (s *Server) Setup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create MCP server with server info
	mcpServer := server.NewMCPServer(
		s.config.ServerName,
		s.config.ServerVersion,
		server.WithToolCapabilities(true),
	)

	// Register the single "graphql" tool
	graphqlTool := mcp.NewTool("graphql",
		mcp.WithDescription(`Execute GraphQL queries against the SemStreams semantic graph.

Supports:
- Entity queries (by ID, alias, type, batch)
- Relationship traversal (outgoing, incoming, both)
- Semantic search with similarity scoring
- Community operations (local/global search, hierarchy)

Use introspection to discover the full schema:
  { __schema { types { name fields { name } } } }

Example queries:
  { entity(id: "robot-1") { id type properties } }
  { semanticSearch(query: "navigation capable", limit: 5) { entity { id } score } }
  { relationships(entityId: "doc-1", direction: OUTGOING) { toEntityId edgeType } }`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("GraphQL query string"),
		),
		mcp.WithObject("variables",
			mcp.Description("Optional GraphQL variables as JSON object"),
		),
	)

	// Add tool handler
	mcpServer.AddTool(graphqlTool, s.handleGraphQLTool)

	s.mcpServer = mcpServer

	// Setup HTTP routes
	s.mux.HandleFunc("/health", s.handleHealth)

	// SSE endpoint for MCP
	// Note: SSE connections are long-lived, so we don't set per-connection timeouts.
	// Tool execution timeouts are handled in handleGraphQLTool.
	sseHandler := server.NewSSEServer(mcpServer)
	s.mux.Handle(s.config.Path, sseHandler)
	s.mux.Handle(s.config.Path+"/", sseHandler)

	// Schema endpoint (for documentation)
	s.mux.HandleFunc("/schema", s.handleSchema)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.BindAddress,
		Handler:      s.mux,
		ReadTimeout:  s.config.Timeout(),
		WriteTimeout: s.config.Timeout() + 5*time.Second, // Extra time for SSE
		IdleTimeout:  120 * time.Second,                  // Long idle for SSE
	}

	s.logger.Info("MCP gateway configured",
		"address", s.config.BindAddress,
		"path", s.config.Path,
		"timeout", s.config.Timeout())

	return nil
}

// Start starts the HTTP server.
func (s *Server) Start(ctx context.Context, ready chan<- struct{}) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errs.WrapFatal(errs.ErrAlreadyStarted, "Server", "Start", "server already running")
	}
	s.running = true
	httpServer := s.httpServer
	s.mu.Unlock()

	errChan := make(chan error, 1)
	go func() {
		defer close(errChan)
		s.logger.Info("MCP gateway starting", "address", s.config.BindAddress)

		if ready != nil {
			close(ready)
		}

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("MCP HTTP server error", "error", err)
			select {
			case errChan <- err:
			case <-ctx.Done():
			case <-s.stopChan:
			}
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("MCP gateway context cancelled, shutting down")
		return s.Stop(30 * time.Second)

	case <-s.stopChan:
		s.logger.Info("MCP gateway stop requested")
		return nil

	case err := <-errChan:
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return errs.WrapFatal(err, "Server", "Start", "HTTP server failed")
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(timeout time.Duration) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	httpServer := s.httpServer
	s.mu.Unlock()

	s.logger.Info("MCP gateway stopping")

	s.stopOnce.Do(func() {
		close(s.stopChan)
	})

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Failed to shutdown MCP gateway gracefully", "error", err)
		return errs.WrapTransient(err, "Server", "Stop", "graceful shutdown failed")
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	s.logger.Info("MCP gateway stopped")
	return nil
}

// handleGraphQLTool handles the "graphql" MCP tool calls.
func (s *Server) handleGraphQLTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	start := time.Now()
	var success bool
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordRequest(ctx, success, time.Since(start))
		}
	}()

	// Rate limiting check
	if !s.rateLimiter.Allow() {
		s.logger.Warn("Rate limit exceeded")
		return mcp.NewToolResultError("rate limit exceeded, please try again later"), nil
	}

	// Get arguments as map
	args := request.GetArguments()
	s.logger.Debug("GraphQL tool called", "arguments", args)

	// Extract query argument
	queryRaw, ok := args["query"]
	if !ok {
		return mcp.NewToolResultError("query argument is required"), nil
	}

	query, ok := queryRaw.(string)
	if !ok {
		return mcp.NewToolResultError("query must be a string"), nil
	}

	// Extract optional variables
	var variables map[string]any
	if varsRaw, ok := args["variables"]; ok {
		switch v := varsRaw.(type) {
		case map[string]any:
			variables = v
		case string:
			// Try to parse as JSON
			if err := json.Unmarshal([]byte(v), &variables); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to parse variables JSON: %v", err)), nil
			}
		}
	}

	// Execute the GraphQL query with timeout enforcement
	queryCtx, cancel := context.WithTimeout(ctx, s.config.Timeout())
	defer cancel()

	result, err := s.executor.Execute(queryCtx, query, variables)
	if err != nil {
		s.logger.Warn("GraphQL execution failed", "error", err)
		if errors.Is(err, context.DeadlineExceeded) {
			return mcp.NewToolResultError(fmt.Sprintf("query exceeded timeout of %v", s.config.Timeout())), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("GraphQL error: %v", err)), nil
	}

	// Format result as JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to serialize result: %v", err)), nil
	}

	// Check response size limit
	if len(resultJSON) > maxResponseSize {
		s.logger.Warn("Response too large",
			"size", len(resultJSON),
			"limit", maxResponseSize)
		return mcp.NewToolResultError(fmt.Sprintf(
			"response too large (%d bytes exceeds %d byte limit)",
			len(resultJSON), maxResponseSize)), nil
	}

	success = true

	// Pretty-print for readability
	var prettyJSON []byte
	prettyJSON, err = json.MarshalIndent(result, "", "  ")
	if err != nil {
		// Fall back to compact JSON if pretty-print fails
		return mcp.NewToolResultText(string(resultJSON)), nil
	}

	return mcp.NewToolResultText(string(prettyJSON)), nil
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	response := map[string]any{
		"status":  "healthy",
		"service": "mcp-gateway",
	}

	if !running {
		response["status"] = "unavailable"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSchema returns the GraphQL schema for documentation.
func (s *Server) handleSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(s.executor.GetSchema()))
}

// IsRunning returns whether the server is currently running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
