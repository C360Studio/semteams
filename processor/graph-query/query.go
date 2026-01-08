// Package graphquery query handlers
package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nats-io/nats.go"
)

// setupQueryHandlers subscribes to all query request subjects
func (c *Component) setupQueryHandlers() error {
	// Subscribe to entity query passthrough
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.entity", c.handleQueryEntity); err != nil {
		return fmt.Errorf("subscribe to entity query: %w", err)
	}

	// Subscribe to entity by alias query (resolves alias then fetches entity)
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.entityByAlias", c.handleQueryEntityByAlias); err != nil {
		return fmt.Errorf("subscribe to entityByAlias query: %w", err)
	}

	// Subscribe to relationships query passthrough
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.relationships", c.handleQueryRelationships); err != nil {
		return fmt.Errorf("subscribe to relationships query: %w", err)
	}

	// Subscribe to path search orchestration
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.pathSearch", c.handlePathSearch); err != nil {
		return fmt.Errorf("subscribe to path search: %w", err)
	}

	// Subscribe to hierarchy stats (orchestrates prefix query to graph-ingest)
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.hierarchyStats", c.handleQueryHierarchyStats); err != nil {
		return fmt.Errorf("subscribe to hierarchy stats: %w", err)
	}

	// Subscribe to prefix query (passthrough to graph-ingest)
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.prefix", c.handleQueryPrefix); err != nil {
		return fmt.Errorf("subscribe to prefix query: %w", err)
	}

	// Subscribe to spatial query passthrough
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.spatial", c.handleQuerySpatial); err != nil {
		return fmt.Errorf("subscribe to spatial query: %w", err)
	}

	// Subscribe to temporal query passthrough
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.temporal", c.handleQueryTemporal); err != nil {
		return fmt.Errorf("subscribe to temporal query: %w", err)
	}

	// Subscribe to semantic search (passthrough to graph-embedding)
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.semantic", c.handleQuerySemantic); err != nil {
		return fmt.Errorf("subscribe to semantic query: %w", err)
	}

	// Subscribe to similar entity search (passthrough to graph-embedding)
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.similar", c.handleQuerySimilar); err != nil {
		return fmt.Errorf("subscribe to similar query: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.query.entity", "graph.query.entityByAlias", "graph.query.relationships", "graph.query.pathSearch", "graph.query.hierarchyStats", "graph.query.prefix", "graph.query.spatial", "graph.query.temporal", "graph.query.semantic", "graph.query.similar"})

	return nil
}

// handleQueryEntity handles entity query requests (passthrough to graph-ingest)
func (c *Component) handleQueryEntity(ctx context.Context, data []byte) ([]byte, error) {
	// Parse and validate request
	var req map[string]string
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req["id"] == "" {
		return nil, errors.New("invalid request: empty id")
	}

	// Route to entity query
	subject := c.router.Route("entity")
	if subject == "" {
		return nil, errors.New("entity query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query entity failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handleQueryEntityByAlias resolves an alias to an entity ID, then fetches the entity.
// It first tries to resolve aliasOrID via graph-index alias lookup.
// If found, it fetches the entity using the canonical ID.
// If not found as alias, it tries aliasOrID as a direct entity ID.
func (c *Component) handleQueryEntityByAlias(ctx context.Context, data []byte) ([]byte, error) {
	// Parse request
	var req struct {
		AliasOrID string `json:"aliasOrID"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.AliasOrID == "" {
		return nil, errors.New("invalid request: empty aliasOrID")
	}

	entityID := req.AliasOrID // Default to using input as entity ID

	// Try to resolve as alias first via graph-index
	aliasReq := map[string]string{"alias": req.AliasOrID}
	aliasReqData, _ := json.Marshal(aliasReq)

	aliasSubject := c.router.Route("alias")
	if aliasSubject == "" {
		return nil, errors.New("alias query routing not available")
	}
	aliasResp, err := c.natsClient.Request(ctx, aliasSubject, aliasReqData, c.config.QueryTimeout)
	if err == nil {
		// Check if response contains a canonical_id
		var aliasResult struct {
			CanonicalID string `json:"canonical_id"`
		}
		if json.Unmarshal(aliasResp, &aliasResult) == nil && aliasResult.CanonicalID != "" {
			entityID = aliasResult.CanonicalID
		}
	}
	// If alias lookup failed, we'll try aliasOrID as a direct entity ID

	// Now fetch the entity using the resolved (or original) ID
	entityReq := map[string]string{"id": entityID}
	entityReqData, _ := json.Marshal(entityReq)

	entitySubject := c.router.Route("entity")
	if entitySubject == "" {
		return nil, errors.New("entity query routing not available")
	}
	response, err := c.natsClient.Request(ctx, entitySubject, entityReqData, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query entity failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handleQueryPrefix handles prefix query requests (passthrough to graph-ingest)
func (c *Component) handleQueryPrefix(ctx context.Context, data []byte) ([]byte, error) {
	// Forward to graph-ingest
	subject := c.router.Route("entityPrefix")
	if subject == "" {
		return nil, errors.New("entityPrefix query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query prefix failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handleQueryRelationships handles relationship query requests (passthrough to graph-index)
func (c *Component) handleQueryRelationships(ctx context.Context, data []byte) ([]byte, error) {
	// Parse and validate request
	var req struct {
		EntityID  string `json:"entity_id"`
		Direction string `json:"direction"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.EntityID == "" {
		return nil, errors.New("invalid request: empty entity_id")
	}

	// Route based on direction
	queryType := "outgoing"
	if req.Direction == "incoming" {
		queryType = "incoming"
	}
	subject := c.router.Route(queryType)
	if subject == "" {
		return nil, fmt.Errorf("%s query routing not available", queryType)
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		return nil, fmt.Errorf("query relationships failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handlePathSearch handles PathRAG traversal queries
func (c *Component) handlePathSearch(ctx context.Context, data []byte) ([]byte, error) {
	var req PathSearchRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Ensure pathSearcher is initialized (for testing with direct component construction)
	searcher := c.pathSearcher
	if searcher == nil {
		searcher = NewPathSearcher(c.natsClient, c.config.QueryTimeout, c.config.MaxDepth, c.logger)
	}

	result, err := searcher.Search(ctx, req)
	if err != nil {
		c.recordError(err)
		return nil, err
	}

	// Return raw PathSearch response - gateway wraps in GraphQL format
	responseData, err := json.Marshal(result)
	if err != nil {
		c.recordError(err)
		return nil, fmt.Errorf("marshal response: %w", err)
	}

	c.recordSuccess(len(data), len(responseData))
	return responseData, nil
}

// HierarchyChild represents a child node in the hierarchy
type HierarchyChild struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

// handleQueryHierarchyStats handles hierarchy stats queries by orchestrating graph-ingest
func (c *Component) handleQueryHierarchyStats(ctx context.Context, data []byte) ([]byte, error) {
	// Parse request
	var req struct {
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Get all entity IDs with prefix from graph-ingest
	prefixReq, err := json.Marshal(map[string]any{"prefix": req.Prefix, "limit": 10000})
	if err != nil {
		return nil, fmt.Errorf("marshal prefix request: %w", err)
	}

	subject := c.router.Route("entityPrefix")
	if subject == "" {
		return nil, errors.New("entityPrefix query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, prefixReq, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query prefix failed: %w", err)
	}

	// Parse prefix response (uses GraphQL-compatible field names)
	var prefixResp struct {
		EntityIDs  []string `json:"entityIds"`
		TotalCount int      `json:"totalCount"`
	}
	if err := json.Unmarshal(response, &prefixResp); err != nil {
		return nil, fmt.Errorf("parse prefix response: %w", err)
	}

	// Group by next hierarchy level
	childCounts := make(map[string]int)
	for _, id := range prefixResp.EntityIDs {
		nextLevel := extractNextLevel(id, req.Prefix)
		if nextLevel != "" {
			childCounts[nextLevel]++
		}
	}

	// Build sorted children array
	children := buildSortedChildren(childCounts)

	// Build response
	result := map[string]any{
		"prefix":        req.Prefix,
		"totalEntities": prefixResp.TotalCount,
		"children":      children,
	}

	responseData, err := json.Marshal(result)
	if err != nil {
		c.recordError(err)
		return nil, fmt.Errorf("marshal response: %w", err)
	}

	c.recordSuccess(len(data), len(responseData))
	return responseData, nil
}

// extractNextLevel extracts the next hierarchy segment after the given prefix
// For example: extractNextLevel("c360.a.b.c", "c360") returns "c360.a"
func extractNextLevel(entityID, prefix string) string {
	// Handle empty prefix (root level)
	if prefix == "" {
		// Return first segment
		parts := strings.SplitN(entityID, ".", 2)
		if len(parts) > 0 {
			return parts[0]
		}
		return ""
	}

	// Entity must start with prefix
	prefixDot := prefix + "."
	if !strings.HasPrefix(entityID, prefixDot) {
		// Check for exact match (entity IS the prefix)
		if entityID == prefix {
			return ""
		}
		return ""
	}

	// Get the part after the prefix
	remainder := strings.TrimPrefix(entityID, prefixDot)
	if remainder == "" {
		return ""
	}

	// Get next segment
	parts := strings.SplitN(remainder, ".", 2)
	if len(parts) > 0 && parts[0] != "" {
		return prefix + "." + parts[0]
	}

	return ""
}

// extractLastSegment returns the last segment of a prefix for display name
func extractLastSegment(prefix string) string {
	if prefix == "" {
		return ""
	}
	parts := strings.Split(prefix, ".")
	return parts[len(parts)-1]
}

// buildSortedChildren builds a sorted slice of HierarchyChild from counts map
func buildSortedChildren(childCounts map[string]int) []HierarchyChild {
	children := make([]HierarchyChild, 0, len(childCounts))

	for prefix, count := range childCounts {
		children = append(children, HierarchyChild{
			Prefix: prefix,
			Name:   extractLastSegment(prefix),
			Count:  count,
		})
	}

	// Sort by count descending, then by name ascending
	sort.Slice(children, func(i, j int) bool {
		if children[i].Count != children[j].Count {
			return children[i].Count > children[j].Count
		}
		return children[i].Name < children[j].Name
	})

	return children
}

// handleQuerySpatial handles spatial query requests (passthrough to graph-index-spatial)
func (c *Component) handleQuerySpatial(ctx context.Context, data []byte) ([]byte, error) {
	// Route to spatial query
	subject := c.router.Route("spatial")
	if subject == "" {
		return nil, errors.New("spatial query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query spatial failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handleQueryTemporal handles temporal query requests (passthrough to graph-index-temporal)
func (c *Component) handleQueryTemporal(ctx context.Context, data []byte) ([]byte, error) {
	// Route to temporal query
	subject := c.router.Route("temporal")
	if subject == "" {
		return nil, errors.New("temporal query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query temporal failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handleQuerySemantic handles semantic search requests (passthrough to graph-embedding)
func (c *Component) handleQuerySemantic(ctx context.Context, data []byte) ([]byte, error) {
	// Route to semantic query
	subject := c.router.Route("semantic")
	if subject == "" {
		return nil, errors.New("semantic query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query semantic failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}

// handleQuerySimilar handles similar entity requests (passthrough to graph-embedding)
func (c *Component) handleQuerySimilar(ctx context.Context, data []byte) ([]byte, error) {
	// Forward to graph-embedding's similar handler
	subject := c.router.Route("similar")
	if subject == "" {
		return nil, errors.New("similar query routing not available")
	}
	response, err := c.natsClient.Request(ctx, subject, data, c.config.QueryTimeout)
	if err != nil {
		c.recordError(err)
		if errors.Is(err, nats.ErrTimeout) {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		return nil, fmt.Errorf("query similar failed: %w", err)
	}

	c.recordSuccess(len(data), len(response))
	return response, nil
}
