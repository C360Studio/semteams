package main

import (
	"fmt"
	"log/slog"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/commands/implementspec"
)

// registerProductCommands wires SemTeams-local slash commands into the
// semstreams dispatch command registry. These remain power-user shortcuts for
// governed graph actions, not a second control plane.
func registerProductCommands(platform types.PlatformMeta, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if err := agenticdispatch.RegisterCommand("implement-spec", implementspec.NewCommand(platform, logger)); err != nil {
		return fmt.Errorf("register implement-spec command: %w", err)
	}
	logger.Info("Registered product commands", slog.Int("count", 1))
	return nil
}
