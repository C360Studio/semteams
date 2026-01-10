// Package graphindexspatial query handlers
package graphindexspatial

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to spatial bounds query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.spatial.query.bounds", c.handleQueryBoundsNATS); err != nil {
		return fmt.Errorf("subscribe bounds query: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.spatial.query.bounds"})

	return nil
}

// SpatialResult represents an entity found in spatial search
type SpatialResult struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// handleQueryBoundsNATS handles spatial bounds queries via NATS request/reply
func (c *Component) handleQueryBoundsNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		North float64 `json:"north"`
		South float64 `json:"south"`
		East  float64 `json:"east"`
		West  float64 `json:"west"`
		Limit int     `json:"limit"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Apply default limit
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	// Collect matching entities
	results := make([]SpatialResult, 0)
	seen := make(map[string]bool)

	// Get all keys from spatial bucket
	keys, err := c.spatialBucket.Keys(ctx)
	if err != nil {
		// If bucket is empty or not initialized, return empty results
		return json.Marshal(results)
	}

	// Iterate through geohash cells
	for _, key := range keys {
		if len(results) >= limit {
			break
		}

		entry, err := c.spatialBucket.Get(ctx, key)
		if err != nil {
			continue
		}

		// Parse spatial data: {"entities": {"entityId": {"lat": ..., "lon": ..., "alt": ...}}}
		var spatialData struct {
			Entities map[string]struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
				Alt float64 `json:"alt"`
			} `json:"entities"`
		}
		if err := json.Unmarshal(entry.Value(), &spatialData); err != nil {
			continue
		}

		// Check each entity's coordinates against bounds
		for entityID, coords := range spatialData.Entities {
			if seen[entityID] {
				continue
			}

			// Check if within bounding box
			if coords.Lat >= req.South && coords.Lat <= req.North &&
				coords.Lon >= req.West && coords.Lon <= req.East {
				seen[entityID] = true
				results = append(results, SpatialResult{
					ID:   entityID,
					Type: "entity",
				})

				if len(results) >= limit {
					break
				}
			}
		}
	}

	return json.Marshal(results)
}
