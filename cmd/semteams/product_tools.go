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

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/semsource"
	"github.com/c360studio/semteams/cmd/semteams/testharness"
	"github.com/c360studio/semteams/cmd/semteams/tools/addsource"
	"github.com/c360studio/semteams/cmd/semteams/tools/bootstrapworkspace"
	"github.com/c360studio/semteams/cmd/semteams/tools/builderdecide"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitconsensus"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitplan"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitspecartifact"
	"github.com/c360studio/semteams/cmd/semteams/verification"
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
	// bootstrap_workspace registration; the builder
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
// R3.7.2.a (ADR-033 §addendum 2026-05-04): verification.Check —
// architect's structured check against a verification surface (target /
// runtime / test_harness / test_runtime / ref / evidence). Wired into
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
	// Headless-semsource compatibility: semsource publishes
	// semsource.entity.v1 / .status.v1 / .manifest.v1 / .predicates.v1
	// payloads on the shared NATS bus. graph-ingest unmarshals them via
	// the registry — without this registration, every entity message is
	// dropped with "unregistered payload type: semsource.entity.v1".
	// Mirrors semspec's headless-host pattern (semspec/semsource/
	// payload.go) — host app owns the registration when running
	// semsource headless.
	if err := semsource.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("semsource payloads: %w", err)
	}
	return nil
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
func registerProductTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, testHarnessMgr *testharness.Manager, logger *slog.Logger) error {
	if err := registerAddSource(reg, natsClient, logger); err != nil {
		return err
	}
	if err := registerEmitArtifact(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerEmitPlan(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerEmitConsensus(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerEmitSpecArtifact(reg, natsClient, platform, testHarnessMgr, logger); err != nil {
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
	// outputDir empty → reads SEMTEAMS_RESEARCH_ARTIFACT_DIR or falls back
	// to the package default ("docs/research"). ADR-038 PR C Phase C1
	// markdown render target.
	executor := emitartifact.NewExecutor(triplePublisher, natsClient, platform, logger, "")
	if err := reg.RegisterTool(emitartifact.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", emitartifact.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", emitartifact.ToolName),
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform))
	return nil
}

// registerEmitPlan wires the ADR-038 PR C Phase C2 emit_plan executor.
// Always registered when natsClient is non-nil — same "always on" policy
// as registerEmitArtifact: any deployment running the dev-via-spec flow
// needs it. Output directory defaults to "docs/plans" but is overrideable
// via SEMTEAMS_PLAN_DIR.
//
// Persona contract (Phase C5, landed): the dev-via-spec-planner persona
// fragment 15-emit-plan.md instructs the planner to call emit_plan
// before terminating with decide(action="planned"). The planner spawn
// rules (rules/research-mode-transition/03-stabilise-and-transition.json
// and rules/dev-via-spec/02 + 04 retry rules) include emit_plan in their
// tool list and prompt body. Configs that route through the dev-via-spec
// chain (osh-demo.json, e2e-dev-via-spec.json) include emit_plan in
// agentic-tools.allowed_tools.
func registerEmitPlan(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; emit_plan registration skipped")
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	executor := emitplan.NewExecutor(triplePublisher, natsClient, platform, logger, "")
	// Smoke #8 run-5 D1 + D2 fix: chain.LineageReader gives the executor
	// canonical lineage IDs + slug stem from the chain entity so the
	// planner persona stops guessing upstream loop IDs and the chain's
	// slug stays consistent. Optional opt-in: if the chain wiring fails
	// to construct (rare), the executor falls back to LLM-supplied values.
	executor.SetChainReader(buildChainLineageReader(natsClient, platform))
	if err := reg.RegisterTool(emitplan.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", emitplan.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", emitplan.ToolName),
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform))
	return nil
}

// buildChainLineageReader composes the chain Resolver +
// NATSEntityReader into a single ChainReader-satisfying adapter the
// emit-tools take via SetChainReader. Centralised here so all three
// emit-tools share identical wiring (subject, platform, ancestry walk
// budget). Subject is the upstream graph-query literal — see
// chain.DefaultGraphQueryEntitySubject doc-comment for why this is a
// constant rather than a config-resolved port at the request side.
func buildChainLineageReader(natsClient *natsclient.Client, platform types.PlatformMeta) *chain.LineageReader {
	parentReader := chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject)
	resolver := chain.NewResolver(parentReader, platform)
	entityReader := chain.NewNATSEntityReader(natsClient, chain.DefaultGraphQueryEntitySubject)
	return chain.NewLineageReader(resolver, entityReader)
}

// Compile-time guards that chain.LineageReader satisfies the
// (structurally-identical) ChainReader interfaces declared by the
// three emit-tool packages. If any one of those interfaces ever
// widens (a new method added) and chain.LineageReader is not extended
// to match, this fails to build — surfacing the drift here rather
// than at SetChainReader call time. The interfaces stay duplicated
// per package (each package owns its narrow contract); these vars
// keep the implementer in lock-step with all three.
var (
	_ emitplan.ChainReader         = (*chain.LineageReader)(nil)
	_ emitconsensus.ChainReader    = (*chain.LineageReader)(nil)
	_ emitspecartifact.ChainReader = (*chain.LineageReader)(nil)
)

// registerEmitConsensus wires the ADR-038 PR C Phase C3 emit_consensus
// executor. Always registered when natsClient is non-nil — same
// "always on" policy as registerEmitPlan / registerEmitArtifact.
// Output directory defaults to "docs/consensus" but is overrideable
// via SEMTEAMS_CONSENSUS_DIR.
//
// Persona contract (Phase C5, landed): the dev-via-spec-challenger
// persona fragment 15-emit-consensus.md instructs the challenger to
// call emit_consensus before terminating with decide(action="accept")
// and to NOT call when terminating with concerns_raised. The spawn
// rule (rules/dev-via-spec/03-reviewer-approved-to-challenger.json)
// includes emit_consensus in its tool list and prompt body. Configs
// that route through the dev-via-spec chain (osh-demo.json,
// e2e-dev-via-spec.json) include emit_consensus in
// agentic-tools.allowed_tools.
func registerEmitConsensus(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; emit_consensus registration skipped")
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	executor := emitconsensus.NewExecutor(triplePublisher, natsClient, platform, logger, "")
	// Smoke #8 run-5 D1 + D2 fix: chain.LineageReader gives the
	// challenger canonical plan_loop, plan_reviewer_loop, and slug
	// stem so depends_on and the rendered slug stop drifting from
	// the planner's pass. See registerEmitPlan for the same wiring.
	executor.SetChainReader(buildChainLineageReader(natsClient, platform))
	if err := reg.RegisterTool(emitconsensus.ToolName, executor); err != nil {
		return fmt.Errorf("register %s: %w", emitconsensus.ToolName, err)
	}
	logger.Info("Registered product tool",
		slog.String("name", emitconsensus.ToolName),
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform))
	return nil
}

// registerEmitSpecArtifact wires the R3.3 emit_dev_via_spec_artifact executor.
// Always registered when natsClient is non-nil — same "always on" policy as
// registerEmitArtifact: the tool is product policy, and any deployment running
// the dev-via-spec flow needs it to close the arc. Output directory defaults
// to "docs/specs" but is overrideable via SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR.
//
// testHarnessMgr resolves catalog names (the architect's check.test_harness
// field) into concrete entries the renderer projects into SPEC.md
// (R3.7.2.h′). Without resolution the builder can't reach the catalog
// from inside its sandbox; rendered fields close that gap. nil testHarnessMgr
// is permitted (rare deployments without test_harness wiring) — the renderer
// falls back to rendering the test_harness name without resolved fields.
func registerEmitSpecArtifact(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, testHarnessMgr *testharness.Manager, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; emit_dev_via_spec_artifact registration skipped")
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	// Adapt the test harness manager's Get signature to the executor's
	// TestHarnessResolver function type. nil manager → nil resolver →
	// renderer falls back to name-only rendering.
	var resolver emitspecartifact.TestHarnessResolver
	if testHarnessMgr != nil {
		resolver = testHarnessMgr.Get
	}
	// Pass empty outputDir so the constructor reads SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR
	// or falls back to the package default ("docs/specs").
	executor := emitspecartifact.NewExecutor(triplePublisher, natsClient, platform, logger, "", resolver)
	// ADR-038 PR B Phase 4: chain entity triple writes. Walks ancestry
	// from artifact.Provenance.ResearchArtifactLoop (a completed
	// researcher loop) — the architect's own loop has not stamped
	// agent.loop.parent yet (only stamped at completion), so we walk
	// from a completed ancestor that has full lineage stamped. Wired
	// optional via SetChainResolver to keep the executor's constructor
	// signature stable and to keep test contexts opt-in.
	// Subject is the upstream graph-query literal — see
	// chain.DefaultGraphQueryEntitySubject doc-comment for why this is a
	// constant rather than a config-resolved port.
	parentReader := chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject)
	chainResolver := chain.NewResolver(parentReader, platform)
	executor.SetChainResolver(chainResolver)
	// Smoke #8 run-5 D1 + D2 fix: SetChainReader gives the architect
	// canonical lineage IDs (research_artifact_loop, plan_loop,
	// plan_reviewer_loop) + slug stem so provenance and the rendered
	// slug stop drifting. consensus_loop was dropped in ADR-041 Slice
	// 2D-4 alongside the chain.consensus.* stamper teardown.
	// ChainResolver above (used for chain.spec_artifact.* triple
	// writes) is independent — same underlying NATS plumbing,
	// different read shape.
	executor.SetChainReader(buildChainLineageReader(natsClient, platform))

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
// builder role's terminal validator. Always registered when
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
// executor — the builder role's iteration-1 setup hook.
// Skipped when SANDBOX_URL is unset: the builder loop is
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
