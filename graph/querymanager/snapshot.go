// Package querymanager provides graph snapshot queries.
// This file implements bounded spatial/temporal graph snapshots.
package querymanager

import (
	"context"
	"fmt"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/pkg/errs"
)

// GetGraphSnapshot creates a snapshot of entities within the specified bounds.
// It supports spatial, temporal, and entity type filtering with configurable limits.
func (qe *Manager) GetGraphSnapshot(ctx context.Context, bounds QueryBounds) (*GraphSnapshot, error) {
	startTime := time.Now()
	defer qe.recordActivity()

	// Apply timeout
	snapshotCtx := ctx
	if qe.config.Query.SnapshotTimeout > 0 {
		var cancel context.CancelFunc
		snapshotCtx, cancel = context.WithTimeout(ctx, qe.config.Query.SnapshotTimeout)
		defer cancel()
	}

	// Validate bounds
	if err := qe.validateQueryBounds(bounds); err != nil {
		return nil, errs.WrapInvalid(err, "QueryManager", "GetGraphSnapshot",
			"bounds validation failed")
	}

	// Execute snapshot query
	snapshot, err := qe.executeSnapshotQuery(snapshotCtx, bounds)
	if err != nil {
		qe.recordError("GetGraphSnapshot", err)
		if qe.metrics != nil {
			qe.metrics.snapshotTotal.Inc()
		}
		return nil, err
	}

	snapshot.Timestamp = time.Now()

	// Record metrics
	if qe.metrics != nil {
		qe.metrics.snapshotTotal.Inc()
		qe.metrics.snapshotDuration.Observe(time.Since(startTime).Seconds())
		qe.metrics.snapshotSizeHistogram.Observe(float64(snapshot.Count))
	}

	return snapshot, nil
}

// executeSnapshotQuery executes a graph snapshot query
func (qe *Manager) executeSnapshotQuery(ctx context.Context, bounds QueryBounds) (*GraphSnapshot, error) {
	snapshot := &GraphSnapshot{
		Entities:      []*gtypes.EntityState{},
		Relationships: []Relationship{},
		Bounds:        bounds,
		Count:         0,
		Truncated:     false,
	}

	var entityIDs []string

	// Query entities based on bounds
	if bounds.Spatial != nil && qe.indexManager != nil {
		// Query by spatial bounds
		spatialIDs, err := qe.QuerySpatial(ctx, *bounds.Spatial)
		if err != nil {
			return nil, errs.WrapTransient(err, "QueryManager", "GetGraphSnapshot",
				"spatial query failed")
		}
		entityIDs = append(entityIDs, spatialIDs...)
	}

	if bounds.Temporal != nil && qe.indexManager != nil {
		// Query by temporal bounds
		temporalIDs, err := qe.QueryTemporal(ctx, bounds.Temporal.Start, bounds.Temporal.End)
		if err != nil {
			return nil, errs.WrapTransient(err, "QueryManager", "GetGraphSnapshot",
				"temporal query failed")
		}
		entityIDs = append(entityIDs, temporalIDs...)
	}

	// If no specific bounds, get entities by type
	if len(entityIDs) == 0 && len(bounds.EntityTypes) > 0 && qe.indexManager != nil {
		for _, entityType := range bounds.EntityTypes {
			typeIDs, err := qe.QueryByPredicate(ctx, "type:"+entityType)
			if err != nil {
				return nil, errs.WrapTransient(err, "QueryManager", "GetGraphSnapshot",
					"type query failed")
			}
			entityIDs = append(entityIDs, typeIDs...)
		}
	}

	// Remove duplicates
	entityIDs = qe.removeDuplicateIDs(entityIDs)

	// Apply max entities limit
	if bounds.MaxEntities > 0 && len(entityIDs) > bounds.MaxEntities {
		entityIDs = entityIDs[:bounds.MaxEntities]
		snapshot.Truncated = true
	}

	// Load entities
	if len(entityIDs) > 0 {
		entities, err := qe.GetEntities(ctx, entityIDs)
		if err != nil {
			return nil, errs.WrapTransient(err, "QueryManager", "GetGraphSnapshot",
				"failed to load entities")
		}
		snapshot.Entities = entities
	}

	snapshot.Count = len(snapshot.Entities)

	// Load relationships between entities in the snapshot
	if len(snapshot.Entities) > 0 {
		relationships, err := qe.loadSnapshotRelationships(ctx, entityIDs)
		if err == nil {
			snapshot.Relationships = relationships
		}
		// If err != nil, we still return entities without relationships
	}

	return snapshot, nil
}

// loadSnapshotRelationships loads relationships between entities in a snapshot
func (qe *Manager) loadSnapshotRelationships(ctx context.Context, entityIDs []string) ([]Relationship, error) {
	if qe.indexManager == nil {
		return nil, nil
	}

	// Create a set for quick lookup
	entitySet := make(map[string]bool, len(entityIDs))
	for _, id := range entityIDs {
		entitySet[id] = true
	}

	var relationships []Relationship

	// For each entity, get outgoing relationships that point to other entities in the snapshot
	for _, entityID := range entityIDs {
		outgoing, err := qe.queryOutgoingRelationships(ctx, entityID)
		if err != nil {
			continue // Skip entities with errors
		}

		for _, rel := range outgoing {
			// Only include relationships where both endpoints are in the snapshot
			if entitySet[rel.ToEntityID] {
				relationships = append(relationships, *rel)
			}
		}
	}

	return relationships, nil
}

// validateQueryBounds validates query bounds
func (qe *Manager) validateQueryBounds(bounds QueryBounds) error {
	if bounds.MaxEntities > qe.config.Query.MaxSnapshotSize {
		msg := fmt.Sprintf("max entities %d exceeds limit %d",
			bounds.MaxEntities, qe.config.Query.MaxSnapshotSize)
		return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
			"validateQueryBounds", msg)
	}
	if bounds.Spatial != nil {
		if bounds.Spatial.North < bounds.Spatial.South {
			return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
				"validateQueryBounds", "north latitude must be >= south latitude")
		}
		if bounds.Spatial.East < bounds.Spatial.West {
			return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
				"validateQueryBounds", "east longitude must be >= west longitude")
		}
	}
	if bounds.Temporal != nil {
		if bounds.Temporal.End.Before(bounds.Temporal.Start) {
			return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
				"validateQueryBounds", "end time must be after start time")
		}
	}
	return nil
}

// removeDuplicateIDs removes duplicate entity IDs from a slice
func (qe *Manager) removeDuplicateIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))

	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	return unique
}
