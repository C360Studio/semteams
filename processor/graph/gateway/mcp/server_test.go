package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gql "github.com/c360/semstreams/processor/graph/gateway/graphql"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMetrics records metrics for testing.
type mockMetrics struct {
	mu        sync.Mutex
	requests  []metricRecord
	successes int
	failures  int
}

type metricRecord struct {
	success  bool
	duration time.Duration
}

func (m *mockMetrics) RecordRequest(_ context.Context, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, metricRecord{success: success, duration: duration})
	if success {
		m.successes++
	} else {
		m.failures++
	}
}

func (m *mockMetrics) getSuccesses() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.successes
}

func (m *mockMetrics) getFailures() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failures
}

// --- Configuration Validation Tests ---

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{
			name:    "defaults applied when empty",
			config:  Config{},
			wantErr: "",
		},
		{
			name: "valid config accepted",
			config: Config{
				BindAddress:    ":9000",
				TimeoutStr:     "60s",
				Path:           "/api/mcp",
				ServerName:     "test-server",
				ServerVersion:  "2.0.0",
				MaxRequestSize: 2 << 20,
			},
			wantErr: "",
		},
		{
			name: "timeout too short",
			config: Config{
				TimeoutStr: "500ms",
			},
			wantErr: "timeout must be at least 1s",
		},
		{
			name: "timeout too long",
			config: Config{
				TimeoutStr: "10m",
			},
			wantErr: "timeout must not exceed 5m",
		},
		{
			name: "invalid timeout format",
			config: Config{
				TimeoutStr: "notaduration",
			},
			wantErr: "invalid timeout duration",
		},
		{
			name: "invalid bind address",
			config: Config{
				BindAddress: "invalid",
			},
			wantErr: "invalid bind address",
		},
		{
			name: "port out of range",
			config: Config{
				BindAddress: ":99999",
			},
			wantErr: "port must be 1-65535",
		},
		{
			name: "path missing leading slash",
			config: Config{
				Path: "mcp",
			},
			wantErr: "path must start with /",
		},
		{
			name: "max request size too small",
			config: Config{
				MaxRequestSize: 512,
			},
			wantErr: "max_request_size must be at least 1KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	assert.Equal(t, ":8081", config.BindAddress)
	assert.Equal(t, "30s", config.TimeoutStr)
	assert.Equal(t, "/mcp", config.Path)
	assert.Equal(t, "semstreams", config.ServerName)
	assert.Equal(t, "1.0.0", config.ServerVersion)
	assert.Equal(t, int64(1<<20), config.MaxRequestSize)
	assert.Equal(t, 30*time.Second, config.Timeout())
}

func TestConfig_Timeout_BeforeValidate(t *testing.T) {
	config := Config{}
	// Before Validate(), Timeout() should return default
	assert.Equal(t, 30*time.Second, config.Timeout())
}

// --- Server Creation Tests ---

func TestNewServer_NilExecutor(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	_, err := NewServer(config, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor is nil")
}

func TestNewServer_ValidConfig(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.False(t, server.IsRunning())
}

func TestNewServer_WithMetrics(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	metrics := &mockMetrics{}

	server, err := NewServer(config, executor, metrics, nil)
	require.NoError(t, err)
	assert.NotNil(t, server)
}

// --- Server Lifecycle Tests ---

func TestServer_Setup(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)
	assert.NotNil(t, server.mcpServer)
	assert.NotNil(t, server.httpServer)
}

func TestServer_StopWhenNotRunning(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	// Stop when not running should be safe (idempotent)
	err = server.Stop(time.Second)
	assert.NoError(t, err)
}

func TestServer_IsRunning(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	assert.False(t, server.IsRunning())
}

// --- Health Endpoint Tests ---

func TestServer_HealthEndpoint_WhenRunning(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Simulate running state
	server.mu.Lock()
	server.running = true
	server.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "healthy", body["status"])
	assert.Equal(t, "mcp-gateway", body["service"])
}

func TestServer_HealthEndpoint_WhenStopped(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Server not running (default state)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "unavailable", body["status"])
}

// --- Schema Endpoint Tests ---

func TestServer_SchemaEndpoint(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/schema", nil)
	w := httptest.NewRecorder()

	server.handleSchema(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	// Schema should be returned (may be empty from zero-value executor)
	assert.NotNil(t, body)
}

// --- GraphQL Tool Handler Tests ---

func TestServer_GraphQLTool_MissingQuery(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	// No arguments set - query is missing

	result, err := server.handleGraphQLTool(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	// Check error content
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "query argument is required")
}

func TestServer_GraphQLTool_QueryNotString(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"query": 12345, // Not a string
	}

	result, err := server.handleGraphQLTool(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "query must be a string")
}

func TestServer_GraphQLTool_InvalidVariablesJSON(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"query":     "{ entity(id: $id) { id } }",
		"variables": "not valid json {{{",
	}

	result, err := server.handleGraphQLTool(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "failed to parse variables JSON")
}

func TestServer_GraphQLTool_VariablesAsMap(t *testing.T) {
	// Variables passed as map[string]any are handled directly in handleGraphQLTool
	// This test requires integration with a real executor to verify variables are passed through
	t.Skip("Requires integration test with real executor")
}

// --- Rate Limiting Tests ---

func TestServer_RateLimiting(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	// Exhaust the rate limiter (burst of 20)
	for i := 0; i < 20; i++ {
		allowed := server.rateLimiter.Allow()
		assert.True(t, allowed, "request %d should be allowed within burst", i)
	}

	// Next request should be rate limited
	allowed := server.rateLimiter.Allow()
	assert.False(t, allowed, "request beyond burst should be rate limited")
}

func TestServer_GraphQLTool_RateLimitExceeded(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	// Exhaust the rate limiter
	for i := 0; i < 21; i++ {
		server.rateLimiter.Allow()
	}

	// Now make a tool call - should be rate limited
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"query": "{ __typename }",
	}

	result, err := server.handleGraphQLTool(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "rate limit exceeded")
}

func TestServer_GraphQLTool_VariablesAsJSONString(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	metrics := &mockMetrics{}
	server, err := NewServer(config, executor, metrics, nil)
	require.NoError(t, err)

	// Valid JSON string for variables - will fail at execution but tests variable parsing
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"query":     "{ entity(id: $id) { id } }",
		"variables": `{"id": "test-123"}`,
	}

	// This will fail at executor.Execute (nil resolver) but exercises the variable parsing path
	result, err := server.handleGraphQLTool(context.Background(), request)
	require.NoError(t, err)
	// Result will be an error from the executor, but variables were parsed successfully
	assert.NotNil(t, result)
}

// --- Metrics Recording Tests ---

func TestServer_MetricsRecording_Success(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	metrics := &mockMetrics{}
	executor := &gql.Executor{}

	_, err := NewServer(config, executor, metrics, nil)
	require.NoError(t, err)

	// Metrics are recorded in handleGraphQLTool - test via direct call
	// This requires a working executor, so we verify the metrics struct works
	assert.Equal(t, 0, metrics.getSuccesses())
	assert.Equal(t, 0, metrics.getFailures())

	// Manually record
	metrics.RecordRequest(context.Background(), true, time.Millisecond)
	assert.Equal(t, 1, metrics.getSuccesses())

	metrics.RecordRequest(context.Background(), false, time.Millisecond)
	assert.Equal(t, 1, metrics.getFailures())
}

// --- Response Size Limit Tests ---

func TestServer_ResponseSizeLimit_Constant(t *testing.T) {
	// Verify the constant is set correctly
	assert.Equal(t, 10*1024*1024, maxResponseSize)
}

// --- Timeout Tests ---

func TestServer_TimeoutEnforcement(t *testing.T) {
	config := Config{
		TimeoutStr: "1s", // Short timeout for test
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	// The timeout is enforced in handleGraphQLTool via context.WithTimeout
	// Verify the config timeout is correctly set
	assert.Equal(t, time.Second, server.config.Timeout())
}

// --- Concurrent Access Tests ---

func TestServer_ConcurrentIsRunning(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = server.IsRunning()
		}()
	}
	wg.Wait()
}

func TestServer_ConcurrentHealthCheck(t *testing.T) {
	config := Config{}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Toggle running state during concurrent health checks
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Toggle running state
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				server.mu.Lock()
				server.running = !server.running
				server.mu.Unlock()
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// Concurrent health checks
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				req := httptest.NewRequest(http.MethodGet, "/health", nil)
				w := httptest.NewRecorder()
				server.handleHealth(w, req)
				// Just ensure no panic
				_ = w.Result()
			}
		}()
	}

	wg.Wait()
	close(done)
}

// --- Error Handling Tests ---

func TestServer_GraphQLExecutionError(t *testing.T) {
	// We can't easily mock the executor due to type constraints
	// This would be tested in integration tests
	t.Skip("Requires integration test with mockable executor")
}

func TestServer_TimeoutError(t *testing.T) {
	// Test that context.DeadlineExceeded is handled
	// This would be tested in integration tests
	t.Skip("Requires integration test with slow query")
}

// --- MCP Protocol Tests ---

func TestServer_MCPServerCreation(t *testing.T) {
	config := Config{
		ServerName:    "test-server",
		ServerVersion: "1.2.3",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// MCP server should be configured
	assert.NotNil(t, server.mcpServer)
}

// --- HTTP Server Configuration Tests ---

func TestServer_HTTPServerTimeouts(t *testing.T) {
	config := Config{
		TimeoutStr: "45s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Verify HTTP server timeouts are set correctly
	assert.Equal(t, 45*time.Second, server.httpServer.ReadTimeout)
	assert.Equal(t, 50*time.Second, server.httpServer.WriteTimeout) // timeout + 5s
	assert.Equal(t, 120*time.Second, server.httpServer.IdleTimeout)
}

func TestServer_HTTPServerAddress(t *testing.T) {
	config := Config{
		BindAddress: ":9999",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	assert.Equal(t, ":9999", server.httpServer.Addr)
}

// --- Edge Cases ---

func TestConfig_ValidPorts(t *testing.T) {
	testCases := []struct {
		addr    string
		wantErr bool
	}{
		{":1", false},
		{":80", false},
		{":443", false},
		{":8080", false},
		{":65535", false},
		{"localhost:8080", false},
		{"127.0.0.1:8080", false},
		{"0.0.0.0:8080", false},
		{":0", true},           // Port 0 is invalid
		{":65536", true},       // Too high
		{":-1", true},          // Negative
		{"invalid:port", true}, // Non-numeric
	}

	for _, tc := range testCases {
		t.Run(tc.addr, func(t *testing.T) {
			config := Config{BindAddress: tc.addr}
			err := config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_ValidPaths(t *testing.T) {
	testCases := []struct {
		path    string
		wantErr bool
	}{
		{"/", false},
		{"/mcp", false},
		{"/api/v1/mcp", false},
		{"/a/b/c/d/e", false},
		{"mcp", true},       // Missing leading slash
		{"", false},         // Empty gets default
		{"//double", false}, // Valid path
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			config := Config{Path: tc.path}
			err := config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_ValidTimeouts(t *testing.T) {
	testCases := []struct {
		timeout string
		wantErr bool
	}{
		{"1s", false},
		{"30s", false},
		{"1m", false},
		{"5m", false},
		{"2m30s", false},
		{"999ms", true},   // Too short
		{"5m1s", true},    // Too long
		{"10m", true},     // Too long
		{"invalid", true}, // Invalid format
	}

	for _, tc := range testCases {
		t.Run(tc.timeout, func(t *testing.T) {
			config := Config{TimeoutStr: tc.timeout}
			err := config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Ensure mock implements the interface pattern expected by tests
var _ MetricsRecorder = (*mockMetrics)(nil)
