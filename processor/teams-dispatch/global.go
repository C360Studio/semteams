package teamsdispatch

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
//	import teamsdispatch "github.com/c360studio/semteams/processor/teams-dispatch"
//
//	func init() {
//	    teamsdispatch.RegisterCommand("spec", &SpecCommandExecutor{})
//	}
//
// Duplicate command names will cause registration to fail with an error.

import (
	"fmt"
	"sync"

	"github.com/c360studio/semstreams/pkg/errs"
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
		return errs.WrapInvalid(fmt.Errorf("name cannot be empty"), "GlobalRegistry", "RegisterCommand", "validate command name")
	}
	if executor == nil {
		panic("RegisterCommand: executor cannot be nil")
	}
	if _, exists := globalCommands[name]; exists {
		return errs.WrapInvalid(fmt.Errorf("command %q already registered", name), "GlobalRegistry", "RegisterCommand", "check duplicate command")
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
