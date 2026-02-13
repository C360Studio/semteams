package trustgraph

import (
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
	vocab "github.com/c360studio/semstreams/vocabulary/trustgraph"
)

// Config holds configuration for the TrustGraph output component.
type Config struct {
	// Ports defines input/output port configuration
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Endpoint is the TrustGraph REST API base URL
	Endpoint string `json:"endpoint" schema:"type:string,description:TrustGraph API base URL,default:http://localhost:8088"`

	// APIKey is an optional API key for authentication
	APIKey string `json:"api_key" schema:"type:string,description:API key for TrustGraph (optional)"`

	// APIKeyEnv is the environment variable containing the API key
	APIKeyEnv string `json:"api_key_env" schema:"type:string,description:Env var containing API key"`

	// KGCoreID is the knowledge core ID to write to
	KGCoreID string `json:"kg_core_id" schema:"type:string,description:Knowledge core ID to write to,default:semstreams-operational"`

	// User is the TrustGraph user for knowledge core operations
	User string `json:"user" schema:"type:string,description:TrustGraph user for knowledge core,default:semstreams"`

	// Collection is the TrustGraph collection name
	Collection string `json:"collection" schema:"type:string,description:TrustGraph collection name,default:operational"`

	// BatchSize is the number of triples per batch
	BatchSize int `json:"batch_size" schema:"type:int,description:Triples per batch,default:100,min:1,max:5000"`

	// FlushInterval is the max time before flush
	FlushInterval string `json:"flush_interval" schema:"type:string,description:Max time before flush,default:5s"`

	// EntityPrefixes are the entity ID prefixes to export (empty = all)
	EntityPrefixes []string `json:"entity_prefixes" schema:"type:array,description:Entity ID prefixes to export (empty = all)"`

	// ExcludeSources are source names to exclude (prevents re-export loops)
	ExcludeSources []string `json:"exclude_sources" schema:"type:array,description:Source names to exclude (prevents re-export loops)"`

	// Vocab contains vocabulary translation settings
	Vocab VocabConfig `json:"vocab" schema:"type:object,description:Vocabulary translation settings"`

	// Timeout is the HTTP request timeout
	Timeout string `json:"timeout" schema:"type:string,description:HTTP request timeout,default:30s"`
}

// VocabConfig holds vocabulary translation configuration.
type VocabConfig struct {
	// OrgMappings maps org segment to base URI
	OrgMappings map[string]string `json:"org_mappings"`

	// URIMappings maps URI domain to SemStreams org and defaults
	URIMappings map[string]vocab.URIMapping `json:"uri_mappings"`

	// PredicateMappings maps SemStreams predicates to RDF URIs
	PredicateMappings map[string]string `json:"predicate_mappings"`

	// DefaultOrg for URIs with unmapped domains
	DefaultOrg string `json:"default_org"`

	// DefaultURIBase for entities with unmapped orgs
	DefaultURIBase string `json:"default_uri_base"`
}

// Validate checks the configuration is valid.
func (c *Config) Validate() error {
	if c.FlushInterval != "" {
		if _, err := time.ParseDuration(c.FlushInterval); err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "invalid flush_interval")
		}
	}

	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "invalid timeout")
		}
	}

	if c.BatchSize < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "batch_size must be non-negative")
	}

	// Validate input ports
	if c.Ports != nil {
		hasInput := false
		for _, input := range c.Ports.Inputs {
			if (input.Type == "nats" || input.Type == "jetstream") && input.Subject != "" {
				hasInput = true
				break
			}
		}
		if !hasInput {
			return errs.WrapInvalid(errs.ErrMissingConfig, "Config", "Validate", "at least one NATS input port with subject is required")
		}
	}

	return nil
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Endpoint:       "http://localhost:8088",
		KGCoreID:       "semstreams-operational",
		User:           "semstreams",
		Collection:     "operational",
		BatchSize:      100,
		FlushInterval:  "5s",
		ExcludeSources: []string{"trustgraph"},
		Timeout:        "30s",
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "entity",
					Type:        "nats",
					Subject:     "entity.>",
					Required:    true,
					Description: "NATS subject for subscribing to entity changes",
				},
			},
		},
	}
}

// GetFlushInterval returns the parsed flush interval duration.
func (c *Config) GetFlushInterval() time.Duration {
	if c.FlushInterval == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(c.FlushInterval)
	if err != nil {
		return 5 * time.Second
	}
	return d
}

// GetTimeout returns the parsed HTTP timeout duration.
func (c *Config) GetTimeout() time.Duration {
	if c.Timeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetAPIKey returns the API key from direct config or environment variable.
func (c *Config) GetAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	// TODO: Read from environment if APIKeyEnv is set
	return ""
}

// ToTranslatorConfig converts VocabConfig to a vocab.TranslatorConfig.
func (c *VocabConfig) ToTranslatorConfig() vocab.TranslatorConfig {
	return vocab.TranslatorConfig{
		OrgMappings:       c.OrgMappings,
		URIMappings:       c.URIMappings,
		PredicateMappings: c.PredicateMappings,
		DefaultOrg:        c.DefaultOrg,
		DefaultURIBase:    c.DefaultURIBase,
	}
}

// GetInputSubject returns the first configured input subject.
func (c *Config) GetInputSubject() string {
	if c.Ports == nil {
		return "entity.>"
	}
	for _, input := range c.Ports.Inputs {
		if (input.Type == "nats" || input.Type == "jetstream") && input.Subject != "" {
			return input.Subject
		}
	}
	return "entity.>"
}
