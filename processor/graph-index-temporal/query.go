// Package graphindextemporal query handlers
package graphindextemporal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to temporal range query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.temporal.query.range", c.handleQueryRangeNATS); err != nil {
		return fmt.Errorf("subscribe range query: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.temporal.query.range"})

	return nil
}

// TemporalResult represents an entity found in temporal search
type TemporalResult struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// handleQueryRangeNATS handles temporal range queries via NATS request/reply
func (c *Component) handleQueryRangeNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		StartTime string `json:"startTime"`
		EndTime   string `json:"endTime"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Parse time strings (RFC3339 format)
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid startTime format: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("invalid endTime format: %w", err)
	}

	// Apply default limit
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	// Collect matching entities
	results := make([]TemporalResult, 0)
	seen := make(map[string]bool)

	// Get all keys from temporal bucket
	keys, err := c.temporalBucket.Keys(ctx)
	if err != nil {
		// If bucket is empty or not initialized, return empty results
		return json.Marshal(results)
	}

	// Iterate through time buckets
	// Key format: YYYY.MM.DD.HH
	for _, key := range keys {
		if len(results) >= limit {
			break
		}

		// Parse the time bucket key
		bucketTime, err := parseTimeBucketKey(key)
		if err != nil {
			continue // Skip malformed keys
		}

		// Skip if bucket is outside the query range
		// Add 1 hour to bucket time to get the end of that hour
		bucketEnd := bucketTime.Add(time.Hour)
		if bucketEnd.Before(startTime) || bucketTime.After(endTime) {
			continue
		}

		entry, err := c.temporalBucket.Get(ctx, key)
		if err != nil {
			continue
		}

		// Parse temporal data: {"events": [{"entity": "...", "type": "...", "timestamp": "..."}], "entity_count": N}
		var temporalData struct {
			Events []struct {
				Entity    string `json:"entity"`
				Type      string `json:"type"`
				Timestamp string `json:"timestamp"`
			} `json:"events"`
		}
		if err := json.Unmarshal(entry.Value(), &temporalData); err != nil {
			continue
		}

		// Check each event's timestamp against the query range
		for _, event := range temporalData.Events {
			if seen[event.Entity] {
				continue
			}

			// Parse event timestamp
			eventTime, err := time.Parse(time.RFC3339, event.Timestamp)
			if err != nil {
				continue
			}

			// Check if within query range
			if eventTime.Before(startTime) || eventTime.After(endTime) {
				continue
			}

			seen[event.Entity] = true
			results = append(results, TemporalResult{
				ID:   event.Entity,
				Type: "entity",
			})

			if len(results) >= limit {
				break
			}
		}
	}

	return json.Marshal(results)
}

// parseTimeBucketKey parses a time bucket key in format "YYYY.MM.DD.HH"
func parseTimeBucketKey(key string) (time.Time, error) {
	var year, month, day, hour int
	_, err := fmt.Sscanf(key, "%04d.%02d.%02d.%02d", &year, &month, &day, &hour)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC), nil
}
