package main

import (
	"testing"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"
)

// Every public team token — live AND parked — must be a registered hint
// command: beta.159's dispatch rejects unknown slash messages, so a missing
// hint means "/research …" (or "/spec …") dead-ends with "Unknown command"
// instead of reaching the coordinator, which is what answers parked asks
// honestly. The bridge SEMANTICS of /implement-spec stay parked (ADR-058);
// only its hint routing is live.
func TestRegisterProductCommands_TeamHintsOnly(t *testing.T) {
	agenticdispatch.ClearGlobalCommands()
	t.Cleanup(agenticdispatch.ClearGlobalCommands)

	err := registerProductCommands(types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil)
	if err != nil {
		t.Fatalf("registerProductCommands returned error: %v", err)
	}
	cmds := agenticdispatch.ListRegisteredCommands()
	want := []string{
		"research", "optimize", "autoresearch",
		"spec", "create-change", "dev-via-test", "build", "dev", "implement-spec",
	}
	for _, name := range want {
		if _, ok := cmds[name]; !ok {
			t.Errorf("team-hint command %q not registered — beta.159 dispatch would reject /%s as Unknown command", name, name)
		}
	}
	if len(cmds) != len(want) {
		t.Errorf("expected exactly the %d team-hint commands, got %v", len(want), cmds)
	}
}
