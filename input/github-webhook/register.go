package githubwebhook

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// CreateInput is the factory function for creating GitHub webhook input components.
// It follows the standard component factory signature so that it can be registered
// with the component registry.
func CreateInput(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()

	if len(rawConfig) > 0 {
		var userConfig Config
		if err := component.SafeUnmarshal(rawConfig, &userConfig); err != nil {
			return nil, errs.Wrap(err, "github-webhook-factory", "create", "secure config parsing")
		}
		cfg = userConfig
	}

	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(
			fmt.Errorf("NATS client is required"),
			"github-webhook-factory", "create", "dependency validation",
		)
	}

	return NewInput(
		"github-webhook-input",
		deps.NATSClient,
		cfg,
		deps.GetLogger(),
	)
}

// Register registers the GitHub webhook input component with the supplied registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "github_webhook",
		Factory:     CreateInput,
		Schema:      githubWebhookSchema,
		Type:        "input",
		Protocol:    "http",
		Domain:      "github",
		Description: "GitHub webhook receiver for issue and PR events",
		Version:     "1.0.0",
	})
}
