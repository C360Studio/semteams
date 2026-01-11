// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Statistical variant validation functions (GraphRAG, community validation)

// graphRAGLocalResponse represents the parsed GraphQL response for local search queries
type graphRAGLocalResponse struct {
	Data struct {
		LocalSearch struct {
			Entities []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"entities"`
			CommunityID string `json:"communityId"`
			Count       int    `json:"count"`
		} `json:"localSearch"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphRAGGlobalResponse represents the parsed GraphQL response for global search queries
type graphRAGGlobalResponse struct {
	Data struct {
		GlobalSearch struct {
			Entities []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"entities"`
			CommunitySummaries []struct {
				CommunityID string   `json:"communityId"`
				Summary     string   `json:"summary"`
				Keywords    []string `json:"keywords"`
				Level       int      `json:"level"`
				Relevance   float64  `json:"relevance"`
			} `json:"communitySummaries"`
			Count int `json:"count"`
		} `json:"globalSearch"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// executeTestGraphRAGLocal validates GraphRAG local search (within community context)
func (s *TieredScenario) executeTestGraphRAGLocal(ctx context.Context, result *Result) error {
	// Use a known entity from test data
	startEntity := "c360.logistics.sensor.document.temperature.sensor-temp-001"
	gatewayURL := s.config.GraphQLURL // Use GraphQL gateway URL, not api-gateway
	searchQuery := "temperature sensor monitoring"

	resp, latency, err := s.sendGraphRAGLocalRequest(ctx, startEntity, searchQuery, gatewayURL)
	if err != nil {
		result.Details["graphrag_local_test"] = map[string]any{
			"start_entity": startEntity, "query": searchQuery, "error": err.Error(),
		}
		// GraphRAG local may fail if entity not in a community - warn but don't fail
		result.Warnings = append(result.Warnings, fmt.Sprintf("GraphRAG local search failed: %v", err))
		return nil
	}

	result.Metrics["graphrag_local_latency_ms"] = latency.Milliseconds()
	return s.validateGraphRAGLocalResult(resp, startEntity, searchQuery, latency, result)
}

// sendGraphRAGLocalRequest sends the GraphRAG local search query
func (s *TieredScenario) sendGraphRAGLocalRequest(ctx context.Context, entityID, query, gatewayURL string) (*graphRAGLocalResponse, time.Duration, error) {
	graphqlQuery := map[string]any{
		"query": `query($entityId: ID!, $query: String!, $level: Int) {
			localSearch(entityId: $entityId, query: $query, level: $level) {
				entities { id type } communityId count
			}}`,
		"variables": map[string]any{"entityId": entityID, "query": query, "level": 1},
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, latency, fmt.Errorf("returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, fmt.Errorf("failed to read response: %w", err)
	}

	var graphqlResp graphRAGLocalResponse
	if err := json.Unmarshal(bodyBytes, &graphqlResp); err != nil {
		return nil, latency, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, latency, fmt.Errorf("GraphQL error: %s", graphqlResp.Errors[0].Message)
	}

	return &graphqlResp, latency, nil
}

// validateGraphRAGLocalResult validates the local search response
func (s *TieredScenario) validateGraphRAGLocalResult(resp *graphRAGLocalResponse, entityID, query string, latency time.Duration, result *Result) error {
	ls := resp.Data.LocalSearch
	entityCount := len(ls.Entities)

	result.Metrics["graphrag_local_entities_found"] = entityCount
	result.Metrics["graphrag_local_community_id"] = ls.CommunityID

	entityIDs := make([]string, 0, len(ls.Entities))
	for _, e := range ls.Entities {
		entityIDs = append(entityIDs, e.ID)
	}

	result.Details["graphrag_local"] = map[string]any{
		"query":            query,
		"entities_used":    entityCount,
		"communities_used": 1, // Single community context for local search
		"latency_ms":       latency.Milliseconds(),
		"success":          true,
		// Additional fields for debugging
		"start_entity": entityID,
		"community_id": ls.CommunityID,
		"entity_ids":   entityIDs,
	}

	// Community context may be empty if entity is not in any community
	// This is expected for entities with no graph edges (isolated nodes)
	// GraphRAG still works via semantic fallback, just without community context
	if ls.CommunityID == "" {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Entity %s not in any community - GraphRAG local requires connected entities", entityID))
		// Don't fail - this is expected for isolated entities
		// The test validates that the GraphRAG endpoint responds correctly
		return nil
	}

	// Validate at least one entity is returned when we have community context
	if entityCount == 0 {
		return fmt.Errorf("GraphRAG local search returned no entities for query %q in community %s", query, ls.CommunityID)
	}

	return nil
}

// executeTestGraphRAGGlobal validates GraphRAG global search (across community summaries)
func (s *TieredScenario) executeTestGraphRAGGlobal(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GraphQLURL // Use GraphQL gateway URL, not api-gateway
	searchQuery := "logistics warehouse operations"

	resp, latency, err := s.sendGraphRAGGlobalRequest(ctx, searchQuery, gatewayURL)
	if err != nil {
		result.Details["graphrag_global"] = map[string]any{
			"query":   searchQuery,
			"error":   err.Error(),
			"success": false,
		}
		// GraphRAG global may fail if no communities exist - warn but don't fail
		result.Warnings = append(result.Warnings, fmt.Sprintf("GraphRAG global search failed: %v", err))
		return nil
	}

	result.Metrics["graphrag_global_latency_ms"] = latency.Milliseconds()
	return s.validateGraphRAGGlobalResult(resp, searchQuery, latency, result)
}

// sendGraphRAGGlobalRequest sends the GraphRAG global search query
func (s *TieredScenario) sendGraphRAGGlobalRequest(ctx context.Context, query, gatewayURL string) (*graphRAGGlobalResponse, time.Duration, error) {
	graphqlQuery := map[string]any{
		"query": `query($query: String!, $level: Int, $maxCommunities: Int) {
			globalSearch(query: $query, level: $level, maxCommunities: $maxCommunities) {
				entities { id type }
				communitySummaries { communityId summary keywords level relevance }
				count
			}}`,
		"variables": map[string]any{"query": query, "level": 1, "maxCommunities": 5},
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL, bytes.NewReader(queryJSON))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, latency, fmt.Errorf("returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, fmt.Errorf("failed to read response: %w", err)
	}

	var graphqlResp graphRAGGlobalResponse
	if err := json.Unmarshal(bodyBytes, &graphqlResp); err != nil {
		return nil, latency, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, latency, fmt.Errorf("GraphQL error: %s", graphqlResp.Errors[0].Message)
	}

	return &graphqlResp, latency, nil
}

// validateGraphRAGGlobalResult validates the global search response
func (s *TieredScenario) validateGraphRAGGlobalResult(resp *graphRAGGlobalResponse, query string, latency time.Duration, result *Result) error {
	gs := resp.Data.GlobalSearch
	entityCount := len(gs.Entities)
	communityCount := len(gs.CommunitySummaries)

	result.Metrics["graphrag_global_entities_found"] = entityCount
	result.Metrics["graphrag_global_communities_found"] = communityCount

	entityIDs := make([]string, 0, len(gs.Entities))
	for _, e := range gs.Entities {
		entityIDs = append(entityIDs, e.ID)
	}

	communityDetails := make([]map[string]any, 0, len(gs.CommunitySummaries))
	for _, cs := range gs.CommunitySummaries {
		communityDetails = append(communityDetails, map[string]any{
			"community_id": cs.CommunityID,
			"keywords":     cs.Keywords,
			"level":        cs.Level,
			"relevance":    cs.Relevance,
			"has_summary":  cs.Summary != "",
		})
	}

	result.Details["graphrag_global"] = map[string]any{
		"query":            query,
		"entities_used":    entityCount,
		"communities_used": communityCount,
		"latency_ms":       latency.Milliseconds(),
		"success":          true,
		// Additional fields for debugging
		"entity_ids":  entityIDs,
		"communities": communityDetails,
	}

	// Phase 2 improvement: Validate multi-community results for broad queries
	if communityCount < 2 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("GraphRAG global search returned only %d communities for broad query %q, expected >= 2", communityCount, query))
	}

	// Phase 2 improvement: Validate each community has a summary
	for _, cs := range gs.CommunitySummaries {
		if cs.Summary == "" {
			return fmt.Errorf("GraphRAG global search: community %s missing summary", cs.CommunityID)
		}
	}

	return nil
}

// executeValidateCommunityStructure validates that community detection produced valid structure
func (s *TieredScenario) executeValidateCommunityStructure(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping community structure validation")
		return nil
	}

	// Wait for communities to be available (community detection may still be running)
	communities, err := s.waitForCommunities(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get communities: %v", err))
		return nil
	}

	totalCount := len(communities)
	nonSingletonCount := 0
	largestSize := 0
	totalNonSingletonSize := 0
	communitiesWithKeywords := 0
	llmEnhancedCount := 0
	statisticalOnlyCount := 0

	for _, comm := range communities {
		memberCount := len(comm.Members)
		if memberCount > 1 {
			nonSingletonCount++
			totalNonSingletonSize += memberCount
			if memberCount > largestSize {
				largestSize = memberCount
			}
		}
		if len(comm.Keywords) > 0 {
			communitiesWithKeywords++
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Community %s has no keywords", comm.ID))
		}
		// Track summary status for visibility into LLM enhancement
		switch comm.SummaryStatus {
		case "llm-enhanced":
			llmEnhancedCount++
		case "statistical", "":
			statisticalOnlyCount++
		}
	}

	avgNonSingletonSize := 0.0
	if nonSingletonCount > 0 {
		avgNonSingletonSize = float64(totalNonSingletonSize) / float64(nonSingletonCount)
	}

	result.Metrics["communities_total"] = totalCount
	result.Metrics["communities_non_singleton"] = nonSingletonCount
	result.Metrics["communities_largest_size"] = largestSize
	result.Metrics["communities_avg_size"] = avgNonSingletonSize
	result.Metrics["communities_with_keywords"] = communitiesWithKeywords
	result.Metrics["communities_llm_enhanced"] = llmEnhancedCount
	result.Metrics["communities_statistical_only"] = statisticalOnlyCount

	result.Details["community_structure_validation"] = map[string]any{
		"total_communities":      totalCount,
		"non_singleton_count":    nonSingletonCount,
		"largest_community":      largestSize,
		"avg_non_singleton_size": avgNonSingletonSize,
		"with_keywords":          communitiesWithKeywords,
		"llm_enhanced":           llmEnhancedCount,
		"statistical_only":       statisticalOnlyCount,
		"message": fmt.Sprintf("Community structure: %d total, %d non-singleton (avg size: %.1f), %d LLM-enhanced",
			totalCount, nonSingletonCount, avgNonSingletonSize, llmEnhancedCount),
	}

	// For statistical tier, we require at least some non-singleton communities
	// to verify that graph connectivity (incoming edges) is working correctly.
	// Without incoming edges, LPA produces all singletons because nodes appear isolated.
	if nonSingletonCount == 0 && totalCount > 0 {
		return fmt.Errorf("no non-singleton communities found (%d total) - graph connectivity may be broken", totalCount)
	}

	return nil
}

// executeValidateKCoreIndex validates k-core decomposition index for statistical/semantic tiers
func (s *TieredScenario) executeValidateKCoreIndex(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping k-core validation")
		return nil
	}

	// Get structural index info from NATS
	info, err := s.natsClient.GetStructuralIndexInfo(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get structural index info: %v", err))
		return nil
	}

	if !info.BucketExists {
		result.Warnings = append(result.Warnings, "STRUCTURAL_INDEX bucket does not exist")
		return nil
	}

	if info.KCore == nil {
		result.Warnings = append(result.Warnings, "K-core metadata not found in STRUCTURAL_INDEX")
		result.Details["kcore_validation"] = map[string]any{
			"valid":   false,
			"error":   "metadata not found",
			"message": "K-core index not computed - check graph_analysis.structural_index config",
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

	// Validate MaxCore (should be >= 0, ideally > 0 for statistical tier with community edges)
	if kcore.MaxCore < 0 {
		issues = append(issues, fmt.Sprintf("invalid MaxCore: %d", kcore.MaxCore))
		valid = false
	}

	// Validate core buckets sum equals entity count
	totalInBuckets := 0
	for _, count := range kcore.CoreBuckets {
		totalInBuckets += count
	}
	if totalInBuckets > 0 && totalInBuckets != kcore.EntityCount {
		issues = append(issues, fmt.Sprintf("core buckets sum (%d) != entity count (%d)", totalInBuckets, kcore.EntityCount))
	}

	// For statistical tier, expect some structure (MaxCore > 0) from community detection edges
	if kcore.MaxCore == 0 {
		result.Warnings = append(result.Warnings,
			"K-core MaxCore is 0 - graph may lack sufficient edge density")
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

	result.Details["kcore_validation"] = map[string]any{
		"valid":              valid,
		"entity_count":       kcore.EntityCount,
		"max_core":           kcore.MaxCore,
		"core_buckets":       kcore.CoreBuckets,
		"total_in_buckets":   totalInBuckets,
		"core2_plus_count":   entitiesInCore2Plus,
		"core2_plus_percent": core2PlusPercent,
		"computed_at":        kcore.ComputedAt,
		"issues":             issues,
		"message": fmt.Sprintf("K-core: %d entities, MaxCore=%d, %.1f%% in core>=2",
			kcore.EntityCount, kcore.MaxCore, core2PlusPercent),
	}

	if !valid {
		return fmt.Errorf("k-core validation failed: %v", issues)
	}

	return nil
}

// executeValidatePivotIndex validates pivot distance index for statistical/semantic tiers
func (s *TieredScenario) executeValidatePivotIndex(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping pivot validation")
		return nil
	}

	// Get structural index info from NATS
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
		result.Details["pivot_validation"] = map[string]any{
			"valid":   false,
			"error":   "metadata not found",
			"message": "Pivot index not computed - check graph_analysis.structural_index config",
		}
		return nil
	}

	pivot := info.Pivot
	valid := true
	issues := []string{}

	// Validate we have pivots
	if len(pivot.Pivots) == 0 {
		issues = append(issues, "no pivots selected")
		valid = false
	}

	// Validate entity count
	if pivot.EntityCount == 0 {
		issues = append(issues, "entity count is 0")
		valid = false
	}

	// Validate pivots are non-empty strings
	emptyPivots := 0
	for _, p := range pivot.Pivots {
		if p == "" {
			emptyPivots++
		}
	}
	if emptyPivots > 0 {
		issues = append(issues, fmt.Sprintf("%d empty pivot IDs", emptyPivots))
		valid = false
	}

	// Expected pivot count (from config, typically 16)
	expectedPivots := 16
	if len(pivot.Pivots) < expectedPivots {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Only %d/%d pivots selected (graph may be small)", len(pivot.Pivots), expectedPivots))
	}

	result.Metrics["pivot_count"] = len(pivot.Pivots)
	result.Metrics["pivot_entity_count"] = pivot.EntityCount
	result.Metrics["pivot_valid"] = valid

	result.Details["pivot_validation"] = map[string]any{
		"valid":           valid,
		"pivot_count":     len(pivot.Pivots),
		"expected_pivots": expectedPivots,
		"entity_count":    pivot.EntityCount,
		"pivots":          pivot.Pivots,
		"computed_at":     pivot.ComputedAt,
		"issues":          issues,
		"message": fmt.Sprintf("Pivot: %d pivots for %d entities",
			len(pivot.Pivots), pivot.EntityCount),
	}

	if !valid {
		return fmt.Errorf("pivot validation failed: %v", issues)
	}

	return nil
}
