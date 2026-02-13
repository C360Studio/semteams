package slim

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config defines the configuration for the SLIM bridge component.
type Config struct {
	// Ports defines the input/output port configuration.
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// SLIMEndpoint is the SLIM service endpoint URL.
	SLIMEndpoint string `json:"slim_endpoint" schema:"type:string,description:SLIM service endpoint URL,category:basic"`

	// GroupIDs specifies which SLIM groups to join.
	// If empty, the bridge will dynamically join groups based on tenant configuration.
	GroupIDs []string `json:"group_ids" schema:"type:array,description:SLIM group IDs to join,category:basic"`

	// KeyRatchetInterval is how often to ratchet MLS keys.
	KeyRatchetInterval string `json:"key_ratchet_interval" schema:"type:string,description:MLS key ratchet interval,category:advanced,default:1h"`

	// ReconnectInterval is the delay between reconnection attempts.
	ReconnectInterval string `json:"reconnect_interval" schema:"type:string,description:Reconnection interval,category:advanced,default:5s"`

	// MaxReconnectAttempts is the maximum number of reconnection attempts.
	MaxReconnectAttempts int `json:"max_reconnect_attempts" schema:"type:int,description:Maximum reconnection attempts,category:advanced,default:10"`

	// MessageBufferSize is the size of the message buffer for async processing.
	MessageBufferSize int `json:"message_buffer_size" schema:"type:int,description:Message buffer size,category:advanced,default:1000"`

	// IdentityProvider specifies which identity provider to use for DID resolution.
	IdentityProvider string `json:"identity_provider" schema:"type:string,description:Identity provider type,category:basic,default:local"`

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
					Name:        "slim_messages",
					Subject:     "slim.message.>",
					Type:        "nats",
					Required:    false,
					Description: "Messages from SLIM groups",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "user_messages",
					Subject:     "user.message.slim.>",
					Type:        "jetstream",
					StreamName:  "USER_MESSAGES",
					Required:    true,
					Description: "User messages to agent dispatch",
				},
				{
					Name:        "task_delegations",
					Subject:     "agent.task.slim.>",
					Type:        "jetstream",
					StreamName:  "AGENT_TASKS",
					Required:    false,
					Description: "Task delegations from external agents",
				},
			},
		},
		KeyRatchetInterval:   "1h",
		ReconnectInterval:    "5s",
		MaxReconnectAttempts: 10,
		MessageBufferSize:    1000,
		IdentityProvider:     "local",
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Ports == nil {
		return fmt.Errorf("ports configuration is required")
	}

	// Validate key ratchet interval if set
	if c.KeyRatchetInterval != "" {
		if _, err := time.ParseDuration(c.KeyRatchetInterval); err != nil {
			return fmt.Errorf("invalid key_ratchet_interval: %w", err)
		}
	}

	// Validate reconnect interval if set
	if c.ReconnectInterval != "" {
		if _, err := time.ParseDuration(c.ReconnectInterval); err != nil {
			return fmt.Errorf("invalid reconnect_interval: %w", err)
		}
	}

	if c.MaxReconnectAttempts < 0 {
		return fmt.Errorf("max_reconnect_attempts must be non-negative")
	}

	if c.MessageBufferSize < 0 {
		return fmt.Errorf("message_buffer_size must be non-negative")
	}

	return nil
}

// GetKeyRatchetInterval returns the key ratchet interval.
func (c *Config) GetKeyRatchetInterval() time.Duration {
	if c.KeyRatchetInterval == "" {
		return time.Hour
	}
	d, err := time.ParseDuration(c.KeyRatchetInterval)
	if err != nil {
		return time.Hour
	}
	return d
}

// GetReconnectInterval returns the reconnect interval.
func (c *Config) GetReconnectInterval() time.Duration {
	if c.ReconnectInterval == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(c.ReconnectInterval)
	if err != nil {
		return 5 * time.Second
	}
	return d
}
