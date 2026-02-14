package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/pkg/types"
)

// GraphQueryResult represents the result of a batch graph query
type GraphQueryResult struct {
	Entities      map[string]json.RawMessage `json:"entities"`
	Relationships []types.Relationship       `json:"relationships,omitempty"`
	TokenCount    int                        `json:"token_count"`
}

// GraphQuerier defines the interface for graph query operations
type GraphQuerier interface {
	QueryEntities(ctx context.Context, entityIDs []string) (map[string]json.RawMessage, error)
	QueryRelationships(ctx context.Context, entityID string, depth int) ([]types.Relationship, error)
}

// GraphQueryAction performs batch graph queries
type GraphQueryAction struct {
	Entities      []string // Entity IDs to query
	Relationships bool     // Include relationships
	Depth         int      // Traversal depth (0 = just the entities, 1+ = include neighbors)
	Include       []string // What to include: properties, triples, neighbors

	// Injected at execution time
	GraphQuerier GraphQuerier
}

// NewGraphQueryAction creates a new graph query action
func NewGraphQueryAction(entities []string, relationships bool, depth int, include []string) *GraphQueryAction {
	return &GraphQueryAction{
		Entities:      entities,
		Relationships: relationships,
		Depth:         depth,
		Include:       include,
	}
}

// Execute performs the batch graph query
func (a *GraphQueryAction) Execute(ctx context.Context, _ *Context) Result {
	start := time.Now()

	if len(a.Entities) == 0 {
		return Result{
			Success:  false,
			Error:    "graph_query action requires at least one entity",
			Duration: time.Since(start),
		}
	}

	if a.GraphQuerier == nil {
		return Result{
			Success:  false,
			Error:    "graph querier not available",
			Duration: time.Since(start),
		}
	}

	result := GraphQueryResult{
		Entities: make(map[string]json.RawMessage),
	}

	// Query all entities in batch
	entities, err := a.GraphQuerier.QueryEntities(ctx, a.Entities)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("failed to query entities: %v", err),
			Duration: time.Since(start),
		}
	}
	result.Entities = entities

	// Query relationships if requested
	if a.Relationships && a.Depth > 0 {
		for _, entityID := range a.Entities {
			rels, err := a.GraphQuerier.QueryRelationships(ctx, entityID, a.Depth)
			if err != nil {
				// Log but don't fail the whole query for relationship errors
				slog.Warn("failed to query relationships",
					"entity_id", entityID,
					"depth", a.Depth,
					"error", err,
				)
				continue
			}
			result.Relationships = append(result.Relationships, rels...)
		}
	}

	// Estimate token count from JSON size (rough: 4 chars per token)
	outputJSON, _ := json.Marshal(result)
	result.TokenCount = len(outputJSON) / 4

	output, err := json.Marshal(result)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("failed to marshal result: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Output:   output,
		Duration: time.Since(start),
	}
}
