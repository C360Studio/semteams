package service

// ComponentManagerConfig configures the ComponentManager service.
//
// The ComponentManager orchestrates component lifecycle (create, start, stop)
// and optionally watches for configuration changes via NATS KV.
type ComponentManagerConfig struct {
	// WatchConfig enables dynamic configuration updates via NATS KV bucket.
	// When true, the manager watches for changes to component configurations
	// and applies them at runtime without service restart.
	WatchConfig bool `json:"watch_config" schema:"type:boolean,description:Enable config watching via NATS KV,default:false,category:basic"`

	// EnabledComponents lists component names to enable.
	// If empty, all registered components are enabled.
	// Use this to selectively enable specific components in a deployment.
	EnabledComponents []string `json:"enabled_components" schema:"type:array,description:List of component names to enable (empty=all),category:basic"`
}

// DefaultComponentManagerConfig returns the default configuration.
func DefaultComponentManagerConfig() ComponentManagerConfig {
	return ComponentManagerConfig{
		WatchConfig:       false,
		EnabledComponents: nil, // nil means all components enabled
	}
}

// Validate checks if the configuration is valid
func (c ComponentManagerConfig) Validate() error {
	// No specific validation needed for component manager config
	// Component names are validated when components are created
	// WatchConfig is a boolean (no validation needed)
	// EnabledComponents can be empty (all components enabled)
	return nil
}
