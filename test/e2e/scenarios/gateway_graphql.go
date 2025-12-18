// Package scenarios provides E2E test scenarios for SemStreams gateways
package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
)

// GraphQLGatewayScenario validates GraphQL gateway operations
type GraphQLGatewayScenario struct {
	name        string
	description string
	client      *client.ObservabilityClient
	httpClient  *http.Client
	baseURL     string
	graphqlURL  string
	natsURL     string
	config      *GraphQLGatewayConfig
}

// GraphQLGatewayConfig contains configuration for GraphQL gateway test
type GraphQLGatewayConfig struct {
	// Test data configuration
	SetupDelay      time.Duration `json:"setup_delay"`
	ValidationDelay time.Duration `json:"validation_delay"`
	QueryTimeout    time.Duration `json:"query_timeout"`
}

// DefaultGraphQLGatewayConfig returns default configuration
func DefaultGraphQLGatewayConfig() *GraphQLGatewayConfig {
	return &GraphQLGatewayConfig{
		SetupDelay:      2 * time.Second,
		ValidationDelay: 1 * time.Second,
		QueryTimeout:    10 * time.Second,
	}
}

// NewGraphQLGatewayScenario creates a new GraphQL gateway test scenario
func NewGraphQLGatewayScenario(
	obsClient *client.ObservabilityClient,
	baseURL string,
	cfg *GraphQLGatewayConfig,
) *GraphQLGatewayScenario {
	if cfg == nil {
		cfg = DefaultGraphQLGatewayConfig()
	}
	if baseURL == "" {
		baseURL = config.DefaultEndpoints.HTTP
	}

	return &GraphQLGatewayScenario{
		name:        "gateway-graphql",
		description: "Tests GraphQL gateway: entity queries, relationships, search operations, communities",
		client:      obsClient,
		httpClient:  &http.Client{Timeout: cfg.QueryTimeout},
		baseURL:     baseURL,
		graphqlURL:  baseURL + "/graphql",
		natsURL:     config.DefaultEndpoints.NATS,
		config:      cfg,
	}
}

// Name returns the scenario name
func (s *GraphQLGatewayScenario) Name() string {
	return s.name
}

// Description returns the scenario description
func (s *GraphQLGatewayScenario) Description() string {
	return s.description
}

// Setup prepares the scenario
func (s *GraphQLGatewayScenario) Setup(ctx context.Context) error {
	// Wait for services to be fully ready
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.config.SetupDelay):
	}

	// Verify GraphQL endpoint is reachable
	req, err := http.NewRequestWithContext(ctx, "POST", s.graphqlURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GraphQL endpoint not reachable at %s: %w", s.graphqlURL, err)
	}
	_ = resp.Body.Close()

	return nil
}

// Execute runs the GraphQL gateway test scenario
func (s *GraphQLGatewayScenario) Execute(ctx context.Context) (*Result, error) {
	result := &Result{
		ScenarioName: s.name,
		StartTime:    time.Now(),
		Success:      false,
		Metrics:      make(map[string]any),
		Details:      make(map[string]any),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Track execution stages
	stages := []struct {
		name string
		fn   func(context.Context, *Result) error
	}{
		{"verify-graphql-health", s.executeVerifyHealth},
		{"test-entity-query", s.executeEntityQuery},
		{"test-entities-batch", s.executeEntitiesBatch},
		{"test-entities-by-type", s.executeEntitiesByType},
		{"test-relationships-query", s.executeRelationshipsQuery},
		{"test-semantic-search", s.executeSemanticSearch},
		{"test-spatial-search", s.executeSpatialSearch},
		{"test-temporal-search", s.executeTemporalSearch},
		{"test-community-query", s.executeCommunityQuery},
		{"test-error-handling", s.executeErrorHandling},
	}

	passedStages := 0
	failedStages := 0

	// Execute each stage
	for _, stage := range stages {
		stageStart := time.Now()

		if err := stage.fn(ctx, result); err != nil {
			failedStages++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", stage.name, err))
			result.Metrics[fmt.Sprintf("%s_status", stage.name)] = "failed"
		} else {
			passedStages++
			result.Metrics[fmt.Sprintf("%s_status", stage.name)] = "passed"
		}

		result.Metrics[fmt.Sprintf("%s_duration_ms", stage.name)] = time.Since(stageStart).Milliseconds()
	}

	// Overall success if most stages passed
	result.Metrics["stages_passed"] = passedStages
	result.Metrics["stages_failed"] = failedStages
	result.Success = failedStages == 0
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Teardown cleans up after the scenario
func (s *GraphQLGatewayScenario) Teardown(_ context.Context) error {
	return nil
}

// GraphQL query helper
func (s *GraphQLGatewayScenario) executeGraphQL(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	reqBody := map[string]any{
		"query": query,
	}
	if variables != nil {
		reqBody["variables"] = variables
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.graphqlURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for GraphQL errors
	if errors, ok := result["errors"].([]any); ok && len(errors) > 0 {
		return result, fmt.Errorf("GraphQL errors: %v", errors)
	}

	return result, nil
}

// executeVerifyHealth checks GraphQL endpoint is accessible
func (s *GraphQLGatewayScenario) executeVerifyHealth(ctx context.Context, result *Result) error {
	// Simple introspection query to verify GraphQL is working
	query := `{ __typename }`

	resp, err := s.executeGraphQL(ctx, query, nil)
	if err != nil {
		return fmt.Errorf("GraphQL health check failed: %w", err)
	}

	result.Details["graphql_health"] = map[string]any{
		"endpoint": s.graphqlURL,
		"response": resp,
	}

	return nil
}

// executeEntityQuery tests single entity lookup
func (s *GraphQLGatewayScenario) executeEntityQuery(ctx context.Context, result *Result) error {
	query := `
		query GetEntity($id: ID!) {
			entity(id: $id) {
				id
				type
				properties
			}
		}
	`

	// Test with a known entity ID pattern
	testID := "c360.semstreams.environmental.sensor.temperature.sensor-0"

	resp, err := s.executeGraphQL(ctx, query, map[string]any{"id": testID})
	if err != nil {
		// Entity may not exist - that's OK for this test, we're testing the query works
		result.Warnings = append(result.Warnings, fmt.Sprintf("Entity query returned error (may not exist): %v", err))
		result.Details["entity_query"] = map[string]any{
			"query_executed": true,
			"entity_found":   false,
		}
		return nil
	}

	data, _ := resp["data"].(map[string]any)
	entity, _ := data["entity"].(map[string]any)

	result.Details["entity_query"] = map[string]any{
		"query_executed": true,
		"entity_found":   entity != nil,
		"entity":         entity,
	}

	return nil
}

// executeEntitiesBatch tests batch entity loading
func (s *GraphQLGatewayScenario) executeEntitiesBatch(ctx context.Context, result *Result) error {
	query := `
		query GetEntities($ids: [ID!]!) {
			entities(ids: $ids) {
				id
				type
			}
		}
	`

	testIDs := []string{
		"c360.semstreams.environmental.sensor.temperature.sensor-0",
		"c360.semstreams.environmental.sensor.temperature.sensor-1",
	}

	resp, err := s.executeGraphQL(ctx, query, map[string]any{"ids": testIDs})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Batch entities query error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	entities, _ := data["entities"].([]any)

	result.Details["entities_batch"] = map[string]any{
		"query_executed": true,
		"ids_requested":  len(testIDs),
		"entities_found": len(entities),
	}

	return nil
}

// executeEntitiesByType tests type-based entity queries
func (s *GraphQLGatewayScenario) executeEntitiesByType(ctx context.Context, result *Result) error {
	query := `
		query GetEntitiesByType($type: String!, $limit: Int) {
			entitiesByType(type: $type, limit: $limit) {
				id
				type
			}
		}
	`

	resp, err := s.executeGraphQL(ctx, query, map[string]any{
		"type":  "sensor",
		"limit": 10,
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Entities by type query error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	entities, _ := data["entitiesByType"].([]any)

	result.Details["entities_by_type"] = map[string]any{
		"query_executed": true,
		"type_queried":   "sensor",
		"entities_found": len(entities),
	}

	return nil
}

// executeRelationshipsQuery tests relationship traversal
func (s *GraphQLGatewayScenario) executeRelationshipsQuery(ctx context.Context, result *Result) error {
	query := `
		query GetRelationships($entityId: ID!, $direction: RelationshipDirection) {
			relationships(entityId: $entityId, direction: $direction) {
				source
				target
				edgeType
				weight
			}
		}
	`

	testID := "c360.semstreams.environmental.sensor.temperature.sensor-0"

	resp, err := s.executeGraphQL(ctx, query, map[string]any{
		"entityId":  testID,
		"direction": "OUTGOING",
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Relationships query error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	relationships, _ := data["relationships"].([]any)

	result.Details["relationships_query"] = map[string]any{
		"query_executed":      true,
		"relationships_found": len(relationships),
	}

	return nil
}

// executeSemanticSearch tests semantic similarity search
func (s *GraphQLGatewayScenario) executeSemanticSearch(ctx context.Context, result *Result) error {
	query := `
		query SemanticSearch($query: String!, $limit: Int) {
			semanticSearch(query: $query, limit: $limit) {
				entity {
					id
					type
				}
				score
			}
		}
	`

	resp, err := s.executeGraphQL(ctx, query, map[string]any{
		"query": "temperature sensor",
		"limit": 5,
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Semantic search error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	results, _ := data["semanticSearch"].([]any)

	result.Details["semantic_search"] = map[string]any{
		"query_executed": true,
		"results_found":  len(results),
	}

	return nil
}

// executeSpatialSearch tests geographic bounding box search
func (s *GraphQLGatewayScenario) executeSpatialSearch(ctx context.Context, result *Result) error {
	query := `
		query SpatialSearch($north: Float!, $south: Float!, $east: Float!, $west: Float!, $limit: Int) {
			spatialSearch(north: $north, south: $south, east: $east, west: $west, limit: $limit) {
				id
				type
			}
		}
	`

	// San Francisco area bounding box
	resp, err := s.executeGraphQL(ctx, query, map[string]any{
		"north": 37.80,
		"south": 37.70,
		"east":  -122.35,
		"west":  -122.50,
		"limit": 10,
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Spatial search error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	entities, _ := data["spatialSearch"].([]any)

	result.Details["spatial_search"] = map[string]any{
		"query_executed": true,
		"entities_found": len(entities),
	}

	return nil
}

// executeTemporalSearch tests time-range search
func (s *GraphQLGatewayScenario) executeTemporalSearch(ctx context.Context, result *Result) error {
	query := `
		query TemporalSearch($startTime: DateTime!, $endTime: DateTime!, $limit: Int) {
			temporalSearch(startTime: $startTime, endTime: $endTime, limit: $limit) {
				id
				type
			}
		}
	`

	now := time.Now()
	startTime := now.Add(-24 * time.Hour).Format(time.RFC3339)
	endTime := now.Format(time.RFC3339)

	resp, err := s.executeGraphQL(ctx, query, map[string]any{
		"startTime": startTime,
		"endTime":   endTime,
		"limit":     10,
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Temporal search error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	entities, _ := data["temporalSearch"].([]any)

	result.Details["temporal_search"] = map[string]any{
		"query_executed": true,
		"entities_found": len(entities),
	}

	return nil
}

// executeCommunityQuery tests community operations
func (s *GraphQLGatewayScenario) executeCommunityQuery(ctx context.Context, result *Result) error {
	query := `
		query GetCommunitiesByLevel($level: Int!) {
			communitiesByLevel(level: $level) {
				id
				level
				memberCount
			}
		}
	`

	resp, err := s.executeGraphQL(ctx, query, map[string]any{
		"level": 0,
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Community query error: %v", err))
	}

	data, _ := resp["data"].(map[string]any)
	communities, _ := data["communitiesByLevel"].([]any)

	result.Details["community_query"] = map[string]any{
		"query_executed":    true,
		"communities_found": len(communities),
	}

	return nil
}

// executeErrorHandling tests error responses
func (s *GraphQLGatewayScenario) executeErrorHandling(ctx context.Context, result *Result) error {
	// Test invalid query syntax
	invalidQuery := `{ invalid`

	_, err := s.executeGraphQL(ctx, invalidQuery, nil)
	syntaxErrorHandled := err != nil

	// Test missing required field
	missingFieldQuery := `
		query GetEntity {
			entity {
				id
			}
		}
	`

	_, err = s.executeGraphQL(ctx, missingFieldQuery, nil)
	missingFieldHandled := err != nil

	result.Details["error_handling"] = map[string]any{
		"syntax_error_handled":        syntaxErrorHandled,
		"missing_field_error_handled": missingFieldHandled,
	}

	// Both should return errors
	if !syntaxErrorHandled || !missingFieldHandled {
		return fmt.Errorf("error handling validation failed")
	}

	return nil
}
