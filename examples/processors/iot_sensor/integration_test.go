//go:build integration

// Package iotsensor provides an example domain processor demonstrating the correct
// Graphable implementation pattern for SemStreams.
package iotsensor

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/processor/graph/datamanager"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/messagemanager"
)

// testBuckets holds all KV buckets needed for integration tests
type testBuckets struct {
	entity    jetstream.KeyValue
	predicate jetstream.KeyValue
	incoming  jetstream.KeyValue
	alias     jetstream.KeyValue
	spatial   jetstream.KeyValue
	temporal  jetstream.KeyValue
}

// setupKVBuckets creates all KV buckets needed for integration testing
func setupKVBuckets(ctx context.Context, t *testing.T, client *natsclient.TestClient) testBuckets {
	t.Helper()

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

	return testBuckets{
		entity:    entityBucket,
		predicate: predicateBucket,
		incoming:  incomingBucket,
		alias:     aliasBucket,
		spatial:   spatialBucket,
		temporal:  temporalBucket,
	}
}

// setupDataManager creates and starts the data manager
func setupDataManager(
	ctx context.Context,
	t *testing.T,
	entityBucket jetstream.KeyValue,
	wg *sync.WaitGroup,
) (*datamanager.Manager, chan error) {
	t.Helper()

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
	wg.Add(1)
	go func() {
		defer wg.Done()
		dataErrors <- dataManager.Run(ctx)
	}()

	return dataManager, dataErrors
}

// setupIndexManager creates and starts the index manager
func setupIndexManager(
	ctx context.Context,
	t *testing.T,
	buckets testBuckets,
	client *natsclient.Client,
	wg *sync.WaitGroup,
) (indexmanager.Indexer, chan error) {
	t.Helper()

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
	wg.Add(1)
	go func() {
		defer wg.Done()
		indexErrors <- indexManager.Run(ctx)
	}()

	return indexManager, indexErrors
}

// setupMessageManager creates the message manager (stateless)
func setupMessageManager(
	t *testing.T,
	dataManager *datamanager.Manager,
	indexManager indexmanager.Indexer,
) messagemanager.MessageHandler {
	t.Helper()

	config := messagemanager.DefaultConfig()
	deps := messagemanager.Dependencies{
		EntityManager: dataManager,
		IndexManager:  indexManager,
		Logger:        slog.Default(),
	}

	return messagemanager.NewManager(config, deps, nil)
}

// checkStartupErrors waits for initialization and checks for startup errors
func checkStartupErrors(t *testing.T, dataErrors, indexErrors chan error) {
	t.Helper()

	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-dataErrors:
		if err != nil {
			t.Fatalf("DataManager failed to start: %v", err)
		}
	case err := <-indexErrors:
		if err != nil {
			t.Fatalf("IndexManager failed to start: %v", err)
		}
	default:
		// No errors yet - services are starting up
	}
}

// waitForShutdown waits for all services to shutdown cleanly
func waitForShutdown(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()

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

// TestSensorReading_ProcessedByMessageManager_ProducesEntityState is an integration
// test that verifies SensorReading payloads are correctly processed by the graph
// processor MessageManager, producing EntityState with triples.
//
// This test satisfies FR-008 and acceptance scenario US2 #4:
// Given an IoT sensor Graphable payload, When it is processed by the graph processor,
// Then the entity is stored correctly with all triples indexed.
func TestSensorReading_ProcessedByMessageManager_ProducesEntityState(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration tests (set INTEGRATION_TESTS=1 to run)")
	}

	// Create test NATS client with JetStream and KV
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	ctx := context.Background()

	// Setup infrastructure
	buckets := setupKVBuckets(ctx, t, testClient)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup managers
	var wg sync.WaitGroup
	dataManager, dataErrors := setupDataManager(ctx, t, buckets.entity, &wg)
	indexManager, indexErrors := setupIndexManager(ctx, t, buckets, testClient.Client, &wg)
	messageManager := setupMessageManager(t, dataManager, indexManager)

	// Verify startup
	checkStartupErrors(t, dataErrors, indexErrors)

	// Ensure clean shutdown
	defer func() {
		cancel()
		waitForShutdown(t, &wg)
	}()

	tests := []struct {
		name                    string
		reading                 SensorReading
		wantEntityParts         int
		wantTriples             int
		wantRelationshipTriples int // relationship triples (triples where Object is an entity ID)
	}{
		{
			name: "temperature sensor with zone reference",
			reading: SensorReading{
				DeviceID:     "sensor-042",
				SensorType:   "temperature",
				Value:        23.5,
				Unit:         "celsius",
				ZoneEntityID: "acme.logistics.facility.zone.area.warehouse-7",
				ObservedAt:   time.Date(2025, 11, 26, 10, 30, 0, 0, time.UTC),
				OrgID:        "acme",
				Platform:     "logistics",
			},
			wantEntityParts:         6,
			wantTriples:             4, // measurement, type, zone reference, timestamp
			wantRelationshipTriples: 1, // zone relationship triple
		},
		{
			name: "humidity sensor with zone reference",
			reading: SensorReading{
				DeviceID:     "hum-001",
				SensorType:   "humidity",
				Value:        65.0,
				Unit:         "percent",
				ZoneEntityID: "acme.facilities.facility.zone.area.office-3",
				ObservedAt:   time.Date(2025, 11, 26, 11, 0, 0, 0, time.UTC),
				OrgID:        "acme",
				Platform:     "facilities",
			},
			wantEntityParts:         6,
			wantTriples:             4, // measurement, type, zone reference, timestamp
			wantRelationshipTriples: 1, // zone relationship triple
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// T040: Write test: SensorReading payload processed by MessageManager produces EntityState
			testCtx, testCancel := context.WithTimeout(ctx, 10*time.Second)
			defer testCancel()

			entityStates, err := messageManager.ProcessMessage(testCtx, &tt.reading)
			if err != nil {
				t.Fatalf("ProcessMessage() failed: %v", err)
			}

			if len(entityStates) != 1 {
				t.Fatalf("ProcessMessage() returned %d entity states, want 1", len(entityStates))
			}

			state := entityStates[0]

			// T041: Write test: EntityState contains correct 6-part entity ID from SensorReading
			entityID := state.Node.ID
			parts := strings.Split(entityID, ".")
			if len(parts) != tt.wantEntityParts {
				t.Errorf("EntityState.Node.ID = %q has %d parts, want %d parts",
					entityID, len(parts), tt.wantEntityParts)
			}

			// Verify entity ID matches what SensorReading.EntityID() returns
			expectedEntityID := tt.reading.EntityID()
			if entityID != expectedEntityID {
				t.Errorf("EntityState.Node.ID = %q, want %q", entityID, expectedEntityID)
			}

			// Verify entity ID is valid per message package
			if !message.IsValidEntityID(entityID) {
				t.Errorf("EntityState.Node.ID = %q is not a valid 6-part entity ID", entityID)
			}

			// T042: Write test: EntityState contains all triples from SensorReading.Triples()
			if len(state.Triples) != tt.wantTriples {
				t.Errorf("EntityState has %d triples, want %d", len(state.Triples), tt.wantTriples)
			}

			// Verify all triples have the correct subject (entity ID)
			for i, triple := range state.Triples {
				if triple.Subject != entityID {
					t.Errorf("Triple[%d].Subject = %q, want %q", i, triple.Subject, entityID)
				}
			}

			// Verify expected predicates exist
			predicateMap := make(map[string]bool)
			for _, triple := range state.Triples {
				predicateMap[triple.Predicate] = true
			}

			expectedPredicates := []string{
				"sensor.measurement." + tt.reading.Unit,
				"sensor.classification.type",
				"geo.location.zone",
				"time.observation.recorded",
			}

			for _, pred := range expectedPredicates {
				if !predicateMap[pred] {
					t.Errorf("EntityState missing expected predicate %q", pred)
				}
			}

			// Count relationship triples (triples where Object is a valid entity ID)
			var relationshipTripleCount int
			for _, triple := range state.Triples {
				if triple.IsRelationship() {
					relationshipTripleCount++
				}
			}
			if relationshipTripleCount != tt.wantRelationshipTriples {
				t.Errorf("EntityState has %d relationship triples, want %d", relationshipTripleCount, tt.wantRelationshipTriples)
			}

			// T043: Write test: Zone entity reference in triples is valid 6-part entity ID
			var foundZoneTriple bool
			for _, triple := range state.Triples {
				if triple.Predicate == "geo.location.zone" {
					foundZoneTriple = true

					zoneID, ok := triple.Object.(string)
					if !ok {
						t.Errorf("geo.location.zone Object is not a string: %T", triple.Object)
						continue
					}

					// Verify zone ID is a valid 6-part entity ID
					zoneParts := strings.Split(zoneID, ".")
					if len(zoneParts) != 6 {
						t.Errorf("Zone entity ID %q has %d parts, want 6", zoneID, len(zoneParts))
					}

					if !message.IsValidEntityID(zoneID) {
						t.Errorf("Zone entity ID %q is not valid per message.IsValidEntityID()", zoneID)
					}

					// Verify it matches the ZoneEntityID from the reading
					if zoneID != tt.reading.ZoneEntityID {
						t.Errorf("Zone entity ID = %q, want %q", zoneID, tt.reading.ZoneEntityID)
					}

					// Verify that the zone triple is a relationship triple
					if !triple.IsRelationship() {
						t.Error("geo.location.zone triple should be identified as a relationship")
					}
				}
			}

			if !foundZoneTriple {
				t.Error("EntityState missing geo.location.zone triple")
			}
		})
	}
}

// TestZone_ProcessedByMessageManager_ProducesEntityState verifies that Zone
// entities are correctly processed by the graph processor.
func TestZone_ProcessedByMessageManager_ProducesEntityState(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration tests (set INTEGRATION_TESTS=1 to run)")
	}

	// Create test NATS client with JetStream and KV
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	ctx := context.Background()

	// Setup infrastructure
	buckets := setupKVBuckets(ctx, t, testClient)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup managers
	var wg sync.WaitGroup
	dataManager, dataErrors := setupDataManager(ctx, t, buckets.entity, &wg)
	indexManager, indexErrors := setupIndexManager(ctx, t, buckets, testClient.Client, &wg)
	messageManager := setupMessageManager(t, dataManager, indexManager)

	// Verify startup
	checkStartupErrors(t, dataErrors, indexErrors)

	// Ensure clean shutdown
	defer func() {
		cancel()
		waitForShutdown(t, &wg)
	}()

	zone := Zone{
		ZoneID:   "warehouse-7",
		ZoneType: "area",
		Name:     "Main Warehouse",
		OrgID:    "acme",
		Platform: "logistics",
	}

	testCtx, testCancel := context.WithTimeout(ctx, 10*time.Second)
	defer testCancel()

	entityStates, err := messageManager.ProcessMessage(testCtx, &zone)
	if err != nil {
		t.Fatalf("ProcessMessage() failed: %v", err)
	}

	if len(entityStates) != 1 {
		t.Fatalf("ProcessMessage() returned %d entity states, want 1", len(entityStates))
	}

	state := entityStates[0]

	// Verify zone entity ID is correct
	expectedEntityID := zone.EntityID()
	if state.Node.ID != expectedEntityID {
		t.Errorf("EntityState.Node.ID = %q, want %q", state.Node.ID, expectedEntityID)
	}

	// Verify zone has expected triples
	predicateMap := make(map[string]bool)
	for _, triple := range state.Triples {
		predicateMap[triple.Predicate] = true
	}

	expectedPredicates := []string{
		"facility.zone.name",
		"facility.zone.type",
	}

	for _, pred := range expectedPredicates {
		if !predicateMap[pred] {
			t.Errorf("Zone EntityState missing expected predicate %q", pred)
		}
	}
}

// TestMultipleSensorReadings_SameDevice_ConsistentEntityID verifies that multiple
// readings from the same sensor device produce EntityStates with the same entity ID.
func TestMultipleSensorReadings_SameDevice_ConsistentEntityID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration tests (set INTEGRATION_TESTS=1 to run)")
	}

	// Create test NATS client with JetStream and KV
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	ctx := context.Background()

	// Setup infrastructure
	buckets := setupKVBuckets(ctx, t, testClient)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup managers
	var wg sync.WaitGroup
	dataManager, dataErrors := setupDataManager(ctx, t, buckets.entity, &wg)
	indexManager, indexErrors := setupIndexManager(ctx, t, buckets, testClient.Client, &wg)
	messageManager := setupMessageManager(t, dataManager, indexManager)

	// Verify startup
	checkStartupErrors(t, dataErrors, indexErrors)

	// Ensure clean shutdown
	defer func() {
		cancel()
		waitForShutdown(t, &wg)
	}()

	testCtx, testCancel := context.WithTimeout(ctx, 10*time.Second)
	defer testCancel()

	// First reading: temperature
	reading1 := SensorReading{
		DeviceID:     "sensor-042",
		SensorType:   "temperature",
		Value:        23.5,
		Unit:         "celsius",
		ZoneEntityID: "acme.logistics.facility.zone.area.warehouse-7",
		ObservedAt:   time.Date(2025, 11, 26, 10, 30, 0, 0, time.UTC),
		OrgID:        "acme",
		Platform:     "logistics",
	}

	states1, err := messageManager.ProcessMessage(testCtx, &reading1)
	if err != nil {
		t.Fatalf("ProcessMessage(reading1) failed: %v", err)
	}

	if len(states1) != 1 {
		t.Fatalf("First reading produced %d states, want 1", len(states1))
	}

	// Second reading: same device, updated temperature
	reading2 := SensorReading{
		DeviceID:     "sensor-042",
		SensorType:   "temperature",
		Value:        24.0,
		Unit:         "celsius",
		ZoneEntityID: "acme.logistics.facility.zone.area.warehouse-7",
		ObservedAt:   time.Date(2025, 11, 26, 10, 35, 0, 0, time.UTC),
		OrgID:        "acme",
		Platform:     "logistics",
	}

	states2, err := messageManager.ProcessMessage(testCtx, &reading2)
	if err != nil {
		t.Fatalf("ProcessMessage(reading2) failed: %v", err)
	}

	if len(states2) != 1 {
		t.Fatalf("Second reading produced %d states, want 1", len(states2))
	}

	// Verify entity IDs are the same (same device)
	// This is the key invariant: the same device should always produce the same entity ID
	if states2[0].Node.ID != states1[0].Node.ID {
		t.Errorf("Second reading has different entity ID: %q vs %q",
			states2[0].Node.ID, states1[0].Node.ID)
	}

	// Verify both states have all expected triples
	expectedEntityID := reading1.EntityID()
	for i, state := range []*graph.EntityState{states1[0], states2[0]} {
		if state.Node.ID != expectedEntityID {
			t.Errorf("State %d has entity ID %q, want %q", i+1, state.Node.ID, expectedEntityID)
		}

		// Each state should have the complete set of triples for that reading
		if len(state.Triples) < 4 {
			t.Errorf("State %d has %d triples, want at least 4", i+1, len(state.Triples))
		}
	}
}
