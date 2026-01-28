// Package agenticloop provides the agentic loop orchestrator component.
// It manages the state machine for agentic loops, coordinating between
// model calls and tool executions while capturing execution trajectories.
package agenticloop

// ArchitectSplit handles the architect-to-editor split logic
// The split logic is implemented in handlers.go as part of HandleModelResponse
// This file exists to satisfy the test structure but the actual logic
// is integrated into the message handler for better cohesion
