package mcp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/c360/semstreams/graph"
	gql "github.com/c360/semstreams/gateway/graphql"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/querymanager"
)

// mockMetricsRecorder records metrics for testing
type mockMetricsRecorder struct {
	mu    sync.Mutex
	calls []mockMetricCall
}

type mockMetricCall struct {
	Success  bool
	Duration time.Duration
}

func newMockMetricsRecorder() *mockMetricsRecorder {
	return &mockMetricsRecorder{
		calls: make([]mockMetricCall, 0),
	}
}

func (m *mockMetricsRecorder) RecordRequest(ctx context.Context, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockMetricCall{
		Success:  success,
		Duration: duration,
	})
}

func (m *mockMetricsRecorder) getCalls() []mockMetricCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockMetricCall, len(m.calls))
	copy(result, m.calls)
	return result
}

// mockQuerier implements querymanager.Querier for testing
type mockQuerier struct {
	mu sync.Mutex

	// Entity results
	entities map[string]*graph.EntityState

	// Relationship results
	relationships []*querymanager.Relationship

	// Search results
	localSearchResult  *querymanager.LocalSearchResult
	globalSearchResult *querymanager.GlobalSearchResult

	// Community results
	communities       map[string]*clustering.Community
	entityCommunities map[string]*clustering.Community

	// Error injection
	err error
}

func newMockQuerier() *mockQuerier {
	return &mockQuerier{
		entities:          make(map[string]*graph.EntityState),
		communities:       make(map[string]*clustering.Community),
		entityCommunities: make(map[string]*clustering.Community),
	}
}

func (m *mockQuerier) GetEntity(ctx context.Context, id string) (*graph.EntityState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.entities[id], nil
}

func (m *mockQuerier) GetEntities(ctx context.Context, ids []string) ([]*graph.EntityState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*graph.EntityState, 0, len(ids))
	for _, id := range ids {
		if e, ok := m.entities[id]; ok {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockQuerier) GetEntityByAlias(ctx context.Context, aliasOrID string) (*graph.EntityState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	// Check by ID first
	if e, ok := m.entities[aliasOrID]; ok {
		return e, nil
	}
	// For test simplicity, aliases map directly to the entities map
	return nil, nil
}

func (m *mockQuerier) ExecutePath(ctx context.Context, start string, pattern querymanager.PathPattern) (*querymanager.QueryResult, error) {
	return nil, nil
}

func (m *mockQuerier) GetGraphSnapshot(ctx context.Context, bounds querymanager.QueryBounds) (*querymanager.GraphSnapshot, error) {
	return nil, nil
}

func (m *mockQuerier) QueryRelationships(ctx context.Context, entityID string, direction querymanager.Direction) ([]*querymanager.Relationship, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.relationships, nil
}

func (m *mockQuerier) LocalSearch(ctx context.Context, entityID string, query string, level int) (*querymanager.LocalSearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.localSearchResult, nil
}

func (m *mockQuerier) GlobalSearch(ctx context.Context, query string, level int, maxCommunities int) (*querymanager.GlobalSearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.globalSearchResult, nil
}

func (m *mockQuerier) GlobalSearchWithOptions(ctx context.Context, opts *querymanager.SearchOptions) (*querymanager.GlobalSearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.globalSearchResult, nil
}

func (m *mockQuerier) GetCommunity(ctx context.Context, communityID string) (*clustering.Community, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.communities[communityID], nil
}

func (m *mockQuerier) GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.entityCommunities[entityID], nil
}

func (m *mockQuerier) GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error) {
	return nil, nil
}

func (m *mockQuerier) QueryByPredicate(ctx context.Context, predicate string) ([]string, error) {
	return nil, nil
}

func (m *mockQuerier) QuerySpatial(ctx context.Context, bounds querymanager.SpatialBounds) ([]string, error) {
	return nil, nil
}

func (m *mockQuerier) QueryTemporal(ctx context.Context, start, end time.Time) ([]string, error) {
	return nil, nil
}

func (m *mockQuerier) InvalidateEntity(entityID string) error {
	return nil
}

func (m *mockQuerier) WarmCache(ctx context.Context, entityIDs []string) error {
	return nil
}

func (m *mockQuerier) GetCacheStats() querymanager.CacheStats {
	return querymanager.CacheStats{}
}

// Test data helpers

func testEntity(id string) *graph.EntityState {
	return &graph.EntityState{
		ID:        id,
		Version:   1,
		UpdatedAt: time.Now(),
	}
}

func testCommunity(id string, level int) *clustering.Community {
	return &clustering.Community{
		ID:                 id,
		Level:              level,
		Members:            []string{"member-1", "member-2"},
		StatisticalSummary: "Test community summary",
	}
}

// testLogger returns a logger for testing
func testLogger() *slog.Logger {
	return slog.Default()
}

// createTestResolver creates a BaseResolver with the mock querier
func createTestResolver(mq *mockQuerier) *gql.BaseResolver {
	return gql.NewBaseResolver(mq, nil)
}

// createTestExecutor creates an Executor with a mock querier for testing
func createTestExecutor(mq *mockQuerier) (*Executor, error) {
	resolver := createTestResolver(mq)
	return NewExecutor(resolver, testLogger())
}
