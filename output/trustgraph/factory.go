package trustgraph

import (
	"encoding/json"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// CreateComponent creates a TrustGraph output component following the service pattern.
func CreateComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()

	if len(rawConfig) > 0 {
		var userConfig Config
		if err := component.SafeUnmarshal(rawConfig, &userConfig); err != nil {
			return nil, errs.Wrap(err, "trustgraph-output-factory", "create", "config parsing")
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
		if userConfig.KGCoreID != "" {
			cfg.KGCoreID = userConfig.KGCoreID
		}
		if userConfig.User != "" {
			cfg.User = userConfig.User
		}
		if userConfig.Collection != "" {
			cfg.Collection = userConfig.Collection
		}
		if userConfig.BatchSize > 0 {
			cfg.BatchSize = userConfig.BatchSize
		}
		if userConfig.FlushInterval != "" {
			cfg.FlushInterval = userConfig.FlushInterval
		}
		if len(userConfig.EntityPrefixes) > 0 {
			cfg.EntityPrefixes = userConfig.EntityPrefixes
		}
		if len(userConfig.ExcludeSources) > 0 {
			cfg.ExcludeSources = userConfig.ExcludeSources
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
		return nil, errs.WrapInvalid(err, "trustgraph-output-factory", "create", "config validation")
	}

	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "trustgraph-output-factory", "create", "NATS client is required")
	}

	return New(ComponentDeps{
		Name:            "trustgraph-output",
		Config:          cfg,
		NATSClient:      deps.NATSClient,
		MetricsRegistry: deps.MetricsRegistry,
		Logger:          deps.GetLoggerWithComponent("trustgraph-output"),
	}), nil
}

// Register registers the TrustGraph output component with the given registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "trustgraph_output",
		Factory:     CreateComponent,
		Schema:      componentSchema,
		Type:        "output",
		Protocol:    "trustgraph",
		Domain:      "bridge",
		Description: "Exports SemStreams entities to TrustGraph knowledge cores via REST API",
		Version:     "1.0.0",
	})
}
