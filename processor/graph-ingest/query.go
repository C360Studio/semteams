// Package graphingest query handlers
package graphingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/nats-io/nats.go/jetstream"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to entity query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.entity", c.handleQueryEntityNATS); err != nil {
		return fmt.Errorf("subscribe entity query: %w", err)
	}

	// Subscribe to batch query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.batch", c.handleQueryBatchNATS); err != nil {
		return fmt.Errorf("subscribe batch query: %w", err)
	}

	// Subscribe to prefix query (for hierarchy listing)
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.ingest.query.prefix", c.handleQueryPrefixNATS); err != nil {
		return fmt.Errorf("subscribe prefix query: %w", err)
	}

	// Subscribe to capabilities discovery
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.ingest.capabilities", c.handleCapabilitiesNATS); err != nil {
		return fmt.Errorf("subscribe capabilities: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.ingest.query.entity", "graph.ingest.query.batch", "graph.ingest.query.prefix", "graph.ingest.capabilities"})

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
		if err == jetstream.ErrKeyNotFound {
			return nil, fmt.Errorf("not found: %s", req.ID)
		}
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return entry.Value(), nil
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

	// Handle empty IDs (return empty array)
	if len(req.IDs) == 0 {
		return []byte("[]"), nil
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
		if err := json.Unmarshal(entry.Value(), &entity); err != nil {
			// Skip entities that fail to unmarshal
			continue
		}

		entities = append(entities, entity)
	}

	// Return entities array
	return json.Marshal(entities)
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

// handleCapabilitiesNATS handles capability discovery requests via NATS request/reply
func (c *Component) handleCapabilitiesNATS(_ context.Context, _ []byte) ([]byte, error) {
	caps := c.QueryCapabilities()
	return json.Marshal(caps)
}

// Ensure Component implements QueryCapabilityProvider
var _ component.QueryCapabilityProvider = (*Component)(nil)

// queryMsg is an interface for query request messages.
// This accommodates both real NATS messages and test mocks.
type queryMsg interface {
	Data() []byte
	Respond(data []byte) error
}

// QueryCapabilities implements QueryCapabilityProvider interface
func (c *Component) QueryCapabilities() component.QueryCapabilities {
	return component.QueryCapabilities{
		Component: "graph-ingest",
		Version:   "1.0.0",
		Queries: []component.QueryCapability{
			{
				Subject:     "graph.ingest.query.entity",
				Operation:   "getEntity",
				Description: "Get single entity by ID",
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Entity ID to retrieve",
						},
					},
					"required": []string{"id"},
				},
				ResponseSchema: map[string]any{
					"$ref": "#/definitions/EntityState",
				},
			},
			{
				Subject:     "graph.ingest.query.batch",
				Operation:   "getBatch",
				Description: "Get multiple entities by IDs",
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ids": map[string]any{
							"type":        "array",
							"description": "Array of entity IDs to retrieve",
							"items": map[string]any{
								"type": "string",
							},
						},
					},
					"required": []string{"ids"},
				},
				ResponseSchema: map[string]any{
					"type": "array",
					"items": map[string]any{
						"$ref": "#/definitions/EntityState",
					},
				},
			},
			{
				Subject:     "graph.ingest.query.prefix",
				Operation:   "listByPrefix",
				Description: "List entity IDs matching a prefix (for hierarchy queries)",
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"prefix": map[string]any{
							"type":        "string",
							"description": "Entity ID prefix to match (empty for all entities)",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum number of IDs to return (default 1000)",
						},
					},
				},
				ResponseSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ids": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"total": map[string]any{
							"type": "integer",
						},
					},
				},
			},
		},
		Definitions: map[string]any{
			"EntityState": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type": "string",
					},
					"triples": map[string]any{
						"type": "array",
					},
					"version": map[string]any{
						"type": "integer",
					},
					"updated_at": map[string]any{
						"type":   "string",
						"format": "date-time",
					},
				},
			},
		},
	}
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
		if err == jetstream.ErrKeyNotFound {
			c.respondError(msg, "not found")
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Respond with entity JSON
	if err := msg.Respond(entry.Value()); err != nil {
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
		if err := json.Unmarshal(entry.Value(), &entity); err != nil {
			// Skip entities that fail to unmarshal
			continue
		}

		entities = append(entities, entity)
	}

	// Respond with entities array
	c.respondJSON(msg, entities)
}

// handleCapabilities handles capability discovery requests
func (c *Component) handleCapabilities(msg queryMsg) {
	caps := c.QueryCapabilities()
	c.respondJSON(msg, caps)
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
