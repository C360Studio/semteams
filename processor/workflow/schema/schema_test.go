package schema

import (
	"testing"
)

// TestValidateInterfaceString tests the interface string validation helper function.
// This validates the format of interface type strings used in InputRef.Interface and OutputDef.Interface.
func TestValidateInterfaceString(t *testing.T) {
	tests := []struct {
		name    string
		typeStr string
		wantErr bool
	}{
		{
			name:    "empty string is valid",
			typeStr: "",
			wantErr: false,
		},
		{
			name:    "valid interface string",
			typeStr: "agentic.task.v1",
			wantErr: false,
		},
		{
			name:    "missing parts",
			typeStr: "agentic.task",
			wantErr: true,
		},
		{
			name:    "single part",
			typeStr: "agentic",
			wantErr: true,
		},
		{
			name:    "empty domain",
			typeStr: ".task.v1",
			wantErr: true,
		},
		{
			name:    "empty category",
			typeStr: "agentic..v1",
			wantErr: true,
		},
		{
			name:    "empty version",
			typeStr: "agentic.task.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInterfaceString(tt.typeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateInterfaceString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestInputRef_Validate tests InputRef validation
func TestInputRef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   InputRef
		wantErr bool
	}{
		{
			name: "valid input with from and interface",
			input: InputRef{
				From:      "trigger.payload.data",
				Interface: "agentic.task.v1",
			},
			wantErr: false,
		},
		{
			name: "valid input with from only",
			input: InputRef{
				From: "steps.previous.output",
			},
			wantErr: false,
		},
		{
			name: "valid input with template only",
			input: InputRef{
				Template: "Hello ${trigger.payload.name}",
			},
			wantErr: false,
		},
		{
			name: "valid input with template and interface",
			input: InputRef{
				Template:  "Process: ${steps.fetch.result}",
				Interface: "agentic.task.v1",
			},
			wantErr: false,
		},
		{
			name: "neither from nor template is invalid",
			input: InputRef{
				Interface: "agentic.task.v1",
			},
			wantErr: true,
		},
		{
			name: "both from and template is invalid",
			input: InputRef{
				From:     "trigger.payload.data",
				Template: "Hello ${name}",
			},
			wantErr: true,
		},
		{
			name: "invalid interface format with from",
			input: InputRef{
				From:      "trigger.payload",
				Interface: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid interface format with template",
			input: InputRef{
				Template:  "Hello",
				Interface: "invalid",
			},
			wantErr: true,
		},
		// Template syntax validation tests
		{
			name: "template with multiple interpolations",
			input: InputRef{
				Template: "Hello ${trigger.payload.first} ${trigger.payload.last}",
			},
			wantErr: false,
		},
		{
			name: "template with no interpolation (static string)",
			input: InputRef{
				Template: "Hello World",
			},
			wantErr: false,
		},
		{
			name: "template with unclosed interpolation",
			input: InputRef{
				Template: "Hello ${trigger.payload.name",
			},
			wantErr: true,
		},
		{
			name: "template with unbalanced braces",
			input: InputRef{
				Template: "Hello ${trigger.payload.name}}",
			},
			wantErr: false, // Extra } after valid interpolation is OK (not inside ${})
		},
		{
			name: "template with nested braces in path",
			input: InputRef{
				Template: "Value: ${steps.fetch.output.data}",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("InputRef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateTemplateSyntax tests the template syntax validation helper function.
func TestValidateTemplateSyntax(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{
			name:     "empty template",
			template: "",
			wantErr:  false,
		},
		{
			name:     "static string no interpolation",
			template: "Hello World",
			wantErr:  false,
		},
		{
			name:     "single interpolation",
			template: "Hello ${name}",
			wantErr:  false,
		},
		{
			name:     "multiple interpolations",
			template: "${first} ${last}",
			wantErr:  false,
		},
		{
			name:     "interpolation only",
			template: "${trigger.payload.data}",
			wantErr:  false,
		},
		{
			name:     "unclosed interpolation",
			template: "Hello ${name",
			wantErr:  true,
		},
		{
			name:     "unclosed at end",
			template: "Value: ${",
			wantErr:  true,
		},
		{
			name:     "multiple unclosed",
			template: "${foo ${bar",
			wantErr:  true,
		},
		{
			name:     "dollar without brace is ok",
			template: "Price: $100",
			wantErr:  false,
		},
		{
			name:     "closing brace without open is ok",
			template: "JSON: }",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateSyntax(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTemplateSyntax() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestOutputDef_Validate tests OutputDef validation
func TestOutputDef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		output  OutputDef
		wantErr bool
	}{
		{
			name: "valid output with interface",
			output: OutputDef{
				Interface: "agentic.response.v1",
			},
			wantErr: false,
		},
		{
			name:    "valid output without interface",
			output:  OutputDef{},
			wantErr: false,
		},
		{
			name: "invalid interface format",
			output: OutputDef{
				Interface: "invalid.format",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.output.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("OutputDef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
