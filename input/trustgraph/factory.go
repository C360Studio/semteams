package trustgraph

import (
	"encoding/json"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// CreateComponent creates a TrustGraph input component following the service pattern.
func CreateComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()

	if len(rawConfig) > 0 {
		var userConfig Config
		if err := component.SafeUnmarshal(rawConfig, &userConfig); err != nil {
			return nil, errs.Wrap(err, "trustgraph-input-factory", "create", "config parsing")
		}

		// Apply user overrides
		if userConfig.Ports != nil {
			cfg.Ports = userConfig.Ports
		}
		if userConfig.Endpoint != "" {
			cfg.Endpoint = userConfig.Endpoint
		}
		if userConfig.APIKey != "" {
			cfg.APIKey = userConfig.APIKey
		}
		if userConfig.APIKeyEnv != "" {
			cfg.APIKeyEnv = userConfig.APIKeyEnv
		}
		if userConfig.PollInterval != "" {
			cfg.PollInterval = userConfig.PollInterval
		}
		if len(userConfig.Collections) > 0 {
			cfg.Collections = userConfig.Collections
		}
		if len(userConfig.KGCoreIDs) > 0 {
			cfg.KGCoreIDs = userConfig.KGCoreIDs
		}
		if userConfig.SubjectFilter != "" {
			cfg.SubjectFilter = userConfig.SubjectFilter
		}
		if len(userConfig.PredicateFilter) > 0 {
			cfg.PredicateFilter = userConfig.PredicateFilter
		}
		if userConfig.Limit > 0 {
			cfg.Limit = userConfig.Limit
		}
		if userConfig.Source != "" {
			cfg.Source = userConfig.Source
		}
		if userConfig.Timeout != "" {
			cfg.Timeout = userConfig.Timeout
		}

		// Merge vocab config
		if userConfig.Vocab.OrgMappings != nil {
			cfg.Vocab.OrgMappings = userConfig.Vocab.OrgMappings
		}
		if userConfig.Vocab.URIMappings != nil {
			cfg.Vocab.URIMappings = userConfig.Vocab.URIMappings
		}
		if userConfig.Vocab.PredicateMappings != nil {
			cfg.Vocab.PredicateMappings = userConfig.Vocab.PredicateMappings
		}
		if userConfig.Vocab.DefaultOrg != "" {
			cfg.Vocab.DefaultOrg = userConfig.Vocab.DefaultOrg
		}
		if userConfig.Vocab.DefaultURIBase != "" {
			cfg.Vocab.DefaultURIBase = userConfig.Vocab.DefaultURIBase
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "trustgraph-input-factory", "create", "config validation")
	}

	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "trustgraph-input-factory", "create", "NATS client is required")
	}

	return New(ComponentDeps{
		Name:            "trustgraph-input",
		Config:          cfg,
		NATSClient:      deps.NATSClient,
		MetricsRegistry: deps.MetricsRegistry,
		Logger:          deps.GetLoggerWithComponent("trustgraph-input"),
	}), nil
}

// Register registers the TrustGraph input component with the given registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "trustgraph_input",
		Factory:     CreateComponent,
		Schema:      componentSchema,
		Type:        "input",
		Protocol:    "trustgraph",
		Domain:      "bridge",
		Description: "Imports entities from TrustGraph knowledge graph via REST API",
		Version:     "1.0.0",
	})
}
