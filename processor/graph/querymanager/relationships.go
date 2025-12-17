// Package querymanager provides relationship queries.
// This file implements incoming/outgoing relationship queries for entities.
package querymanager

import (
	"context"
	"fmt"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/pkg/errs"
)

// QueryRelationships queries relationships for an entity in the specified direction.
// Direction can be incoming, outgoing, or both.
func (qe *Manager) QueryRelationships(
	ctx context.Context, entityID string, direction Direction,
) ([]*Relationship, error) {
	startTime := time.Now()
	defer qe.recordActivity()

	// Validate direction
	if !qe.isValidDirection(direction) {
		return nil, errs.WrapInvalid(gtypes.ErrInvalidQueryParams, "QueryManager", "QueryRelationships",
			fmt.Sprintf("invalid direction: %s", direction))
	}

	// Check if index manager is available for relationship queries
	if qe.indexManager == nil {
		return nil, errs.WrapTransient(ErrIndexManagerUnavailable, "QueryManager", "QueryRelationships",
			"index manager dependency unavailable")
	}

	var relationships []*Relationship
	var err error

	// Query based on direction
	switch direction {
	case DirectionIncoming:
		relationships, err = qe.queryIncomingRelationships(ctx, entityID)
	case DirectionOutgoing:
		relationships, err = qe.queryOutgoingRelationships(ctx, entityID)
	case DirectionBoth:
		incoming, err1 := qe.queryIncomingRelationships(ctx, entityID)
		if err1 != nil {
			return nil, err1
		}
		outgoing, err2 := qe.queryOutgoingRelationships(ctx, entityID)
		if err2 != nil {
			return nil, err2
		}
		relationships = append(incoming, outgoing...)
	default:
		return nil, errs.WrapInvalid(gtypes.ErrInvalidQueryParams, "QueryManager", "QueryRelationships",
			fmt.Sprintf("unsupported direction: %s", direction))
	}

	if err != nil {
		qe.recordError("QueryRelationships", err)
		if qe.metrics != nil {
			qe.metrics.RecordQuery("query_engine", "relationships", time.Since(startTime), 0, false)
		}
		return nil, err
	}

	// Record metrics
	if qe.metrics != nil {
		qe.metrics.RecordQuery("query_engine", "relationships", time.Since(startTime), len(relationships), true)
	}

	return relationships, nil
}

// queryIncomingRelationships queries incoming relationships for an entity.
// Uses the INCOMING_INDEX via IndexManager.
func (qe *Manager) queryIncomingRelationships(ctx context.Context, entityID string) ([]*Relationship, error) {
	// Use index manager to get incoming relationships
	indexRelationships, err := qe.indexManager.GetIncomingRelationships(ctx, entityID)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GetIncomingRelationships",
			fmt.Sprintf("index manager failed for entity: %s", entityID))
	}

	// Convert to query manager relationship format
	relationships := make([]*Relationship, len(indexRelationships))
	for i, rel := range indexRelationships {
		relationships[i] = &Relationship{
			FromEntityID: rel.FromEntityID,
			ToEntityID:   entityID, // This is the target entity
			EdgeType:     rel.EdgeType,
			Properties:   rel.Properties,
			Weight:       rel.Weight,
			CreatedAt:    rel.CreatedAt,
			UpdatedAt:    rel.CreatedAt, // index manager doesn't track updated time
		}
	}

	return relationships, nil
}

// queryOutgoingRelationships queries outgoing relationships for an entity.
// Uses the OUTGOING_INDEX via IndexManager for efficient forward edge traversal.
func (qe *Manager) queryOutgoingRelationships(ctx context.Context, entityID string) ([]*Relationship, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Use index manager to get outgoing relationships (mirrors incoming pattern)
	indexRelationships, err := qe.indexManager.GetOutgoingRelationships(ctx, entityID)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "queryOutgoingRelationships",
			fmt.Sprintf("index manager failed for entity: %s", entityID))
	}

	// Convert to query manager relationship format
	relationships := make([]*Relationship, len(indexRelationships))
	now := time.Now()
	for i, rel := range indexRelationships {
		relationships[i] = &Relationship{
			FromEntityID: entityID,
			ToEntityID:   rel.ToEntityID,
			EdgeType:     rel.EdgeType,
			Properties:   make(map[string]interface{}),
			Weight:       1.0, // Index doesn't store weight
			CreatedAt:    now,
			UpdatedAt:    now,
		}
	}

	return relationships, nil
}

// isValidDirection checks if a direction is valid
func (qe *Manager) isValidDirection(direction Direction) bool {
	switch direction {
	case DirectionIncoming, DirectionOutgoing, DirectionBoth:
		return true
	default:
		return false
	}
}
