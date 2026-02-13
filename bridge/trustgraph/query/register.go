package query

import (
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
)

func init() {
	// Register the TrustGraph query tool globally
	// This makes it available to all agentic-tools instances
	executor := NewDefaultExecutor()
	if err := agentictools.RegisterTool("trustgraph_query", executor); err != nil {
		// Don't panic - just log the error
		// The tool won't be available if registration fails
		// but the system should still function
		println("WARNING: failed to register trustgraph_query tool:", err.Error())
	}
}
