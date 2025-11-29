package iotsensor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

// TestProcessor_Process_JSONTransformation verifies that the processor correctly
// transforms incoming JSON into Graphable payloads.
func TestProcessor_Process_JSONTransformation(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		inputJSON string
		wantType  string
		wantValue float64
		wantErr   bool
	}{
		{
			name: "temperature reading",
			config: Config{
				OrgID:    "acme",
				Platform: "logistics",
			},
			inputJSON: `{
				"device_id": "sensor-042",
				"type": "temperature",
				"reading": 23.5,
				"unit": "celsius",
				"location": "warehouse-7",
				"timestamp": "2025-11-26T10:30:00Z"
			}`,
			wantType:  "temperature",
			wantValue: 23.5,
			wantErr:   false,
		},
		{
			name: "humidity reading",
			config: Config{
				OrgID:    "acme",
				Platform: "facilities",
			},
			inputJSON: `{
				"device_id": "hum-001",
				"type": "humidity",
				"reading": 65.0,
				"unit": "percent",
				"location": "office-3",
				"timestamp": "2025-11-26T11:00:00Z"
			}`,
			wantType:  "humidity",
			wantValue: 65.0,
			wantErr:   false,
		},
		{
			name: "missing device_id",
			config: Config{
				OrgID:    "acme",
				Platform: "logistics",
			},
			inputJSON: `{
				"type": "temperature",
				"reading": 23.5,
				"unit": "celsius",
				"location": "warehouse-7",
				"timestamp": "2025-11-26T10:30:00Z"
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProcessor(tt.config)

			var input map[string]any
			if err := json.Unmarshal([]byte(tt.inputJSON), &input); err != nil {
				t.Fatalf("failed to unmarshal test input: %v", err)
			}

			result, err := p.Process(input)

			if tt.wantErr {
				if err == nil {
					t.Error("Process() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Process() unexpected error: %v", err)
			}

			// Verify result implements Graphable (compile-time check)
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

			// Verify the type and value
			if result.SensorType != tt.wantType {
				t.Errorf("SensorType = %q, want %q", result.SensorType, tt.wantType)
			}
			if result.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", result.Value, tt.wantValue)
			}
		})
	}
}

// TestProcessor_Process_ContextFields verifies that processor applies config context.
func TestProcessor_Process_ContextFields(t *testing.T) {
	config := Config{
		OrgID:    "testorg",
		Platform: "testplatform",
	}

	p := NewProcessor(config)

	input := map[string]any{
		"device_id": "sensor-001",
		"type":      "pressure",
		"reading":   1013.25,
		"unit":      "hpa",
		"location":  "lab-1",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	sr, err := p.Process(input)
	if err != nil {
		t.Fatalf("Process() unexpected error: %v", err)
	}

	// Verify context fields from config are applied
	if sr.OrgID != config.OrgID {
		t.Errorf("OrgID = %q, want %q", sr.OrgID, config.OrgID)
	}
	if sr.Platform != config.Platform {
		t.Errorf("Platform = %q, want %q", sr.Platform, config.Platform)
	}

	// Verify EntityID includes org and platform
	entityID := sr.EntityID()
	if entityID[:len("testorg.testplatform")] != "testorg.testplatform" {
		t.Errorf("EntityID() = %q, want to start with %q", entityID, "testorg.testplatform")
	}
}

// TestProcessor_Process_ZoneEntityID verifies processor computes ZoneEntityID correctly.
func TestProcessor_Process_ZoneEntityID(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		wantZoneID string
	}{
		{
			name: "default zone type (area)",
			input: map[string]any{
				"device_id": "sensor-001",
				"type":      "temperature",
				"reading":   20.0,
				"unit":      "celsius",
				"location":  "warehouse-7",
			},
			wantZoneID: "acme.logistics.facility.zone.area.warehouse-7",
		},
		{
			name: "explicit zone type",
			input: map[string]any{
				"device_id": "sensor-002",
				"type":      "humidity",
				"reading":   50.0,
				"unit":      "percent",
				"location":  "building-a",
				"zone_type": "building",
			},
			wantZoneID: "acme.logistics.facility.zone.building.building-a",
		},
	}

	p := NewProcessor(Config{OrgID: "acme", Platform: "logistics"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr, err := p.Process(tt.input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}
			if sr.ZoneEntityID != tt.wantZoneID {
				t.Errorf("ZoneEntityID = %q, want %q", sr.ZoneEntityID, tt.wantZoneID)
			}
			// Verify ZoneEntityID is valid 6-part format
			if !message.IsValidEntityID(sr.ZoneEntityID) {
				t.Errorf("ZoneEntityID %q is not valid 6-part format", sr.ZoneEntityID)
			}
		})
	}
}

// TestConfig_Validation verifies Config validation.
func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				OrgID:    "acme",
				Platform: "logistics",
			},
			wantErr: false,
		},
		{
			name: "missing OrgID",
			config: Config{
				Platform: "logistics",
			},
			wantErr: true,
		},
		{
			name: "missing Platform",
			config: Config{
				OrgID: "acme",
			},
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
