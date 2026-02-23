package workflow

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Config represents the configuration for the workflow processor
type Config struct {
	// Port configuration for inputs and outputs
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration for workflow inputs and outputs,category:basic"`

	// KV bucket for workflow definitions
	DefinitionsBucket string `json:"definitions_bucket" schema:"type:string,description:NATS KV bucket for workflow definitions,default:WORKFLOW_DEFINITIONS,category:advanced,required"`

	// KV bucket for workflow executions
	ExecutionsBucket string `json:"executions_bucket" schema:"type:string,description:NATS KV bucket for workflow execution state,default:WORKFLOW_EXECUTIONS,category:advanced,required"`

	// JetStream stream name for workflow messages
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name for workflow messages,default:WORKFLOW,category:advanced,required"`

	// Consumer name suffix for unique consumer identification
	ConsumerNameSuffix string `json:"consumer_name_suffix,omitempty" schema:"type:string,description:Suffix appended to consumer names for uniqueness,category:advanced"`

	// Default timeout for workflows without explicit timeout
	DefaultTimeout string `json:"default_timeout" schema:"type:string,description:Default timeout for workflows (e.g. 10m),default:10m,category:basic"`

	// Default max iterations for loop workflows
	DefaultMaxIterations int `json:"default_max_iterations" schema:"type:int,description:Default max iterations for loop workflows,default:10,min:1,max:100,category:basic"`

	// Request timeout for call actions
	RequestTimeout string `json:"request_timeout" schema:"type:string,description:Timeout for NATS request/response calls,default:30s,category:advanced"`

	// WorkflowFiles is a list of file paths to JSON workflow definitions
	// to load at startup. Supports glob patterns.
	WorkflowFiles []string `json:"workflow_files,omitempty" schema:"type:array,items:string,description:Paths to JSON workflow definition files. Supports glob patterns.,category:basic"`
}

// Validate validates the configuration
func (c Config) Validate() error {
	if strings.TrimSpace(c.DefinitionsBucket) == "" {
		return errs.WrapInvalid(fmt.Errorf("definitions_bucket is required"), "workflow-config", "Validate", "validate definitions_bucket")
	}

	if strings.TrimSpace(c.ExecutionsBucket) == "" {
		return errs.WrapInvalid(fmt.Errorf("executions_bucket is required"), "workflow-config", "Validate", "validate executions_bucket")
	}

	if strings.TrimSpace(c.StreamName) == "" {
		return errs.WrapInvalid(fmt.Errorf("stream_name is required"), "workflow-config", "Validate", "validate stream_name")
	}

	if strings.TrimSpace(c.DefaultTimeout) == "" {
		return errs.WrapInvalid(fmt.Errorf("default_timeout is required"), "workflow-config", "Validate", "validate default_timeout")
	}

	// Parse default timeout to ensure it's valid
	duration, err := time.ParseDuration(c.DefaultTimeout)
	if err != nil {
		return errs.WrapInvalid(err, "workflow-config", "Validate", "parse default_timeout")
	}
	if duration <= 0 {
		return errs.WrapInvalid(fmt.Errorf("default_timeout must be positive"), "workflow-config", "Validate", "validate default_timeout value")
	}

	if c.DefaultMaxIterations <= 0 {
		return errs.WrapInvalid(fmt.Errorf("default_max_iterations must be greater than 0"), "workflow-config", "Validate", "validate default_max_iterations")
	}

	if strings.TrimSpace(c.RequestTimeout) != "" {
		_, err := time.ParseDuration(c.RequestTimeout)
		if err != nil {
			return errs.WrapInvalid(err, "workflow-config", "Validate", "parse request_timeout")
		}
	}

	// Validate glob patterns in workflow files
	for _, pattern := range c.WorkflowFiles {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return errs.WrapInvalid(err, "workflow-config", "Validate", fmt.Sprintf("validate workflow_files pattern %q", pattern))
		}
	}

	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		DefinitionsBucket:    "WORKFLOW_DEFINITIONS",
		ExecutionsBucket:     "WORKFLOW_EXECUTIONS",
		StreamName:           "WORKFLOW",
		DefaultTimeout:       "10m",
		DefaultMaxIterations: 10,
		RequestTimeout:       "30s",
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "workflow.trigger",
					Type:        "jetstream",
					Subject:     "workflow.trigger.>",
					StreamName:  "WORKFLOW",
					Required:    true,
					Description: "Workflow trigger events",
				},
				{
					Name:        "workflow.step.complete",
					Type:        "jetstream",
					Subject:     "workflow.step.complete.>",
					StreamName:  "WORKFLOW",
					Required:    true,
					Description: "Step completion events from external sources",
				},
				{
					Name:        "workflow.step.result",
					Type:        "jetstream",
					Subject:     "workflow.step.result.>",
					StreamName:  "WORKFLOW",
					Required:    true,
					Description: "Async step results from any executor (agentic, HTTP, custom)",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "workflow.events",
					Type:        "jetstream",
					Subject:     "workflow.events.*",
					StreamName:  "WORKFLOW",
					Description: "Workflow lifecycle events (started, completed, failed, timed_out, step.*)",
				},
				{
					Name:        "agent.task",
					Type:        "jetstream",
					Subject:     "agent.task.*",
					StreamName:  "AGENT",
					Description: "Agent task requests for call/publish_agent actions",
				},
			},
			KVWrite: []component.PortDefinition{
				{
					Name:        "definitions",
					Type:        "kv-write",
					Bucket:      "WORKFLOW_DEFINITIONS",
					Description: "Workflow definitions storage",
				},
				{
					Name:        "executions",
					Type:        "kv-write",
					Bucket:      "WORKFLOW_EXECUTIONS",
					Description: "Workflow execution state storage",
				},
			},
		},
	}
}
