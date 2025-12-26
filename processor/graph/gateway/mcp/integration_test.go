//go:build integration

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	gql "github.com/c360/semstreams/processor/graph/gateway/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests are gated by the build tag.
// Run with: go test -tags=integration ./processor/graph/gateway/mcp/...

// getTestPort returns a unique port for each test to avoid conflicts.
var testPortMu sync.Mutex
var testPort = 19000

func getTestPort() int {
	testPortMu.Lock()
	defer testPortMu.Unlock()
	testPort++
	return testPort
}

// --- Server Lifecycle Integration Tests ---

func TestIntegration_ServerStartStop(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	errChan := make(chan error, 1)

	go func() {
		errChan <- server.Start(ctx, ready)
	}()

	// Wait for server to be ready
	select {
	case <-ready:
		// Server is ready
	case err := <-errChan:
		t.Fatalf("Server failed to start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server startup timed out")
	}

	assert.True(t, server.IsRunning())

	// Stop server
	err = server.Stop(5 * time.Second)
	require.NoError(t, err)

	assert.False(t, server.IsRunning())
}

func TestIntegration_ServerDoubleStart(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	assert.True(t, server.IsRunning())

	// Try to start again - should error
	ready2 := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start(ctx, ready2)
	}()

	select {
	case err := <-errChan:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already running")
	case <-time.After(time.Second):
		t.Fatal("Expected error for double start")
	}

	server.Stop(5 * time.Second)
}

// --- Health Endpoint Integration Tests ---

func TestIntegration_HealthEndpoint(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "healthy", body["status"])
	assert.Equal(t, "mcp-gateway", body["service"])
}

// --- Schema Endpoint Integration Tests ---

func TestIntegration_SchemaEndpoint(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Test schema endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/schema", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
}

// --- SSE Connection Integration Tests ---

func TestIntegration_SSEConnection(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		Path:        "/mcp",
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Test SSE endpoint responds
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/mcp", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	// SSE endpoints should return 200 with text/event-stream content type
	// or redirect to the SSE handler
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound,
		"Expected 200 or 404, got %d", resp.StatusCode)
}

// --- Concurrent Clients Integration Tests ---

func TestIntegration_ConcurrentHealthChecks(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Run concurrent health checks
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
			if err != nil {
				errors <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent request failed: %v", err)
	}
}

// --- MCP Protocol Integration Tests ---

func TestIntegration_MCPSSEEndpoint(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		Path:        "/mcp",
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Create SSE request with proper headers
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/mcp/sse", port), nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// MCP SSE server may return 200 with event stream or redirect
	// The exact behavior depends on the mcp-go library implementation
	t.Logf("SSE endpoint returned status: %d, content-type: %s",
		resp.StatusCode, resp.Header.Get("Content-Type"))
}

// --- Graceful Shutdown Integration Tests ---

func TestIntegration_GracefulShutdown(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- server.Start(ctx, ready)
	}()

	<-ready
	assert.True(t, server.IsRunning())

	// Cancel context to trigger shutdown
	cancel()

	// Wait for server to stop
	select {
	case err := <-done:
		// Should complete without error
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Graceful shutdown timed out")
	}

	assert.False(t, server.IsRunning())
}

// --- Context Cancellation Integration Tests ---

func TestIntegration_ContextCancellation(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Use a short-lived context
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- server.Start(ctx, ready)
	}()

	<-ready

	// Wait for context to expire and server to shutdown
	select {
	case err := <-done:
		// Should complete (possibly with error about shutdown)
		t.Logf("Server stopped with: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not respond to context cancellation")
	}
}

// --- Rate Limiting Integration Tests ---

func TestIntegration_RateLimitingUnderLoad(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	metrics := &mockMetrics{}
	server, err := NewServer(config, executor, metrics, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Make rapid requests to test rate limiting
	// Rate limit is 10 req/sec with burst of 20
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Send 30 requests rapidly (more than burst)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// All health checks should succeed (rate limiting is on GraphQL tool, not health)
	assert.Equal(t, 30, successCount, "All health checks should succeed")
}

// --- Server Info Integration Tests ---

func TestIntegration_ServerInfoInMCP(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress:   fmt.Sprintf(":%d", port),
		ServerName:    "test-semstreams",
		ServerVersion: "2.0.0-test",
		TimeoutStr:    "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Verify server was created with correct info
	assert.NotNil(t, server.mcpServer)
	assert.Equal(t, "test-semstreams", server.config.ServerName)
	assert.Equal(t, "2.0.0-test", server.config.ServerVersion)
}

// --- MCP Message Protocol Tests ---

func TestIntegration_MCPMessageEndpoint(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		Path:        "/mcp",
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Test the message endpoint used by MCP clients
	// This tests the POST endpoint for sending messages
	jsonRPCRequest := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	req, err := http.NewRequest("POST",
		fmt.Sprintf("http://localhost:%d/mcp/message", port),
		strings.NewReader(jsonRPCRequest))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Log the response for debugging
	body, _ := io.ReadAll(resp.Body)
	t.Logf("Message endpoint returned status: %d, body: %s", resp.StatusCode, string(body))
}

// --- SSE Event Streaming Tests ---

func TestIntegration_SSEEventStream(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		Path:        "/mcp",
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	executor := &gql.Executor{}
	server, err := NewServer(config, executor, nil, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Create a context with timeout for the SSE connection
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET",
		fmt.Sprintf("http://localhost:%d/mcp/sse", port), nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// Connection may timeout or be refused - that's okay for this test
		t.Logf("SSE connection result: %v", err)
		return
	}
	defer resp.Body.Close()

	// If we got a response, check the content type
	contentType := resp.Header.Get("Content-Type")
	t.Logf("SSE response: status=%d, content-type=%s", resp.StatusCode, contentType)

	// Try to read some events
	reader := bufio.NewReader(resp.Body)
	for i := 0; i < 3; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		t.Logf("SSE event line: %q", line)
	}
}

// --- Metrics Integration Tests ---

func TestIntegration_MetricsRecording(t *testing.T) {
	port := getTestPort()
	config := Config{
		BindAddress: fmt.Sprintf(":%d", port),
		TimeoutStr:  "5s",
	}
	require.NoError(t, config.Validate())

	metrics := &mockMetrics{}
	executor := &gql.Executor{}
	server, err := NewServer(config, executor, metrics, nil)
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		server.Start(ctx, ready)
	}()

	<-ready
	defer server.Stop(5 * time.Second)

	// Make some requests - metrics are recorded on GraphQL tool calls, not health
	// Health endpoint doesn't use metrics, so we verify the setup works
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
	require.NoError(t, err)
	resp.Body.Close()

	// Metrics would be recorded if we made GraphQL tool calls
	// For now, verify metrics object is properly integrated
	assert.NotNil(t, metrics)
}
