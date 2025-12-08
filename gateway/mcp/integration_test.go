//go:build integration

package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/processor/graph/datamanager"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/querymanager"

	gql "github.com/c360/semstreams/gateway/graphql"
)

// integrationBuckets holds all KV buckets needed for integration tests
type integrationBuckets struct {
	entity    jetstream.KeyValue
	predicate jetstream.KeyValue
	incoming  jetstream.KeyValue
	alias     jetstream.KeyValue
	spatial   jetstream.KeyValue
	temporal  jetstream.KeyValue
}

// setupIntegrationBuckets creates all KV buckets needed for integration testing
func setupIntegrationBuckets(ctx context.Context, t *testing.T, client *natsclient.TestClient) integrationBuckets {
	entityBucket, err := client.CreateKVBucket(ctx, "ENTITY_STATES")
	require.NoError(t, err, "Failed to create ENTITY_STATES bucket")

	predicateBucket, err := client.CreateKVBucket(ctx, "PREDICATE_INDEX")
	require.NoError(t, err, "Failed to create PREDICATE_INDEX bucket")

	incomingBucket, err := client.CreateKVBucket(ctx, "INCOMING_INDEX")
	require.NoError(t, err, "Failed to create INCOMING_INDEX bucket")

	aliasBucket, err := client.CreateKVBucket(ctx, "ALIAS_INDEX")
	require.NoError(t, err, "Failed to create ALIAS_INDEX bucket")

	spatialBucket, err := client.CreateKVBucket(ctx, "SPATIAL_INDEX")
	require.NoError(t, err, "Failed to create SPATIAL_INDEX bucket")

	temporalBucket, err := client.CreateKVBucket(ctx, "TEMPORAL_INDEX")
	require.NoError(t, err, "Failed to create TEMPORAL_INDEX bucket")

	return integrationBuckets{
		entity:    entityBucket,
		predicate: predicateBucket,
		incoming:  incomingBucket,
		alias:     aliasBucket,
		spatial:   spatialBucket,
		temporal:  temporalBucket,
	}
}

// setupIntegrationDataManager creates and starts the data manager
func setupIntegrationDataManager(
	ctx context.Context,
	t *testing.T,
	entityBucket jetstream.KeyValue,
	wg *sync.WaitGroup,
) (*datamanager.Manager, chan error, chan struct{}) {
	dataConfig := datamanager.DefaultConfig()
	dataDeps := datamanager.Dependencies{
		KVBucket:        entityBucket,
		MetricsRegistry: metric.NewMetricsRegistry(),
		Logger:          slog.Default(),
		Config:          dataConfig,
	}
	dataManager, err := datamanager.NewDataManager(dataDeps)
	require.NoError(t, err, "Failed to create data manager")

	dataErrors := make(chan error, 1)
	dataReady := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		dataErrors <- dataManager.Run(ctx, func() {
			close(dataReady)
		})
	}()

	return dataManager, dataErrors, dataReady
}

// setupIntegrationIndexManager creates and starts the index manager
func setupIntegrationIndexManager(
	ctx context.Context,
	t *testing.T,
	buckets integrationBuckets,
	client *natsclient.Client,
	wg *sync.WaitGroup,
) (indexmanager.Indexer, chan error, chan struct{}) {
	indexConfig := indexmanager.DefaultConfig()
	indexBuckets := map[string]jetstream.KeyValue{
		"ENTITY_STATES":   buckets.entity,
		"PREDICATE_INDEX": buckets.predicate,
		"INCOMING_INDEX":  buckets.incoming,
		"ALIAS_INDEX":     buckets.alias,
		"SPATIAL_INDEX":   buckets.spatial,
		"TEMPORAL_INDEX":  buckets.temporal,
	}
	indexManager, err := indexmanager.NewManager(indexConfig, indexBuckets, client, nil, nil)
	require.NoError(t, err, "Failed to create index manager")

	indexErrors := make(chan error, 1)
	indexReady := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		indexErrors <- indexManager.Run(ctx, func() {
			close(indexReady)
		})
	}()

	return indexManager, indexErrors, indexReady
}

// setupIntegrationQueryManager creates the query manager (stateless, no Run method)
func setupIntegrationQueryManager(
	t *testing.T,
	dataManager *datamanager.Manager,
	indexManager indexmanager.Indexer,
) querymanager.Querier {
	queryConfig := querymanager.Config{}
	queryConfig.SetDefaults()

	queryDeps := querymanager.Deps{
		Config:       queryConfig,
		EntityReader: dataManager,
		IndexManager: indexManager,
		Registry:     nil,
		Logger:       nil,
	}
	queryManager, err := querymanager.NewManager(queryDeps)
	require.NoError(t, err, "Failed to create query manager")

	return queryManager
}

// waitForStartup waits for managers to be ready or errors
func waitForStartup(t *testing.T, dataReady, indexReady chan struct{}, dataErrors, indexErrors chan error) {
	timeout := time.After(10 * time.Second)

	// Wait for data manager
	select {
	case <-dataReady:
	case err := <-dataErrors:
		t.Fatalf("DataManager failed to start: %v", err)
	case <-timeout:
		t.Fatal("DataManager startup timed out")
	}

	// Wait for index manager
	select {
	case <-indexReady:
	case err := <-indexErrors:
		t.Fatalf("IndexManager failed to start: %v", err)
	case <-timeout:
		t.Fatal("IndexManager startup timed out")
	}
}

// waitForIntegrationShutdown waits for all services to shutdown cleanly
func waitForIntegrationShutdown(t *testing.T, wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown
	case <-time.After(5 * time.Second):
		t.Error("Services did not shutdown within 5 seconds")
	}
}

func TestMCP_IntegrationWithNATS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test NATS client with JetStream and KV
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())

	// Create cancellable context for entire test
	ctx, cancel := context.WithCancel(context.Background())

	// Setup infrastructure
	buckets := setupIntegrationBuckets(ctx, t, testClient)

	// Setup managers
	var wg sync.WaitGroup
	dataManager, dataErrors, dataReady := setupIntegrationDataManager(ctx, t, buckets.entity, &wg)
	indexManager, indexErrors, indexReady := setupIntegrationIndexManager(ctx, t, buckets, testClient.Client, &wg)

	// Ensure cleanup happens even on test failure
	t.Cleanup(func() {
		cancel()
		waitForIntegrationShutdown(t, &wg)
	})

	// Wait for startup
	waitForStartup(t, dataReady, indexReady, dataErrors, indexErrors)

	queryManager := setupIntegrationQueryManager(t, dataManager, indexManager)

	// Create MCP components
	logger := slog.Default()
	resolver := gql.NewBaseResolver(queryManager, nil)
	executor, err := NewExecutor(resolver, logger)
	require.NoError(t, err, "Failed to create executor")

	t.Run("GraphQL Entity Query via Executor", func(t *testing.T) {
		// Create a test entity
		entity := &gtypes.EntityState{
			ID: "test.platform.domain.system.type.mcp1",
			Triples: []message.Triple{
				{
					Subject:   "test.platform.domain.system.type.mcp1",
					Predicate: "type",
					Object:    "domain.type",
				},
				{
					Subject:   "test.platform.domain.system.type.mcp1",
					Predicate: "domain.entity.name",
					Object:    "MCPTestEntity",
				},
			},
			Version:   1,
			UpdatedAt: time.Now(),
		}

		// Store the entity via DataManager
		_, err := dataManager.CreateEntity(ctx, entity)
		require.NoError(t, err, "Failed to create entity")

		// Wait for entity to be persisted using polling instead of sleep
		entityID := "test.platform.domain.system.type.mcp1"
		require.Eventually(t, func() bool {
			e, err := dataManager.GetEntity(ctx, entityID)
			return err == nil && e != nil
		}, 2*time.Second, 10*time.Millisecond, "Entity should be persisted")

		// Execute GraphQL query through MCP executor
		query := `{ entity(id: "test.platform.domain.system.type.mcp1") { id } }`
		result, err := executor.Execute(ctx, query, nil)
		require.NoError(t, err, "GraphQL execution failed")

		// Verify result with safe type assertions
		data, ok := result.(map[string]any)
		require.True(t, ok, "Result should be a map")
		require.Contains(t, data, "data")

		queryResult, ok := data["data"].(map[string]any)
		require.True(t, ok, "data should be a map")
		require.Contains(t, queryResult, "entity")

		entityResult, ok := queryResult["entity"].(map[string]any)
		require.True(t, ok, "entity should be a map")
		assert.Equal(t, "test.platform.domain.system.type.mcp1", entityResult["id"])
	})

	t.Run("GraphQL Query NonExistent Entity Returns Error", func(t *testing.T) {
		query := `{ entity(id: "nonexistent.entity.id") { id } }`
		_, err := executor.Execute(ctx, query, nil)

		// Real QueryManager returns error for non-existent entities
		// This validates that errors propagate correctly through the stack
		require.Error(t, err, "Non-existent entity should return error")
		assert.Contains(t, err.Error(), "not found", "Error should indicate entity not found")
	})

	t.Run("Schema Retrieval", func(t *testing.T) {
		schema := executor.GetSchema()
		assert.Contains(t, schema, "type Query")
		assert.Contains(t, schema, "entity(id: ID!)")
		assert.Contains(t, schema, "Entity")
	})
	// Cleanup handled by t.Cleanup
}

func TestMCP_ServerIntegrationWithNATS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test NATS client with JetStream and KV
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())

	// Create cancellable context for entire test
	ctx, cancel := context.WithCancel(context.Background())

	// Setup infrastructure
	buckets := setupIntegrationBuckets(ctx, t, testClient)

	// Setup managers
	var wg sync.WaitGroup
	dataManager, dataErrors, dataReady := setupIntegrationDataManager(ctx, t, buckets.entity, &wg)
	indexManager, indexErrors, indexReady := setupIntegrationIndexManager(ctx, t, buckets, testClient.Client, &wg)

	// Ensure cleanup happens even on test failure
	t.Cleanup(func() {
		cancel()
		waitForIntegrationShutdown(t, &wg)
	})

	// Wait for startup
	waitForStartup(t, dataReady, indexReady, dataErrors, indexErrors)

	queryManager := setupIntegrationQueryManager(t, dataManager, indexManager)

	// Create MCP server components
	logger := slog.Default()
	resolver := gql.NewBaseResolver(queryManager, nil)
	executor, err := NewExecutor(resolver, logger)
	require.NoError(t, err, "Failed to create executor")

	cfg := Config{
		BindAddress:    "127.0.0.1:18998", // High port for integration tests
		TimeoutStr:     "5s",
		Path:           "/mcp",
		ServerName:     "integration-test-server",
		ServerVersion:  "1.0.0",
		MaxRequestSize: 1 << 20,
	}
	err = cfg.Validate()
	require.NoError(t, err, "Config validation failed")

	metrics := newMockMetricsRecorder()
	server, err := NewServer(cfg, executor, metrics, logger)
	require.NoError(t, err, "Failed to create server")

	err = server.Setup()
	require.NoError(t, err, "Server setup failed")

	// Start server in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	ready := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start(serverCtx, ready)
	}()

	// Wait for server to be ready
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("Server failed to start within timeout")
	}

	t.Run("Health Endpoint Returns Healthy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		server.handleHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "healthy", response["status"])
	})

	t.Run("Schema Endpoint Returns GraphQL Schema", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/schema", nil)
		w := httptest.NewRecorder()

		server.handleSchema(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "type Query")
		assert.Contains(t, w.Body.String(), "entity(id: ID!)")
	})

	t.Run("Server Is Running", func(t *testing.T) {
		assert.True(t, server.IsRunning())
	})

	// Shutdown server
	serverCancel()

	// Wait for server shutdown
	select {
	case err := <-serverErr:
		// Expect either nil or context.Canceled
		if err != nil {
			assert.ErrorIs(t, err, context.Canceled, "Expected context.Canceled or nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not shutdown within timeout")
	}
	// Manager cleanup handled by t.Cleanup
}
