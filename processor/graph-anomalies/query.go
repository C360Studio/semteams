// Package graphanomalies query handlers
package graphanomalies

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/c360/semstreams/component"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to k-core query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.anomalies.query.kcore", c.handleQueryKCoreNATS); err != nil {
		return fmt.Errorf("subscribe kcore query: %w", err)
	}

	// Subscribe to pivot query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.anomalies.query.pivot", c.handleQueryPivotNATS); err != nil {
		return fmt.Errorf("subscribe pivot query: %w", err)
	}

	// Subscribe to outliers query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.anomalies.query.outliers", c.handleQueryOutliersNATS); err != nil {
		return fmt.Errorf("subscribe outliers query: %w", err)
	}

	// Subscribe to capabilities discovery
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.anomalies.capabilities", c.handleCapabilitiesNATS); err != nil {
		return fmt.Errorf("subscribe capabilities: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{
			"graph.anomalies.query.kcore",
			"graph.anomalies.query.pivot",
			"graph.anomalies.query.outliers",
			"graph.anomalies.capabilities",
		})

	return nil
}

// KCoreRequest is the request format for k-core query
type KCoreRequest struct {
	EntityID string `json:"entity_id,omitempty"`
	MinCore  int    `json:"min_core,omitempty"`
}

// KCoreResponse is the response format for k-core query
type KCoreResponse struct {
	EntityID    string       `json:"entity_id,omitempty"`
	CoreNumber  int          `json:"core_number,omitempty"`
	MaxCore     int          `json:"max_core"`
	CoreBuckets map[int]int  `json:"core_buckets,omitempty"` // core number -> count
	Entities    []EntityCore `json:"entities,omitempty"`     // if min_core specified
}

// EntityCore represents an entity with its core number
type EntityCore struct {
	EntityID   string `json:"entity_id"`
	CoreNumber int    `json:"core_number"`
}

// PivotRequest is the request format for pivot query
type PivotRequest struct {
	EntityID string `json:"entity_id,omitempty"`
}

// PivotResponse is the response format for pivot query
type PivotResponse struct {
	EntityID        string   `json:"entity_id,omitempty"`
	Pivots          []string `json:"pivots"`
	DistanceVector  []int    `json:"distance_vector,omitempty"`
	AverageDistance float64  `json:"average_distance,omitempty"`
}

// OutliersRequest is the request format for outliers query
type OutliersRequest struct {
	MaxKCore       int `json:"max_kcore,omitempty"`        // Filter entities with k-core <= this
	MinAvgDistance int `json:"min_avg_distance,omitempty"` // Filter entities with avg pivot distance >= this
	Limit          int `json:"limit,omitempty"`
}

// OutliersResponse is the response format for outliers query
type OutliersResponse struct {
	Outliers []Outlier `json:"outliers"`
	Count    int       `json:"count"`
}

// Outlier represents a structural outlier
type Outlier struct {
	EntityID        string  `json:"entity_id"`
	CoreNumber      int     `json:"core_number"`
	AverageDistance float64 `json:"average_distance"`
	Score           float64 `json:"score"` // Anomaly score (higher = more anomalous)
}

// handleQueryKCoreNATS handles k-core query requests via NATS request/reply
func (c *Component) handleQueryKCoreNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for storage operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req KCoreRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Check storage is initialized
	if c.storage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	// Get k-core index
	kcoreIndex, err := c.storage.GetKCoreIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("get kcore index: %w", err)
	}
	if kcoreIndex == nil {
		// Return empty response if index not computed yet
		return json.Marshal(KCoreResponse{MaxCore: 0})
	}

	response := KCoreResponse{MaxCore: kcoreIndex.MaxCore}

	// If entity ID specified, return that entity's core number
	if req.EntityID != "" {
		if coreNum, ok := kcoreIndex.CoreNumbers[req.EntityID]; ok {
			response.EntityID = req.EntityID
			response.CoreNumber = coreNum
		} else {
			return nil, fmt.Errorf("not found: %s", req.EntityID)
		}
	}

	// If min_core specified, filter entities
	if req.MinCore > 0 {
		var entities []EntityCore
		for entityID, coreNum := range kcoreIndex.CoreNumbers {
			if coreNum >= req.MinCore {
				entities = append(entities, EntityCore{
					EntityID:   entityID,
					CoreNumber: coreNum,
				})
			}
		}
		// Sort by core number descending
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].CoreNumber > entities[j].CoreNumber
		})
		response.Entities = entities
	}

	// Include core bucket summary
	if kcoreIndex.CoreBuckets != nil {
		response.CoreBuckets = make(map[int]int)
		for k, entityIDs := range kcoreIndex.CoreBuckets {
			response.CoreBuckets[k] = len(entityIDs)
		}
	}

	return json.Marshal(response)
}

// handleQueryPivotNATS handles pivot query requests via NATS request/reply
func (c *Component) handleQueryPivotNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for storage operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req PivotRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Check storage is initialized
	if c.storage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	// Get pivot index
	pivotIndex, err := c.storage.GetPivotIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pivot index: %w", err)
	}
	if pivotIndex == nil {
		// Return empty response if index not computed yet
		return json.Marshal(PivotResponse{Pivots: []string{}})
	}

	response := PivotResponse{Pivots: pivotIndex.Pivots}

	// If entity ID specified, return that entity's distance vector
	if req.EntityID != "" {
		if distVec, ok := pivotIndex.DistanceVectors[req.EntityID]; ok {
			response.EntityID = req.EntityID
			response.DistanceVector = distVec
			response.AverageDistance = calculateAverageDistance(distVec, c.config.MaxHopDistance)
		} else {
			return nil, fmt.Errorf("not found: %s", req.EntityID)
		}
	}

	return json.Marshal(response)
}

// handleQueryOutliersNATS handles outliers query requests via NATS request/reply
func (c *Component) handleQueryOutliersNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for storage operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse request
	var req OutliersRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Apply defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	maxKCore := req.MaxKCore
	if maxKCore <= 0 {
		maxKCore = 2 // Default: entities with core number <= 2 are potential outliers
	}

	// Check storage is initialized
	if c.storage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	// Get indices
	kcoreIndex, err := c.storage.GetKCoreIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("get kcore index: %w", err)
	}

	pivotIndex, err := c.storage.GetPivotIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pivot index: %w", err)
	}

	// Find outliers
	var outliers []Outlier

	// Score entities based on k-core and pivot distance
	for entityID, coreNum := range kcoreIndex.CoreNumbers {
		// Filter by max k-core
		if coreNum > maxKCore {
			continue
		}

		avgDist := 0.0
		if pivotIndex != nil {
			if distVec, ok := pivotIndex.DistanceVectors[entityID]; ok {
				avgDist = calculateAverageDistance(distVec, c.config.MaxHopDistance)
			}
		}

		// Filter by min avg distance if specified
		if req.MinAvgDistance > 0 && avgDist < float64(req.MinAvgDistance) {
			continue
		}

		// Calculate anomaly score: lower k-core + higher distance = more anomalous
		// Normalize: k-core inversely contributes, distance directly contributes
		maxCore := kcoreIndex.MaxCore
		if maxCore == 0 {
			maxCore = 1
		}
		kcoreScore := 1.0 - (float64(coreNum) / float64(maxCore))
		distScore := avgDist / float64(c.config.MaxHopDistance)
		anomalyScore := (kcoreScore + distScore) / 2.0

		outliers = append(outliers, Outlier{
			EntityID:        entityID,
			CoreNumber:      coreNum,
			AverageDistance: avgDist,
			Score:           anomalyScore,
		})
	}

	// Sort by anomaly score descending
	sort.Slice(outliers, func(i, j int) bool {
		return outliers[i].Score > outliers[j].Score
	})

	// Apply limit
	if len(outliers) > limit {
		outliers = outliers[:limit]
	}

	return json.Marshal(OutliersResponse{
		Outliers: outliers,
		Count:    len(outliers),
	})
}

// calculateAverageDistance calculates average distance from distance vector, excluding unreachable
func calculateAverageDistance(distVec []int, maxHop int) float64 {
	if len(distVec) == 0 {
		return 0
	}

	sum := 0
	count := 0
	for _, d := range distVec {
		if d < maxHop { // Exclude unreachable (maxHop indicates unreachable)
			sum += d
			count++
		}
	}

	if count == 0 {
		return float64(maxHop) // All unreachable = max distance
	}

	return float64(sum) / float64(count)
}

// handleCapabilitiesNATS handles capability discovery requests via NATS request/reply
func (c *Component) handleCapabilitiesNATS(_ context.Context, _ []byte) ([]byte, error) {
	caps := c.QueryCapabilities()
	return json.Marshal(caps)
}

// Ensure Component implements QueryCapabilityProvider
var _ component.QueryCapabilityProvider = (*Component)(nil)

// QueryCapabilities implements QueryCapabilityProvider interface
func (c *Component) QueryCapabilities() component.QueryCapabilities {
	return component.QueryCapabilities{
		Component: "graph-anomalies",
		Version:   "1.0.0",
		Queries: []component.QueryCapability{
			{
				Subject:     "graph.anomalies.query.kcore",
				Operation:   "getKCore",
				Description: "Get k-core decomposition information for an entity or filtered by core number",
				IntentTags:  []string{component.IntentTagAnomaly, component.IntentTagAggregate},
				EntityTypes: []string{"*"},
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_id": map[string]any{
							"type":        "string",
							"description": "Entity ID to get k-core for (optional)",
						},
						"min_core": map[string]any{
							"type":        "integer",
							"description": "Filter entities with core number >= this value",
						},
					},
				},
				ResponseSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_id":    map[string]any{"type": "string"},
						"core_number":  map[string]any{"type": "integer"},
						"max_core":     map[string]any{"type": "integer"},
						"core_buckets": map[string]any{"type": "object"},
						"entities":     map[string]any{"type": "array"},
					},
				},
			},
			{
				Subject:     "graph.anomalies.query.pivot",
				Operation:   "getPivotDistances",
				Description: "Get pivot distances for an entity or list all pivots",
				IntentTags:  []string{component.IntentTagAnomaly},
				EntityTypes: []string{"*"},
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_id": map[string]any{
							"type":        "string",
							"description": "Entity ID to get pivot distances for (optional)",
						},
					},
				},
				ResponseSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_id":        map[string]any{"type": "string"},
						"pivots":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"distance_vector":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
						"average_distance": map[string]any{"type": "number"},
					},
				},
			},
			{
				Subject:     "graph.anomalies.query.outliers",
				Operation:   "findOutliers",
				Description: "Find structural outliers based on k-core and pivot distances",
				IntentTags:  []string{component.IntentTagAnomaly},
				EntityTypes: []string{"*"},
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"max_kcore": map[string]any{
							"type":        "integer",
							"description": "Include entities with core number <= this (default 2)",
						},
						"min_avg_distance": map[string]any{
							"type":        "integer",
							"description": "Include entities with average pivot distance >= this",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum outliers to return (default 20, max 100)",
						},
					},
				},
				ResponseSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"outliers": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"entity_id":        map[string]any{"type": "string"},
									"core_number":      map[string]any{"type": "integer"},
									"average_distance": map[string]any{"type": "number"},
									"score":            map[string]any{"type": "number"},
								},
							},
						},
						"count": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
}
