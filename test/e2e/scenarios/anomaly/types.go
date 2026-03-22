// Package anomaly provides ground truth validation for anomaly detection.
package anomaly

// Expectation defines expected or unexpected anomalies for validation.
type Expectation struct {
	// ExpectedGaps are entity pairs that SHOULD be flagged as semantic gaps.
	// These represent missing relationships that the system should detect.
	ExpectedGaps []EntityPair `json:"expected_gaps,omitempty"`

	// UnexpectedGaps are related entity pairs that should NOT be flagged.
	// These represent false positives - pairs that are semantically related
	// and should not be detected as gaps.
	UnexpectedGaps []EntityPair `json:"unexpected_gaps,omitempty"`

	// MaxFalsePositiveRate is the maximum acceptable false positive rate (0.0-1.0).
	// If more than this percentage of detected anomalies are in UnexpectedGaps,
	// the validation fails. Default 0 means no false positive checking.
	MaxFalsePositiveRate float64 `json:"max_false_positive_rate,omitempty"`
}

// EntityPair represents a pair of entities for anomaly validation.
type EntityPair struct {
	// EntityA is the first entity ID pattern (substring match)
	EntityA string `json:"entity_a"`

	// EntityB is the second entity ID pattern (substring match, optional for core anomalies)
	EntityB string `json:"entity_b,omitempty"`

	// Type is the expected anomaly type (semantic_structural_gap, core_isolation, etc.)
	Type string `json:"type,omitempty"`

	// Reason explains why this pair should/shouldn't be an anomaly
	Reason string `json:"reason,omitempty"`
}

// Violation describes an anomaly validation violation.
type Violation struct {
	// Type indicates the violation type
	Type ViolationType `json:"type"`

	// Details provides human-readable explanation
	Details string `json:"details"`

	// EntityPair is the entity pair involved (if applicable)
	EntityPair *EntityPair `json:"entity_pair,omitempty"`

	// AnomalyID is the ID of the anomaly involved (if applicable)
	AnomalyID string `json:"anomaly_id,omitempty"`
}

// ViolationType categorizes anomaly validation violations.
type ViolationType string

const (
	// ViolationMissingExpected means an expected anomaly was not detected
	ViolationMissingExpected ViolationType = "missing_expected"

	// ViolationFalsePositive means an anomaly was detected for a related pair
	ViolationFalsePositive ViolationType = "false_positive"

	// ViolationHighFalsePositiveRate means the false positive rate exceeded threshold
	ViolationHighFalsePositiveRate ViolationType = "high_false_positive_rate"
)

// ValidationResult contains the outcome of anomaly ground truth validation.
type ValidationResult struct {
	// ExpectedTotal is the count of expected anomalies defined
	ExpectedTotal int `json:"expected_total"`

	// ExpectedFound is the count of expected anomalies that were detected
	ExpectedFound int `json:"expected_found"`

	// FalsePositiveTotal is the count of detected anomalies that match unexpected pairs
	FalsePositiveTotal int `json:"false_positive_total"`

	// DetectedTotal is the total count of anomalies detected
	DetectedTotal int `json:"detected_total"`

	// Violations lists all detected violations
	Violations []Violation `json:"violations,omitempty"`
}

// Passed returns true if validation passed (no critical violations).
func (r *ValidationResult) Passed() bool {
	return len(r.Violations) == 0
}

// DefaultExpectation returns anomaly ground truth derived from test data.
// These expectations are based on the semantic relationships in testdata/semantic/.
func DefaultExpectation() *Expectation {
	return &Expectation{
		// Expected anomalies - these SHOULD be detected
		// Note: doc-safety-001 was previously expected as core_isolation but
		// system-affinity clustering (part[3] grouping) correctly groups it
		// with other document entities. A safety doc among documents is not
		// anomalous — it's where it belongs.
		ExpectedGaps: []EntityPair{
			{
				EntityA: "doc-emergency-001",
				Type:    "core_isolation",
				Reason:  "Emergency response plan is generic policy - isolated from operational data",
			},
		},

		// Related entities that should NOT be flagged as gaps
		// (false positive detection)
		UnexpectedGaps: []EntityPair{
			{
				EntityA: "sensor-temp-001",
				EntityB: "sensor-temp-002",
				Type:    "semantic_structural_gap",
				Reason:  "Temperature sensors are in same hierarchy and should not be flagged as gap",
			},
			{
				EntityA: "maint-001",
				EntityB: "maint-002",
				Type:    "semantic_structural_gap",
				Reason:  "Maintenance records in same group should not be flagged as gap",
			},
		},

		// Allow up to 20% false positive rate
		MaxFalsePositiveRate: 0.20,
	}
}
