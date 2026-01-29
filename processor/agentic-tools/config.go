package agentictools

import (
	"fmt"
	"time"

	"github.com/c360/semstreams/component"
)

// Config holds configuration for agentic-tools processor component
type Config struct {
	Ports              *component.PortConfig `json:"ports"                schema:"type:ports,description:Port configuration,category:basic"`
	StreamName         string                `json:"stream_name"          schema:"type:string,description:JetStream stream name for agentic messages,category:basic,default:AGENT"`
	ConsumerNameSuffix string                `json:"consumer_name_suffix" schema:"type:string,description:Suffix appended to consumer names for uniqueness,category:advanced"`
	Timeout            string                `json:"timeout"              schema:"type:string,description:Tool execution timeout,category:advanced,default:60s"`
	AllowedTools       []string              `json:"allowed_tools"        schema:"type:array,description:List of allowed tools (nil/empty allows all),category:advanced"`
	HeartbeatTimeout   string                `json:"heartbeat_timeout"    schema:"type:string,description:External tool heartbeat timeout,category:advanced,default:30s"`
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	// Validate timeout
	if c.Timeout == "" {
		return fmt.Errorf("timeout is required")
	}

	// Parse timeout to ensure it's valid
	duration, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout format: %w", err)
	}

	// Timeout must be positive
	if duration <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	// AllowedTools can be nil or empty (both mean allow all tools)
	// No validation needed for allowed_tools

	return nil
}

// DefaultConfig returns default configuration for agentic-tools processor
func DefaultConfig() Config {
	inputDefs := []component.PortDefinition{
		{
			Name:        "tool.execute",
			Type:        "jetstream",
			Subject:     "tool.execute.>",
			StreamName:  "AGENT",
			Required:    true,
			Description: "Tool execution requests (JetStream)",
		},
	}

	outputDefs := []component.PortDefinition{
		{
			Name:        "tool.result",
			Type:        "jetstream",
			Subject:     "tool.result.*",
			StreamName:  "AGENT",
			Required:    true,
			Description: "Tool execution results (JetStream)",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Inputs:  inputDefs,
			Outputs: outputDefs,
		},
		StreamName:   "AGENT",
		Timeout:      "60s",
		AllowedTools: nil, // nil means allow all tools
	}
}
