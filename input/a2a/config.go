package a2a

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config defines the configuration for the A2A adapter component.
type Config struct {
	// Ports defines the input/output port configuration.
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Transport specifies the A2A transport mechanism.
	// Supported values: "http", "slim"
	Transport string `json:"transport" schema:"type:string,description:A2A transport type,category:basic,default:http"`

	// ListenAddress is the address to listen on for incoming A2A requests.
	// Only used when transport is "http".
	ListenAddress string `json:"listen_address" schema:"type:string,description:HTTP listen address,category:basic,default::8080"`

	// AgentCardPath is the path to serve the agent card.
	AgentCardPath string `json:"agent_card_path" schema:"type:string,description:Path for agent card endpoint,category:basic,default:/.well-known/agent.json"`

	// SLIMGroupID is the SLIM group for A2A communication.
	// Only used when transport is "slim".
	SLIMGroupID string `json:"slim_group_id" schema:"type:string,description:SLIM group for A2A,category:advanced"`

	// RequestTimeout is the timeout for processing A2A requests.
	RequestTimeout string `json:"request_timeout" schema:"type:string,description:Request processing timeout,category:advanced,default:30s"`

	// MaxConcurrentTasks is the maximum number of concurrent task executions.
	MaxConcurrentTasks int `json:"max_concurrent_tasks" schema:"type:int,description:Maximum concurrent tasks,category:advanced,default:10"`

	// EnableAuthentication enables DID-based authentication for requests.
	EnableAuthentication bool `json:"enable_authentication" schema:"type:bool,description:Enable DID authentication,category:security,default:true"`

	// AllowedAgents is a list of DIDs allowed to send tasks.
	// Empty list allows all authenticated agents.
	AllowedAgents []string `json:"allowed_agents" schema:"type:array,description:Allowed agent DIDs,category:security"`

	// OASFBucket is the KV bucket containing OASF records for agent card generation.
	OASFBucket string `json:"oasf_bucket" schema:"type:string,description:OASF records KV bucket,category:advanced,default:OASF_RECORDS"`

	// ConsumerNameSuffix adds a suffix to consumer names for uniqueness in tests.
	ConsumerNameSuffix string `json:"consumer_name_suffix" schema:"type:string,description:Suffix for consumer names,category:advanced"`

	// DeleteConsumerOnStop enables consumer cleanup on stop (for testing).
	DeleteConsumerOnStop bool `json:"delete_consumer_on_stop,omitempty" schema:"type:bool,description:Delete consumers on Stop,category:advanced,default:false"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "a2a_requests",
					Subject:     "a2a.request.>",
					Type:        "nats",
					Required:    false,
					Description: "Incoming A2A task requests",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "task_messages",
					Subject:     "agent.task.a2a.>",
					Type:        "jetstream",
					StreamName:  "AGENT_TASKS",
					Required:    true,
					Description: "Task messages to agent dispatch",
				},
				{
					Name:        "a2a_responses",
					Subject:     "a2a.response.>",
					Type:        "nats",
					Required:    false,
					Description: "Outgoing A2A task responses",
				},
			},
		},
		Transport:            "http",
		ListenAddress:        ":8080",
		AgentCardPath:        "/.well-known/agent.json",
		RequestTimeout:       "30s",
		MaxConcurrentTasks:   10,
		EnableAuthentication: true,
		OASFBucket:           "OASF_RECORDS",
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Ports == nil {
		return fmt.Errorf("ports configuration is required")
	}

	if c.Transport != "" && c.Transport != "http" && c.Transport != "slim" {
		return fmt.Errorf("invalid transport: %s (must be 'http' or 'slim')", c.Transport)
	}

	if c.Transport == "slim" && c.SLIMGroupID == "" {
		return fmt.Errorf("slim_group_id is required when transport is 'slim'")
	}

	if c.RequestTimeout != "" {
		if _, err := time.ParseDuration(c.RequestTimeout); err != nil {
			return fmt.Errorf("invalid request_timeout: %w", err)
		}
	}

	if c.MaxConcurrentTasks < 0 {
		return fmt.Errorf("max_concurrent_tasks must be non-negative")
	}

	return nil
}

// GetRequestTimeout returns the request timeout duration.
func (c *Config) GetRequestTimeout() time.Duration {
	if c.RequestTimeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.RequestTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}
