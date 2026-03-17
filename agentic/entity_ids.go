package agentic

import (
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/message"
)

// ModelEndpointEntityID constructs a 6-part entity ID for a model registry endpoint.
// Format: {org}.{platform}.agent.model-registry.endpoint.{endpointName}
//
// Example: ModelEndpointEntityID("c360", "ops", "claude-sonnet")
// Returns: "c360.ops.agent.model-registry.endpoint.claude-sonnet"
//
// Panics if any input part is empty or contains a dot, as these represent
// programming errors — the caller is responsible for supplying well-formed identifiers.
func ModelEndpointEntityID(org, platform, endpointName string) string {
	if err := validatePart("org", org); err != nil {
		panic(fmt.Sprintf("ModelEndpointEntityID: %s", err))
	}
	if err := validatePart("platform", platform); err != nil {
		panic(fmt.Sprintf("ModelEndpointEntityID: %s", err))
	}
	if err := validatePart("endpointName", endpointName); err != nil {
		panic(fmt.Sprintf("ModelEndpointEntityID: %s", err))
	}

	id := fmt.Sprintf("%s.%s.agent.model-registry.endpoint.%s", org, platform, endpointName)

	if !message.IsValidEntityID(id) {
		panic(fmt.Sprintf("ModelEndpointEntityID: constructed id %q failed IsValidEntityID — check input values", id))
	}

	return id
}

// LoopExecutionEntityID constructs a 6-part entity ID for an agentic loop execution.
// Format: {org}.{platform}.agent.agentic-loop.execution.{loopID}
//
// Example: LoopExecutionEntityID("c360", "ops", "abc123")
// Returns: "c360.ops.agent.agentic-loop.execution.abc123"
//
// Panics if any input part is empty or contains a dot, as these represent
// programming errors — the caller is responsible for supplying well-formed identifiers.
func LoopExecutionEntityID(org, platform, loopID string) string {
	if err := validatePart("org", org); err != nil {
		panic(fmt.Sprintf("LoopExecutionEntityID: %s", err))
	}
	if err := validatePart("platform", platform); err != nil {
		panic(fmt.Sprintf("LoopExecutionEntityID: %s", err))
	}
	if err := validatePart("loopID", loopID); err != nil {
		panic(fmt.Sprintf("LoopExecutionEntityID: %s", err))
	}

	id := fmt.Sprintf("%s.%s.agent.agentic-loop.execution.%s", org, platform, loopID)

	if !message.IsValidEntityID(id) {
		panic(fmt.Sprintf("LoopExecutionEntityID: constructed id %q failed IsValidEntityID — check input values", id))
	}

	return id
}

// TrajectoryStepEntityID constructs a 6-part entity ID for a trajectory step.
// Format: {org}.{platform}.agent.agentic-loop.step.{loopID}-{stepIndex}
//
// Example: TrajectoryStepEntityID("c360", "ops", "abc123", 0)
// Returns: "c360.ops.agent.agentic-loop.step.abc123-0"
//
// Panics if any input part is empty or contains a dot, as these represent
// programming errors — the caller is responsible for supplying well-formed identifiers.
func TrajectoryStepEntityID(org, platform, loopID string, stepIndex int) string {
	if err := validatePart("org", org); err != nil {
		panic(fmt.Sprintf("TrajectoryStepEntityID: %s", err))
	}
	if err := validatePart("platform", platform); err != nil {
		panic(fmt.Sprintf("TrajectoryStepEntityID: %s", err))
	}
	if err := validatePart("loopID", loopID); err != nil {
		panic(fmt.Sprintf("TrajectoryStepEntityID: %s", err))
	}
	if stepIndex < 0 {
		panic(fmt.Sprintf("TrajectoryStepEntityID: stepIndex must be non-negative, got %d", stepIndex))
	}

	instance := fmt.Sprintf("%s-%d", loopID, stepIndex)
	id := fmt.Sprintf("%s.%s.agent.agentic-loop.step.%s", org, platform, instance)

	if !message.IsValidEntityID(id) {
		panic(fmt.Sprintf("TrajectoryStepEntityID: constructed id %q failed IsValidEntityID — check input values", id))
	}

	return id
}

// validatePart checks that a single entity ID component is non-empty and contains no dots.
// Dots are reserved as part separators in the 6-part entity ID format.
func validatePart(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	if strings.Contains(value, ".") {
		return fmt.Errorf("%s %q must not contain dots (dots are entity ID separators)", name, value)
	}
	return nil
}
