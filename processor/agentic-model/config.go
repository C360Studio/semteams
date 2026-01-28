package agenticmodel

import (
	"fmt"
	"time"

	"github.com/c360/semstreams/component"
)

// Config holds configuration for agentic-model processor component
type Config struct {
	Ports              *component.PortConfig `json:"ports"                schema:"type:ports,description:Port configuration,category:basic"`
	Endpoints          map[string]Endpoint   `json:"endpoints"            schema:"type:object,description:Model endpoints,category:basic"`
	StreamName         string                `json:"stream_name"          schema:"type:string,description:JetStream stream name for agentic messages,category:basic,default:AGENT"`
	ConsumerNameSuffix string                `json:"consumer_name_suffix" schema:"type:string,description:Suffix appended to consumer names for uniqueness,category:advanced"`
	Timeout            string                `json:"timeout"              schema:"type:string,description:Request timeout,category:advanced,default:120s"`
	Retry              RetryConfig           `json:"retry"                schema:"type:object,description:Retry configuration,category:advanced"`
}

// Endpoint represents a single model endpoint configuration
type Endpoint struct {
	URL       string `json:"url"         schema:"type:string,description:OpenAI-compatible API URL,category:basic"`
	Model     string `json:"model"       schema:"type:string,description:Model name,category:basic"`
	APIKeyEnv string `json:"api_key_env" schema:"type:string,description:Environment variable for API key,category:advanced"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts int    `json:"max_attempts" schema:"type:int,description:Maximum retry attempts,category:advanced,default:3"`
	Backoff     string `json:"backoff"      schema:"type:enum,description:Backoff strategy,category:advanced,enum:exponential|linear,default:exponential"`
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	if c.Endpoints == nil || len(c.Endpoints) == 0 {
		return fmt.Errorf("endpoints cannot be empty")
	}

	for name, endpoint := range c.Endpoints {
		if err := endpoint.Validate(); err != nil {
			return fmt.Errorf("endpoint %q: %w", name, err)
		}
	}

	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %w", err)
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
		return fmt.Errorf("retry config: %w", err)
	}

	return nil
}

// Validate checks the endpoint configuration for errors
func (e *Endpoint) Validate() error {
	if e.URL == "" {
		return fmt.Errorf("url is required")
	}
	if e.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

// Validate checks the retry configuration for errors
func (r *RetryConfig) Validate() error {
	if r.MaxAttempts < 1 {
		return fmt.Errorf("max_attempts must be at least 1")
	}

	// Empty backoff defaults to exponential
	if r.Backoff == "" {
		return nil
	}

	if r.Backoff != "exponential" && r.Backoff != "linear" {
		return fmt.Errorf("backoff must be 'exponential' or 'linear'")
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
		Endpoints:  make(map[string]Endpoint),
		StreamName: "AGENT",
		Timeout:    "120s",
		Retry: RetryConfig{
			MaxAttempts: 3,
			Backoff:     "exponential",
		},
	}
}
