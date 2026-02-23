package workflow

import (
	"testing"

	"github.com/c360studio/semstreams/component"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRegistryPayload is a test payload type for registry validation tests
type testRegistryPayload struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

// testRegistryFactory creates test payload instances
func testRegistryFactory() any {
	return &testRegistryPayload{}
}

// testRegistryBuilder builds test payloads from field maps
func testRegistryBuilder(fields map[string]any) (any, error) {
	payload := &testRegistryPayload{}
	if field1, ok := fields["field1"].(string); ok {
		payload.Field1 = field1
	}
	if field2, ok := fields["field2"].(float64); ok {
		payload.Field2 = int(field2)
	}
	return payload, nil
}

// testAnotherPayload is another test payload type
type testAnotherPayload struct {
	Data string `json:"data"`
}

func testAnotherFactory() any {
	return &testAnotherPayload{}
}

func testAnotherBuilder(fields map[string]any) (any, error) {
	payload := &testAnotherPayload{}
	if data, ok := fields["data"].(string); ok {
		payload.Data = data
	}
	return payload, nil
}

func TestValidateWorkflowTypes(t *testing.T) {
	// Create a test registry with registered types
	registry := component.NewPayloadRegistry()
	err := registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testRegistryFactory,
		Builder:     testRegistryBuilder,
		Domain:      "test",
		Category:    "registry",
		Version:     "v1",
		Description: "Test registry payload",
	})
	require.NoError(t, err)

	err = registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testAnotherFactory,
		Builder:     testAnotherBuilder,
		Domain:      "test",
		Category:    "another",
		Version:     "v1",
		Description: "Another test payload",
	})
	require.NoError(t, err)

	tests := []struct {
		name             string
		workflow         *wfschema.Definition
		expectedWarnings int
		warningContains  []string
	}{
		{
			name: "no type declarations - no warnings",
			workflow: &wfschema.Definition{
				ID:      "test-no-types",
				Name:    "Test No Types",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name: "step1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "registered input_type - no warnings",
			workflow: &wfschema.Definition{
				ID:      "test-registered-input",
				Name:    "Test Registered Input",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:      "step1",
						InputType: "test.registry.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "registered output_type - no warnings",
			workflow: &wfschema.Definition{
				ID:      "test-registered-output",
				Name:    "Test Registered Output",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:       "step1",
						OutputType: "test.registry.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "both registered types - no warnings",
			workflow: &wfschema.Definition{
				ID:      "test-both-registered",
				Name:    "Test Both Registered",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:       "step1",
						InputType:  "test.registry.v1",
						OutputType: "test.another.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "unregistered input_type - warning",
			workflow: &wfschema.Definition{
				ID:      "test-unregistered-input",
				Name:    "Test Unregistered Input",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:      "step1",
						InputType: "unknown.type.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 1,
			warningContains:  []string{"step1", "input_type", "unknown.type.v1", "not registered"},
		},
		{
			name: "unregistered output_type - warning",
			workflow: &wfschema.Definition{
				ID:      "test-unregistered-output",
				Name:    "Test Unregistered Output",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:       "step1",
						OutputType: "missing.payload.v2",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 1,
			warningContains:  []string{"step1", "output_type", "missing.payload.v2", "not registered"},
		},
		{
			name: "multiple unregistered types - multiple warnings",
			workflow: &wfschema.Definition{
				ID:      "test-multiple-unregistered",
				Name:    "Test Multiple Unregistered",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:       "step1",
						InputType:  "unknown.input.v1",
						OutputType: "unknown.output.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
					{
						Name:      "step2",
						InputType: "another.missing.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output2",
						},
					},
				},
			},
			expectedWarnings: 3,
			warningContains: []string{
				"step1",
				"unknown.input.v1",
				"unknown.output.v1",
				"another.missing.v1",
			},
		},
		{
			name: "mixed registered and unregistered - warnings only for unregistered",
			workflow: &wfschema.Definition{
				ID:      "test-mixed",
				Name:    "Test Mixed",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:       "step1",
						InputType:  "test.registry.v1", // registered - OK
						OutputType: "unknown.type.v1",  // unregistered - warning
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
					{
						Name:      "step2",
						InputType: "test.another.v1", // registered - OK
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output2",
						},
					},
				},
			},
			expectedWarnings: 1,
			warningContains:  []string{"step1", "output_type", "unknown.type.v1"},
		},
		{
			name: "parallel step with nested types",
			workflow: &wfschema.Definition{
				ID:      "test-parallel-nested",
				Name:    "Test Parallel Nested",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name: "parallel1",
						Type: "parallel",
						Steps: []wfschema.StepDef{
							{
								Name:      "nested1",
								InputType: "unknown.nested.v1",
								Action: wfschema.ActionDef{
									Type:    "publish",
									Subject: "test.nested1",
								},
							},
							{
								Name:       "nested2",
								OutputType: "test.registry.v1", // registered - OK
								Action: wfschema.ActionDef{
									Type:    "publish",
									Subject: "test.nested2",
								},
							},
						},
					},
				},
			},
			expectedWarnings: 1,
			warningContains:  []string{"nested1", "input_type", "unknown.nested.v1"},
		},
		{
			name: "invalid type format - caught by validation",
			workflow: &wfschema.Definition{
				ID:      "test-invalid-format",
				Name:    "Test Invalid Format",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.trigger"},
				Steps: []wfschema.StepDef{
					{
						Name:      "step1",
						InputType: "invalid-format", // Missing dots
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectedWarnings: 1,
			warningContains:  []string{"step1", "invalid", "input_type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := validateWorkflowTypes(tt.workflow, registry)

			// Check warning count
			assert.Equal(t, tt.expectedWarnings, len(warnings),
				"Expected %d warnings, got %d: %v",
				tt.expectedWarnings, len(warnings), warnings)

			// Check warning contents
			if tt.expectedWarnings > 0 {
				warningsText := ""
				for _, w := range warnings {
					warningsText += w + " "
				}

				for _, expectedText := range tt.warningContains {
					assert.Contains(t, warningsText, expectedText,
						"Warning should contain %q. Warnings: %v", expectedText, warnings)
				}
			}
		})
	}
}

func TestRegistry_ValidateTypesOnRegister(t *testing.T) {
	// This test verifies that Registry.Register() validates workflow types
	// and logs warnings for unregistered types without failing registration.

	// Register a test payload type in the global registry
	err := component.GlobalPayloadRegistry().RegisterPayload(&component.PayloadRegistration{
		Factory:     testRegistryFactory,
		Builder:     testRegistryBuilder,
		Domain:      "test",
		Category:    "registered",
		Version:     "v1",
		Description: "Test registered payload",
	})
	require.NoError(t, err)

	// Clean up after test
	defer func() {
		// Note: In real code, we can't unregister from global registry,
		// but this is OK for tests as the registration persists for the test process
	}()

	tests := []struct {
		name             string
		workflow         *wfschema.Definition
		expectValidation bool // Should Register succeed?
		expectedLogWarn  bool // Should warnings be logged?
		warningContains  string
	}{
		{
			name: "workflow with registered types - no warnings",
			workflow: &wfschema.Definition{
				ID:      "test-valid-types",
				Name:    "Test Valid Types",
				Version: "1.0.0",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.valid"},
				Steps: []wfschema.StepDef{
					{
						Name:      "step1",
						InputType: "test.registered.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectValidation: true,
			expectedLogWarn:  false,
		},
		{
			name: "workflow with unregistered types - warnings logged but registration succeeds",
			workflow: &wfschema.Definition{
				ID:      "test-unregistered-types",
				Name:    "Test Unregistered Types",
				Version: "1.0.0",
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.unregistered"},
				Steps: []wfschema.StepDef{
					{
						Name:       "step1",
						InputType:  "missing.input.v1",
						OutputType: "missing.output.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectValidation: true, // Registration still succeeds
			expectedLogWarn:  true,
			warningContains:  "missing.input.v1",
		},
		{
			name: "workflow with invalid schema - fails validation",
			workflow: &wfschema.Definition{
				ID:      "test-invalid-schema",
				Name:    "", // Invalid: empty name
				Enabled: true,
				Trigger: wfschema.TriggerDef{Subject: "test.invalid"},
				Steps: []wfschema.StepDef{
					{
						Name: "step1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.output",
						},
					},
				},
			},
			expectValidation: false, // Schema validation should fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The actual validation happens inside Register() which logs warnings.
			// Since we can't easily capture log output in unit tests without
			// complex setup, we verify the behavior indirectly:
			// 1. Invalid schemas fail validation (tested in schema_test.go)
			// 2. Type warnings don't block registration (tested via validateWorkflowTypes)
			// 3. Registration succeeds for valid workflows regardless of type registration

			// We mainly test that type validation doesn't break registration
			err := tt.workflow.Validate()
			if tt.expectValidation {
				assert.NoError(t, err, "Workflow should pass schema validation")

				// Verify that validateWorkflowTypes produces expected warnings
				warnings := validateWorkflowTypes(tt.workflow, component.GlobalPayloadRegistry())
				if tt.expectedLogWarn {
					assert.NotEmpty(t, warnings, "Expected warnings for unregistered types")
					if tt.warningContains != "" {
						warningsText := ""
						for _, w := range warnings {
							warningsText += w + " "
						}
						assert.Contains(t, warningsText, tt.warningContains)
					}
				} else {
					assert.Empty(t, warnings, "Expected no warnings for registered types")
				}
			} else {
				assert.Error(t, err, "Workflow should fail schema validation")
			}
		})
	}
}

func TestValidateStepTypes(t *testing.T) {
	// Create a test registry with registered types
	registry := component.NewPayloadRegistry()
	err := registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testRegistryFactory,
		Builder:     testRegistryBuilder,
		Domain:      "test",
		Category:    "registry",
		Version:     "v1",
		Description: "Test registry payload",
	})
	require.NoError(t, err)

	tests := []struct {
		name             string
		step             *wfschema.StepDef
		expectedWarnings int
		warningContains  string
	}{
		{
			name: "no types declared",
			step: &wfschema.StepDef{
				Name: "basic-step",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "valid input type",
			step: &wfschema.StepDef{
				Name:      "valid-input",
				InputType: "test.registry.v1",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "invalid input type",
			step: &wfschema.StepDef{
				Name:      "invalid-input",
				InputType: "missing.type.v1",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 1,
			warningContains:  "missing.type.v1",
		},
		{
			name: "valid output type",
			step: &wfschema.StepDef{
				Name:       "valid-output",
				OutputType: "test.registry.v1",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "invalid output type",
			step: &wfschema.StepDef{
				Name:       "invalid-output",
				OutputType: "unregistered.result.v1",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 1,
			warningContains:  "unregistered.result.v1",
		},
		{
			name: "both valid types",
			step: &wfschema.StepDef{
				Name:       "both-valid",
				InputType:  "test.registry.v1",
				OutputType: "test.registry.v1",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 0,
		},
		{
			name: "both invalid types",
			step: &wfschema.StepDef{
				Name:       "both-invalid",
				InputType:  "missing.input.v1",
				OutputType: "missing.output.v1",
				Action: wfschema.ActionDef{
					Type:    "publish",
					Subject: "test.output",
				},
			},
			expectedWarnings: 2,
		},
		{
			name: "parallel step with nested invalid types",
			step: &wfschema.StepDef{
				Name: "parallel-parent",
				Type: "parallel",
				Steps: []wfschema.StepDef{
					{
						Name:      "nested-invalid",
						InputType: "nested.missing.v1",
						Action: wfschema.ActionDef{
							Type:    "publish",
							Subject: "test.nested",
						},
					},
				},
			},
			expectedWarnings: 1,
			warningContains:  "nested.missing.v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := validateStepTypes(tt.step, registry)

			assert.Equal(t, tt.expectedWarnings, len(warnings),
				"Expected %d warnings, got %d: %v",
				tt.expectedWarnings, len(warnings), warnings)

			if tt.warningContains != "" {
				warningsText := ""
				for _, w := range warnings {
					warningsText += w + " "
				}
				assert.Contains(t, warningsText, tt.warningContains)
			}
		})
	}
}
