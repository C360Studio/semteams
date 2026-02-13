package weatherstation

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

func TestProcessor_Process_JSONTransformation(t *testing.T) {
	p := NewProcessor(Config{OrgID: "acme", Platform: "weather"})

	inputJSON := `{
		"station_id": "ws-001",
		"temperature": 22.5,
		"humidity": 65.0,
		"condition": "sunny",
		"city": "San Francisco"
	}`

	var input map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		t.Fatalf("failed to unmarshal test input: %v", err)
	}

	result, err := p.Process(input)
	if err != nil {
		t.Fatalf("Process() unexpected error: %v", err)
	}

	// Verify result implements Graphable
	var _ graph.Graphable = result

	// Verify EntityID is valid 6-part format
	entityID := result.EntityID()
	if !message.IsValidEntityID(entityID) {
		t.Errorf("EntityID() = %q is not valid 6-part format", entityID)
	}

	// Verify Triples returns meaningful data
	triples := result.Triples()
	if len(triples) < 3 {
		t.Errorf("Triples() returned %d triples, want at least 3", len(triples))
	}

	// Verify specific values
	if result.Temperature != 22.5 {
		t.Errorf("Temperature = %v, want 22.5", result.Temperature)
	}
	if result.Condition != "sunny" {
		t.Errorf("Condition = %q, want sunny", result.Condition)
	}
}

func TestProcessor_Process_MissingField(t *testing.T) {
	p := NewProcessor(Config{OrgID: "acme", Platform: "weather"})

	// Missing condition
	input := map[string]any{
		"station_id":  "ws-001",
		"temperature": 22.5,
		"humidity":    65.0,
	}

	_, err := p.Process(input)
	if err == nil {
		t.Error("Process() expected error for missing condition, got nil")
	}
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  Config{OrgID: "acme", Platform: "weather"},
			wantErr: false,
		},
		{
			name:    "missing OrgID",
			config:  Config{Platform: "weather"},
			wantErr: true,
		},
		{
			name:    "missing Platform",
			config:  Config{OrgID: "acme"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
