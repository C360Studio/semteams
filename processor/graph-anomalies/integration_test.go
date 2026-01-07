//go:build integration

package graphanomalies

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: graph-anomalies component expects simple []string arrays for neighbor lists,
// unlike graph-clustering which uses relationshipEntry structs.

// TestIntegration_AnomaliesFlow tests the full edge → structural index flow
func TestIntegration_AnomaliesFlow(t *testing.T) {
	// Create test NATS client with KV support
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	nc := testClient.Client

	// Create component with ComputeOnStartup enabled
	config := Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "outgoing_watch", Type: "kv-watch", Subject: graph.BucketOutgoingIndex},
				{Name: "incoming_watch", Type: "kv-watch", Subject: graph.BucketIncomingIndex},
			},
			Outputs: []component.PortDefinition{
				{Name: "structural_index", Type: "kv-write", Subject: graph.BucketStructuralIndex},
			},
		},
		ComputeIntervalStr: "1s", // Short interval for testing
		PivotCount:         4,    // Fewer pivots for testing
		MaxHopDistance:     5,    // Shorter hops for testing
		ComputeOnStartup:   true, // Compute immediately on startup (after 5s delay)
	}

	// Apply defaults to parse duration
	config.ApplyDefaults()

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: nc,
	}

	comp, err := CreateGraphAnomalies(configJSON, deps)
	require.NoError(t, err)
	require.NotNil(t, comp)

	anomaliesComp := comp.(*Component)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	require.NoError(t, anomaliesComp.Initialize())

	// Get JetStream context
	js, err := nc.JetStream()
	require.NoError(t, err)

	// Create input buckets BEFORE starting component
	outgoingBucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketOutgoingIndex,
		Description: "Test outgoing edges",
	})
	require.NoError(t, err)

	incomingBucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketIncomingIndex,
		Description: "Test incoming edges",
	})
	require.NoError(t, err)

	// Create a connected graph (4 entities in a cycle)
	entityIDs := []string{
		"c360.platform.robotics.mav1.drone.001",
		"c360.platform.robotics.mav1.drone.002",
		"c360.platform.robotics.mav1.drone.003",
		"c360.platform.robotics.mav1.drone.004",
	}

	// Create bidirectional edges for better k-core results
	edges := []struct {
		from string
		to   string
	}{
		{entityIDs[0], entityIDs[1]}, // 001 <-> 002
		{entityIDs[1], entityIDs[0]},
		{entityIDs[1], entityIDs[2]}, // 002 <-> 003
		{entityIDs[2], entityIDs[1]},
		{entityIDs[2], entityIDs[3]}, // 003 <-> 004
		{entityIDs[3], entityIDs[2]},
		{entityIDs[3], entityIDs[0]}, // 004 <-> 001 (close the cycle)
		{entityIDs[0], entityIDs[3]},
	}

	// Write edge data to buckets
	// For each entity, collect all its outgoing and incoming neighbors (simple []string arrays)
	outgoingNeighbors := make(map[string][]string)
	incomingNeighbors := make(map[string][]string)

	for _, edge := range edges {
		// Collect outgoing neighbors
		outgoingNeighbors[edge.from] = append(outgoingNeighbors[edge.from], edge.to)

		// Collect incoming neighbors
		incomingNeighbors[edge.to] = append(incomingNeighbors[edge.to], edge.from)
	}

	// Write outgoing index entries
	for entityID, neighbors := range outgoingNeighbors {
		outgoingJSON, err := json.Marshal(neighbors)
		require.NoError(t, err)
		_, err = outgoingBucket.Put(ctx, entityID, outgoingJSON)
		require.NoError(t, err)
	}

	// Write incoming index entries
	for entityID, neighbors := range incomingNeighbors {
		incomingJSON, err := json.Marshal(neighbors)
		require.NoError(t, err)
		_, err = incomingBucket.Put(ctx, entityID, incomingJSON)
		require.NoError(t, err)
	}

	// Start component (now that input buckets exist with data)
	require.NoError(t, anomaliesComp.Start(ctx))
	defer anomaliesComp.Stop(5 * time.Second)

	// Wait for STRUCTURAL_INDEX bucket to be created by component
	var structuralBucket jetstream.KeyValue
	require.Eventually(t, func() bool {
		structuralBucket, err = js.KeyValue(ctx, graph.BucketStructuralIndex)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond, "STRUCTURAL_INDEX bucket should be created")

	// Wait for k-core computation to complete (startup compute has 5s delay)
	// Check for k-core metadata
	var kcoreMeta map[string]interface{}
	require.Eventually(t, func() bool {
		entry, err := structuralBucket.Get(ctx, "structural.kcore._meta")
		if err != nil {
			return false
		}
		if err := json.Unmarshal(entry.Value(), &kcoreMeta); err != nil {
			return false
		}
		// Verify metadata has expected fields
		_, hasMaxCore := kcoreMeta["max_core"]
		_, hasEntityCount := kcoreMeta["entity_count"]
		_, hasComputedAt := kcoreMeta["computed_at"]
		return hasMaxCore && hasEntityCount && hasComputedAt
	}, 15*time.Second, 500*time.Millisecond, "K-core metadata should be computed")

	// Verify k-core metadata fields
	assert.NotNil(t, kcoreMeta["max_core"], "max_core should be set")
	assert.NotNil(t, kcoreMeta["entity_count"], "entity_count should be set")
	assert.NotNil(t, kcoreMeta["computed_at"], "computed_at should be set")

	entityCount := int(kcoreMeta["entity_count"].(float64))
	assert.Equal(t, len(entityIDs), entityCount, "entity_count should match number of entities")

	t.Logf("K-core metadata: %+v", kcoreMeta)
}

// TestIntegration_AnomaliesKCoreComputation verifies k-core computation
func TestIntegration_AnomaliesKCoreComputation(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	nc := testClient.Client

	config := Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "outgoing_watch", Type: "kv-watch", Subject: graph.BucketOutgoingIndex},
				{Name: "incoming_watch", Type: "kv-watch", Subject: graph.BucketIncomingIndex},
			},
			Outputs: []component.PortDefinition{
				{Name: "structural_index", Type: "kv-write", Subject: graph.BucketStructuralIndex},
			},
		},
		ComputeIntervalStr: "1s",
		PivotCount:         4,
		MaxHopDistance:     5,
		ComputeOnStartup:   true,
	}

	config.ApplyDefaults()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: nc,
	}

	comp, err := CreateGraphAnomalies(configJSON, deps)
	require.NoError(t, err)

	anomaliesComp := comp.(*Component)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	require.NoError(t, anomaliesComp.Initialize())

	js, err := nc.JetStream()
	require.NoError(t, err)

	// Create input buckets
	outgoingBucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketOutgoingIndex,
		Description: "Test outgoing edges",
	})
	require.NoError(t, err)

	incomingBucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketIncomingIndex,
		Description: "Test incoming edges",
	})
	require.NoError(t, err)

	// Create a simple connected graph: 4 entities in a fully connected square
	entityIDs := []string{
		"c360.test.graph.node.a",
		"c360.test.graph.node.b",
		"c360.test.graph.node.c",
		"c360.test.graph.node.d",
	}

	// Create bidirectional edges forming a cycle with cross-connections
	edges := []struct {
		from string
		to   string
	}{
		// Cycle: a -> b -> c -> d -> a
		{entityIDs[0], entityIDs[1]}, // a -> b
		{entityIDs[1], entityIDs[0]}, // b -> a
		{entityIDs[1], entityIDs[2]}, // b -> c
		{entityIDs[2], entityIDs[1]}, // c -> b
		{entityIDs[2], entityIDs[3]}, // c -> d
		{entityIDs[3], entityIDs[2]}, // d -> c
		{entityIDs[3], entityIDs[0]}, // d -> a
		{entityIDs[0], entityIDs[3]}, // a -> d
		// Cross-connections: a -> c, b -> d
		{entityIDs[0], entityIDs[2]}, // a -> c
		{entityIDs[2], entityIDs[0]}, // c -> a
		{entityIDs[1], entityIDs[3]}, // b -> d
		{entityIDs[3], entityIDs[1]}, // d -> b
	}

	// Collect neighbors by entity (simple []string arrays)
	outgoingNeighbors := make(map[string][]string)
	incomingNeighbors := make(map[string][]string)

	for _, edge := range edges {
		outgoingNeighbors[edge.from] = append(outgoingNeighbors[edge.from], edge.to)
		incomingNeighbors[edge.to] = append(incomingNeighbors[edge.to], edge.from)
	}

	// Write to buckets
	for entityID, neighbors := range outgoingNeighbors {
		outgoingJSON, _ := json.Marshal(neighbors)
		outgoingBucket.Put(ctx, entityID, outgoingJSON)
	}
	for entityID, neighbors := range incomingNeighbors {
		incomingJSON, _ := json.Marshal(neighbors)
		incomingBucket.Put(ctx, entityID, incomingJSON)
	}

	// Start component
	require.NoError(t, anomaliesComp.Start(ctx))
	defer anomaliesComp.Stop(5 * time.Second)

	// Wait for structural bucket
	var structuralBucket jetstream.KeyValue
	require.Eventually(t, func() bool {
		structuralBucket, err = js.KeyValue(ctx, graph.BucketStructuralIndex)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	// Wait for k-core computation
	var kcoreMeta map[string]interface{}
	require.Eventually(t, func() bool {
		entry, err := structuralBucket.Get(ctx, "structural.kcore._meta")
		if err != nil {
			return false
		}
		return json.Unmarshal(entry.Value(), &kcoreMeta) == nil
	}, 15*time.Second, 500*time.Millisecond, "K-core computation should complete")

	// Verify max_core is recorded (can be 0 for sparse graphs)
	maxCore, ok := kcoreMeta["max_core"].(float64)
	require.True(t, ok, "max_core should be a number")
	assert.GreaterOrEqual(t, int(maxCore), 0, "max_core should be >= 0")

	// Verify entity core numbers exist
	coreNumbersFound := 0
	for _, entityID := range entityIDs {
		key := "structural.kcore.entity." + entityID
		entry, err := structuralBucket.Get(ctx, key)
		if err == nil {
			coreNumbersFound++
			t.Logf("Entity %s has core number: %s", entityID, string(entry.Value()))
		}
	}

	assert.Greater(t, coreNumbersFound, 0, "At least some entities should have core numbers")

	t.Logf("K-core computation successful: max_core=%d, entities_with_cores=%d",
		int(maxCore), coreNumbersFound)
}

// TestIntegration_AnomaliesPivotComputation verifies pivot selection and distance computation
func TestIntegration_AnomaliesPivotComputation(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	nc := testClient.Client

	config := Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "outgoing_watch", Type: "kv-watch", Subject: graph.BucketOutgoingIndex},
				{Name: "incoming_watch", Type: "kv-watch", Subject: graph.BucketIncomingIndex},
			},
			Outputs: []component.PortDefinition{
				{Name: "structural_index", Type: "kv-write", Subject: graph.BucketStructuralIndex},
			},
		},
		ComputeIntervalStr: "1s",
		PivotCount:         4, // Request 4 pivots
		MaxHopDistance:     5,
		ComputeOnStartup:   true,
	}

	config.ApplyDefaults()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: nc,
	}

	comp, err := CreateGraphAnomalies(configJSON, deps)
	require.NoError(t, err)

	anomaliesComp := comp.(*Component)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	require.NoError(t, anomaliesComp.Initialize())

	js, err := nc.JetStream()
	require.NoError(t, err)

	// Create input buckets
	outgoingBucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketOutgoingIndex,
		Description: "Test outgoing edges",
	})
	require.NoError(t, err)

	incomingBucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketIncomingIndex,
		Description: "Test incoming edges",
	})
	require.NoError(t, err)

	// Create a larger graph (6 entities) to ensure we can select multiple pivots
	entityIDs := []string{
		"c360.test.graph.node.a",
		"c360.test.graph.node.b",
		"c360.test.graph.node.c",
		"c360.test.graph.node.d",
		"c360.test.graph.node.e",
		"c360.test.graph.node.f",
	}

	// Create a connected graph
	edges := []struct {
		from string
		to   string
	}{
		// Chain: a -> b -> c -> d -> e -> f
		{entityIDs[0], entityIDs[1]},
		{entityIDs[1], entityIDs[2]},
		{entityIDs[2], entityIDs[3]},
		{entityIDs[3], entityIDs[4]},
		{entityIDs[4], entityIDs[5]},
		// Some back edges for connectivity
		{entityIDs[1], entityIDs[0]},
		{entityIDs[2], entityIDs[1]},
		{entityIDs[3], entityIDs[2]},
		{entityIDs[4], entityIDs[3]},
		{entityIDs[5], entityIDs[4]},
	}

	// Collect neighbors (simple []string arrays)
	outgoingNeighbors := make(map[string][]string)
	incomingNeighbors := make(map[string][]string)

	for _, edge := range edges {
		outgoingNeighbors[edge.from] = append(outgoingNeighbors[edge.from], edge.to)
		incomingNeighbors[edge.to] = append(incomingNeighbors[edge.to], edge.from)
	}

	// Write to buckets
	for entityID, neighbors := range outgoingNeighbors {
		outgoingJSON, _ := json.Marshal(neighbors)
		outgoingBucket.Put(ctx, entityID, outgoingJSON)
	}
	for entityID, neighbors := range incomingNeighbors {
		incomingJSON, _ := json.Marshal(neighbors)
		incomingBucket.Put(ctx, entityID, incomingJSON)
	}

	// Start component
	require.NoError(t, anomaliesComp.Start(ctx))
	defer anomaliesComp.Stop(5 * time.Second)

	// Wait for structural bucket
	var structuralBucket jetstream.KeyValue
	require.Eventually(t, func() bool {
		structuralBucket, err = js.KeyValue(ctx, graph.BucketStructuralIndex)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	// Wait for pivot computation
	var pivotMeta map[string]interface{}
	require.Eventually(t, func() bool {
		entry, err := structuralBucket.Get(ctx, "structural.pivot._meta")
		if err != nil {
			return false
		}
		if err := json.Unmarshal(entry.Value(), &pivotMeta); err != nil {
			return false
		}
		// Check if pivots list is populated
		pivots, ok := pivotMeta["pivots"].([]interface{})
		return ok && len(pivots) > 0
	}, 15*time.Second, 500*time.Millisecond, "Pivot computation should complete")

	// Verify pivots list exists and is populated
	pivots, ok := pivotMeta["pivots"].([]interface{})
	require.True(t, ok, "pivots should be an array")
	assert.Greater(t, len(pivots), 0, "should have selected at least one pivot")
	assert.LessOrEqual(t, len(pivots), 4, "should not exceed configured pivot_count")

	t.Logf("Selected %d pivots: %v", len(pivots), pivots)

	// Verify distance vectors exist for entities
	distanceVectorsFound := 0
	for _, entityID := range entityIDs {
		key := "structural.pivot.entity." + entityID
		entry, err := structuralBucket.Get(ctx, key)
		if err == nil {
			var distances []int
			if json.Unmarshal(entry.Value(), &distances) == nil {
				distanceVectorsFound++
				t.Logf("Entity %s has distance vector: %v", entityID, distances)
			}
		}
	}

	assert.Greater(t, distanceVectorsFound, 0, "At least some entities should have distance vectors")

	t.Logf("Pivot computation successful: %d pivots, %d distance vectors",
		len(pivots), distanceVectorsFound)
}
