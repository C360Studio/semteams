package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/processor/agentic-tools/executors"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/sandboxfleet"
	"github.com/c360studio/semteams/cmd/semteams/semsource"
	"github.com/c360studio/semteams/cmd/semteams/tools/addsource"
	"github.com/c360studio/semteams/cmd/semteams/tools/chainbash"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitautoresearchartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitautoresearchbaseline"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitautoresearchmeasurement"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitbootstrapcommitted"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitbootstrapplan"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitbootstrapverify"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitplan"
	"github.com/c360studio/semteams/cmd/semteams/tools/querysandboxtenant"
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
	// Mirrors upstream BashExecutor's SANDBOX_URL: the chain-scoped
	// bash wrapper reuses the same env var so operators don't have to
	// wire two URLs that should always be the same. Empty leaves the
	// inner BashExecutor in its local-exec mode.
	envSandboxURL = "SANDBOX_URL"
)

// registerProductPayloads registers all SemTeams-local payload types on top
// of the framework's first-party payload set (payloadbuiltins.Register).
// R3.1 (ADR-031): research.Artifact — revision-keyed researcher snapshot.
// ADR-038 PR C Phase C2: devviaspec.Plan — planner output emitted by
// emit_plan during the research arc (the wider dev-via-spec arc retired
// in the ADR-042 MVP-7 follow-up sweep; only Plan survives).
// Add new product-local payload registrations here; keep the ADR reference
// in the comment so future readers know which slice introduced each type.
func registerProductPayloads(reg *payloadregistry.Registry) error {
	if err := research.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("research payloads: %w", err)
	}
	if err := devviaspec.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("dev-via-spec payloads: %w", err)
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
// add_source_repo. R3.2.1 adds emit_research_artifact. ADR-038 PR C
// Phase C2 adds emit_plan. ADR-041 Phase 4 adds the chain-scoped bash
// wrapper.
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
	if err := registerEmitPlan(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerSandboxFleetTools(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerAutoresearchTools(reg, natsClient, platform, logger); err != nil {
		return err
	}
	return registerChainBash(reg, natsClient, platform, logger)
}

// registerSandboxFleetTools wires the 4 sandbox-bootstrap pack tools:
// query_sandbox_tenant + emit_bootstrap_{plan,verify,committed}.
// Per ADR-042 §addendum 2026-05-29 §A.
//
// All four share a single sandboxfleet.TenantRegistry instance —
// constructing one per tool would still be correct (the registry is
// stateless across calls) but the shared instance keeps the logger
// + identity wiring uniform and saves a few allocations per boot.
//
// Skipped when natsClient is nil: the registry depends on a live
// TriplePublisher + EntityTripleReader for the entity-namespace
// storage. A nil-NATS deployment couldn't honor the rule pack's
// registry contract; better to surface tool-absent than tool-broken
// in the LLM's catalog.
func registerSandboxFleetTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; sandbox-fleet tools skipped",
			slog.String("category", "sandbox-bootstrap"))
		return nil
	}

	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	entityReader := chain.NewNATSEntityReader(natsClient, chain.DefaultGraphQueryEntitySubject)
	// Defensive belt-and-suspenders. Upstream NewNATSTriplePublisher /
	// NewNATSEntityReader return non-nil today, but their nil-return
	// behavior is not in the upstream API contract; a future beta
	// could degrade non-fatally. Surface as a Warn + skip instead of
	// panicking inside TenantRegistry's constructor.
	if triplePublisher == nil || entityReader == nil {
		logger.Warn("sandbox-fleet tools skipped: upstream returned nil triple-publisher or entity-reader",
			slog.String("category", "sandbox-bootstrap"))
		return nil
	}
	tenantRegistry := sandboxfleet.NewTenantRegistry(triplePublisher, entityReader, platform.Org, platform.Platform, logger)

	queryExecutor := querysandboxtenant.NewExecutor(tenantRegistry, logger)
	if err := reg.RegisterTool(querysandboxtenant.ToolName, queryExecutor); err != nil {
		return fmt.Errorf("register %s: %w", querysandboxtenant.ToolName, err)
	}
	planExecutor := emitbootstrapplan.NewExecutor(triplePublisher, tenantRegistry, platform, logger)
	if err := reg.RegisterTool(emitbootstrapplan.ToolName, planExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitbootstrapplan.ToolName, err)
	}
	verifyExecutor := emitbootstrapverify.NewExecutor(triplePublisher, platform, logger)
	if err := reg.RegisterTool(emitbootstrapverify.ToolName, verifyExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitbootstrapverify.ToolName, err)
	}
	committedExecutor := emitbootstrapcommitted.NewExecutor(triplePublisher, tenantRegistry, platform, logger)
	if err := reg.RegisterTool(emitbootstrapcommitted.ToolName, committedExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitbootstrapcommitted.ToolName, err)
	}
	logger.Info("Registered sandbox-fleet product tools",
		slog.String("category", "sandbox-bootstrap"),
		slog.Int("count", 4))
	return nil
}

// registerAutoresearchTools wires the 3 autoresearch pack tools:
// emit_autoresearch_{baseline,measurement,artifact}. Per ADR-042
// §addendum 2026-05-29.
//
// The measurement tool is the load-bearing empirical reviewer —
// reads autoresearch.best.value from the run entity via a
// chain.NATSEntityReader-backed adapter, compares numerically,
// updates best on outcome=kept. The structural-test-of-the-substrate
// per the pack README.
//
// Skipped when natsClient is nil: baseline + measurement both need
// the live publisher; measurement additionally needs the entity
// reader.
func registerAutoresearchTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; autoresearch tools skipped",
			slog.String("category", "autoresearch"))
		return nil
	}

	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	entityReader := chain.NewNATSEntityReader(natsClient, chain.DefaultGraphQueryEntitySubject)
	// Defensive: same posture as registerSandboxFleetTools above.
	if triplePublisher == nil || entityReader == nil {
		logger.Warn("autoresearch tools skipped: upstream returned nil triple-publisher or entity-reader",
			slog.String("category", "autoresearch"))
		return nil
	}

	baselineExecutor := emitautoresearchbaseline.NewExecutor(triplePublisher, logger)
	if err := reg.RegisterTool(emitautoresearchbaseline.ToolName, baselineExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitautoresearchbaseline.ToolName, err)
	}

	measurementExecutor := emitautoresearchmeasurement.NewExecutor(
		triplePublisher,
		bestValueReaderAdapter{reader: entityReader},
		platform,
		logger,
	)
	if err := reg.RegisterTool(emitautoresearchmeasurement.ToolName, measurementExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitautoresearchmeasurement.ToolName, err)
	}

	artifactExecutor := emitautoresearchartifact.NewExecutor(triplePublisher, platform, logger, "")
	if err := reg.RegisterTool(emitautoresearchartifact.ToolName, artifactExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitautoresearchartifact.ToolName, err)
	}
	logger.Info("Registered autoresearch product tools",
		slog.String("category", "autoresearch"),
		slog.Int("count", 3))
	return nil
}

// bestValueReaderAdapter narrows chain.NATSEntityReader's
// ReadEntity(ctx, id) → map shape to the
// emit_autoresearch_measurement BestValueReader contract. Reads the
// autoresearch.best.value triple off the supplied run entity ID.
//
// Numeric shape widening: the graph-query wire decodes JSON
// numbers into float64 in the steady state, but operator-edited
// triples / future encoder changes could land int / json.Number /
// numeric string. The adapter widens for the common defensive
// shapes; anything else returns a typed error so the executor
// surfaces "type=string" instead of a misleading "absent" wedge.
//
// Returns:
//   - (val, true, nil)  — numeric hit (float64, int, or json.Number
//     coercible).
//   - (0, false, nil)   — triple absent (entity has other triples
//     but no autoresearch.best.value).
//   - (0, false, err)   — graph error OR triple present but not
//     numeric. The error message names the actual type so the wedge
//     is debuggable from a single log line.
type bestValueReaderAdapter struct {
	reader *chain.NATSEntityReader
}

func (a bestValueReaderAdapter) ReadBestValue(ctx context.Context, runEntityID string) (float64, bool, error) {
	triples, err := a.reader.ReadEntity(ctx, runEntityID)
	if err != nil {
		return 0, false, err
	}
	v, ok := triples["autoresearch.best.value"]
	if !ok {
		return 0, false, nil
	}
	switch n := v.(type) {
	case float64:
		return n, true, nil
	case float32:
		return float64(n), true, nil
	case int:
		return float64(n), true, nil
	case int32:
		return float64(n), true, nil
	case int64:
		return float64(n), true, nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false, fmt.Errorf("autoresearch.best.value present as json.Number %q but not parseable as float64: %w", n, err)
		}
		return f, true, nil
	default:
		return 0, false, fmt.Errorf("autoresearch.best.value present but not numeric (type=%T value=%v); graph stamp must be a number", v, v)
	}
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
// as registerEmitArtifact: any deployment running a planning role needs
// it. Output directory defaults to "docs/plans" but is overrideable via
// SEMTEAMS_PLAN_DIR.
//
// Persona contract: the research-pack planner persona
// (researcher-research-plan/15-emit-plan.md) instructs the planner
// phase to call emit_plan before terminating with decide(action="gather").
// configs/rules/research/{01,05,06}*.json wire emit_plan into the
// planner's allowed_tools.
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
// emit-tools take via SetChainReader. Centralised here so all
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
// (structurally-identical) ChainReader interface declared by the
// emit-tool package. If the interface ever widens (a new method
// added) and chain.LineageReader is not extended to match, this
// fails to build — surfacing the drift here rather than at
// SetChainReader call time.
var (
	_ emitplan.ChainReader    = (*chain.LineageReader)(nil)
	_ chainbash.ChainResolver = (*chain.Resolver)(nil)
	_ chainbash.ChainResolver = identityChainResolver{}
	_ chainbash.Inner         = (*executors.BashExecutor)(nil)
)

// registerChainBash wires the ADR-041 Phase 4 chain-scoped bash wrapper
// under the canonical "bash" name. The framework bash was omitted via
// SkipBuiltins=[bash] in setupToolsAndPreprocessor, so the slot is free.
//
// Wrapper composition:
//   - Inner: upstream's executors.BashExecutor (constructed from env
//     so it picks up SANDBOX_URL identically to how the framework's
//     RegisterBuiltins would have).
//   - Resolver: chain.Resolver backed by NATSParentReader. Walks
//     agent.loop.parent triples back to the chain root and returns
//     loop_id == chain_id (ADR-038 D1).
//
// At every Execute, the wrapper resolves call.LoopID → chain_id and
// rewrites Metadata["task_id"] = chain_id before delegating. Upstream's
// BashExecutor uses task_id over loop_id when picking the sandbox
// worktree, so every role in the chain shares one worktree.
//
// Fail-soft: resolver errors (no parent yet, NATS timeout) skip the
// rewrite — upstream falls back to loop_id. Matches the behavior the
// framework had before this wrapper, so a graph regression cannot make
// non-chain bash unusable.
//
// Always registered: even when natsClient is nil (resolver still works
// against the chain root case where loop_id == chain_id), the wrapper
// stays in place under the canonical name so the LLM's tool catalog is
// consistent across deployments.
func registerChainBash(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	inner := executors.NewBashExecutorFromEnv()

	var resolver chainbash.ChainResolver
	if natsClient != nil {
		parentReader := chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject)
		resolver = chain.NewResolver(parentReader, platform)
	} else {
		// No NATS → no ancestry walk possible. Use an identity resolver
		// so the wrapper still delegates correctly; every call looks like
		// a chain root (loop_id == chain_id) and upstream's loop_id
		// fallback is what determines the sandbox bucket.
		resolver = identityChainResolver{}
		logger.Warn("Registered chain-scoped bash with identity resolver (nats client unavailable)",
			slog.String("name", chainbash.ToolName))
	}

	wrapper := chainbash.NewExecutor(inner, resolver, logger)
	if err := reg.RegisterTool(chainbash.ToolName, wrapper); err != nil {
		return fmt.Errorf("register %s: %w", chainbash.ToolName, err)
	}
	mode := "local"
	if strings.TrimSpace(os.Getenv(envSandboxURL)) != "" {
		mode = "sandbox"
	}
	logger.Info("Registered product tool (chain-scoped bash wrapper)",
		slog.String("name", chainbash.ToolName),
		slog.String("mode", mode))
	return nil
}

// identityChainResolver is the no-NATS fallback. ChainID returns the
// input loop_id unchanged — the wrapper interprets that as "loop is the
// chain root" and skips the metadata rewrite.
type identityChainResolver struct{}

func (identityChainResolver) ChainID(_ context.Context, loopID string) (string, error) {
	return loopID, nil
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
