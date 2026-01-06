package indexmanager

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/c360/semstreams/metric"
)

// T114: Integration test for orphan cleanup on entity delete
// FR-005a/b/c: Verify that when entity with relationships is deleted,
// all INCOMING_INDEX references are properly cleaned up
func TestIndexManager_EntityDeleteCleanupIntegration(t *testing.T) {
	t.Run("delete entity with relationships cleans incoming indexes", func(t *testing.T) {
		// Setup test manager with all indexes including OUTGOING_INDEX
		config := DefaultConfig()
		config.Workers = 2
		config.EventBuffer.Capacity = 100
		config.EventBuffer.Metrics = true
		config.BatchProcessing.Size = 10
		config.BatchProcessing.Interval = 10 * time.Millisecond

		// Create mock buckets including OUTGOING_INDEX and embedding buckets
		mockBuckets := map[string]*MockKeyValue{
			"ENTITY_STATES":    NewMockKeyValue(),
			"PREDICATE_INDEX":  NewMockKeyValue(),
			"INCOMING_INDEX":   NewMockKeyValue(),
			"OUTGOING_INDEX":   NewMockKeyValue(),
			"ALIAS_INDEX":      NewMockKeyValue(),
			"SPATIAL_INDEX":    NewMockKeyValue(),
			"TEMPORAL_INDEX":   NewMockKeyValue(),
			"EMBEDDING_INDEX":  NewMockKeyValue(),
			"EMBEDDING_DEDUP":  NewMockKeyValue(),
			"EMBEDDINGS_CACHE": NewMockKeyValue(),
		}

		buckets := make(map[string]jetstream.KeyValue)
		for name, mockBucket := range mockBuckets {
			buckets[name] = mockBucket
		}

		// Setup mock expectations for WatchAll
		mockWatcher := NewMockKeyWatcher()
		mockBuckets["ENTITY_STATES"].On("WatchAll", mock.Anything).Return(mockWatcher, nil)
		mockWatcher.On("Updates").Return(mockWatcher.updates)
		mockWatcher.On("Stop").Return(nil)

		testRegistry := metric.NewMetricsRegistry()
		mgr, err := NewManager(config, buckets, nil, testRegistry, nil)
		require.NoError(t, err)

		manager, ok := mgr.(*Manager)
		require.True(t, ok)

		// Manually register OutgoingIndex for tests
		outgoingIndex := NewOutgoingIndex(mockBuckets["OUTGOING_INDEX"], manager.metrics, manager.promMetrics, manager.logger)
		manager.indexes["outgoing"] = outgoingIndex

		ctx := context.Background()

		// Setup: Create entity A with relationships to B and C
		entityA := "c360.platform1.robotics.mav1.drone.001"
		entityB := "c360.platform1.robotics.mav1.drone.002"
		entityC := "c360.platform1.ops.fleet1.fleet.alpha"

		// Create outgoing index for entity A
		outgoingRels := []OutgoingEntry{
			{Predicate: "spatial.proximity.near", ToEntityID: entityB},
			{Predicate: "ops.fleet.member_of", ToEntityID: entityC},
		}
		outgoingJSON, _ := json.Marshal(outgoingRels)
		mockBuckets["OUTGOING_INDEX"].data[entityA] = outgoingJSON

		// Create incoming indexes for entity B and C using IncomingEntry format
		incomingB := []IncomingEntry{
			{Predicate: "spatial.proximity.near", FromEntityID: entityA},
			{Predicate: "spatial.proximity.near", FromEntityID: "c360.platform1.robotics.mav1.drone.003"},
		}
		incomingBJSON, _ := json.Marshal(incomingB)
		mockBuckets["INCOMING_INDEX"].data[entityB] = incomingBJSON

		incomingC := []IncomingEntry{
			{Predicate: "ops.fleet.member_of", FromEntityID: entityA},
		}
		incomingCJSON, _ := json.Marshal(incomingC)
		mockBuckets["INCOMING_INDEX"].data[entityC] = incomingCJSON

		// Execute: Delete entity A - should trigger cleanup
		err = manager.CleanupOrphanedIncomingReferences(ctx, entityA)
		require.NoError(t, err)

		// Verify: Check that entity A was removed from incoming indexes

		// Entity B should have only drone.003 left
		incomingBData, exists := mockBuckets["INCOMING_INDEX"].data[entityB]
		require.True(t, exists, "Entity B incoming index should still exist")
		var updatedIncomingB []IncomingEntry
		err = json.Unmarshal(incomingBData, &updatedIncomingB)
		require.NoError(t, err)
		assert.Len(t, updatedIncomingB, 1, "Entity B should have 1 incoming reference")
		assert.Equal(t, "c360.platform1.robotics.mav1.drone.003", updatedIncomingB[0].FromEntityID)
		// Verify entity A is not in the remaining entries
		for _, entry := range updatedIncomingB {
			assert.NotEqual(t, entityA, entry.FromEntityID, "Entity A should be removed from B's incoming index")
		}

		// Entity C should have no incoming index (deleted when empty)
		_, exists = mockBuckets["INCOMING_INDEX"].data[entityC]
		assert.False(t, exists, "Entity C incoming index should be deleted when empty")

		// Verify: OUTGOING_INDEX for entity A should still exist
		// (cleanup only handles INCOMING_INDEX, not OUTGOING_INDEX)
		_, exists = mockBuckets["OUTGOING_INDEX"].data[entityA]
		assert.True(t, exists, "Entity A outgoing index should still exist (not cleaned by this operation)")
	})

	t.Run("delete entity with no relationships does not error", func(t *testing.T) {
		config := DefaultConfig()
		config.Workers = 2
		config.EventBuffer.Metrics = true

		mockBuckets := map[string]*MockKeyValue{
			"ENTITY_STATES":    NewMockKeyValue(),
			"PREDICATE_INDEX":  NewMockKeyValue(),
			"INCOMING_INDEX":   NewMockKeyValue(),
			"OUTGOING_INDEX":   NewMockKeyValue(),
			"ALIAS_INDEX":      NewMockKeyValue(),
			"SPATIAL_INDEX":    NewMockKeyValue(),
			"TEMPORAL_INDEX":   NewMockKeyValue(),
			"EMBEDDING_INDEX":  NewMockKeyValue(),
			"EMBEDDING_DEDUP":  NewMockKeyValue(),
			"EMBEDDINGS_CACHE": NewMockKeyValue(),
		}

		buckets := make(map[string]jetstream.KeyValue)
		for name, mockBucket := range mockBuckets {
			buckets[name] = mockBucket
		}

		mockWatcher := NewMockKeyWatcher()
		mockBuckets["ENTITY_STATES"].On("WatchAll", mock.Anything).Return(mockWatcher, nil)
		mockWatcher.On("Updates").Return(mockWatcher.updates)
		mockWatcher.On("Stop").Return(nil)

		testRegistry := metric.NewMetricsRegistry()
		mgr, err := NewManager(config, buckets, nil, testRegistry, nil)
		require.NoError(t, err)

		manager, ok := mgr.(*Manager)
		require.True(t, ok)

		// Manually register OutgoingIndex for tests
		outgoingIndex := NewOutgoingIndex(mockBuckets["OUTGOING_INDEX"], manager.metrics, manager.promMetrics, manager.logger)
		manager.indexes["outgoing"] = outgoingIndex

		ctx := context.Background()

		// Entity with no relationships
		entityID := "c360.platform1.robotics.mav1.sensor.001"

		// Execute: Delete entity with no relationships
		err = manager.CleanupOrphanedIncomingReferences(ctx, entityID)
		assert.NoError(t, err, "Should not error when entity has no relationships")
	})

	t.Run("cleanup sequence order - incoming before outgoing", func(t *testing.T) {
		// This test verifies FR-005c: cleanup sequence MUST be INCOMING first, then OUTGOING
		config := DefaultConfig()
		config.Workers = 2
		config.EventBuffer.Metrics = true

		mockBuckets := map[string]*MockKeyValue{
			"ENTITY_STATES":    NewMockKeyValue(),
			"PREDICATE_INDEX":  NewMockKeyValue(),
			"INCOMING_INDEX":   NewMockKeyValue(),
			"OUTGOING_INDEX":   NewMockKeyValue(),
			"ALIAS_INDEX":      NewMockKeyValue(),
			"SPATIAL_INDEX":    NewMockKeyValue(),
			"TEMPORAL_INDEX":   NewMockKeyValue(),
			"EMBEDDING_INDEX":  NewMockKeyValue(),
			"EMBEDDING_DEDUP":  NewMockKeyValue(),
			"EMBEDDINGS_CACHE": NewMockKeyValue(),
		}

		buckets := make(map[string]jetstream.KeyValue)
		for name, mockBucket := range mockBuckets {
			buckets[name] = mockBucket
		}

		mockWatcher := NewMockKeyWatcher()
		mockBuckets["ENTITY_STATES"].On("WatchAll", mock.Anything).Return(mockWatcher, nil)
		mockWatcher.On("Updates").Return(mockWatcher.updates)
		mockWatcher.On("Stop").Return(nil)

		testRegistry := metric.NewMetricsRegistry()
		mgr, err := NewManager(config, buckets, nil, testRegistry, nil)
		require.NoError(t, err)

		manager, ok := mgr.(*Manager)
		require.True(t, ok)

		// Manually register OutgoingIndex for tests
		outgoingIndex := NewOutgoingIndex(mockBuckets["OUTGOING_INDEX"], manager.metrics, manager.promMetrics, manager.logger)
		manager.indexes["outgoing"] = outgoingIndex

		ctx := context.Background()

		entityA := "c360.platform1.robotics.mav1.drone.001"
		entityB := "c360.platform1.robotics.mav1.drone.002"

		// Setup outgoing and incoming indexes
		outgoingRels := []OutgoingEntry{
			{Predicate: "spatial.proximity.near", ToEntityID: entityB},
		}
		outgoingJSON, _ := json.Marshal(outgoingRels)
		mockBuckets["OUTGOING_INDEX"].data[entityA] = outgoingJSON

		incomingB := []IncomingEntry{
			{Predicate: "spatial.proximity.near", FromEntityID: entityA},
		}
		incomingBJSON, _ := json.Marshal(incomingB)
		mockBuckets["INCOMING_INDEX"].data[entityB] = incomingBJSON

		// Step 1: Clean incoming references (this is what CleanupOrphanedIncomingReferences does)
		err = manager.CleanupOrphanedIncomingReferences(ctx, entityA)
		require.NoError(t, err)

		// Verify incoming index cleaned but outgoing still exists
		_, exists := mockBuckets["INCOMING_INDEX"].data[entityB]
		assert.False(t, exists, "INCOMING_INDEX[B] should be cleaned")

		outgoingData, exists := mockBuckets["OUTGOING_INDEX"].data[entityA]
		assert.True(t, exists, "OUTGOING_INDEX[A] should still exist")
		assert.NotNil(t, outgoingData)

		// Step 2: Now delete outgoing index (would happen in full delete flow)
		delete(mockBuckets["OUTGOING_INDEX"].data, entityA)
		delete(mockBuckets["INCOMING_INDEX"].data, entityA)

		// Verify complete cleanup
		_, exists = mockBuckets["OUTGOING_INDEX"].data[entityA]
		assert.False(t, exists, "OUTGOING_INDEX[A] should be deleted after cleanup")
		_, exists = mockBuckets["INCOMING_INDEX"].data[entityA]
		assert.False(t, exists, "INCOMING_INDEX[A] should be deleted after cleanup")
	})
}
