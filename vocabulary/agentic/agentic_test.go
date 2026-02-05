package agentic_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
	"github.com/c360studio/semstreams/vocabulary/agentic"
)

func TestPredicateFormat(t *testing.T) {
	// Verify all predicates follow the three-level dotted notation
	predicates := []string{
		// Intent
		agentic.IntentGoal,
		agentic.IntentType,
		agentic.IntentParameter,
		agentic.IntentAuthorized,
		agentic.IntentProduces,
		// Capability
		agentic.CapabilityName,
		agentic.CapabilityDescription,
		agentic.CapabilityExpression,
		agentic.CapabilityConfidence,
		agentic.CapabilitySkill,
		agentic.CapabilityConstraint,
		agentic.CapabilityPermission,
		// Delegation
		agentic.DelegationFrom,
		agentic.DelegationTo,
		agentic.DelegationScope,
		agentic.DelegationCapability,
		agentic.DelegationValidFrom,
		agentic.DelegationValidUntil,
		agentic.DelegationChain,
		// Accountability
		agentic.AccountabilityActor,
		agentic.AccountabilityAction,
		agentic.AccountabilityAssigned,
		agentic.AccountabilityRationale,
		agentic.AccountabilityCompliance,
		agentic.AccountabilityTimestamp,
		// Execution
		agentic.ExecutionEnvironment,
		agentic.ExecutionSecurity,
		agentic.ExecutionConstraint,
		agentic.ExecutionRateLimit,
		agentic.ExecutionBudget,
		agentic.ExecutionInput,
		agentic.ExecutionOutput,
		// Action
		agentic.ActionType,
		agentic.ActionExecutedBy,
		agentic.ActionProduced,
		agentic.ActionContext,
		agentic.ActionTrace,
		// Task
		agentic.TaskAssigned,
		agentic.TaskCapability,
		agentic.TaskSubtask,
		agentic.TaskDependency,
		agentic.TaskStatus,
	}

	for _, p := range predicates {
		if !vocabulary.IsValidPredicate(p) {
			t.Errorf("predicate %q does not follow three-level dotted notation", p)
		}
	}
}

func TestPredicateDomainPrefix(t *testing.T) {
	// Verify all predicates use the "agent" domain
	predicates := map[string]string{
		"IntentGoal":           agentic.IntentGoal,
		"CapabilityName":       agentic.CapabilityName,
		"DelegationFrom":       agentic.DelegationFrom,
		"AccountabilityActor":  agentic.AccountabilityActor,
		"ExecutionEnvironment": agentic.ExecutionEnvironment,
		"ActionType":           agentic.ActionType,
		"TaskStatus":           agentic.TaskStatus,
	}

	for name, p := range predicates {
		if !strings.HasPrefix(p, "agent.") {
			t.Errorf("%s: predicate %q does not start with 'agent.'", name, p)
		}
	}
}

func TestIRINamespaces(t *testing.T) {
	// Verify namespace constants are properly formatted
	namespaces := map[string]string{
		"Core":           agentic.CoreNamespace,
		"Intent":         agentic.IntentNamespace,
		"Capability":     agentic.CapabilityNamespace,
		"Delegation":     agentic.DelegationNamespace,
		"Accountability": agentic.AccountabilityNamespace,
		"Execution":      agentic.ExecutionNamespace,
	}

	for name, ns := range namespaces {
		if len(ns) < 10 {
			t.Errorf("%s namespace is too short: %q", name, ns)
		}
		if ns[len(ns)-1] != '#' {
			t.Errorf("%s namespace should end with '#': %q", name, ns)
		}
	}
}

func TestIRIConstants(t *testing.T) {
	// Verify IRI constants use correct namespaces
	tests := []struct {
		name      string
		iri       string
		namespace string
	}{
		{"IriAgent", agentic.IriAgent, agentic.CoreNamespace},
		{"IriIntent", agentic.IriIntent, agentic.CoreNamespace},
		{"IriCapability", agentic.IriCapability, agentic.CoreNamespace},
		{"IriHasIntentType", agentic.IriHasIntentType, agentic.IntentNamespace},
		{"IriHasSkill", agentic.IriHasSkill, agentic.CapabilityNamespace},
		{"IriDelegatedBy", agentic.IriDelegatedBy, agentic.DelegationNamespace},
		{"IriActor", agentic.IriActor, agentic.AccountabilityNamespace},
		{"IriExecutionEnvironment", agentic.IriExecutionEnvironment, agentic.ExecutionNamespace},
	}

	for _, tt := range tests {
		if len(tt.iri) <= len(tt.namespace) {
			t.Errorf("%s: IRI %q is not longer than namespace %q", tt.name, tt.iri, tt.namespace)
			continue
		}
		if tt.iri[:len(tt.namespace)] != tt.namespace {
			t.Errorf("%s: IRI %q does not start with namespace %q", tt.name, tt.iri, tt.namespace)
		}
	}
}

func TestRegistration(t *testing.T) {
	// Clear registry before test
	vocabulary.ClearRegistry()
	defer vocabulary.ClearRegistry()

	// Register all agentic predicates
	agentic.Register()

	// Verify key predicates are registered with correct metadata
	tests := []struct {
		predicate   string
		expectedIRI string
		hasInverse  bool
	}{
		{agentic.IntentGoal, agentic.IriIntent, false},
		{agentic.IntentType, agentic.IriHasIntentType, false},
		{agentic.CapabilityName, agentic.IriCapability, false},
		{agentic.CapabilityConfidence, agentic.IriCapabilityConfidence, false},
		{agentic.DelegationFrom, agentic.IriDelegatedBy, true},
		{agentic.DelegationTo, agentic.IriDelegatesTo, true},
		{agentic.AccountabilityActor, agentic.IriActor, false},
	}

	for _, tt := range tests {
		meta := vocabulary.GetPredicateMetadata(tt.predicate)
		if meta == nil {
			t.Errorf("predicate %q not registered", tt.predicate)
			continue
		}

		if meta.StandardIRI != tt.expectedIRI {
			t.Errorf("predicate %q: expected IRI %q, got %q", tt.predicate, tt.expectedIRI, meta.StandardIRI)
		}

		if tt.hasInverse && meta.InverseOf == "" {
			t.Errorf("predicate %q: expected inverse to be set", tt.predicate)
		}
	}
}

func TestDelegationInverseRelationship(t *testing.T) {
	vocabulary.ClearRegistry()
	defer vocabulary.ClearRegistry()

	agentic.Register()

	// Verify delegation predicates are properly linked as inverses
	fromMeta := vocabulary.GetPredicateMetadata(agentic.DelegationFrom)
	toMeta := vocabulary.GetPredicateMetadata(agentic.DelegationTo)

	if fromMeta == nil || toMeta == nil {
		t.Fatal("delegation predicates not registered")
	}

	if fromMeta.InverseOf != agentic.DelegationTo {
		t.Errorf("DelegationFrom.InverseOf = %q, want %q", fromMeta.InverseOf, agentic.DelegationTo)
	}

	if toMeta.InverseOf != agentic.DelegationFrom {
		t.Errorf("DelegationTo.InverseOf = %q, want %q", toMeta.InverseOf, agentic.DelegationFrom)
	}
}

func TestCapabilityConfidenceMetadata(t *testing.T) {
	vocabulary.ClearRegistry()
	defer vocabulary.ClearRegistry()

	agentic.Register()

	meta := vocabulary.GetPredicateMetadata(agentic.CapabilityConfidence)
	if meta == nil {
		t.Fatal("CapabilityConfidence not registered")
	}

	if meta.DataType != "float64" {
		t.Errorf("CapabilityConfidence.DataType = %q, want \"float64\"", meta.DataType)
	}

	if meta.Range != "0-1" {
		t.Errorf("CapabilityConfidence.Range = %q, want \"0-1\"", meta.Range)
	}
}

func TestPredicateCount(t *testing.T) {
	vocabulary.ClearRegistry()
	defer vocabulary.ClearRegistry()

	agentic.Register()

	predicates := vocabulary.ListRegisteredPredicates()

	// Expected predicates by category:
	// Intent: 5, Capability: 7, Delegation: 7, Accountability: 6, Execution: 7, Action: 5, Task: 5
	// Total: 42 predicates
	expectedMin := 42
	if len(predicates) < expectedMin {
		t.Errorf("expected at least %d predicates, got %d", expectedMin, len(predicates))
	}
}
