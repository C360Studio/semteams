package anomaly

import (
	"fmt"
	"strings"

	"github.com/c360studio/semteams/test/e2e/client"
)

// Validator validates anomalies against ground truth expectations.
type Validator struct {
	expectation *Expectation
}

// NewValidator creates a validator with the given expectation.
func NewValidator(expectation *Expectation) *Validator {
	return &Validator{expectation: expectation}
}

// NewDefaultValidator creates a validator with default expectations.
func NewDefaultValidator() *Validator {
	return NewValidator(DefaultExpectation())
}

// Validate checks anomalies against expectations and returns results.
func (v *Validator) Validate(anomalies []*client.Anomaly) *ValidationResult {
	result := &ValidationResult{
		ExpectedTotal: len(v.expectation.ExpectedGaps),
		DetectedTotal: len(anomalies),
	}

	// Check for expected anomalies that should exist
	for _, expected := range v.expectation.ExpectedGaps {
		if !v.findAnomaly(anomalies, expected) {
			result.Violations = append(result.Violations, Violation{
				Type:       ViolationMissingExpected,
				Details:    fmt.Sprintf("expected anomaly not found: %s <-> %s", expected.EntityA, expected.EntityB),
				EntityPair: &expected,
			})
		} else {
			result.ExpectedFound++
		}
	}

	// Check for false positives (anomalies that shouldn't exist)
	for _, anomaly := range anomalies {
		if v.isUnexpected(anomaly) {
			result.FalsePositiveTotal++
			result.Violations = append(result.Violations, Violation{
				Type:      ViolationFalsePositive,
				Details:   fmt.Sprintf("false positive: anomaly detected for related entities %s <-> %s", anomaly.EntityA, anomaly.EntityB),
				AnomalyID: anomaly.ID,
				EntityPair: &EntityPair{
					EntityA: anomaly.EntityA,
					EntityB: anomaly.EntityB,
					Type:    anomaly.Type,
				},
			})
		}
	}

	// Check false positive rate
	if v.expectation.MaxFalsePositiveRate > 0 && result.DetectedTotal > 0 {
		fpRate := float64(result.FalsePositiveTotal) / float64(result.DetectedTotal)
		if fpRate > v.expectation.MaxFalsePositiveRate {
			result.Violations = append(result.Violations, Violation{
				Type: ViolationHighFalsePositiveRate,
				Details: fmt.Sprintf("false positive rate %.1f%% exceeds threshold %.1f%%",
					fpRate*100, v.expectation.MaxFalsePositiveRate*100),
			})
		}
	}

	return result
}

// findAnomaly checks if an expected anomaly pair exists in the detected anomalies.
func (v *Validator) findAnomaly(anomalies []*client.Anomaly, expected EntityPair) bool {
	for _, a := range anomalies {
		// Check if anomaly type matches (if specified)
		if expected.Type != "" && a.Type != expected.Type {
			continue
		}

		// Check if entity pair matches (in either direction)
		if v.matchesPair(a.EntityA, a.EntityB, expected.EntityA, expected.EntityB) {
			return true
		}
	}
	return false
}

// isUnexpected checks if an anomaly matches any unexpected gap definition.
func (v *Validator) isUnexpected(anomaly *client.Anomaly) bool {
	for _, unexpected := range v.expectation.UnexpectedGaps {
		// Check if anomaly type matches (if specified)
		if unexpected.Type != "" && anomaly.Type != unexpected.Type {
			continue
		}

		// Check if entity pair matches
		if v.matchesPair(anomaly.EntityA, anomaly.EntityB, unexpected.EntityA, unexpected.EntityB) {
			return true
		}
	}
	return false
}

// matchesPair checks if two entity pairs match using pattern matching.
// Matches in either direction (A-B or B-A).
func (v *Validator) matchesPair(entityA, entityB, patternA, patternB string) bool {
	// Forward match: A matches patternA and B matches patternB
	if v.matchesPattern(entityA, patternA) && v.matchesPattern(entityB, patternB) {
		return true
	}
	// Reverse match: A matches patternB and B matches patternA
	if v.matchesPattern(entityA, patternB) && v.matchesPattern(entityB, patternA) {
		return true
	}
	return false
}

// matchesPattern checks if an entity ID matches a pattern (case-insensitive substring).
func (v *Validator) matchesPattern(entityID, pattern string) bool {
	if pattern == "" {
		return true // Empty pattern matches anything
	}
	return strings.Contains(strings.ToLower(entityID), strings.ToLower(pattern))
}
