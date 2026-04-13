package teamtools

import (
	"sync"

	"github.com/c360studio/semteams/teams"
)

var (
	globalRegistry = NewExecutorRegistry()
	registryMu     sync.RWMutex
)

// RegisterTool registers a tool executor globally via init().
// Thread-safe and can be called from any package's init() function.
//
// Example usage:
//
//	func init() {
//	    teamtools.RegisterTool("my_tool", &MyToolExecutor{})
//	}
func RegisterTool(name string, executor ToolExecutor) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	return globalRegistry.RegisterTool(name, executor)
}

// GetGlobalRegistry returns the global tool registry.
// Used by agentic-tools component to access registered tools.
func GetGlobalRegistry() *ExecutorRegistry {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return globalRegistry
}

// ListRegisteredTools returns all globally registered tool definitions.
func ListRegisteredTools() []teams.ToolDefinition {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return globalRegistry.ListTools()
}
