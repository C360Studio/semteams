package directorybridge

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config defines the configuration for the directory bridge component.
type Config struct {
	// Ports defines the input/output port configuration.
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// DirectoryURL is the AGNTCY directory service URL.
	DirectoryURL string `json:"directory_url" schema:"type:string,description:AGNTCY directory service URL,category:basic"`

	// HeartbeatInterval is how often to send heartbeats to the directory.
	HeartbeatInterval string `json:"heartbeat_interval" schema:"type:string,description:Heartbeat interval,category:basic,default:30s"`

	// RegistrationTTL is the time-to-live for registrations.
	RegistrationTTL string `json:"registration_ttl" schema:"type:string,description:Registration time-to-live,category:basic,default:5m"`

	// IdentityProvider specifies which identity provider to use.
	// Values: "local", "agntcy"
	IdentityProvider string `json:"identity_provider" schema:"type:string,description:Identity provider type,category:basic,default:local"`

	// OASFKVBucket is the KV bucket to watch for OASF records.
	OASFKVBucket string `json:"oasf_kv_bucket" schema:"type:string,description:KV bucket for OASF records,category:basic,default:OASF_RECORDS"`

	// RetryCount is the number of retries for failed registrations.
	RetryCount int `json:"retry_count" schema:"type:int,description:Number of registration retries,category:advanced,default:3"`

	// RetryDelay is the initial delay between retries.
	RetryDelay string `json:"retry_delay" schema:"type:string,description:Initial retry delay,category:advanced,default:1s"`

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
					Name:        "oasf_records",
					Subject:     "oasf.record.generated.>",
					Type:        "kv-watch",
					Required:    true,
					Description: "Watch for generated OASF records",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "registration_events",
					Subject:     "directory.registration.*",
					Type:        "jetstream",
					Required:    false,
					Description: "Registration events",
				},
			},
		},
		HeartbeatInterval: "30s",
		RegistrationTTL:   "5m",
		IdentityProvider:  "local",
		OASFKVBucket:      "OASF_RECORDS",
		RetryCount:        3,
		RetryDelay:        "1s",
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Ports == nil {
		return fmt.Errorf("ports configuration is required")
	}

	// DirectoryURL empty is allowed for local development/testing

	if c.OASFKVBucket == "" {
		return fmt.Errorf("oasf_kv_bucket is required")
	}

	// Validate heartbeat interval if set
	if c.HeartbeatInterval != "" {
		if _, err := time.ParseDuration(c.HeartbeatInterval); err != nil {
			return fmt.Errorf("invalid heartbeat_interval: %w", err)
		}
	}

	// Validate registration TTL if set
	if c.RegistrationTTL != "" {
		if _, err := time.ParseDuration(c.RegistrationTTL); err != nil {
			return fmt.Errorf("invalid registration_ttl: %w", err)
		}
	}

	// Validate retry delay if set
	if c.RetryDelay != "" {
		if _, err := time.ParseDuration(c.RetryDelay); err != nil {
			return fmt.Errorf("invalid retry_delay: %w", err)
		}
	}

	return nil
}

// GetHeartbeatInterval returns the heartbeat interval.
func (c *Config) GetHeartbeatInterval() time.Duration {
	if c.HeartbeatInterval == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.HeartbeatInterval)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetRegistrationTTL returns the registration TTL.
func (c *Config) GetRegistrationTTL() time.Duration {
	if c.RegistrationTTL == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(c.RegistrationTTL)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// GetRetryDelay returns the retry delay.
func (c *Config) GetRetryDelay() time.Duration {
	if c.RetryDelay == "" {
		return time.Second
	}
	d, err := time.ParseDuration(c.RetryDelay)
	if err != nil {
		return time.Second
	}
	return d
}
