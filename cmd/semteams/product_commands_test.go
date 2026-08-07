package main

import (
	"testing"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"
)

// The /implement-spec command is parked with the dev-from-task pack
// (ADR-058): registerProductCommands must register NOTHING until the pack
// is re-wired. This pins the parked state so an accidental re-registration
// shows up as a test failure rather than a silently advertised command.
func TestRegisterProductCommands_RegistersNothingWhileParked(t *testing.T) {
	agenticdispatch.ClearGlobalCommands()
	t.Cleanup(agenticdispatch.ClearGlobalCommands)

	err := registerProductCommands(types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil)
	if err != nil {
		t.Fatalf("registerProductCommands returned error: %v", err)
	}
	if cmds := agenticdispatch.ListRegisteredCommands(); len(cmds) != 0 {
		t.Fatalf("expected no product commands while dev packs are parked, got %v", cmds)
	}
}
