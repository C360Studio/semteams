package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/processor/agentic-tools/sandbox"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/tools/addsource"
	"github.com/c360studio/semteams/cmd/semteams/tools/bootstrapworkspace"
	"github.com/c360studio/semteams/cmd/semteams/tools/builderdecide"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitspecartifact"
	"github.com/c360studio/semteams/cmd/semteams/verification"
	"github.com/c360studio/semteams/cmd/semteams/verification/families"
	"github.com/c360studio/semteams/cmd/semteams/verification/families/tcpbinaryprotobuf"
	"github.com/c360studio/semteams/cmd/semteams/verification/runtimes"
	"github.com/c360studio/semteams/cmd/semteams/verification/runtimes/javajunittestcontainers"
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

	// envSandboxURL is the base URL of the sandbox HTTP server.
	// Mirrors upstream BashExecutor's SANDBOX_URL: the bootstrap
	// tool reuses the same env var so operators don't have to wire
	// two URLs that should always be the same. Empty disables
	// bootstrap_workspace registration; the dev-via-spec-builder
	// loop is non-functional without it.
	envSandboxURL = "SANDBOX_URL"
)

// registerProductPayloads registers all SemTeams-local payload types on top
// of the framework's first-party payload set (payloadbuiltins.Register).
// R3.1 (ADR-031): research.Artifact — revision-keyed researcher snapshot.
// R3.3 (ADR-031 §R3.3): devviaspec.Artifact — terminal architect output.
// R3.6.2.b (ADR-032 §15): no payload — builder_decide tool emits triples
// only; full args round-trip through the tool result Content for
// read_loop_result consumers.
// R3.7.2.a (ADR-033 §addendum 2026-05-04): verification.Commitment —
// architect's structured commitment to a verification surface (target /
// approach / harness / runtime / convention / evidence). Wired into
// dev_via_spec.artifact.v2 in R3.7.2.b.
// Add new product-local payload registrations here; keep the ADR reference
// in the comment so future readers know which slice introduced each type.
func registerProductPayloads(reg *payloadregistry.Registry) error {
	if err := research.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("research payloads: %w", err)
	}
	if err := devviaspec.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("dev-via-spec payloads: %w", err)
	}
	if err := verification.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("verification payloads: %w", err)
	}
	return nil
}

// registerProductFamiliesAndRuntimes builds the (family × runtime)
// verification matrix registries (R3.7.2.d, ADR-033 §addendum
// 2026-05-04). Pattern-A boot-registries: framework code, no
// operator-curated state, dependency-injected into the schema gate
// (R3.7.2.e) and builder invocation (R3.7.2.h).
//
// R3.7.2.d ships ONE family (tcp.binary-protobuf.v1) and ONE runtime
// (java-junit-testcontainers). Future families/runtimes are pure
// additions: drop a new sub-package, add a Register call here.
//
// Returns nil registries on registration failure so the chain still
// boots; the schema gate then rejects every commitment that names a
// family or runtime, which is the right loud-failure mode. A registry
// initialisation error is logged at WARN with the underlying cause.
func registerProductFamiliesAndRuntimes(logger *slog.Logger) (*families.Registry, *runtimes.Registry) {
	familyReg := families.NewRegistry()
	if err := tcpbinaryprotobuf.Register(familyReg); err != nil {
		logger.Warn("verification family registration failed; schema gate will reject all commitments referencing families",
			slog.String("family", "tcp.binary-protobuf.v1"),
			slog.Any("error", err))
		familyReg = nil
	}

	runtimeReg := runtimes.NewRegistry()
	if err := javajunittestcontainers.Register(runtimeReg); err != nil {
		logger.Warn("verification runtime registration failed; schema gate will reject all commitments referencing runtimes",
			slog.String("runtime", "java-junit-testcontainers"),
			slog.Any("error", err))
		runtimeReg = nil
	}

	logger.Info("verification matrix registries initialised",
		slog.Int("families", regLen(familyReg)),
		slog.Int("runtimes", regLen(runtimeReg)))
	return familyReg, runtimeReg
}

// regLen returns 0 for a nil registry, or registry.Len() otherwise.
// Helper exists so the boot log line stays readable when a
// registration failure has nilled out a registry.
func regLen(r interface{ Len() int }) int {
	if r == nil {
		return 0
	}
	// Reflect-safe nil check via type-assert path; the typed nils
	// (*families.Registry)(nil) and (*runtimes.Registry)(nil) flow
	// through the interface unscathed and would NPE on .Len(). Guard.
	switch reg := r.(type) {
	case *families.Registry:
		if reg == nil {
			return 0
		}
		return reg.Len()
	case *runtimes.Registry:
		if reg == nil {
			return 0
		}
		return reg.Len()
	default:
		return r.Len()
	}
}

// registerProductTools wires product-shell-local tool executors onto
// the shared registry, after the framework's RegisterBuiltins has
// populated it with first-party tools. R2 of ADR-031 adds
// add_source_repo. R3.2.1 adds emit_research_artifact. R3.3 adds
// emit_dev_via_spec_artifact. R3.6.2.b adds builder_decide. R3.6.2.d
// adds bootstrap_workspace.
// Re-introduce a *config.Config parameter (or switch to a deps struct)
// once a tool needs deployment-config visibility.
//
// Discipline gate (commission-not-omission): adding a new tool
// registration here requires a framework-alignment review per
// cmd/semteams/tools/README.md. The pattern we are explicitly
// avoiding: each tool is individually defensible; the cumulative
// drift away from framework idiom turns the product shell into a
// bespoke monster. Survey upstream first; if the pattern exists,
// port don't fork; if it's planned but not shipped (e.g. upstream's
// write_artifact suite per ADR-028 §What's not built here), document
// the migration target in tools/README.md and an ADR addendum.
func registerProductTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if err := registerAddSource(reg, natsClient, logger); err != nil {
		return err
	}
	if err := registerEmitArtifact(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerEmitSpecArtifact(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerBuilderDecide(reg, natsClient, platform, logger); err != nil {
		return err
	}
	return registerBootstrapWorkspace(reg, logger)
}

// registerAddSource wires the R2 add_source_repo executor. Stays inert
// (skipped at boot) when no SemSource namespace allowlist is configured,
// to keep the LLM from seeing a tool every call would fail.
func registerAddSource(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, logger *slog.Logger) error {
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

// registerEmitArtifact wires the R3.2.1 emit_research_artifact executor.
// Always registered when natsClient is non-nil — there's no per-deployment
// gating: the tool is product policy, and any deployment running the
// research flow needs it to drive the mode-transition machinery.
// Reuses the framework's NATSTriplePublisher so the marker triples flow
// through the same graph.mutation.triple.add path the decide and
// emit_diagnosis tools use.
func registerEmitArtifact(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; emit_research_artifact registration skipped")
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	executor := emitartifact.NewExecutor(triplePublisher, natsClient, platform, logger)
	if err := reg.RegisterTool(emitartifact.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", emitartifact.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", emitartifact.ToolName),
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform))
	return nil
}

// registerEmitSpecArtifact wires the R3.3 emit_dev_via_spec_artifact executor.
// Always registered when natsClient is non-nil — same "always on" policy as
// registerEmitArtifact: the tool is product policy, and any deployment running
// the dev-via-spec flow needs it to close the arc. Output directory defaults
// to "docs/specs" but is overrideable via SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR.
func registerEmitSpecArtifact(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; emit_dev_via_spec_artifact registration skipped")
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	// Pass empty outputDir so the constructor reads SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR
	// or falls back to the package default ("docs/specs").
	executor := emitspecartifact.NewExecutor(triplePublisher, natsClient, platform, logger, "")
	if err := reg.RegisterTool(emitspecartifact.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", emitspecartifact.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", emitspecartifact.ToolName),
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform))
	return nil
}

// registerBuilderDecide wires the R3.6.2.b builder_decide executor — the
// dev-via-spec-builder role's terminal validator. Always registered when
// natsClient is non-nil — same "always on" policy as registerEmitArtifact:
// the tool is product policy, and any deployment running the dev-via-spec
// builder slice needs it to close the arc with action-specific evidence.
// See ADR-032 §15 for the contract and cmd/semteams/tools/builderdecide
// for the package-level discussion of why this is a sibling tool to
// upstream `decide` rather than a wrapping replacement.
func registerBuilderDecide(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; builder_decide registration skipped")
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	executor := builderdecide.NewExecutor(triplePublisher, platform, logger)
	if err := reg.RegisterTool(builderdecide.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", builderdecide.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", builderdecide.ToolName),
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform))
	return nil
}

// registerBootstrapWorkspace wires the R3.6.2.d bootstrap_workspace
// executor — the dev-via-spec-builder role's iteration-1 setup hook.
// Skipped when SANDBOX_URL is unset: the dev-via-spec-builder loop is
// non-functional without a sandbox, and the upstream BashExecutor uses
// the same env var to route bash to the sandbox, so an unset value
// disables the entire builder slice consistently. See ADR-032 §addendum
// 2026-05-03 R3.6.2.d for why this lives in product code (chicken-and-
// egg between rule action timing and publish_agent's task_id generation).
func registerBootstrapWorkspace(reg *agentictools.ExecutorRegistry, logger *slog.Logger) error {
	sandboxURL := strings.TrimSpace(os.Getenv(envSandboxURL))
	if sandboxURL == "" {
		logger.Info("Product tool not registered (SANDBOX_URL unset)",
			slog.String("name", bootstrapworkspace.ToolName),
			slog.String("env_var", envSandboxURL))
		return nil
	}
	client := sandbox.NewClient(sandboxURL)
	executor, err := bootstrapworkspace.NewExecutor(client, logger, "")
	if err != nil {
		return fmt.Errorf("construct %s executor: %w", bootstrapworkspace.ToolName, err)
	}
	if err := reg.RegisterTool(bootstrapworkspace.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", bootstrapworkspace.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", bootstrapworkspace.ToolName),
		slog.String("sandbox_url", sandboxURL))
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
