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

func TestFindStepByName_DeepNested(t *testing.T) {
	// Test deeply nested parallel steps (parallel within parallel)
	def := &wfschema.Definition{
		Steps: []wfschema.StepDef{
			{Name: "top-level"},
			{
				Name: "outer-parallel",
				Type: "parallel",
				Steps: []wfschema.StepDef{
					{Name: "level-1-step"},
					{
						Name: "inner-parallel",
						Type: "parallel",
						Steps: []wfschema.StepDef{
							{Name: "deeply-nested-step"},
							{Name: "another-deep-step"},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		stepName string
		found    bool
	}{
		{
			name:     "top level step",
			stepName: "top-level",
			found:    true,
		},
		{
			name:     "outer parallel container",
			stepName: "outer-parallel",
			found:    true,
		},
		{
			name:     "level 1 nested step",
			stepName: "level-1-step",
			found:    true,
		},
		{
			name:     "inner parallel container",
			stepName: "inner-parallel",
			found:    true,
		},
		{
			name:     "deeply nested step (level 2)",
			stepName: "deeply-nested-step",
			found:    true,
		},
		{
			name:     "another deeply nested step",
			stepName: "another-deep-step",
			found:    true,
		},
		{
			name:     "non-existent step",
			stepName: "does-not-exist",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findStepByName(tt.stepName, def)
			if tt.found {
				assert.NotNil(t, result, "expected to find step %q", tt.stepName)
				assert.Equal(t, tt.stepName, result.Name)
			} else {
				assert.Nil(t, result, "expected not to find step %q", tt.stepName)
			}
		})
	}
}

func TestValidateFromReference(t *testing.T) {
	def := &wfschema.Definition{
		Steps: []wfschema.StepDef{
			{
				Name: "fetch",
				Outputs: map[string]wfschema.OutputDef{
					"result":  {},
					"details": {},
				},
			},
			{
				Name: "process",
				// No outputs declared - validation should skip output checking
			},
			{
				Name: "parallel-container",
				Type: "parallel",
				Steps: []wfschema.StepDef{
					{
						Name: "nested-fetch",
						Outputs: map[string]wfschema.OutputDef{
							"data": {},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name         string
		fromRef      string
		stepName     string
		inputName    string
		wantWarnings int
		wantContains string
	}{
		// Valid references
		{
			name:         "valid full path to declared output",
			fromRef:      "steps.fetch.output.result",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 0,
		},
		{
			name:         "valid shorthand to declared output",
			fromRef:      "fetch.result",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 0,
		},
		{
			name:         "valid trigger reference",
			fromRef:      "trigger.payload.field",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 0,
		},
		{
			name:         "valid execution reference",
			fromRef:      "execution.id",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 0,
		},
		{
			name:         "valid reference to step without outputs (skip validation)",
			fromRef:      "process.something",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 0, // No outputs declared = skip output validation
		},
		// Invalid references
		{
			name:         "reference to undeclared output",
			fromRef:      "steps.fetch.output.missing",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 1,
			wantContains: "references output \"missing\"",
		},
		{
			name:         "shorthand reference to undeclared output",
			fromRef:      "fetch.missing",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 1,
			wantContains: "references output \"missing\"",
		},
		{
			name:         "reference to non-existent step",
			fromRef:      "steps.nonexistent.output.field",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 1,
			wantContains: "non-existent step",
		},
		{
			name:         "empty reference",
			fromRef:      "",
			stepName:     "consumer",
			inputName:    "data",
			wantWarnings: 1,
			wantContains: "empty 'from' reference",
		},
		// Nested step references
		{
			name:         "valid nested step reference",
			fromRef:      "nested-fetch.data",
			stepName:     "consumer",
			inputName:    "input",
			wantWarnings: 0,
		},
		{
			name:         "nested step reference to undeclared output",
			fromRef:      "nested-fetch.missing",
			stepName:     "consumer",
			inputName:    "input",
			wantWarnings: 1,
			wantContains: "references output \"missing\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := validateFromReference(tt.fromRef, tt.stepName, tt.inputName, def)
			assert.Equal(t, tt.wantWarnings, len(warnings), "unexpected warning count: %v", warnings)
			if tt.wantContains != "" && len(warnings) > 0 {
				assert.Contains(t, warnings[0], tt.wantContains)
			}
		})
	}
}
