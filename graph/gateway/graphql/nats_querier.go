// Package graphql provides GraphQL gateway implementation.
// nats_querier implements Querier using NATS request/reply to graph components.
package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/clustering"
	"github.com/c360/semstreams/natsclient"
)

// NATSQuerier implements Querier using NATS request/reply.
// It routes queries to the appropriate graph component endpoints.
type NATSQuerier struct {
	client  *natsclient.Client
	timeout time.Duration
	logger  *slog.Logger
}

// Compile-time interface check
var _ Querier = (*NATSQuerier)(nil)

// NewNATSQuerier creates a new NATS-based querier.
func NewNATSQuerier(client *natsclient.Client, timeout time.Duration, logger *slog.Logger) *NATSQuerier {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &NATSQuerier{
		client:  client,
		timeout: timeout,
		logger:  logger,
	}
}

// request performs a NATS request and unmarshals the response.
func (q *NATSQuerier) request(ctx context.Context, subject string, req any, resp any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	respData, err := q.client.Request(ctx, subject, data, q.timeout)
	if err != nil {
		return fmt.Errorf("request %s: %w", subject, err)
	}

	// Check for error response
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(respData, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("%s: %s", subject, errResp.Error)
	}

	if err := json.Unmarshal(respData, resp); err != nil {
		return fmt.Errorf("unmarshal response from %s: %w", subject, err)
	}

	return nil
}

// GetEntity queries a single entity by ID via graph-ingest.
func (q *NATSQuerier) GetEntity(ctx context.Context, id string) (*graph.EntityState, error) {
	var resp struct {
		Entity *graph.EntityState `json:"entity"`
	}

	if err := q.request(ctx, "graph.ingest.query.entity", map[string]string{"id": id}, &resp); err != nil {
		return nil, err
	}

	return resp.Entity, nil
}

// GetEntities queries multiple entities by ID via graph-ingest.
func (q *NATSQuerier) GetEntities(ctx context.Context, ids []string) ([]*graph.EntityState, error) {
	if len(ids) == 0 {
		return []*graph.EntityState{}, nil
	}

	var resp struct {
		Entities []*graph.EntityState `json:"entities"`
	}

	if err := q.request(ctx, "graph.ingest.query.entities", map[string]any{"ids": ids}, &resp); err != nil {
		return nil, err
	}

	return resp.Entities, nil
}

// GetEntityByAlias queries an entity by alias or direct ID.
// First tries to resolve aliasOrID as an alias via graph-index.
// If not found as alias, tries it as a direct entity ID via graph-ingest.
func (q *NATSQuerier) GetEntityByAlias(ctx context.Context, aliasOrID string) (*graph.EntityState, error) {
	// Try to resolve as alias first via graph-index
	var aliasResp graph.AliasQueryResponse

	entityID := aliasOrID // Default to using input as entity ID

	err := q.request(ctx, "graph.index.query.alias", map[string]string{"alias": aliasOrID}, &aliasResp)
	if err == nil && aliasResp.Error == nil && aliasResp.Data.CanonicalID != nil {
		// Alias resolved - use canonical ID
		entityID = *aliasResp.Data.CanonicalID
	}
	// If alias lookup failed, we'll try aliasOrID as a direct entity ID

	// Fetch entity by ID via graph-ingest
	return q.GetEntity(ctx, entityID)
}

// ExecutePath performs path search via graph-query.
func (q *NATSQuerier) ExecutePath(ctx context.Context, start string, pattern PathPattern) (*QueryResult, error) {
	req := map[string]any{
		"start_entity":     start,
		"max_depth":        pattern.MaxDepth,
		"max_nodes":        pattern.MaxNodes,
		"direction":        string(pattern.Direction),
		"edge_types":       pattern.EdgeTypes,
		"decay_factor":     pattern.DecayFactor,
		"include_self":     pattern.IncludeSelf,
		"include_siblings": pattern.IncludeSiblings,
	}

	var resp QueryResult
	if err := q.request(ctx, "graph.query.pathSearch", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetGraphSnapshot retrieves a bounded subgraph via graph-query.
func (q *NATSQuerier) GetGraphSnapshot(ctx context.Context, bounds QueryBounds) (*QMGraphSnapshot, error) {
	req := map[string]any{
		"entity_types": bounds.EntityTypes,
		"max_entities": bounds.MaxEntities,
	}

	if bounds.Spatial != nil {
		req["spatial"] = map[string]float64{
			"north": bounds.Spatial.North,
			"south": bounds.Spatial.South,
			"east":  bounds.Spatial.East,
			"west":  bounds.Spatial.West,
		}
	}

	if bounds.Temporal != nil {
		req["temporal"] = map[string]time.Time{
			"start": bounds.Temporal.Start,
			"end":   bounds.Temporal.End,
		}
	}

	var resp QMGraphSnapshot
	if err := q.request(ctx, "graph.query.snapshot", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// QueryRelationships queries relationships via graph-query.
func (q *NATSQuerier) QueryRelationships(ctx context.Context, entityID string, direction Direction) ([]*QMRelationship, error) {
	req := map[string]string{
		"entity_id": entityID,
		"direction": string(direction),
	}

	var resp struct {
		Relationships []*QMRelationship `json:"relationships"`
	}

	if err := q.request(ctx, "graph.query.relationships", req, &resp); err != nil {
		return nil, err
	}

	return resp.Relationships, nil
}

// LocalSearch performs GraphRAG local search via graph-query.
func (q *NATSQuerier) LocalSearch(ctx context.Context, entityID string, query string, level int) (*QMLocalSearchResult, error) {
	req := map[string]any{
		"entity_id": entityID,
		"query":     query,
		"level":     level,
	}

	var resp QMLocalSearchResult
	if err := q.request(ctx, "graph.query.localSearch", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GlobalSearch performs GraphRAG global search via graph-query.
func (q *NATSQuerier) GlobalSearch(ctx context.Context, query string, level int, maxCommunities int) (*QMGlobalSearchResult, error) {
	req := map[string]any{
		"query":           query,
		"level":           level,
		"max_communities": maxCommunities,
	}

	var resp QMGlobalSearchResult
	if err := q.request(ctx, "graph.query.globalSearch", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GlobalSearchWithOptions performs GraphRAG global search with advanced options.
func (q *NATSQuerier) GlobalSearchWithOptions(ctx context.Context, opts *SearchOptions) (*QMGlobalSearchResult, error) {
	var resp QMGlobalSearchResult
	if err := q.request(ctx, "graph.query.globalSearchWithOptions", opts, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetCommunity retrieves a community by ID via graph-clustering.
func (q *NATSQuerier) GetCommunity(ctx context.Context, communityID string) (*clustering.Community, error) {
	var resp struct {
		Community *clustering.Community `json:"community"`
	}

	if err := q.request(ctx, "graph.clustering.query.community", map[string]string{"id": communityID}, &resp); err != nil {
		return nil, err
	}

	return resp.Community, nil
}

// GetEntityCommunity retrieves the community containing an entity via graph-clustering.
func (q *NATSQuerier) GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error) {
	req := map[string]any{
		"entity_id": entityID,
		"level":     level,
	}

	var resp struct {
		Community *clustering.Community `json:"community"`
	}

	if err := q.request(ctx, "graph.clustering.query.entity", req, &resp); err != nil {
		return nil, err
	}

	return resp.Community, nil
}

// GetCommunitiesByLevel retrieves all communities at a level via graph-clustering.
func (q *NATSQuerier) GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error) {
	var resp struct {
		Communities []*clustering.Community `json:"communities"`
	}

	if err := q.request(ctx, "graph.clustering.query.level", map[string]int{"level": level}, &resp); err != nil {
		return nil, err
	}

	return resp.Communities, nil
}

// QueryByPredicate queries entities by predicate via graph-ingest.
func (q *NATSQuerier) QueryByPredicate(ctx context.Context, predicate string) ([]string, error) {
	var resp struct {
		EntityIDs []string `json:"entity_ids"`
	}

	if err := q.request(ctx, "graph.ingest.query.predicate", map[string]string{"predicate": predicate}, &resp); err != nil {
		return nil, err
	}

	return resp.EntityIDs, nil
}

// QuerySpatial queries entities within spatial bounds via graph-index-spatial.
func (q *NATSQuerier) QuerySpatial(ctx context.Context, bounds SpatialBounds) ([]string, error) {
	req := map[string]float64{
		"north": bounds.North,
		"south": bounds.South,
		"east":  bounds.East,
		"west":  bounds.West,
	}

	var resp struct {
		EntityIDs []string `json:"entity_ids"`
	}

	if err := q.request(ctx, "graph.spatial.query.bounds", req, &resp); err != nil {
		return nil, err
	}

	return resp.EntityIDs, nil
}

// QueryTemporal queries entities within temporal range via graph-index-temporal.
func (q *NATSQuerier) QueryTemporal(ctx context.Context, start, end time.Time) ([]string, error) {
	req := map[string]time.Time{
		"start": start,
		"end":   end,
	}

	var resp struct {
		EntityIDs []string `json:"entity_ids"`
	}

	if err := q.request(ctx, "graph.temporal.query.range", req, &resp); err != nil {
		return nil, err
	}

	return resp.EntityIDs, nil
}

// InvalidateEntity invalidates cached entity data.
// This is a local operation - NATS querier doesn't maintain a cache.
func (q *NATSQuerier) InvalidateEntity(_ string) error {
	// NATS querier doesn't cache - nothing to invalidate
	return nil
}

// WarmCache pre-warms cache with entity data.
// This is a no-op for NATS querier which doesn't maintain a local cache.
func (q *NATSQuerier) WarmCache(_ context.Context, _ []string) error {
	// NATS querier doesn't cache - nothing to warm
	return nil
}

// GetCacheStats returns cache statistics.
// Returns empty stats since NATS querier doesn't maintain a cache.
func (q *NATSQuerier) GetCacheStats() CacheStats {
	return CacheStats{}
}

// ListWithPrefix returns entity IDs matching a prefix via graph-ingest.
func (q *NATSQuerier) ListWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	var resp struct {
		EntityIDs []string `json:"entity_ids"`
	}

	if err := q.request(ctx, "graph.ingest.query.prefix", map[string]string{"prefix": prefix}, &resp); err != nil {
		return nil, err
	}

	return resp.EntityIDs, nil
}

// GetHierarchyStats returns entity hierarchy statistics via graph-ingest.
func (q *NATSQuerier) GetHierarchyStats(ctx context.Context, prefix string) (*HierarchyStats, error) {
	var resp HierarchyStats

	if err := q.request(ctx, "graph.ingest.query.hierarchy", map[string]string{"prefix": prefix}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SearchSimilar performs similarity search via graph-embedding.
func (q *NATSQuerier) SearchSimilar(ctx context.Context, query string, limit int) (*QMSimilaritySearchResult, error) {
	req := map[string]any{
		"query": query,
		"limit": limit,
	}

	var resp QMSimilaritySearchResult
	if err := q.request(ctx, "graph.embedding.query.search", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// ResolvePartialEntityID resolves a partial entity ID to a full 6-part ID.
// Uses the EntityID structure: org.platform.domain.system.type.instance
//
// Resolution strategy:
// 1. If already full (5+ dots), return as-is
// 2. Try alias lookup via graph-index
// 3. Try wildcard suffix match via graph-ingest (*.*.*.*.*.partial)
//
// This enables NL queries to use partial IDs like "temp-sensor-001" which get
// resolved to full IDs like "c360.logistics.environmental.sensor.temperature.temp-sensor-001".
func (q *NATSQuerier) ResolvePartialEntityID(ctx context.Context, partial string) (string, error) {
	// If already looks like a full 6-part ID (has 5+ dots), return as-is
	dotCount := 0
	for _, c := range partial {
		if c == '.' {
			dotCount++
		}
	}
	if dotCount >= 5 {
		return partial, nil
	}

	// Step 1: Try alias lookup first via graph-index
	var aliasResp graph.AliasQueryResponse
	err := q.request(ctx, "graph.index.query.alias", map[string]string{"alias": partial}, &aliasResp)
	if err == nil && aliasResp.Error == nil && aliasResp.Data.CanonicalID != nil {
		return *aliasResp.Data.CanonicalID, nil
	}

	// Step 2: Try wildcard suffix match via graph-ingest
	var suffixResp struct {
		ID string `json:"id"`
	}
	err = q.request(ctx, "graph.ingest.query.suffix", map[string]string{"suffix": partial}, &suffixResp)
	if err == nil && suffixResp.ID != "" {
		return suffixResp.ID, nil
	}

	// No match found
	return "", nil
}
