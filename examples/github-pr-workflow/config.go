package githubprworkflow

// Config holds configuration for the PR workflow spawner component.
type Config struct {
	// Model is the model endpoint name for agent tasks (default: "default")
	Model string `json:"model,omitempty"`

	// TokenBudget is the maximum tokens per workflow execution (default: 500000)
	TokenBudget int `json:"token_budget,omitempty"`

	// MaxReviewCycles is the maximum review rejection/retry loops (default: 3)
	MaxReviewCycles int `json:"max_review_cycles,omitempty"`

	// Ports defines the component's input and output ports
	Ports PortConfig `json:"ports,omitempty"`
}

// PortConfig defines port configuration.
type PortConfig struct {
	Inputs  []PortDef `json:"inputs,omitempty"`
	Outputs []PortDef `json:"outputs,omitempty"`
}

// PortDef defines a single port.
type PortDef struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Type    string `json:"type"`
	Stream  string `json:"stream,omitempty"`
}

// withDefaults returns the config with defaults applied.
func (c Config) withDefaults() Config {
	if c.Model == "" {
		c.Model = "default"
	}
	if c.TokenBudget <= 0 {
		c.TokenBudget = DefaultTokenBudget
	}
	if c.MaxReviewCycles <= 0 {
		c.MaxReviewCycles = MaxReviewCycles
	}
	return c
}
