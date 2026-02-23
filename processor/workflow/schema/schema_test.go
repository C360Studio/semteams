package schema

import (
	"encoding/json"
	"testing"
)

// TestStepDef_ValidateTypeStrings tests InputType and OutputType validation
func TestStepDef_ValidateTypeStrings(t *testing.T) {
	tests := []struct {
		name      string
		inputType string
		wantErr   bool
	}{
		{
			name:      "empty type string is valid",
			inputType: "",
			wantErr:   false,
		},
		{
			name:      "valid type string",
			inputType: "agentic.task.v1",
			wantErr:   false,
		},
		{
			name:      "valid type with complex domain",
			inputType: "workflow.orchestration.v2",
			wantErr:   false,
		},
		{
			name:      "missing category",
			inputType: "agentic.v1",
			wantErr:   true,
		},
		{
			name:      "missing version",
			inputType: "agentic.task",
			wantErr:   true,
		},
		{
			name:      "only domain",
			inputType: "agentic",
			wantErr:   true,
		},
		{
			name:      "empty domain",
			inputType: ".task.v1",
			wantErr:   true,
		},
		{
			name:      "empty category",
			inputType: "agentic..v1",
			wantErr:   true,
		},
		{
			name:      "empty version",
			inputType: "agentic.task.",
			wantErr:   true,
		},
		{
			name:      "too many parts",
			inputType: "agentic.task.v1.extra",
			wantErr:   false, // SplitN(3) will treat "v1.extra" as version
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &StepDef{
				Name:      "test-step",
				InputType: tt.inputType,
				Action: ActionDef{
					Type:    "publish",
					Subject: "test.subject",
				},
			}

			err := step.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("StepDef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStepDef_ValidateOutputType tests OutputType validation separately
func TestStepDef_ValidateOutputType(t *testing.T) {
	tests := []struct {
		name       string
		outputType string
		wantErr    bool
	}{
		{
			name:       "empty output type is valid",
			outputType: "",
			wantErr:    false,
		},
		{
			name:       "valid output type",
			outputType: "agentic.response.v1",
			wantErr:    false,
		},
		{
			name:       "invalid output type format",
			outputType: "invalid",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &StepDef{
				Name:       "test-step",
				OutputType: tt.outputType,
				Action: ActionDef{
					Type:    "publish",
					Subject: "test.subject",
				},
			}

			err := step.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("StepDef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestActionDef_PayloadExclusivity tests that Payload and PayloadMapping are mutually exclusive
func TestActionDef_PayloadExclusivity(t *testing.T) {
	tests := []struct {
		name           string
		payload        json.RawMessage
		payloadMapping map[string]string
		wantErr        bool
	}{
		{
			name:           "no payload fields is valid",
			payload:        nil,
			payloadMapping: nil,
			wantErr:        false,
		},
		{
			name:           "only payload is valid",
			payload:        json.RawMessage(`{"key":"value"}`),
			payloadMapping: nil,
			wantErr:        false,
		},
		{
			name:    "only payload_mapping is valid",
			payload: nil,
			payloadMapping: map[string]string{
				"task_id": "trigger.task_id",
			},
			wantErr: false,
		},
		{
			name:    "both payload and payload_mapping is invalid",
			payload: json.RawMessage(`{"key":"value"}`),
			payloadMapping: map[string]string{
				"task_id": "trigger.task_id",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &ActionDef{
				Type:           "publish",
				Subject:        "test.subject",
				Payload:        tt.payload,
				PayloadMapping: tt.payloadMapping,
			}

			err := action.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ActionDef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestActionDef_PayloadMapping tests PayloadMapping field validation
func TestActionDef_PayloadMapping(t *testing.T) {
	tests := []struct {
		name           string
		payloadMapping map[string]string
		wantErr        bool
	}{
		{
			name:           "empty mapping is valid",
			payloadMapping: map[string]string{},
			wantErr:        false,
		},
		{
			name: "valid mapping",
			payloadMapping: map[string]string{
				"task_id": "trigger.task_id",
				"model":   "steps.config.output.model",
			},
			wantErr: false,
		},
		{
			name: "empty key is invalid",
			payloadMapping: map[string]string{
				"": "trigger.task_id",
			},
			wantErr: true,
		},
		{
			name: "empty value is invalid",
			payloadMapping: map[string]string{
				"task_id": "",
			},
			wantErr: true,
		},
		{
			name: "whitespace key is invalid",
			payloadMapping: map[string]string{
				"   ": "trigger.task_id",
			},
			wantErr: true,
		},
		{
			name: "whitespace value is invalid",
			payloadMapping: map[string]string{
				"task_id": "   ",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &ActionDef{
				Type:           "publish",
				Subject:        "test.subject",
				PayloadMapping: tt.payloadMapping,
			}

			err := action.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ActionDef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStepDef_JSONSerialization tests that new fields serialize correctly
func TestStepDef_JSONSerialization(t *testing.T) {
	step := &StepDef{
		Name:       "test-step",
		InputType:  "agentic.task.v1",
		OutputType: "agentic.response.v1",
		Action: ActionDef{
			Type:    "publish",
			Subject: "test.subject",
			PayloadMapping: map[string]string{
				"task_id": "trigger.task_id",
			},
			PassThrough: []string{"session_id", "user_id"},
		},
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Failed to marshal StepDef: %v", err)
	}

	var unmarshaled StepDef
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal StepDef: %v", err)
	}

	if unmarshaled.InputType != step.InputType {
		t.Errorf("InputType mismatch: got %s, want %s", unmarshaled.InputType, step.InputType)
	}
	if unmarshaled.OutputType != step.OutputType {
		t.Errorf("OutputType mismatch: got %s, want %s", unmarshaled.OutputType, step.OutputType)
	}
	if len(unmarshaled.Action.PayloadMapping) != len(step.Action.PayloadMapping) {
		t.Errorf("PayloadMapping length mismatch: got %d, want %d", len(unmarshaled.Action.PayloadMapping), len(step.Action.PayloadMapping))
	}
	if len(unmarshaled.Action.PassThrough) != len(step.Action.PassThrough) {
		t.Errorf("PassThrough length mismatch: got %d, want %d", len(unmarshaled.Action.PassThrough), len(step.Action.PassThrough))
	}
}

// TestValidateTypeString tests the helper function directly
func TestValidateTypeString(t *testing.T) {
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
			name:    "valid type string",
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
			err := validateTypeString(tt.typeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTypeString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
