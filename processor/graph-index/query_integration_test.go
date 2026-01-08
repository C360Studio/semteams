//go:build integration

package graphindex

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startNATSContainer starts a NATS container with JetStream enabled
func startNATSContainer(ctx context.Context, t *testing.T) (testcontainers.Container, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "nats:2.10",
		ExposedPorts: []string{"4222/tcp", "8222/tcp"},
		WaitingFor:   wait.ForListeningPort("4222/tcp"),
		Cmd:          []string{"-js", "-m", "8222"}, // Enable JetStream and monitoring
	}

	natsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := natsContainer.Host(ctx)
	require.NoError(t, err)

	port, err := natsContainer.MappedPort(ctx, "4222")
	require.NoError(t, err)

	natsURL := fmt.Sprintf("nats://%s:%s", host, port.Port())

	// Wait for NATS to be fully ready
	time.Sleep(200 * time.Millisecond)

	return natsContainer, natsURL
}

// setupIntegrationTest creates a real NATS container and component
func setupIntegrationTest(t *testing.T) (*Component, *natsclient.Client, func()) {
	t.Helper()

	ctx := context.Background()

	// Start NATS container
	natsContainer, natsURL := startNATSContainer(ctx, t)
	require.NotNil(t, natsContainer)

	// Create NATS client
	natsClient, err := natsclient.NewClient(natsURL)
	require.NoError(t, err)

	// Wait for NATS to be ready
	time.Sleep(100 * time.Millisecond)

	// Create component
	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIndex(configJSON, deps)
	require.NoError(t, err)

	component := comp.(*Component)

	// Initialize and start component
	require.NoError(t, component.Initialize())
	require.NoError(t, component.Start(ctx))

	// Wait for component to be ready
	time.Sleep(200 * time.Millisecond)

	// Set up query request handlers using NATS request/reply
	nc := natsClient.GetConnection()

	// Subscribe to outgoing query requests
	_, err = nc.Subscribe("graph.index.query.outgoing", func(msg *nats.Msg) {
		component.handleQueryOutgoing(&natsMsg{Msg: msg})
	})
	require.NoError(t, err)

	// Subscribe to incoming query requests
	_, err = nc.Subscribe("graph.index.query.incoming", func(msg *nats.Msg) {
		component.handleQueryIncoming(&natsMsg{Msg: msg})
	})
	require.NoError(t, err)

	// Subscribe to alias query requests
	_, err = nc.Subscribe("graph.index.query.alias", func(msg *nats.Msg) {
		component.handleQueryAlias(&natsMsg{Msg: msg})
	})
	require.NoError(t, err)

	// Subscribe to predicate query requests
	_, err = nc.Subscribe("graph.index.query.predicate", func(msg *nats.Msg) {
		component.handleQueryPredicate(&natsMsg{Msg: msg})
	})
	require.NoError(t, err)

	// Wait for subscriptions to be established
	nc.Flush()
	time.Sleep(100 * time.Millisecond)

	// Cleanup function
	cleanup := func() {
		component.Stop(5 * time.Second)
		natsClient.Close(ctx)
		natsContainer.Terminate(ctx)
	}

	return component, natsClient, cleanup
}

// TestQueryOutgoing_Integration tests outgoing query with real NATS
func TestQueryOutgoing_Integration(t *testing.T) {
	_, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data - add entity state to trigger indexing
	entityID := "c360.platform.robotics.mav1.drone.001"
	targetID := "c360.platform.robotics.mav1.mission.001"
	predicate := "robotics.assigned.mission"

	// Write entity state to trigger indexing
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	entityBucket, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)

	state := graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{
				Subject:   entityID,
				Predicate: predicate,
				Object:    targetID,
			},
		},
	}

	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	_, err = entityBucket.Put(ctx, entityID, stateJSON)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(300 * time.Millisecond)

	// Create query request
	nc := natsClient.GetConnection()
	request := map[string]string{"entity_id": entityID}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	// Send query request
	msg, err := nc.Request("graph.index.query.outgoing", requestJSON, 2*time.Second)
	require.NoError(t, err)

	// Parse response
	var response []OutgoingEntry
	err = json.Unmarshal(msg.Data, &response)
	require.NoError(t, err)

	// Verify response
	assert.Len(t, response, 1, "should have one outgoing relationship")
	if len(response) > 0 {
		assert.Equal(t, targetID, response[0].ToEntityID)
		assert.Equal(t, predicate, response[0].Predicate)
	}
}

// TestQueryIncoming_Integration tests incoming query with real NATS
func TestQueryIncoming_Integration(t *testing.T) {
	_, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data - add entity state to trigger indexing
	sourceID := "c360.platform.robotics.mav1.drone.001"
	targetID := "c360.platform.robotics.mav1.mission.001"
	predicate := "robotics.assigned.mission"

	// Write entity state to trigger indexing
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	entityBucket, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)

	state := graph.EntityState{
		ID: sourceID,
		Triples: []message.Triple{
			{
				Subject:   sourceID,
				Predicate: predicate,
				Object:    targetID,
			},
		},
	}

	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	_, err = entityBucket.Put(ctx, sourceID, stateJSON)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(300 * time.Millisecond)

	// Create query request for incoming relationships
	nc := natsClient.GetConnection()
	request := map[string]string{"entity_id": targetID}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	// Send query request
	msg, err := nc.Request("graph.index.query.incoming", requestJSON, 2*time.Second)
	require.NoError(t, err)

	// Parse response
	var response []IncomingEntry
	err = json.Unmarshal(msg.Data, &response)
	require.NoError(t, err)

	// Verify response
	assert.Len(t, response, 1, "should have one incoming relationship")
	if len(response) > 0 {
		assert.Equal(t, sourceID, response[0].FromEntityID)
		assert.Equal(t, predicate, response[0].Predicate)
	}
}

// TestQueryAlias_Integration tests alias query with real NATS
func TestQueryAlias_Integration(t *testing.T) {
	_, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data with alias
	entityID := "c360.platform.robotics.mav1.drone.001"
	alias := "drone-001"

	// Write entity state with alias to trigger indexing
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	entityBucket, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)

	state := graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{
				Subject:   entityID,
				Predicate: "core.identity.alias",
				Object:    alias,
			},
		},
	}

	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	_, err = entityBucket.Put(ctx, entityID, stateJSON)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(300 * time.Millisecond)

	// Create query request
	nc := natsClient.GetConnection()
	request := map[string]string{"alias": alias}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	// Send query request
	msg, err := nc.Request("graph.index.query.alias", requestJSON, 2*time.Second)
	require.NoError(t, err)

	// Parse response
	var response map[string]string
	err = json.Unmarshal(msg.Data, &response)
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, entityID, response["canonical_id"])
}

// TestQueryPredicate_Integration tests predicate query with real NATS
func TestQueryPredicate_Integration(t *testing.T) {
	_, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data with multiple entities using same predicate
	predicate := "robotics.type.drone"
	entities := []string{
		"c360.platform.robotics.mav1.drone.001",
		"c360.platform.robotics.mav1.drone.002",
	}

	// Write entity states to trigger indexing
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	entityBucket, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)

	for _, entityID := range entities {
		state := graph.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: predicate,
					Object:    "drone",
				},
			},
		}

		stateJSON, err := json.Marshal(state)
		require.NoError(t, err)

		_, err = entityBucket.Put(ctx, entityID, stateJSON)
		require.NoError(t, err)
	}

	// Wait for indexing
	time.Sleep(300 * time.Millisecond)

	// Create query request
	nc := natsClient.GetConnection()
	request := map[string]string{"predicate": predicate}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	// Send query request
	msg, err := nc.Request("graph.index.query.predicate", requestJSON, 2*time.Second)
	require.NoError(t, err)

	// Parse response
	var response map[string][]string
	err = json.Unmarshal(msg.Data, &response)
	require.NoError(t, err)

	// Verify response
	assert.Len(t, response["entities"], 2, "should have two entities with predicate")

	// Verify all expected entities are present
	entityMap := make(map[string]bool)
	for _, id := range response["entities"] {
		entityMap[id] = true
	}

	for _, expected := range entities {
		assert.True(t, entityMap[expected], "entity %s should be in response", expected)
	}
}

// TestContextTimeout_Integration tests context timeout behavior
func TestContextTimeout_Integration(t *testing.T) {
	comp, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add data first
	entityID := "c360.platform.robotics.mav1.drone.001"
	require.NoError(t, comp.UpdateOutgoingIndex(ctx, entityID, "target", "predicate"))

	// Create query request
	nc := natsClient.GetConnection()
	request := map[string]string{"entity_id": entityID}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	// Send query request (should complete within timeout)
	msg, err := nc.Request("graph.index.query.outgoing", requestJSON, 2*time.Second)
	require.NoError(t, err)

	// Verify we got a response
	var response []OutgoingEntry
	err = json.Unmarshal(msg.Data, &response)
	require.NoError(t, err)
}

// TestConcurrentQueries_Integration tests concurrent query requests
func TestConcurrentQueries_Integration(t *testing.T) {
	comp, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add multiple entities
	for i := 0; i < 10; i++ {
		entityID := "c360.platform.robotics.mav1.drone." + string(rune('0'+i))
		require.NoError(t, comp.UpdateOutgoingIndex(ctx, entityID, "target", "predicate"))
	}

	// Create query requests concurrently
	nc := natsClient.GetConnection()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			entityID := "c360.platform.robotics.mav1.drone." + string(rune('0'+idx))
			request := map[string]string{"entity_id": entityID}
			requestJSON, _ := json.Marshal(request)

			// Send query request
			msg, err := nc.Request("graph.index.query.outgoing", requestJSON, 2*time.Second)
			assert.NoError(t, err)

			if err == nil {
				var response []OutgoingEntry
				err = json.Unmarshal(msg.Data, &response)
				assert.NoError(t, err)
			}

			done <- true
		}(i)
	}

	// Wait for all queries to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Query completed
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent query timed out")
		}
	}
}

// TestQueryNotFound_Integration tests not found scenarios
func TestQueryNotFound_Integration(t *testing.T) {
	_, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	nc := natsClient.GetConnection()

	tests := []struct {
		name    string
		subject string
		request any
	}{
		{
			name:    "outgoing not found",
			subject: "graph.index.query.outgoing",
			request: map[string]string{"entity_id": "non.existent.entity"},
		},
		{
			name:    "incoming not found",
			subject: "graph.index.query.incoming",
			request: map[string]string{"entity_id": "non.existent.entity"},
		},
		{
			name:    "alias not found",
			subject: "graph.index.query.alias",
			request: map[string]string{"alias": "non-existent-alias"},
		},
		{
			name:    "predicate not found",
			subject: "graph.index.query.predicate",
			request: map[string]string{"predicate": "non.existent.predicate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestJSON, err := json.Marshal(tt.request)
			require.NoError(t, err)

			// Send query request
			msg, err := nc.Request(tt.subject, requestJSON, 2*time.Second)
			require.NoError(t, err)

			// Should get either empty array or error
			var responseMap map[string]any
			err = json.Unmarshal(msg.Data, &responseMap)
			require.NoError(t, err)

			// Either has error field or empty entities/results
			if errorMsg, hasError := responseMap["error"]; hasError {
				assert.NotEmpty(t, errorMsg)
			} else {
				// Should have empty array response
				t.Logf("Response: %v", responseMap)
			}
		})
	}
}

// TestQueryInvalidRequest_Integration tests invalid request handling
func TestQueryInvalidRequest_Integration(t *testing.T) {
	_, natsClient, cleanup := setupIntegrationTest(t)
	defer cleanup()

	nc := natsClient.GetConnection()

	tests := []struct {
		name    string
		subject string
		request []byte
	}{
		{
			name:    "outgoing malformed JSON",
			subject: "graph.index.query.outgoing",
			request: []byte(`{invalid json}`),
		},
		{
			name:    "outgoing empty entity_id",
			subject: "graph.index.query.outgoing",
			request: []byte(`{"entity_id": ""}`),
		},
		{
			name:    "incoming empty entity_id",
			subject: "graph.index.query.incoming",
			request: []byte(`{"entity_id": ""}`),
		},
		{
			name:    "alias empty alias",
			subject: "graph.index.query.alias",
			request: []byte(`{"alias": ""}`),
		},
		{
			name:    "predicate empty predicate",
			subject: "graph.index.query.predicate",
			request: []byte(`{"predicate": ""}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Send query request
			msg, err := nc.Request(tt.subject, tt.request, 2*time.Second)
			require.NoError(t, err)

			// Should get error response
			var response map[string]string
			err = json.Unmarshal(msg.Data, &response)
			require.NoError(t, err)

			assert.Contains(t, response, "error")
			assert.NotEmpty(t, response["error"])
		})
	}
}

// natsMsg wraps nats.Msg to implement queryMsg interface
type natsMsg struct {
	*nats.Msg
}

func (n *natsMsg) Data() []byte {
	return n.Msg.Data
}

func (n *natsMsg) Respond(data []byte) error {
	return n.Msg.Respond(data)
}
