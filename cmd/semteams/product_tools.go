package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"

	"github.com/c360studio/semteams/cmd/semteams/tools/addsource"
)

// Environment variables that configure product-shell-local tools.
// Kept as env vars (not framework config keys) so they don't drift the
// upstream config schema. Operators set them per deployment.
const (
	// envSemSourceNamespaces is a comma-separated list of SemSource
	// namespaces this deployment may add sources to. Empty (or unset)
	// disables the add_source_repo tool.
	envSemSourceNamespaces = "SEMTEAMS_SEMSOURCE_NAMESPACES"

	// envSemSourceDefaultNamespace is the namespace used when an LLM
	// invokes add_source_repo without an explicit namespace. Must be in
	// SEMTEAMS_SEMSOURCE_NAMESPACES.
	envSemSourceDefaultNamespace = "SEMTEAMS_SEMSOURCE_DEFAULT_NAMESPACE"

	// envSemSourceActor overrides the Provenance.Actor stamp on
	// outgoing AddRequests. Defaults to "semteams.researcher".
	envSemSourceActor = "SEMTEAMS_SEMSOURCE_ACTOR"
)

// registerProductTools wires product-shell-local tool executors onto
// the shared registry, after the framework's RegisterBuiltins has
// populated it with first-party tools. R2 of ADR-031 adds
// add_source_repo. Future product slices register here.
//
// The cfg parameter is reserved for future tools that read deployment
// state from the framework config; current tool reads only env vars
// (TODO: thread namespace allowlist through cfg.Components or a new
// product config block once we have a second product tool that
// benefits from JSON-config visibility).
func registerProductTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, _ *config.Config, logger *slog.Logger) error {
	addSourceCfg := readAddSourceConfig()

	// Skip registration entirely when the deployment has no namespace
	// allowlist. Otherwise the LLM sees add_source_repo in its tool
	// catalog, plans with it, and every invocation fails — wasted
	// tokens and confusing failure-mode reporting. An operator
	// flipping the env var on requires a process restart to register
	// the tool, which is consistent with how every other product
	// config field works.
	if len(addSourceCfg.AllowedNamespaces) == 0 {
		logger.Info("Product tool not registered (no namespace allowlist)",
			slog.String("name", addsource.RepoToolName),
			slog.String("env_var", envSemSourceNamespaces))
		return nil
	}

	executor := addsource.NewRepoExecutor(natsClient, addSourceCfg, logger)
	if err := reg.RegisterTool(addsource.RepoToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", addsource.RepoToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", addsource.RepoToolName),
		slog.Int("allowed_namespaces", len(addSourceCfg.AllowedNamespaces)),
		slog.String("default_namespace", addSourceCfg.DefaultNamespace))
	return nil
}

// readAddSourceConfig parses the environment-driven namespace
// allowlist + default + actor override into an addsource.Config.
// Empty allowlist is legal (the executor errors on every call,
// keeping the tool registered-but-inert) so operators can flip it on
// without re-registering.
func readAddSourceConfig() addsource.Config {
	return addsource.Config{
		AllowedNamespaces: parseCSV(os.Getenv(envSemSourceNamespaces)),
		DefaultNamespace:  strings.TrimSpace(os.Getenv(envSemSourceDefaultNamespace)),
		Actor:             strings.TrimSpace(os.Getenv(envSemSourceActor)),
	}
}

// parseCSV splits a comma-separated env var into trimmed non-empty
// values. Empty input yields a nil slice (so addsource.Config's
// "empty allowlist disables tool" check fires correctly).
func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
