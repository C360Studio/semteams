// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Structural variant validation functions for rules-only testing

// executeValidateZeroEmbeddings validates that NO embeddings were generated (structural tier constraint)
func (s *TieredScenario) executeValidateZeroEmbeddings(ctx context.Context, result *Result) error {
	embeddingCount, _ := s.metrics.SumMetricsByName(ctx, "indexengine_embeddings_generated_total")

	result.Metrics["embeddings_generated"] = int(embeddingCount)

	constraintMet := int(embeddingCount) <= s.config.ExpectedEmbeddings

	result.Details["zero_embeddings_validation"] = map[string]any{
		"embeddings_generated": int(embeddingCount),
		"expected":             s.config.ExpectedEmbeddings,
		"constraint_met":       constraintMet,
		"message":              fmt.Sprintf("Embeddings: %d (expected %d for structural tier)", int(embeddingCount), s.config.ExpectedEmbeddings),
	}

	// Structural tier constraint: embeddings MUST be zero (or within expected limit)
	if !constraintMet {
		return fmt.Errorf("structural tier constraint violated: embeddings=%d (expected max %d)",
			int(embeddingCount), s.config.ExpectedEmbeddings)
	}

	return nil
}

// executeValidateZeroClusters validates that NO clustering occurred (structural tier constraint)
func (s *TieredScenario) executeValidateZeroClusters(ctx context.Context, result *Result) error {
	clusteringCount, _ := s.metrics.SumMetricsByName(ctx, "semstreams_clustering_runs_total")

	result.Metrics["clustering_runs"] = int(clusteringCount)

	constraintMet := int(clusteringCount) <= s.config.ExpectedClusters

	result.Details["zero_clusters_validation"] = map[string]any{
		"clustering_runs": int(clusteringCount),
		"expected":        s.config.ExpectedClusters,
		"constraint_met":  constraintMet,
		"message":         fmt.Sprintf("Clustering runs: %d (expected %d for structural tier)", int(clusteringCount), s.config.ExpectedClusters),
	}

	// Structural tier constraint: clustering MUST NOT occur
	if !constraintMet {
		return fmt.Errorf("structural tier constraint violated: clustering_runs=%d (expected max %d)",
			int(clusteringCount), s.config.ExpectedClusters)
	}

	return nil
}

// executeValidateRuleTransitions validates stateful rule OnEnter/OnExit transitions (structural tier)
func (s *TieredScenario) executeValidateRuleTransitions(ctx context.Context, result *Result) error {
	// Get rule metrics using MetricsClient
	ruleMetrics, err := s.metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to extract rule metrics: %v", err))
		return nil
	}

	onEnterFired := int(ruleMetrics.OnEnterFired)
	onExitFired := int(ruleMetrics.OnExitFired)

	result.Metrics["on_enter_fired"] = onEnterFired
	result.Metrics["on_exit_fired"] = onExitFired

	// Validate minimum state transitions
	violations := []string{}
	if onEnterFired < s.config.MinOnEnterFired {
		violations = append(violations,
			fmt.Sprintf("OnEnter: %d < %d (expected)", onEnterFired, s.config.MinOnEnterFired))
	}
	if onExitFired < s.config.MinOnExitFired {
		violations = append(violations,
			fmt.Sprintf("OnExit: %d < %d (expected)", onExitFired, s.config.MinOnExitFired))
	}

	result.Details["rule_transitions_validation"] = map[string]any{
		"on_enter_fired":    onEnterFired,
		"on_exit_fired":     onExitFired,
		"min_on_enter":      s.config.MinOnEnterFired,
		"min_on_exit":       s.config.MinOnExitFired,
		"violations":        violations,
		"validation_passed": len(violations) == 0,
		"stateful_behavior": onEnterFired > 0 || onExitFired > 0,
		"dynamic_graph":     onExitFired > 0, // OnExit removes triples = dynamic graph
		"message":           fmt.Sprintf("Rule transitions: %d OnEnter, %d OnExit", onEnterFired, onExitFired),
	}

	if len(violations) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rule transition validation issues: %v", violations))
	}

	return nil
}

// pathRAGResponse represents the parsed GraphQL response for PathRAG queries
type pathRAGResponse struct {
	Data struct {
		PathSearch struct {
			Entities  []pathRAGEntity `json:"entities"`
			Paths     [][]pathRAGStep `json:"paths"` // Each path is a sequence of steps
			Truncated bool            `json:"truncated"`
		} `json:"pathSearch"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type pathRAGEntity struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Score float64 `json:"score"`
}

type pathRAGStep struct {
	From      string `json:"from"`
	Predicate string `json:"predicate"`
	To        string `json:"to"`
}

// executeTestPathRAGSensor validates PathRAG traversal using a sensor entity.
// Sensor entities demonstrate EntityID sibling inference (structured IoT data).
// PathRAG is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestPathRAGSensor(ctx context.Context, result *Result) error {
	startEntity := s.getPathRAGSensorEntity()
	gatewayURL := s.config.GraphQLURL

	resp, latency, err := s.sendPathRAGRequest(ctx, startEntity, gatewayURL)
	if err != nil {
		result.Details["pathrag_sensor_test"] = map[string]any{
			"start_entity": startEntity, "error": err.Error(), "gateway_url": gatewayURL,
		}
		return err
	}

	result.Metrics["pathrag_sensor_latency_ms"] = latency.Milliseconds()
	return s.validatePathRAGResultNamed(resp, startEntity, latency, result, "pathrag_sensor_test")
}

// executeTestPathRAGDocument validates PathRAG traversal using a document entity.
// Document entities demonstrate text-based similarity (statistical/semantic enhancements).
// PathRAG is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestPathRAGDocument(ctx context.Context, result *Result) error {
	startEntity := s.getPathRAGDocumentEntity()
	gatewayURL := s.config.GraphQLURL

	resp, latency, err := s.sendPathRAGRequest(ctx, startEntity, gatewayURL)
	if err != nil {
		result.Details["pathrag_document_test"] = map[string]any{
			"start_entity": startEntity, "error": err.Error(), "gateway_url": gatewayURL,
		}
		return err
	}

	result.Metrics["pathrag_document_latency_ms"] = latency.Milliseconds()
	return s.validatePathRAGResultNamed(resp, startEntity, latency, result, "pathrag_document_test")
}

// getPathRAGSensorEntity returns a sensor entity for PathRAG testing.
// All tiers now use testdata/semantic/sensors.jsonl which contains temperature sensors.
// Sensor entities demonstrate EntityID sibling inference (structural IoT data).
func (s *TieredScenario) getPathRAGSensorEntity() string {
	// All tiers use testdata/semantic/sensors.jsonl
	// Entity IDs follow format: {org}.{platform}.environmental.sensor.{type}.{device_id}
	// From sensors.jsonl: device_id=temp-sensor-001, type=temperature
	// Config: org_id=c360, platform=logistics
	return "c360.logistics.environmental.sensor.temperature.temp-sensor-001"
}

// getPathRAGDocumentEntity returns a document entity for PathRAG testing.
// All tiers use testdata/semantic/maintenance.jsonl which contains maintenance records.
// Document entities demonstrate text-based similarity (statistical/semantic enhancements).
func (s *TieredScenario) getPathRAGDocumentEntity() string {
	// All tiers use testdata/semantic/maintenance.jsonl
	// Use maintenance entity which has 15+ siblings with same type prefix
	// This allows sibling inference to find related entities
	return "c360.logistics.maintenance.work.completed.maint-001"
}

// sendPathRAGRequest sends the PathRAG GraphQL query and returns the parsed response
// Uses includeSiblings=true to leverage EntityID hierarchy for sibling detection
func (s *TieredScenario) sendPathRAGRequest(ctx context.Context, startEntity, gatewayURL string) (*pathRAGResponse, time.Duration, error) {
	graphqlQuery := map[string]any{
		"query": `query($startEntity: ID!, $maxDepth: Int, $maxNodes: Int, $includeSiblings: Boolean) {
			pathSearch(startEntity: $startEntity, maxDepth: $maxDepth, maxNodes: $maxNodes, includeSiblings: $includeSiblings) {
				entities { id type score } paths { from predicate to } truncated
			}}`,
		"variables": map[string]any{"startEntity": startEntity, "maxDepth": 2, "maxNodes": 10, "includeSiblings": true},
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal PathRAG query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create PathRAG request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, fmt.Errorf("PathRAG request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, latency, fmt.Errorf("PathRAG returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, fmt.Errorf("failed to read PathRAG response: %w", err)
	}

	var graphqlResp pathRAGResponse
	if err := json.Unmarshal(bodyBytes, &graphqlResp); err != nil {
		return nil, latency, fmt.Errorf("failed to parse PathRAG response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, latency, fmt.Errorf("PathRAG GraphQL error: %s", graphqlResp.Errors[0].Message)
	}

	return &graphqlResp, latency, nil
}

// validatePathRAGResult validates the PathRAG response and records results (backward compatible)
func (s *TieredScenario) validatePathRAGResult(resp *pathRAGResponse, startEntity string, latency time.Duration, result *Result) error {
	return s.validatePathRAGResultNamed(resp, startEntity, latency, result, "pathrag_test")
}

// validatePathRAGResultNamed validates the PathRAG response and records results with a custom test name
func (s *TieredScenario) validatePathRAGResultNamed(resp *pathRAGResponse, startEntity string, latency time.Duration, result *Result, testName string) error {
	ps := resp.Data.PathSearch
	entityCount := len(ps.Entities)
	// Count total paths (each path is a sequence of steps)
	pathCount := len(ps.Paths)

	// Use test-specific metric names
	metricsPrefix := testName[:len(testName)-5] // Remove "_test" suffix
	result.Metrics[metricsPrefix+"_entities_found"] = entityCount
	result.Metrics[metricsPrefix+"_paths_found"] = pathCount

	if entityCount == 0 {
		result.Details[testName] = map[string]any{
			"start_entity": startEntity, "entities_found": 0, "message": "No entities returned",
		}
		return fmt.Errorf("PathRAG returned no entities for start entity %s", startEntity)
	}

	// Verify scores decrease with depth (decay factor working)
	// This is a hard failure - with controlled input, decay scoring should be deterministic
	scoresValid := true
	var decayViolation string
	prevScore := 2.0
	entityIDs := make([]string, 0, len(ps.Entities))
	entityScores := make([]float64, 0, len(ps.Entities))
	for i, e := range ps.Entities {
		entityIDs = append(entityIDs, e.ID)
		entityScores = append(entityScores, e.Score)
		if i > 0 && e.Score > prevScore {
			scoresValid = false
			decayViolation = fmt.Sprintf("entity %s has score %.3f > previous %.3f", e.ID, e.Score, prevScore)
		}
		prevScore = e.Score
	}

	result.Details[testName] = map[string]any{
		"start_entity": startEntity, "entities_found": entityCount, "paths_found": pathCount,
		"truncated": ps.Truncated, "entity_ids": entityIDs, "entity_scores": entityScores,
		"scores_valid": scoresValid, "latency_ms": latency.Milliseconds(),
		"message": fmt.Sprintf("PathRAG traversal successful: found %d entities via %d paths", entityCount, pathCount),
	}

	// Hard failure on decay scoring violation - input is controlled, results should be deterministic
	if !scoresValid {
		return fmt.Errorf("PathRAG decay scoring violated: %s", decayViolation)
	}

	return nil
}

// executeTestEntityIDHierarchy validates the EntityID hierarchy GraphQL queries.
// This tests that the 6-part EntityID structure can be navigated via GraphQL.
// EntityID hierarchy is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestEntityIDHierarchy(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GraphQLURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test 1: Get hierarchy stats from root
	hierarchyQuery := map[string]any{
		"query": `query($prefix: String) {
			entityIdHierarchy(prefix: $prefix) {
				prefix totalEntities children { prefix name count }
			}}`,
		"variables": map[string]any{"prefix": ""},
	}

	queryJSON, err := json.Marshal(hierarchyQuery)
	if err != nil {
		return fmt.Errorf("failed to marshal hierarchy query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return fmt.Errorf("failed to create hierarchy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return fmt.Errorf("hierarchy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hierarchy returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read hierarchy response: %w", err)
	}

	var hierarchyResp struct {
		Data struct {
			EntityIDHierarchy struct {
				Prefix        string `json:"prefix"`
				TotalEntities int    `json:"totalEntities"`
				Children      []struct {
					Prefix string `json:"prefix"`
					Name   string `json:"name"`
					Count  int    `json:"count"`
				} `json:"children"`
			} `json:"entityIdHierarchy"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &hierarchyResp); err != nil {
		return fmt.Errorf("failed to parse hierarchy response: %w", err)
	}

	if len(hierarchyResp.Errors) > 0 {
		return fmt.Errorf("hierarchy GraphQL error: %s", hierarchyResp.Errors[0].Message)
	}

	hierarchy := hierarchyResp.Data.EntityIDHierarchy

	result.Metrics["hierarchy_total_entities"] = hierarchy.TotalEntities
	result.Metrics["hierarchy_children_count"] = len(hierarchy.Children)
	result.Metrics["hierarchy_latency_ms"] = latency.Milliseconds()

	// Validate we found entities
	if hierarchy.TotalEntities == 0 {
		result.Details["entityid_hierarchy_test"] = map[string]any{
			"prefix":         "",
			"total_entities": 0,
			"error":          "No entities found in hierarchy",
		}
		return fmt.Errorf("entityIdHierarchy returned 0 entities")
	}

	// Validate we have at least one child level (org level should have platforms)
	if len(hierarchy.Children) == 0 {
		result.Details["entityid_hierarchy_test"] = map[string]any{
			"prefix":         "",
			"total_entities": hierarchy.TotalEntities,
			"error":          "No children found at root level",
		}
		return fmt.Errorf("entityIdHierarchy returned no children at root level")
	}

	// Collect child info for logging
	childInfo := make([]map[string]any, len(hierarchy.Children))
	for i, child := range hierarchy.Children {
		childInfo[i] = map[string]any{
			"prefix": child.Prefix,
			"name":   child.Name,
			"count":  child.Count,
		}
	}

	result.Details["entityid_hierarchy_test"] = map[string]any{
		"prefix":         "",
		"total_entities": hierarchy.TotalEntities,
		"children":       childInfo,
		"latency_ms":     latency.Milliseconds(),
		"message":        fmt.Sprintf("Hierarchy query successful: %d entities across %d org-level children", hierarchy.TotalEntities, len(hierarchy.Children)),
	}

	return nil
}

// executeTestEntitiesByPrefix validates the entitiesByPrefix GraphQL query.
// This tests that entities can be queried by EntityID prefix.
// EntityID prefix query is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestEntitiesByPrefix(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GraphQLURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test: Query entities by prefix (all temperature sensors)
	prefix := "c360.logistics.environmental.sensor.temperature"
	prefixQuery := map[string]any{
		"query": `query($prefix: String!, $limit: Int) {
			entitiesByPrefix(prefix: $prefix, limit: $limit) {
				entityIds totalCount truncated prefix
			}}`,
		"variables": map[string]any{"prefix": prefix, "limit": 100},
	}

	queryJSON, err := json.Marshal(prefixQuery)
	if err != nil {
		return fmt.Errorf("failed to marshal prefix query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return fmt.Errorf("failed to create prefix request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return fmt.Errorf("prefix request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("prefix query returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read prefix response: %w", err)
	}

	var prefixResp struct {
		Data struct {
			EntitiesByPrefix struct {
				EntityIDs  []string `json:"entityIds"`
				TotalCount int      `json:"totalCount"`
				Truncated  bool     `json:"truncated"`
				Prefix     string   `json:"prefix"`
			} `json:"entitiesByPrefix"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &prefixResp); err != nil {
		return fmt.Errorf("failed to parse prefix response: %w", err)
	}

	if len(prefixResp.Errors) > 0 {
		return fmt.Errorf("prefix query GraphQL error: %s", prefixResp.Errors[0].Message)
	}

	prefixResult := prefixResp.Data.EntitiesByPrefix

	result.Metrics["prefix_query_total_count"] = prefixResult.TotalCount
	result.Metrics["prefix_query_returned"] = len(prefixResult.EntityIDs)
	result.Metrics["prefix_query_latency_ms"] = latency.Milliseconds()

	// We expect at least 1 temperature sensor from the test data
	if prefixResult.TotalCount == 0 {
		result.Details["entities_by_prefix_test"] = map[string]any{
			"prefix":      prefix,
			"total_count": 0,
			"error":       "No entities found for temperature sensor prefix",
		}
		return fmt.Errorf("entitiesByPrefix returned 0 entities for prefix %s", prefix)
	}

	// Verify all returned entity IDs match the prefix
	for _, entityID := range prefixResult.EntityIDs {
		if !strings.HasPrefix(entityID, prefix) {
			result.Details["entities_by_prefix_test"] = map[string]any{
				"prefix":    prefix,
				"entity_id": entityID,
				"error":     "Entity ID does not match prefix",
			}
			return fmt.Errorf("entity %s does not match prefix %s", entityID, prefix)
		}
	}

	result.Details["entities_by_prefix_test"] = map[string]any{
		"prefix":      prefix,
		"total_count": prefixResult.TotalCount,
		"returned":    len(prefixResult.EntityIDs),
		"truncated":   prefixResult.Truncated,
		"entity_ids":  prefixResult.EntityIDs,
		"latency_ms":  latency.Milliseconds(),
		"message":     fmt.Sprintf("Prefix query successful: found %d temperature sensors", prefixResult.TotalCount),
	}

	return nil
}

// executeTestSpatialQuery validates spatial index queries via GraphQL.
// Tests that entities can be found using bounding box search.
// Spatial query is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestSpatialQuery(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GraphQLURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test data is in SF Bay Area: ~37.77, -122.42
	// Create bounding box that should include all test sensors
	spatialQuery := map[string]any{
		"query": `query($north: Float!, $south: Float!, $east: Float!, $west: Float!, $limit: Int) {
			spatialSearch(north: $north, south: $south, east: $east, west: $west, limit: $limit) {
				id type
			}}`,
		"variables": map[string]any{
			"north": 37.78,
			"south": 37.77,
			"east":  -122.41,
			"west":  -122.43,
			"limit": 100,
		},
	}

	queryJSON, err := json.Marshal(spatialQuery)
	if err != nil {
		return fmt.Errorf("failed to marshal spatial query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return fmt.Errorf("failed to create spatial request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return fmt.Errorf("spatial request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("spatial query returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read spatial response: %w", err)
	}

	var spatialResp struct {
		Data struct {
			SpatialSearch []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"spatialSearch"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &spatialResp); err != nil {
		return fmt.Errorf("failed to parse spatial response: %w", err)
	}

	if len(spatialResp.Errors) > 0 {
		return fmt.Errorf("spatial query GraphQL error: %s", spatialResp.Errors[0].Message)
	}

	entityCount := len(spatialResp.Data.SpatialSearch)
	result.Metrics["spatial_query_count"] = entityCount
	result.Metrics["spatial_query_latency_ms"] = latency.Milliseconds()

	// Collect entity IDs for logging
	entityIDs := make([]string, entityCount)
	for i, e := range spatialResp.Data.SpatialSearch {
		entityIDs[i] = e.ID
	}

	result.Details["spatial_query_test"] = map[string]any{
		"bounds": map[string]float64{
			"north": 37.78, "south": 37.77, "east": -122.41, "west": -122.43,
		},
		"entities_found": entityCount,
		"entity_ids":     entityIDs,
		"latency_ms":     latency.Milliseconds(),
		"message":        fmt.Sprintf("Spatial query returned %d entities within bounding box", entityCount),
	}

	// Note: We don't require a minimum count since spatial indexing depends on
	// the processor creating geo.location.* triples. If count is 0, it's a warning.
	if entityCount == 0 {
		result.Warnings = append(result.Warnings, "Spatial query returned 0 entities - check if geo triples are being indexed")
	}

	return nil
}

// executeTestTemporalQuery validates temporal index queries via GraphQL.
// Tests that entities can be found using time range search.
// Temporal query is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestTemporalQuery(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GraphQLURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Temporal index uses entity UpdatedAt (current time), not historical timestamps from test data.
	// Query for entities updated in the last hour to capture recently processed entities.
	now := time.Now().UTC()
	startTime := now.Add(-1 * time.Hour).Format(time.RFC3339)
	endTime := now.Add(1 * time.Hour).Format(time.RFC3339)

	temporalQuery := map[string]any{
		"query": `query($startTime: DateTime!, $endTime: DateTime!, $limit: Int) {
			temporalSearch(startTime: $startTime, endTime: $endTime, limit: $limit) {
				id type
			}}`,
		"variables": map[string]any{
			"startTime": startTime,
			"endTime":   endTime,
			"limit":     100,
		},
	}

	queryJSON, err := json.Marshal(temporalQuery)
	if err != nil {
		return fmt.Errorf("failed to marshal temporal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return fmt.Errorf("failed to create temporal request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return fmt.Errorf("temporal request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("temporal query returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read temporal response: %w", err)
	}

	var temporalResp struct {
		Data struct {
			TemporalSearch []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"temporalSearch"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &temporalResp); err != nil {
		return fmt.Errorf("failed to parse temporal response: %w", err)
	}

	if len(temporalResp.Errors) > 0 {
		return fmt.Errorf("temporal query GraphQL error: %s", temporalResp.Errors[0].Message)
	}

	entityCount := len(temporalResp.Data.TemporalSearch)
	result.Metrics["temporal_query_count"] = entityCount
	result.Metrics["temporal_query_latency_ms"] = latency.Milliseconds()

	// Collect entity IDs for logging (limit to first 10 for brevity)
	maxDisplay := 10
	if entityCount < maxDisplay {
		maxDisplay = entityCount
	}
	entityIDs := make([]string, maxDisplay)
	for i := 0; i < maxDisplay; i++ {
		entityIDs[i] = temporalResp.Data.TemporalSearch[i].ID
	}

	result.Details["temporal_query_test"] = map[string]any{
		"time_range": map[string]string{
			"start": "2024-11-15T00:00:00Z",
			"end":   "2024-11-16T00:00:00Z",
		},
		"entities_found":    entityCount,
		"entity_ids_sample": entityIDs,
		"latency_ms":        latency.Milliseconds(),
		"message":           fmt.Sprintf("Temporal query returned %d entities within time range", entityCount),
	}

	// Note: We don't require a minimum count since temporal indexing depends on
	// entity UpdatedAt timestamps. If count is 0, it's a warning.
	if entityCount == 0 {
		result.Warnings = append(result.Warnings, "Temporal query returned 0 entities - check if temporal index is being populated")
	}

	return nil
}

// executeTestZoneRelationships validates zone-based relationship queries.
// Tests that querying a zone entity's incoming edges returns all sensors in that zone.
// This validates the geo.location.zone relationship triple indexing.
func (s *TieredScenario) executeTestZoneRelationships(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GraphQLURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Zone entity ID format: {org}.{platform}.facility.zone.{zoneType}.{locationID}
	// From test data sensors.jsonl, "cold-storage-1" is a known location with default zone type "area"
	// The IoT processor generates: c360.logistics.facility.zone.area.cold-storage-1
	zoneEntityID := "c360.logistics.facility.zone.area.cold-storage-1"

	// Query incoming relationships to the zone entity
	relationshipsQuery := map[string]any{
		"query": `query($entityId: ID!, $direction: RelationshipDirection) {
			relationships(entityId: $entityId, direction: $direction) {
				fromEntityId toEntityId edgeType
			}}`,
		"variables": map[string]any{
			"entityId":  zoneEntityID,
			"direction": "INCOMING",
		},
	}

	queryJSON, err := json.Marshal(relationshipsQuery)
	if err != nil {
		return fmt.Errorf("failed to marshal relationships query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return fmt.Errorf("failed to create relationships request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return fmt.Errorf("relationships request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("relationships query returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read relationships response: %w", err)
	}

	var relationshipsResp struct {
		Data struct {
			Relationships []struct {
				FromEntityID string `json:"fromEntityId"`
				ToEntityID   string `json:"toEntityId"`
				EdgeType     string `json:"edgeType"`
			} `json:"relationships"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &relationshipsResp); err != nil {
		return fmt.Errorf("failed to parse relationships response: %w", err)
	}

	if len(relationshipsResp.Errors) > 0 {
		return fmt.Errorf("relationships query GraphQL error: %s", relationshipsResp.Errors[0].Message)
	}

	relationships := relationshipsResp.Data.Relationships
	relationshipCount := len(relationships)
	result.Metrics["zone_relationships_count"] = relationshipCount
	result.Metrics["zone_relationships_latency_ms"] = latency.Milliseconds()

	// Count relationships by edge type
	edgeTypeCounts := make(map[string]int)
	sensorIDs := []string{}
	for _, rel := range relationships {
		edgeTypeCounts[rel.EdgeType]++
		// Collect sensor IDs (entities pointing to this zone)
		if rel.EdgeType == "geo.location.zone" {
			sensorIDs = append(sensorIDs, rel.FromEntityID)
		}
	}

	result.Details["zone_relationships_test"] = map[string]any{
		"zone_entity_id":      zoneEntityID,
		"total_relationships": relationshipCount,
		"edge_type_counts":    edgeTypeCounts,
		"sensor_ids":          sensorIDs,
		"latency_ms":          latency.Milliseconds(),
		"message":             fmt.Sprintf("Zone %s has %d incoming relationships", zoneEntityID, relationshipCount),
	}

	// Note: We don't require a minimum count since this depends on the zone existing
	// and sensors being in that zone. If count is 0, it's a warning.
	if relationshipCount == 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Zone %s has 0 incoming relationships - check if zone triples are being indexed", zoneEntityID))
	}

	return nil
}

// executeTestPathRAGBoundary validates PathRAG respects maxNodes limit
// PathRAG is a Tier 0 capability that runs on ALL tiers.
func (s *TieredScenario) executeTestPathRAGBoundary(ctx context.Context, result *Result) error {
	startEntity := s.getPathRAGSensorEntity()
	gatewayURL := s.config.GraphQLURL

	// Query with tight bounds to verify maxNodes is respected
	graphqlQuery := map[string]any{
		"query": `query($startEntity: ID!, $maxDepth: Int, $maxNodes: Int) {
			pathSearch(startEntity: $startEntity, maxDepth: $maxDepth, maxNodes: $maxNodes) {
				entities { id type score } paths { from predicate to } truncated
			}}`,
		"variables": map[string]any{"startEntity": startEntity, "maxDepth": 2, "maxNodes": 3},
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		return fmt.Errorf("failed to marshal PathRAG boundary query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return fmt.Errorf("failed to create PathRAG boundary request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PathRAG boundary request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PathRAG boundary returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read PathRAG boundary response: %w", err)
	}

	var graphqlResp pathRAGResponse
	if err := json.Unmarshal(bodyBytes, &graphqlResp); err != nil {
		return fmt.Errorf("failed to parse PathRAG boundary response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return fmt.Errorf("PathRAG boundary GraphQL error: %s", graphqlResp.Errors[0].Message)
	}

	// Verify result count respects maxNodes limit
	// Note: maxNodes refers to traversal nodes, but start entity is always included
	// So total entities = start entity (1) + up to maxNodes traversed nodes
	entityCount := len(graphqlResp.Data.PathSearch.Entities)
	maxNodes := 3
	expectedMax := maxNodes + 1 // +1 for start entity which is always included

	result.Metrics["pathrag_boundary_entities"] = entityCount
	result.Metrics["pathrag_boundary_max_nodes"] = maxNodes
	result.Details["pathrag_boundary_test"] = map[string]any{
		"entities_returned":     entityCount,
		"max_nodes_limit":       maxNodes,
		"expected_max_total":    expectedMax,
		"respected_limit":       entityCount <= expectedMax,
		"includes_start_entity": true,
	}

	if entityCount > expectedMax {
		return fmt.Errorf("PathRAG maxNodes violated: got %d entities, expected <= %d (maxNodes=%d + start entity)", entityCount, expectedMax, maxNodes)
	}

	return nil
}

// executeValidateKCoreIndexStructural validates k-core decomposition for the structural tier.
// Unlike statistical/semantic tiers which rely on community detection edges, structural tier
// uses EntityID sibling edges (6-part hierarchy) to create graph structure for k-core.
//
// Test data expectation: Multiple sensors of the same type (e.g., temperature sensors)
// share a 5-part type prefix and form sibling edges. With 3+ siblings, we expect max_core >= 2.
func (s *TieredScenario) executeValidateKCoreIndexStructural(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping k-core validation")
		return nil
	}

	info, err := s.natsClient.GetStructuralIndexInfo(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get structural index info: %v", err))
		return nil
	}

	if !info.BucketExists {
		result.Warnings = append(result.Warnings, "STRUCTURAL_INDEX bucket does not exist - check if structural_index.enabled=true")
		return nil
	}

	if info.KCore == nil {
		result.Warnings = append(result.Warnings, "K-core metadata not found in STRUCTURAL_INDEX")
		result.Details["kcore_validation_structural"] = map[string]any{
			"valid":   false,
			"error":   "metadata not found",
			"message": "K-core index not computed - check graph_analysis.structural_index and entityid_edges config",
		}
		return nil
	}

	kcore := info.KCore
	valid := true
	issues := []string{}

	// Validate entity count
	if kcore.EntityCount == 0 {
		issues = append(issues, "entity count is 0")
		valid = false
	}

	// Validate MaxCore (should be >= 0)
	if kcore.MaxCore < 0 {
		issues = append(issues, fmt.Sprintf("invalid MaxCore: %d", kcore.MaxCore))
		valid = false
	}

	// For structural tier with EntityID sibling edges, we expect max_core > 0
	// because sensors of the same type form cliques via sibling edges
	if kcore.MaxCore == 0 {
		result.Warnings = append(result.Warnings,
			"K-core MaxCore is 0 - EntityID sibling edges may not be generating structure. "+
				"Check that entityid_edges.enabled=true and test data has entities with shared type prefixes.")
	}

	// Calculate percentage of entities in core >= 2 (non-leaf nodes)
	entitiesInCore2Plus := 0
	for core, count := range kcore.CoreBuckets {
		if core >= 2 {
			entitiesInCore2Plus += count
		}
	}
	core2PlusPercent := 0.0
	if kcore.EntityCount > 0 {
		core2PlusPercent = 100.0 * float64(entitiesInCore2Plus) / float64(kcore.EntityCount)
	}

	result.Metrics["kcore_entity_count"] = kcore.EntityCount
	result.Metrics["kcore_max_core"] = kcore.MaxCore
	result.Metrics["kcore_core2_plus_percent"] = core2PlusPercent
	result.Metrics["kcore_valid"] = valid

	result.Details["kcore_validation_structural"] = map[string]any{
		"valid":              valid,
		"entity_count":       kcore.EntityCount,
		"max_core":           kcore.MaxCore,
		"core_buckets":       kcore.CoreBuckets,
		"core2_plus_count":   entitiesInCore2Plus,
		"core2_plus_percent": core2PlusPercent,
		"computed_at":        kcore.ComputedAt,
		"issues":             issues,
		"source":             "EntityID sibling edges (no ML)",
		"message": fmt.Sprintf("K-core (structural): %d entities, MaxCore=%d, %.1f%% in core>=2",
			kcore.EntityCount, kcore.MaxCore, core2PlusPercent),
	}

	if !valid {
		return fmt.Errorf("k-core validation failed: %v", issues)
	}

	return nil
}

// executeValidatePivotIndexStructural validates pivot distance index for the structural tier.
func (s *TieredScenario) executeValidatePivotIndexStructural(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping pivot validation")
		return nil
	}

	info, err := s.natsClient.GetStructuralIndexInfo(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get structural index info: %v", err))
		return nil
	}

	if !info.BucketExists {
		result.Warnings = append(result.Warnings, "STRUCTURAL_INDEX bucket does not exist")
		return nil
	}

	if info.Pivot == nil {
		result.Warnings = append(result.Warnings, "Pivot metadata not found in STRUCTURAL_INDEX")
		result.Details["pivot_validation_structural"] = map[string]any{
			"valid":   false,
			"error":   "metadata not found",
			"message": "Pivot index not computed - check graph_analysis.structural_index.pivot config",
		}
		return nil
	}

	pivot := info.Pivot
	valid := true
	issues := []string{}

	// Validate entity count
	if pivot.EntityCount == 0 {
		issues = append(issues, "entity count is 0")
		valid = false
	}

	// Validate pivot count (should have selected pivots)
	pivotCount := len(pivot.Pivots)
	if pivotCount == 0 {
		issues = append(issues, "no pivots selected")
		valid = false
	}

	result.Metrics["pivot_entity_count"] = pivot.EntityCount
	result.Metrics["pivot_count"] = pivotCount
	result.Metrics["pivot_valid"] = valid

	result.Details["pivot_validation_structural"] = map[string]any{
		"valid":        valid,
		"entity_count": pivot.EntityCount,
		"pivot_count":  pivotCount,
		"pivots":       pivot.Pivots,
		"computed_at":  pivot.ComputedAt,
		"issues":       issues,
		"source":       "EntityID sibling edges (no ML)",
		"message":      fmt.Sprintf("Pivot (structural): %d entities, %d pivots selected", pivot.EntityCount, pivotCount),
	}

	if !valid {
		return fmt.Errorf("pivot validation failed: %v", issues)
	}

	return nil
}
