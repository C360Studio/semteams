package anomaly

import (
	"testing"

	"github.com/c360/semstreams/test/e2e/client"
)

func TestValidator_NoViolations(t *testing.T) {
	anomalies := []*client.Anomaly{
		{
			ID:      "anomaly-1",
			Type:    "semantic_structural_gap",
			EntityA: "doc-safety-001",
			EntityB: "doc-ops-001",
		},
	}

	expectation := &Expectation{
		UnexpectedGaps: []EntityPair{
			{EntityA: "sensor-temp", EntityB: "sensor-temp"},
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if !result.Passed() {
		t.Errorf("expected validation to pass, got violations: %v", result.Violations)
	}
}

func TestValidator_MissingExpected(t *testing.T) {
	anomalies := []*client.Anomaly{
		{
			ID:      "anomaly-1",
			Type:    "semantic_structural_gap",
			EntityA: "doc-safety-001",
			EntityB: "doc-ops-001",
		},
	}

	expectation := &Expectation{
		ExpectedGaps: []EntityPair{
			{
				EntityA: "obs-001",
				EntityB: "maint-001",
				Type:    "semantic_structural_gap",
				Reason:  "Observation should be linked to maintenance",
			},
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if result.Passed() {
		t.Errorf("expected validation to fail due to missing expected anomaly")
	}
	if len(result.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(result.Violations))
	}
	if result.Violations[0].Type != ViolationMissingExpected {
		t.Errorf("expected ViolationMissingExpected, got %v", result.Violations[0].Type)
	}
}

func TestValidator_ExpectedFound(t *testing.T) {
	anomalies := []*client.Anomaly{
		{
			ID:      "anomaly-1",
			Type:    "semantic_structural_gap",
			EntityA: "c360.logistics.observation.obs-001",
			EntityB: "c360.logistics.maintenance.maint-001",
		},
	}

	expectation := &Expectation{
		ExpectedGaps: []EntityPair{
			{
				EntityA: "obs-001",
				EntityB: "maint-001",
				Type:    "semantic_structural_gap",
			},
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if !result.Passed() {
		t.Errorf("expected validation to pass, got violations: %v", result.Violations)
	}
	if result.ExpectedFound != 1 {
		t.Errorf("expected ExpectedFound=1, got %d", result.ExpectedFound)
	}
}

func TestValidator_FalsePositive(t *testing.T) {
	anomalies := []*client.Anomaly{
		{
			ID:      "anomaly-1",
			Type:    "semantic_structural_gap",
			EntityA: "c360.logistics.sensor.temperature.sensor-temp-001",
			EntityB: "c360.logistics.sensor.temperature.sensor-temp-002",
		},
	}

	expectation := &Expectation{
		UnexpectedGaps: []EntityPair{
			{
				EntityA: "sensor-temp-001",
				EntityB: "sensor-temp-002",
				Type:    "semantic_structural_gap",
				Reason:  "Same-type sensors should not be flagged",
			},
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if result.Passed() {
		t.Errorf("expected validation to fail due to false positive")
	}
	if result.FalsePositiveTotal != 1 {
		t.Errorf("expected FalsePositiveTotal=1, got %d", result.FalsePositiveTotal)
	}
	if len(result.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(result.Violations))
	}
	if result.Violations[0].Type != ViolationFalsePositive {
		t.Errorf("expected ViolationFalsePositive, got %v", result.Violations[0].Type)
	}
}

func TestValidator_FalsePositiveRateExceeded(t *testing.T) {
	anomalies := []*client.Anomaly{
		{ID: "1", Type: "semantic_structural_gap", EntityA: "sensor-temp-001", EntityB: "sensor-temp-002"},
		{ID: "2", Type: "semantic_structural_gap", EntityA: "sensor-temp-002", EntityB: "sensor-temp-003"},
		{ID: "3", Type: "semantic_structural_gap", EntityA: "doc-safety", EntityB: "doc-ops"},
	}

	expectation := &Expectation{
		UnexpectedGaps: []EntityPair{
			{EntityA: "sensor-temp", EntityB: "sensor-temp", Type: "semantic_structural_gap"},
		},
		MaxFalsePositiveRate: 0.30, // 30% threshold
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	// 2 out of 3 are false positives = 66.7% > 30%
	if result.FalsePositiveTotal != 2 {
		t.Errorf("expected 2 false positives, got %d", result.FalsePositiveTotal)
	}

	// Should have violations for both false positives + rate exceeded
	hasRateViolation := false
	for _, v := range result.Violations {
		if v.Type == ViolationHighFalsePositiveRate {
			hasRateViolation = true
			break
		}
	}
	if !hasRateViolation {
		t.Errorf("expected ViolationHighFalsePositiveRate violation")
	}
}

func TestValidator_ReverseOrderMatching(t *testing.T) {
	// Anomaly has entities in reverse order
	anomalies := []*client.Anomaly{
		{
			ID:      "anomaly-1",
			Type:    "semantic_structural_gap",
			EntityA: "c360.logistics.maintenance.maint-001",
			EntityB: "c360.logistics.observation.obs-001",
		},
	}

	expectation := &Expectation{
		ExpectedGaps: []EntityPair{
			{EntityA: "obs-001", EntityB: "maint-001"}, // Order different from anomaly
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if !result.Passed() {
		t.Errorf("expected validation to pass with reverse order matching, got violations: %v", result.Violations)
	}
}

func TestValidator_TypeFiltering(t *testing.T) {
	anomalies := []*client.Anomaly{
		{
			ID:      "anomaly-1",
			Type:    "core_isolation", // Different type
			EntityA: "obs-001",
			EntityB: "",
		},
	}

	expectation := &Expectation{
		ExpectedGaps: []EntityPair{
			{
				EntityA: "obs-001",
				EntityB: "maint-001",
				Type:    "semantic_structural_gap", // Expects different type
			},
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if result.Passed() {
		t.Errorf("expected validation to fail due to type mismatch")
	}
}

func TestValidator_EmptyAnomalies(t *testing.T) {
	var anomalies []*client.Anomaly

	expectation := &Expectation{
		ExpectedGaps: []EntityPair{
			{EntityA: "obs-001", EntityB: "maint-001"},
		},
	}

	validator := NewValidator(expectation)
	result := validator.Validate(anomalies)

	if result.Passed() {
		t.Errorf("expected validation to fail with no anomalies")
	}
	if result.DetectedTotal != 0 {
		t.Errorf("expected DetectedTotal=0, got %d", result.DetectedTotal)
	}
}

func TestDefaultExpectation(t *testing.T) {
	expectation := DefaultExpectation()

	if expectation == nil {
		t.Error("expected default expectation to be non-nil")
	}

	// Should have some unexpected gaps defined for false positive detection
	if len(expectation.UnexpectedGaps) == 0 {
		t.Error("expected default expectation to have UnexpectedGaps")
	}
}
