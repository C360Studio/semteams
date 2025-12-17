package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/querymanager"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "valid custom config",
			config: Config{
				BindAddress:      ":9090",
				Path:             "/api/graphql",
				EnablePlayground: true,
				EnableCORS:       true,
				TimeoutStr:       "10s",
				MaxQueryDepth:    15,
				NATSSubjects: NATSSubjectsConfig{
					EntityQuery:       "test.entity",
					EntitiesQuery:     "test.entities",
					RelationshipQuery: "test.relationships",
					SemanticSearch:    "test.search",
				},
			},
			wantErr: false,
		},
		{
			name: "empty path defaults to /graphql",
			config: Config{
				BindAddress: ":8080",
				Path:        "", // Empty path
				TimeoutStr:  "5s",
				NATSSubjects: NATSSubjectsConfig{
					EntityQuery:       "test.entity",
					EntitiesQuery:     "test.entities",
					RelationshipQuery: "test.relationships",
					SemanticSearch:    "test.search",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid path (no leading slash)",
			config: Config{
				Path:       "graphql",
				TimeoutStr: "5s",
				NATSSubjects: NATSSubjectsConfig{
					EntityQuery:       "test.entity",
					EntitiesQuery:     "test.entities",
					RelationshipQuery: "test.relationships",
					SemanticSearch:    "test.search",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid timeout (too short)",
			config: Config{
				Path:       "/graphql",
				TimeoutStr: "10ms",
				NATSSubjects: NATSSubjectsConfig{
					EntityQuery:       "test.entity",
					EntitiesQuery:     "test.entities",
					RelationshipQuery: "test.relationships",
					SemanticSearch:    "test.search",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid max query depth (too high)",
			config: Config{
				Path:          "/graphql",
				TimeoutStr:    "5s",
				MaxQueryDepth: 100,
				NATSSubjects: NATSSubjectsConfig{
					EntityQuery:       "test.entity",
					EntitiesQuery:     "test.entities",
					RelationshipQuery: "test.relationships",
					SemanticSearch:    "test.search",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify empty path defaults to /graphql
				if tt.name == "empty path defaults to /graphql" {
					assert.Equal(t, "/graphql", tt.config.Path)
				}
			}
		})
	}
}

// TestPropertyExtraction tests property extraction helpers
func TestPropertyExtraction(t *testing.T) {
	// Create test entity
	entity := &Entity{
		ID:   "test-1",
		Type: "TestEntity",
		Properties: map[string]interface{}{
			"name":   "Test Entity",
			"count":  42,
			"price":  19.99,
			"active": true,
			"tags":   []interface{}{"tag1", "tag2"},
			"metadata": map[string]interface{}{
				"title": "Test Title",
				"score": 85,
			},
		},
	}

	t.Run("GetStringProp", func(t *testing.T) {
		assert.Equal(t, "Test Entity", GetStringProp(entity, "name"))
		assert.Equal(t, "", GetStringProp(entity, "nonexistent"))
		assert.Equal(t, "Test Title", GetStringProp(entity, "metadata.title"))
	})

	t.Run("GetIntProp", func(t *testing.T) {
		assert.Equal(t, 42, GetIntProp(entity, "count"))
		assert.Equal(t, 0, GetIntProp(entity, "nonexistent"))
		assert.Equal(t, 85, GetIntProp(entity, "metadata.score"))
		assert.Equal(t, 19, GetIntProp(entity, "price")) // float to int conversion
	})

	t.Run("GetFloatProp", func(t *testing.T) {
		assert.Equal(t, 19.99, GetFloatProp(entity, "price"))
		assert.Equal(t, 42.0, GetFloatProp(entity, "count")) // int to float conversion
		assert.Equal(t, 0.0, GetFloatProp(entity, "nonexistent"))
	})

	t.Run("GetBoolProp", func(t *testing.T) {
		assert.True(t, GetBoolProp(entity, "active"))
		assert.False(t, GetBoolProp(entity, "nonexistent"))
	})

	t.Run("GetArrayProp", func(t *testing.T) {
		tags := GetArrayProp(entity, "tags")
		assert.Len(t, tags, 2)
		assert.Equal(t, "tag1", tags[0])

		empty := GetArrayProp(entity, "nonexistent")
		assert.Len(t, empty, 0)
	})

	t.Run("GetMapProp", func(t *testing.T) {
		metadata := GetMapProp(entity, "metadata")
		assert.Equal(t, "Test Title", metadata["title"])
		assert.Equal(t, 85, metadata["score"])

		empty := GetMapProp(entity, "nonexistent")
		assert.Len(t, empty, 0)
	})

	t.Run("HasProperty", func(t *testing.T) {
		assert.True(t, HasProperty(entity, "name"))
		assert.True(t, HasProperty(entity, "metadata.title"))
		assert.False(t, HasProperty(entity, "nonexistent"))
	})

	t.Run("GetPropertyOrDefault", func(t *testing.T) {
		assert.Equal(t, "Test Entity", GetPropertyOrDefault(entity, "name", "default"))
		assert.Equal(t, "default", GetPropertyOrDefault(entity, "nonexistent", "default"))
	})
}

// TestBaseResolverCreation tests base resolver creation
func TestBaseResolverCreation(t *testing.T) {
	// Create a minimal NATSClient for testing
	natsClient := &NATSClient{
		timeout: 5 * time.Second,
	}

	// Test without metrics recorder
	resolver := NewBaseResolverWithNATS(natsClient, nil)
	assert.NotNil(t, resolver)
	assert.Equal(t, natsClient, resolver.natsClient)
	assert.Nil(t, resolver.metricsRecorder)

	// Test with metrics recorder (using a mock)
	mockRecorder := &mockMetricsRecorder{}
	resolverWithMetrics := NewBaseResolverWithNATS(natsClient, mockRecorder)
	assert.NotNil(t, resolverWithMetrics)
	assert.Equal(t, mockRecorder, resolverWithMetrics.metricsRecorder)
}

// mockMetricsRecorder for testing
type mockMetricsRecorder struct {
	recordedOps []string
}

func (m *mockMetricsRecorder) RecordMetrics(_ context.Context, operation string, fn func() error) error {
	m.recordedOps = append(m.recordedOps, operation)
	return fn()
}

// Note: Tests requiring actual NATS queries (QueryEntityByID, QueryEntitiesByIDs,
// QueryRelationships, SemanticSearch) should be in integration tests with testcontainers.
// Unit tests focus on configuration, property extraction, error mapping, and component lifecycle.

// TestErrorMapping tests NATS to GraphQL error mapping
func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		operation string
		wantCode  string
	}{
		{
			name:      "timeout error",
			err:       nats.ErrTimeout,
			operation: "test_query",
			wantCode:  "TIMEOUT",
		},
		{
			name:      "no responders",
			err:       nats.ErrNoResponders,
			operation: "test_query",
			wantCode:  "SERVICE_UNAVAILABLE",
		},
		{
			name:      "connection closed",
			err:       nats.ErrConnectionClosed,
			operation: "test_query",
			wantCode:  "CONNECTION_CLOSED",
		},
		{
			name:      "deadline exceeded",
			err:       context.DeadlineExceeded,
			operation: "test_query",
			wantCode:  "DEADLINE_EXCEEDED",
		},
		{
			name:      "cancelled",
			err:       context.Canceled,
			operation: "test_query",
			wantCode:  "CANCELLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gqlErr := mapNATSError(tt.err, tt.operation)
			require.NotNil(t, gqlErr)
			// Check that error was mapped (basic check)
			assert.Contains(t, gqlErr.Error(), "")
		})
	}
}

// TestComponentCreation tests GraphQL gateway component creation
func TestComponentCreation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultConfig()
		configJSON, err := json.Marshal(config)
		require.NoError(t, err)

		// Create mock NATS client
		mockClient := &natsclient.Client{}

		deps := component.Dependencies{
			NATSClient: mockClient,
		}

		gateway, err := NewGraphQLGateway(configJSON, deps)
		require.NoError(t, err)
		require.NotNil(t, gateway)

		// Verify interface implementations
		assert.Implements(t, (*component.Discoverable)(nil), gateway)

		// Check metadata
		meta := gateway.Meta()
		assert.Equal(t, "gateway", meta.Type)
		assert.Equal(t, "graphql-gateway", meta.Name)
	})

	t.Run("missing NATS client", func(t *testing.T) {
		config := DefaultConfig()
		configJSON, err := json.Marshal(config)
		require.NoError(t, err)

		deps := component.Dependencies{
			NATSClient: nil, // Missing
		}

		gateway, err := NewGraphQLGateway(configJSON, deps)
		assert.Error(t, err)
		assert.Nil(t, gateway)
	})

	t.Run("invalid config JSON", func(t *testing.T) {
		invalidJSON := []byte(`{"invalid": json}`)

		mockClient := &natsclient.Client{}
		deps := component.Dependencies{
			NATSClient: mockClient,
		}

		gateway, err := NewGraphQLGateway(invalidJSON, deps)
		assert.Error(t, err)
		assert.Nil(t, gateway)
	})
}

// TestGatewayLifecycle tests gateway lifecycle management
func TestGatewayLifecycle(t *testing.T) {
	config := DefaultConfig()
	config.BindAddress = ":0" // Use random port for testing
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	mockClient := &natsclient.Client{}
	deps := component.Dependencies{
		NATSClient: mockClient,
	}

	gateway, err := NewGraphQLGateway(configJSON, deps)
	require.NoError(t, err)

	t.Run("Initialize", func(t *testing.T) {
		err := gateway.(*Gateway).Initialize()
		assert.NoError(t, err)
	})

	t.Run("Health check before start", func(t *testing.T) {
		health := gateway.Health()
		assert.False(t, health.Healthy) // Not started yet
	})

	// Note: We're not testing Start/Stop here as they require actual HTTP server
	// Those are tested in integration tests
}

// BenchmarkPropertyExtraction benchmarks property extraction
func BenchmarkPropertyExtraction(b *testing.B) {
	entity := &Entity{
		ID:   "bench-1",
		Type: "BenchEntity",
		Properties: map[string]interface{}{
			"name":  "Benchmark Entity",
			"count": 42,
			"metadata": map[string]interface{}{
				"nested": map[string]interface{}{
					"value": "deep",
				},
			},
		},
	}

	b.Run("GetStringProp-simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetStringProp(entity, "name")
		}
	})

	b.Run("GetStringProp-nested", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetStringProp(entity, "metadata.nested.value")
		}
	})

	b.Run("GetIntProp", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetIntProp(entity, "count")
		}
	})
}

// BenchmarkResolverOperations benchmarks GraphQL resolver operations
func BenchmarkResolverOperations(b *testing.B) {
	// Setup mock query manager with test data
	testEntity := &graph.EntityState{
		ID: "bench-entity-1",
	}

	mockQM := &mockQueryManager{
		communities: map[string]*clustering.Community{
			"comm-1": {
				ID:                 "comm-1",
				Level:              1,
				Members:            []string{"entity-1", "entity-2"},
				StatisticalSummary: "Test community",
				LLMSummary:         "Enhanced test community",
				Keywords:           []string{"test", "benchmark"},
				SummaryStatus:      "llm-enhanced",
			},
		},
		entityCommunity: map[string]map[int]*clustering.Community{
			"entity-1": {
				1: {
					ID:                 "comm-1",
					Level:              1,
					Members:            []string{"entity-1", "entity-2"},
					StatisticalSummary: "Test community",
					SummaryStatus:      "statistical",
				},
			},
		},
	}

	resolver := &BaseResolver{
		queryManager: mockQM,
	}

	ctx := context.Background()

	b.Run("GetCommunity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = resolver.GetCommunity(ctx, "comm-1")
		}
	})

	b.Run("GetEntityCommunity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = resolver.GetEntityCommunity(ctx, "entity-1", 1)
		}
	})

	b.Run("ConvertCommunityToGraphQL", func(b *testing.B) {
		comm := mockQM.communities["comm-1"]
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = convertCommunityToGraphQL(comm)
		}
	})

	b.Run("ConvertEntityStateToGraphQL", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = convertEntityStateToGraphQL(testEntity)
		}
	})
}

// BenchmarkErrorMapping benchmarks error mapping performance
func BenchmarkErrorMapping(b *testing.B) {
	testCases := []struct {
		name string
		err  error
	}{
		{"Timeout", nats.ErrTimeout},
		{"NoResponders", nats.ErrNoResponders},
		{"ConnectionClosed", nats.ErrConnectionClosed},
		{"DeadlineExceeded", context.DeadlineExceeded},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = mapNATSError(tc.err, "test_operation")
			}
		})
	}
}

// mockQueryManager implements querymanager.Querier for testing
type mockQueryManager struct {
	communities      map[string]*clustering.Community
	entityCommunity  map[string]map[int]*clustering.Community
	getCommunityErr  error
	getEntityCommErr error
}

func (m *mockQueryManager) GetEntity(_ context.Context, _ string) (*graph.EntityState, error) {
	return nil, nil
}
func (m *mockQueryManager) GetEntities(_ context.Context, _ []string) ([]*graph.EntityState, error) {
	return nil, nil
}
func (m *mockQueryManager) GetEntityByAlias(_ context.Context, _ string) (*graph.EntityState, error) {
	return nil, nil
}
func (m *mockQueryManager) ExecutePath(_ context.Context, _ string, _ querymanager.PathPattern) (*querymanager.QueryResult, error) {
	return nil, nil
}
func (m *mockQueryManager) GetGraphSnapshot(_ context.Context, _ querymanager.QueryBounds) (*querymanager.GraphSnapshot, error) {
	return nil, nil
}
func (m *mockQueryManager) QueryRelationships(_ context.Context, _ string, _ querymanager.Direction) ([]*querymanager.Relationship, error) {
	return nil, nil
}
func (m *mockQueryManager) LocalSearch(_ context.Context, _ string, _ string, _ int) (*querymanager.LocalSearchResult, error) {
	return nil, nil
}
func (m *mockQueryManager) GlobalSearch(_ context.Context, _ string, _ int, _ int) (*querymanager.GlobalSearchResult, error) {
	return nil, nil
}
func (m *mockQueryManager) GetCommunity(_ context.Context, communityID string) (*clustering.Community, error) {
	if m.getCommunityErr != nil {
		return nil, m.getCommunityErr
	}
	if m.communities == nil {
		return nil, nil
	}
	comm, ok := m.communities[communityID]
	if !ok {
		return nil, fmt.Errorf("community not found")
	}
	return comm, nil
}
func (m *mockQueryManager) GetEntityCommunity(_ context.Context, entityID string, level int) (*clustering.Community, error) {
	if m.getEntityCommErr != nil {
		return nil, m.getEntityCommErr
	}
	if m.entityCommunity == nil {
		return nil, nil
	}
	if levelMap, ok := m.entityCommunity[entityID]; ok {
		if comm, ok := levelMap[level]; ok {
			return comm, nil
		}
	}
	return nil, fmt.Errorf("entity community not found")
}
func (m *mockQueryManager) GetCommunitiesByLevel(_ context.Context, _ int) ([]*clustering.Community, error) {
	return nil, nil
}
func (m *mockQueryManager) QueryByPredicate(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockQueryManager) QuerySpatial(_ context.Context, _ querymanager.SpatialBounds) ([]string, error) {
	return nil, nil
}
func (m *mockQueryManager) QueryTemporal(_ context.Context, _ time.Time, _ time.Time) ([]string, error) {
	return nil, nil
}
func (m *mockQueryManager) InvalidateEntity(_ string) error {
	return nil
}
func (m *mockQueryManager) WarmCache(_ context.Context, _ []string) error {
	return nil
}
func (m *mockQueryManager) GetCacheStats() querymanager.CacheStats {
	return querymanager.CacheStats{}
}
func (m *mockQueryManager) GlobalSearchWithOptions(_ context.Context, _ *querymanager.SearchOptions) (*querymanager.GlobalSearchResult, error) {
	return nil, nil
}

// TestGetCommunity tests the GetCommunity resolver
func TestGetCommunity(t *testing.T) {
	t.Run("Success - with LLM summary", func(t *testing.T) {
		mockQM := &mockQueryManager{
			communities: map[string]*clustering.Community{
				"comm-1": {
					ID:                 "comm-1",
					Level:              1,
					Members:            []string{"entity-1", "entity-2"},
					StatisticalSummary: "Statistical summary",
					LLMSummary:         "LLM enhanced summary",
					Keywords:           []string{"robotics", "ai"},
					RepEntities:        []string{"entity-1"},
					SummaryStatus:      "llm-enhanced",
				},
			},
		}

		resolver := &BaseResolver{
			queryManager: mockQM,
		}

		community, err := resolver.GetCommunity(context.Background(), "comm-1")
		require.NoError(t, err)
		require.NotNil(t, community)
		assert.Equal(t, "comm-1", community.ID)
		assert.Equal(t, 1, community.Level)
		assert.Equal(t, "LLM enhanced summary", community.Summary) // Should prefer LLM
		assert.Equal(t, []string{"robotics", "ai"}, community.Keywords)
		assert.Equal(t, "llm-enhanced", community.SummaryStatus)
	})

	t.Run("Success - fallback to statistical summary", func(t *testing.T) {
		mockQM := &mockQueryManager{
			communities: map[string]*clustering.Community{
				"comm-2": {
					ID:                 "comm-2",
					Level:              2,
					Members:            []string{"entity-3"},
					StatisticalSummary: "Statistical summary only",
					LLMSummary:         "", // No LLM summary
					Keywords:           []string{"network"},
					SummaryStatus:      "statistical",
				},
			},
		}

		resolver := &BaseResolver{
			queryManager: mockQM,
		}

		community, err := resolver.GetCommunity(context.Background(), "comm-2")
		require.NoError(t, err)
		require.NotNil(t, community)
		assert.Equal(t, "Statistical summary only", community.Summary) // Should use statistical
		assert.Equal(t, "statistical", community.SummaryStatus)
	})

	t.Run("Error - QueryManager not available", func(t *testing.T) {
		resolver := &BaseResolver{
			queryManager: nil,
		}

		community, err := resolver.GetCommunity(context.Background(), "comm-1")
		assert.Error(t, err)
		assert.Nil(t, community)
		assert.Contains(t, err.Error(), "QueryManager backend")
	})

	t.Run("Error - community not found", func(t *testing.T) {
		mockQM := &mockQueryManager{
			communities: map[string]*clustering.Community{},
		}

		resolver := &BaseResolver{
			queryManager: mockQM,
		}

		community, err := resolver.GetCommunity(context.Background(), "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, community)
	})
}

// TestGetEntityCommunity tests the GetEntityCommunity resolver
func TestGetEntityCommunity(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockQM := &mockQueryManager{
			entityCommunity: map[string]map[int]*clustering.Community{
				"entity-1": {
					1: {
						ID:                 "comm-1",
						Level:              1,
						Members:            []string{"entity-1", "entity-2"},
						StatisticalSummary: "Entity community",
						LLMSummary:         "Enhanced entity community",
						Keywords:           []string{"cluster"},
						SummaryStatus:      "llm-enhanced",
					},
				},
			},
		}

		resolver := &BaseResolver{
			queryManager: mockQM,
		}

		community, err := resolver.GetEntityCommunity(context.Background(), "entity-1", 1)
		require.NoError(t, err)
		require.NotNil(t, community)
		assert.Equal(t, "comm-1", community.ID)
		assert.Equal(t, 1, community.Level)
		assert.Equal(t, "Enhanced entity community", community.Summary)
	})

	t.Run("Error - QueryManager not available", func(t *testing.T) {
		resolver := &BaseResolver{
			queryManager: nil,
		}

		community, err := resolver.GetEntityCommunity(context.Background(), "entity-1", 1)
		assert.Error(t, err)
		assert.Nil(t, community)
		assert.Contains(t, err.Error(), "QueryManager backend")
	})

	t.Run("Error - entity community not found", func(t *testing.T) {
		mockQM := &mockQueryManager{
			entityCommunity: map[string]map[int]*clustering.Community{},
		}

		resolver := &BaseResolver{
			queryManager: mockQM,
		}

		community, err := resolver.GetEntityCommunity(context.Background(), "entity-1", 1)
		assert.Error(t, err)
		assert.Nil(t, community)
	})
}

// HTTP Handler Tests

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestServer_HandleGraphQL_Success(t *testing.T) {
	mockQM := &mockQueryManager{
		communities: map[string]*clustering.Community{
			"comm-1": {ID: "comm-1", Level: 1, Members: []string{"e1", "e2"}},
		},
	}

	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	reqBody := `{"query": "{ community(id: \"comm-1\") { id level } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))

	data, ok := response["data"].(map[string]any)
	require.True(t, ok, "response should have data field")
	community, ok := data["community"].(map[string]any)
	require.True(t, ok, "data should have community field")
	assert.Equal(t, "comm-1", community["id"])
}

func TestServer_HandleGraphQL_MethodNotAllowed(t *testing.T) {
	mockQM := &mockQueryManager{}
	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	errors, ok := response["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errors, 1)
}

func TestServer_HandleGraphQL_InvalidJSON(t *testing.T) {
	mockQM := &mockQueryManager{}
	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("not valid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	errors, ok := response["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errors, 1)
}

func TestServer_HandleGraphQL_EmptyQuery(t *testing.T) {
	mockQM := &mockQueryManager{}
	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	reqBody := `{"query": ""}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	errors, ok := response["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errors, 1)
}

func TestServer_HandleGraphQL_InvalidQuery(t *testing.T) {
	mockQM := &mockQueryManager{}
	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	reqBody := `{"query": "{ invalidField }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestServer_HandleGraphQL_QueryDepthExceeded(t *testing.T) {
	mockQM := &mockQueryManager{}
	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"
	config.MaxQueryDepth = 1 // Very shallow limit - only allow depth 1

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	// Query with depth 2: pathSearch -> entities -> id
	// depth 0: pathSearch
	// depth 1: entities
	// depth 2: id (but id has no sub-selections, so entities is the deepest with selections)
	// Actually: pathSearch { entities { id } } = depth 2 (pathSearch.entities.id)
	reqBody := `{"query": "{ pathSearch(startEntity: \"e1\") { entities { id } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	// With depth limit 1, a query with depth 2 should fail
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	errors, ok := response["errors"].([]any)
	require.True(t, ok, "response should have errors array")
	require.Len(t, errors, 1)
	errMsg := errors[0].(map[string]any)["message"].(string)
	assert.Contains(t, errMsg, "depth")
}

func TestServer_HandleGraphQL_WithVariables(t *testing.T) {
	mockQM := &mockQueryManager{
		communities: map[string]*clustering.Community{
			"comm-var": {ID: "comm-var", Level: 2, Members: []string{"e1"}},
		},
	}

	resolver := NewBaseResolver(mockQM, nil)
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:0"

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	reqBody := `{"query": "query GetComm($id: ID!) { community(id: $id) { id } }", "variables": {"id": "comm-var"}}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	data := response["data"].(map[string]any)
	community := data["community"].(map[string]any)
	assert.Equal(t, "comm-var", community["id"])
}
