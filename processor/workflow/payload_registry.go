package workflow

import "github.com/c360studio/semstreams/component"

// init registers all workflow payload types with the global PayloadRegistry.
// This enables BaseMessage.UnmarshalJSON to recreate typed payloads from JSON
// when the message type matches one of the workflow types.
func init() {
	// Register TriggerPayload factory
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "trigger",
		Version:     "v1",
		Description: "Workflow trigger event",
		Factory:     func() any { return &TriggerPayload{} },
	})
	if err != nil {
		panic("failed to register TriggerPayload: " + err.Error())
	}

	// Register StepCompleteMessage factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "step_complete",
		Version:     "v1",
		Description: "Step completion from agents",
		Factory:     func() any { return &StepCompleteMessage{} },
	})
	if err != nil {
		panic("failed to register StepCompleteMessage: " + err.Error())
	}

	// Register event factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "event",
		Version:     "v1",
		Description: "Workflow lifecycle event",
		Factory:     func() any { return &event{} },
	})
	if err != nil {
		panic("failed to register event: " + err.Error())
	}
}
