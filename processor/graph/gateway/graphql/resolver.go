package graphql

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/querymanager"
)

// MetricsRecorder interface for recording GraphQL operation metrics.
type MetricsRecorder interface {
	RecordMetrics(ctx context.Context, operation string, fn func() error) error
}

// DefaultMetricsRecorder provides a simple metrics recorder with atomic counters.
// It tracks request counts, success/failure rates, and last activity time.
type DefaultMetricsRecorder struct {
	logger          *slog.Logger
	requestsTotal   atomic.Uint64
	requestsSuccess atomic.Uint64
	requestsFailed  atomic.Uint64
	lastActivity    atomic.Value // stores time.Time
	mu              sync.RWMutex
}

// NewMetricsRecorder creates a new default metrics recorder.
func NewMetricsRecorder(logger *slog.Logger) *DefaultMetricsRecorder {
	r := &DefaultMetricsRecorder{
		logger: logger,
	}
	r.lastActivity.Store(time.Now())
	return r
}

// RecordMetrics wraps a GraphQL operation to record metrics.
func (r *DefaultMetricsRecorder) RecordMetrics(ctx context.Context, operation string, fn func() error) error {
	start := time.Now()

	r.requestsTotal.Add(1)

	err := fn()
	duration := time.Since(start)

	if err != nil {
		r.requestsFailed.Add(1)
		if r.logger != nil {
			r.logger.WarnContext(ctx, "GraphQL operation failed",
				"operation", operation,
				"duration_ms", duration.Milliseconds(),
				"error", err)
		}
	} else {
		r.requestsSuccess.Add(1)
		if r.logger != nil {
			r.logger.DebugContext(ctx, "GraphQL operation succeeded",
				"operation", operation,
				"duration_ms", duration.Milliseconds())
		}
	}

	r.lastActivity.Store(time.Now())

	return err
}

// Stats returns current metrics.
func (r *DefaultMetricsRecorder) Stats() (total, success, failed uint64, lastActivity time.Time) {
	total = r.requestsTotal.Load()
	success = r.requestsSuccess.Load()
	failed = r.requestsFailed.Load()
	if v := r.lastActivity.Load(); v != nil {
		lastActivity = v.(time.Time)
	}
	return
}

// Resolver provides resolver methods for GraphQL queries.
// It uses the QueryManager directly for in-process access to graph data.
type Resolver struct {
	queryManager    querymanager.Querier
	metricsRecorder MetricsRecorder
}

// NewResolver creates a new resolver with direct QueryManager access.
func NewResolver(queryManager querymanager.Querier, metricsRecorder MetricsRecorder) *Resolver {
	return &Resolver{
		queryManager:    queryManager,
		metricsRecorder: metricsRecorder,
	}
}

// QueryEntityByID queries a single entity by ID.
// Returns nil if entity not found (GraphQL null).
func (r *Resolver) QueryEntityByID(ctx context.Context, id string) (*Entity, error) {
	var entity *Entity
	var err error

	queryFn := func() error {
		entityState, qErr := r.queryManager.GetEntity(ctx, id)
		if qErr != nil {
			return qErr
		}
		entity = convertEntityStateToGraphQL(entityState)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "QueryEntityByID", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "QueryEntityByID")
	}
	return entity, nil
}

// QueryEntityByAlias queries a single entity by alias or ID.
// Returns nil if entity not found (GraphQL null).
func (r *Resolver) QueryEntityByAlias(ctx context.Context, aliasOrID string) (*Entity, error) {
	var entity *Entity
	var err error

	queryFn := func() error {
		entityState, qErr := r.queryManager.GetEntityByAlias(ctx, aliasOrID)
		if qErr != nil {
			return qErr
		}
		entity = convertEntityStateToGraphQL(entityState)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "QueryEntityByAlias", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "QueryEntityByAlias")
	}
	return entity, nil
}

// QueryEntitiesByIDs queries multiple entities by their IDs (batch operation).
func (r *Resolver) QueryEntitiesByIDs(ctx context.Context, ids []string) ([]*Entity, error) {
	if len(ids) == 0 {
		return []*Entity{}, nil
	}

	var entities []*Entity
	var err error

	queryFn := func() error {
		entityStates, qErr := r.queryManager.GetEntities(ctx, ids)
		if qErr != nil {
			return qErr
		}
		entities = convertEntityStatesToGraphQL(entityStates)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "QueryEntitiesByIDs", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "QueryEntitiesByIDs")
	}
	return entities, nil
}

// QueryEntitiesByType queries all entities of a specific type.
func (r *Resolver) QueryEntitiesByType(ctx context.Context, entityType string, limit int) ([]*Entity, error) {
	var entities []*Entity
	var err error

	queryFn := func() error {
		entityIDs, qErr := r.queryManager.QueryByPredicate(ctx, entityType)
		if qErr != nil {
			return qErr
		}

		if limit > 0 && len(entityIDs) > limit {
			entityIDs = entityIDs[:limit]
		}

		entityStates, qErr := r.queryManager.GetEntities(ctx, entityIDs)
		if qErr != nil {
			return qErr
		}

		entities = convertEntityStatesToGraphQL(entityStates)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "QueryEntitiesByType", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "QueryEntitiesByType")
	}
	return entities, nil
}

// QueryRelationships queries relationships based on filters.
func (r *Resolver) QueryRelationships(ctx context.Context, filters RelationshipFilters) ([]*Relationship, error) {
	var relationships []*Relationship
	var err error

	queryFn := func() error {
		direction := querymanager.DirectionOutgoing
		switch filters.Direction {
		case "incoming":
			direction = querymanager.DirectionIncoming
		case "both":
			direction = querymanager.DirectionBoth
		}

		rels, qErr := r.queryManager.QueryRelationships(ctx, filters.EntityID, direction)
		if qErr != nil {
			return qErr
		}

		relationships = make([]*Relationship, 0, len(rels))
		for _, rel := range rels {
			if len(filters.EdgeTypes) > 0 {
				found := false
				for _, edgeType := range filters.EdgeTypes {
					if rel.EdgeType == edgeType {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			relationships = append(relationships, &Relationship{
				FromEntityID: rel.FromEntityID,
				ToEntityID:   rel.ToEntityID,
				EdgeType:     rel.EdgeType,
			})
		}
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "QueryRelationships", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "QueryRelationships")
	}
	return relationships, nil
}

// LocalSearch performs a local search within an entity's community.
func (r *Resolver) LocalSearch(ctx context.Context, entityID string, query string, level int) (*LocalSearchResult, error) {
	var result *LocalSearchResult
	var err error

	queryFn := func() error {
		qmResult, qErr := r.queryManager.LocalSearch(ctx, entityID, query, level)
		if qErr != nil {
			return qErr
		}

		result = &LocalSearchResult{
			Entities:    convertEntityStatesToGraphQL(qmResult.Entities),
			CommunityID: qmResult.CommunityID,
			Count:       qmResult.Count,
		}
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "LocalSearch", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "LocalSearch")
	}
	return result, nil
}

// GlobalSearch performs a global search across community summaries.
func (r *Resolver) GlobalSearch(ctx context.Context, query string, level int, maxCommunities int) (*GlobalSearchResult, error) {
	var result *GlobalSearchResult
	var err error

	queryFn := func() error {
		qmResult, qErr := r.queryManager.GlobalSearch(ctx, query, level, maxCommunities)
		if qErr != nil {
			return qErr
		}

		summaries := make([]CommunitySummary, len(qmResult.CommunitySummaries))
		for i, s := range qmResult.CommunitySummaries {
			summaries[i] = CommunitySummary{
				CommunityID: s.CommunityID,
				Summary:     s.Summary,
				Keywords:    s.Keywords,
				Level:       s.Level,
				Relevance:   s.Relevance,
			}
		}

		result = &GlobalSearchResult{
			Entities:           convertEntityStatesToGraphQL(qmResult.Entities),
			CommunitySummaries: summaries,
			Count:              qmResult.Count,
		}
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "GlobalSearch", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "GlobalSearch")
	}
	return result, nil
}

// GetCommunity retrieves a community by ID.
func (r *Resolver) GetCommunity(ctx context.Context, communityID string) (*Community, error) {
	var community *Community
	var err error

	queryFn := func() error {
		comm, qErr := r.queryManager.GetCommunity(ctx, communityID)
		if qErr != nil {
			return qErr
		}
		community = convertCommunityToGraphQL(comm)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "GetCommunity", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "GetCommunity")
	}
	return community, nil
}

// GetEntityCommunity retrieves the community containing a specific entity.
func (r *Resolver) GetEntityCommunity(ctx context.Context, entityID string, level int) (*Community, error) {
	var community *Community
	var err error

	queryFn := func() error {
		comm, qErr := r.queryManager.GetEntityCommunity(ctx, entityID, level)
		if qErr != nil {
			return qErr
		}
		community = convertCommunityToGraphQL(comm)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "GetEntityCommunity", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "GetEntityCommunity")
	}
	return community, nil
}

// CommunitiesByLevel retrieves all communities at a specific hierarchical level.
func (r *Resolver) CommunitiesByLevel(ctx context.Context, level int) ([]*Community, error) {
	var communities []*Community
	var err error

	queryFn := func() error {
		comms, qErr := r.queryManager.GetCommunitiesByLevel(ctx, level)
		if qErr != nil {
			return qErr
		}

		communities = make([]*Community, len(comms))
		for i, comm := range comms {
			communities[i] = convertCommunityToGraphQL(comm)
		}
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "CommunitiesByLevel", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "CommunitiesByLevel")
	}
	return communities, nil
}

// PathSearch performs bounded graph traversal from a starting entity (PathRAG).
// When includeSiblings is true, PathRAG will also traverse inferred sibling relationships
// based on the 6-part EntityID structure (entities with the same type prefix are siblings).
func (r *Resolver) PathSearch(ctx context.Context, startEntity string,
	maxDepth, maxNodes int, direction string, edgeTypes []string,
	decayFactor float64, includeSiblings bool) (*PathSearchResult, error) {

	var result *PathSearchResult
	var err error

	queryFn := func() error {
		dir := querymanager.DirectionOutgoing
		switch strings.ToUpper(direction) {
		case "INCOMING":
			dir = querymanager.DirectionIncoming
		case "BOTH":
			dir = querymanager.DirectionBoth
		}

		pattern := querymanager.PathPattern{
			MaxDepth:        maxDepth,
			MaxNodes:        maxNodes,
			Direction:       dir,
			EdgeTypes:       edgeTypes,
			DecayFactor:     decayFactor,
			IncludeSelf:     true,
			IncludeSiblings: includeSiblings,
		}

		qmResult, qErr := r.queryManager.ExecutePath(ctx, startEntity, pattern)
		if qErr != nil {
			return qErr
		}

		result = convertPathResultToGraphQL(qmResult)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "PathSearch", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "PathSearch")
	}
	return result, nil
}

// SpatialSearch finds entities within geographic bounds.
func (r *Resolver) SpatialSearch(ctx context.Context,
	north, south, east, west float64, limit int) ([]*Entity, error) {
	var entities []*Entity
	var err error

	queryFn := func() error {
		bounds := querymanager.SpatialBounds{
			North: north,
			South: south,
			East:  east,
			West:  west,
		}

		entityIDs, qErr := r.queryManager.QuerySpatial(ctx, bounds)
		if qErr != nil {
			return qErr
		}

		if limit > 0 && len(entityIDs) > limit {
			entityIDs = entityIDs[:limit]
		}

		entityStates, qErr := r.queryManager.GetEntities(ctx, entityIDs)
		if qErr != nil {
			return qErr
		}

		entities = convertEntityStatesToGraphQL(entityStates)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "SpatialSearch", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "SpatialSearch")
	}
	return entities, nil
}

// TemporalSearch finds entities within a time range.
func (r *Resolver) TemporalSearch(ctx context.Context,
	startTime, endTime time.Time, limit int) ([]*Entity, error) {

	var entities []*Entity
	var err error

	queryFn := func() error {
		entityIDs, qErr := r.queryManager.QueryTemporal(ctx, startTime, endTime)
		if qErr != nil {
			return qErr
		}

		if limit > 0 && len(entityIDs) > limit {
			entityIDs = entityIDs[:limit]
		}

		entityStates, qErr := r.queryManager.GetEntities(ctx, entityIDs)
		if qErr != nil {
			return qErr
		}

		entities = convertEntityStatesToGraphQL(entityStates)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "TemporalSearch", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "TemporalSearch")
	}
	return entities, nil
}

// GraphSnapshot retrieves a bounded spatial/temporal subgraph.
func (r *Resolver) GraphSnapshot(ctx context.Context,
	north, south, east, west *float64,
	startTime, endTime *time.Time,
	entityTypes []string, maxEntities int) (*GraphSnapshot, error) {

	var snapshot *GraphSnapshot
	var err error

	queryFn := func() error {
		bounds := querymanager.QueryBounds{
			EntityTypes: entityTypes,
			MaxEntities: maxEntities,
		}

		if north != nil && south != nil && east != nil && west != nil {
			bounds.Spatial = &querymanager.SpatialBounds{
				North: *north,
				South: *south,
				East:  *east,
				West:  *west,
			}
		}

		if startTime != nil && endTime != nil {
			bounds.Temporal = &querymanager.TemporalBounds{
				Start: *startTime,
				End:   *endTime,
			}
		}

		qmSnapshot, qErr := r.queryManager.GetGraphSnapshot(ctx, bounds)
		if qErr != nil {
			return qErr
		}

		snapshot = convertSnapshotToGraphQL(qmSnapshot)
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "GraphSnapshot", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "GraphSnapshot")
	}
	return snapshot, nil
}

// EntitiesByPrefix retrieves entities matching an EntityID prefix.
// This enables hierarchical navigation of the 6-part EntityID structure.
func (r *Resolver) EntitiesByPrefix(ctx context.Context, prefix string, limit int) (*PrefixQueryResult, error) {
	var result *PrefixQueryResult
	var err error

	queryFn := func() error {
		entityIDs, qErr := r.queryManager.ListWithPrefix(ctx, prefix)
		if qErr != nil {
			return qErr
		}

		totalCount := len(entityIDs)
		truncated := false

		if limit > 0 && len(entityIDs) > limit {
			entityIDs = entityIDs[:limit]
			truncated = true
		}

		result = &PrefixQueryResult{
			EntityIDs:  entityIDs,
			TotalCount: totalCount,
			Truncated:  truncated,
			Prefix:     prefix,
		}
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "EntitiesByPrefix", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "EntitiesByPrefix")
	}
	return result, nil
}

// EntityIdHierarchy retrieves statistics about the EntityID hierarchy.
// This enables understanding the graph structure at each level.
func (r *Resolver) EntityIdHierarchy(ctx context.Context, prefix string) (*HierarchyStats, error) {
	var result *HierarchyStats
	var err error

	queryFn := func() error {
		stats, qErr := r.queryManager.GetHierarchyStats(ctx, prefix)
		if qErr != nil {
			return qErr
		}

		children := make([]HierarchyLevel, len(stats.Children))
		for i, c := range stats.Children {
			children[i] = HierarchyLevel{
				Prefix: c.Prefix,
				Name:   c.Name,
				Count:  c.Count,
			}
		}

		result = &HierarchyStats{
			Prefix:        stats.Prefix,
			TotalEntities: stats.TotalEntities,
			Children:      children,
		}
		return nil
	}

	if r.metricsRecorder != nil {
		err = r.metricsRecorder.RecordMetrics(ctx, "EntityIdHierarchy", queryFn)
	} else {
		err = queryFn()
	}

	if err != nil {
		return nil, wrapError(err, "EntityIdHierarchy")
	}
	return result, nil
}

// Converter helpers

func convertEntityStateToGraphQL(state *graph.EntityState) *Entity {
	if state == nil {
		return nil
	}

	var createdAt time.Time
	if ct, ok := state.GetPropertyValue("created_at"); ok {
		if ctTime, ok := ct.(time.Time); ok {
			createdAt = ctTime
		}
	}

	properties := make(map[string]interface{})
	for _, triple := range state.Triples {
		if !triple.IsRelationship() {
			properties[triple.Predicate] = triple.Object
		}
	}

	eid, _ := message.ParseEntityID(state.ID)

	return &Entity{
		ID:         state.ID,
		Type:       eid.Type,
		Properties: properties,
		CreatedAt:  createdAt,
		UpdatedAt:  state.UpdatedAt,
	}
}

func convertEntityStatesToGraphQL(states []*graph.EntityState) []*Entity {
	entities := make([]*Entity, 0, len(states))
	for _, state := range states {
		if entity := convertEntityStateToGraphQL(state); entity != nil {
			entities = append(entities, entity)
		}
	}
	return entities
}

func convertCommunityToGraphQL(comm *clustering.Community) *Community {
	if comm == nil {
		return nil
	}

	summary := comm.LLMSummary
	if summary == "" {
		summary = comm.StatisticalSummary
	}

	return &Community{
		ID:            comm.ID,
		Level:         comm.Level,
		Members:       comm.Members,
		Summary:       summary,
		Keywords:      comm.Keywords,
		RepEntities:   comm.RepEntities,
		SummaryStatus: comm.SummaryStatus,
	}
}

func convertPathResultToGraphQL(qmResult *querymanager.QueryResult) *PathSearchResult {
	if qmResult == nil {
		return nil
	}

	entities := make([]*PathEntity, len(qmResult.Entities))
	for i, e := range qmResult.Entities {
		eid, _ := message.ParseEntityID(e.ID)

		properties := make(map[string]interface{})
		for _, triple := range e.Triples {
			if !triple.IsRelationship() {
				properties[triple.Predicate] = triple.Object
			}
		}

		entities[i] = &PathEntity{
			ID:         e.ID,
			Type:       eid.Type,
			Properties: properties,
		}
	}

	// Use pre-calculated decay scores from PathRAG traversal
	scores := qmResult.Scores
	if scores == nil {
		scores = make(map[string]float64)
	}

	for _, pe := range entities {
		if score, ok := scores[pe.ID]; ok {
			pe.Score = score
		}
	}

	// Sort entities by score descending with ID as stable tiebreaker
	// Using SliceStable ensures deterministic ordering for equal scores
	sort.SliceStable(entities, func(i, j int) bool {
		if entities[i].Score != entities[j].Score {
			return entities[i].Score > entities[j].Score
		}
		return entities[i].ID < entities[j].ID // Deterministic tiebreaker
	})

	paths := make([][]PathStep, len(qmResult.Paths))
	for i, p := range qmResult.Paths {
		steps := make([]PathStep, len(p.Edges))
		for j, edge := range p.Edges {
			steps[j] = PathStep{
				From:      edge.From,
				To:        edge.To,
				Predicate: edge.EdgeType,
			}
		}
		paths[i] = steps
	}

	return &PathSearchResult{
		Entities:  entities,
		Paths:     paths,
		Truncated: qmResult.Truncated,
	}
}

func convertSnapshotToGraphQL(qmSnapshot *querymanager.GraphSnapshot) *GraphSnapshot {
	if qmSnapshot == nil {
		return nil
	}

	entities := convertEntityStatesToGraphQL(qmSnapshot.Entities)

	relationships := make([]SnapshotRelationship, len(qmSnapshot.Relationships))
	for i, rel := range qmSnapshot.Relationships {
		relationships[i] = SnapshotRelationship{
			FromEntityID: rel.FromEntityID,
			ToEntityID:   rel.ToEntityID,
			EdgeType:     rel.EdgeType,
		}
	}

	return &GraphSnapshot{
		Entities:      entities,
		Relationships: relationships,
		Count:         qmSnapshot.Count,
		Truncated:     qmSnapshot.Truncated,
		Timestamp:     qmSnapshot.Timestamp,
	}
}

// Property extraction helpers

func getProperty(entity *Entity, path string) interface{} {
	if entity == nil || entity.Properties == nil {
		return nil
	}

	parts := strings.Split(path, ".")
	var current interface{} = entity.Properties

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			val, exists := m[part]
			if !exists {
				return nil
			}
			current = val
		} else {
			return nil
		}
	}

	return current
}

// GetStringProp extracts a string property from entity.
func GetStringProp(entity *Entity, path string) string {
	val := getProperty(entity, path)
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

// GetIntProp extracts an int property from entity.
func GetIntProp(entity *Entity, path string) int {
	val := getProperty(entity, path)
	if val == nil {
		return 0
	}

	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 0
}

// GetFloatProp extracts a float64 property from entity.
func GetFloatProp(entity *Entity, path string) float64 {
	val := getProperty(entity, path)
	if val == nil {
		return 0.0
	}

	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0.0
}

// GetBoolProp extracts a bool property from entity.
func GetBoolProp(entity *Entity, path string) bool {
	val := getProperty(entity, path)
	if val == nil {
		return false
	}

	if b, ok := val.(bool); ok {
		return b
	}

	if s, ok := val.(string); ok {
		switch strings.ToLower(s) {
		case "true", "yes", "1", "t", "y":
			return true
		}
	}
	return false
}

// GetStringArrayProp extracts a string array property from entity.
func GetStringArrayProp(entity *Entity, path string) []string {
	val := getProperty(entity, path)
	if val == nil {
		return []string{}
	}

	if arr, ok := val.([]string); ok {
		return arr
	}

	if arr, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

// HasProperty checks if an entity has a property at the given path.
func HasProperty(entity *Entity, path string) bool {
	return getProperty(entity, path) != nil
}

// GetArrayProp extracts an array property from an entity.
func GetArrayProp(entity *Entity, path string) []interface{} {
	value := getProperty(entity, path)
	if value == nil {
		return []interface{}{}
	}
	if arr, ok := value.([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// GetMapProp extracts a map property from an entity.
func GetMapProp(entity *Entity, path string) map[string]interface{} {
	value := getProperty(entity, path)
	if value == nil {
		return map[string]interface{}{}
	}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

// GetPropertyOrDefault returns the property value or a default if not found.
func GetPropertyOrDefault(entity *Entity, path string, defaultValue interface{}) interface{} {
	value := getProperty(entity, path)
	if value == nil {
		return defaultValue
	}
	return value
}
