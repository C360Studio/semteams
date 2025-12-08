package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	gql "github.com/c360/semstreams/gateway/graphql"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// GraphQL schema for the MCP gateway.
// This schema exposes the BaseResolver methods to AI agents.
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

  """Perform semantic similarity search"""
  semanticSearch(query: String!, limit: Int): [SemanticSearchResult!]!

  """Local search within an entity's community (GraphRAG)"""
  localSearch(entityId: ID!, query: String!, level: Int): LocalSearchResult

  """Global search across community summaries (GraphRAG)"""
  globalSearch(query: String!, level: Int, maxCommunities: Int): GlobalSearchResult

  """Get a community by ID"""
  community(id: ID!): Community

  """Get the community containing an entity at a specific level"""
  entityCommunity(entityId: ID!, level: Int!): Community
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
}

"""A relationship between two entities"""
type Relationship {
  fromEntityId: ID!
  toEntityId: ID!
  edgeType: String!
  properties: JSON
  createdAt: DateTime
}

"""Result from semantic search"""
type SemanticSearchResult {
  entity: Entity!
  score: Float!
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

"""JSON scalar for arbitrary property data"""
scalar JSON

"""DateTime scalar for timestamps"""
scalar DateTime
`

// Executor provides in-process GraphQL execution against the BaseResolver.
type Executor struct {
	schema   *ast.Schema
	resolver *gql.BaseResolver
	logger   *slog.Logger
}

// NewExecutor creates a new GraphQL executor.
func NewExecutor(resolver *gql.BaseResolver, logger *slog.Logger) (*Executor, error) {
	// Parse schema
	schema, err := gqlparser.LoadSchema(&ast.Source{
		Name:  "schema.graphql",
		Input: schemaSDL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	return &Executor{
		schema:   schema,
		resolver: resolver,
		logger:   logger,
	}, nil
}

// Execute executes a GraphQL query and returns the result.
func (e *Executor) Execute(ctx context.Context, query string, variables map[string]any) (any, error) {
	// Parse query
	doc, parseErrs := gqlparser.LoadQuery(e.schema, query)
	if parseErrs != nil {
		return nil, fmt.Errorf("GraphQL parse error: %v", parseErrs)
	}

	// Execute operations
	result := make(map[string]any)

	for _, op := range doc.Operations {
		if op.Operation != ast.Query {
			return nil, fmt.Errorf("only Query operations are supported, got %s", op.Operation)
		}

		// Execute each field in the selection set
		opResult, err := e.executeSelectionSet(ctx, op.SelectionSet, variables)
		if err != nil {
			return nil, err
		}

		// Merge results
		for k, v := range opResult {
			result[k] = v
		}
	}

	return map[string]any{"data": result}, nil
}

// executeSelectionSet executes a selection set and returns the results.
func (e *Executor) executeSelectionSet(ctx context.Context, selections ast.SelectionSet, variables map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for _, sel := range selections {
		switch s := sel.(type) {
		case *ast.Field:
			// Handle introspection fields
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

			// Execute resolver method
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
			// TODO: Handle fragment spreads if needed
			return nil, fmt.Errorf("fragment spreads not yet supported")

		case *ast.InlineFragment:
			// TODO: Handle inline fragments if needed
			return nil, fmt.Errorf("inline fragments not yet supported")
		}
	}

	return result, nil
}

// executeField executes a single field and returns its value.
func (e *Executor) executeField(ctx context.Context, field *ast.Field, variables map[string]any) (any, error) {
	// Get argument values
	args := make(map[string]any)
	for _, arg := range field.Arguments {
		val, err := e.resolveValue(arg.Value, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve argument %s: %w", arg.Name, err)
		}
		args[arg.Name] = val
	}

	// Dispatch to resolver method
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

	case "semanticSearch":
		return e.resolveSemanticSearch(ctx, args, field.SelectionSet)

	case "localSearch":
		return e.resolveLocalSearch(ctx, args, field.SelectionSet)

	case "globalSearch":
		return e.resolveGlobalSearch(ctx, args, field.SelectionSet)

	case "community":
		return e.resolveCommunity(ctx, args, field.SelectionSet)

	case "entityCommunity":
		return e.resolveEntityCommunity(ctx, args, field.SelectionSet)

	default:
		return nil, fmt.Errorf("unknown field: %s", field.Name)
	}
}

// Resolver methods that delegate to BaseResolver

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

	filters := gql.RelationshipFilters{
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

func (e *Executor) resolveSemanticSearch(ctx context.Context, args map[string]any, selections ast.SelectionSet) (any, error) {
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

	results, err := e.resolver.SemanticSearch(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	return e.formatSemanticSearchResults(results, selections)
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

	result, err := e.resolver.GlobalSearch(ctx, query, level, maxCommunities)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return e.formatGlobalSearchResult(result, selections)
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

// Formatting helpers - convert internal types to GraphQL-compatible maps

func (e *Executor) formatEntity(entity *gql.Entity, selections ast.SelectionSet) (map[string]any, error) {
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

func (e *Executor) formatEntities(entities []*gql.Entity, selections ast.SelectionSet) ([]map[string]any, error) {
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

func (e *Executor) formatRelationship(rel *gql.Relationship, selections ast.SelectionSet) (map[string]any, error) {
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

func (e *Executor) formatRelationships(rels []*gql.Relationship, selections ast.SelectionSet) ([]map[string]any, error) {
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

func (e *Executor) formatSemanticSearchResult(r *gql.SemanticSearchResult, selections ast.SelectionSet) (map[string]any, error) {
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
		case "entity":
			if r.Entity != nil {
				formatted, err := e.formatEntity(r.Entity, field.SelectionSet)
				if err != nil {
					return nil, err
				}
				result[key] = formatted
			}
		case "score":
			result[key] = r.Score
		}
	}

	return result, nil
}

func (e *Executor) formatSemanticSearchResults(results []*gql.SemanticSearchResult, selections ast.SelectionSet) ([]map[string]any, error) {
	formatted := make([]map[string]any, len(results))
	for i, r := range results {
		f, err := e.formatSemanticSearchResult(r, selections)
		if err != nil {
			return nil, err
		}
		formatted[i] = f
	}
	return formatted, nil
}

func (e *Executor) formatLocalSearchResult(r *gql.LocalSearchResult, selections ast.SelectionSet) (map[string]any, error) {
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

func (e *Executor) formatGlobalSearchResult(r *gql.GlobalSearchResult, selections ast.SelectionSet) (map[string]any, error) {
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

func (e *Executor) formatCommunitySummary(s *gql.CommunitySummary, selections ast.SelectionSet) map[string]any {
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

func (e *Executor) formatCommunity(c *gql.Community, selections ast.SelectionSet) (map[string]any, error) {
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

// resolveValue resolves a GraphQL value, handling variables.
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

// executeIntrospection handles GraphQL introspection queries.
func (e *Executor) executeIntrospection(field *ast.Field) (any, error) {
	switch field.Name {
	case "__schema":
		return e.introspectSchema(field.SelectionSet)
	case "__type":
		// Get type name from arguments
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
