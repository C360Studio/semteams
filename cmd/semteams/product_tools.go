package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/processor/agentic-tools/executors"
	"github.com/c360studio/semstreams/processor/agentic-tools/runner"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
	"github.com/c360studio/semteams/cmd/semteams/sandboxruntime"
	"github.com/c360studio/semteams/cmd/semteams/semsource"
	"github.com/c360studio/semteams/cmd/semteams/tools/addsource"
	"github.com/c360studio/semteams/cmd/semteams/tools/chainbash"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitautoresearchartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitautoresearchbaseline"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitautoresearchmeasurement"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitdevviatestmeasurement"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitdevviatestplan"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitplan"
	"github.com/c360studio/semteams/cmd/semteams/tools/querysandboxattestation"
	"github.com/c360studio/semteams/cmd/semteams/tools/requestsandbox"
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

	// envSandboxRepoRoot points BuiltinCatalog at the directory
	// holding `.devcontainer/<profile>/devcontainer.json`. Production
	// default "/app" matches docker/Dockerfile's COPY layout.
	envSandboxRepoRoot = "SEMTEAMS_SANDBOX_REPO_ROOT"

	// envSandboxTenantRoot is the host filesystem path under which
	// the manager creates per-(profile, requirements) workspace
	// folders. PR 4.5 F6 makes this path symmetric: docker-compose
	// binds the same absolute path on host AND inside the sandbox
	// container so devcontainer-cli's DooD-spawned sibling containers
	// see consistent bind sources. Production default
	// "/var/lib/semteams-tenants" matches the /var/lib convention;
	// e2e overrides to a per-run path under the repo so dev laptops
	// don't need sudo. The sandbox container's `--workspace` flag
	// must point at the same value (compose substitutes both).
	envSandboxTenantRoot = "SEMTEAMS_TENANT_ROOT"

	// envSandboxAllowPublicNetwork unlocks NetworkPolicy=public
	// admission. Default false (default-deny). When true, requests
	// declaring `network: public` admit without reviewer signoff
	// (assuming profile allows it).
	envSandboxAllowPublicNetwork = "SEMTEAMS_SANDBOX_ALLOW_PUBLIC_NETWORK"

	// envSandboxAllowPrivileged unlocks Privilege=privileged
	// admission. Default false (forbidden — operator-only opt-in).
	envSandboxAllowPrivileged = "SEMTEAMS_SANDBOX_ALLOW_PRIVILEGED"

	// envSandboxAvailableSecrets is a comma-separated list of secret
	// identifiers the operator has pre-provisioned (env vars set on
	// the backend process). Requests declaring a secret not in this
	// set fail admission terminally.
	envSandboxAvailableSecrets = "SEMTEAMS_SANDBOX_AVAILABLE_SECRETS"

	// envSandboxRunner selects the Runner implementation:
	//   - "api"  → SandboxAPIRunner. Preserves the DooD trust
	//     boundary: shells `devcontainer up` / `devcontainer exec`
	//     via the existing sandbox HTTP API (SANDBOX_URL). The
	//     sandbox container has both the docker socket AND
	//     @devcontainers/cli (PR 4.1). Production default for
	//     deployments that already run the sandbox compose service.
	//     REQUIRES: .devcontainer/ profiles reachable at
	//     /app/.devcontainer/ inside the sandbox container (compose
	//     mount; ui/docker-compose.agentic-e2e.yml is the template).
	//   - "cli"  → CLIRunner. Direct os/exec from the backend
	//     process. Requires @devcontainers/cli + docker-socket
	//     access on the backend (NOT mounted in docker/Dockerfile
	//     today; operator must extend the image if choosing this
	//     path).
	//   - "mock" → MockRunner. In-memory happy-path Runner used by
	//     the mock-LLM journey (test/fixtures/journeys/
	//     sandbox-mvp.yaml). NOT a production runner — returns
	//     fabricated Ready attestations. Real-LLM smokes that
	//     exercise request_sandbox MUST set SEMTEAMS_SANDBOX_RUNNER=api
	//     explicitly to avoid silently fabricating a Ready attestation
	//     against the real LLM (would render the smoke meaningless).
	//   - unset/anything else → NoopRunner. Fail-shut: Manager
	//     produces Terminal attestation explaining the gap;
	//     Coordinator routes to respond_direct rather than 500.
	envSandboxRunner = "SEMTEAMS_SANDBOX_RUNNER"
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
	if err := registerSandboxManagerTools(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerAutoresearchTools(reg, natsClient, platform, logger); err != nil {
		return err
	}
	if err := registerDevViaTestTools(reg, natsClient, platform, logger); err != nil {
		return err
	}
	return registerChainBash(reg, natsClient, platform, logger)
}

// registerDevViaTestTools wires the dev-via-test pack's emit tools.
// Slice 1 adds emit_dev_via_test_plan (Lisa's terminal commit);
// Slice 2 adds emit_dev_via_test_measurement (Ralph's per-iteration
// stamp). Slice 4 may add emit_dev_via_test_artifact for CBG.
//
// Skipped when natsClient is nil: both tools need the live
// publisher to stamp triples. Mirrors the registerEmitPlan /
// registerAutoresearchTools nil-NATS posture.
//
// Per ADR-044 §addendum 2026-06-03 framework-alignment review: no
// upstream equivalent; migration target is the same generic
// write_artifact upstream is sketching (ADR-028 §What's not built
// here). See cmd/semteams/tools/README.md for the per-tool
// migration table.
func registerDevViaTestTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; dev-via-test tools skipped",
			slog.String("category", "dev-via-test"))
		return nil
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	if triplePublisher == nil {
		logger.Warn("dev-via-test tools skipped: upstream returned nil triple-publisher",
			slog.String("category", "dev-via-test"))
		return nil
	}
	// Slice 6: the remover lets emit_dev_via_test_plan UPSERT on a
	// plan-review re-plan (revision > 1). Product-local remove path
	// (ADR-044 §addendum Slice 6 — mirrors upstream write_todos).
	planRemover := emitdevviatestplan.NewNATSTripleRemover(natsClient)
	planExecutor := emitdevviatestplan.NewExecutor(triplePublisher, planRemover, logger)
	if err := reg.RegisterTool(emitdevviatestplan.ToolName, planExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitdevviatestplan.ToolName, err)
	}
	measurementExecutor := emitdevviatestmeasurement.NewExecutor(triplePublisher, platform, logger)
	if err := reg.RegisterTool(emitdevviatestmeasurement.ToolName, measurementExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitdevviatestmeasurement.ToolName, err)
	}
	logger.Info("Registered dev-via-test product tools",
		slog.String("category", "dev-via-test"),
		slog.Int("count", 2))
	return nil
}

// registerSandboxManagerTools wires the ADR-043 Layer 2 tools:
// request_sandbox + query_sandbox_attestation. The shared
// sandboxmanager.Manager holds the canonical-profile catalog,
// operator-config'd AdmissionPolicy, the production CLIRunner
// (shells out to @devcontainers/cli), and the triple publisher.
// Both tools key off the same chain entity (related_loops
// "chain-entity-id" pinned by the admission rule pack, or fallback
// to chain.Resolver).
//
// Skipped when natsClient is nil: both tools need the live
// publisher (request_sandbox) or entity reader
// (query_sandbox_attestation). A nil-NATS deployment couldn't
// honor the attestation-stamp contract.
//
// Operator config: SEMTEAMS_SANDBOX_REPO_ROOT, _ALLOW_PUBLIC_NETWORK,
// _ALLOW_PRIVILEGED, _AVAILABLE_SECRETS env vars (see const block).
func registerSandboxManagerTools(reg *agentictools.ExecutorRegistry, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	if natsClient == nil {
		logger.Warn("nats client unavailable; sandbox-manager tools skipped",
			slog.String("category", "sandbox-manager"))
		return nil
	}

	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	entityReader := chain.NewNATSEntityReader(natsClient, chain.DefaultGraphQueryEntitySubject)
	if triplePublisher == nil || entityReader == nil {
		logger.Warn("sandbox-manager tools skipped: upstream returned nil triple-publisher or entity-reader",
			slog.String("category", "sandbox-manager"))
		return nil
	}

	repoRoot := os.Getenv(envSandboxRepoRoot)
	if repoRoot == "" {
		repoRoot = "/app"
	}
	catalog, err := sandboxmanager.BuiltinCatalog(repoRoot)
	if err != nil {
		return fmt.Errorf("sandbox catalog: %w", err)
	}

	policy := sandboxmanager.AdmissionPolicy{
		AllowPublicNetwork:        boolEnv(envSandboxAllowPublicNetwork),
		AllowPrivilegedContainers: boolEnv(envSandboxAllowPrivileged),
		AvailableSecrets:          parseCSV(os.Getenv(envSandboxAvailableSecrets)),
	}

	runner := selectSandboxRunner(logger)
	tenantRoot := strings.TrimSpace(os.Getenv(envSandboxTenantRoot))
	manager := sandboxmanager.New(sandboxmanager.ManagerConfig{
		Catalog:    catalog,
		Policy:     policy,
		Runner:     runner,
		Publisher:  triplePublisher,
		TenantRoot: tenantRoot,
		Logger:     logger,
	})

	// Both tools share the same chain.Resolver fallback for chain
	// entity ID resolution when the rule pack hasn't pinned it
	// explicitly (smoke tests, isolated coordinator calls).
	parentReader := chain.NewNATSParentReader(natsClient, platform, "")
	resolver := chain.NewResolver(parentReader, platform)

	requestExecutor := requestsandbox.NewExecutor(manager, resolver, platform, logger)
	if err := reg.RegisterTool(requestsandbox.ToolName, requestExecutor); err != nil {
		return fmt.Errorf("register %s: %w", requestsandbox.ToolName, err)
	}

	queryExecutor := querysandboxattestation.NewExecutor(catalog, entityReader, resolver, platform, nil, logger)
	if err := reg.RegisterTool(querysandboxattestation.ToolName, queryExecutor); err != nil {
		return fmt.Errorf("register %s: %w", querysandboxattestation.ToolName, err)
	}

	logger.Info("Registered sandbox-manager product tools",
		slog.String("category", "sandbox-manager"),
		slog.String("repo_root", repoRoot),
		slog.String("tenant_root", manager.TenantRoot()),
		slog.Int("profiles", len(catalog.Profiles())),
		slog.Bool("allow_public_network", policy.AllowPublicNetwork),
		slog.Bool("allow_privileged", policy.AllowPrivilegedContainers),
		slog.Int("available_secrets", len(policy.AvailableSecrets)),
		slog.Int("count", 2))
	return nil
}

// boolEnv accepts "1", "true", "yes" (case-insensitive, trimmed)
// as opt-in; anything else (including "on", "y", "enabled") is
// treated as deny. The narrow set is intentional — admission is a
// security surface where the wrong default direction is the failure
// mode.
func boolEnv(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

// selectSandboxRunner picks the Runner based on
// SEMTEAMS_SANDBOX_RUNNER. Default (unset / unrecognized) returns
// a NoopRunner that surfaces "no production runner configured" via
// the attestation, preventing silent 500s on first call.
func selectSandboxRunner(logger *slog.Logger) sandboxmanager.Runner {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envSandboxRunner)))
	switch v {
	case "api":
		baseURL := strings.TrimSpace(os.Getenv(envSandboxURL))
		runner := sandboxmanager.NewSandboxAPIRunner(baseURL)
		if runner == nil {
			// Operator opted into the API runner explicitly but the
			// URL is empty — config-rotate window typo or compose
			// regression. Loud Error (not Warn) so it surfaces in
			// any aggregated log search; remediation field tells
			// the next operator how to fix it. Per PR 4.4 finding M2.
			logger.Error("sandbox runner: SEMTEAMS_SANDBOX_RUNNER=api but SANDBOX_URL is empty; falling back to noop",
				slog.String("env", envSandboxRunner),
				slog.String("remediation", "set SANDBOX_URL=http://sandbox:8090 (or your compose sandbox-service URL) OR change SEMTEAMS_SANDBOX_RUNNER=mock for tests"))
			return sandboxmanager.NoopRunner{}
		}
		runner.Logger = logger
		logger.Info("sandbox runner: production API",
			slog.String("env", envSandboxRunner),
			slog.String("base_url", baseURL))
		return runner
	case "cli":
		runner := sandboxmanager.NewCLIRunner()
		runner.Logger = logger
		logger.Info("sandbox runner: production CLI",
			slog.String("env", envSandboxRunner),
			slog.String("value", v))
		return runner
	case "mock":
		logger.Info("sandbox runner: mock (test-only; returns fabricated Ready attestations)",
			slog.String("env", envSandboxRunner))
		return sandboxmanager.MockRunner{}
	default:
		logger.Warn("sandbox runner: noop (set SEMTEAMS_SANDBOX_RUNNER=api for production; see product_tools.go const block)",
			slog.String("env", envSandboxRunner),
			slog.String("value", v))
		return sandboxmanager.NoopRunner{}
	}
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

	baselineExecutor := emitautoresearchbaseline.NewExecutor(triplePublisher, platform, logger)
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
	// Attestation-aware artifact write (semteams#194): when the
	// sandbox stack is wired AND a chain has a ready per-tenant
	// devcontainer, the artifact is written INTO that workspace via
	// /exec so the reviewer's `bash cat` (which routes through
	// chainbash → AttestationRunner → `devcontainer exec`) resolves
	// the same file. Without SANDBOX_URL the writer stays nil and the
	// executor falls back to local backend-fs writes (dev shells,
	// unit tests, deployments without the sandbox stack).
	sandboxURL := strings.TrimSpace(os.Getenv(envSandboxURL))
	if sandboxURL != "" {
		parentReader := chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject)
		resolver := chain.NewResolver(parentReader, platform)
		chainReader := &chainAttestationReader{
			entities: entityReader,
			resolver: resolver,
			platform: platform,
			logger:   logger,
		}
		writer := newSandboxArtifactWriter(sandboxURL, logger)
		artifactExecutor.SetAttestationWriter(chainReader, writer)
		logger.Info("emit_autoresearch_artifact: attestation-aware write enabled",
			slog.String("sandbox_url", sandboxURL))
	} else {
		logger.Info("emit_autoresearch_artifact: SANDBOX_URL unset; local-fs fallback only",
			slog.String("env", envSandboxURL))
	}
	if err := reg.RegisterTool(emitautoresearchartifact.ToolName, artifactExecutor); err != nil {
		return fmt.Errorf("register %s: %w", emitautoresearchartifact.ToolName, err)
	}
	logger.Info("Registered autoresearch product tools",
		slog.String("category", "autoresearch"),
		slog.Int("count", 3))
	return nil
}

// chainAttestationReader implements
// emitautoresearchartifact.ChainAttestationReader by composing
// chain.ResolveChainEntityID (related_loops → resolver-walk fallback)
// with chain.NATSEntityReader. Returns the
// sandbox.attestation.host_workspace_folder triple when the chain has
// a ready per-tenant devcontainer; empty string otherwise (no
// attestation, not ready, or wsf empty — all "no per-tenant container
// available" from the writer's perspective).
//
// Mirrors sandboxruntime.AttestationRunner.resolveWorkspaceFolder: same
// ready+wsf gating, same fail-soft posture. Distinct types because
// AttestationRunner reads via taskID (the chain ID rewritten by
// chainbash) while this reader works from a tool ToolCall — different
// call sites, identical decision logic.
type chainAttestationReader struct {
	entities *chain.NATSEntityReader
	resolver chain.IDResolver
	platform types.PlatformMeta
	logger   *slog.Logger
}

func (r *chainAttestationReader) HostWorkspaceFolder(ctx context.Context, call agentic.ToolCall) (string, error) {
	chainEntityID, err := chain.ResolveChainEntityID(ctx, call, r.resolver, r.platform, r.logger)
	if err != nil {
		// Surface the resolver error so the executor can decide whether
		// to log + fall back. The chain-root case (loop_id == chain_id,
		// no parent triple) returns a valid entity ID via the resolver
		// branch, NOT an error.
		return "", fmt.Errorf("resolve chain entity from call: %w", err)
	}
	triples, err := r.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		return "", fmt.Errorf("read chain entity %s: %w", chainEntityID, err)
	}
	if triples == nil {
		return "", nil
	}
	ready, _ := triples[sandboxmanager.PredicateAttestationReady].(bool)
	if !ready {
		return "", nil
	}
	wsf, _ := triples[sandboxmanager.PredicateAttestationHostWorkspaceFolder].(string)
	return wsf, nil
}

// sandboxArtifactWriter implements
// emitautoresearchartifact.SandboxFileWriter by POST-ing a single
// bash command through upstream's runner.Client (the same /exec
// endpoint chainbash + SandboxAPIRunner use). The command mkdir's
// the target parent + base64-decodes the artifact content into the
// destination — base64 over single-quoted argv side-steps every
// shell-escape collision class (newlines, quotes, backticks) for
// arbitrary markdown content.
//
// task_id is a fixed string because the artifact write is idempotent
// and doesn't need to serialize against any other per-chain queue —
// the synthesize phase emits exactly one artifact between gather
// and review. (Per sandbox /exec contract, task_id only governs
// serialization; the file ends up at an absolute host path
// regardless of which queue handled it.)
// sandboxExecer is the narrow surface sandboxArtifactWriter uses to
// POST a single command at the sandbox /exec endpoint.
// *runner.Client satisfies it; tests inject fakes to assert command
// shape without spinning up a sandbox.
type sandboxExecer interface {
	Exec(ctx context.Context, taskID, command string, timeoutMs int) (*runner.ExecResult, error)
}

type sandboxArtifactWriter struct {
	execer sandboxExecer
	logger *slog.Logger
}

const (
	// sandboxWriterTaskID names the per-task serialization queue used
	// for artifact writes. Constant: every artifact write serializes
	// against every other — fine because they're rare + fast.
	sandboxWriterTaskID = "emit-autoresearch-artifact"

	// sandboxWriterTimeoutMs caps the sandbox-side wallclock per write.
	// 15s is generous for a mkdir + base64-decode + redirect on a few-KB
	// markdown artifact.
	sandboxWriterTimeoutMs = 15_000

	// sandboxWriterMaxContentBytes is the upper bound on raw content
	// size. Linux argv typically tops out at ~128KB; base64 inflates
	// content by ~33%, so 64KB raw + framing ≈ 86KB on argv. An
	// autoresearch artifact has never exceeded a few KB; cap at 256KB
	// to leave room without ever realistically hitting it. Beyond
	// this, switch to a multi-step write (e.g. POST through runner
	// WriteFile's /file endpoint) — not worth the complexity until
	// the data actually demands it.
	sandboxWriterMaxContentBytes = 256 * 1024
)

func newSandboxArtifactWriter(sandboxURL string, logger *slog.Logger) *sandboxArtifactWriter {
	return &sandboxArtifactWriter{
		execer: runner.NewClient(sandboxURL),
		logger: logger,
	}
}

func (w *sandboxArtifactWriter) WriteFile(ctx context.Context, hostWsf, relPath string, content []byte) error {
	if hostWsf == "" {
		return fmt.Errorf("sandboxArtifactWriter: hostWorkspaceFolder is empty")
	}
	if relPath == "" {
		return fmt.Errorf("sandboxArtifactWriter: relPath is empty")
	}
	if len(content) > sandboxWriterMaxContentBytes {
		return fmt.Errorf("sandboxArtifactWriter: content size %d exceeds cap %d (switch to chunked write)",
			len(content), sandboxWriterMaxContentBytes)
	}
	abs := hostWsf
	if !strings.HasSuffix(abs, "/") {
		abs += "/"
	}
	abs += strings.TrimPrefix(relPath, "/")
	// abs is guaranteed to contain a "/" by the normalization above:
	// hostWsf is non-empty (empty rejected at the API boundary) and we
	// always append a trailing slash before concatenating relPath. The
	// LastIndex contract is then total — but a future refactor that
	// bypasses the normalization would hit the -1 panic shape, so the
	// guard stays explicit.
	idx := strings.LastIndex(abs, "/")
	if idx < 0 {
		return fmt.Errorf("sandboxArtifactWriter: composed path %q has no directory component (normalization broken)", abs)
	}
	dir := abs[:idx]
	encoded := base64.StdEncoding.EncodeToString(content)

	// Composed command: `mkdir -p '<dir>' && printf '%s' '<b64>' | base64 -d > '<abs>'`.
	// All three substitutions go through shellSingleQuote so any
	// metacharacters in hostWsf / relPath round-trip safely. base64
	// output (charset [A-Za-z0-9+/=]) has no metacharacters, but we
	// quote it uniformly to keep the boundary discipline obvious.
	cmd := "mkdir -p " + shellSingleQuote(dir) +
		" && printf '%s' " + shellSingleQuote(encoded) +
		" | base64 -d > " + shellSingleQuote(abs)

	res, err := w.execer.Exec(ctx, sandboxWriterTaskID, cmd, sandboxWriterTimeoutMs)
	if err != nil {
		return fmt.Errorf("sandbox /exec write to %s: %w", abs, err)
	}
	if res.TimedOut {
		return fmt.Errorf("sandbox /exec write to %s timed out after %dms", abs, sandboxWriterTimeoutMs)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("sandbox /exec write to %s: exit %d; stderr: %s",
			abs, res.ExitCode, truncateForLog(res.Stderr))
	}
	w.logger.Debug("emit_autoresearch_artifact: wrote into tenant workspace",
		slog.String("host_wsf", hostWsf),
		slog.String("rel_path", relPath),
		slog.Int("bytes", len(content)))
	return nil
}

// shellSingleQuote single-quote-escapes s for safe inclusion in a
// bash command line. Mirrors sandboxruntime.shellQuote +
// sandboxmanager.joinShell's per-token escape — duplicated locally
// rather than cross-package-coupling because the escape rule is
// small + each consumer's call site has slightly different shapes.
// The output is ALWAYS wrapped in outer single quotes (even for
// empty input — `”` is valid bash for an empty positional arg).
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// truncateForLog caps a stderr/stdout snippet at a few hundred bytes
// so a runaway error doesn't blow up structured-log shippers. The
// full content stays in the sandbox container's own logs for ops.
// ToValidUTF8 guards against the slice landing mid-rune — sandbox
// stderr is overwhelmingly ASCII, but a UTF-8 message (e.g. a path
// with non-ASCII bytes) would otherwise leave a half-encoded rune in
// the structured log.
func truncateForLog(s string) string {
	const maxLogBytes = 512
	s = strings.TrimSpace(s)
	if len(s) <= maxLogBytes {
		return s
	}
	return strings.ToValidUTF8(s[:maxLogBytes], "") + "…(truncated)"
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

// Compile-time guards that the product-shell adapters satisfy the
// (structurally-identical) interfaces declared by the consumer
// packages. If a consumer interface ever widens (new method added)
// and the adapter is not extended to match, this fails to build —
// surfacing the drift here rather than at the SetX call site.
// Covers: emitplan.ChainReader, chainbash.ChainResolver/Inner, and
// the emit_autoresearch_artifact attestation-aware write path (#194).
var (
	_ emitplan.ChainReader                            = (*chain.LineageReader)(nil)
	_ chainbash.ChainResolver                         = (*chain.Resolver)(nil)
	_ chainbash.ChainResolver                         = identityChainResolver{}
	_ chainbash.Inner                                 = (*executors.BashExecutor)(nil)
	_ emitautoresearchartifact.ChainAttestationReader = (*chainAttestationReader)(nil)
	_ emitautoresearchartifact.SandboxFileWriter      = (*sandboxArtifactWriter)(nil)
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
	// Build the inner BashExecutor. Three construction shapes, picked
	// by SANDBOX_URL + natsClient availability:
	//
	//   - No SANDBOX_URL → local-exec mode. The framework's
	//     NewBashExecutorFromEnv() path; runs commands on the local
	//     process via os/exec. No attestation routing applies (there
	//     is no remote runner to wrap). Used for dev shells without
	//     the sandbox stack up.
	//
	//   - SANDBOX_URL set, no natsClient → always-warm-only mode.
	//     Construct upstream's *runner.Client directly; skip the
	//     AttestationRunner wrap because without NATS we have no way
	//     to read chain attestation triples. Functionally identical
	//     to pre-Phase-D behavior (NewBashExecutorFromEnv).
	//
	//   - SANDBOX_URL set, natsClient present → attestation-aware
	//     mode. Wrap upstream's *runner.Client with
	//     sandboxruntime.AttestationRunner so chains with a Phase-A-
	//     stamped sandbox.attestation.host_workspace_folder route into
	//     their per-tenant devcontainer via `devcontainer exec`. Other
	//     chains pass through unchanged.
	mode, bashExec := buildBashExecutor(natsClient, platform, logger)

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

	wrapper := chainbash.NewExecutor(bashExec, resolver, logger)
	if err := reg.RegisterTool(chainbash.ToolName, wrapper); err != nil {
		return fmt.Errorf("register %s: %w", chainbash.ToolName, err)
	}
	logger.Info("Registered product tool (chain-scoped bash wrapper)",
		slog.String("name", chainbash.ToolName),
		slog.String("mode", mode))
	return nil
}

// buildBashExecutor constructs the inner BashExecutor for chainbash to
// wrap, returning the construction mode (for logging) and the executor.
// See registerChainBash's docstring for the three-shape decision tree.
//
// The attestation-aware mode is the one Phase D adds: when SANDBOX_URL
// is set AND we have a NATS client (so chain entities are readable),
// the runner that BashExecutor dispatches through becomes a
// sandboxruntime.AttestationRunner wrapping upstream's *runner.Client.
// AttestationRunner reads the chain entity per call and, when an
// attested per-tenant devcontainer exists, wraps the command in
// `devcontainer exec --workspace-folder <wsf> bash -lc <cmd>` before
// delegating. Chains without attestation passthrough unchanged.
func buildBashExecutor(natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) (string, *executors.BashExecutor) {
	sandboxURL := strings.TrimSpace(os.Getenv(envSandboxURL))
	if sandboxURL == "" {
		// Local-exec mode. Nothing to wrap; matches pre-Phase-D behavior
		// for dev shells where the sandbox stack isn't up.
		return "local", executors.NewBashExecutorFromEnv()
	}
	if natsClient == nil {
		// Always-warm-only mode. No NATS → no attestation reads → no
		// routing decisions; construct upstream's default *runner.Client
		// directly from sandboxURL (vs NewBashExecutorFromEnv which
		// re-reads the env var — would open a TOCTOU window between our
		// read above and upstream's, and obscures the symmetry with the
		// attestation-aware branch below where the only delta is the
		// WithRunner option).
		return "sandbox", executors.NewBashExecutor("", sandboxURL)
	}
	// Attestation-aware mode. Construct the default *runner.Client
	// explicitly (mirroring upstream's URL→Client mapping) so we can
	// hand it to AttestationRunner as the fallback target. The
	// resulting BashExecutor sends every dispatch through our runner,
	// which decides per-call whether to wrap-and-route or passthrough.
	defaultRunner := runner.NewClient(sandboxURL)
	entityReader := chain.NewNATSEntityReader(natsClient, chain.DefaultGraphQueryEntitySubject)
	attRunner := sandboxruntime.NewAttestationRunner(defaultRunner, entityReader, platform, logger)
	// Mode string uses `-` not `+` so log shippers that tokenize on `+`
	// (some Splunk + ELK configs) see a single value, not two.
	return "sandbox-attestation", executors.NewBashExecutor("", "", executors.WithRunner(attRunner))
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
