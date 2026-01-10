// Package inference provides structural anomaly detection for missing relationships.
// It analyzes the gap between semantic similarity and structural distance to identify
// potential missing edges in the knowledge graph.
package inference

import (
	"time"
)

// AnomalyType identifies the category of structural anomaly detected.
type AnomalyType string

const (
	// AnomalySemanticStructuralGap indicates high semantic similarity with high structural distance.
	// This suggests entities should probably be related but aren't explicitly connected.
	AnomalySemanticStructuralGap AnomalyType = "semantic_structural_gap"

	// AnomalyCoreIsolation indicates a high k-core entity with few same-core peer connections.
	// This suggests missing hub relationships within the core structure.
	AnomalyCoreIsolation AnomalyType = "core_isolation"

	// AnomalyCoreDemotion indicates an entity dropped from its k-core level between computations.
	// This signals lost relationships or potential data quality issues.
	AnomalyCoreDemotion AnomalyType = "core_demotion"

	// AnomalyTransitivityGap indicates A->B and B->C exist but A-C distance exceeds expected.
	// This suggests a missing transitive relationship for configured predicates.
	AnomalyTransitivityGap AnomalyType = "transitivity_gap"
)

// AnomalyStatus represents the lifecycle state of an anomaly.
type AnomalyStatus string

const (
	// StatusPending indicates the anomaly is awaiting review.
	StatusPending AnomalyStatus = "pending"

	// StatusLLMReviewing indicates the anomaly is currently being processed by LLM.
	StatusLLMReviewing AnomalyStatus = "llm_reviewing"

	// StatusLLMApproved indicates the LLM approved this anomaly (high confidence).
	StatusLLMApproved AnomalyStatus = "llm_approved"

	// StatusLLMRejected indicates the LLM rejected this anomaly (low confidence).
	StatusLLMRejected AnomalyStatus = "llm_rejected"

	// StatusHumanReview indicates the anomaly has been escalated for human decision.
	StatusHumanReview AnomalyStatus = "human_review"

	// StatusApproved indicates a human approved this anomaly.
	StatusApproved AnomalyStatus = "approved"

	// StatusRejected indicates a human rejected this anomaly.
	StatusRejected AnomalyStatus = "rejected"

	// StatusApplied indicates the suggested relationship was created in the graph.
	StatusApplied AnomalyStatus = "applied"

	// StatusDismissed indicates the anomaly was dismissed and should not be re-detected.
	// This prevents the same entity pair from being flagged repeatedly.
	StatusDismissed AnomalyStatus = "dismissed"

	// StatusAutoApplied indicates the relationship was automatically applied
	// because it met the auto-apply threshold (high confidence).
	StatusAutoApplied AnomalyStatus = "auto_applied"
)

// StructuralAnomaly represents a detected potential issue in the graph structure.
type StructuralAnomaly struct {
	// ID is a unique identifier for this anomaly (UUID).
	ID string `json:"id"`

	// Type categorizes the anomaly detection method.
	Type AnomalyType `json:"type"`

	// EntityA is the primary entity involved in the anomaly.
	EntityA string `json:"entity_a"`

	// EntityB is the secondary entity (empty for single-entity anomalies like CoreDemotion).
	EntityB string `json:"entity_b,omitempty"`

	// Confidence is a score from 0.0 to 1.0 indicating detection certainty.
	Confidence float64 `json:"confidence"`

	// Evidence contains type-specific proof of the anomaly.
	Evidence Evidence `json:"evidence"`

	// Suggestion is the proposed relationship to address the anomaly.
	Suggestion *RelationshipSuggestion `json:"suggestion,omitempty"`

	// Status is the current lifecycle state.
	Status AnomalyStatus `json:"status"`

	// DetectedAt is when the anomaly was first identified.
	DetectedAt time.Time `json:"detected_at"`

	// ReviewedAt is when the anomaly was reviewed (nil if not yet reviewed).
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`

	// ReviewedBy identifies who reviewed (e.g., "llm", "user@example.com").
	ReviewedBy string `json:"reviewed_by,omitempty"`

	// LLMReasoning is the explanation provided by the LLM for its decision.
	LLMReasoning string `json:"llm_reasoning,omitempty"`

	// ReviewNotes are additional notes from human review.
	ReviewNotes string `json:"review_notes,omitempty"`

	// EntityAContext is cached context for human review display.
	EntityAContext string `json:"entity_a_context,omitempty"`

	// EntityBContext is cached context for human review display.
	EntityBContext string `json:"entity_b_context,omitempty"`
}

// Evidence contains type-specific proof of an anomaly.
// Different anomaly types populate different fields.
type Evidence struct {
	// Semantic-Structural Gap evidence
	Similarity         float64 `json:"similarity,omitempty"`           // Embedding similarity score
	StructuralDistance int     `json:"structural_distance,omitempty"`  // Actual or estimated hop count
	DistanceLowerBound int     `json:"distance_lower_bound,omitempty"` // Triangle inequality lower bound
	DistanceUpperBound int     `json:"distance_upper_bound,omitempty"` // Triangle inequality upper bound

	// Core Isolation evidence
	CoreLevel         int     `json:"core_level,omitempty"`          // Entity's k-core number
	PeerCount         int     `json:"peer_count,omitempty"`          // Number of same-core neighbors
	ExpectedPeerCount int     `json:"expected_peer_count,omitempty"` // Expected based on core level
	PeerConnectivity  float64 `json:"peer_connectivity,omitempty"`   // Ratio of actual/expected peers
	CommunityID       string  `json:"community_id,omitempty"`        // Community where isolation detected

	// Core Demotion evidence
	PreviousCoreLevel int `json:"previous_core_level,omitempty"` // Core level before demotion
	CurrentCoreLevel  int `json:"current_core_level,omitempty"`  // Core level after demotion
	LostConnections   int `json:"lost_connections,omitempty"`    // Number of connections lost

	// Transitivity Gap evidence
	Predicate       string   `json:"predicate,omitempty"`         // The transitive predicate
	ChainPath       []string `json:"chain_path,omitempty"`        // The A->B->C path entities
	ActualDistance  int      `json:"actual_distance,omitempty"`   // Measured A-C distance
	ExpectedMaxHops int      `json:"expected_max_hops,omitempty"` // Config max for transitivity
}

// RelationshipSuggestion proposes a relationship to address an anomaly.
type RelationshipSuggestion struct {
	// FromEntity is the source entity ID for the suggested relationship.
	FromEntity string `json:"from_entity"`

	// ToEntity is the target entity ID for the suggested relationship.
	ToEntity string `json:"to_entity"`

	// Predicate is the suggested relationship type (e.g., "inferred.related_to").
	Predicate string `json:"predicate"`

	// Confidence is the confidence in this specific suggestion.
	Confidence float64 `json:"confidence"`

	// Reasoning explains why this relationship is suggested.
	Reasoning string `json:"reasoning"`
}

// IsResolved returns true if the anomaly has reached a terminal state.
func (a *StructuralAnomaly) IsResolved() bool {
	switch a.Status {
	case StatusApproved, StatusRejected, StatusApplied, StatusLLMRejected,
		StatusDismissed, StatusAutoApplied:
		return true
	default:
		return false
	}
}

// NeedsHumanReview returns true if the anomaly requires human attention.
func (a *StructuralAnomaly) NeedsHumanReview() bool {
	return a.Status == StatusHumanReview
}

// CanAutoApprove returns true if confidence is above the given threshold.
func (a *StructuralAnomaly) CanAutoApprove(threshold float64) bool {
	return a.Confidence >= threshold
}

// CanAutoReject returns true if confidence is below the given threshold.
func (a *StructuralAnomaly) CanAutoReject(threshold float64) bool {
	return a.Confidence <= threshold
}

// Decision represents the outcome of anomaly review.
type Decision int

const (
	// DecisionApprove indicates the anomaly should be applied to the graph.
	DecisionApprove Decision = iota
	// DecisionReject indicates the anomaly should be dismissed.
	DecisionReject
	// DecisionHumanReview indicates the anomaly needs human attention.
	DecisionHumanReview
)

// String returns a human-readable representation of the decision.
func (d Decision) String() string {
	switch d {
	case DecisionApprove:
		return "approve"
	case DecisionReject:
		return "reject"
	case DecisionHumanReview:
		return "human_review"
	default:
		return "unknown"
	}
}
