package agenticloop

import (
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config represents the configuration for the agentic-loop processor
type Config struct {
	MaxIterations      int                   `json:"max_iterations"`
	Timeout            string                `json:"timeout"`
	StreamName         string                `json:"stream_name"`
	ConsumerNameSuffix string                `json:"consumer_name_suffix"`
	LoopsBucket        string                `json:"loops_bucket"`
	TrajectoriesBucket string                `json:"trajectories_bucket"`
	Context            ContextConfig         `json:"context"`
	Ports              *component.PortConfig `json:"ports,omitempty"`
}

// ContextConfig represents configuration for context memory management
type ContextConfig struct {
	Enabled            bool           `json:"enabled"`
	CompactThreshold   float64        `json:"compact_threshold"`
	ToolResultMaxAge   int            `json:"tool_result_max_age"`
	HeadroomTokens     int            `json:"headroom_tokens"`
	SummarizationModel string         `json:"summarization_model"`
	ModelLimits        map[string]int `json:"model_limits"`
}

// Validate validates the configuration
func (c Config) Validate() error {
	// Validate max_iterations
	if c.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations must be greater than 0")
	}

	// Validate timeout
	if strings.TrimSpace(c.Timeout) == "" {
		return fmt.Errorf("timeout is required")
	}

	// Parse timeout to ensure it's valid
	duration, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout format: %w", err)
	}

	// Ensure timeout is positive
	if duration <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	// Validate loops_bucket
	if strings.TrimSpace(c.LoopsBucket) == "" {
		return fmt.Errorf("loops_bucket is required")
	}

	// Validate trajectories_bucket
	if strings.TrimSpace(c.TrajectoriesBucket) == "" {
		return fmt.Errorf("trajectories_bucket is required")
	}

	// Validate context config
	return c.Context.Validate()
}

// Validate validates the context configuration
func (c ContextConfig) Validate() error {
	if !c.Enabled {
		return nil // Disabled config doesn't need validation
	}

	// Validate threshold: must be 0.01-1.0
	if c.CompactThreshold < 0.01 || c.CompactThreshold > 1.0 {
		return fmt.Errorf("compact_threshold must be between 0.01 and 1.0")
	}

	// Validate ToolResultMaxAge: must be > 0
	if c.ToolResultMaxAge <= 0 {
		return fmt.Errorf("tool_result_max_age must be greater than 0")
	}

	// Validate HeadroomTokens: must be >= 0
	if c.HeadroomTokens < 0 {
		return fmt.Errorf("headroom_tokens must be non-negative")
	}

	// Validate SummarizationModel: must not be empty
	if c.SummarizationModel == "" {
		return fmt.Errorf("summarization_model is required when context management is enabled")
	}

	// Validate ModelLimits: must have "default" key, all values > 0
	if c.ModelLimits == nil || len(c.ModelLimits) == 0 {
		return fmt.Errorf("model_limits cannot be empty")
	}
	if _, hasDefault := c.ModelLimits["default"]; !hasDefault {
		return fmt.Errorf("model_limits must contain 'default' entry")
	}
	for model, limit := range c.ModelLimits {
		if limit <= 0 {
			return fmt.Errorf("model_limits for %q must be greater than 0", model)
		}
	}

	return nil
}

// DefaultContextConfig returns the default context configuration
func DefaultContextConfig() ContextConfig {
	return ContextConfig{
		Enabled:            true,
		CompactThreshold:   0.60,
		ToolResultMaxAge:   3,
		HeadroomTokens:     6400,
		SummarizationModel: "fast",
		ModelLimits: map[string]int{
			"gpt-4o":        128000,
			"gpt-4o-mini":   128000,
			"claude-sonnet": 200000,
			"claude-opus":   200000,
			"default":       128000,
		},
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		MaxIterations:      20,
		Timeout:            "120s",
		StreamName:         "AGENT",
		LoopsBucket:        "AGENT_LOOPS",
		TrajectoriesBucket: "AGENT_TRAJECTORIES",
		Context:            DefaultContextConfig(),
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "agent.task",
					Type:        "jetstream",
					Subject:     "agent.task.*",
					StreamName:  "AGENT",
					Required:    true,
					Description: "Agent task requests (JetStream)",
				},
				{
					Name:        "agent.response",
					Type:        "jetstream",
					Subject:     "agent.response.>",
					StreamName:  "AGENT",
					Required:    true,
					Description: "Agent model responses (JetStream)",
				},
				{
					Name:        "tool.result",
					Type:        "jetstream",
					Subject:     "tool.result.>",
					StreamName:  "AGENT",
					Required:    true,
					Description: "Tool execution results (JetStream)",
				},
				{
					Name:        "agent.signal",
					Type:        "jetstream",
					Subject:     "agent.signal.*",
					StreamName:  "AGENT",
					Required:    false,
					Description: "Control signals for loops (cancel, pause, etc.)",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "agent.request",
					Type:        "jetstream",
					Subject:     "agent.request.*",
					StreamName:  "AGENT",
					Description: "Agent model requests (JetStream)",
				},
				{
					Name:        "tool.execute",
					Type:        "jetstream",
					Subject:     "tool.execute.*",
					StreamName:  "AGENT",
					Description: "Tool execution requests (JetStream)",
				},
				{
					Name:        "agent.complete",
					Type:        "jetstream",
					Subject:     "agent.complete.*",
					StreamName:  "AGENT",
					Description: "Agent task completions (JetStream)",
				},
				{
					Name:        "agent.context.compaction",
					Type:        "jetstream",
					Subject:     "agent.context.compaction.*",
					StreamName:  "AGENT",
					Description: "Context compaction events (JetStream)",
				},
			},
			KVWrite: []component.PortDefinition{
				{
					Name:        "loops",
					Type:        "kv-write",
					Bucket:      "AGENT_LOOPS",
					Description: "Loop state storage",
				},
				{
					Name:        "trajectories",
					Type:        "kv-write",
					Bucket:      "AGENT_TRAJECTORIES",
					Description: "Trajectory storage",
				},
			},
		},
	}
}
