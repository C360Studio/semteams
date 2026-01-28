package agenticloop

import (
	"fmt"
	"strings"
	"time"

	"github.com/c360/semstreams/component"
)

// Config represents the configuration for the agentic-loop processor
type Config struct {
	MaxIterations      int                   `json:"max_iterations"`
	Timeout            string                `json:"timeout"`
	StreamName         string                `json:"stream_name"`
	ConsumerNameSuffix string                `json:"consumer_name_suffix"`
	LoopsBucket        string                `json:"loops_bucket"`
	TrajectoriesBucket string                `json:"trajectories_bucket"`
	Ports              *component.PortConfig `json:"ports,omitempty"`
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

	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		MaxIterations:      20,
		Timeout:            "120s",
		StreamName:         "AGENT",
		LoopsBucket:        "AGENT_LOOPS",
		TrajectoriesBucket: "AGENT_TRAJECTORIES",
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
