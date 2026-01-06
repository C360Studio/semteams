//go:build integration

package indexmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

func TestIntegration_SpatialIndex_ProcessesTriples(t *testing.T) {
	// Use real NATS with testcontainers
	natsClient := getSharedNATSClient(t)
	ctx := context.Background()

	// Create real KV bucket for spatial index
	spatialBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "TEST_SPATIAL_INDEX",
		Description: "Test spatial index bucket",
		History:     10,
	})
	require.NoError(t, err)

	// Create SpatialIndex with real dependencies
	metrics := &InternalMetrics{}
	spatialIndex := NewSpatialIndex(spatialBucket, natsClient, metrics, nil, nil)

	// Test entity with PROPER triples (not properties) - triples are single source of truth
	entity := &gtypes.EntityState{
		ID: "c360.platform1.robotics.test.drone.0",
		Triples: []message.Triple{
			{
				Subject:   "c360.platform1.robotics.test.drone.0",
				Predicate: "geo.location.latitude",
				Object:    37.7749,
				Source:    "test_position",
				Timestamp: time.Now(),
			},
			{
				Subject:   "c360.platform1.robotics.test.drone.0",
				Predicate: "geo.location.longitude",
				Object:    -122.4194,
				Source:    "test_position",
				Timestamp: time.Now(),
			},
			{
				Subject:   "c360.platform1.robotics.test.drone.0",
				Predicate: "geo.location.altitude",
				Object:    100.0,
				Source:    "test_position",
				Timestamp: time.Now(),
			},
		},
	}

	// Process through spatial index
	err = spatialIndex.HandleCreate(ctx, "c360.platform1.robotics.test.drone.0", entity)
	require.NoError(t, err)

	// Verify real KV bucket contents
	entries := getAllKVEntries(t, spatialBucket, "*")
	assert.Equal(t, 1, len(entries), "Should have exactly one geohash entry")

	// Verify geohash structure
	if len(entries) > 0 {
		var spatialData map[string]interface{}
		err = json.Unmarshal(entries[0].Value(), &spatialData)
		require.NoError(t, err)

		// Check entities map
		entitiesRaw, ok := spatialData["entities"]
		require.True(t, ok, "Spatial data should have entities map")

		entities, ok := entitiesRaw.(map[string]interface{})
		require.True(t, ok, "Entities should be a map")

		// Verify our drone is in the spatial index
		droneData, ok := entities["c360.platform1.robotics.test.drone.0"]
		require.True(t, ok, "Drone should be in spatial index")

		droneMap, ok := droneData.(map[string]interface{})
		require.True(t, ok, "Drone data should be a map")

		// Verify coordinates
		assert.Equal(t, 37.7749, droneMap["lat"])
		assert.Equal(t, -122.4194, droneMap["lon"])
		assert.Equal(t, 100.0, droneMap["alt"])
	}
}

func TestIntegration_SpatialIndex_SkipsWithoutTriples(t *testing.T) {
	// Use real NATS with testcontainers
	natsClient := getSharedNATSClient(t)
	ctx := context.Background()

	// Create real KV bucket for spatial index
	spatialBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "TEST_SPATIAL_INDEX_EMPTY",
		Description: "Test spatial index bucket for empty case",
		History:     10,
	})
	require.NoError(t, err)

	// Create SpatialIndex with real dependencies
	metrics := &InternalMetrics{}
	spatialIndex := NewSpatialIndex(spatialBucket, natsClient, metrics, nil, nil)

	// Test entity with NO triples - spatial index should gracefully handle this
	entity := &gtypes.EntityState{
		ID:      "c360.platform1.robotics.test.drone.1",
		Triples: nil, // Entity with no triples should not be indexed
	}

	// Process through spatial index
	err = spatialIndex.HandleCreate(ctx, "c360.platform1.robotics.test.drone.1", entity)
	require.NoError(t, err)

	// Verify KV bucket is EMPTY (spatial index should skip entities without triples)
	entries := getAllKVEntries(t, spatialBucket, "*")
	assert.Equal(t, 0, len(entries), "Spatial index should be empty when entity has no geo triples")
}

func TestIntegration_SpatialIndex_ConfigurablePrecision(t *testing.T) {
	// Test different precision levels generate different geohashes
	testCases := []struct {
		precision int
		lat, lon  float64
		expected  string // Expected geohash pattern
	}{
		{4, 37.7749, -122.4194, "geo_4_"}, // ~2.5km bins
		{7, 37.7749, -122.4194, "geo_7_"}, // ~30m bins (default)
		{8, 37.7749, -122.4194, "geo_8_"}, // ~5m bins
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("precision_%d", tc.precision), func(t *testing.T) {
			spatialIndex := &SpatialIndex{precision: tc.precision}
			geohash := spatialIndex.calculateGeohash(tc.lat, tc.lon, tc.precision)
			assert.Contains(t, geohash, tc.expected, "Geohash should include precision level")
		})
	}
}

// TestSpatialIndex_RoundTrip verifies that entities stored via HandleCreate can be
// queried back via QueryBounds. This is the critical test that validates the geohash
// calculation is consistent between storage and query operations.
func TestIntegration_SpatialIndex_RoundTrip(t *testing.T) {
	natsClient := getSharedNATSClient(t)
	ctx := context.Background()

	// Create real KV bucket for spatial index
	spatialBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "TEST_SPATIAL_ROUNDTRIP",
		Description: "Test spatial index round-trip",
		History:     10,
	})
	require.NoError(t, err)

	// Create SpatialIndex with real dependencies
	metrics := &InternalMetrics{}
	spatialIndex := NewSpatialIndex(spatialBucket, natsClient, metrics, nil, nil)

	// Test coordinates in San Francisco (same as E2E test data)
	lat, lon := 37.7749, -122.4194
	entityID := "c360.logistics.environmental.sensor.temperature.roundtrip-001"

	// Create entity with geo triples
	entity := &gtypes.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{
				Subject:   entityID,
				Predicate: "geo.location.latitude",
				Object:    lat,
				Source:    "test",
				Timestamp: time.Now(),
			},
			{
				Subject:   entityID,
				Predicate: "geo.location.longitude",
				Object:    lon,
				Source:    "test",
				Timestamp: time.Now(),
			},
		},
	}

	// Store entity via HandleCreate
	err = spatialIndex.HandleCreate(ctx, entityID, entity)
	require.NoError(t, err)

	// Query with bounds containing the entity (small box around the point)
	bounds := Bounds{
		North: lat + 0.01, // ~1.1km north
		South: lat - 0.01, // ~1.1km south
		East:  lon + 0.01, // ~850m east
		West:  lon - 0.01, // ~850m west
	}

	results, err := spatialIndex.QueryBounds(ctx, bounds)
	require.NoError(t, err)

	// CRITICAL: Entity must be found - this validates geohash consistency
	assert.Contains(t, results, entityID, "Round-trip query must find stored entity")
	assert.Len(t, results, 1, "Should find exactly one entity")
}

// TestIntegration_SpatialIndex_QueryBounds_MultipleEntities tests querying multiple
// entities within a bounding box.
func TestIntegration_SpatialIndex_QueryBounds_MultipleEntities(t *testing.T) {
	natsClient := getSharedNATSClient(t)
	ctx := context.Background()

	// Create real KV bucket for spatial index
	spatialBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "TEST_SPATIAL_MULTI",
		Description: "Test spatial index multi-entity",
		History:     10,
	})
	require.NoError(t, err)

	// Create SpatialIndex with real dependencies
	metrics := &InternalMetrics{}
	spatialIndex := NewSpatialIndex(spatialBucket, natsClient, metrics, nil, nil)

	// Create multiple entities in a small area (SF downtown)
	testEntities := []struct {
		id       string
		lat, lon float64
	}{
		{"sensor-001", 37.7749, -122.4194}, // Union Square
		{"sensor-002", 37.7751, -122.4185}, // ~100m east
		{"sensor-003", 37.7745, -122.4200}, // ~100m southwest
		{"sensor-004", 37.8000, -122.4000}, // ~3km north (outside bounds)
	}

	// Store all entities
	for _, te := range testEntities {
		entity := &gtypes.EntityState{
			ID: te.id,
			Triples: []message.Triple{
				{Subject: te.id, Predicate: "geo.location.latitude", Object: te.lat, Source: "test", Timestamp: time.Now()},
				{Subject: te.id, Predicate: "geo.location.longitude", Object: te.lon, Source: "test", Timestamp: time.Now()},
			},
		}
		err := spatialIndex.HandleCreate(ctx, te.id, entity)
		require.NoError(t, err)
	}

	// Query bounds around Union Square (should include first 3, exclude 4th)
	bounds := Bounds{
		North: 37.776,   // ~100m north of center
		South: 37.774,   // ~100m south of center
		East:  -122.418, // ~100m east
		West:  -122.421, // ~100m west
	}

	results, err := spatialIndex.QueryBounds(ctx, bounds)
	require.NoError(t, err)

	// Should find 3 entities in bounds, not the 4th
	assert.Len(t, results, 3, "Should find exactly 3 entities within bounds")
	assert.Contains(t, results, "sensor-001")
	assert.Contains(t, results, "sensor-002")
	assert.Contains(t, results, "sensor-003")
	assert.NotContains(t, results, "sensor-004", "Entity outside bounds should not be returned")
}

// TestIntegration_SpatialIndex_QueryBounds_EmptyArea tests querying an area with no entities.
func TestIntegration_SpatialIndex_QueryBounds_EmptyArea(t *testing.T) {
	natsClient := getSharedNATSClient(t)
	ctx := context.Background()

	// Create real KV bucket for spatial index
	spatialBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "TEST_SPATIAL_EMPTY_QUERY",
		Description: "Test spatial index empty query",
		History:     10,
	})
	require.NoError(t, err)

	// Create SpatialIndex with real dependencies
	metrics := &InternalMetrics{}
	spatialIndex := NewSpatialIndex(spatialBucket, natsClient, metrics, nil, nil)

	// Store entity in San Francisco
	entity := &gtypes.EntityState{
		ID: "sf-entity",
		Triples: []message.Triple{
			{Subject: "sf-entity", Predicate: "geo.location.latitude", Object: 37.7749, Source: "test", Timestamp: time.Now()},
			{Subject: "sf-entity", Predicate: "geo.location.longitude", Object: -122.4194, Source: "test", Timestamp: time.Now()},
		},
	}
	err = spatialIndex.HandleCreate(ctx, "sf-entity", entity)
	require.NoError(t, err)

	// Query bounds in New York (no entities there)
	bounds := Bounds{
		North: 40.76,
		South: 40.74,
		East:  -73.97,
		West:  -73.99,
	}

	results, err := spatialIndex.QueryBounds(ctx, bounds)
	require.NoError(t, err)

	// Should return empty slice, not error
	assert.Empty(t, results, "Query in empty area should return empty slice")
}

// Helper function to get all KV entries for testing
func getAllKVEntries(t *testing.T, bucket jetstream.KeyValue, _ string) []jetstream.KeyValueEntry {
	ctx := context.Background()
	watcher, err := bucket.WatchAll(ctx)
	require.NoError(t, err)
	defer watcher.Stop()

	var entries []jetstream.KeyValueEntry

	// Collect initial entries
	for entry := range watcher.Updates() {
		if entry == nil {
			break // End of initial entries
		}
		entries = append(entries, entry)
	}

	return entries
}
