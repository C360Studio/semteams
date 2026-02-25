package reactive

import (
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Config represents the configuration for the reactive workflow engine.
type Config struct {
	// Port configuration for inputs and outputs
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration for workflow inputs and outputs,category:basic"`

	// StateBucket is the KV bucket for workflow execution state
	StateBucket string `json:"state_bucket" schema:"type:string,description:NATS KV bucket for workflow execution state,default:REACTIVE_WORKFLOW_STATE,category:advanced,required"`

	// CallbackStreamName is the JetStream stream for callback messages
	CallbackStreamName string `json:"callback_stream_name" schema:"type:string,description:JetStream stream name for callback messages,default:WORKFLOW_CALLBACKS,category:advanced,required"`

	// EventStreamName is the JetStream stream for workflow events
	EventStreamName string `json:"event_stream_name" schema:"type:string,description:JetStream stream name for workflow events,default:WORKFLOW_EVENTS,category:advanced,required"`

	// DefaultTimeout is the default timeout for workflows without explicit timeout
	DefaultTimeout string `json:"default_timeout" schema:"type:string,description:Default timeout for workflows (e.g. 10m),default:10m,category:basic"`

	// DefaultMaxIterations is the default max iterations for loop workflows
	DefaultMaxIterations int `json:"default_max_iterations" schema:"type:int,description:Default max iterations for loop workflows,default:10,min:1,max:100,category:basic"`

	// CleanupRetention is how long to retain completed executions
	CleanupRetention string `json:"cleanup_retention" schema:"type:string,description:How long to retain completed executions before cleanup,default:24h,category:advanced"`

	// CleanupInterval is how often to run cleanup
	CleanupInterval string `json:"cleanup_interval" schema:"type:string,description:How often to run cleanup of completed executions,default:1h,category:advanced"`

	// TaskTimeoutDefault is the default timeout for async tasks
	TaskTimeoutDefault string `json:"task_timeout_default" schema:"type:string,description:Default timeout for async tasks,default:5m,category:advanced"`

	// ConsumerNamePrefix is prepended to consumer names for uniqueness
	ConsumerNamePrefix string `json:"consumer_name_prefix,omitempty" schema:"type:string,description:Prefix for NATS consumer names,category:advanced"`

	// EnableMetrics enables Prometheus metrics
	EnableMetrics bool `json:"enable_metrics" schema:"type:bool,description:Enable Prometheus metrics,default:true,category:advanced"`
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if strings.TrimSpace(c.StateBucket) == "" {
		return errs.WrapInvalid(fmt.Errorf("state_bucket is required"), "reactive-config", "Validate", "validate state_bucket")
	}

	if strings.TrimSpace(c.CallbackStreamName) == "" {
		return errs.WrapInvalid(fmt.Errorf("callback_stream_name is required"), "reactive-config", "Validate", "validate callback_stream_name")
	}

	if strings.TrimSpace(c.EventStreamName) == "" {
		return errs.WrapInvalid(fmt.Errorf("event_stream_name is required"), "reactive-config", "Validate", "validate event_stream_name")
	}

	if strings.TrimSpace(c.DefaultTimeout) == "" {
		return errs.WrapInvalid(fmt.Errorf("default_timeout is required"), "reactive-config", "Validate", "validate default_timeout")
	}

	// Parse and validate default timeout
	duration, err := time.ParseDuration(c.DefaultTimeout)
	if err != nil {
		return errs.WrapInvalid(err, "reactive-config", "Validate", "parse default_timeout")
	}
	if duration <= 0 {
		return errs.WrapInvalid(fmt.Errorf("default_timeout must be positive"), "reactive-config", "Validate", "validate default_timeout value")
	}

	if c.DefaultMaxIterations <= 0 {
		return errs.WrapInvalid(fmt.Errorf("default_max_iterations must be greater than 0"), "reactive-config", "Validate", "validate default_max_iterations")
	}

	// Validate cleanup retention if provided
	if strings.TrimSpace(c.CleanupRetention) != "" {
		_, err := time.ParseDuration(c.CleanupRetention)
		if err != nil {
			return errs.WrapInvalid(err, "reactive-config", "Validate", "parse cleanup_retention")
		}
	}

	// Validate cleanup interval if provided
	if strings.TrimSpace(c.CleanupInterval) != "" {
		_, err := time.ParseDuration(c.CleanupInterval)
		if err != nil {
			return errs.WrapInvalid(err, "reactive-config", "Validate", "parse cleanup_interval")
		}
	}

	// Validate task timeout if provided
	if strings.TrimSpace(c.TaskTimeoutDefault) != "" {
		_, err := time.ParseDuration(c.TaskTimeoutDefault)
		if err != nil {
			return errs.WrapInvalid(err, "reactive-config", "Validate", "parse task_timeout_default")
		}
	}

	return nil
}

// GetDefaultTimeout returns the parsed default timeout duration.
func (c Config) GetDefaultTimeout() time.Duration {
	d, _ := time.ParseDuration(c.DefaultTimeout)
	return d
}

// GetCleanupRetention returns the parsed cleanup retention duration.
func (c Config) GetCleanupRetention() time.Duration {
	if c.CleanupRetention == "" {
		return 24 * time.Hour
	}
	d, _ := time.ParseDuration(c.CleanupRetention)
	return d
}

// GetCleanupInterval returns the parsed cleanup interval duration.
func (c Config) GetCleanupInterval() time.Duration {
	if c.CleanupInterval == "" {
		return time.Hour
	}
	d, _ := time.ParseDuration(c.CleanupInterval)
	return d
}

// GetTaskTimeoutDefault returns the parsed task timeout duration.
func (c Config) GetTaskTimeoutDefault() time.Duration {
	if c.TaskTimeoutDefault == "" {
		return 5 * time.Minute
	}
	d, _ := time.ParseDuration(c.TaskTimeoutDefault)
	return d
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		StateBucket:          "REACTIVE_WORKFLOW_STATE",
		CallbackStreamName:   "WORKFLOW_CALLBACKS",
		EventStreamName:      "WORKFLOW_EVENTS",
		DefaultTimeout:       "10m",
		DefaultMaxIterations: 10,
		CleanupRetention:     "24h",
		CleanupInterval:      "1h",
		TaskTimeoutDefault:   "5m",
		EnableMetrics:        true,
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "reactive.trigger",
					Type:        "jetstream",
					Subject:     "reactive.trigger.>",
					StreamName:  "WORKFLOW_EVENTS",
					Required:    true,
					Description: "Workflow trigger events",
				},
				{
					Name:        "reactive.callback",
					Type:        "jetstream",
					Subject:     "workflow.callback.>",
					StreamName:  "WORKFLOW_CALLBACKS",
					Required:    true,
					Description: "Async callback results",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "reactive.events",
					Type:        "jetstream",
					Subject:     "reactive.events.*",
					StreamName:  "WORKFLOW_EVENTS",
					Description: "Workflow lifecycle events",
				},
				{
					Name:        "reactive.action",
					Type:        "jetstream",
					Subject:     "reactive.action.>",
					StreamName:  "WORKFLOW_EVENTS",
					Description: "Action dispatch messages",
				},
			},
			KVWrite: []component.PortDefinition{
				{
					Name:        "state",
					Type:        "kv-write",
					Bucket:      "REACTIVE_WORKFLOW_STATE",
					Description: "Workflow execution state storage",
				},
			},
		},
	}
}
