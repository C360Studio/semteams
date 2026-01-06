package clustering

import (
	"context"

	"github.com/c360/semstreams/pkg/errs"
)

// Direction represents the direction of relationship traversal.
// Local copy to avoid import cycle with querymanager.
type Direction string

// Direction constants for relationship traversal.
const (
	DirectionOutgoing Direction = "outgoing"
	DirectionIncoming Direction = "incoming"
	DirectionBoth     Direction = "both"
)

// Relationship represents an edge between entities.
// Local copy to avoid import cycle with querymanager.
type Relationship struct {
	Subject      string
	Predicate    string
	Object       interface{}
	FromEntityID string  // Source entity
	ToEntityID   string  // Target entity
	Weight       float64 // For weighted edges
}

// RelationshipQuerier provides minimal interface for querying relationships.
// This interface exists to avoid import cycle with querymanager package.
type RelationshipQuerier interface {
	EntityQuerier
	QueryRelationships(ctx context.Context, entityID string, direction Direction) ([]*Relationship, error)
	QueryByPredicate(ctx context.Context, predicate string) ([]string, error)
}

// QueryManagerGraphProvider implements GraphProvider using QueryManager
type QueryManagerGraphProvider struct {
	queryManager RelationshipQuerier
}

// NewQueryManagerGraphProvider creates a GraphProvider backed by QueryManager
func NewQueryManagerGraphProvider(qm RelationshipQuerier) *QueryManagerGraphProvider {
	return &QueryManagerGraphProvider{
		queryManager: qm,
	}
}

// GetAllEntityIDs returns all entity IDs in the graph
func (p *QueryManagerGraphProvider) GetAllEntityIDs(_ context.Context) ([]string, error) {
	// QueryManager doesn't have a "get all entities" method
	// We'll need to query by common predicates or use a different approach

	// For now, we'll return an error indicating this needs implementation
	// In production, this would likely use a predicate index scan or
	// iterate through the entity store

	return nil, errs.WrapFatal(
		errs.ErrMissingConfig,
		"QueryManagerGraphProvider",
		"GetAllEntityIDs",
		"full graph scan not yet implemented - use PredicateGraphProvider instead",
	)
}

// GetNeighbors returns entity IDs connected to the given entity
func (p *QueryManagerGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	if entityID == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "QueryManagerGraphProvider", "GetNeighbors", "entityID is empty")
	}

	// Map direction string to QueryManager direction
	var qmDirection Direction
	switch direction {
	case "outgoing":
		qmDirection = DirectionOutgoing
	case "incoming":
		qmDirection = DirectionIncoming
	case "both":
		qmDirection = DirectionBoth
	default:
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "QueryManagerGraphProvider", "GetNeighbors", "invalid direction")
	}

	// Query relationships
	rels, err := p.queryManager.QueryRelationships(ctx, entityID, qmDirection)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManagerGraphProvider", "GetNeighbors", "query relationships")
	}

	// Extract unique neighbor IDs
	neighborMap := make(map[string]bool)
	for _, rel := range rels {
		// Add both endpoints (filter out self later)
		if rel.FromEntityID != entityID {
			neighborMap[rel.FromEntityID] = true
		}
		if rel.ToEntityID != entityID {
			neighborMap[rel.ToEntityID] = true
		}
	}

	// Convert to slice
	neighbors := make([]string, 0, len(neighborMap))
	for neighborID := range neighborMap {
		neighbors = append(neighbors, neighborID)
	}

	return neighbors, nil
}

// GetEdgeWeight returns the weight of an edge between two entities
func (p *QueryManagerGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	if fromID == "" || toID == "" {
		return 0.0, errs.WrapInvalid(errs.ErrMissingConfig, "QueryManagerGraphProvider", "GetEdgeWeight", "entity IDs are empty")
	}

	// Query outgoing relationships from fromID
	rels, err := p.queryManager.QueryRelationships(ctx, fromID, DirectionOutgoing)
	if err != nil {
		return 0.0, errs.WrapTransient(err, "QueryManagerGraphProvider", "GetEdgeWeight", "query relationships")
	}

	// Find edge to toID
	for _, rel := range rels {
		if rel.ToEntityID == toID {
			// Edge exists - return weight if available, else 1.0
			// Note: Current relationship model doesn't have weights
			// Future enhancement: extract from rel.Properties["weight"]
			return 1.0, nil
		}
	}

	// No edge found
	return 0.0, nil
}

// PredicateGraphProvider implements GraphProvider for entities matching a predicate
// This is more practical for real-world use: cluster entities of specific types
type PredicateGraphProvider struct {
	queryManager  RelationshipQuerier
	predicate     string          // Entity type/predicate to cluster
	validEntities map[string]bool // Cached set of valid entity IDs
}

// NewPredicateGraphProvider creates a GraphProvider for entities matching a predicate
// It caches the valid entity set at construction time for performance
func NewPredicateGraphProvider(qm RelationshipQuerier, predicate string) *PredicateGraphProvider {
	return &PredicateGraphProvider{
		queryManager:  qm,
		predicate:     predicate,
		validEntities: nil, // Lazy initialization on first use
	}
}

// GetAllEntityIDs returns all entities matching the predicate
func (p *PredicateGraphProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	// Initialize cache if needed
	if err := p.ensureValidEntitiesCache(ctx); err != nil {
		return nil, err
	}

	// Return all cached entity IDs
	entityIDs := make([]string, 0, len(p.validEntities))
	for id := range p.validEntities {
		entityIDs = append(entityIDs, id)
	}

	return entityIDs, nil
}

// ensureValidEntitiesCache initializes the valid entities cache if not already done
func (p *PredicateGraphProvider) ensureValidEntitiesCache(ctx context.Context) error {
	if p.validEntities != nil {
		return nil // Already cached
	}

	// Query all entities matching predicate
	entityIDs, err := p.queryManager.QueryByPredicate(ctx, p.predicate)
	if err != nil {
		return errs.WrapTransient(err, "PredicateGraphProvider", "ensureValidEntitiesCache", "query by predicate")
	}

	// Build cache map for O(1) lookup
	p.validEntities = make(map[string]bool, len(entityIDs))
	for _, id := range entityIDs {
		p.validEntities[id] = true
	}

	return nil
}

// GetNeighbors returns entity IDs connected to the given entity
func (p *PredicateGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	if entityID == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "PredicateGraphProvider", "GetNeighbors", "entityID is empty")
	}

	// Ensure cache is initialized
	if err := p.ensureValidEntitiesCache(ctx); err != nil {
		return nil, err
	}

	// Map direction
	var qmDirection Direction
	switch direction {
	case "outgoing":
		qmDirection = DirectionOutgoing
	case "incoming":
		qmDirection = DirectionIncoming
	case "both":
		qmDirection = DirectionBoth
	default:
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "PredicateGraphProvider", "GetNeighbors", "invalid direction")
	}

	// Query relationships
	rels, err := p.queryManager.QueryRelationships(ctx, entityID, qmDirection)
	if err != nil {
		return nil, errs.WrapTransient(err, "PredicateGraphProvider", "GetNeighbors", "query relationships")
	}

	// Extract neighbors that match predicate (using cached validEntities)
	neighborMap := make(map[string]bool)
	for _, rel := range rels {
		// Only include neighbors that match the predicate
		if rel.FromEntityID != entityID && p.validEntities[rel.FromEntityID] {
			neighborMap[rel.FromEntityID] = true
		}
		if rel.ToEntityID != entityID && p.validEntities[rel.ToEntityID] {
			neighborMap[rel.ToEntityID] = true
		}
	}

	// Convert to slice
	neighbors := make([]string, 0, len(neighborMap))
	for neighborID := range neighborMap {
		neighbors = append(neighbors, neighborID)
	}

	return neighbors, nil
}

// GetEdgeWeight returns the weight of an edge (unweighted: 1.0 or 0.0)
func (p *PredicateGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	if fromID == "" || toID == "" {
		return 0.0, errs.WrapInvalid(errs.ErrMissingConfig, "PredicateGraphProvider", "GetEdgeWeight", "entity IDs are empty")
	}

	// Query outgoing relationships
	rels, err := p.queryManager.QueryRelationships(ctx, fromID, DirectionOutgoing)
	if err != nil {
		return 0.0, errs.WrapTransient(err, "PredicateGraphProvider", "GetEdgeWeight", "query relationships")
	}

	// Find edge to toID
	for _, rel := range rels {
		if rel.ToEntityID == toID {
			return 1.0, nil
		}
	}

	return 0.0, nil
}
