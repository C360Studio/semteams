package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateway_Meta(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	gw := &Gateway{
		name:     "test-mcp-gateway",
		config:   testConfig(),
		logger:   testLogger(),
		resolver: createTestResolver(mq),
		executor: exec,
		server:   server,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	meta := gw.Meta()
	assert.Equal(t, "test-mcp-gateway", meta.Name)
	assert.Equal(t, "gateway", meta.Type)
	assert.Contains(t, meta.Description, "MCP")
}

func TestGateway_InputPorts(t *testing.T) {
	gw := &Gateway{}
	ports := gw.InputPorts()
	assert.Empty(t, ports)
}

func TestGateway_OutputPorts(t *testing.T) {
	gw := &Gateway{}
	ports := gw.OutputPorts()
	assert.Empty(t, ports)
}

func TestGateway_ConfigSchema(t *testing.T) {
	gw := &Gateway{}
	schema := gw.ConfigSchema()
	assert.NotNil(t, schema)
}

func TestGateway_Health_NotRunning(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	gw := &Gateway{
		name:     "test-mcp-gateway",
		config:   testConfig(),
		logger:   testLogger(),
		resolver: createTestResolver(mq),
		executor: exec,
		server:   server,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	health := gw.Health()
	assert.False(t, health.Healthy)
}

func TestGateway_DataFlow(t *testing.T) {
	gw := &Gateway{
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	flow := gw.DataFlow()
	assert.Equal(t, float64(0), flow.ErrorRate)
}

func TestGateway_DataFlow_WithMetrics(t *testing.T) {
	gw := &Gateway{
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	// Record some metrics
	gw.requestsTotal.Store(100)
	gw.requestsFailed.Store(10)
	gw.lastActivity.Store(time.Now())

	flow := gw.DataFlow()
	assert.Equal(t, 0.1, flow.ErrorRate)
	assert.False(t, flow.LastActivity.IsZero())
}

func TestGateway_RecordRequest(t *testing.T) {
	gw := &Gateway{
		logger:   testLogger(),
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	ctx := context.Background()

	// Record successful request
	gw.RecordRequest(ctx, true, 100*time.Millisecond)

	assert.Equal(t, uint64(1), gw.requestsTotal.Load())
	assert.Equal(t, uint64(1), gw.requestsSuccess.Load())
	assert.Equal(t, uint64(0), gw.requestsFailed.Load())

	// Record failed request
	gw.RecordRequest(ctx, false, 50*time.Millisecond)

	assert.Equal(t, uint64(2), gw.requestsTotal.Load())
	assert.Equal(t, uint64(1), gw.requestsSuccess.Load())
	assert.Equal(t, uint64(1), gw.requestsFailed.Load())
}

func TestGateway_Stop_NotRunning(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	metrics := newMockMetricsRecorder()
	server, err := NewServer(testConfig(), exec, metrics, testLogger())
	require.NoError(t, err)

	gw := &Gateway{
		name:     "test-mcp-gateway",
		config:   testConfig(),
		logger:   testLogger(),
		resolver: createTestResolver(mq),
		executor: exec,
		server:   server,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	// Stop when not running should be no-op
	err = gw.Stop(time.Second)
	require.NoError(t, err)
}
