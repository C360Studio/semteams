package main

import (
	"log/slog"

	"github.com/c360studio/semstreams/types"
)

// registerProductCommands wires SemTeams-local slash commands into the
// semstreams dispatch command registry. These remain power-user shortcuts for
// governed graph actions, not a second control plane.
//
// Currently registers nothing: the only product command, /implement-spec
// (the dev-from-task bridge in commands/implementspec), is parked with the
// dev-side packs pending the canonical-predicate migration (ADR-058).
// Re-register it here when the dev-from-task pack is re-wired.
func registerProductCommands(_ types.PlatformMeta, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("Registered product commands", slog.Int("count", 0))
	return nil
}
