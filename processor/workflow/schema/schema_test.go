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
			name: "valid input with interface",
			input: InputRef{
				From:      "trigger.payload.data",
				Interface: "agentic.task.v1",
			},
			wantErr: false,
		},
		{
			name: "valid input without interface",
			input: InputRef{
				From: "steps.previous.output",
			},
			wantErr: false,
		},
		{
			name: "empty from is invalid",
			input: InputRef{
				From:      "",
				Interface: "agentic.task.v1",
			},
			wantErr: true,
		},
		{
			name: "invalid interface format",
			input: InputRef{
				From:      "trigger.payload",
				Interface: "invalid",
			},
			wantErr: true,
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
