package boid

import (
	"fmt"
	"log/slog"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/rule"
)

// RuleFactory creates boid coordination rules.
type RuleFactory struct {
	ruleType string
}

// NewRuleFactory creates a new boid rule factory.
func NewRuleFactory() *RuleFactory {
	return &RuleFactory{
		ruleType: "boid",
	}
}

// Type returns the factory type identifier.
func (f *RuleFactory) Type() string {
	return f.ruleType
}

// Create creates a boid rule from the definition.
func (f *RuleFactory) Create(id string, def rule.Definition, deps rule.Dependencies) (rule.Rule, error) {
	config, err := ParseConfig(def.Metadata)
	if err != nil {
		return nil, fmt.Errorf("parse boid config: %w", err)
	}

	// Parse cooldown if specified
	var cooldown time.Duration
	if config.Cooldown != "" {
		cooldown, err = time.ParseDuration(config.Cooldown)
		if err != nil {
			return nil, fmt.Errorf("invalid cooldown duration %q: %w", config.Cooldown, err)
		}
	}

	// Create appropriate rule type based on config
	switch config.BoidRule {
	case RuleTypeSeparation:
		return NewSeparationRule(id, def, config, cooldown, deps.Logger), nil
	case RuleTypeCohesion:
		return NewCohesionRule(id, def, config, cooldown, deps.Logger), nil
	case RuleTypeAlignment:
		return NewAlignmentRule(id, def, config, cooldown, deps.Logger), nil
	default:
		return nil, fmt.Errorf("unknown boid rule type: %s", config.BoidRule)
	}
}

// Validate validates the boid rule definition.
func (f *RuleFactory) Validate(def rule.Definition) error {
	if def.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	// Parse and validate boid config
	config, err := ParseConfig(def.Metadata)
	if err != nil {
		return fmt.Errorf("invalid boid config: %w", err)
	}

	// Validate boid_rule is specified
	if config.BoidRule == "" {
		return fmt.Errorf("rule %s: metadata.boid_rule is required", def.ID)
	}

	// Validate boid_rule is a known type
	validTypes := map[string]bool{
		RuleTypeSeparation: true,
		RuleTypeCohesion:   true,
		RuleTypeAlignment:  true,
	}
	if !validTypes[config.BoidRule] {
		return fmt.Errorf("rule %s: unknown boid_rule type %q (must be: separation, cohesion, alignment)",
			def.ID, config.BoidRule)
	}

	// Validate steering strength is in range
	if config.SteeringStrength < 0 || config.SteeringStrength > 1 {
		return fmt.Errorf("rule %s: steering_strength must be between 0.0 and 1.0", def.ID)
	}

	// Validate centrality weight for cohesion rules
	if config.BoidRule == RuleTypeCohesion {
		if config.CentralityWeight < 0 || config.CentralityWeight > 1 {
			return fmt.Errorf("rule %s: centrality_weight must be between 0.0 and 1.0", def.ID)
		}
	}

	// Validate cooldown if specified
	if config.Cooldown != "" {
		if _, err := time.ParseDuration(config.Cooldown); err != nil {
			return fmt.Errorf("rule %s: invalid cooldown: %w", def.ID, err)
		}
	}

	return nil
}

// Schema returns the boid rule schema for documentation.
func (f *RuleFactory) Schema() rule.Schema {
	return rule.Schema{
		Type:        "boid",
		DisplayName: "Boid Coordination Rule",
		Description: "Local coordination rule based on Boids flocking behavior for multi-agent teams",
		Category:    "coordination",
		Required:    []string{"id", "metadata.boid_rule"},
		Examples: []rule.Example{
			{
				Name:        "Separation Rule",
				Description: "Prevent agents from working on overlapping graph neighborhoods",
				Config: rule.Definition{
					ID:      "boid_separation",
					Type:    "boid",
					Name:    "Agent Separation",
					Enabled: true,
					Entity: rule.EntityConfig{
						WatchBuckets: []string{KVBucketAgentPositions},
					},
					Metadata: map[string]any{
						"boid_rule": "separation",
						"role_thresholds": map[string]int{
							"general":   2,
							"architect": 3,
						},
						"steering_strength": 0.8,
					},
				},
			},
		},
	}
}

// baseBoidRule provides common functionality for all boid rules.
type baseBoidRule struct {
	id            string
	name          string
	description   string
	enabled       bool
	config        *Config
	cooldown      time.Duration
	lastTriggered time.Time
	logger        *slog.Logger
}

// newBaseBoidRule creates base rule from definition.
func newBaseBoidRule(id string, def rule.Definition, config *Config, cooldown time.Duration, logger *slog.Logger) baseBoidRule {
	if logger == nil {
		logger = slog.Default()
	}
	return baseBoidRule{
		id:          id,
		name:        def.Name,
		description: def.Description,
		enabled:     def.Enabled,
		config:      config,
		cooldown:    cooldown,
		logger:      logger,
	}
}

// Name returns the rule name.
func (r *baseBoidRule) Name() string {
	return r.name
}

// Subscribe returns subjects this rule subscribes to.
// Boid rules watch the AGENT_POSITIONS KV bucket, not NATS subjects.
func (r *baseBoidRule) Subscribe() []string {
	return []string{} // Uses KV watcher, not subject subscription
}

// Evaluate is not used for boid rules (they use EvaluateEntityState).
func (r *baseBoidRule) Evaluate(_ []message.Message) bool {
	return false
}

// ExecuteEvents is not used for boid rules (signals are generated in EvaluateEntityState).
func (r *baseBoidRule) ExecuteEvents(_ []message.Message) ([]rule.Event, error) {
	return nil, nil
}

// canTrigger checks if cooldown has elapsed since last trigger.
func (r *baseBoidRule) canTrigger() bool {
	if r.cooldown == 0 {
		return true
	}
	return time.Since(r.lastTriggered) >= r.cooldown
}

// markTriggered updates the last triggered time.
func (r *baseBoidRule) markTriggered() {
	r.lastTriggered = time.Now()
}

// matchesRoleFilter checks if the agent's role matches the filter.
func (r *baseBoidRule) matchesRoleFilter(role string) bool {
	if r.config.RoleFilter == "" {
		return true // No filter, matches all roles
	}
	return r.config.RoleFilter == role
}

// extractAgentPosition extracts AgentPosition from EntityState triples.
func extractAgentPosition(entityState *gtypes.EntityState) (*AgentPosition, error) {
	if entityState == nil {
		return nil, fmt.Errorf("nil entity state")
	}

	pos := &AgentPosition{
		LoopID: entityState.ID,
	}

	for _, triple := range entityState.Triples {
		switch triple.Predicate {
		case "boid.role":
			if s, ok := triple.Object.(string); ok {
				pos.Role = s
			}
		case "boid.focus_entities":
			if arr, ok := triple.Object.([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						pos.FocusEntities = append(pos.FocusEntities, s)
					}
				}
			}
		case "boid.traversal_vector":
			if arr, ok := triple.Object.([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						pos.TraversalVector = append(pos.TraversalVector, s)
					}
				}
			}
		case "boid.velocity":
			if f, ok := triple.Object.(float64); ok {
				pos.Velocity = f
			}
		case "boid.iteration":
			switch v := triple.Object.(type) {
			case int:
				pos.Iteration = v
			case float64:
				pos.Iteration = int(v)
			}
		case "boid.last_update":
			if s, ok := triple.Object.(string); ok {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					pos.LastUpdate = t
				}
			}
		}
	}

	return pos, nil
}

// init registers the boid rule factory.
func init() {
	factory := NewRuleFactory()
	if err := rule.RegisterRuleFactory("boid", factory); err != nil {
		// Log but don't panic - allows tests to re-register
		fmt.Printf("Warning: Failed to register boid factory: %v\n", err)
	}
}
