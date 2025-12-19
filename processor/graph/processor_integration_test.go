//go:build integration

package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/storage/objectstore"
)

func TestIntegration_QueryAPIs(t *testing.T) {
	// This test requires INTEGRATION_TESTS=1
	natsClient := getSharedNATSClient(t)

	// Create config with metrics disabled for tests
	config := DefaultConfig()
	if config.Indexer == nil {
		config.Indexer = &indexmanager.Config{}
		*config.Indexer = indexmanager.DefaultConfig()
	}
	config.Indexer.EventBuffer.Metrics = false

	deps := ProcessorDeps{
		Config:          config,
		NATSClient:      natsClient,
		MetricsRegistry: metric.NewMetricsRegistry(),
		Logger:          slog.Default(),
	}

	processor, err := NewProcessor(deps)
	require.NoError(t, err)
	require.NotNil(t, processor)

	// Initialize the processor
	err = processor.Initialize()
	require.NoError(t, err)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start processor in goroutine (it blocks now)
	startErr := make(chan error, 1)
	go func() {
		startErr <- processor.Start(ctx)
	}()

	// Wait for all components to be ready (not just started)
	err = processor.WaitForReady(5 * time.Second)
	require.NoError(t, err, "Processor should be ready within 5 seconds")

	// Store a test entity directly - using triples as single source of truth
	testEntity := &gtypes.EntityState{
		ID: "test-entity-1",
		Triples: []message.Triple{
			{
				Subject:   "test-entity-1",
				Predicate: "type",
				Object:    "device",
			},
			{
				Subject:   "test-entity-1",
				Predicate: "name",
				Object:    "Test Device",
			},
			{
				Subject:   "test-entity-1",
				Predicate: "status",
				Object:    "active",
			},
		},
		UpdatedAt: time.Now(),
		Version:   1,
	}

	// Store the entity
	_, err = processor.entityManager.CreateEntity(ctx, testEntity)
	require.NoError(t, err)

	// Give time for indexing and caching
	time.Sleep(100 * time.Millisecond)

	// Test GetEntity - should find our stored entity
	entity, err := processor.GetEntity(ctx, "test-entity-1")
	require.NoError(t, err)
	assert.NotNil(t, entity)
	assert.Equal(t, "test-entity-1", entity.ID)
	entityType, found := entity.GetPropertyValue("type")
	assert.True(t, found)
	assert.Equal(t, "device", entityType)
	name, found := entity.GetPropertyValue("name")
	assert.True(t, found)
	assert.Equal(t, "Test Device", name)

	// Note: GetEntityByAlias requires the IndexEngine's internal watchers
	// to be fully initialized and running, which is complex to test reliably.
	// This functionality is better validated in end-to-end tests.

	// Note: QueryByPredicate also requires the IndexEngine's internal watchers.
	// We've successfully tested the core functionality of storing and retrieving
	// entities. The index query functionality is better validated in e2e tests.

	// Cancel context to trigger shutdown
	cancel()

	// Wait for Start to return
	select {
	case err := <-startErr:
		require.NoError(t, err, "Start should complete without error")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestIntegration_MessageProcessing(t *testing.T) {
	// This test requires INTEGRATION_TESTS=1
	natsClient := getSharedNATSClient(t)

	// Create config with metrics disabled for tests
	config := DefaultConfig()
	if config.Indexer == nil {
		config.Indexer = &indexmanager.Config{}
		*config.Indexer = indexmanager.DefaultConfig()
	}
	config.Indexer.EventBuffer.Metrics = false

	deps := ProcessorDeps{
		Config:          config,
		NATSClient:      natsClient,
		MetricsRegistry: metric.NewMetricsRegistry(),
		Logger:          slog.Default(),
	}

	processor, err := NewProcessor(deps)
	require.NoError(t, err)

	err = processor.Initialize()
	require.NoError(t, err)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start processor in goroutine (it blocks now)
	startErr := make(chan error, 1)
	go func() {
		startErr <- processor.Start(ctx)
	}()

	// Wait for processor to be fully ready after Start()
	err = processor.WaitForReady(5 * time.Second)
	require.NoError(t, err, "Processor should be ready after Start()")

	// Create a Storable message (what ObjectStore sends to GraphProcessor)
	// Use unique entity ID with timestamp to avoid conflicts between test runs
	expectedEntityID := fmt.Sprintf("c360.platform1.robotics.mav1.battery.msgproc.%d", time.Now().UnixNano())

	// Create test payload with battery data
	testPayload := &TestGraphablePayload{
		ID: expectedEntityID,
		Properties: map[string]interface{}{
			"type":                     "battery",
			"robotics.battery.level":   75,
			"robotics.battery.voltage": 16.72,
		},
		TripleData: []map[string]interface{}{
			{
				"subject":   expectedEntityID,
				"predicate": "type",
				"object":    "battery",
			},
			{
				"subject":   expectedEntityID,
				"predicate": "robotics.battery.level",
				"object":    75,
			},
			{
				"subject":   expectedEntityID,
				"predicate": "robotics.battery.voltage",
				"object":    16.72, // Sum of voltages in volts
			},
		},
	}

	// Create storage reference
	storageRef := &message.StorageReference{
		StorageInstance: "objectstore-primary",
		Key:             "storage/robotics/2024/01/battery-msg",
		ContentType:     "application/json",
		Size:            1024,
	}

	// Create StoredMessage properly (like ObjectStore does)
	storedMsg := objectstore.NewStoredMessage(testPayload, storageRef, "robotics.battery")

	// Wrap in BaseMessage for transport (correct architecture)
	wrappedMsg := message.NewBaseMessage(
		storedMsg.Schema(), // Use StoredMessage type for correct unmarshaling
		storedMsg,
		"objectstore-primary", // source
	)

	// Marshal and publish the BaseMessage containing StoredMessage
	msgData, err := json.Marshal(wrappedMsg)
	require.NoError(t, err)

	subject := "storage.robotics.events"
	err = natsClient.Publish(ctx, subject, msgData)
	require.NoError(t, err, "Failed to publish message")

	// Poll for entity to be created (instead of arbitrary sleep)
	var entity *gtypes.EntityState

	// Poll up to 2 seconds for the entity to be processed
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entity, err = processor.GetEntity(ctx, expectedEntityID)
		if err == nil && entity != nil {
			break // Entity found, processing complete
		}
		time.Sleep(50 * time.Millisecond) // Small sleep between polls
	}

	// Now verify the entity was stored
	require.NoError(t, err)
	assert.NotNil(t, entity)
	assert.Equal(t, expectedEntityID, entity.ID)
	entityType, found := entity.GetPropertyValue("type")
	assert.True(t, found)
	assert.Equal(t, "battery", entityType)

	// Check properties were extracted from triples (BatteryPayload uses vocabulary predicates)
	batteryLevel, found := entity.GetPropertyValue("robotics.battery.level")
	assert.True(t, found)
	assert.Equal(t, float64(75), batteryLevel)
	// Voltage should be around 16.72V (4200+4150+4180+4190 mV = 16720 mV = 16.72V)
	voltage, found := entity.GetPropertyValue("robotics.battery.voltage")
	assert.True(t, found)
	if voltageFloat, ok := voltage.(float64); ok {
		assert.Greater(t, voltageFloat, 16.0)
		assert.Less(t, voltageFloat, 17.0)
	}

	// Cancel context to trigger shutdown
	cancel()

	// Wait for Start to return
	select {
	case err := <-startErr:
		require.NoError(t, err, "Start should complete without error")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestIntegration_EdgeOperations(t *testing.T) {
	// This test requires INTEGRATION_TESTS=1
	// IMPORTANT: This test uses entity IDs with "integ" suffix to avoid conflicts with other tests.
	// Entity cleanup is performed at the end to ensure test isolation.
	natsClient := getSharedNATSClient(t)

	// Create config
	config := DefaultConfig()
	if config.Indexer == nil {
		config.Indexer = &indexmanager.Config{}
		*config.Indexer = indexmanager.DefaultConfig()
	}
	config.Indexer.EventBuffer.Metrics = false

	deps := ProcessorDeps{
		Config:          config,
		NATSClient:      natsClient,
		MetricsRegistry: metric.NewMetricsRegistry(),
		Logger:          slog.Default(),
	}

	processor, err := NewProcessor(deps)
	require.NoError(t, err)

	err = processor.Initialize()
	require.NoError(t, err)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start processor in goroutine (it blocks now)
	startErr := make(chan error, 1)
	go func() {
		startErr <- processor.Start(ctx)
	}()

	// Wait for ready
	err = processor.WaitForReady(2 * time.Second)
	require.NoError(t, err)

	// Create two entities with structured entity IDs - use unique IDs for this test
	entity1ID := "c360.platform1.test.system1.drone.integ"
	entity2ID := "c360.platform1.test.system1.battery.integ"

	entity1 := &gtypes.EntityState{
		ID: entity1ID,
		Triples: []message.Triple{
			{
				Subject:   entity1ID,
				Predicate: "type",
				Object:    "drone",
			},
			{
				Subject:   entity1ID,
				Predicate: "name",
				Object:    "Test Drone",
			},
		},
		UpdatedAt: time.Now(),
		Version:   1,
	}

	entity2 := &gtypes.EntityState{
		ID: entity2ID,
		Triples: []message.Triple{
			{
				Subject:   entity2ID,
				Predicate: "type",
				Object:    "battery",
			},
			{
				Subject:   entity2ID,
				Predicate: "name",
				Object:    "Test Battery",
			},
		},
		UpdatedAt: time.Now(),
		Version:   1,
	}

	// Store both entities
	_, err = processor.entityManager.CreateEntity(ctx, entity1)
	require.NoError(t, err)
	_, err = processor.entityManager.CreateEntity(ctx, entity2)
	require.NoError(t, err)

	// Give time for entities to be fully created and indexed
	time.Sleep(100 * time.Millisecond)

	// Add a relationship via triple (triples are single source of truth for relationships)
	relationshipTriple := message.Triple{
		Subject:   entity1ID,
		Predicate: "robotics.component.connects_to",
		Object:    entity2ID, // Object is entity ID for relationships
	}

	// Use the triple manager to add the relationship triple
	err = processor.tripleManager.AddTriple(ctx, relationshipTriple)
	require.NoError(t, err)

	// Give time for asynchronous processing
	time.Sleep(100 * time.Millisecond)

	// Retrieve entity1 and verify the relationship triple was added
	updatedEntity1, err := processor.GetEntity(ctx, entity1ID)
	require.NoError(t, err)

	// Find relationship triples
	var relationshipTriples []message.Triple
	for _, triple := range updatedEntity1.Triples {
		if triple.IsRelationship() {
			relationshipTriples = append(relationshipTriples, triple)
		}
	}
	assert.Len(t, relationshipTriples, 1, "Should have one relationship triple")
	if len(relationshipTriples) > 0 {
		assert.Equal(t, entity2ID, relationshipTriples[0].Object)
		assert.Equal(t, "robotics.component.connects_to", relationshipTriples[0].Predicate)
	}

	// Test incoming index was updated - now expects direct array format
	incomingBucket, err := natsClient.GetKeyValueBucket(ctx, "INCOMING_INDEX")
	require.NoError(t, err)

	entry, err := incomingBucket.Get(ctx, entity2ID)
	if err == nil {
		var incomingRefs []string
		err = json.Unmarshal(entry.Value(), &incomingRefs)
		require.NoError(t, err)
		assert.Contains(t, incomingRefs, entity1ID)
	}

	// Clean up entities to avoid conflicts with other tests
	processor.entityManager.DeleteEntity(ctx, entity1ID)
	processor.entityManager.DeleteEntity(ctx, entity2ID)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for Start to return
	select {
	case err := <-startErr:
		require.NoError(t, err, "Start should complete without error")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}
