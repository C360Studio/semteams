// Package graphingest query handlers
package graphingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to entity query
	sub, err := c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.entity", c.handleQueryEntityNATS)
	if err != nil {
		return fmt.Errorf("subscribe entity query: %w", err)
	}
	c.subscriptions = append(c.subscriptions, sub)

	// Subscribe to batch query
	sub, err = c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.batch", c.handleQueryBatchNATS)
	if err != nil {
		return fmt.Errorf("subscribe batch query: %w", err)
	}
	c.subscriptions = append(c.subscriptions, sub)

	// Subscribe to prefix query (for hierarchy listing)
	sub, err = c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.prefix", c.handleQueryPrefixNATS)
	if err != nil {
		return fmt.Errorf("subscribe prefix query: %w", err)
	}
	c.subscriptions = append(c.subscriptions, sub)

	// Subscribe to suffix query (for partial entity ID resolution)
	sub, err = c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.suffix", c.handleQuerySuffixNATS)
	if err != nil {
		return fmt.Errorf("subscribe suffix query: %w", err)
	}
	c.subscriptions = append(c.subscriptions, sub)

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.ingest.query.entity", "graph.ingest.query.batch", "graph.ingest.query.prefix", "graph.ingest.query.suffix"})

	return nil
}

// handleQueryEntityNATS handles single entity query requests via NATS request/reply
func (c *Component) handleQueryEntityNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.ID == "" {
		return nil, fmt.Errorf("invalid request: empty id")
	}

	// Get entity from KV bucket
	entry, err := c.entityBucket.Get(ctx, req.ID)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("not found: %s", req.ID)
		}
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return entry.Value, nil
}

// handleQueryBatchNATS handles batch entity query requests via NATS request/reply
func (c *Component) handleQueryBatchNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Handle empty IDs (return empty entities)
	if len(req.IDs) == 0 {
		return []byte(`{"entities":[]}`), nil
	}

	// Fetch entities
	entities := make([]graph.EntityState, 0, len(req.IDs))
	for _, id := range req.IDs {
		if id == "" {
			continue // Skip empty IDs
		}

		entry, err := c.entityBucket.Get(ctx, id)
		if err != nil {
			// Skip not found entities (partial success)
			continue
		}

		var entity graph.EntityState
		if err := json.Unmarshal(entry.Value, &entity); err != nil {
			// Skip entities that fail to unmarshal
			continue
		}

		entities = append(entities, entity)
	}

	// Return entities wrapped in a struct for consistency with loadEntities expectations
	return json.Marshal(map[string]any{
		"entities": entities,
	})
}

// handleQueryPrefixNATS handles prefix-based entity listing for hierarchy queries
func (c *Component) handleQueryPrefixNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		Prefix string `json:"prefix"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Get all keys from KV bucket
	keys, err := c.entityBucket.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get keys: %w", err)
	}

	// Filter by prefix
	var matched []string
	prefixDot := req.Prefix
	if req.Prefix != "" {
		prefixDot = req.Prefix + "."
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 1000 // Default limit
	}

	for _, key := range keys {
		// Match if prefix is empty (all entities) or key starts with prefix.
		if req.Prefix == "" || strings.HasPrefix(key, prefixDot) || key == req.Prefix {
			matched = append(matched, key)
			if len(matched) >= limit {
				break
			}
		}
	}

	// Return response in GraphQL-compatible format
	response := map[string]any{
		"entityIds":  matched,
		"totalCount": len(matched),
		"truncated":  len(matched) >= limit,
		"prefix":     req.Prefix,
	}
	return json.Marshal(response)
}

// handleQuerySuffixNATS handles suffix-based entity ID resolution.
// Scans entity keys and matches by suffix to resolve partial entity IDs.
// This enables NL queries to use partial entity IDs like "temp-sensor-001" which
// get resolved to full 6-part IDs like "c360.logistics.environmental.sensor.temperature.temp-sensor-001".
func (c *Component) handleQuerySuffixNATS(_ context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req struct {
		Suffix string `json:"suffix"` // e.g., "temp-sensor-001"
	}
	if err := json.Unmarshal(data, &req); err != nil {
		c.logger.Error("suffix query unmarshal failed", "error", err)
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	c.logger.Debug("suffix query received", "suffix", req.Suffix)

	if req.Suffix == "" {
		return nil, fmt.Errorf("invalid request: empty suffix")
	}

	// Get all entity keys and find one matching the suffix
	// This matches the instance part of a 6-part EntityID: org.platform.domain.system.type.instance
	keys, err := c.entityBucket.Keys(ctx)
	if err != nil {
		c.logger.Error("suffix query keys failed", "error", err)
		return nil, fmt.Errorf("failed to get keys: %w", err)
	}
	// Handle empty bucket
	if keys == nil {
		return json.Marshal(map[string]string{"id": ""})
	}

	c.logger.Debug("suffix query scanning keys", "suffix", req.Suffix, "key_count", len(keys))

	// Match keys ending with ".suffix" (the instance part)
	suffixWithDot := "." + req.Suffix
	var matchedID string
	for _, key := range keys {
		if strings.HasSuffix(key, suffixWithDot) || key == req.Suffix {
			matchedID = key
			c.logger.Debug("suffix query matched", "suffix", req.Suffix, "matched", matchedID)
			break
		}
	}

	if matchedID == "" {
		c.logger.Debug("suffix query no match found", "suffix", req.Suffix)
	}

	return json.Marshal(map[string]string{"id": matchedID})
}

// queryMsg is an interface for query request messages.
// This accommodates both real NATS messages and test mocks.
type queryMsg interface {
	Data() []byte
	Respond(data []byte) error
}

// handleQueryEntity handles single entity query requests
func (c *Component) handleQueryEntity(msg queryMsg) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Validate request
	if req.ID == "" {
		c.respondError(msg, "invalid request")
		return
	}

	// Get entity from KV bucket
	entry, err := c.entityBucket.Get(ctx, req.ID)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			c.respondError(msg, "not found")
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Respond with entity JSON
	if err := msg.Respond(entry.Value); err != nil {
		c.logger.Error("failed to respond to query", "error", err)
	}
}

// handleQueryBatch handles batch entity query requests
func (c *Component) handleQueryBatch(msg queryMsg) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First unmarshal into map to detect explicit null values
	var rawReq map[string]any
	if err := json.Unmarshal(msg.Data(), &rawReq); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Check if ids field exists and is explicitly null
	idsValue, hasIDs := rawReq["ids"]
	if hasIDs && idsValue == nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Now parse into typed struct
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Handle empty or missing IDs (return empty array)
	if len(req.IDs) == 0 {
		c.respondJSON(msg, []graph.EntityState{})
		return
	}

	// Fetch entities
	entities := make([]graph.EntityState, 0, len(req.IDs))
	for _, id := range req.IDs {
		if id == "" {
			continue // Skip empty IDs
		}

		entry, err := c.entityBucket.Get(ctx, id)
		if err != nil {
			// Skip not found entities (partial success)
			continue
		}

		var entity graph.EntityState
		if err := json.Unmarshal(entry.Value, &entity); err != nil {
			// Skip entities that fail to unmarshal
			continue
		}

		entities = append(entities, entity)
	}

	// Respond with entities array
	c.respondJSON(msg, entities)
}

// respondError sends an error response
func (c *Component) respondError(msg queryMsg, errorMsg string) {
	response := map[string]string{"error": errorMsg}
	data, _ := json.Marshal(response)
	if err := msg.Respond(data); err != nil {
		c.logger.Error("failed to respond with error", "error", err)
	}
}

// respondJSON sends a JSON response
func (c *Component) respondJSON(msg queryMsg, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		c.respondError(msg, "internal error")
		return
	}
	if err := msg.Respond(data); err != nil {
		c.logger.Error("failed to respond with JSON", "error", err)
	}
}
