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

	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/querymanager"
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
				Enabled:          true,
				BindAddress:      ":9090",
				Path:             "/api/graphql",
				EnablePlayground: true,
				EnableCORS:       true,
				TimeoutStr:       "10s",
				MaxQueryDepth:    15,
			},
			wantErr: false,
		},
		{
			name: "disabled config skips validation",
			config: Config{
				Enabled:     false,
				BindAddress: "",
				Path:        "", // Would be invalid if enabled
				TimeoutStr:  "",
			},
			wantErr: false,
		},
		{
			name: "empty path defaults to /graphql when enabled",
			config: Config{
				Enabled:     true,
				BindAddress: ":8080",
				Path:        "", // Empty path
				TimeoutStr:  "5s",
			},
			wantErr: false,
		},
		{
			name: "invalid path (no leading slash)",
			config: Config{
				Enabled:     true,
				BindAddress: ":8080",
				Path:        "graphql",
				TimeoutStr:  "5s",
			},
			wantErr: true,
		},
		{
			name: "invalid timeout (too short)",
			config: Config{
				Enabled:     true,
				BindAddress: ":8080",
				Path:        "/graphql",
				TimeoutStr:  "10ms",
			},
			wantErr: true,
		},
		{
			name: "invalid max query depth (too high)",
			config: Config{
				Enabled:       true,
				BindAddress:   ":8080",
				Path:          "/graphql",
				TimeoutStr:    "5s",
				MaxQueryDepth: 100,
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
				// Verify empty path defaults to /graphql when enabled
				if tt.name == "empty path defaults to /graphql when enabled" {
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

// TestResolverCreation tests resolver creation
func TestResolverCreation(t *testing.T) {
	mockQM := &mockQueryManager{}

	// Test without metrics recorder
	resolver := NewResolver(mockQM, nil)
	assert.NotNil(t, resolver)
	assert.Equal(t, mockQM, resolver.queryManager)
	assert.Nil(t, resolver.metricsRecorder)

	// Test with metrics recorder
	mockRecorder := &mockMetricsRecorder{}
	resolverWithMetrics := NewResolver(mockQM, mockRecorder)
	assert.NotNil(t, resolverWithMetrics)
	assert.Equal(t, mockRecorder, resolverWithMetrics.metricsRecorder)
}

// TestMetricsRecorder tests the default metrics recorder
func TestMetricsRecorder(t *testing.T) {
	logger := slog.Default()
	recorder := NewMetricsRecorder(logger)

	t.Run("RecordMetrics_Success", func(t *testing.T) {
		err := recorder.RecordMetrics(context.Background(), "TestOp", func() error {
			return nil
		})
		assert.NoError(t, err)

		total, success, failed, _ := recorder.Stats()
		assert.Equal(t, uint64(1), total)
		assert.Equal(t, uint64(1), success)
		assert.Equal(t, uint64(0), failed)
	})

	t.Run("RecordMetrics_Error", func(t *testing.T) {
		testErr := fmt.Errorf("test error")
		err := recorder.RecordMetrics(context.Background(), "TestOp", func() error {
			return testErr
		})
		assert.Error(t, err)
		assert.Equal(t, testErr, err)

		total, success, failed, _ := recorder.Stats()
		assert.Equal(t, uint64(2), total)
		assert.Equal(t, uint64(1), success)
		assert.Equal(t, uint64(1), failed)
	})
}

// mockMetricsRecorder for testing
type mockMetricsRecorder struct {
	recordedOps []string
}

func (m *mockMetricsRecorder) RecordMetrics(_ context.Context, operation string, fn func() error) error {
	m.recordedOps = append(m.recordedOps, operation)
	return fn()
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
func (m *mockQueryManager) GetHierarchyStats(_ context.Context, _ string) (*querymanager.HierarchyStats, error) {
	return nil, nil
}
func (m *mockQueryManager) ListWithPrefix(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockQueryManager) SearchSimilar(_ context.Context, _ string, _ int) (*querymanager.SimilaritySearchResult, error) {
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

		resolver := NewResolver(mockQM, nil)

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

		resolver := NewResolver(mockQM, nil)

		community, err := resolver.GetCommunity(context.Background(), "comm-2")
		require.NoError(t, err)
		require.NotNil(t, community)
		assert.Equal(t, "Statistical summary only", community.Summary) // Should use statistical
		assert.Equal(t, "statistical", community.SummaryStatus)
	})

	t.Run("Error - QueryManager returns error", func(t *testing.T) {
		mockQM := &mockQueryManager{
			communities: map[string]*clustering.Community{},
		}

		resolver := NewResolver(mockQM, nil)

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

		resolver := NewResolver(mockQM, nil)

		community, err := resolver.GetEntityCommunity(context.Background(), "entity-1", 1)
		require.NoError(t, err)
		require.NotNil(t, community)
		assert.Equal(t, "comm-1", community.ID)
		assert.Equal(t, 1, community.Level)
		assert.Equal(t, "Enhanced entity community", community.Summary)
	})

	t.Run("Error - entity community not found", func(t *testing.T) {
		mockQM := &mockQueryManager{
			entityCommunity: map[string]map[int]*clustering.Community{},
		}

		resolver := NewResolver(mockQM, nil)

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

	resolver := NewResolver(mockQM, nil)
	config := Config{
		BindAddress: "127.0.0.1:0",
		Path:        "/graphql",
		TimeoutStr:  "30s",
	}
	require.NoError(t, config.Validate())

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
	resolver := NewResolver(mockQM, nil)
	config := Config{
		BindAddress: "127.0.0.1:0",
		Path:        "/graphql",
		TimeoutStr:  "30s",
	}
	require.NoError(t, config.Validate())

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
	resolver := NewResolver(mockQM, nil)
	config := Config{
		BindAddress: "127.0.0.1:0",
		Path:        "/graphql",
		TimeoutStr:  "30s",
	}
	require.NoError(t, config.Validate())

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
	resolver := NewResolver(mockQM, nil)
	config := Config{
		BindAddress: "127.0.0.1:0",
		Path:        "/graphql",
		TimeoutStr:  "30s",
	}
	require.NoError(t, config.Validate())

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

func TestServer_HandleGraphQL_WithVariables(t *testing.T) {
	mockQM := &mockQueryManager{
		communities: map[string]*clustering.Community{
			"comm-var": {ID: "comm-var", Level: 2, Members: []string{"e1"}},
		},
	}

	resolver := NewResolver(mockQM, nil)
	config := Config{
		BindAddress: "127.0.0.1:0",
		Path:        "/graphql",
		TimeoutStr:  "30s",
	}
	require.NoError(t, config.Validate())

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

func TestServer_HandleHealth(t *testing.T) {
	mockQM := &mockQueryManager{}
	resolver := NewResolver(mockQM, nil)
	config := Config{
		BindAddress: "127.0.0.1:0",
		Path:        "/graphql",
		TimeoutStr:  "30s",
	}
	require.NoError(t, config.Validate())

	server, err := NewServer(config, resolver, testLogger())
	require.NoError(t, err)
	require.NoError(t, server.Setup())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	// Server not running, should return unavailable
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
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

	resolver := NewResolver(mockQM, nil)

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
}
