package agenticmodel

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Config holds configuration for agentic-model processor component.
// Model endpoints are resolved from the unified model registry (component.Dependencies.ModelRegistry).
type Config struct {
	Ports                *component.PortConfig `json:"ports"                schema:"type:ports,description:Port configuration,category:basic"`
	StreamName           string                `json:"stream_name"          schema:"type:string,description:JetStream stream name for agentic messages,category:basic,default:AGENT"`
	ConsumerNameSuffix   string                `json:"consumer_name_suffix" schema:"type:string,description:Suffix appended to consumer names for uniqueness,category:advanced"`
	DeleteConsumerOnStop bool                  `json:"delete_consumer_on_stop,omitempty" schema:"type:bool,description:Delete durable consumers on Stop (use for tests only),category:advanced,default:false"`
	Timeout              string                `json:"timeout"              schema:"type:string,description:Request timeout,category:advanced,default:120s"`
	Retry                RetryConfig           `json:"retry"                schema:"type:object,description:Retry configuration,category:advanced"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts int    `json:"max_attempts" schema:"type:int,description:Maximum retry attempts,category:advanced,default:3"`
	Backoff     string `json:"backoff"      schema:"type:enum,description:Backoff strategy,category:advanced,enum:exponential|linear,default:exponential"`
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "parse timeout")
		}
	}

	// Apply defaults before validation if Retry is zero value
	if c.Retry.MaxAttempts == 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.Backoff == "" {
		c.Retry.Backoff = "exponential"
	}

	if err := c.Retry.Validate(); err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "validate retry config")
	}

	return nil
}

// Validate checks the retry configuration for errors
func (r *RetryConfig) Validate() error {
	if r.MaxAttempts < 1 {
		return errs.WrapInvalid(fmt.Errorf("max_attempts must be at least 1"), "RetryConfig", "Validate", "check max_attempts")
	}

	// Empty backoff defaults to exponential
	if r.Backoff == "" {
		return nil
	}

	if r.Backoff != "exponential" && r.Backoff != "linear" {
		return errs.WrapInvalid(fmt.Errorf("backoff must be 'exponential' or 'linear'"), "RetryConfig", "Validate", "check backoff type")
	}

	return nil
}

// DefaultConfig returns default configuration for agentic-model processor
func DefaultConfig() Config {
	inputDefs := []component.PortDefinition{
		{
			Name:        "agent.request",
			Type:        "jetstream",
			Subject:     "agent.request.>",
			StreamName:  "AGENT",
			Required:    true,
			Description: "Agent request input (JetStream)",
		},
	}

	outputDefs := []component.PortDefinition{
		{
			Name:        "agent.response",
			Type:        "jetstream",
			Subject:     "agent.response.*",
			StreamName:  "AGENT",
			Required:    true,
			Description: "Agent response output (JetStream)",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Inputs:  inputDefs,
			Outputs: outputDefs,
		},
		StreamName: "AGENT",
		Timeout:    "120s",
		Retry: RetryConfig{
			MaxAttempts: 3,
			Backoff:     "exponential",
		},
	}
}
