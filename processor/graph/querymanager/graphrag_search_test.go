package querymanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/processor/graph/clustering"
)

// mockCommunityDetector implements communityDetectorInterface for testing
type mockCommunityDetector struct {
	communities map[string]*clustering.Community         // by ID
	entityComm  map[string]map[int]*clustering.Community // by entityID -> level -> community
	getErr      error
	listErr     error
}

func (m *mockCommunityDetector) GetCommunity(_ context.Context, communityID string) (*clustering.Community, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.communities == nil {
		return nil, nil
	}
	return m.communities[communityID], nil
}

func (m *mockCommunityDetector) GetEntityCommunity(_ context.Context, entityID string, level int) (*clustering.Community, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.entityComm == nil {
		return nil, nil
	}
	if levelMap, ok := m.entityComm[entityID]; ok {
		return levelMap[level], nil
	}
	return nil, nil
}

func (m *mockCommunityDetector) GetCommunitiesByLevel(_ context.Context, level int) ([]*clustering.Community, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var result []*clustering.Community
	for _, comm := range m.communities {
		if comm.Level == level {
			result = append(result, comm)
		}
	}
	return result, nil
}

// mockEntityReader implements EntityReader interface for testing
type mockEntityReader struct {
	entities map[string]*gtypes.EntityState
}

// GetEntity returns an entity by ID
func (m *mockEntityReader) GetEntity(_ context.Context, id string) (*gtypes.EntityState, error) {
	return m.entities[id], nil
}

// ExistsEntity checks if an entity exists
func (m *mockEntityReader) ExistsEntity(_ context.Context, id string) (bool, error) {
	_, ok := m.entities[id]
	return ok, nil
}

// BatchGet retrieves multiple entities efficiently
func (m *mockEntityReader) BatchGet(_ context.Context, ids []string) ([]*gtypes.EntityState, error) {
	result := make([]*gtypes.EntityState, 0, len(ids))
	for _, id := range ids {
		if entity, ok := m.entities[id]; ok {
			result = append(result, entity)
		}
	}
	return result, nil
}

func Test_scoreCommunitySummaries(t *testing.T) {
	m := &Manager{}

	t.Run("Empty communities", func(t *testing.T) {
		result := m.scoreCommunitySummaries([]*clustering.Community{}, "test query")
		assert.Empty(t, result)
	})

	t.Run("Score by summary match", func(t *testing.T) {
		communities := []*clustering.Community{
			&clustering.Community{
				ID:                 "comm-1",
				StatisticalSummary: "This is about robotics and automation",
			},
			&clustering.Community{
				ID:                 "comm-2",
				StatisticalSummary: "This is about web development",
			},
		}

		result := m.scoreCommunitySummaries(communities, "robotics")

		require.Len(t, result, 2)
		assert.Equal(t, "comm-1", result[0].ID, "robotics community should be first")
		assert.Equal(t, "comm-2", result[1].ID)
	})

	t.Run("Score by keyword match", func(t *testing.T) {
		communities := []*clustering.Community{
			&clustering.Community{
				ID:       "comm-1",
				Keywords: []string{"python", "django", "flask"},
			},
			&clustering.Community{
				ID:       "comm-2",
				Keywords: []string{"go", "concurrent", "microservices"},
			},
		}

		result := m.scoreCommunitySummaries(communities, "go microservices")

		require.Len(t, result, 2)
		assert.Equal(t, "comm-2", result[0].ID, "go community should be first")
	})

	t.Run("Combined scoring", func(t *testing.T) {
		communities := []*clustering.Community{
			&clustering.Community{
				ID:                 "comm-1",
				StatisticalSummary: "Machine learning models",
				Keywords:           []string{"ml", "ai"},
			},
			&clustering.Community{
				ID:                 "comm-2",
				StatisticalSummary: "AI and machine learning techniques",
				Keywords:           []string{"machine-learning", "deep-learning"},
			},
			&clustering.Community{
				ID:                 "comm-3",
				StatisticalSummary: "Web development frameworks",
				Keywords:           []string{"web", "http"},
			},
		}

		result := m.scoreCommunitySummaries(communities, "machine learning")

		require.Len(t, result, 3)
		// comm-2 should score highest (matches in both summary and keywords)
		assert.Equal(t, "comm-2", result[0].ID)
		// comm-1 should be second (matches in summary)
		assert.Equal(t, "comm-1", result[1].ID)
		// comm-3 should be last (no matches)
		assert.Equal(t, "comm-3", result[2].ID)
	})
}

func Test_filterEntitiesByQuery(t *testing.T) {
	m := &Manager{}

	// Use proper 6-part entity IDs: org.platform.domain.system.type.instance
	// Type is extracted from ID (5th component) for filtering
	entities := []*gtypes.EntityState{
		{
			ID: "c360.platform.robotics.system.drone.e1",
		},
		{
			ID: "c360.platform.robotics.system.router.e2",
		},
		{
			ID: "c360.platform.networking.system.switch.e3",
		},
	}

	t.Run("Match by domain in ID", func(t *testing.T) {
		result := m.filterEntitiesByQuery(entities, "robotics")
		assert.Len(t, result, 2, "Should match 2 robotics entities")
	})

	t.Run("Match by type", func(t *testing.T) {
		result := m.filterEntitiesByQuery(entities, "drone")
		assert.Len(t, result, 1)
		assert.Equal(t, "c360.platform.robotics.system.drone.e1", result[0].ID)
	})

	t.Run("Match by instance ID", func(t *testing.T) {
		result := m.filterEntitiesByQuery(entities, "e2")
		assert.Len(t, result, 1)
		assert.Equal(t, "c360.platform.robotics.system.router.e2", result[0].ID)
	})

	t.Run("No match", func(t *testing.T) {
		result := m.filterEntitiesByQuery(entities, "nonexistent")
		assert.Empty(t, result)
	})

	t.Run("Empty query returns all", func(t *testing.T) {
		result := m.filterEntitiesByQuery(entities, "")
		assert.Len(t, result, 3)
	})

	t.Run("Multiple terms - any match", func(t *testing.T) {
		result := m.filterEntitiesByQuery(entities, "drone router")
		assert.Len(t, result, 2, "Should match entities with either 'drone' or 'router'")
	})
}

func Test_entityMatchesQuery(t *testing.T) {
	m := &Manager{}

	// Entity ID format: org.platform.domain.system.type.instance
	entity := &gtypes.EntityState{
		ID: "c360.platform.robotics.system.drone.test-entity",
	}

	tests := []struct {
		name       string
		queryTerms []string
		want       bool
	}{
		{
			name:       "Match instance in ID",
			queryTerms: []string{"test-entity"},
			want:       true,
		},
		{
			name:       "Match domain in ID",
			queryTerms: []string{"robotics"},
			want:       true,
		},
		{
			name:       "No match",
			queryTerms: []string{"nonexistent"},
			want:       false,
		},
		{
			name:       "Case insensitive type match",
			queryTerms: []string{"DRONE"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.entityMatchesQuery(entity, tt.queryTerms)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLocalSearch_Success(t *testing.T) {
	ctx := context.Background()

	// Entity IDs using 6-part format: org.platform.domain.system.type.instance
	e1ID := "c360.platform.robotics.system.drone.e1"
	e2ID := "c360.platform.robotics.system.drone.e2"
	e3ID := "c360.platform.networking.system.switch.e3"

	// Setup mock community detector
	comm := &clustering.Community{
		ID:                 "comm-0-robotics",
		Level:              0,
		Members:            []string{e1ID, e2ID, e3ID},
		StatisticalSummary: "Robotics community",
	}

	detector := &mockCommunityDetector{
		entityComm: map[string]map[int]*clustering.Community{
			e1ID: {0: comm},
		},
	}

	// Setup mock data handler
	dataHandler := &mockEntityReader{
		entities: map[string]*gtypes.EntityState{
			e1ID: {
				ID: e1ID,
			},
			e2ID: {
				ID: e2ID,
			},
			e3ID: {
				ID: e3ID,
			},
		},
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      dataHandler,
	}

	result, err := m.LocalSearch(ctx, e1ID, "robotics", 0)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "comm-0-robotics", result.CommunityID)
	assert.Equal(t, 2, result.Count, "Should match 2 robotics entities")
	assert.Len(t, result.Entities, 2)
}

func TestLocalSearch_EntityNotInCommunity(t *testing.T) {
	ctx := context.Background()

	detector := &mockCommunityDetector{
		entityComm: map[string]map[int]*clustering.Community{},
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      &mockEntityReader{entities: map[string]*gtypes.EntityState{}},
	}

	result, err := m.LocalSearch(ctx, "nonexistent", "query", 0)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not in any community")
}

func TestLocalSearch_CommunityDetectorUnavailable(t *testing.T) {
	ctx := context.Background()

	m := &Manager{
		communityDetector: nil,
		entityReader:      &mockEntityReader{entities: map[string]*gtypes.EntityState{}},
	}

	result, err := m.LocalSearch(ctx, "e1", "query", 0)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not available")
}

func TestGlobalSearch_Success(t *testing.T) {
	ctx := context.Background()

	// Entity IDs using 6-part format: org.platform.domain.system.type.instance
	e1ID := "c360.platform.robotics.system.drone.e1"
	e2ID := "c360.platform.robotics.system.drone.e2"
	e3ID := "c360.platform.networking.system.router.e3"

	// Setup communities
	comm1 := &clustering.Community{
		ID:                 "comm-0-robotics",
		Level:              0,
		Members:            []string{e1ID, e2ID},
		StatisticalSummary: "Robotics and autonomous systems",
		Keywords:           []string{"robotics", "autonomous", "drone"},
	}
	comm2 := &clustering.Community{
		ID:                 "comm-0-network",
		Level:              0,
		Members:            []string{e3ID},
		StatisticalSummary: "Network infrastructure",
		Keywords:           []string{"network", "router", "switch"},
	}

	detector := &mockCommunityDetector{
		communities: map[string]*clustering.Community{
			"comm-0-robotics": comm1,
			"comm-0-network":  comm2,
		},
	}

	dataHandler := &mockEntityReader{
		entities: map[string]*gtypes.EntityState{
			e1ID: {
				ID: e1ID,
			},
			e2ID: {
				ID: e2ID,
			},
			e3ID: {
				ID: e3ID,
			},
		},
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      dataHandler,
	}

	result, err := m.GlobalSearch(ctx, "robotics autonomous", 0, 1)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.CommunitySummaries, 1, "Should return top 1 community")
	assert.Equal(t, "comm-0-robotics", result.CommunitySummaries[0].CommunityID)
	assert.Greater(t, result.CommunitySummaries[0].Relevance, 0.0)
	assert.Equal(t, 2, result.Count, "Should find 2 entities in robotics community")
}

func TestGlobalSearch_EmptyCommunities(t *testing.T) {
	ctx := context.Background()

	detector := &mockCommunityDetector{
		communities: map[string]*clustering.Community{},
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      &mockEntityReader{entities: map[string]*gtypes.EntityState{}},
	}

	result, err := m.GlobalSearch(ctx, "query", 0, 5)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Entities)
	assert.Empty(t, result.CommunitySummaries)
	assert.Equal(t, 0, result.Count)
}

func TestGlobalSearch_MaxCommunitiesLimit(t *testing.T) {
	ctx := context.Background()

	// Create 10 communities with 6-part entity IDs
	communities := make(map[string]*clustering.Community)
	for i := 0; i < 10; i++ {
		commID := fmt.Sprintf("comm-0-%d", i)
		entityID := fmt.Sprintf("c360.platform.test.system.entity.e%d", i)
		communities[commID] = &clustering.Community{
			ID:                 commID,
			Level:              0,
			Members:            []string{entityID},
			StatisticalSummary: fmt.Sprintf("Community %d about testing", i),
			Keywords:           []string{"test", "community"},
		}
	}

	detector := &mockCommunityDetector{
		communities: communities,
	}

	// Create corresponding entities
	entities := make(map[string]*gtypes.EntityState)
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("c360.platform.test.system.entity.e%d", i)
		entities[id] = &gtypes.EntityState{
			ID: id,
		}
	}

	dataHandler := &mockEntityReader{
		entities: entities,
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      dataHandler,
	}

	// Request only top 3 communities
	result, err := m.GlobalSearch(ctx, "testing", 0, 3)

	require.NoError(t, err)
	assert.Len(t, result.CommunitySummaries, 3, "Should limit to 3 communities")
}

func TestGlobalSearch_DefaultMaxCommunities(t *testing.T) {
	ctx := context.Background()

	detector := &mockCommunityDetector{
		communities: map[string]*clustering.Community{},
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      &mockEntityReader{entities: map[string]*gtypes.EntityState{}},
	}

	// maxCommunities = 0 should default to 5
	result, err := m.GlobalSearch(ctx, "query", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// Result should be empty due to no communities, but the code path is tested
}

// Benchmarks for GraphRAG hot paths

func BenchmarkLocalSearch(b *testing.B) {
	ctx := context.Background()

	// Setup: Create a realistic community with 100 entities
	memberIDs := make([]string, 100)
	entities := make(map[string]*gtypes.EntityState)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("entity-%d", i)
		memberIDs[i] = id
		entities[id] = &gtypes.EntityState{
			ID: id,
		}
	}

	comm := &clustering.Community{
		ID:                 "bench-comm",
		Level:              0,
		Members:            memberIDs,
		StatisticalSummary: "IoT sensor network for environmental monitoring",
	}

	detector := &mockCommunityDetector{
		entityComm: map[string]map[int]*clustering.Community{
			"entity-0": {0: comm},
		},
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      &mockEntityReader{entities: entities},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := m.LocalSearch(ctx, "entity-0", "temperature sensor", 0)
		if err != nil {
			b.Fatalf("LocalSearch failed: %v", err)
		}
	}
}

func BenchmarkGlobalSearch(b *testing.B) {
	ctx := context.Background()

	// Setup: Create 10 communities with 50 entities each
	communities := make(map[string]*clustering.Community)
	allEntities := make(map[string]*gtypes.EntityState)

	for commIdx := 0; commIdx < 10; commIdx++ {
		memberIDs := make([]string, 50)
		for i := 0; i < 50; i++ {
			id := fmt.Sprintf("e-%d-%d", commIdx, i)
			memberIDs[i] = id
			allEntities[id] = &gtypes.EntityState{
				ID: id,
			}
		}

		commID := fmt.Sprintf("comm-%d", commIdx)
		communities[commID] = &clustering.Community{
			ID:                 commID,
			Level:              0,
			Members:            memberIDs,
			StatisticalSummary: fmt.Sprintf("Community %d with test entities", commIdx),
			Keywords:           []string{fmt.Sprintf("test-%d", commIdx), "benchmark"},
		}
	}

	detector := &mockCommunityDetector{
		communities: communities,
	}

	m := &Manager{
		communityDetector: detector,
		entityReader:      &mockEntityReader{entities: allEntities},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := m.GlobalSearch(ctx, "test benchmark entity", 0, 5)
		if err != nil {
			b.Fatalf("GlobalSearch failed: %v", err)
		}
	}
}

func BenchmarkScoreCommunitySummaries(b *testing.B) {
	m := &Manager{}

	// Create 100 communities with varying summaries and keywords
	communities := make([]*clustering.Community, 100)
	for i := 0; i < 100; i++ {
		communities[i] = &clustering.Community{
			ID:                 fmt.Sprintf("comm-%d", i),
			Level:              0,
			StatisticalSummary: fmt.Sprintf("Community about robotics automation and sensor networks topic-%d", i),
			Keywords:           []string{"robotics", "automation", "sensors", fmt.Sprintf("topic-%d", i)},
		}
	}

	query := "robotics sensor automation"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.scoreCommunitySummaries(communities, query)
	}
}

func BenchmarkFilterEntitiesByQuery(b *testing.B) {
	m := &Manager{}

	// Create 1000 entities
	entities := make([]*gtypes.EntityState, 1000)
	for i := 0; i < 1000; i++ {
		entities[i] = &gtypes.EntityState{
			ID: fmt.Sprintf("entity-%d", i),
		}
	}

	query := "test entity description"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.filterEntitiesByQuery(entities, query)
	}
}
