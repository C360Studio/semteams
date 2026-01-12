// Package community provides ground truth validation for community detection.
package community

// Expectation defines ground truth for community validation.
// It describes entities that should (or should not) be grouped together.
type Expectation struct {
	// Name is a human-readable identifier for this expectation
	Name string `json:"name"`

	// MustContain lists entity ID patterns that must be in the same community.
	// Uses substring matching (case-insensitive).
	MustContain []string `json:"must_contain"`

	// MustNotContain lists entity ID patterns that must NOT be in the same community
	// as the MustContain entities. Used to detect over-grouping.
	MustNotContain []string `json:"must_not_contain,omitempty"`

	// MinSize is the minimum size of the community containing MustContain entities.
	// Optional - 0 means no minimum size requirement.
	MinSize int `json:"min_size,omitempty"`

	// Reason explains why these entities should be grouped together
	Reason string `json:"reason,omitempty"`
}

// Violation describes a community coherence violation.
type Violation struct {
	// ExpectationName identifies which expectation was violated
	ExpectationName string `json:"expectation_name"`

	// Type indicates the violation type
	Type ViolationType `json:"type"`

	// Details provides human-readable explanation
	Details string `json:"details"`

	// Entities lists the specific entities involved
	Entities []string `json:"entities,omitempty"`

	// CommunityIDs lists the communities where entities were found
	CommunityIDs []string `json:"community_ids,omitempty"`
}

// ViolationType categorizes community violations.
type ViolationType string

const (
	// ViolationNotGrouped means expected entities are in different communities
	ViolationNotGrouped ViolationType = "not_grouped"

	// ViolationUnexpectedGrouped means excluded entities are in the same community
	ViolationUnexpectedGrouped ViolationType = "unexpected_grouped"

	// ViolationEntityNotFound means a required entity was not found in any community
	ViolationEntityNotFound ViolationType = "entity_not_found"

	// ViolationCommunityTooSmall means the community is smaller than MinSize
	ViolationCommunityTooSmall ViolationType = "community_too_small"
)

// ValidationResult contains the outcome of community ground truth validation.
type ValidationResult struct {
	// ExpectationsTotal is the number of expectations evaluated
	ExpectationsTotal int `json:"expectations_total"`

	// ExpectationsPassed is the number of expectations that passed
	ExpectationsPassed int `json:"expectations_passed"`

	// Violations lists all detected violations
	Violations []Violation `json:"violations,omitempty"`
}

// Passed returns true if all expectations passed.
func (r *ValidationResult) Passed() bool {
	return len(r.Violations) == 0
}

// DefaultExpectations returns community ground truth derived from test data.
// These expectations are based on the semantic relationships in testdata/semantic/.
func DefaultExpectations() []Expectation {
	return []Expectation{
		{
			Name:           "temperature_sensors",
			MustContain:    []string{"sensor-temp-001", "sensor-temp-002", "sensor-temp-003"},
			MustNotContain: []string{"doc-hr"},
			Reason:         "Temperature sensors monitoring cold storage should cluster together",
		},
		{
			Name:        "maintenance_records",
			MustContain: []string{"maint-001", "maint-002", "maint-003"},
			Reason:      "Maintenance records should cluster based on equipment relationships",
		},
		{
			Name:           "safety_documents",
			MustContain:    []string{"doc-safety", "doc-emergency"},
			MustNotContain: []string{"sensor-motion"},
			Reason:         "Safety-related documents should cluster together",
		},
	}
}
