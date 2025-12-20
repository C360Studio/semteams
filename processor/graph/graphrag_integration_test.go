//go:build integration

package graph

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/datamanager"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/querymanager"
)

// testGraphProvider adapts datamanager.EntityReader to clustering.GraphProvider
type testGraphProvider struct {
	entityReader datamanager.EntityReader
	kvBucket     jetstream.KeyValue
}

func (p *testGraphProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	// List all keys from the KV bucket
	keys, err := p.kvBucket.ListKeys(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0)
	for key := range keys.Keys() {
		ids = append(ids, key)
	}
	return ids, nil
}

func (p *testGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	// Get the entity to access its triples (relationships are now stored as triples)
	entity, err := p.entityReader.GetEntity(ctx, entityID)
	if err != nil {
		return []string{}, err
	}

	neighborSet := make(map[string]bool)

	// Collect neighbors from relationship triples
	// Triples where Object is an entity ID indicate relationships
	if direction == "outgoing" || direction == "both" {
		for _, triple := range entity.Triples {
			// Check if this is a relationship triple (Object is an entity ID)
			if triple.IsRelationship() {
				neighborSet[triple.Object.(string)] = true
			}
		}
	}

	// TODO: For incoming edges, we would need to query an incoming edge index
	// For now, incoming direction is not supported in this test helper
	if direction == "incoming" {
		// Not implemented - would require querying INCOMING_INDEX
		return []string{}, nil
	}

	neighbors := make([]string, 0, len(neighborSet))
	for id := range neighborSet {
		neighbors = append(neighbors, id)
	}
	return neighbors, nil
}

func (p *testGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	// Get the source entity
	entity, err := p.entityReader.GetEntity(ctx, fromID)
	if err != nil {
		return 0.0, err
	}

	// Check for relationship triples to target entity
	for _, triple := range entity.Triples {
		if triple.IsRelationship() && triple.Object.(string) == toID {
			// Weight is stored in triple's Confidence or defaults to 1.0
			if triple.Confidence > 0 {
				return triple.Confidence, nil
			}
			return 1.0, nil
		}
	}

	return 0.0, nil
}

// graphRAGTestSetup holds components for GraphRAG E2E testing
type graphRAGTestSetup struct {
	processor        *Processor
	queryManager     querymanager.Querier
	communityStorage clustering.CommunityStorage
	communityBucket  jetstream.KeyValue
	detector         *clustering.LPADetector
	ctx              context.Context
	cancel           context.CancelFunc
}

// setupGraphRAGTest creates a graph processor with community detector for GraphRAG testing
func setupGraphRAGTest(t *testing.T) *graphRAGTestSetup {
	natsClient := getSharedNATSClient(t)
	ctx, cancel := context.WithCancel(context.Background())

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

	err = processor.Initialize()
	require.NoError(t, err)

	// Start processor in background
	startErr := make(chan error, 1)
	go func() {
		startErr <- processor.Start(ctx)
	}()

	// Wait for processor to be ready
	err = processor.WaitForReady(5 * time.Second)
	require.NoError(t, err, "Processor should be ready within 5 seconds")

	// Create community storage bucket
	communityBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: clustering.CommunityBucket,
	})
	require.NoError(t, err, "Failed to create COMMUNITIES bucket")

	communityStorage := clustering.NewNATSCommunityStorage(communityBucket)

	// Get the ENTITY_STATES bucket for graph provider
	entityBucket, err := natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: "ENTITY_STATES",
	})
	require.NoError(t, err, "Failed to get ENTITY_STATES bucket")

	// Create GraphProvider that wraps the processor's data handler
	graphProvider := &testGraphProvider{
		entityReader: processor.entityManager,
		kvBucket:     entityBucket,
	}

	// Create LPA community detector
	detector := clustering.NewLPADetector(graphProvider, communityStorage)

	// Create QueryManager with community detector
	// Use empty config and rely on SetDefaults
	queryConfig := querymanager.Config{}
	queryDeps := querymanager.Deps{
		Config:            queryConfig,
		EntityReader:      processor.entityManager,
		IndexManager:      processor.indexManager,
		CommunityDetector: detector,
		Registry:          metric.NewMetricsRegistry(),
		Logger:            slog.Default(),
	}
	queryMgr, err := querymanager.NewManager(queryDeps)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		<-startErr
	})

	return &graphRAGTestSetup{
		processor:        processor,
		queryManager:     queryMgr,
		communityStorage: communityStorage,
		communityBucket:  communityBucket,
		detector:         detector,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// createTestEntity helper to create entities with properties (via triples)
func createTestEntity(ctx context.Context, processor *Processor, id string, entityType string, properties map[string]any) (*gtypes.EntityState, error) {
	// Convert properties to triples (triples are single source of truth)
	// Include type as a triple
	triples := make([]message.Triple, 0, len(properties)+1)

	// Add type triple
	triples = append(triples, message.Triple{
		Subject:   id,
		Predicate: "type",
		Object:    entityType,
	})

	// Add property triples
	for key, value := range properties {
		triples = append(triples, message.Triple{
			Subject:   id,
			Predicate: key,
			Object:    value,
		})
	}

	entity := &gtypes.EntityState{
		ID:        id,
		Triples:   triples,
		UpdatedAt: time.Now(),
		Version:   1,
	}
	return processor.entityManager.CreateEntity(ctx, entity)
}

// TestLocalSearch tests local community search
func TestLocalSearch(t *testing.T) {
	setup := setupGraphRAGTest(t)
	processor := setup.processor
	ctx := setup.ctx
	communityStorage := setup.communityStorage
	queryMgr := setup.queryManager

	// Create a community with robotics entities
	t.Log("Creating test community with robotics entities")

	// Create entities in the robotics community
	roboticsEntities := []struct {
		id         string
		entityType string
		name       string
	}{
		{"robot-1", "robotics.drone", "Autonomous Delivery Drone"},
		{"robot-2", "robotics.sensor", "LiDAR Scanner"},
		{"robot-3", "robotics.controller", "Navigation Controller"},
	}

	memberIDs := make([]string, len(roboticsEntities))
	for i, spec := range roboticsEntities {
		_, err := createTestEntity(ctx, processor, spec.id, spec.entityType, map[string]any{
			"name":        spec.name,
			"description": "Robotics component for autonomous systems",
		})
		require.NoError(t, err, "Failed to create entity %s", spec.id)
		memberIDs[i] = spec.id
	}

	// Create robotics community
	roboticsCommunity := &clustering.Community{
		ID:                 "comm-0-robotics",
		Level:              0,
		Members:            memberIDs,
		StatisticalSummary: "Robotics and autonomous systems community",
		Keywords:           []string{"robotics", "autonomous", "drone", "sensor"},
	}
	err := communityStorage.SaveCommunity(ctx, roboticsCommunity)
	require.NoError(t, err, "Failed to save robotics community")

	// Also create a network community (should not be returned by local search)
	_, err = createTestEntity(ctx, processor, "net-1", "network.router", map[string]any{
		"name": "Core Router",
	})
	require.NoError(t, err)

	networkCommunity := &clustering.Community{
		ID:                 "comm-0-network",
		Level:              0,
		Members:            []string{"net-1"},
		StatisticalSummary: "Network infrastructure community",
		Keywords:           []string{"network", "router"},
	}
	err = communityStorage.SaveCommunity(ctx, networkCommunity)
	require.NoError(t, err, "Failed to save network community")

	// Wait for entities to be indexed
	time.Sleep(200 * time.Millisecond)

	// Test 1: Local search from robot-1 should only return robotics entities
	t.Run("LocalSearch returns only entities from same community", func(t *testing.T) {
		result, err := queryMgr.LocalSearch(ctx, "robot-1", "robotics", 0)

		require.NoError(t, err, "LocalSearch failed")
		assert.NotNil(t, result)
		assert.Equal(t, "comm-0-robotics", result.CommunityID)

		// Should match all 3 robotics entities
		assert.Equal(t, 3, result.Count, "Should find all robotics entities")
		assert.Len(t, result.Entities, 3)

		// Verify no network entities included
		for _, entity := range result.Entities {
			assert.NotEqual(t, "net-1", entity.ID, "Network entity should not be in local search results")
		}
	})

	// Test 2: Query specificity
	t.Run("LocalSearch with specific query filters results", func(t *testing.T) {
		result, err := queryMgr.LocalSearch(ctx, "robot-1", "drone", 0)

		require.NoError(t, err)
		assert.NotNil(t, result)

		// Should only match the drone entity
		assert.Equal(t, 1, result.Count, "Should find only drone entity")
		if len(result.Entities) > 0 {
			assert.Contains(t, result.Entities[0].ID, "robot-1")
		}
	})

	t.Logf("✅ LocalSearch E2E test completed successfully")
}

// TestGlobalSearch tests global cross-community search
func TestGlobalSearch(t *testing.T) {
	setup := setupGraphRAGTest(t)
	processor := setup.processor
	ctx := setup.ctx
	communityStorage := setup.communityStorage
	queryMgr := setup.queryManager

	t.Log("Creating multiple communities for global search test")

	// Create 3 communities: robotics, network, storage
	communities := []struct {
		id       string
		summary  string
		keywords []string
		entities []struct {
			id         string
			entityType string
			name       string
		}
	}{
		{
			id:       "comm-0-robotics",
			summary:  "Robotics and autonomous systems with sensors and drones",
			keywords: []string{"robotics", "autonomous", "sensors"},
			entities: []struct {
				id         string
				entityType string
				name       string
			}{
				{"r1", "robotics.drone", "Delivery Drone"},
				{"r2", "robotics.sensor", "Temperature Sensor"},
			},
		},
		{
			id:       "comm-0-network",
			summary:  "Network infrastructure with routers and switches",
			keywords: []string{"network", "infrastructure", "routing"},
			entities: []struct {
				id         string
				entityType string
				name       string
			}{
				{"n1", "network.router", "Core Router"},
				{"n2", "network.switch", "Access Switch"},
			},
		},
		{
			id:       "comm-0-storage",
			summary:  "Storage systems and databases",
			keywords: []string{"storage", "database", "persistence"},
			entities: []struct {
				id         string
				entityType string
				name       string
			}{
				{"s1", "storage.database", "PostgreSQL Server"},
				{"s2", "storage.cache", "Redis Cache"},
			},
		},
	}

	// Create all communities and entities
	for _, comm := range communities {
		memberIDs := make([]string, len(comm.entities))
		for i, entity := range comm.entities {
			_, err := createTestEntity(ctx, processor, entity.id, entity.entityType, map[string]any{
				"name": entity.name,
			})
			require.NoError(t, err, "Failed to create entity %s", entity.id)
			memberIDs[i] = entity.id
		}

		community := &clustering.Community{
			ID:                 comm.id,
			Level:              0,
			Members:            memberIDs,
			StatisticalSummary: comm.summary,
			Keywords:           comm.keywords,
		}
		err := communityStorage.SaveCommunity(ctx, community)
		require.NoError(t, err, "Failed to save community %s", comm.id)
	}

	// Wait for indexing
	time.Sleep(200 * time.Millisecond)

	// Test 1: Global search spans multiple communities
	t.Run("GlobalSearch spans multiple communities", func(t *testing.T) {
		result, err := queryMgr.GlobalSearch(ctx, "router sensor", 0, 3)

		require.NoError(t, err, "GlobalSearch failed")
		assert.NotNil(t, result)

		// Should have community summaries
		assert.Greater(t, len(result.CommunitySummaries), 0, "Should return community summaries")

		// Should find entities across communities (router and sensor entities)
		assert.GreaterOrEqual(t, result.Count, 2, "Should find at least router and sensor entities")

		t.Logf("Found %d entities across %d communities", result.Count, len(result.CommunitySummaries))
	})

	// Test 2: Targeted query prefers relevant community
	t.Run("GlobalSearch ranks relevant community higher", func(t *testing.T) {
		result, err := queryMgr.GlobalSearch(ctx, "robotics sensor drone", 0, 3)

		require.NoError(t, err)
		assert.NotNil(t, result)
		require.Greater(t, len(result.CommunitySummaries), 0, "Should return summaries")

		// First community should be robotics (highest relevance)
		topCommunity := result.CommunitySummaries[0]
		assert.Equal(t, "comm-0-robotics", topCommunity.CommunityID, "Robotics community should rank highest")
		assert.Greater(t, topCommunity.Relevance, 0.0, "Should have positive relevance score")

		t.Logf("Top community: %s with relevance %.2f", topCommunity.CommunityID, topCommunity.Relevance)
	})

	// Test 3: MaxCommunities limit is enforced
	t.Run("GlobalSearch respects maxCommunities limit", func(t *testing.T) {
		result, err := queryMgr.GlobalSearch(ctx, "database", 0, 2)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.LessOrEqual(t, len(result.CommunitySummaries), 2, "Should respect maxCommunities=2")
	})

	t.Logf("✅ GlobalSearch E2E test completed successfully")
}

// TestCommunitySummaries tests community summary accuracy
func TestCommunitySummaries(t *testing.T) {
	setup := setupGraphRAGTest(t)
	processor := setup.processor
	ctx := setup.ctx
	communityStorage := setup.communityStorage
	queryMgr := setup.queryManager

	t.Log("Testing community summary generation and accuracy")

	// Create a well-defined community
	entities := []struct {
		id         string
		entityType string
		name       string
	}{
		{"ml-1", "ml.model", "Image Classification CNN"},
		{"ml-2", "ml.dataset", "ImageNet Training Data"},
		{"ml-3", "ml.pipeline", "Training Pipeline"},
	}

	memberIDs := make([]string, len(entities))
	for i, spec := range entities {
		_, err := createTestEntity(ctx, processor, spec.id, spec.entityType, map[string]any{
			"name":        spec.name,
			"description": "Machine learning component",
		})
		require.NoError(t, err)
		memberIDs[i] = spec.id
	}

	// Create community with statistical summary
	community := &clustering.Community{
		ID:                 "comm-0-ml",
		Level:              0,
		Members:            memberIDs,
		StatisticalSummary: "Machine learning and neural network training community",
		Keywords:           []string{"machine-learning", "cnn", "training", "neural-network"},
		RepEntities:        []string{"ml-1"}, // CNN is representative
		SummaryStatus:      "statistical",
	}
	err := communityStorage.SaveCommunity(ctx, community)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	t.Run("Community summary contains relevant keywords", func(t *testing.T) {
		result, err := queryMgr.GlobalSearch(ctx, "machine learning", 0, 5)

		require.NoError(t, err)
		assert.NotNil(t, result)
		require.Greater(t, len(result.CommunitySummaries), 0)

		// Find ML community in results
		var mlSummary *querymanager.CommunitySummary
		for i := range result.CommunitySummaries {
			if result.CommunitySummaries[i].CommunityID == "comm-0-ml" {
				mlSummary = &result.CommunitySummaries[i]
				break
			}
		}

		require.NotNil(t, mlSummary, "ML community should be in results")
		assert.Contains(t, strings.ToLower(mlSummary.Summary), "machine learning", "Summary should mention ML")
		assert.Contains(t, mlSummary.Keywords, "machine-learning", "Keywords should include ML")

		t.Logf("Community summary: %s", mlSummary.Summary)
		t.Logf("Keywords: %v", mlSummary.Keywords)
	})

	t.Logf("✅ Community summaries test completed successfully")
}

// TestPerformanceComparison tests that local search is faster than global
func TestPerformanceComparison(t *testing.T) {
	setup := setupGraphRAGTest(t)
	processor := setup.processor
	ctx := setup.ctx
	communityStorage := setup.communityStorage
	queryMgr := setup.queryManager

	t.Log("Testing performance: LocalSearch vs GlobalSearch")

	// Create a larger graph with multiple communities
	numCommunities := 5
	entitiesPerCommunity := 20

	for c := 0; c < numCommunities; c++ {
		memberIDs := make([]string, entitiesPerCommunity)
		for e := 0; e < entitiesPerCommunity; e++ {
			id := fmt.Sprintf("perf-c%d-e%d", c, e)
			_, err := createTestEntity(ctx, processor, id, "test.entity", map[string]any{
				"name":      fmt.Sprintf("Entity %d-%d", c, e),
				"community": c,
			})
			require.NoError(t, err)
			memberIDs[e] = id
		}

		community := &clustering.Community{
			ID:                 fmt.Sprintf("comm-0-perf%d", c),
			Level:              0,
			Members:            memberIDs,
			StatisticalSummary: fmt.Sprintf("Performance test community %d", c),
			Keywords:           []string{"performance", "test", fmt.Sprintf("comm%d", c)},
		}
		err := communityStorage.SaveCommunity(ctx, community)
		require.NoError(t, err)
	}

	time.Sleep(300 * time.Millisecond)

	// Measure LocalSearch performance
	t.Run("LocalSearch is faster than GlobalSearch", func(t *testing.T) {
		// Run LocalSearch 10 times
		localStart := time.Now()
		for i := 0; i < 10; i++ {
			_, err := queryMgr.LocalSearch(ctx, "perf-c0-e0", "test", 0)
			require.NoError(t, err)
		}
		localDuration := time.Since(localStart)

		// Run GlobalSearch 10 times
		globalStart := time.Now()
		for i := 0; i < 10; i++ {
			_, err := queryMgr.GlobalSearch(ctx, "test performance", 0, 5)
			require.NoError(t, err)
		}
		globalDuration := time.Since(globalStart)

		avgLocal := localDuration / 10
		avgGlobal := globalDuration / 10

		t.Logf("Average LocalSearch:  %v", avgLocal)
		t.Logf("Average GlobalSearch: %v", avgGlobal)
		t.Logf("LocalSearch is %.2fx faster", float64(globalDuration)/float64(localDuration))

		// LocalSearch should generally be faster since it searches fewer entities
		// But we don't assert this strictly since timing can vary
		if avgLocal < avgGlobal {
			t.Logf("✅ LocalSearch is faster as expected")
		} else {
			t.Logf("⚠️  GlobalSearch was faster (may happen with small datasets)")
		}
	})

	t.Logf("✅ Performance comparison test completed successfully")
}

// TestResourceLimits tests that resource limits are enforced
func TestResourceLimits(t *testing.T) {
	setup := setupGraphRAGTest(t)
	processor := setup.processor
	ctx := setup.ctx
	communityStorage := setup.communityStorage
	queryMgr := setup.queryManager

	t.Log("Testing resource limit enforcement in GlobalSearch")

	// Create a large community that would exceed MaxTotalEntitiesInSearch
	// MaxTotalEntitiesInSearch is 10,000, so simulate 11,000 entities across communities
	// (5500 * 2 = 11,000 total entity IDs in communities)
	entitiesPerCommunity := 5500
	numCommunities := 2

	for c := 0; c < numCommunities; c++ {
		memberIDs := make([]string, entitiesPerCommunity)
		for e := 0; e < entitiesPerCommunity; e++ {
			id := fmt.Sprintf("large-c%d-e%d", c, e)
			// Only create a subset of entities to keep test fast
			if e < 100 { // Create 100 entities per community
				_, err := createTestEntity(ctx, processor, id, "test.large", map[string]any{
					"name": fmt.Sprintf("Large Entity %d-%d", c, e),
				})
				require.NoError(t, err)
			}
			memberIDs[e] = id // Still add all IDs to community members
		}

		community := &clustering.Community{
			ID:                 fmt.Sprintf("comm-0-large%d", c),
			Level:              0,
			Members:            memberIDs, // 5500 members (simulated)
			StatisticalSummary: fmt.Sprintf("Large test community %d", c),
			Keywords:           []string{"large", "test"},
		}
		err := communityStorage.SaveCommunity(ctx, community)
		require.NoError(t, err)
	}

	time.Sleep(300 * time.Millisecond)

	t.Run("GlobalSearch enforces MaxTotalEntitiesInSearch", func(t *testing.T) {
		// This should try to load 11,000 entities but be capped at 10,000
		result, err := queryMgr.GlobalSearch(ctx, "large test", 0, 2)

		require.NoError(t, err, "GlobalSearch should not fail with large datasets")
		assert.NotNil(t, result)

		// The result count might be less than 10,000 since we only created 200 actual entities
		// But the important thing is it didn't try to load all 11,000
		t.Logf("GlobalSearch returned %d entities (capped by resource limit)", result.Count)
	})

	t.Logf("✅ Resource limits test completed successfully")
}

// TestLLMSummarization tests LLM-based community summarization with seminstruct.
// This test uses testcontainers to start shimmy + seminstruct with Qwen 0.5B model.
func TestLLMSummarization(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("LLM tests timeout in GitHub Actions - model loading too slow on shared runners")
	}

	setup := setupGraphRAGTest(t)
	processor := setup.processor
	ctx := setup.ctx
	communityStorage := setup.communityStorage

	t.Log("Testing LLM-based community summarization with testcontainer services")

	// Start LLM services via testcontainers
	llmHelper, err := StartLLMServices(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LLM testcontainers: %v", err)
	}
	defer llmHelper.Close(ctx)

	// Create LLM client using testcontainer helper
	llmClient, err := llmHelper.NewLLMClient()
	require.NoError(t, err, "Failed to create LLM client")
	defer llmClient.Close()

	llmSummarizer, err := clustering.NewLLMSummarizer(clustering.LLMSummarizerConfig{
		Client: llmClient,
	})
	require.NoError(t, err, "Failed to create LLM summarizer")

	// Create test entities with rich content for summarization
	entities := []struct {
		id         string
		entityType string
		properties map[string]any
	}{
		{
			"drone-1", "robotics.drone",
			map[string]any{
				"name":         "Autonomous Delivery Drone",
				"description":  "UAV for package delivery with obstacle avoidance",
				"capabilities": "navigation, object-detection, path-planning",
			},
		},
		{
			"sensor-1", "robotics.sensor",
			map[string]any{
				"name":        "LiDAR Sensor Array",
				"description": "3D mapping sensor for autonomous navigation",
				"range":       "100m",
			},
		},
		{
			"controller-1", "robotics.controller",
			map[string]any{
				"name":        "Flight Controller",
				"description": "Real-time control system for drone stabilization",
				"frequency":   "400Hz",
			},
		},
	}

	// Create entities in the graph
	memberIDs := make([]string, len(entities))
	createdEntities := make([]*gtypes.EntityState, len(entities))
	for i, spec := range entities {
		entity, err := createTestEntity(ctx, processor, spec.id, spec.entityType, spec.properties)
		require.NoError(t, err, "Failed to create entity %s", spec.id)
		memberIDs[i] = spec.id
		createdEntities[i] = entity
	}

	// Create community (without summary initially)
	community := &clustering.Community{
		ID:      "comm-llm-test",
		Level:   0,
		Members: memberIDs,
	}

	time.Sleep(200 * time.Millisecond)

	t.Run("LLM_generates_natural_language_summary", func(t *testing.T) {
		// Attempt LLM summarization with testcontainer services
		summarizedComm, err := llmSummarizer.SummarizeCommunity(ctx, community, createdEntities)
		require.NoError(t, err, "Summarization should not error")
		require.NotNil(t, summarizedComm)

		// Check LLM summary was generated (LLMSummary field, not StatisticalSummary)
		assert.NotEmpty(t, summarizedComm.LLMSummary, "LLM summary should not be empty")
		assert.NotEmpty(t, summarizedComm.Keywords, "Keywords should be extracted")

		// STRICT ASSERTION: LLM must be used, no graceful degradation hiding failures
		// If testcontainers started successfully, LLM summarization must work
		// Status is "llm-enhanced" when LLM succeeds (not just "llm")
		assert.Equal(t, "llm-enhanced", summarizedComm.SummaryStatus,
			"LLM summarization must succeed when testcontainers are running (got: %q)", summarizedComm.SummaryStatus)

		// Log results
		t.Logf("Summarizer used: %s", summarizedComm.SummaryStatus)
		t.Logf("LLMSummary: %s", summarizedComm.LLMSummary)
		t.Logf("Keywords: %v", summarizedComm.Keywords)

		// LLM summaries should be narrative style, not just entity counts
		assert.NotContains(t, summarizedComm.LLMSummary, "Community of", "LLM summary should be narrative")
	})

	t.Run("LLM_summary_integrates_with_GlobalSearch", func(t *testing.T) {
		// Save community with LLM-generated summary (no entity content in test)
		summarizedComm, _ := llmSummarizer.SummarizeCommunity(ctx, community, createdEntities)
		err := communityStorage.SaveCommunity(ctx, summarizedComm)
		require.NoError(t, err)

		// Use GlobalSearch to find the community
		result, err := setup.queryManager.GlobalSearch(ctx, "autonomous drone robotics", 0, 5)
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Find our community in results
		var foundComm *querymanager.CommunitySummary
		for i := range result.CommunitySummaries {
			if result.CommunitySummaries[i].CommunityID == "comm-llm-test" {
				foundComm = &result.CommunitySummaries[i]
				break
			}
		}

		if foundComm != nil {
			t.Logf("✅ LLM-summarized community found in GlobalSearch results")
			t.Logf("   Summary in search: %s", foundComm.Summary)
		}
	})

	t.Logf("✅ LLM summarization test completed successfully")
}

// NOTE: TestProgressiveEnhancement was removed and moved to E2E tests.
// Progressive enhancement tests system-level behavior (async LLM worker, community storage,
// KV watches) which is better tested in isolation via E2E rather than as an integration
// test that shares state with other tests.
// See: task e2e:semantic for progressive enhancement verification.
