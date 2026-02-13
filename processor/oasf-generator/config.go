package oasfgenerator

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config defines the configuration for the OASF generator component.
type Config struct {
	// Ports defines the input/output port configuration.
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// StreamName is the JetStream stream to subscribe to for entity changes.
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name for entity events,category:basic,default:ENTITY"`

	// EntityKVBucket is the KV bucket to watch for entity state changes.
	EntityKVBucket string `json:"entity_kv_bucket" schema:"type:string,description:KV bucket for entity states,category:basic,default:ENTITY_STATES"`

	// OASFKVBucket is the KV bucket to store generated OASF records.
	OASFKVBucket string `json:"oasf_kv_bucket" schema:"type:string,description:KV bucket for OASF records,category:basic,default:OASF_RECORDS"`

	// WatchPattern is the key pattern to watch in the entity KV bucket.
	// Use ">" for all keys (including those with dots) or a specific pattern.
	// Note: "*" only matches single tokens without dots, ">" matches any depth.
	WatchPattern string `json:"watch_pattern" schema:"type:string,description:Key pattern to watch for entity changes,category:advanced,default:>"`

	// GenerationDebounce is the debounce duration for OASF generation after entity changes.
	GenerationDebounce string `json:"generation_debounce" schema:"type:string,description:Debounce duration for generation,category:advanced,default:1s"`

	// ConsumerNameSuffix adds a suffix to consumer names for uniqueness in tests.
	ConsumerNameSuffix string `json:"consumer_name_suffix" schema:"type:string,description:Suffix for consumer names,category:advanced"`

	// DeleteConsumerOnStop enables consumer cleanup on stop (for testing).
	DeleteConsumerOnStop bool `json:"delete_consumer_on_stop,omitempty" schema:"type:bool,description:Delete consumers on Stop,category:advanced,default:false"`

	// DefaultAgentVersion is used when an agent doesn't specify its version.
	DefaultAgentVersion string `json:"default_agent_version" schema:"type:string,description:Default agent version for OASF records,category:advanced,default:1.0.0"`

	// DefaultAuthors is used when authors aren't specified in entity metadata.
	DefaultAuthors []string `json:"default_authors" schema:"type:array,description:Default authors for OASF records,category:advanced"`

	// IncludeExtensions enables SemStreams-specific extensions in OASF output.
	IncludeExtensions bool `json:"include_extensions" schema:"type:bool,description:Include SemStreams extensions,category:advanced,default:true"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "entity_changes",
					Subject:     "entity.state.>",
					Type:        "kv-watch",
					Required:    true,
					Description: "Watch for entity state changes via KV watch",
				},
				{
					Name:        "generate_request",
					Subject:     "oasf.generate.request",
					Type:        "nats",
					Required:    false,
					Description: "On-demand OASF generation requests",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "oasf_records",
					Subject:     "oasf.record.generated.*",
					Type:        "jetstream",
					Required:    true,
					Description: "Generated OASF records",
				},
			},
		},
		StreamName:          "ENTITY",
		EntityKVBucket:      "ENTITY_STATES",
		OASFKVBucket:        "OASF_RECORDS",
		WatchPattern:        ">",
		GenerationDebounce:  "1s",
		DefaultAgentVersion: "1.0.0",
		IncludeExtensions:   true,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Ports == nil {
		return fmt.Errorf("ports configuration is required")
	}

	if c.EntityKVBucket == "" {
		return fmt.Errorf("entity_kv_bucket is required")
	}

	if c.OASFKVBucket == "" {
		return fmt.Errorf("oasf_kv_bucket is required")
	}

	// Validate debounce duration if set
	if c.GenerationDebounce != "" {
		if _, err := time.ParseDuration(c.GenerationDebounce); err != nil {
			return fmt.Errorf("invalid generation_debounce: %w", err)
		}
	}

	return nil
}

// GetGenerationDebounce returns the debounce duration.
func (c *Config) GetGenerationDebounce() time.Duration {
	if c.GenerationDebounce == "" {
		return time.Second
	}
	d, err := time.ParseDuration(c.GenerationDebounce)
	if err != nil {
		return time.Second
	}
	return d
}
