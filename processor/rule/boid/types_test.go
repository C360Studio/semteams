package boid

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAgentPosition_MarshalUnmarshal(t *testing.T) {
	pos := &AgentPosition{
		LoopID:          "loop-123",
		Role:            "general",
		FocusEntities:   []string{"entity-1", "entity-2"},
		TraversalVector: []string{"relation.type.a", "relation.type.b"},
		Velocity:        0.75,
		Iteration:       5,
		LastUpdate:      time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(pos)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled AgentPosition
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.LoopID != pos.LoopID {
		t.Errorf("LoopID mismatch: got %s, want %s", unmarshaled.LoopID, pos.LoopID)
	}
	if unmarshaled.Role != pos.Role {
		t.Errorf("Role mismatch: got %s, want %s", unmarshaled.Role, pos.Role)
	}
	if len(unmarshaled.FocusEntities) != len(pos.FocusEntities) {
		t.Errorf("FocusEntities length mismatch: got %d, want %d", len(unmarshaled.FocusEntities), len(pos.FocusEntities))
	}
	if unmarshaled.Velocity != pos.Velocity {
		t.Errorf("Velocity mismatch: got %f, want %f", unmarshaled.Velocity, pos.Velocity)
	}
}

func TestAgentPosition_Schema(t *testing.T) {
	pos := &AgentPosition{}
	schema := pos.Schema()

	if schema.Domain != Domain {
		t.Errorf("Domain mismatch: got %s, want %s", schema.Domain, Domain)
	}
	if schema.Category != CategoryPosition {
		t.Errorf("Category mismatch: got %s, want %s", schema.Category, CategoryPosition)
	}
	if schema.Version != SchemaVersion {
		t.Errorf("Version mismatch: got %s, want %s", schema.Version, SchemaVersion)
	}
}

func TestSteeringSignal_MarshalUnmarshal(t *testing.T) {
	signal := &SteeringSignal{
		LoopID:        "loop-456",
		SignalType:    SignalTypeSeparation,
		AvoidEntities: []string{"avoid-1", "avoid-2"},
		Strength:      0.8,
		SourceRule:    "test-rule",
		Timestamp:     time.Now().Truncate(time.Second),
		Metadata:      map[string]any{"key": "value"},
	}

	data, err := json.Marshal(signal)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled SteeringSignal
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.LoopID != signal.LoopID {
		t.Errorf("LoopID mismatch: got %s, want %s", unmarshaled.LoopID, signal.LoopID)
	}
	if unmarshaled.SignalType != signal.SignalType {
		t.Errorf("SignalType mismatch: got %s, want %s", unmarshaled.SignalType, signal.SignalType)
	}
	if len(unmarshaled.AvoidEntities) != len(signal.AvoidEntities) {
		t.Errorf("AvoidEntities length mismatch: got %d, want %d", len(unmarshaled.AvoidEntities), len(signal.AvoidEntities))
	}
}

func TestConfig_GetSeparationThreshold(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		role     string
		expected int
	}{
		{
			name: "role threshold exists",
			config: &Config{
				RoleThresholds: map[string]int{
					"general":   2,
					"architect": 3,
				},
			},
			role:     "general",
			expected: 2,
		},
		{
			name: "role threshold missing, use default field",
			config: &Config{
				RoleThresholds:      map[string]int{"other": 5},
				SeparationThreshold: 4,
			},
			role:     "general",
			expected: 4,
		},
		{
			name:     "no thresholds, use constant default",
			config:   &Config{},
			role:     "general",
			expected: DefaultSeparationThreshold,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetSeparationThreshold(tt.role)
			if result != tt.expected {
				t.Errorf("got %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	metadata := map[string]any{
		"boid_rule":         "separation",
		"role_filter":       "general",
		"steering_strength": 0.9,
		"role_thresholds": map[string]any{
			"general": 2,
		},
	}

	config, err := ParseConfig(metadata)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.BoidRule != "separation" {
		t.Errorf("BoidRule mismatch: got %s, want separation", config.BoidRule)
	}
	if config.RoleFilter != "general" {
		t.Errorf("RoleFilter mismatch: got %s, want general", config.RoleFilter)
	}
	if config.SteeringStrength != 0.9 {
		t.Errorf("SteeringStrength mismatch: got %f, want 0.9", config.SteeringStrength)
	}
}

func TestParseConfig_Defaults(t *testing.T) {
	config, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.SeparationThreshold != DefaultSeparationThreshold {
		t.Errorf("SeparationThreshold default: got %d, want %d", config.SeparationThreshold, DefaultSeparationThreshold)
	}
	if config.SteeringStrength != DefaultSteeringStrength {
		t.Errorf("SteeringStrength default: got %f, want %f", config.SteeringStrength, DefaultSteeringStrength)
	}
	if config.AlignmentWindow != DefaultAlignmentWindow {
		t.Errorf("AlignmentWindow default: got %d, want %d", config.AlignmentWindow, DefaultAlignmentWindow)
	}
	if config.CentralityWeight != DefaultCentralityWeight {
		t.Errorf("CentralityWeight default: got %f, want %f", config.CentralityWeight, DefaultCentralityWeight)
	}
}
