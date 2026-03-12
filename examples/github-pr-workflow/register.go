package githubprworkflow

import "github.com/c360studio/semstreams/component"

// prWorkflowSchema defines the configuration schema for the PR workflow spawner.
var prWorkflowSchema = component.ConfigSchema{
	Properties: map[string]component.PropertySchema{
		"model": {
			Type:        "string",
			Description: "Model endpoint name for agent tasks",
		},
		"token_budget": {
			Type:        "integer",
			Description: "Maximum tokens per workflow execution",
		},
		"max_review_cycles": {
			Type:        "integer",
			Description: "Maximum review rejection/retry loops",
		},
	},
}

// Register registers the PR workflow spawner component with the given registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        componentName,
		Factory:     NewComponent,
		Schema:      prWorkflowSchema,
		Type:        "processor",
		Protocol:    "pr-workflow",
		Domain:      "github",
		Description: "Spawns agent tasks for the GitHub issue-to-PR pipeline",
		Version:     componentVersion,
	})
}
