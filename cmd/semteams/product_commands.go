package main

import (
	"fmt"
	"log/slog"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/commands/teamhint"
)

// registerProductCommands wires SemTeams-local slash commands into the
// semstreams dispatch command registry. These remain power-user shortcuts for
// governed graph actions, not a second control plane.
//
// beta.159's dispatch rejects unknown slash-prefixed messages instead of
// passing them through as chat, so EVERY public team token is a registered
// hint command that re-enters the front door (cmd/semteams/commands/teamhint)
// — including the parked-team tokens: the coordinator persona, not Go, is
// what answers a parked ask honestly ("hints, not bypasses"). The
// dev-from-task BRIDGE semantics of /implement-spec (run-scoped triple
// stamping in commands/implementspec) stay parked with their pack (ADR-058);
// the token routes to the coordinator as a plain hint until then.
func registerProductCommands(_ types.PlatformMeta, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	hints := []struct {
		name, team, help string
	}{
		// Live teams.
		{"research", "research", "/research <question> - Route an evidence-gathering research ask through the coordinator"},
		{"optimize", "optimize", "/optimize <target> - Route a measurable optimization ask through the coordinator"},
		{"autoresearch", "optimize", "/autoresearch <target> - Alias of /optimize"},
		// Parked teams (ADR-058): the coordinator explains they are not
		// available in this deployment and offers what is.
		{"spec", "spec", "/spec <request> - Spec authoring is parked in this deployment; the coordinator will respond"},
		{"create-change", "spec", "/create-change <request> - Alias of /spec (parked; coordinator responds)"},
		{"dev-via-test", "build", "/dev-via-test <request> - Implementation is parked in this deployment; the coordinator will respond"},
		{"build", "build", "/build <request> - Alias of /dev-via-test (parked; coordinator responds)"},
		{"dev", "build", "/dev <request> - Alias of /dev-via-test (parked; coordinator responds)"},
		{"implement-spec", "implement-spec", "/implement-spec <slug> - The spec-to-dev bridge is parked; the coordinator will respond"},
	}
	for _, h := range hints {
		if err := agenticdispatch.RegisterCommand(h.name, teamhint.New(h.name, h.team, h.help, logger)); err != nil {
			return fmt.Errorf("register %s command: %w", h.name, err)
		}
	}
	logger.Info("Registered product commands", slog.Int("count", len(hints)))
	return nil
}
