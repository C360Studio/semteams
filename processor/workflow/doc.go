// Package workflow provides a workflow processor for orchestrating multi-step
// agentic patterns that require loops, limits, and timeouts beyond what the
// rules engine can handle.
//
// The workflow processor handles patterns like:
//   - reviewer -> fixer -> reviewer (max 3x)
//   - plan -> implement -> test (with timeout)
//   - multi-agent coordination with step tracking
//
// Key features:
//   - Workflow definition loading from KV bucket
//   - Step tracking and sequencing
//   - Loop limits (max_iterations)
//   - Workflow timeout enforcement
//   - Variable interpolation (${trigger.*}, ${steps.*}, ${execution.id})
//
// Actions supported:
//   - call: NATS request/response
//   - publish: Fire-and-forget NATS publish
//   - set_state: Entity state mutation via graph processor
//
// NATS Subjects:
//   - workflow.trigger.{id}: Start workflow
//   - workflow.step.complete.{exec_id}: Step completed
//   - workflow.events: Execution lifecycle events
//
// KV Buckets:
//   - WORKFLOW_DEFINITIONS: Workflow JSON definitions
//   - WORKFLOW_EXECUTIONS: Execution state (7d TTL)
package workflow
