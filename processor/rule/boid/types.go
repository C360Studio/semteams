package boid

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
)

// Constants for the boid domain and message types.
const (
	// Domain identifies boid-related messages.
	Domain = "boid"

	// CategoryPosition is the category for agent position updates.
	CategoryPosition = "position"

	// CategorySignal is the category for steering signals.
	CategorySignal = "signal"

	// SchemaVersion is the current schema version.
	SchemaVersion = "v1"

	// KVBucketAgentPositions is the KV bucket name for agent positions.
	KVBucketAgentPositions = "AGENT_POSITIONS"
)

// Boid rule type identifiers.
const (
	// RuleTypeSeparation identifies separation rules.
	RuleTypeSeparation = "separation"

	// RuleTypeCohesion identifies cohesion rules.
	RuleTypeCohesion = "cohesion"

	// RuleTypeAlignment identifies alignment rules.
	RuleTypeAlignment = "alignment"
)

// Signal type identifiers.
const (
	SignalTypeSeparation = "separation"
	SignalTypeCohesion   = "cohesion"
	SignalTypeAlignment  = "alignment"
)

// Default configuration values.
const (
	// DefaultSeparationThreshold is the default k-hop distance for separation rules.
	DefaultSeparationThreshold = 2

	// DefaultSteeringStrength is the default influence strength (0.0-1.0).
	DefaultSteeringStrength = 0.5

	// DefaultAlignmentWindow is the default number of recent traversals to consider.
	DefaultAlignmentWindow = 5

	// DefaultCentralityWeight is the default weight for centrality attraction.
	DefaultCentralityWeight = 0.7
)

// AgentPosition tracks an agent's current focus and traversal direction in the graph.
// This is stored in the AGENT_POSITIONS KV bucket, keyed by LoopID.
type AgentPosition struct {
	// LoopID is the unique identifier for the agentic loop.
	LoopID string `json:"loop_id"`

	// Role is the agent's role (general, architect, editor, reviewer).
	Role string `json:"role"`

	// FocusEntities are the entity IDs the agent is currently working on.
	FocusEntities []string `json:"focus_entities"`

	// TraversalVector contains the relationship predicates being followed.
	// This represents the agent's "heading" in graph space.
	TraversalVector []string `json:"traversal_vector"`

	// Velocity represents the rate of position change (0.0-1.0).
	// Higher values indicate more active exploration.
	Velocity float64 `json:"velocity"`

	// Iteration is the current loop iteration number.
	Iteration int `json:"iteration"`

	// LastUpdate is when this position was last updated.
	LastUpdate time.Time `json:"last_update"`
}

// Schema returns the message type schema for AgentPosition.
func (p *AgentPosition) Schema() message.Type {
	return message.Type{
		Domain:   Domain,
		Category: CategoryPosition,
		Version:  SchemaVersion,
	}
}

// MarshalJSON implements json.Marshaler.
func (p *AgentPosition) MarshalJSON() ([]byte, error) {
	type Alias AgentPosition
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *AgentPosition) UnmarshalJSON(data []byte) error {
	type Alias AgentPosition
	return json.Unmarshal(data, (*Alias)(p))
}

// SteeringSignal is published when a Boid rule fires.
// It provides guidance to the agentic-loop for adjusting agent behavior.
type SteeringSignal struct {
	// LoopID identifies which agent loop should receive this signal.
	LoopID string `json:"loop_id"`

	// SignalType identifies the rule type: separation, cohesion, or alignment.
	SignalType string `json:"signal_type"`

	// SuggestedFocus contains entity IDs to prioritize (cohesion signals).
	SuggestedFocus []string `json:"suggested_focus,omitempty"`

	// AvoidEntities contains entity IDs to deprioritize (separation signals).
	AvoidEntities []string `json:"avoid_entities,omitempty"`

	// AlignWith contains predicate patterns to favor (alignment signals).
	AlignWith []string `json:"align_with,omitempty"`

	// Strength indicates how strongly to apply the signal (0.0-1.0).
	Strength float64 `json:"strength"`

	// SourceRule identifies which rule generated this signal.
	SourceRule string `json:"source_rule,omitempty"`

	// Timestamp is when this signal was generated.
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional context about the signal.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Schema returns the message type schema for SteeringSignal.
func (s *SteeringSignal) Schema() message.Type {
	return message.Type{
		Domain:   Domain,
		Category: CategorySignal,
		Version:  SchemaVersion,
	}
}

// Validate checks that the SteeringSignal has required fields.
func (s *SteeringSignal) Validate() error {
	if s.LoopID == "" {
		return fmt.Errorf("loop_id required")
	}
	if s.SignalType == "" {
		return fmt.Errorf("signal_type required")
	}
	if s.SignalType != SignalTypeSeparation &&
		s.SignalType != SignalTypeCohesion &&
		s.SignalType != SignalTypeAlignment {
		return fmt.Errorf("signal_type must be one of: %s, %s, %s",
			SignalTypeSeparation, SignalTypeCohesion, SignalTypeAlignment)
	}
	if s.Strength < 0 || s.Strength > 1 {
		return fmt.Errorf("strength must be between 0.0 and 1.0")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s *SteeringSignal) MarshalJSON() ([]byte, error) {
	type Alias SteeringSignal
	return json.Marshal((*Alias)(s))
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *SteeringSignal) UnmarshalJSON(data []byte) error {
	type Alias SteeringSignal
	return json.Unmarshal(data, (*Alias)(s))
}

// Config contains configuration extracted from rule metadata.
type Config struct {
	// BoidRule specifies the rule type: separation, cohesion, or alignment.
	BoidRule string `json:"boid_rule"`

	// RoleFilter limits the rule to agents with this role (empty = all roles).
	RoleFilter string `json:"role_filter,omitempty"`

	// RoleThresholds maps role names to k-hop separation thresholds.
	// Used by separation rules to allow different thresholds per role.
	RoleThresholds map[string]int `json:"role_thresholds,omitempty"`

	// SeparationThreshold is the default k-hop distance for separation (if RoleThresholds not set).
	SeparationThreshold int `json:"separation_threshold,omitempty"`

	// CentralityWeight is the weight for centrality attraction in cohesion rules (0.0-1.0).
	CentralityWeight float64 `json:"centrality_weight,omitempty"`

	// AlignmentWindow is the number of recent traversals to consider for alignment.
	AlignmentWindow int `json:"alignment_window,omitempty"`

	// SteeringStrength is how much to influence agent behavior (0.0-1.0).
	SteeringStrength float64 `json:"steering_strength"`

	// Cooldown specifies minimum time between signal emissions.
	Cooldown string `json:"cooldown,omitempty"`
}

// GetSeparationThreshold returns the separation threshold for a given role.
func (c *Config) GetSeparationThreshold(role string) int {
	if c.RoleThresholds != nil {
		if threshold, ok := c.RoleThresholds[role]; ok {
			return threshold
		}
	}
	if c.SeparationThreshold > 0 {
		return c.SeparationThreshold
	}
	return DefaultSeparationThreshold
}

// ParseConfig extracts Config from rule metadata.
func ParseConfig(metadata map[string]any) (*Config, error) {
	// Marshal to JSON then unmarshal to Config for type safety
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	config := &Config{
		// Set defaults
		SeparationThreshold: DefaultSeparationThreshold,
		SteeringStrength:    DefaultSteeringStrength,
		AlignmentWindow:     DefaultAlignmentWindow,
		CentralityWeight:    DefaultCentralityWeight,
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}
