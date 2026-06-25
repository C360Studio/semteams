package main

import (
	"strings"
	"testing"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"
)

func TestRegisterProductCommands_RegistersImplementSpec(t *testing.T) {
	agenticdispatch.ClearGlobalCommands()
	t.Cleanup(agenticdispatch.ClearGlobalCommands)

	err := registerProductCommands(types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil)
	if err != nil {
		t.Fatalf("registerProductCommands returned error: %v", err)
	}
	if _, ok := agenticdispatch.ListRegisteredCommands()["implement-spec"]; !ok {
		t.Fatalf("implement-spec command was not registered")
	}
}

func TestRegisterProductCommands_RejectsDuplicateImplementSpec(t *testing.T) {
	agenticdispatch.ClearGlobalCommands()
	t.Cleanup(agenticdispatch.ClearGlobalCommands)

	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}
	if err := registerProductCommands(platform, nil); err != nil {
		t.Fatalf("first registerProductCommands returned error: %v", err)
	}

	err := registerProductCommands(platform, nil)
	if err == nil || !strings.Contains(err.Error(), `command "implement-spec" already registered`) {
		t.Fatalf("second registerProductCommands error = %v, want duplicate command error", err)
	}
}
