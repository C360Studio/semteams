package rule

import (
	"encoding/json"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/pkg/cache"
)

// Config holds configuration for the RuleProcessor
type Config struct {
	// Port configuration for inputs and outputs
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration for inputs (KV watch: ENTITY_STATES PREDICATE_INDEX) and outputs (NATS: control commands),category:basic"`

	// Rule configuration sources
	RulesFiles  []string     `json:"rules_files" schema:"type:array,description:Paths to JSON rule definition files,default:[],category:basic"`
	InlineRules []Definition `json:"inline_rules,omitempty" schema:"type:array,description:Inline rule definitions (alternative to files),category:basic"`

	// Message cache configuration (not exposed in schema - internal config)
	MessageCache cache.Config `json:"message_cache"`

	// Buffer window size for time-window analysis
	BufferWindowSize string `json:"buffer_window_size" schema:"type:string,description:Time window for message buffering (e.g. '10m'),default:10m,category:advanced"`

	// Alert cooldown period to prevent spam
	AlertCooldownPeriod string `json:"alert_cooldown_period" schema:"type:string,description:Minimum time between repeated alerts (e.g. '2m'),default:2m,category:advanced"`

	// Graph processor integration
	EnableGraphIntegration bool `json:"enable_graph_integration" schema:"type:bool,description:Enable graph entity creation from rules,default:true,category:basic"`

	// NATS KV patterns to watch for entity changes (e.g., 'telemetry.robotics.>')
	EntityWatchPatterns []string `json:"entity_watch_patterns" schema:"type:array,description:NATS KV patterns to watch for entity changes (e.g. 'telemetry.robotics.>'),category:advanced"`

	// Debounce delay for rule evaluation (settling time for entity state)
	// Default is 0 (disabled) to ensure rules evaluate against each state change.
	// Set to a positive value (e.g., 100) to batch rapid updates and evaluate final state only.
	DebounceDelayMs time.Duration `json:"debounce_delay_ms" schema:"type:int,description:Debounce delay in milliseconds for rule evaluation (0=disabled),default:0,category:advanced"`

	// JetStream consumer configuration (not exposed in schema - internal config)
	Consumer struct {
		Enabled        bool   `json:"enabled"`          // Enable JetStream consumer
		AckWaitSeconds int    `json:"ack_wait_seconds"` // Acknowledgment timeout
		MaxDeliver     int    `json:"max_deliver"`      // Max delivery attempts
		ReplayPolicy   string `json:"replay_policy"`    // "instant" or "original"
	} `json:"consumer"`
}

// MarshalJSON implements custom JSON marshaling for Config
func (c Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	return json.Marshal(&struct {
		DebounceDelayMs int `json:"debounce_delay_ms"`
		*Alias
	}{
		DebounceDelayMs: int(c.DebounceDelayMs / time.Millisecond),
		Alias:           (*Alias)(&c),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Config
func (c *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	aux := &struct {
		DebounceDelayMs int `json:"debounce_delay_ms"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	c.DebounceDelayMs = time.Duration(aux.DebounceDelayMs) * time.Millisecond
	return nil
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "entity_states",
					Type:        "kv-watch",
					Required:    true,
					Description: "Watch entity state changes from ENTITY_STATES KV bucket",
				},
				{
					Name:        "predicate_index",
					Type:        "kv-watch",
					Required:    false,
					Description: "Watch predicate index changes for pattern-based rules",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "control_commands",
					Type:        "nats",
					Subject:     "control.*.commands",
					Required:    false,
					Description: "Control commands based on rules",
				},
			},
		},
		MessageCache: cache.Config{
			Enabled:         true,
			Strategy:        cache.StrategyTTL,
			MaxSize:         1000,
			TTL:             30 * time.Second,
			CleanupInterval: 15 * time.Second,
			StatsInterval:   30 * time.Second,
		},
		BufferWindowSize:       "10m",
		AlertCooldownPeriod:    "2m",
		EnableGraphIntegration: true,
		DebounceDelayMs:        0, // Disabled by default for real-time rule evaluation
		Consumer: struct {
			Enabled        bool   `json:"enabled"`
			AckWaitSeconds int    `json:"ack_wait_seconds"`
			MaxDeliver     int    `json:"max_deliver"`
			ReplayPolicy   string `json:"replay_policy"`
		}{
			Enabled:        true,
			AckWaitSeconds: 30,
			MaxDeliver:     3,
			ReplayPolicy:   "instant",
		},
	}
}
