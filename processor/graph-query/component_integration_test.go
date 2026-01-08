//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/natsclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestNATS starts a NATS server using testcontainers
func setupTestNATS(t *testing.T) (*natsclient.Client, func()) {
	t.Helper()

	ctx := context.Background()

	// Start NATS container with JetStream
	req := testcontainers.ContainerRequest{
		Image:        "nats:2.10-alpine",
		ExposedPorts: []string{"4222/tcp"},
		WaitingFor:   wait.ForListeningPort("4222/tcp"),
		Cmd:          []string{"-js"}, // Enable JetStream
	}

	natsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	// Get connection URL
	host, err := natsContainer.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := natsContainer.MappedPort(ctx, "4222")
	require.NoError(t, err)

	natsURL := "nats://" + host + ":" + mappedPort.Port()

	// Create NATS client
	client, err := natsclient.NewClient(natsURL,
		natsclient.WithMaxReconnects(-1),
		natsclient.WithReconnectWait(1*time.Second),
	)
	require.NoError(t, err)

	// Connect
	err = client.Connect(ctx)
	require.NoError(t, err)

	cleanup := func() {
		client.Close(ctx)
		_ = natsContainer.Terminate(ctx)
	}

	return client, cleanup
}

func TestIntegration_ComponentLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// Create component
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)
	require.NotNil(t, comp)

	// Type assert to access component-specific fields
	graphQuery, ok := comp.(*Component)
	require.True(t, ok, "component should be *Component type")

	// Test lifecycle
	ctx := context.Background()

	// Initialize
	err = graphQuery.Initialize()
	assert.NoError(t, err)

	// Start
	err = graphQuery.Start(ctx)
	assert.NoError(t, err)

	// Check health
	health := graphQuery.Health()
	assert.True(t, health.Healthy, "component should be healthy after start")

	// Stop
	err = graphQuery.Stop(5 * time.Second)
	assert.NoError(t, err)
}

func TestIntegration_ComponentDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// Create component
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)

	// Test Discoverable interface methods
	meta := comp.Meta()
	assert.Equal(t, "processor", meta.Type)
	assert.Equal(t, "graph-query", meta.Name)

	inputPorts := comp.InputPorts()
	assert.NotEmpty(t, inputPorts, "should have input ports")

	outputPorts := comp.OutputPorts()
	assert.Empty(t, outputPorts, "query coordinator should have no output ports")

	schema := comp.ConfigSchema()
	assert.NotNil(t, schema.Properties)
}

func TestIntegration_QueryCapabilities_GracefulDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// Create and start component
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)

	graphQuery := comp.(*Component)
	require.NoError(t, graphQuery.Initialize())

	ctx := context.Background()
	require.NoError(t, graphQuery.Start(ctx))
	defer graphQuery.Stop(1 * time.Second)

	// Query capabilities when no other components are running
	// Should return empty list, not error (graceful degradation)
	response, err := graphQuery.handleQueryCapabilities(ctx, []byte{})

	assert.NoError(t, err)
	assert.NotNil(t, response)

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	require.NoError(t, err)

	components, ok := result["components"].([]interface{})
	assert.True(t, ok)
	assert.Empty(t, components, "should return empty list when components unavailable")
}

func TestIntegration_ConfigValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name:        "valid config",
			config:      DefaultConfig(),
			expectError: false,
		},
		{
			name: "missing ports",
			config: Config{
				Ports: nil,
			},
			expectError: true,
		},
		{
			name: "empty inputs",
			config: Config{
				Ports: &component.PortConfig{
					Inputs:  []component.PortDefinition{},
					Outputs: []component.PortDefinition{},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configJSON, err := json.Marshal(tt.config)
			require.NoError(t, err)

			deps := component.Dependencies{
				NATSClient: natsClient,
			}

			comp, err := CreateGraphQuery(configJSON, deps)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, comp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, comp)
			}
		})
	}
}

func TestIntegration_MetricsTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// Create and start component
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)

	graphQuery := comp.(*Component)
	require.NoError(t, graphQuery.Initialize())

	ctx := context.Background()
	require.NoError(t, graphQuery.Start(ctx))
	defer graphQuery.Stop(1 * time.Second)

	// Get initial metrics
	metrics := graphQuery.DataFlow()
	assert.GreaterOrEqual(t, metrics.MessagesPerSecond, float64(0))
	assert.GreaterOrEqual(t, metrics.BytesPerSecond, float64(0))
	assert.GreaterOrEqual(t, metrics.ErrorRate, float64(0))
}

func TestIntegration_PathSearch_Structure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// Create and start component
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)

	graphQuery := comp.(*Component)
	require.NoError(t, graphQuery.Initialize())

	ctx := context.Background()
	require.NoError(t, graphQuery.Start(ctx))
	defer graphQuery.Stop(1 * time.Second)

	// Test PathSearch request structure
	req := PathSearchRequest{
		StartEntity: "test.entity.001",
		MaxDepth:    3,
	}

	reqData, err := json.Marshal(req)
	require.NoError(t, err)

	// This will fail because graph-ingest isn't running,
	// but it validates request parsing and structure
	response, err := graphQuery.handlePathSearch(ctx, reqData)

	// Should error (entity not found) but not panic
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "entity")
}

// TestIntegration_LazyDiscoveryFromIngest verifies that graph-query lazily discovers
// routes from graph-ingest when queries are made (not at startup).
// This tests the fix for the startup race condition where graph-query
// was trying to discover capabilities before graph-ingest had registered handlers.
func TestIntegration_LazyDiscoveryFromIngest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// Step 1: Start a capability handler that simulates graph-ingest
	// This registers the capabilities BEFORE graph-query starts
	ingestCaps := component.QueryCapabilities{
		Component: "graph-ingest",
		Version:   "1.0.0",
		Queries: []component.QueryCapability{
			{
				Subject:   "graph.ingest.query.entity",
				Operation: "getEntity",
				Intent: component.QueryIntent{
					Type:     component.IntentTypeEntity,
					Strategy: component.StrategyDirect,
					Scope:    component.ScopeSingle,
				},
			},
			{
				Subject:   "graph.ingest.query.batch",
				Operation: "getBatch",
				Intent: component.QueryIntent{
					Type:     component.IntentTypeEntity,
					Strategy: component.StrategyBatch,
					Scope:    component.ScopeSet,
				},
			},
			{
				Subject:   "graph.ingest.query.prefix",
				Operation: "listByPrefix",
				Intent: component.QueryIntent{
					Type:     component.IntentTypeEntity,
					Strategy: component.StrategyDirect,
					Scope:    component.ScopeSet,
				},
			},
		},
	}

	err := natsClient.SubscribeForRequests(ctx, "graph.ingest.capabilities", func(_ context.Context, _ []byte) ([]byte, error) {
		return json.Marshal(ingestCaps)
	})
	require.NoError(t, err, "should subscribe for capabilities")

	// Small delay to ensure subscription is active
	time.Sleep(100 * time.Millisecond)

	// Step 2: Create and start graph-query
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)

	graphQuery := comp.(*Component)
	require.NoError(t, graphQuery.Initialize())
	require.NoError(t, graphQuery.Start(ctx))
	defer graphQuery.Stop(1 * time.Second)

	// Step 3: Verify router cache is empty at startup (lazy discovery)
	initialRouteCount := graphQuery.router.RouteCount()
	assert.Equal(t, 0, initialRouteCount, "router cache should be empty at startup (lazy discovery)")

	// Step 4: Trigger discovery by calling Route() with an uncached intent
	intent := component.QueryIntent{
		Type:     component.IntentTypeEntity,
		Strategy: component.StrategyDirect,
		Scope:    component.ScopeSingle,
	}
	subject := graphQuery.router.Route(ctx, intent)

	// Step 5: Verify discovery occurred
	assert.Equal(t, "graph.ingest.query.entity", subject, "should discover route from graph-ingest")

	// Step 6: Verify routes are cached (all 3 routes from ingest should be cached)
	finalRouteCount := graphQuery.router.RouteCount()
	assert.Equal(t, 3, finalRouteCount, "should cache all routes from discovery response")

	// Step 7: Verify subsequent queries use cached routes (no additional discovery)
	// Query for another intent that was in the same capability response
	intent2 := component.QueryIntent{
		Type:     component.IntentTypeEntity,
		Strategy: component.StrategyBatch,
		Scope:    component.ScopeSet,
	}
	subject2 := graphQuery.router.Route(ctx, intent2)
	assert.Equal(t, "graph.ingest.query.batch", subject2, "should return cached route")

	// Route count should still be 3 (no new discovery needed)
	assert.Equal(t, 3, graphQuery.router.RouteCount(), "route count should remain unchanged")
}

// TestIntegration_LazyDiscoveryFallback verifies fallback behavior when
// discovery fails (no capability handlers available).
func TestIntegration_LazyDiscoveryFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	natsClient, cleanup := setupTestNATS(t)
	defer cleanup()

	// DO NOT register any capability handlers - simulate no components available

	// Create and start graph-query
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := CreateGraphQuery(configJSON, deps)
	require.NoError(t, err)

	graphQuery := comp.(*Component)
	require.NoError(t, graphQuery.Initialize())
	require.NoError(t, graphQuery.Start(ctx))
	defer graphQuery.Stop(1 * time.Second)

	// Verify router cache is empty at startup
	assert.Equal(t, 0, graphQuery.router.RouteCount(), "router cache should be empty at startup")

	// Trigger discovery by calling Route() - should fall back to hardcoded routes
	intent := component.QueryIntent{
		Type:     component.IntentTypeEntity,
		Strategy: component.StrategyDirect,
		Scope:    component.ScopeSingle,
	}
	subject := graphQuery.router.Route(ctx, intent)

	// Should return fallback subject (from hardcoded fallback map)
	assert.Equal(t, "graph.ingest.query.entity", subject, "should use fallback when discovery fails")

	// Cache should still be empty (discovery failed, fallback used)
	assert.Equal(t, 0, graphQuery.router.RouteCount(), "cache should be empty when discovery fails")
}
