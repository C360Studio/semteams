//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegration_QueryHandlers tests query handlers with real NATS JetStream
func TestIntegration_QueryHandlers(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:2.10-alpine",
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForLog("Server is ready"),
			Cmd:          []string{"-js"},
		},
		Started: true,
	})
	require.NoError(t, err)
	defer func() {
		_ = natsContainer.Terminate(ctx)
	}()

	// Get NATS connection details
	host, err := natsContainer.Host(ctx)
	require.NoError(t, err)
	port, err := natsContainer.MappedPort(ctx, "4222")
	require.NoError(t, err)

	natsURL := "nats://" + host + ":" + port.Port()

	// Create NATS client
	natsClient, err := natsclient.NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, natsClient.Connect(ctx))
	defer func() {
		_ = natsClient.Close(ctx)
	}()
	require.NoError(t, natsClient.WaitForConnection(ctx))

	// Create component
	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIngest(configJSON, deps)
	require.NoError(t, err)

	component := comp.(*Component)
	require.NoError(t, component.Initialize())
	require.NoError(t, component.Start(ctx))
	defer func() {
		_ = component.Stop(5 * time.Second)
	}()

	// Wait for component to be ready
	time.Sleep(100 * time.Millisecond)

	// Create test entities
	entities := []*graph.EntityState{
		{
			ID: "c360.platform.robotics.mav1.drone.001",
			Triples: []message.Triple{
				{
					Subject:   "c360.platform.robotics.mav1.drone.001",
					Predicate: "robotics.status.armed",
					Object:    true,
					Timestamp: time.Now(),
				},
			},
			Version:   1,
			UpdatedAt: time.Now(),
		},
		{
			ID: "c360.platform.robotics.mav1.drone.002",
			Triples: []message.Triple{
				{
					Subject:   "c360.platform.robotics.mav1.drone.002",
					Predicate: "robotics.battery.level",
					Object:    85.5,
					Timestamp: time.Now(),
				},
			},
			Version:   1,
			UpdatedAt: time.Now(),
		},
	}

	// Store entities
	for _, entity := range entities {
		require.NoError(t, component.CreateEntity(ctx, entity))
	}

	t.Run("query single entity with real NATS", func(t *testing.T) {
		// Subscribe to handle query requests
		querySubject := "graph.ingest.query.entity"
		err := natsClient.SubscribeForRequests(ctx, querySubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
			// Create mock message for handler
			mockMsg := &mockNATSMsg{data: data}
			component.handleQueryEntity(mockMsg)
			return mockMsg.response, nil
		})
		require.NoError(t, err)

		// Send query request
		request := map[string]string{"id": "c360.platform.robotics.mav1.drone.001"}
		requestJSON, err := json.Marshal(request)
		require.NoError(t, err)

		responseData, err := natsClient.Request(ctx, querySubject, requestJSON, 5*time.Second)
		require.NoError(t, err)

		// Verify response
		var responseEntity graph.EntityState
		err = json.Unmarshal(responseData, &responseEntity)
		require.NoError(t, err)

		assert.Equal(t, entities[0].ID, responseEntity.ID)
		assert.Equal(t, len(entities[0].Triples), len(responseEntity.Triples))
	})

	t.Run("batch query with real NATS", func(t *testing.T) {
		// Subscribe to handle batch query requests
		batchSubject := "graph.ingest.query.batch"
		err := natsClient.SubscribeForRequests(ctx, batchSubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
			mockMsg := &mockNATSMsg{data: data}
			component.handleQueryBatch(mockMsg)
			return mockMsg.response, nil
		})
		require.NoError(t, err)

		// Send batch query request
		request := map[string][]string{
			"ids": {
				"c360.platform.robotics.mav1.drone.001",
				"c360.platform.robotics.mav1.drone.002",
			},
		}
		requestJSON, err := json.Marshal(request)
		require.NoError(t, err)

		responseData, err := natsClient.Request(ctx, batchSubject, requestJSON, 5*time.Second)
		require.NoError(t, err)

		// Verify response
		var responseEntities []graph.EntityState
		err = json.Unmarshal(responseData, &responseEntities)
		require.NoError(t, err)

		assert.Equal(t, 2, len(responseEntities))
	})

	t.Run("capabilities query over real NATS", func(t *testing.T) {
		// Subscribe to handle capabilities requests
		capSubject := "graph.ingest.capabilities"
		err := natsClient.SubscribeForRequests(ctx, capSubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
			mockMsg := &mockNATSMsg{data: data}
			component.handleCapabilities(mockMsg)
			return mockMsg.response, nil
		})
		require.NoError(t, err)

		// Send capabilities query
		responseData, err := natsClient.Request(ctx, capSubject, []byte(`{}`), 5*time.Second)
		require.NoError(t, err)

		// Verify response
		var caps QueryCapabilitiesResponse
		err = json.Unmarshal(responseData, &caps)
		require.NoError(t, err)

		assert.Equal(t, "graph-ingest", caps.Component)
		assert.NotEmpty(t, caps.Queries)
	})

	t.Run("concurrent query requests", func(t *testing.T) {
		querySubject := "graph.ingest.query.concurrent"
		err := natsClient.SubscribeForRequests(ctx, querySubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
			mockMsg := &mockNATSMsg{data: data}
			component.handleQueryEntity(mockMsg)
			return mockMsg.response, nil
		})
		require.NoError(t, err)

		// Send multiple concurrent requests
		concurrency := 10
		results := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func(entityID string) {
				request := map[string]string{"id": entityID}
				requestJSON, _ := json.Marshal(request)

				_, err := natsClient.Request(ctx, querySubject, requestJSON, 5*time.Second)
				results <- err
			}("c360.platform.robotics.mav1.drone.001")
		}

		// Collect results
		for i := 0; i < concurrency; i++ {
			err := <-results
			assert.NoError(t, err, "concurrent request should succeed")
		}
	})

	t.Run("context timeout behavior", func(t *testing.T) {
		// This test verifies that the handler respects context timeouts
		// The handler creates its own context with timeout, so we just verify
		// it doesn't hang indefinitely

		querySubject := "graph.ingest.query.timeout"
		err := natsClient.SubscribeForRequests(ctx, querySubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
			mockMsg := &mockNATSMsg{data: data}
			component.handleQueryEntity(mockMsg)
			return mockMsg.response, nil
		})
		require.NoError(t, err)

		// Send request with short overall timeout
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		request := map[string]string{"id": "c360.platform.robotics.mav1.drone.001"}
		requestJSON, _ := json.Marshal(request)

		responseData, err := natsClient.Request(reqCtx, querySubject, requestJSON, 2*time.Second)
		require.NoError(t, err)

		var responseEntity graph.EntityState
		err = json.Unmarshal(responseData, &responseEntity)
		require.NoError(t, err)
	})
}

// QueryCapabilitiesResponse is a helper type for unmarshaling capabilities responses
type QueryCapabilitiesResponse struct {
	Component string                      `json:"component"`
	Version   string                      `json:"version"`
	Queries   []component.QueryCapability `json:"queries"`
}

// mockNATSMsg is reused from the unit tests for integration testing
// This allows us to test handlers without exposing them as public methods
type mockNATSMsgIntegration struct {
	data     []byte
	response []byte
}

func (m *mockNATSMsgIntegration) Data() []byte {
	return m.data
}

func (m *mockNATSMsgIntegration) Respond(data []byte) error {
	m.response = data
	return nil
}
