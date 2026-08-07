package main

import (
	"testing"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"
)

// The live product-command set is the team-hint trio; /implement-spec is
// parked with the dev-from-task pack (ADR-058) and must NOT be registered
// until that pack is re-wired. beta.159's dispatch rejects unknown slash
// messages, so a missing hint command means "/research …" dead-ends with
// "Unknown command" instead of reaching the coordinator — this pins both
// sides of that boundary.
func TestRegisterProductCommands_TeamHintsOnly(t *testing.T) {
	agenticdispatch.ClearGlobalCommands()
	t.Cleanup(agenticdispatch.ClearGlobalCommands)

	err := registerProductCommands(types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil)
	if err != nil {
		t.Fatalf("registerProductCommands returned error: %v", err)
	}
	cmds := agenticdispatch.ListRegisteredCommands()
	for _, want := range []string{"research", "optimize", "autoresearch"} {
		if _, ok := cmds[want]; !ok {
			t.Errorf("team-hint command %q not registered — beta.159 dispatch would reject /%s as Unknown command", want, want)
		}
	}
	if _, ok := cmds["implement-spec"]; ok {
		t.Error("implement-spec is parked with the dev-from-task pack (ADR-058) and must not be registered")
	}
	if len(cmds) != 3 {
		t.Errorf("expected exactly the 3 team-hint commands, got %v", cmds)
	}
}
