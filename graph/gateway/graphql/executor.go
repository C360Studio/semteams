package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// GraphQL schema for the SemStreams gateway.
const schemaSDL = `
type Query {
  """Get a single entity by ID"""
  entity(id: ID!): Entity

  """Get a single entity by alias or ID (supports alias resolution)"""
  entityByAlias(aliasOrID: String!): Entity

  """Get multiple entities by their IDs (batch operation)"""
  entities(ids: [ID!]!): [Entity!]!

  """Get all entities of a specific type"""
  entitiesByType(type: String!, limit: Int): [Entity!]!

  """Query relationships for an entity"""
  relationships(
    entityId: ID!
    direction: RelationshipDirection = OUTGOING
    edgeTypes: [String!]
    limit: Int
  ): [Relationship!]!

  """Local search within an entity's community (GraphRAG)"""
  localSearch(entityId: ID!, query: String!, level: Int): LocalSearchResult

  """Global search across community summaries (GraphRAG)"""
  globalSearch(query: String!, level: Int, maxCommunities: Int): GlobalSearchResult

  """
  Similarity search using embeddings (BM25 or neural depending on tier config).
  Returns entities ranked by cosine similarity score.
  Works on both statistical (BM25) and semantic (neural) tiers.
  """
  similaritySearch(query: String!, limit: Int): [Entity!]!

  """Get a community by ID"""
  community(id: ID!): Community

  """Get the community containing an entity at a specific level"""
  entityCommunity(entityId: ID!, level: Int!): Community

  """Get all communities at a specific hierarchical level"""
  communitiesByLevel(level: Int!): [Community!]!

  """Perform bounded graph traversal from a starting entity (PathRAG)"""
  pathSearch(
    startEntity: ID!
    maxDepth: Int
    maxNodes: Int
    direction: RelationshipDirection
    edgeTypes: [String!]
    decayFactor: Float
    """Include inferred sibling relationships based on EntityID hierarchy (same type prefix)"""
    includeSiblings: Boolean
  ): PathSearchResult

  """Find entities within geographic bounds"""
  spatialSearch(
    north: Float!
    south: Float!
    east: Float!
    west: Float!
    limit: Int
  ): [Entity!]!

  """Find entities within a time range"""
  temporalSearch(
    startTime: DateTime!
    endTime: DateTime!
    limit: Int
  ): [Entity!]!

  """Get a bounded graph snapshot (spatial/temporal/type filtered)"""
  graphSnapshot(
    north: Float
    south: Float
    east: Float
    west: Float
    startTime: DateTime
    endTime: DateTime
    entityTypes: [String!]
    maxEntities: Int
  ): GraphSnapshot

  """
  List entities matching an EntityID prefix (hierarchy navigation).
  Prefix examples: 'c360', 'c360.logistics', 'c360.logistics.environmental.sensor.temperature'
  Returns entity IDs only for efficiency; use entities(ids:) to fetch full state.
  """
  entitiesByPrefix(
    """The EntityID prefix to match (e.g., 'c360.logistics')"""
    prefix: String!
    """Maximum number of entity IDs to return"""
    limit: Int
  ): PrefixQueryResult!

  """
  Get EntityID hierarchy statistics (counts at each level).
  Useful for understanding graph structure and navigating the implicit hierarchy.
  """
  entityIdHierarchy(
    """The prefix to get hierarchy stats for (empty or omitted = root level)"""
    prefix: String
  ): HierarchyStats!
}

"""Direction for relationship queries"""
enum RelationshipDirection {
  OUTGOING
  INCOMING
  BOTH
}

"""A generic entity in the semantic graph"""
type Entity {
  id: ID!
  type: String!
  properties: JSON
  createdAt: DateTime
  updatedAt: DateTime
  """Similarity score for search results (0.0-1.0, higher is more relevant)"""
  score: Float
}

"""A relationship between two entities"""
type Relationship {
  fromEntityId: ID!
  toEntityId: ID!
  edgeType: String!
  properties: JSON
  createdAt: DateTime
}

"""Result from local community search"""
type LocalSearchResult {
  entities: [Entity!]!
  communityId: String!
  count: Int!
}

"""Result from global search across communities"""
type GlobalSearchResult {
  entities: [Entity!]!
  communitySummaries: [CommunitySummary!]!
  count: Int!
}

"""Community summary with relevance score"""
type CommunitySummary {
  communityId: String!
  summary: String!
  keywords: [String!]!
  level: Int!
  relevance: Float!
}

"""A community/cluster in the graph hierarchy"""
type Community {
  id: ID!
  level: Int!
  members: [String!]!
  summary: String
  keywords: [String!]
  repEntities: [String!]
  summaryStatus: String
}

"""Result from path traversal search (PathRAG)"""
type PathSearchResult {
  entities: [PathEntity!]!
  paths: [[PathStep!]!]!
  truncated: Boolean!
}

"""An entity discovered during path traversal"""
type PathEntity {
  id: ID!
  type: String!
  score: Float!
  properties: JSON
}

"""A single edge in a traversal path"""
type PathStep {
  from: ID!
  to: ID!
  predicate: String!
}

"""A bounded graph snapshot"""
type GraphSnapshot {
  entities: [Entity!]!
  relationships: [SnapshotRelationship!]!
  count: Int!
  truncated: Boolean!
  timestamp: DateTime!
}

"""A relationship in a graph snapshot"""
type SnapshotRelationship {
  fromEntityId: ID!
  toEntityId: ID!
  edgeType: String!
}

"""Result from prefix-based entity query"""
type PrefixQueryResult {
  """Entity IDs matching the prefix"""
  entityIds: [ID!]!

  """Total count (may exceed returned IDs if limit applied)"""
  totalCount: Int!

  """Whether results were truncated by limit"""
  truncated: Boolean!

  """The prefix that was queried"""
  prefix: String!
}

"""EntityID hierarchy statistics"""
type HierarchyStats {
  """The prefix queried (empty string = root)"""
  prefix: String!

  """Total entities under this prefix"""
  totalEntities: Int!

  """Breakdown by next level (e.g., platforms under org)"""
  children: [HierarchyLevel!]!
}

"""A single level in the EntityID hierarchy"""
type HierarchyLevel {
  """The full prefix for this level"""
  prefix: String!

  """Human-readable name (last segment of prefix)"""
  name: String!

  """Entity count at or under this level"""
  count: Int!
}

"""JSON scalar for arbitrary property data"""
scalar JSON

"""DateTime scalar for timestamps"""
scalar DateTime
`

const defaultMaxQueryDepth = 10

// Executor provides in-process GraphQL execution against the Resolver.
type Executor struct {
	schema     *ast.Schema
	resolver   *Resolver
	logger     *slog.Logger
	maxDepth   int
	classifier QueryClassifier
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithMaxDepth sets the maximum query nesting depth.
func WithMaxDepth(depth int) ExecutorOption {
	return func(e *Executor) {
		if depth > 0 {
			e.maxDepth = depth
		}
	}
}

// WithClassifier sets the query classifier for NL query analysis.
func WithClassifier(c QueryClassifier) ExecutorOption {
	return func(e *Executor) {
		if c != nil {
			e.classifier = c
		}
	}
}

// NewExecutor creates a new GraphQL executor.
func NewExecutor(resolver *Resolver, logger *slog.Logger, opts ...ExecutorOption) (*Executor, error) {
	schema, err := gqlparser.LoadSchema(&ast.Source{
		Name:  "schema.graphql",
		Input: schemaSDL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	e := &Executor{
		schema:     schema,
		resolver:   resolver,
		logger:     logger,
		maxDepth:   defaultMaxQueryDepth,
		classifier: &KeywordClassifier{}, // Default to keyword-based classification
	}

	for _, opt := range opts {
		opt(e)
	}

	return e, nil
}

// Execute executes a GraphQL query and returns the result.
func (e *Executor) Execute(ctx context.Context, query string, variables map[string]any) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before execution: %w", err)
	}

	doc, parseErrs := gqlparser.LoadQuery(e.schema, query)
	if parseErrs != nil {
		return nil, fmt.Errorf("GraphQL parse error: %v", parseErrs)
	}

	if err := e.validateQueryDepth(doc); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled during parsing: %w", err)
	}

	result := make(map[string]any)

	for _, op := range doc.Operations {
		if op.Operation != ast.Query {
			return nil, fmt.Errorf("only Query operations are supported, got %s", op.Operation)
		}

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during execution: %w", err)
		}

		opResult, err := e.executeSelectionSet(ctx, op.SelectionSet, variables)
		if err != nil {
			return nil, err
		}

		for k, v := range opResult {
			result[k] = v
		}
	}

	return map[string]any{"data": result}, nil
}

func (e *Executor) validateQueryDepth(doc *ast.QueryDocument) error {
	for _, op := range doc.Operations {
		depth := e.calculateDepth(op.SelectionSet, 0)
		if depth > e.maxDepth {
			return fmt.Errorf("query depth %d exceeds maximum allowed depth of %d", depth, e.maxDepth)
		}
	}
	return nil
}

func (e *Executor) calculateDepth(selections ast.SelectionSet, current int) int {
	maxDepth := current
	for _, sel := range selections {
		if field, ok := sel.(*ast.Field); ok {
			if strings.HasPrefix(field.Name, "__") {
				continue
			}
			if len(field.SelectionSet) > 0 {
				depth := e.calculateDepth(field.SelectionSet, current+1)
				if depth > maxDepth {
					maxDepth = depth
				}
			}
		}
	}
	return maxDepth
}

func (e *Executor) executeSelectionSet(ctx context.Context, selections ast.SelectionSet, variables map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		switch s := sel.(type) {
		case *ast.Field:
			if strings.HasPrefix(s.Name, "__") {
				val, err := e.executeIntrospection(s)
				if err != nil {
					return nil, err
				}
				result[s.Alias] = val
				if s.Alias == "" {
					result[s.Name] = val
				}
				continue
			}

			val, err := e.executeField(ctx, s, variables)
			if err != nil {
				return nil, err
			}

			key := s.Name
			if s.Alias != "" {
				key = s.Alias
			}
			result[key] = val

		case *ast.FragmentSpread:
			return nil, fmt.Errorf("fragment spreads not yet supported")

		case *ast.InlineFragment:
			return nil, fmt.Errorf("inline fragments not yet supported")
		}
	}

	return result, nil
}

func (e *Executor) executeField(ctx context.Context, field *ast.Field, variables map[string]any) (any, error) {
	args := make(map[string]any)
	for _, arg := range field.Arguments {
		val, err := e.resolveValue(arg.Value, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve argument %s: %w", arg.Name, err)
		}
		args[arg.Name] = val
	}

	switch field.Name {
	case "entity":
		return e.resolveEntity(ctx, args, field.SelectionSet)
	case "entityByAlias":
		return e.resolveEntityByAlias(ctx, args, field.SelectionSet)
	case "entities":
		return e.resolveEntities(ctx, args, field.SelectionSet)
	case "entitiesByType":
		return e.resolveEntitiesByType(ctx, args, field.SelectionSet)
	case "relationships":
		return e.resolveRelationships(ctx, args, field.SelectionSet)
	case "localSearch":
		return e.resolveLocalSearch(ctx, args, field.SelectionSet)
	case "globalSearch":
		return e.resolveGlobalSearch(ctx, args, field.SelectionSet)
	case "similaritySearch":
		return e.resolveSimilaritySearch(ctx, args, field.SelectionSet)
	case "community":
		return e.resolveCommunity(ctx, args, field.SelectionSet)
	case "entityCommunity":
		return e.resolveEntityCommunity(ctx, args, field.SelectionSet)
	case "communitiesByLevel":
		return e.resolveCommunitiesByLevel(ctx, args, field.SelectionSet)
	case "pathSearch":
		return e.resolvePathSearch(ctx, args, field.SelectionSet)
	case "spatialSearch":
		return e.resolveSpatialSearch(ctx, args, field.SelectionSet)
	case "temporalSearch":
		return e.resolveTemporalSearch(ctx, args, field.SelectionSet)
	case "graphSnapshot":
		return e.resolveGraphSnapshot(ctx, args, field.SelectionSet)
	case "entitiesByPrefix":
		return e.resolveEntitiesByPrefix(ctx, args, field.SelectionSet)
	case "entityIdHierarchy":
		return e.resolveEntityIDHierarchy(ctx, args, field.SelectionSet)
	default:
		return nil, fmt.Errorf("unknown field: %s", field.Name)
	}
}

// Resolver method implementations

func (e *Executor) resolveEntity(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id argument required")
	}

	entity, err := e.resolver.QueryEntityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}

	return e.formatEntity(entity, selections)
}

func (e *Executor) resolveEntityByAlias(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	aliasOrID, ok := args["aliasOrID"].(string)
	if !ok {
		return nil, fmt.Errorf("aliasOrID argument required")
	}

	entity, err := e.resolver.QueryEntityByAlias(ctx, aliasOrID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}

	return e.formatEntity(entity, selections)
}

func (e *Executor) resolveEntities(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	idsRaw, ok := args["ids"].([]any)
	if !ok {
		return nil, fmt.Errorf("ids argument required")
	}

	ids := make([]string, len(idsRaw))
	for i, v := range idsRaw {
		ids[i] = fmt.Sprintf("%v", v)
	}

	entities, err := e.resolver.QueryEntitiesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	return e.formatEntities(entities, selections)
}

func (e *Executor) resolveEntitiesByType(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	typeName, ok := args["type"].(string)
	if !ok {
		return nil, fmt.Errorf("type argument required")
	}

	limit := 0
	if l, ok := args["limit"].(int); ok {
		limit = l
	} else if l, ok := args["limit"].(int64); ok {
		limit = int(l)
	}

	entities, err := e.resolver.QueryEntitiesByType(ctx, typeName, limit)
	if err != nil {
		return nil, err
	}

	return e.formatEntities(entities, selections)
}

func (e *Executor) resolveRelationships(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	entityID, ok := args["entityId"].(string)
	if !ok {
		return nil, fmt.Errorf("entityId argument required")
	}

	direction := "outgoing"
	if d, ok := args["direction"].(string); ok {
		direction = strings.ToLower(d)
	}

	var edgeTypes []string
	if et, ok := args["edgeTypes"].([]any); ok {
		edgeTypes = make([]string, len(et))
		for i, v := range et {
			edgeTypes[i] = fmt.Sprintf("%v", v)
		}
	}

	filters := RelationshipFilters{
		EntityID:  entityID,
		Direction: direction,
		EdgeTypes: edgeTypes,
	}

	if l, ok := args["limit"].(int); ok {
		filters.Limit = l
	}

	relationships, err := e.resolver.QueryRelationships(ctx, filters)
	if err != nil {
		return nil, err
	}

	return e.formatRelationships(relationships, selections)
}

func (e *Executor) resolveLocalSearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	entityID, ok := args["entityId"].(string)
	if !ok {
		return nil, fmt.Errorf("entityId argument required")
	}

	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query argument required")
	}

	level := 0
	if l, ok := args["level"].(int); ok {
		level = l
	} else if l, ok := args["level"].(int64); ok {
		level = int(l)
	}

	result, err := e.resolver.LocalSearch(ctx, entityID, query, level)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return e.formatLocalSearchResult(result, selections)
}

func (e *Executor) resolveGlobalSearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query argument required")
	}

	level := 0
	if l, ok := args["level"].(int); ok {
		level = l
	} else if l, ok := args["level"].(int64); ok {
		level = int(l)
	}

	maxCommunities := 5
	if mc, ok := args["maxCommunities"].(int); ok {
		maxCommunities = mc
	} else if mc, ok := args["maxCommunities"].(int64); ok {
		maxCommunities = int(mc)
	}

	// Classify NL query to extract temporal/spatial/intent information
	opts := e.classifier.ClassifyQuery(ctx, query)
	opts.Level = level
	opts.MaxCommunities = maxCommunities
	opts.SetDefaults()

	result, err := e.resolver.GlobalSearchWithOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return e.formatGlobalSearchResult(result, selections)
}

func (e *Executor) resolveSimilaritySearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query argument required")
	}

	limit := 10
	if l, ok := args["limit"].(int); ok {
		limit = l
	} else if l, ok := args["limit"].(int64); ok {
		limit = int(l)
	}

	entities, err := e.resolver.SimilaritySearch(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	// Format entities with scores
	results := make([]map[string]any, 0, len(entities))
	for _, entity := range entities {
		results = append(results, e.formatEntityWithScore(entity, selections))
	}

	return results, nil
}

func (e *Executor) resolveCommunity(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id argument required")
	}

	community, err := e.resolver.GetCommunity(ctx, id)
	if err != nil {
		return nil, err
	}
	if community == nil {
		return nil, nil
	}

	return e.formatCommunity(community, selections)
}

func (e *Executor) resolveEntityCommunity(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	entityID, ok := args["entityId"].(string)
	if !ok {
		return nil, fmt.Errorf("entityId argument required")
	}

	level := 0
	if l, ok := args["level"].(int); ok {
		level = l
	} else if l, ok := args["level"].(int64); ok {
		level = int(l)
	}

	community, err := e.resolver.GetEntityCommunity(ctx, entityID, level)
	if err != nil {
		return nil, err
	}
	if community == nil {
		return nil, nil
	}

	return e.formatCommunity(community, selections)
}

func (e *Executor) resolveCommunitiesByLevel(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	level := 0
	if l, ok := args["level"].(int); ok {
		level = l
	} else if l, ok := args["level"].(int64); ok {
		level = int(l)
	}

	communities, err := e.resolver.CommunitiesByLevel(ctx, level)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, len(communities))
	for i, c := range communities {
		formatted, err := e.formatCommunity(c, selections)
		if err != nil {
			return nil, err
		}
		result[i] = formatted
	}
	return result, nil
}

func (e *Executor) resolvePathSearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	startEntity, ok := args["startEntity"].(string)
	if !ok {
		return nil, fmt.Errorf("startEntity argument required")
	}

	maxDepth := 3
	maxNodes := 100
	direction := "OUTGOING"
	decayFactor := 0.8
	includeSiblings := false
	var edgeTypes []string

	if d, ok := args["maxDepth"].(int); ok {
		maxDepth = d
	} else if d, ok := args["maxDepth"].(int64); ok {
		maxDepth = int(d)
	}
	if n, ok := args["maxNodes"].(int); ok {
		maxNodes = n
	} else if n, ok := args["maxNodes"].(int64); ok {
		maxNodes = int(n)
	}
	if d, ok := args["direction"].(string); ok {
		direction = d
	}
	if f, ok := args["decayFactor"].(float64); ok {
		decayFactor = f
	}
	if s, ok := args["includeSiblings"].(bool); ok {
		includeSiblings = s
	}
	if et, ok := args["edgeTypes"].([]any); ok {
		edgeTypes = make([]string, len(et))
		for i, v := range et {
			edgeTypes[i] = fmt.Sprintf("%v", v)
		}
	}

	result, err := e.resolver.PathSearch(ctx, startEntity, maxDepth, maxNodes, direction, edgeTypes, decayFactor, includeSiblings)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return e.formatPathSearchResult(result, selections)
}

func (e *Executor) resolveSpatialSearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	north, ok := args["north"].(float64)
	if !ok {
		return nil, fmt.Errorf("north argument required")
	}
	south, ok := args["south"].(float64)
	if !ok {
		return nil, fmt.Errorf("south argument required")
	}
	east, ok := args["east"].(float64)
	if !ok {
		return nil, fmt.Errorf("east argument required")
	}
	west, ok := args["west"].(float64)
	if !ok {
		return nil, fmt.Errorf("west argument required")
	}

	limit := 100
	if l, ok := args["limit"].(int); ok {
		limit = l
	} else if l, ok := args["limit"].(int64); ok {
		limit = int(l)
	}

	entities, err := e.resolver.SpatialSearch(ctx, north, south, east, west, limit)
	if err != nil {
		return nil, err
	}

	return e.formatEntities(entities, selections)
}

func (e *Executor) resolveTemporalSearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	startTimeStr, ok := args["startTime"].(string)
	if !ok {
		return nil, fmt.Errorf("startTime argument required")
	}
	endTimeStr, ok := args["endTime"].(string)
	if !ok {
		return nil, fmt.Errorf("endTime argument required")
	}

	startTime, err := parseDateTime(startTimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid startTime: %w", err)
	}
	endTime, err := parseDateTime(endTimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid endTime: %w", err)
	}

	limit := 100
	if l, ok := args["limit"].(int); ok {
		limit = l
	} else if l, ok := args["limit"].(int64); ok {
		limit = int(l)
	}

	entities, err := e.resolver.TemporalSearch(ctx, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}

	return e.formatEntities(entities, selections)
}

func (e *Executor) resolveGraphSnapshot(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	var north, south, east, west *float64
	var startTime, endTime *time.Time
	var entityTypes []string
	maxEntities := 1000

	if n, ok := args["north"].(float64); ok {
		north = &n
	}
	if s, ok := args["south"].(float64); ok {
		south = &s
	}
	if eastVal, ok := args["east"].(float64); ok {
		east = &eastVal
	}
	if w, ok := args["west"].(float64); ok {
		west = &w
	}
	if st, ok := args["startTime"].(string); ok {
		t, err := parseDateTime(st)
		if err != nil {
			return nil, fmt.Errorf("invalid startTime: %w", err)
		}
		startTime = &t
	}
	if et, ok := args["endTime"].(string); ok {
		t, err := parseDateTime(et)
		if err != nil {
			return nil, fmt.Errorf("invalid endTime: %w", err)
		}
		endTime = &t
	}
	if types, ok := args["entityTypes"].([]any); ok {
		entityTypes = make([]string, len(types))
		for i, v := range types {
			entityTypes[i] = fmt.Sprintf("%v", v)
		}
	}
	if m, ok := args["maxEntities"].(int); ok {
		maxEntities = m
	} else if m, ok := args["maxEntities"].(int64); ok {
		maxEntities = int(m)
	}

	result, err := e.resolver.GraphSnapshot(ctx, north, south, east, west, startTime, endTime, entityTypes, maxEntities)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return e.formatGraphSnapshot(result, selections)
}

func (e *Executor) resolveEntitiesByPrefix(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	prefix, ok := args["prefix"].(string)
	if !ok {
		return nil, fmt.Errorf("prefix argument required")
	}

	limit := 0
	if l, ok := args["limit"].(int); ok {
		limit = l
	} else if l, ok := args["limit"].(int64); ok {
		limit = int(l)
	}

	result, err := e.resolver.EntitiesByPrefix(ctx, prefix, limit)
	if err != nil {
		return nil, err
	}

	return e.formatPrefixQueryResult(result, selections)
}

func (e *Executor) resolveEntityIDHierarchy(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
	prefix := ""
	if p, ok := args["prefix"].(string); ok {
		prefix = p
	}

	result, err := e.resolver.EntityIDHierarchy(ctx, prefix)
	if err != nil {
		return nil, err
	}

	return e.formatHierarchyStats(result, selections)
}

// Formatting helpers

func (e *Executor) formatEntity(entity *Entity, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "id":
			result[key] = entity.ID
		case "type":
			result[key] = entity.Type
		case "properties":
			result[key] = entity.Properties
		case "createdAt":
			if !entity.CreatedAt.IsZero() {
				result[key] = entity.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
		case "updatedAt":
			if !entity.UpdatedAt.IsZero() {
				result[key] = entity.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
		}
	}

	return result, nil
}

func (e *Executor) formatEntities(entities []*Entity, selections ast.SelectionSet) ([]map[string]any, error) {
	result := make([]map[string]any, len(entities))
	for i, entity := range entities {
		formatted, err := e.formatEntity(entity, selections)
		if err != nil {
			return nil, err
		}
		result[i] = formatted
	}
	return result, nil
}

// formatEntityWithScore formats an entity including its score field.
// Used by similaritySearch to include similarity scores in results.
func (e *Executor) formatEntityWithScore(entity *Entity, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "id":
			result[key] = entity.ID
		case "type":
			result[key] = entity.Type
		case "properties":
			result[key] = entity.Properties
		case "createdAt":
			if !entity.CreatedAt.IsZero() {
				result[key] = entity.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
		case "updatedAt":
			if !entity.UpdatedAt.IsZero() {
				result[key] = entity.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
		case "score":
			result[key] = entity.Score
		}
	}

	return result
}

func (e *Executor) formatRelationship(rel *Relationship, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "fromEntityId":
			result[key] = rel.FromEntityID
		case "toEntityId":
			result[key] = rel.ToEntityID
		case "edgeType":
			result[key] = rel.EdgeType
		case "properties":
			result[key] = rel.Properties
		case "createdAt":
			if !rel.CreatedAt.IsZero() {
				result[key] = rel.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
		}
	}

	return result, nil
}

func (e *Executor) formatRelationships(rels []*Relationship, selections ast.SelectionSet) ([]map[string]any, error) {
	result := make([]map[string]any, len(rels))
	for i, rel := range rels {
		formatted, err := e.formatRelationship(rel, selections)
		if err != nil {
			return nil, err
		}
		result[i] = formatted
	}
	return result, nil
}

func (e *Executor) formatLocalSearchResult(r *LocalSearchResult, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "entities":
			formatted, err := e.formatEntities(r.Entities, field.SelectionSet)
			if err != nil {
				return nil, err
			}
			result[key] = formatted
		case "communityId":
			result[key] = r.CommunityID
		case "count":
			result[key] = r.Count
		}
	}

	return result, nil
}

func (e *Executor) formatGlobalSearchResult(r *GlobalSearchResult, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "entities":
			formatted, err := e.formatEntities(r.Entities, field.SelectionSet)
			if err != nil {
				return nil, err
			}
			result[key] = formatted
		case "communitySummaries":
			summaries := make([]map[string]any, len(r.CommunitySummaries))
			for i, s := range r.CommunitySummaries {
				summaries[i] = e.formatCommunitySummary(&s, field.SelectionSet)
			}
			result[key] = summaries
		case "count":
			result[key] = r.Count
		}
	}

	return result, nil
}

func (e *Executor) formatCommunitySummary(s *CommunitySummary, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "communityId":
			result[key] = s.CommunityID
		case "summary":
			result[key] = s.Summary
		case "keywords":
			result[key] = s.Keywords
		case "level":
			result[key] = s.Level
		case "relevance":
			result[key] = s.Relevance
		}
	}

	return result
}

func (e *Executor) formatCommunity(c *Community, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "id":
			result[key] = c.ID
		case "level":
			result[key] = c.Level
		case "members":
			result[key] = c.Members
		case "summary":
			result[key] = c.Summary
		case "keywords":
			result[key] = c.Keywords
		case "repEntities":
			result[key] = c.RepEntities
		case "summaryStatus":
			result[key] = c.SummaryStatus
		}
	}

	return result, nil
}

func (e *Executor) formatPathSearchResult(r *PathSearchResult, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "entities":
			entities := make([]map[string]any, len(r.Entities))
			for i, pe := range r.Entities {
				entities[i] = e.formatPathEntity(pe, field.SelectionSet)
			}
			result[key] = entities
		case "paths":
			paths := make([][]map[string]any, len(r.Paths))
			for i, path := range r.Paths {
				steps := make([]map[string]any, len(path))
				for j, step := range path {
					steps[j] = e.formatPathStep(&step, field.SelectionSet)
				}
				paths[i] = steps
			}
			result[key] = paths
		case "truncated":
			result[key] = r.Truncated
		}
	}

	return result, nil
}

func (e *Executor) formatPathEntity(pe *PathEntity, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "id":
			result[key] = pe.ID
		case "type":
			result[key] = pe.Type
		case "score":
			result[key] = pe.Score
		case "properties":
			result[key] = pe.Properties
		}
	}

	return result
}

func (e *Executor) formatPathStep(step *PathStep, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "from":
			result[key] = step.From
		case "to":
			result[key] = step.To
		case "predicate":
			result[key] = step.Predicate
		}
	}

	return result
}

func (e *Executor) formatGraphSnapshot(s *GraphSnapshot, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "entities":
			formatted, err := e.formatEntities(s.Entities, field.SelectionSet)
			if err != nil {
				return nil, err
			}
			result[key] = formatted
		case "relationships":
			rels := make([]map[string]any, len(s.Relationships))
			for i, r := range s.Relationships {
				rels[i] = e.formatSnapshotRelationship(&r, field.SelectionSet)
			}
			result[key] = rels
		case "count":
			result[key] = s.Count
		case "truncated":
			result[key] = s.Truncated
		case "timestamp":
			result[key] = s.Timestamp.Format("2006-01-02T15:04:05Z07:00")
		}
	}

	return result, nil
}

func (e *Executor) formatSnapshotRelationship(r *SnapshotRelationship, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "fromEntityId":
			result[key] = r.FromEntityID
		case "toEntityId":
			result[key] = r.ToEntityID
		case "edgeType":
			result[key] = r.EdgeType
		}
	}

	return result
}

func (e *Executor) formatPrefixQueryResult(r *PrefixQueryResult, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "entityIds":
			result[key] = r.EntityIDs
		case "totalCount":
			result[key] = r.TotalCount
		case "truncated":
			result[key] = r.Truncated
		case "prefix":
			result[key] = r.Prefix
		}
	}

	return result, nil
}

func (e *Executor) formatHierarchyStats(s *HierarchyStats, selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "prefix":
			result[key] = s.Prefix
		case "totalEntities":
			result[key] = s.TotalEntities
		case "children":
			children := make([]map[string]any, len(s.Children))
			for i, c := range s.Children {
				children[i] = e.formatHierarchyLevel(&c, field.SelectionSet)
			}
			result[key] = children
		}
	}

	return result, nil
}

func (e *Executor) formatHierarchyLevel(l *HierarchyLevel, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "prefix":
			result[key] = l.Prefix
		case "name":
			result[key] = l.Name
		case "count":
			result[key] = l.Count
		}
	}

	return result
}

func parseDateTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", s)
}

func (e *Executor) resolveValue(value *ast.Value, variables map[string]any) (any, error) {
	switch value.Kind {
	case ast.Variable:
		if v, ok := variables[value.Raw]; ok {
			return v, nil
		}
		return nil, nil

	case ast.IntValue:
		var i int64
		if err := json.Unmarshal([]byte(value.Raw), &i); err != nil {
			return nil, err
		}
		return int(i), nil

	case ast.FloatValue:
		var f float64
		if err := json.Unmarshal([]byte(value.Raw), &f); err != nil {
			return nil, err
		}
		return f, nil

	case ast.StringValue, ast.BlockValue:
		return value.Raw, nil

	case ast.BooleanValue:
		return value.Raw == "true", nil

	case ast.NullValue:
		return nil, nil

	case ast.EnumValue:
		return value.Raw, nil

	case ast.ListValue:
		list := make([]any, len(value.Children))
		for i, child := range value.Children {
			v, err := e.resolveValue(child.Value, variables)
			if err != nil {
				return nil, err
			}
			list[i] = v
		}
		return list, nil

	case ast.ObjectValue:
		obj := make(map[string]any)
		for _, child := range value.Children {
			v, err := e.resolveValue(child.Value, variables)
			if err != nil {
				return nil, err
			}
			obj[child.Name] = v
		}
		return obj, nil
	}

	return value.Raw, nil
}

func (e *Executor) executeIntrospection(field *ast.Field) (any, error) {
	switch field.Name {
	case "__schema":
		return e.introspectSchema(field.SelectionSet)
	case "__type":
		for _, arg := range field.Arguments {
			if arg.Name == "name" {
				typeName := arg.Value.Raw
				return e.introspectType(typeName, field.SelectionSet)
			}
		}
		return nil, fmt.Errorf("__type requires name argument")
	case "__typename":
		return "Query", nil
	default:
		return nil, fmt.Errorf("unknown introspection field: %s", field.Name)
	}
}

func (e *Executor) introspectSchema(selections ast.SelectionSet) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "types":
			types := make([]map[string]any, 0)
			for _, t := range e.schema.Types {
				typeInfo, _ := e.introspectType(t.Name, field.SelectionSet)
				if typeInfo != nil {
					types = append(types, typeInfo)
				}
			}
			result[key] = types

		case "queryType":
			result[key] = map[string]any{"name": "Query"}

		case "mutationType":
			result[key] = nil

		case "subscriptionType":
			result[key] = nil

		case "directives":
			result[key] = []map[string]any{}
		}
	}

	return result, nil
}

func (e *Executor) introspectType(typeName string, selections ast.SelectionSet) (map[string]any, error) {
	t, ok := e.schema.Types[typeName]
	if !ok {
		return nil, nil
	}

	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "name":
			result[key] = t.Name
		case "kind":
			result[key] = string(t.Kind)
		case "description":
			result[key] = t.Description
		case "fields":
			if t.Kind == ast.Object || t.Kind == ast.Interface {
				fields := make([]map[string]any, 0)
				for _, f := range t.Fields {
					fields = append(fields, e.introspectField(f, field.SelectionSet))
				}
				result[key] = fields
			}
		case "enumValues":
			if t.Kind == ast.Enum {
				values := make([]map[string]any, len(t.EnumValues))
				for i, v := range t.EnumValues {
					values[i] = map[string]any{
						"name":              v.Name,
						"description":       v.Description,
						"isDeprecated":      false,
						"deprecationReason": nil,
					}
				}
				result[key] = values
			}
		case "inputFields":
			result[key] = nil
		case "interfaces":
			result[key] = []map[string]any{}
		case "possibleTypes":
			result[key] = nil
		case "ofType":
			result[key] = nil
		}
	}

	return result, nil
}

func (e *Executor) introspectField(f *ast.FieldDefinition, selections ast.SelectionSet) map[string]any {
	result := make(map[string]any)

	for _, sel := range selections {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}

		key := field.Name
		if field.Alias != "" {
			key = field.Alias
		}

		switch field.Name {
		case "name":
			result[key] = f.Name
		case "description":
			result[key] = f.Description
		case "args":
			args := make([]map[string]any, len(f.Arguments))
			for i, arg := range f.Arguments {
				args[i] = map[string]any{
					"name":         arg.Name,
					"description":  arg.Description,
					"type":         e.introspectTypeRef(arg.Type),
					"defaultValue": nil,
				}
			}
			result[key] = args
		case "type":
			result[key] = e.introspectTypeRef(f.Type)
		case "isDeprecated":
			result[key] = false
		case "deprecationReason":
			result[key] = nil
		}
	}

	return result
}

func (e *Executor) introspectTypeRef(t *ast.Type) map[string]any {
	if t == nil {
		return nil
	}

	if t.NonNull {
		return map[string]any{
			"kind":   "NON_NULL",
			"name":   nil,
			"ofType": e.introspectTypeRef(&ast.Type{NamedType: t.NamedType, Elem: t.Elem}),
		}
	}

	if t.Elem != nil {
		return map[string]any{
			"kind":   "LIST",
			"name":   nil,
			"ofType": e.introspectTypeRef(t.Elem),
		}
	}

	return map[string]any{
		"kind":   e.getTypeKind(t.NamedType),
		"name":   t.NamedType,
		"ofType": nil,
	}
}

func (e *Executor) getTypeKind(typeName string) string {
	if t, ok := e.schema.Types[typeName]; ok {
		return string(t.Kind)
	}
	return "SCALAR"
}

// GetSchema returns the GraphQL schema SDL for documentation.
func (e *Executor) GetSchema() string {
	return schemaSDL
}
