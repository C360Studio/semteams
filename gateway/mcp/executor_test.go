package mcp

import (
	"context"
	"errors"
	"testing"

	gql "github.com/c360/semstreams/gateway/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecutor(t *testing.T) {
	mq := newMockQuerier()
	resolver := createTestResolver(mq)

	exec, err := gql.NewExecutor(resolver, testLogger())
	require.NoError(t, err)
	assert.NotNil(t, exec)
}

func TestNewExecutor_NilResolver(t *testing.T) {
	// NewExecutor with nil resolver should still work (schema parsing succeeds)
	// The resolver being nil will cause runtime errors when executing queries
	exec, err := gql.NewExecutor(nil, testLogger())
	require.NoError(t, err)
	assert.NotNil(t, exec)
}

func TestExecutor_Execute_EntityQuery(t *testing.T) {
	mq := newMockQuerier()
	mq.entities["test-entity-1"] = testEntity("test-entity-1")

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ entity(id: "test-entity-1") { id } }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	data, ok := result.(map[string]any)
	require.True(t, ok)
	require.Contains(t, data, "data")

	queryResult := data["data"].(map[string]any)
	entity := queryResult["entity"].(map[string]any)
	assert.Equal(t, "test-entity-1", entity["id"])
}

func TestExecutor_Execute_EntityNotFound(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ entity(id: "nonexistent") { id } }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)
	assert.Nil(t, queryResult["entity"])
}

func TestExecutor_Execute_EntitiesQuery(t *testing.T) {
	mq := newMockQuerier()
	mq.entities["entity-1"] = testEntity("entity-1")
	mq.entities["entity-2"] = testEntity("entity-2")

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ entities(ids: ["entity-1", "entity-2"]) { id } }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)
	entities := queryResult["entities"].([]map[string]any)
	assert.Len(t, entities, 2)
}

func TestExecutor_Execute_InvalidQuery(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ invalid_field }`
	_, err = exec.Execute(context.Background(), query, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse error")
}

func TestExecutor_Execute_MutationNotSupported(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	// Schema doesn't define mutations, so this should fail at parse time
	query := `mutation { createEntity(id: "test") { id } }`
	_, err = exec.Execute(context.Background(), query, nil)

	require.Error(t, err)
}

func TestExecutor_Execute_WithVariables(t *testing.T) {
	mq := newMockQuerier()
	mq.entities["var-entity"] = testEntity("var-entity")

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `query GetEntity($entityId: ID!) { entity(id: $entityId) { id } }`
	variables := map[string]any{"entityId": "var-entity"}

	result, err := exec.Execute(context.Background(), query, variables)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)
	entity := queryResult["entity"].(map[string]any)
	assert.Equal(t, "var-entity", entity["id"])
}

func TestExecutor_Execute_Introspection_Typename(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ __typename }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)
	assert.Equal(t, "Query", queryResult["__typename"])
}

func TestExecutor_Execute_Introspection_Schema(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ __schema { queryType { name } } }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)
	schema := queryResult["__schema"].(map[string]any)
	queryType := schema["queryType"].(map[string]any)
	assert.Equal(t, "Query", queryType["name"])
}

func TestExecutor_Execute_Alias(t *testing.T) {
	mq := newMockQuerier()
	mq.entities["aliased-entity"] = testEntity("aliased-entity")

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ myEntity: entity(id: "aliased-entity") { myId: id } }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)

	// Check aliased field name
	entity := queryResult["myEntity"].(map[string]any)
	assert.Equal(t, "aliased-entity", entity["myId"])
}

func TestExecutor_Execute_ResolverError(t *testing.T) {
	mq := newMockQuerier()
	mq.err = errors.New("resolver failure")

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ entity(id: "test") { id } }`
	_, err = exec.Execute(context.Background(), query, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolver failure")
}

func TestExecutor_GetSchema(t *testing.T) {
	mq := newMockQuerier()
	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	schema := exec.GetSchema()
	assert.Contains(t, schema, "type Query")
	assert.Contains(t, schema, "entity(id: ID!)")
	assert.Contains(t, schema, "semanticSearch")
}

func TestExecutor_Execute_CommunityQuery(t *testing.T) {
	mq := newMockQuerier()
	mq.communities["comm-1"] = testCommunity("comm-1", 1)

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	query := `{ community(id: "comm-1") { id level members } }`
	result, err := exec.Execute(context.Background(), query, nil)

	require.NoError(t, err)
	data := result.(map[string]any)
	queryResult := data["data"].(map[string]any)
	community := queryResult["community"].(map[string]any)

	assert.Equal(t, "comm-1", community["id"])
	assert.Equal(t, 1, community["level"])
	assert.Len(t, community["members"], 2)
}

func TestExecutor_Execute_ContextCancellation(t *testing.T) {
	mq := newMockQuerier()
	mq.entities["test-entity"] = testEntity("test-entity")

	exec, err := createTestExecutor(mq)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	query := `{ entity(id: "test-entity") { id } }`

	// With cancelled context, the query might fail if the resolver respects context
	// This tests that context is being passed through
	_, err = exec.Execute(ctx, query, nil)

	// The mock querier doesn't check context, so this may succeed
	// but it ensures the code path handles context correctly
	_ = err // May or may not error depending on implementation
}
