package input

import (
	"time"

	"github.com/c360studio/semstreams/bridge/trustgraph/vocab"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Config holds configuration for the TrustGraph input component.
type Config struct {
	// Ports defines input/output port configuration
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Endpoint is the TrustGraph REST API base URL
	Endpoint string `json:"endpoint" schema:"type:string,description:TrustGraph API base URL,default:http://localhost:8088"`

	// APIKey is an optional API key for authentication
	APIKey string `json:"api_key" schema:"type:string,description:API key for TrustGraph (optional)"`

	// APIKeyEnv is the environment variable containing the API key
	APIKeyEnv string `json:"api_key_env" schema:"type:string,description:Env var containing API key"`

	// PollInterval is the interval between polls
	PollInterval string `json:"poll_interval" schema:"type:string,description:Polling interval (e.g. 30s 5m),default:60s"`

	// Collections are the TrustGraph collections to import from
	Collections []string `json:"collections" schema:"type:array,description:TrustGraph collections to import from"`

	// KGCoreIDs are specific knowledge core IDs to import
	KGCoreIDs []string `json:"kg_core_ids" schema:"type:array,description:Specific knowledge core IDs to import"`

	// SubjectFilter is a URI prefix filter for subjects
	SubjectFilter string `json:"subject_filter" schema:"type:string,description:URI prefix filter for subjects"`

	// PredicateFilter are predicate URIs to include (empty = all)
	PredicateFilter []string `json:"predicate_filter" schema:"type:array,description:Predicate URIs to include (empty = all)"`

	// Limit is the max triples per poll
	Limit int `json:"limit" schema:"type:int,description:Max triples per poll,default:1000,min:1,max:10000"`

	// Source is the source identifier for imported triples
	Source string `json:"source" schema:"type:string,description:Source identifier for imported triples,default:trustgraph"`

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
	if c.PollInterval != "" {
		if _, err := time.ParseDuration(c.PollInterval); err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "invalid poll_interval")
		}
	}

	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "invalid timeout")
		}
	}

	if c.Limit < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "limit must be non-negative")
	}

	// Validate output ports
	if c.Ports != nil {
		hasOutput := false
		for _, output := range c.Ports.Outputs {
			if (output.Type == "nats" || output.Type == "jetstream") && output.Subject != "" {
				hasOutput = true
				break
			}
		}
		if !hasOutput {
			return errs.WrapInvalid(errs.ErrMissingConfig, "Config", "Validate", "at least one NATS output port with subject is required")
		}
	}

	return nil
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Endpoint:     "http://localhost:8088",
		PollInterval: "60s",
		Limit:        1000,
		Source:       "trustgraph",
		Timeout:      "30s",
		Ports: &component.PortConfig{
			Outputs: []component.PortDefinition{
				{
					Name:        "entity",
					Type:        "nats",
					Subject:     "entity.>",
					Required:    true,
					Description: "NATS subject for publishing imported entities",
				},
			},
		},
	}
}

// GetPollInterval returns the parsed poll interval duration.
func (c *Config) GetPollInterval() time.Duration {
	if c.PollInterval == "" {
		return 60 * time.Second
	}
	d, err := time.ParseDuration(c.PollInterval)
	if err != nil {
		return 60 * time.Second
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

// GetOutputSubject returns the first configured output subject.
func (c *Config) GetOutputSubject() string {
	if c.Ports == nil {
		return "entity.>"
	}
	for _, output := range c.Ports.Outputs {
		if (output.Type == "nats" || output.Type == "jetstream") && output.Subject != "" {
			return output.Subject
		}
	}
	return "entity.>"
}
