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
// The live set is the team-hint pair (/research, /optimize + /autoresearch
// alias): beta.159's dispatch rejects unknown slash-prefixed messages instead
// of passing them through as chat, so the hint commands re-enter the front
// door explicitly (see cmd/semteams/commands/teamhint). The dev-from-task
// bridge command (/implement-spec, commands/implementspec) is parked with the
// dev-side packs (ADR-058); re-register it here when that pack is re-wired.
func registerProductCommands(_ types.PlatformMeta, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	hints := []struct {
		name, team, help string
	}{
		{"research", "research", "/research <question> - Route an evidence-gathering research ask through the coordinator"},
		{"optimize", "optimize", "/optimize <target> - Route a measurable optimization ask through the coordinator"},
		{"autoresearch", "optimize", "/autoresearch <target> - Alias of /optimize"},
	}
	for _, h := range hints {
		if err := agenticdispatch.RegisterCommand(h.name, teamhint.New(h.name, h.team, h.help, logger)); err != nil {
			return fmt.Errorf("register %s command: %w", h.name, err)
		}
	}
	logger.Info("Registered product commands", slog.Int("count", len(hints)))
	return nil
}
