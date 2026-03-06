package federation

import (
	"strings"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

const (
	// edgePredicatePrefix is the triple predicate prefix for federation edges.
	// Full predicate format: "federation.edge.{EdgeType}"
	// Queryable via NATS wildcard: "federation.edge.>"
	edgePredicatePrefix = "federation.edge."

	// edgeWeightSuffix is appended to edge predicates for weight triples.
	// Full predicate format: "federation.edge.{EdgeType}.weight"
	edgeWeightSuffix = ".weight"

	// provenancePredicatePrefix is the triple predicate prefix for provenance metadata.
	provenancePredicatePrefix = "federation.provenance."
)

// ToEntityState converts a federation Entity to a graph.EntityState.
//
// Conversion rules:
//   - Entity.Triples are copied directly (same message.Triple type)
//   - Each Edge becomes a relationship triple with predicate "federation.edge.{EdgeType}"
//   - Non-zero edge weights become a separate triple with predicate "federation.edge.{EdgeType}.weight"
//   - Primary provenance is stored as a "federation.provenance.source_type" triple
//
// The msgType parameter sets EntityState.MessageType for downstream consumers.
func ToEntityState(entity Entity, msgType message.Type) *graph.EntityState {
	state := &graph.EntityState{
		ID:          entity.ID,
		MessageType: msgType,
		UpdatedAt:   entity.Provenance.Timestamp,
	}

	// Direct triple copy.
	state.Triples = make([]message.Triple, len(entity.Triples))
	copy(state.Triples, entity.Triples)

	// Edges → relationship triples.
	for _, edge := range entity.Edges {
		state.Triples = append(state.Triples, message.Triple{
			Subject:    edge.FromID,
			Predicate:  edgePredicatePrefix + edge.EdgeType,
			Object:     edge.ToID,
			Source:     "federation",
			Timestamp:  entity.Provenance.Timestamp,
			Confidence: 1.0,
		})
		if edge.Weight != 0 {
			state.Triples = append(state.Triples, message.Triple{
				Subject:    edge.FromID,
				Predicate:  edgePredicatePrefix + edge.EdgeType + edgeWeightSuffix,
				Object:     edge.Weight,
				Source:     "federation",
				Timestamp:  entity.Provenance.Timestamp,
				Confidence: 1.0,
			})
		}
	}

	// Provenance → metadata triple.
	if entity.Provenance.SourceType != "" {
		state.Triples = append(state.Triples, message.Triple{
			Subject:    entity.ID,
			Predicate:  provenancePredicatePrefix + "source_type",
			Object:     entity.Provenance.SourceType,
			Source:     "federation",
			Timestamp:  entity.Provenance.Timestamp,
			Confidence: 1.0,
		})
	}

	return state
}

// FromEntityState converts a graph.EntityState back to a federation Entity.
//
// Conversion rules:
//   - Triples with "federation.edge.{EdgeType}" predicate are extracted as Edges
//   - Triples with "federation.edge.{EdgeType}.weight" predicate set edge weights
//   - All other triples are preserved as-is
//   - The provenance parameter becomes the Entity's primary provenance
func FromEntityState(state *graph.EntityState, provenance Provenance) Entity {
	entity := Entity{
		ID:         state.ID,
		Provenance: provenance,
	}

	// edgeKey is used for matching weight triples to their parent edges.
	type edgeKey struct{ from, to, edgeType string }
	edgeMap := make(map[edgeKey]*Edge)

	var regularTriples []message.Triple

	for _, t := range state.Triples {
		switch {
		case isEdgeWeightTriple(t.Predicate):
			// Extract edge type from "federation.edge.{EdgeType}.weight"
			edgeType := extractEdgeType(t.Predicate)
			if edgeType != "" {
				if w, wOk := t.Object.(float64); wOk {
					// Weight triples only carry Subject (FromID) and the weight value.
					// Match all edges with the same (from, edgeType) and set weight.
					for k, e := range edgeMap {
						if k.from == t.Subject && k.edgeType == edgeType {
							e.Weight = w
						}
					}
				}
			}

		case isEdgeTriple(t.Predicate):
			edgeType := extractEdgeType(t.Predicate)
			toID, _ := t.Object.(string)
			edge := Edge{
				FromID:   t.Subject,
				ToID:     toID,
				EdgeType: edgeType,
			}
			entity.Edges = append(entity.Edges, edge)
			edgeMap[edgeKey{t.Subject, toID, edgeType}] = &entity.Edges[len(entity.Edges)-1]

		case isProvenanceTriple(t.Predicate):
			// Skip provenance triples — provenance comes from the parameter.

		default:
			regularTriples = append(regularTriples, t)
		}
	}

	entity.Triples = regularTriples
	return entity
}

// isEdgeTriple returns true if the predicate represents a federation edge relationship.
// Matches "federation.edge.{EdgeType}" but NOT "federation.edge.{EdgeType}.weight".
func isEdgeTriple(predicate string) bool {
	if !strings.HasPrefix(predicate, edgePredicatePrefix) {
		return false
	}
	suffix := predicate[len(edgePredicatePrefix):]
	return suffix != "" && !strings.Contains(suffix, ".")
}

// isEdgeWeightTriple returns true if the predicate is an edge weight triple.
func isEdgeWeightTriple(predicate string) bool {
	return strings.HasPrefix(predicate, edgePredicatePrefix) &&
		strings.HasSuffix(predicate, edgeWeightSuffix)
}

// isProvenanceTriple returns true if the predicate is a federation provenance triple.
func isProvenanceTriple(predicate string) bool {
	return strings.HasPrefix(predicate, provenancePredicatePrefix)
}

// extractEdgeType extracts the edge type from a federation edge predicate.
// "federation.edge.calls" → "calls"
// "federation.edge.calls.weight" → "calls"
func extractEdgeType(predicate string) string {
	suffix := predicate[len(edgePredicatePrefix):]
	return strings.TrimSuffix(suffix, edgeWeightSuffix)
}

// NewFederationMessageType creates a message.Type for federation events.
// This is a convenience for use with ToEntityState.
func NewFederationMessageType() message.Type {
	return message.Type{Domain: "federation", Category: "graph_event", Version: "v1"}
}
