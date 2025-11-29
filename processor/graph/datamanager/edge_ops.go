package datamanager

import (
	"context"
	"fmt"
	"time"

	"github.com/c360/semstreams/errors"
	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

// Triple Operations

// AddTriple adds a triple to an entity
func (m *Manager) AddTriple(ctx context.Context, triple message.Triple) error {
	// Get entity
	entity, err := m.GetEntity(ctx, triple.Subject)
	if err != nil {
		// GetEntity already records its errors
		return err
	}

	// Validate relationship target if this is a relationship triple (object is an entity ID)
	if m.config.Edge.ValidateEdgeTargets {
		// Check if the object looks like an entity ID (simple heuristic: string that could be an entity)
		if objStr, ok := triple.Object.(string); ok {
			// Only validate if it looks like it could be a relationship
			// (e.g., predicates that typically indicate relationships)
			if triple.IsRelationship() {
				exists, err := m.ExistsEntity(ctx, objStr)
				if err != nil {
					err = errors.Wrap(err, "DataManager", "AddTriple", "validate target entity")
					return err
				}
				if !exists {
					err = errors.WrapInvalid(nil, "DataManager", "AddTriple",
						fmt.Sprintf("target entity %s does not exist", objStr))
					return err
				}
			}
		}
	}

	// Add or update triple
	found := false
	for i := range entity.Triples {
		if entity.Triples[i].Predicate == triple.Predicate {
			entity.Triples[i] = triple
			found = true
			break
		}
	}
	if !found {
		entity.Triples = append(entity.Triples, triple)
	}

	// Update entity
	_, err = m.UpdateEntity(ctx, entity)
	// UpdateEntity already records its errors
	return err
}

// RemoveTriple removes a triple from an entity by predicate
func (m *Manager) RemoveTriple(ctx context.Context, subject, predicate string) error {
	// Get entity
	entity, err := m.GetEntity(ctx, subject)
	if err != nil {
		// GetEntity already records its errors
		return err
	}

	// Remove triple
	removed := false
	newTriples := []message.Triple{}
	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			removed = true
			continue
		}
		newTriples = append(newTriples, triple)
	}

	if !removed {
		err = errors.WrapInvalid(nil, "DataManager", "RemoveTriple",
			fmt.Sprintf("triple with predicate %s not found for subject %s", predicate, subject))
		return err
	}

	entity.Triples = newTriples

	// Update entity
	_, err = m.UpdateEntity(ctx, entity)
	// UpdateEntity already records its errors
	return err
}

// CreateRelationship creates a relationship between two entities using a triple
func (m *Manager) CreateRelationship(
	ctx context.Context,
	fromEntityID, toEntityID string,
	predicate string,
	metadata map[string]any,
) error {
	// Create relationship triple
	triple := message.Triple{
		Subject:   fromEntityID,
		Predicate: predicate,
		Object:    toEntityID,
		Timestamp: time.Now(),
	}

	// If there's metadata, we could store it as additional triples
	// For now, simple approach: just create the relationship triple
	err := m.AddTriple(ctx, triple)
	if err != nil {
		return err
	}

	// If metadata is provided, add additional triples for each metadata field
	if len(metadata) > 0 {
		entity, err := m.GetEntity(ctx, fromEntityID)
		if err != nil {
			return err
		}

		for key, value := range metadata {
			metaTriple := message.Triple{
				Subject:   fromEntityID,
				Predicate: fmt.Sprintf("%s.%s", predicate, key),
				Object:    value,
				Timestamp: time.Now(),
			}
			entity.Triples = append(entity.Triples, metaTriple)
		}

		_, err = m.UpdateEntity(ctx, entity)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteRelationship deletes a relationship between entities
func (m *Manager) DeleteRelationship(ctx context.Context, fromEntityID, toEntityID string, predicate string) error {
	// Get entity to find the relationship triple
	entity, err := m.GetEntity(ctx, fromEntityID)
	if err != nil {
		return err
	}

	// Find and remove the relationship triple
	found := false
	newTriples := []message.Triple{}
	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			if objStr, ok := triple.Object.(string); ok && objStr == toEntityID {
				found = true
				continue
			}
		}
		newTriples = append(newTriples, triple)
	}

	if !found {
		return errors.WrapInvalid(nil, "DataManager", "DeleteRelationship",
			fmt.Sprintf("relationship %s from %s to %s not found", predicate, fromEntityID, toEntityID))
	}

	entity.Triples = newTriples

	// Update entity
	_, err = m.UpdateEntity(ctx, entity)
	return err
}

// CleanupIncomingReferences is a no-op - actual cleanup is handled by IndexManager
// The IndexManager watches for entity delete events and uses CleanupOrphanedIncomingReferences
// to remove references from INCOMING_INDEX before deleting from OUTGOING_INDEX.
// This method exists for interface compatibility.
func (m *Manager) CleanupIncomingReferences(
	ctx context.Context,
	deletedEntityID string,
	outgoingTriples []message.Triple,
) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// NOTE: Actual cleanup is handled by IndexManager.CleanupOrphanedIncomingReferences
	// which is called during delete event processing in IndexManager.updateIndex()
	m.logger.Debug("CleanupIncomingReferences called (cleanup handled by IndexManager)",
		"deleted_entity", deletedEntityID,
		"outgoing_triples_count", len(outgoingTriples),
	)

	return nil
}

// CheckOutgoingTriplesConsistency checks relationship triple consistency
func (m *Manager) CheckOutgoingTriplesConsistency(
	ctx context.Context,
	_ string,
	entity *gtypes.EntityState,
	status *EntityIndexStatus,
) {
	if entity == nil || status == nil {
		return
	}

	status.OutgoingEdgesConsistent = true // Keep field name for now (would need schema update)
	status.InconsistentEdges = []string{}

	// Check each relationship triple's target exists
	for _, triple := range entity.Triples {
		if triple.IsRelationship() {
			if targetID, ok := triple.Object.(string); ok {
				exists, err := m.ExistsEntity(ctx, targetID)
				if err != nil || !exists {
					status.OutgoingEdgesConsistent = false
					status.InconsistentEdges = append(status.InconsistentEdges, targetID)
				}
			}
		}
	}
}

// HasRelationshipToEntity checks if an entity has a relationship triple to a target
func (m *Manager) HasRelationshipToEntity(entity *gtypes.EntityState, targetEntityID string, predicate string) bool {
	if entity == nil {
		return false
	}

	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			if targetID, ok := triple.Object.(string); ok && targetID == targetEntityID {
				return true
			}
		}
	}
	return false
}
