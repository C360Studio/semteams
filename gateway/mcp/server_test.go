package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() Config {
	cfg := Config{
		BindAddress:    "127.0.0.1:18999", // High port for testing
		TimeoutStr:     "5s",
		Path:           "/mcp",
		ServerName:     "test-server",
		ServerVersion:  "1.0.0",
		MaxRequestSize: 1 << 20,
	}
	err := cfg.Validate()
	if err != nil {
		panic(err)
	}
	return cfg
}

func TestNewServer(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())

	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.False(t, server.IsRunning())
}

func TestNewServer_NilExecutor(t *testing.T) {
	metrics := newMockMetricsRecorder()
	_, err := NewServer(testConfig(), nil, metrics, testLogger())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor")
}

func TestNewServer_NilLogger(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, nil)

	require.NoError(t, err)
	assert.NotNil(t, server)
}

func TestServer_Setup(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Verify MCP server was created
	assert.NotNil(t, server.mcpServer)
	assert.NotNil(t, server.httpServer)
}

func TestServer_IsRunning(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	assert.False(t, server.IsRunning())

	err = server.Setup()
	require.NoError(t, err)

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})

	go func() {
		_ = server.Start(ctx, ready)
	}()

	// Wait for server to be ready
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("Server failed to start")
	}

	assert.True(t, server.IsRunning())

	// Stop server
	cancel()
	time.Sleep(100 * time.Millisecond) // Give time for shutdown
}

func TestServer_HandleHealth(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Test health handler directly
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "unavailable", response["status"])
	assert.Equal(t, "mcp-gateway", response["service"])
}

func TestServer_HandleHealth_WhenRunning(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		_ = server.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("Server failed to start")
	}

	// Test health handler when running
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
}

func TestServer_HandleSchema(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/schema", nil)
	w := httptest.NewRecorder()

	server.handleSchema(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "type Query")
	assert.Contains(t, w.Body.String(), "entity(id: ID!)")
}

func TestServer_Stop_WhenNotRunning(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	// Stop without starting should be no-op
	err = server.Stop(time.Second)
	require.NoError(t, err)
}

func TestServer_Start_AlreadyRunning(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	err = server.Setup()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		_ = server.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("Server failed to start")
	}

	// Try to start again
	err = server.Start(ctx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already")
}
