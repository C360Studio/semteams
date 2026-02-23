package workflow

import (
	"testing"

	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
	"github.com/stretchr/testify/assert"
)

// NOTE: Tests for InputType, OutputType, PayloadMapping, and PassThrough validation
// have been removed as these fields were replaced by the Inputs/Outputs pattern (ADR-020).
//
// Removed tests:
// - TestValidateWorkflowTypes: validated InputType/OutputType fields
// - TestRegistry_ValidateTypesOnRegister: validated type registration at workflow load
// - TestValidateStepTypes: validated step-level type annotations
// - TestValidatePayloadMappingPath: validated PayloadMapping path references
// - TestValidateActionPayloadMapping: validated action payload mappings
//
// The new validation logic for Inputs/Outputs is tested through:
// - schema_test.go: InputRef.Validate() and OutputDef.Validate()
// - Integration tests exercising the full workflow execution path

func TestStepExistsInWorkflow(t *testing.T) {
	def := &wfschema.Definition{
		Steps: []wfschema.StepDef{
			{Name: "top-level-step"},
			{
				Name: "parallel-container",
				Type: "parallel",
				Steps: []wfschema.StepDef{
					{Name: "nested-step-one"},
					{Name: "nested-step-two"},
				},
			},
		},
	}

	tests := []struct {
		name     string
		stepName string
		expected bool
	}{
		{
			name:     "top level step exists",
			stepName: "top-level-step",
			expected: true,
		},
		{
			name:     "parallel container exists",
			stepName: "parallel-container",
			expected: true,
		},
		{
			name:     "nested step exists",
			stepName: "nested-step-one",
			expected: true,
		},
		{
			name:     "second nested step exists",
			stepName: "nested-step-two",
			expected: true,
		},
		{
			name:     "non-existent step",
			stepName: "missing-step",
			expected: false,
		},
		{
			name:     "empty step name",
			stepName: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stepExistsInWorkflow(tt.stepName, def)
			assert.Equal(t, tt.expected, result)
		})
	}
}
