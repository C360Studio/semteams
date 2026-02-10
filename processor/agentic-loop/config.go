package agenticloop

import (
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Context configuration constants
const (
	// DefaultModelKey is the key used in ModelLimits for the fallback context limit
	DefaultModelKey = "default"

	// MaxReasonableContextLimit is the maximum allowed token limit (2M tokens, future-proof)
	MaxReasonableContextLimit = 2_000_000

	// MinContextLimit is the minimum allowed token limit (sanity check)
	MinContextLimit = 1024
)

// Config represents the configuration for the agentic-loop processor
type Config struct {
	MaxIterations      int                   `json:"max_iterations" schema:"type:int,description:Maximum number of iterations before loop terminates,default:20,min:1,max:1000,category:basic,required"`
	Timeout            string                `json:"timeout" schema:"type:string,description:Timeout duration for loop execution (e.g. 120s or 5m),default:120s,category:basic,required"`
	StreamName         string                `json:"stream_name" schema:"type:string,description:JetStream stream name,default:AGENT,category:advanced"`
	ConsumerNameSuffix string                `json:"consumer_name_suffix" schema:"type:string,description:Suffix for consumer names,category:advanced"`
	LoopsBucket        string                `json:"loops_bucket" schema:"type:string,description:NATS KV bucket name for storing loop state,default:AGENT_LOOPS,category:advanced,required"`
	TrajectoriesBucket string                `json:"trajectories_bucket" schema:"type:string,description:NATS KV bucket name for storing trajectories,default:AGENT_TRAJECTORIES,category:advanced,required"`
	Context            ContextConfig         `json:"context" schema:"type:object,description:Context window management. model_limits maps model names to context window sizes in tokens,category:advanced"`
	Ports              *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration for inputs and outputs,category:basic"`
}

// ContextConfig represents configuration for context memory management.
// ModelLimits maps model names (e.g., "llama3.2:8b", "mistral-7b-instruct") to their
// context window sizes in tokens. Must include a "default" key for unknown models.
type ContextConfig struct {
	Enabled            bool           `json:"enabled" description:"Enable context memory management"`
	CompactThreshold   float64        `json:"compact_threshold" description:"Utilization threshold (0.01-1.0) that triggers context compaction"`
	ToolResultMaxAge   int            `json:"tool_result_max_age" description:"Maximum age in iterations before tool results are garbage collected"`
	HeadroomTokens     int            `json:"headroom_tokens" description:"Token headroom to reserve for model responses"`
	SummarizationModel string         `json:"summarization_model" description:"Model alias to use for context summarization"`
	ModelLimits        map[string]int `json:"model_limits" description:"Map of model name to context window size in tokens. Must include 'default' key for fallback."`
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

	// Validate ModelLimits: must have "default" key, all values within bounds
	if c.ModelLimits == nil || len(c.ModelLimits) == 0 {
		return fmt.Errorf("model_limits cannot be empty")
	}
	if _, hasDefault := c.ModelLimits[DefaultModelKey]; !hasDefault {
		return fmt.Errorf("model_limits must contain %q entry", DefaultModelKey)
	}
	for model, limit := range c.ModelLimits {
		if limit <= 0 {
			return fmt.Errorf("model_limits for %q must be greater than 0", model)
		}
		if limit < MinContextLimit {
			return fmt.Errorf("model_limits[%q] = %d is below minimum %d", model, limit, MinContextLimit)
		}
		if limit > MaxReasonableContextLimit {
			return fmt.Errorf("model_limits[%q] = %d exceeds maximum %d", model, limit, MaxReasonableContextLimit)
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
