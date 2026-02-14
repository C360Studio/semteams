// Package actions provides action executors for the workflow processor.
package actions

import (
	"context"
	"encoding/json"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
)

// Result represents the result of an action execution
type Result struct {
	Success  bool            `json:"success"`
	Output   json.RawMessage `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
	Duration time.Duration   `json:"duration"`
}

// Context provides dependencies for action execution
type Context struct {
	NATSClient  *natsclient.Client
	Timeout     time.Duration
	ExecutionID string // Workflow execution ID for callback correlation

	// Multi-agent hierarchy context
	ParentLoopID string // Parent loop ID for nested agents
	Depth        int    // Current depth in agent tree (0 = root)
	MaxDepth     int    // Maximum allowed depth for spawned agents

	// Pre-constructed context for embedded context pattern
	EmbeddedContext *agentic.ConstructedContext
}

// Action is the interface for executable actions
type Action interface {
	Execute(ctx context.Context, actx *Context) Result
}
