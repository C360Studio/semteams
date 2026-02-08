package agenticdispatch

import (
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
)

// init registers dispatch payload types with the global PayloadRegistry.
func init() {
	// Register SignalMessage payload factory
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      agentic.Domain,
		Category:    agentic.CategorySignalMessage,
		Version:     agentic.SchemaVersion,
		Description: "Control signal sent to a loop",
		Factory:     func() any { return &SignalMessage{} },
	})
	if err != nil {
		panic("failed to register SignalMessage payload: " + err.Error())
	}
}
