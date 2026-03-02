// Package graphindexspatial query handlers
package graphindexspatial

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to spatial bounds query
	sub, err := c.natsClient.SubscribeForRequests(ctx, "graph.spatial.query.bounds", c.handleQueryBoundsNATS)
	if err != nil {
		return errs.WrapTransient(err, "Component", "setupQueryHandlers", "subscribe bounds query")
	}
	c.querySubscriptions = append(c.querySubscriptions, sub)

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.spatial.query.bounds"})

	return nil
}

// SpatialResult represents an entity found in spatial search
type SpatialResult struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// boundsRequest holds parsed spatial bounds query parameters
type boundsRequest struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
	Limit int     `json:"limit"`
}

// handleQueryBoundsNATS handles spatial bounds queries via NATS request/reply
func (c *Component) handleQueryBoundsNATS(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var req boundsRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "handleQueryBoundsNATS", "invalid request")
	}

	if req.Limit <= 0 {
		req.Limit = 100
	}

	// Compute candidate cell keys from the bounding box.
	// Returns nil when the box is too large (> 10,000 cells) → fall back to full scan.
	candidateCells := geohashCellsInBounds(req.North, req.South, req.East, req.West, c.config.GeohashPrecision)

	var keys []string
	if candidateCells != nil {
		keys = candidateCells
	} else {
		var err error
		keys, err = c.spatialBucket.Keys(ctx)
		if err != nil {
			return json.Marshal([]SpatialResult{})
		}
	}

	results := c.collectSpatialResults(ctx, keys, req)
	return json.Marshal(results)
}

// collectSpatialResults fetches spatial cells by key and filters entities within bounds.
func (c *Component) collectSpatialResults(ctx context.Context, keys []string, req boundsRequest) []SpatialResult {
	results := make([]SpatialResult, 0)
	seen := make(map[string]bool)

	for _, key := range keys {
		if len(results) >= req.Limit {
			break
		}
		entry, err := c.spatialBucket.Get(ctx, key)
		if err != nil {
			continue
		}

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

		for entityID, coords := range spatialData.Entities {
			if seen[entityID] {
				continue
			}
			if coords.Lat >= req.South && coords.Lat <= req.North &&
				coords.Lon >= req.West && coords.Lon <= req.East {
				seen[entityID] = true
				results = append(results, SpatialResult{ID: entityID, Type: "entity"})
				if len(results) >= req.Limit {
					break
				}
			}
		}
	}
	return results
}

// geohashMultiplier returns the bin-size multiplier for the given precision level.
// This mirrors the algorithm in component.go calculateGeohash so the two are
// always in sync.
func geohashMultiplier(precision int) float64 {
	switch precision {
	case 4:
		return 10.0
	case 5:
		return 50.0
	case 6:
		return 100.0
	case 7:
		return 300.0
	case 8:
		return 1000.0
	default:
		return 300.0
	}
}

// geohashCellsInBounds enumerates all geohash cell keys that fall within the
// given bounding box for the given precision level.
//
// The cell key format is "geo_<precision>_<latBin>_<lonBin>", matching the
// algorithm used by calculateGeohash in component.go.
//
// To prevent runaway allocations on very large bounding boxes (e.g. a global
// query) the function returns nil when the number of candidate cells would
// exceed 10,000. The caller must then fall back to a full key scan.
func geohashCellsInBounds(north, south, east, west float64, precision int) []string {
	const maxCells = 10_000

	multiplier := geohashMultiplier(precision)

	minLatBin := int(math.Floor((south + 90.0) * multiplier))
	maxLatBin := int(math.Floor((north + 90.0) * multiplier))
	minLonBin := int(math.Floor((west + 180.0) * multiplier))
	maxLonBin := int(math.Floor((east + 180.0) * multiplier))

	// Guard against degenerate or inverted bounds.
	if minLatBin > maxLatBin || minLonBin > maxLonBin {
		return nil
	}

	latCount := maxLatBin - minLatBin + 1
	lonCount := maxLonBin - minLonBin + 1
	if latCount*lonCount > maxCells {
		return nil // bounding box too large; caller falls back to full scan
	}

	cells := make([]string, 0, latCount*lonCount)
	for latBin := minLatBin; latBin <= maxLatBin; latBin++ {
		for lonBin := minLonBin; lonBin <= maxLonBin; lonBin++ {
			cells = append(cells, fmt.Sprintf("geo_%d_%d_%d", precision, latBin, lonBin))
		}
	}
	return cells
}
