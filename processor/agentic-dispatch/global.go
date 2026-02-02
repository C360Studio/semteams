package agenticdispatch

// Global command registration
//
// External packages can register custom commands with the agentic-dispatch
// component via init() functions using RegisterCommand. Commands are loaded
// by the component during initialization, after built-in commands are registered.
//
// Example usage:
//
//	package semspec
//
//	import agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
//
//	func init() {
//	    agenticdispatch.RegisterCommand("spec", &SpecCommandExecutor{})
//	}
//
// Duplicate command names will cause registration to fail with an error.

import (
	"fmt"
	"sync"
)

var (
	globalCommands = make(map[string]CommandExecutor)
	globalMu       sync.RWMutex
)

// RegisterCommand registers a command executor globally via init().
// Returns an error if the command name is empty or already registered.
// Panics if executor is nil (programmer error).
func RegisterCommand(name string, executor CommandExecutor) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if name == "" {
		return fmt.Errorf("register global command: name cannot be empty")
	}
	if executor == nil {
		panic("RegisterCommand: executor cannot be nil")
	}
	if _, exists := globalCommands[name]; exists {
		return fmt.Errorf("register global command: command %q already registered", name)
	}
	globalCommands[name] = executor
	return nil
}

// ListRegisteredCommands returns a copy of all globally registered commands.
// The returned map is safe to mutate without affecting the internal registry.
func ListRegisteredCommands() map[string]CommandExecutor {
	globalMu.RLock()
	defer globalMu.RUnlock()

	result := make(map[string]CommandExecutor, len(globalCommands))
	for k, v := range globalCommands {
		result[k] = v
	}
	return result
}

// ClearGlobalCommands removes all globally registered commands.
// This is intended for testing use only.
func ClearGlobalCommands() {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalCommands = make(map[string]CommandExecutor)
}
