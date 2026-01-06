//go:build integration

package indexmanager

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
)

// TestOutgoingIndex_Integration tests OUTGOING_INDEX with real NATS
func TestOutgoingIndex_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Create real NATS client with testcontainers
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	ctx := context.Background()

	// Create real KV bucket for outgoing index
	outgoingBucket, err := testClient.CreateKVBucket(ctx, "OUTGOING_INDEX")
	require.NoError(t, err, "Failed to create OUTGOING_INDEX bucket")

	// Create OutgoingIndex with real NATS
	outgoingIndex := NewOutgoingIndex(outgoingBucket, nil, nil, nil)
	require.NotNil(t, outgoingIndex, "outgoing index should be created")

	t.Run("create_entity_with_relationships", func(t *testing.T) {
		// Create entity with multiple outgoing relationships
		entityID := "acme.telemetry.robotics.gcs1.drone.001"
		state := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				// Relationship triple: fleet membership
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				// Relationship triple: operator
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
				// Property triple (not a relationship)
				{
					Subject:   entityID,
					Predicate: "robotics.battery.level",
					Object:    85.5,
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		// HandleCreate - should index only relationship triples
		err := outgoingIndex.HandleCreate(ctx, entityID, state)
		require.NoError(t, err)

		// Verify we can retrieve the outgoing relationships
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 2, "should have 2 outgoing relationships (property triple excluded)")

		// Verify specific relationships
		expectedRels := map[string]string{
			"ops.fleet.member_of":             "acme.ops.logistics.hq.fleet.rescue",
			"robotics.operator.controlled_by": "acme.platform.auth.main.user.alice",
		}

		foundRels := make(map[string]string)
		for _, entry := range outgoing {
			foundRels[entry.Predicate] = entry.ToEntityID
		}

		for pred, targetID := range expectedRels {
			actualTarget, found := foundRels[pred]
			assert.True(t, found, "should find relationship with predicate: %s", pred)
			assert.Equal(t, targetID, actualTarget, "target entity should match for predicate: %s", pred)
		}

		// Verify direct bucket access shows correct storage
		bucketEntry, err := outgoingBucket.Get(ctx, entityID)
		require.NoError(t, err)
		assert.NotNil(t, bucketEntry.Value(), "bucket entry should have value")
	})

	t.Run("query_by_predicate", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs2.drone.002"
		state := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.search",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs2.drone.003",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs2.drone.004",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, state)
		require.NoError(t, err)

		// Query by specific predicate
		proximityRels, err := outgoingIndex.GetOutgoingByPredicate(ctx, entityID, "spatial.proximity.near")
		require.NoError(t, err)
		assert.Len(t, proximityRels, 2, "should have 2 proximity relationships")

		fleetRels, err := outgoingIndex.GetOutgoingByPredicate(ctx, entityID, "ops.fleet.member_of")
		require.NoError(t, err)
		assert.Len(t, fleetRels, 1, "should have 1 fleet relationship")

		// Query for non-existent predicate
		nonExistent, err := outgoingIndex.GetOutgoingByPredicate(ctx, entityID, "does.not.exist")
		require.NoError(t, err)
		assert.Len(t, nonExistent, 0, "should have 0 relationships for non-existent predicate")
	})

	t.Run("update_add_relationships", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs3.drone.005"

		// Initial state with 1 relationship
		initialState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, initialState)
		require.NoError(t, err)

		// Verify initial state
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 1)

		// Update: add more relationships
		updatedState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				// Keep existing
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				// Add new
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.bob",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs3.drone.006",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err = outgoingIndex.HandleUpdate(ctx, entityID, updatedState)
		require.NoError(t, err)

		// Verify all 3 relationships now exist
		outgoing, err = outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 3, "should have 3 relationships after update")
	})

	t.Run("update_remove_relationships", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs4.drone.007"

		// Initial state with 3 relationships
		initialState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.charlie",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs4.drone.008",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, initialState)
		require.NoError(t, err)

		// Verify initial state
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 3)

		// Update: keep only 1 relationship
		updatedState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err = outgoingIndex.HandleUpdate(ctx, entityID, updatedState)
		require.NoError(t, err)

		// Verify only 1 relationship remains
		outgoing, err = outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 1, "should have 1 relationship after update")
		assert.Equal(t, "ops.fleet.member_of", outgoing[0].Predicate)
	})

	t.Run("update_change_target", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs5.drone.009"

		// Initial state
		initialState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.dave",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, initialState)
		require.NoError(t, err)

		// Update: change target entity for same predicate
		updatedState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.eve", // Changed target
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err = outgoingIndex.HandleUpdate(ctx, entityID, updatedState)
		require.NoError(t, err)

		// Verify target entity changed
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 1)
		assert.Equal(t, "robotics.operator.controlled_by", outgoing[0].Predicate)
		assert.Equal(t, "acme.platform.auth.main.user.eve", outgoing[0].ToEntityID)
	})

	t.Run("update_empty_to_populated", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs6.drone.010"

		// Initial state: no relationships
		initialState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "robotics.battery.level",
					Object:    75.0, // Property only
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, initialState)
		require.NoError(t, err)

		// Verify empty result (no relationships) - not-found returns empty slice, not error
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		assert.NoError(t, err, "not-found is not an error")
		assert.Empty(t, outgoing, "should return empty slice for entity with no relationships")

		// Update: add relationships
		updatedState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err = outgoingIndex.HandleUpdate(ctx, entityID, updatedState)
		require.NoError(t, err)

		// Verify relationships now indexed
		outgoing, err = outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 1)
	})

	t.Run("update_populated_to_empty", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs7.drone.011"

		// Initial state with relationships
		initialState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.frank",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, initialState)
		require.NoError(t, err)

		// Verify initial relationships
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 2)

		// Update: remove all relationships
		updatedState := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "robotics.battery.level",
					Object:    90.0, // Property only
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err = outgoingIndex.HandleUpdate(ctx, entityID, updatedState)
		require.NoError(t, err)

		// Verify empty result after all relationships removed - not-found returns empty slice, not error
		outgoing, err = outgoingIndex.GetOutgoing(ctx, entityID)
		assert.NoError(t, err, "not-found is not an error")
		assert.Empty(t, outgoing, "should return empty slice after all relationships removed")
	})

	t.Run("delete_entity_cleanup", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.gcs8.drone.012"

		// Create entity with relationships
		state := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   entityID,
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.grace",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		err := outgoingIndex.HandleCreate(ctx, entityID, state)
		require.NoError(t, err)

		// Verify relationships exist
		outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
		require.NoError(t, err)
		assert.Len(t, outgoing, 2)

		// Delete entity
		err = outgoingIndex.HandleDelete(ctx, entityID)
		require.NoError(t, err)

		// Verify empty result after delete - not-found returns empty slice, not error
		outgoing, err = outgoingIndex.GetOutgoing(ctx, entityID)
		assert.NoError(t, err, "not-found is not an error")
		assert.Empty(t, outgoing, "should return empty slice after delete")

		// Verify bucket entry removed (low-level check)
		_, err = outgoingBucket.Get(ctx, entityID)
		assert.Error(t, err, "bucket entry should be removed")
		assert.Equal(t, jetstream.ErrKeyNotFound, err, "should return key not found error")
	})

	t.Run("concurrent_operations", func(t *testing.T) {
		// Test concurrent creates on different entities
		numEntities := 5
		done := make(chan bool, numEntities)

		for i := 0; i < numEntities; i++ {
			go func(index int) {
				defer func() { done <- true }()

				entityID := "acme.telemetry.robotics.concurrent." + string(rune('a'+index))
				state := &gtypes.EntityState{
					ID: entityID,
					Triples: []message.Triple{
						{
							Subject:   entityID,
							Predicate: "ops.fleet.member_of",
							Object:    "acme.ops.logistics.hq.fleet.concurrent",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
					UpdatedAt: time.Now(),
				}

				err := outgoingIndex.HandleCreate(context.Background(), entityID, state)
				if err != nil {
					t.Errorf("concurrent create failed for entity %s: %v", entityID, err)
				}
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numEntities; i++ {
			<-done
		}

		// Verify all entities were indexed
		for i := 0; i < numEntities; i++ {
			entityID := "acme.telemetry.robotics.concurrent." + string(rune('a'+i))
			outgoing, err := outgoingIndex.GetOutgoing(ctx, entityID)
			assert.NoError(t, err, "should find entity %s", entityID)
			if err == nil {
				assert.Len(t, outgoing, 1, "entity %s should have 1 relationship", entityID)
			}
		}
	})

	t.Run("context_cancellation", func(t *testing.T) {
		entityID := "acme.telemetry.robotics.cancel.drone.001"
		state := &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{
					Subject:   entityID,
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			UpdatedAt: time.Now(),
		}

		// Create with cancelled context
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := outgoingIndex.HandleCreate(cancelledCtx, entityID, state)
		assert.Error(t, err, "should fail with cancelled context")
		assert.Equal(t, context.Canceled, err, "should return context.Canceled error")
	})

	t.Run("get_nonexistent_entity", func(t *testing.T) {
		nonExistentID := "acme.telemetry.robotics.does.not.exist"

		// Not-found returns empty slice, not error (same convention as IncomingIndex)
		outgoing, err := outgoingIndex.GetOutgoing(ctx, nonExistentID)
		assert.NoError(t, err, "not-found is not an error")
		assert.Empty(t, outgoing, "should return empty slice for non-existent entity")
	})
}
