// Package main implements the entry point for the SemStreams application.
// SemStreams is a semantic stream processing framework that combines
// protocol-level data processing with semantic knowledge graph capabilities.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers on DefaultServeMux
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	"github.com/c360studio/semstreams/config"
	flowengine "github.com/c360studio/semstreams/engine"
	"github.com/c360studio/semstreams/flowstore"
	"github.com/c360studio/semstreams/flowtemplate"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/persona"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/processor/agentic-tools/executors"
	rulepkg "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/chainpause"
	"github.com/c360studio/semteams/cmd/semteams/evidence"
	"github.com/c360studio/semteams/cmd/semteams/flowtemplates"
	"github.com/c360studio/semteams/cmd/semteams/portresolver"
	"github.com/c360studio/semteams/cmd/semteams/testharness"
)

// Build information constants
const (
	Version   = "0.1.0"
	BuildTime = "dev"
	appName   = "semstreams"
)

func main() {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			_, _ = fmt.Fprintf(os.Stderr, "PANIC: %v\nStack trace:\n%s\n", r, string(buf[:n]))
			os.Exit(2)
		}
	}()

	// Run application with proper error handling
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Print banner
	printBanner()

	// 2. Parse and validate CLI flags
	cliCfg, shouldExit, err := parseCLI()
	if shouldExit || err != nil {
		return err
	}

	// 2.5. Start pprof server if debug mode enabled (before NATS - independent)
	if cliCfg.Debug && cliCfg.DebugPort > 0 {
		go startPProfServer(cliCfg.DebugPort)
	}

	// 3. Load and validate configuration
	cfg, err := loadConfig(cliCfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if cliCfg.Validate {
		fmt.Println("✓ Configuration is valid")
		return nil
	}

	// 4. Connect to NATS (required - semstreams cannot operate without NATS)
	ctx := context.Background()
	natsClient, err := connectToNATSWithSpinner(ctx, cfg)
	if err != nil {
		return err
	}
	defer natsClient.Close(ctx)

	// 5. Ensure JetStream streams exist (LOGS, HEALTH, METRICS, FLOWS)
	if err := ensureStreamsWithSpinner(ctx, cfg, natsClient); err != nil {
		return err
	}

	// 6. NOW create the full logger with NATS publisher (no nil, no mutation)
	logger := setupLogger(cliCfg.LogLevel, cliCfg.LogFormat, natsClient, cfg)
	slog.SetDefault(logger)

	slog.Info("SemStreams ready",
		"version", Version,
		"build_time", BuildTime)

	// 7. Create remaining infrastructure
	metricsRegistry, platform, configManager, err := setupRemainingInfrastructure(ctx, cfg, natsClient, logger)
	if err != nil {
		return err
	}
	defer configManager.Stop(5 * time.Second)

	// 8. Setup registries and manager
	componentRegistry, manager, err := setupRegistriesAndManager(cfg)
	if err != nil {
		return err
	}

	// 9a. Build the shared payload registry and register first-party
	// builtins (agentic, message, dispatch, rule, operating-model,
	// github-webhook, objectstore). Per beta.18: payload registry is
	// constructor-injected via component.Dependencies.PayloadRegistry,
	// so it must exist before services are constructed. Mirrors upstream
	// cmd/semstreams/main.go.
	payloadReg := payloadregistry.New()
	if err := payloadbuiltins.Register(payloadReg); err != nil {
		return fmt.Errorf("register builtin payloads: %w", err)
	}

	// 9a.1. Register SemTeams-local product payloads on top of the
	// framework's first-party set. See registerProductPayloads for the
	// full set and their ADR references.
	if err := registerProductPayloads(payloadReg); err != nil {
		return fmt.Errorf("register product payloads: %w", err)
	}

	// 9b. Load operator-curated platform assets (test_harness catalog +
	// persona fragments + rendered test_harness fragment for researcher
	// roles). Test harness catalog (ADR-033 R3.7.1) is built FIRST so
	// the rendered fragment reflects the curated state; persona
	// file load runs next; the rendered list is upserted into the
	// PERSONAS bucket last. Both managers are returned — persona
	// feeds the tool registry below for Pattern-B persona CRUD,
	// testHarnessMgr feeds the /harnesses HTTP middleware (R3.7.1.f).
	personaMgr, testHarnessMgr := loadPlatformAssets(ctx, natsClient, cliCfg, slog.Default())

	// 9b-bis. Seed the FLOW_TEMPLATES KV bucket from configs/flow-templates/.
	// ADR-042 Phase 1. Must run before setupToolsAndPreprocessor so the
	// list_flow_templates tool sees the inventory on first call. Manager
	// is threaded through to RegisterBuiltins via the deps struct.
	flowTemplateMgr := loadFlowTemplates(ctx, natsClient, cliCfg.FlowTemplatesPath, slog.Default())

	// 9c–9f. Build + register tool executors, start the evidence preprocessor
	// (ADR-036 §Phase 2), and start the chain-pause subscriber (ADR-037 v1).
	// Extracted to keep run() under revive's function-length threshold while
	// keeping the ordering invariant: tools before svcDeps, preprocessors after.
	toolRegistry, chainPauseHTTP, err := setupToolsAndPreprocessor(ctx, cfg, natsClient, platform, configManager, componentRegistry, personaMgr, flowTemplateMgr, testHarnessMgr, metricsRegistry, cliCfg.WorkspaceRoot, slog.Default())
	if err != nil {
		return err
	}

	// 10. Create service dependencies, plumbing the shared registries so
	// every component constructed by the service manager sees the same
	// tool + payload registry instances.
	svcDeps := createServiceDependencies(natsClient, metricsRegistry, logger, platform, configManager, componentRegistry)
	svcDeps.ToolRegistry = toolRegistry
	svcDeps.PayloadRegistry = payloadReg

	// 11. Configure and create services
	if err := configureAndCreateServices(cfg, manager, svcDeps); err != nil {
		return err
	}

	// 12. Register product HTTP middleware. Per beta.23: must run before
	// StartAll, since the framework reads the chain at server boot. The
	// chain lifts X-User-Id headers into agentic-dispatch's identity ctx
	// so beta.22's POST /teams-dispatch/loops/{id}/approval handler (and
	// any other identity-aware handler) sees the caller without a body
	// claim. See cmd/semteams/middleware.go for the chain definition.
	//
	// Placement: any step after configureAndCreateServices and before
	// runWithSignalHandling works. Moving it after StartAll silently
	// drops the chain — the framework logs a warning, but the binary
	// boots green and X-User-Id is ignored.
	manager.UseHTTPMiddleware(productMiddleware(testHarnessMgr, chainPauseHTTP, slog.Default())...)

	// 13. Run application with signal handling
	return runWithSignalHandling(ctx, manager, cliCfg.ShutdownTimeout)
}

// setupToolsAndPreprocessor groups the ordered tool-registry + evidence-
// preprocessor + chain-pause subscriber wiring so run()'s statement count
// stays under revive's function-length limit. Ordering within this function is
// load-bearing: tools must be registered before svcDeps is built; the
// preprocessors subscribe after tools so their triple-publishers are live.
//
// Returns the tool registry and the chain-pause HTTP handler. The HTTP handler
// is wired into productMiddleware by run() so it is registered before StartAll.
func setupToolsAndPreprocessor(
	ctx context.Context,
	cfg *config.Config,
	natsClient *natsclient.Client,
	platform types.PlatformMeta,
	configManager *config.Manager,
	componentRegistry *component.Registry,
	personaMgr *persona.Manager,
	flowTemplateMgr *flowtemplate.Manager,
	testHarnessMgr *testharness.Manager,
	metricsRegistry *metric.MetricsRegistry,
	workspaceRoot string,
	logger *slog.Logger,
) (*agentictools.ExecutorRegistry, *chainpause.HTTPHandler, error) {
	// 9c. First-party tool executors. Pattern-B tools (create_rule, etc.)
	// each need their matching manager; nil manager → registerX skips.
	//
	// SkipBuiltins[bash]: ADR-041 Phase 4. The product shell registers
	// its own chain-scoped wrapper under the canonical "bash" name in
	// registerProductTools (registerChainBash). The wrapper rewrites
	// Metadata["task_id"] to chain_id so every role in a chain shares one
	// sandbox worktree. SkipBuiltins=[bash] omits the framework bash so
	// the slot is free for the wrapper.
	// Build flow manager + engine together so they share a single
	// *flowstore.Manager instance. Engine writes to semstreams_config KV;
	// component_manager (upstream) watches that bucket and spins up
	// components dynamically (multi-flow runtime per ADR-042 §Phase 4
	// addendum). Nil flow manager → engine nil → registerFlowLifecycle
	// skips. Same gate posture as the other Pattern-B tools.
	flowMgr := buildFlowManager(natsClient, logger)
	flowEngine := buildFlowEngine(configManager, flowMgr, componentRegistry, natsClient, metricsRegistry, logger)

	toolRegistry := agentictools.NewExecutorRegistry()
	if err := executors.RegisterBuiltins(ctx, toolRegistry, executors.ToolDependencies{
		NATSClient:          natsClient,
		Platform:            platform,
		Logger:              logger,
		RuleManager:         buildRuleManager(ctx, natsClient, configManager, logger),
		FlowManager:         flowMgr,
		PersonaManager:      personaMgr,
		FlowTemplateManager: flowTemplateMgr,
		FlowEngineManager:   flowEngine,
		ComponentRegistry:   componentRegistry,
		LoopsBucket:         extractLoopsBucket(cfg),
		SkipBuiltins:        []string{"bash"},
	}); err != nil {
		return nil, nil, fmt.Errorf("register builtin tools: %w", err)
	}

	// 9d. Product-shell-local tool executors (add_source_repo,
	// emit_research_artifact, emit_dev_via_spec_artifact, builder_decide,
	// bootstrap_workspace, chain-scoped bash wrapper). Each tool's
	// registration comment lives in registerProductTools; see ADR-029 +
	// tools/README.md for discipline.
	if err := registerProductTools(toolRegistry, natsClient, platform, testHarnessMgr, logger); err != nil {
		return nil, nil, fmt.Errorf("register product tools: %w", err)
	}

	// 9e. Evidence preprocessor (ADR-036 §Phase 2, R3.7.2.k′-bis).
	// Subscribes to agent.complete.> and stamps evidence.summary +
	// evidence.summary_ready on builder loop entities.
	// Disabled when workspaceRoot is empty — non-sandbox deployments.
	if err := startEvidencePreprocessor(ctx, cfg, natsClient, platform, workspaceRoot, logger); err != nil {
		return nil, nil, fmt.Errorf("start evidence preprocessor: %w", err)
	}

	// 9f. Chain-pause subscriber + HTTP handler (ADR-037 v1).
	// Subscribes to agent.failed.> and stamps §D5 audit triples when a
	// managed-arc loop fails. Returns the HTTP handler for POST
	// /teams-loop/chain-pause/decide — mounted in productMiddleware.
	chainPauseHTTP, err := startChainPauseSubscriber(ctx, cfg, natsClient, platform, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("start chain-pause subscriber: %w", err)
	}

	// 9g. Chain milestone subscribers (ADR-038 PR B).
	// One CompletionSubscriber on agent.complete.> demuxes to every
	// registered chain.CompletionHandler. Each handler decides whether
	// to fire on the event and writes its predicate cluster onto the
	// canonical 6-part chain entity (c360.<platform>.agent.chain.execution.<chain_id>).
	if err := startChainMilestoneSubscribers(ctx, cfg, natsClient, platform, logger); err != nil {
		return nil, nil, fmt.Errorf("start chain milestone subscribers: %w", err)
	}

	return toolRegistry, chainPauseHTTP, nil
}

// startChainMilestoneSubscribers wires every ADR-038 chain-milestone
// CompletionHandler into a single agent.complete.> subscription.
// Each handler picks events matching its milestone and writes its
// predicate cluster onto the canonical chain entity. Adding a new
// milestone is one line in the handler slice.
//
// Phase 1b: chain.dispatched_at on chain root.
// Phase 2:  chain.research_artifact.* on research-reviewer approval.
// Phase C4 (evidence summary milestone) plugs in via the same slice.
// ADR-039 Phase 1 Slice B: chain.needs_review.* on builder
// needs_clarification (Tier 3 fallback).
// ADR-040 §addendum 2026-05-11: chain.recovery.count (audit) + per-cycle
// chain.recovery.proceed (gate sentinel on reviewer entity) +
// chain.recovery.exhausted (cap-hit marker) on research-reviewer
// "insufficient" terminals. Counter-owned gating; rule_02's third
// condition (`chain.recovery.proceed eq "true"`) fires only when the
// Counter has approved the cycle.
func startChainMilestoneSubscribers(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)

	// SUBSCRIBE-side subjects (wildcards) come from running config so an
	// operator port-rewire follows automatically. See ADR-039 / fix-plan
	// Phase 2: hardcoded subjects across cmd/semteams/ are the smoke #8
	// root-cause class.
	loopCompletedSubject := portresolver.SubjectOrDefault(cfg, "teams-loop", "agent.complete", chain.DefaultLoopCompletedSubject)

	// REQUEST/REPLY subject is the constant — graph-query subscribes to
	// the specific literal "graph.query.entity" inside its
	// setupQueryHandlers; the port config declares the wildcard
	// `graph.query.>` only as namespace metadata. An operator who wants
	// to rewire the entity-read RPC has to patch upstream graph-query's
	// handler, not just the port config. Constructor still accepts the
	// param so tests + future config-overrides work the moment that
	// upstream surface gets parameterized.
	entityReader := chain.NewNATSEntityReader(natsClient, chain.DefaultGraphQueryEntitySubject)
	resolver := chain.NewResolver(chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject), platform)

	dispatched := chain.NewDispatchedStamper(triplePublisher, platform, logger)
	research := chain.NewResearchMilestoneStamper(triplePublisher, resolver, entityReader, platform, logger)
	needsReview := chain.NewNeedsReviewStamper(triplePublisher, resolver, entityReader, platform, logger)

	// chain.terminal stamper. Fires on terminating-reviewer success
	// (reviewer-research/approved under the MVP roster; see ADR-042
	// §Phase 2 redesign for the closed taxonomy) and writes the
	// chain.terminal.* audit cluster on the canonical 6-part chain
	// entity. The wake-up rule (research/07-reviewer-approved-to-
	// coordinator.json) fires on the reviewer loop's role + decision
	// triples directly (rule engine's firing entity = the loop, not
	// the chain); the chain-entity cluster is for ops queries and
	// operator dashboards.
	terminalStamper := chain.NewTerminalStamper(triplePublisher, resolver, entityReader, platform, logger)

	subscriber := chain.NewCompletionSubscriber([]chain.CompletionHandler{
		dispatched,
		terminalStamper,
		research,
		needsReview,
	}, loopCompletedSubject, logger)
	if err := subscriber.Start(ctx, natsClient); err != nil {
		return fmt.Errorf("subscribe to loop completed for chain milestones: %w", err)
	}
	logger.Info("chain milestone subscribers started",
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform),
		slog.String("loop_completed_subject", loopCompletedSubject))
	return nil
}

// startChainPauseSubscriber builds the chain-pause pauser + decision handler,
// starts the NATS subscriber on agent.failed.>, and returns the HTTP handler
// for POST /teams-loop/chain-pause/decide. ADR-037 v1 — operator authority only.
//
// TODO(adr-037-d3): When chain_failure_authority config field lands, validate
// at config-load that only "operator" is accepted in v1. "coordinator" / "auto"
// must reject with explicit migration message.
func startChainPauseSubscriber(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) (*chainpause.HTTPHandler, error) {
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	taskPublisher := chainpause.NewNATSTaskPublisher(natsClient)

	// SUBSCRIBE-side (agent.failed wildcard): config-derived. REQUEST-
	// side (graph.query.entity literal): constant — see startChainMilestoneSubscribers
	// for the same rationale.
	loopFailedSubject := portresolver.SubjectOrDefault(cfg, "teams-loop", "agent.failed", chainpause.DefaultLoopFailedSubject)
	pauseDataReader := chainpause.NewNATSPauseDataReader(natsClient, chainpause.DefaultGraphQueryEntitySubject)

	// ADR-038 PR B Phase 3: chainpause writes §D5 audit triples on the
	// canonical chain entity (post-semstreams beta.57 the publish-vs-write
	// race is closed, so the resolver's ancestry walk is reliable on
	// failed loops too). Same chain.Resolver shape used by every other
	// chain-triple consumer in the product shell.
	chainResolver := chain.NewResolver(chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject), platform)

	pauser := chainpause.NewPauser(triplePublisher, chainResolver)
	sub := chainpause.NewSubscriber(pauser, loopFailedSubject, logger)
	if err := sub.Start(ctx, natsClient); err != nil {
		return nil, fmt.Errorf("subscribe to agent.failed events: %w", err)
	}
	logger.Info("chain-pause subscriber started",
		slog.String("org", platform.Org),
		slog.String("platform", platform.Platform),
		slog.String("loop_failed_subject", loopFailedSubject))

	decisionHandler := chainpause.NewDecisionHandler(triplePublisher, taskPublisher, pauseDataReader, chainResolver, logger)
	httpHandler := chainpause.NewHTTPHandler(decisionHandler, logger)
	return httpHandler, nil
}

// startEvidencePreprocessor builds the evidence registry with builtins,
// wraps it in a Preprocessor, and starts its NATS subscription. The
// subscription is bound to ctx — cancelling ctx (on shutdown signal) will
// unsubscribe cleanly.
//
// workspaceRoot="" disables the preprocessor; the Preprocessor's
// HandleLoopCompleted returns immediately for every event. This keeps
// non-sandbox deployments booting without error.
func startEvidencePreprocessor(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client, platform types.PlatformMeta, workspaceRoot string, logger *slog.Logger) error {
	reg, err := evidence.NewWithBuiltins()
	if err != nil {
		return fmt.Errorf("build evidence registry: %w", err)
	}
	triplePublisher := agentictools.NewNATSTriplePublisher(natsClient)
	// outputDir empty → preprocessor reads SEMTEAMS_EVIDENCE_DIR or falls
	// back to "docs/evidence". ADR-038 PR C Phase C4: rendered markdown
	// view at the chain.evidence.summary.path reference.
	preprocessor := evidence.New(reg, triplePublisher, workspaceRoot, "", platform, logger)
	// ADR-038 PR B Phase 5 + PR C Phase C4: opt the preprocessor in to
	// chain entity triple writes so chain.evidence.summary_ready and
	// chain.evidence.summary.path land alongside the existing
	// loop-entity triples. Drift-safe — rule_07 still matches on the
	// loop entity. Markdown render fires from the same chain-opt-in
	// path.
	preprocessor.SetChainResolver(chain.NewResolver(chain.NewNATSParentReader(natsClient, platform, chain.DefaultGraphQueryEntitySubject), platform))

	// SUBSCRIBE-side: config-derived. Same agentic-loop port the chain
	// milestone subscriber binds to.
	loopCompletedSubject := portresolver.SubjectOrDefault(cfg, "teams-loop", "agent.complete", evidence.DefaultLoopCompletedSubject)
	sub := evidence.NewNATSSubscriber(preprocessor, loopCompletedSubject, logger)
	if err := sub.Start(ctx, natsClient); err != nil {
		return fmt.Errorf("subscribe to loop completed events: %w", err)
	}
	if workspaceRoot != "" {
		logger.Info("evidence preprocessor started",
			slog.String("workspace_root", workspaceRoot),
			slog.String("output_dir", preprocessor.OutputDir()),
			slog.String("loop_completed_subject", loopCompletedSubject),
			slog.String("org", platform.Org),
			slog.String("platform", platform.Platform))
	} else {
		logger.Info("evidence preprocessor disabled (workspace-root unset; set --workspace-root for sandbox deployments)")
	}
	return nil
}

// loadPersonaFragments seeds the PERSONAS KV bucket from a directory
// tree shaped <root>/<role>/*.md and returns the manager so it can be
// threaded into executors.RegisterBuiltins. Non-fatal on init failure —
// callers must nil-check the return before relying on it.
func loadPersonaFragments(ctx context.Context, natsClient *natsclient.Client, root string) *persona.Manager {
	if root == "" {
		slog.Debug("persona fragments path empty, skipping load")
		return nil
	}
	mgr, err := persona.NewManager(natsClient)
	if err != nil {
		slog.Warn("persona manager init failed; persona CRUD tools and fragment loading disabled",
			"error", err)
		return nil
	}
	slog.Info("loading persona fragments", "root", root)
	if err := persona.LoadFromDirectory(ctx, root, mgr, slog.Default()); err != nil {
		slog.Warn("persona fragment loader reported errors",
			"path", root,
			"error", err)
		// Return the manager anyway — partial load is better than no persona
		// CRUD tooling and the caller already logged the specifics.
	}
	return mgr
}

// extractLoopsBucket pulls the agentic-tools loops_bucket config value so
// executors.RegisterBuiltins can thread it into the stateful-tool registrations
// (read_loop_result, flow_monitor). Empty return lets RegisterBuiltins fall
// back to the AGENT_LOOPS default. Independent reimplementation of
// upstream cmd/semstreams/main.go per ADR-029 — not an import.
func extractLoopsBucket(cfg *config.Config) string {
	for _, cc := range cfg.Components {
		if cc.Name != "agentic-tools" || !cc.Enabled {
			continue
		}
		var tcfg struct {
			LoopsBucket string `json:"loops_bucket"`
		}
		if err := json.Unmarshal(cc.Config, &tcfg); err == nil && tcfg.LoopsBucket != "" {
			return tcfg.LoopsBucket
		}
	}
	return ""
}

// buildRuleManager constructs a rule.ConfigManager (KV-backed rule CRUD)
// for use by the Pattern-B rule executors. Nil on init failure →
// registerRules skips. Note: upstream's runtime hot-reload ConfigManager
// lives on the rule processor itself and reads the same KV bucket — two
// instances coexist safely (NATS KV serialises per-key writes). Ours is
// write-only CRUD for agentic-tools; the processor-internal one is
// read+apply. Independent reimplementation per ADR-029.
func buildRuleManager(ctx context.Context, natsClient *natsclient.Client, configMgr *config.Manager, logger *slog.Logger) executors.RuleManager {
	rcm := rulepkg.NewConfigManager(nil, configMgr, logger)
	if err := rcm.InitializeKVStore(natsClient); err != nil {
		logger.Warn("rule CRUD tools disabled: could not initialise rules KV store",
			"error", err)
		return nil
	}
	_ = ctx // reserved for future use if KV init needs a context
	return rcm
}

// buildFlowManager constructs a flowstore.Manager (KV-backed flow CRUD).
// Nil on init failure → registerFlows skips. Independent reimplementation
// per ADR-029.
//
// Returns the concrete *flowstore.Manager rather than the
// executors.FlowManager interface so the same instance can be threaded
// into both the agentic-tools deps struct (for create_flow / get_flow
// CRUD) and flowengine.NewEngine (for deploy_flow / start_flow
// lifecycle, ADR-042 Phase 4). The concrete type satisfies the
// interface implicitly.
func buildFlowManager(natsClient *natsclient.Client, logger *slog.Logger) *flowstore.Manager {
	mgr, err := flowstore.NewManager(natsClient)
	if err != nil {
		logger.Warn("flow CRUD tools disabled: could not initialise flow store",
			"error", err)
		return nil
	}
	return mgr
}

// buildFlowEngine constructs a flowengine.Engine for ADR-042 Phase 4's
// deploy_flow / start_flow / stop_flow / undeploy_flow agent tools
// (semstreams beta.76 surface). Returns nil when the flow manager is
// nil — registerFlowLifecycle skips when the dep is nil, same gate
// pattern as the other Pattern-B tools.
//
// The engine writes to semstreams_config KV; service/component_manager
// already watches that bucket and spins up new components dynamically
// (multi-flow runtime). No restart, no second binary. ADR-042 Phase 4
// addendum captures the investigation.
func buildFlowEngine(configMgr *config.Manager, flowMgr *flowstore.Manager, componentRegistry *component.Registry, natsClient *natsclient.Client, metricsRegistry *metric.MetricsRegistry, logger *slog.Logger) *flowengine.Engine {
	if flowMgr == nil {
		logger.Warn("flow-lifecycle tools disabled: flow manager is nil")
		return nil
	}
	return flowengine.NewEngine(configMgr, flowMgr, componentRegistry, natsClient, logger, metricsRegistry)
}

// buildFlowTemplateManager constructs a flowtemplate.Manager (KV-backed
// template CRUD + render). Nil on init failure → registerFlowTemplates
// skips and the seed loader becomes a no-op. Independent reimplementation
// per ADR-029.
//
// Returns the concrete *flowtemplate.Manager rather than the
// executors.FlowTemplateManager interface so the same instance can be
// threaded into both the seed loader (ADR-042 Phase 1) and the
// agentic-tools deps struct. The concrete type satisfies the interface
// implicitly.
func buildFlowTemplateManager(natsClient *natsclient.Client, logger *slog.Logger) *flowtemplate.Manager {
	mgr, err := flowtemplate.NewManager(natsClient)
	if err != nil {
		logger.Warn("flow-template tools disabled: could not initialise flow-template store",
			"error", err)
		return nil
	}
	return mgr
}

// loadFlowTemplates seeds the FLOW_TEMPLATES KV bucket from a directory
// of flat *.json files. Mirror of loadPersonaFragments — same boot-time
// upsert pattern with a different manager. ADR-042 Phase 1.
//
// Returns the manager threaded through so the caller can pass it into
// the agentic-tools deps struct alongside other Pattern-B managers.
// Nil manager and missing directory are both non-fatal: the manager
// being nil disables the template tools entirely; a missing directory
// just means the operator hasn't authored any templates yet.
func loadFlowTemplates(ctx context.Context, natsClient *natsclient.Client, root string, logger *slog.Logger) *flowtemplate.Manager {
	mgr := buildFlowTemplateManager(natsClient, logger)
	if mgr == nil {
		return nil
	}
	if root == "" {
		logger.Debug("flow-templates path empty, skipping seed")
		return mgr
	}
	logger.Info("loading flow templates", "root", root)
	if err := flowtemplates.LoadFromDirectory(ctx, root, mgr, logger); err != nil {
		logger.Warn("flow-template loader reported errors",
			"path", root,
			"error", err)
		// Return the manager anyway — partial seed is better than no
		// template CRUD tooling, same posture as loadPersonaFragments.
	}
	return mgr
}

// loadPlatformAssets builds the harness catalog (operator-curated
// test-harness registry; ADR-033 R3.7.1) and seeds the PERSONAS KV
// bucket from on-disk fragment files plus a synthetic researcher
// fragment rendered from the live catalog. Harness load runs FIRST
// so the rendered list reflects the catalog state operators just
// curated; persona.LoadFromDirectory then loads the static
// fragments; finally the rendered list is upserted under a stable
// fragment ID — Upsert rather than file-write keeps the source
// tree clean (no boot-time git diffs in configs/personas/).
func loadPlatformAssets(ctx context.Context, natsClient *natsclient.Client, cliCfg *CLIConfig, logger *slog.Logger) (*persona.Manager, *testharness.Manager) {
	testHarnessMgr := buildTestHarnessManager(ctx, natsClient, cliCfg.HarnessCatalogPath, logger)
	personaMgr := loadPersonaFragments(ctx, natsClient, cliCfg.PersonaFragmentsPath)
	injectRenderedTestHarnessFragment(ctx, personaMgr, testHarnessMgr, logger)
	return personaMgr, testHarnessMgr
}

// injectRenderedTestHarnessFragment renders the live test harness catalog
// into a researcher persona fragment and upserts it into the
// PERSONAS KV bucket. Skipped when either manager is nil (tests,
// or boot-time KV failure that already logged its own warning).
// Uses multi-role on the Persona so a single record applies to
// every researcher / reviewer role that touches a test_harness
// field. Under the ADR-042 MVP-7 roster the live roster is the
// research-category synthesize phase + reviewer-research; future
// dev-via-spec category packs reintroducing architect/reviewer-spec
// would extend this list.
//
// Fragment ID `test-harness-catalog.rendered` is intentionally NOT in
// the project's `\d+-` prefix style operators use for hand-
// authored markdown files — the dot-separator and prefix-less
// shape mark it as synthetic. The persona file loader keys
// Upserts on ID, so an operator who later authors a file at
// `test-harness-catalog.rendered.md` would still collide; the visual
// distinctness is the operator-facing breadcrumb.
//
// Category=0 / Priority=45: Category matches the project's
// existing baseline (every hand-authored fragment defaults to
// Category=0 → CategorySystem because the file loader doesn't
// parse front-matter). Priority=45 places this record AFTER our
// static 40-harness-catalog.md instructions within the same
// category, which is the design intent. Intra-Priority ordering
// for Priority=0 fragments is governed by map-iteration order
// (pre-existing framework nondeterminism, see ADR-033 §addendum
// 2026-05-03).
func injectRenderedTestHarnessFragment(ctx context.Context, personaMgr *persona.Manager, testHarnessMgr *testharness.Manager, logger *slog.Logger) {
	if personaMgr == nil || testHarnessMgr == nil {
		return
	}
	catalog, err := testHarnessMgr.List(ctx)
	if err != nil {
		logger.Warn("test_harness catalog: skipped persona-render injection (List failed)",
			"error", err)
		return
	}
	body := testharness.RenderResearcherFragment(catalog)
	p := &persona.Persona{
		ID:       "test-harness-catalog.rendered",
		Category: 0, // CategorySystem — matches project baseline; see doc-comment.
		Priority: 45,
		Content:  body,
		// The catalog is loaded for every role that touches a
		// research.artifact.test_harness field — under the ADR-042
		// MVP-7 roster this is the research-category synthesize phase
		// (selects test_harness) + reviewer-research (verifies the
		// stance). Future category packs introducing architect /
		// reviewer-spec roles would extend the list.
		Roles: []string{
			"researcher-research-synthesize",
			"reviewer-research",
		},
		Description: "Auto-generated from configs/harnesses.json at boot (ADR-033 R3.7.1).",
	}
	if err := personaMgr.Upsert(ctx, p); err != nil {
		logger.Warn("test_harness catalog: synthetic persona fragment upsert failed",
			"fragment_id", p.ID,
			"error", err)
		return
	}
	if len(catalog) == 0 {
		logger.Info("test_harness catalog: rendered persona fragment injected (empty catalog notice)",
			"fragment_id", p.ID,
			"roles", p.Roles)
	} else {
		logger.Info("test_harness catalog: rendered persona fragment injected",
			"fragment_id", p.ID,
			"catalog_entries", len(catalog),
			"roles", p.Roles)
	}
}

// buildTestHarnessManager constructs the SemTeams test harness catalog manager
// (R3.7.1, ADR-033) and seeds it from the operator-curated JSON file.
// Returns nil if the KV bucket cannot be opened — the chain still
// boots; `needs_test_harness` paths report that no catalog is available.
// A missing catalog file is NOT an error: deployments that don't use
// test harnesses simply have an empty catalog.
func buildTestHarnessManager(ctx context.Context, natsClient *natsclient.Client, catalogPath string, logger *slog.Logger) *testharness.Manager {
	mgr, err := testharness.NewManager(natsClient)
	if err != nil {
		logger.Warn("test_harness catalog disabled: could not initialise HARNESSES KV bucket",
			"error", err)
		return nil
	}
	n, err := mgr.LoadFromFile(ctx, catalogPath)
	if err != nil {
		logger.Warn("test_harness catalog file load failed; catalog left in current state",
			"path", catalogPath,
			"error", err)
		return mgr
	}
	logger.Info("test_harness catalog loaded",
		"path", catalogPath,
		"entries_loaded", n)
	return mgr
}

// parseCLI parses and validates CLI flags.
func parseCLI() (*CLIConfig, bool, error) {
	cliCfg := parseFlags()
	if err := validateFlags(cliCfg); err != nil {
		return nil, false, fmt.Errorf("invalid flags: %w", err)
	}

	if cliCfg.ShowVersion {
		fmt.Printf("%s version %s\n", appName, Version)
		return nil, true, nil
	}

	if cliCfg.ShowHelp {
		printHelp()
		return nil, true, nil
	}

	return cliCfg, false, nil
}

// connectToNATSWithSpinner connects to NATS with a spinner for user feedback.
// NATS is a hard requirement - semstreams cannot operate without it.
func connectToNATSWithSpinner(ctx context.Context, cfg *config.Config) (*natsclient.Client, error) {
	spinner := NewSpinner("Connecting to NATS...")
	spinner.Start()

	natsClient, err := createNATSClient(cfg)
	if err != nil {
		spinner.StopWithError(err)
		return nil, fmt.Errorf("create NATS client: %w", err)
	}

	if err := natsClient.Connect(ctx); err != nil {
		spinner.StopWithError(err)
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := natsClient.WaitForConnection(connCtx); err != nil {
		spinner.StopWithError(err)
		return nil, fmt.Errorf("NATS connection timeout: %w", err)
	}

	spinner.Stop()
	return natsClient, nil
}

// ensureStreamsWithSpinner creates JetStream streams with a spinner for user feedback.
func ensureStreamsWithSpinner(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client) error {
	spinner := NewSpinner("Creating JetStream streams...")
	spinner.Start()

	// Use a quiet logger for stream creation (we have the spinner for feedback)
	quietLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	streamsManager := config.NewStreamsManager(natsClient, quietLogger)

	if err := streamsManager.EnsureStreams(ctx, cfg); err != nil {
		spinner.StopWithError(err)
		return fmt.Errorf("ensure streams: %w", err)
	}

	spinner.Stop()
	return nil
}

// setupRemainingInfrastructure creates metrics, platform, and config manager.
func setupRemainingInfrastructure(
	ctx context.Context,
	cfg *config.Config,
	natsClient *natsclient.Client,
	logger *slog.Logger,
) (*metric.MetricsRegistry, types.PlatformMeta, *config.Manager, error) {
	// Create metrics registry
	metricsRegistry := metric.NewMetricsRegistry()

	// Extract platform identity
	platform := extractPlatformMeta(cfg)

	slog.Info("Platform identity configured",
		"org", platform.Org,
		"platform", platform.Platform,
		"environment", cfg.Platform.Environment)

	// Create and start config manager
	configManager, err := config.NewConfigManager(cfg, natsClient, logger)
	if err != nil {
		return nil, types.PlatformMeta{}, nil, fmt.Errorf("create config manager: %w", err)
	}

	if err := configManager.Start(ctx); err != nil {
		return nil, types.PlatformMeta{}, nil, fmt.Errorf("start config manager: %w", err)
	}

	return metricsRegistry, platform, configManager, nil
}

// createNATSClient creates a NATS client from config.
func createNATSClient(cfg *config.Config) (*natsclient.Client, error) {
	natsURLs := "nats://localhost:4222"

	// Environment variable override takes precedence
	if envURL := os.Getenv("SEMSTREAMS_NATS_URLS"); envURL != "" {
		natsURLs = envURL
	} else if len(cfg.NATS.URLs) > 0 {
		natsURLs = strings.Join(cfg.NATS.URLs, ",")
	}

	return natsclient.NewClient(natsURLs)
}

// extractPlatformMeta extracts platform identity from config.
func extractPlatformMeta(cfg *config.Config) types.PlatformMeta {
	platformID := cfg.Platform.InstanceID
	if platformID == "" {
		platformID = cfg.Platform.ID
	}

	return types.PlatformMeta{
		Org:      cfg.Platform.Org,
		Platform: platformID,
	}
}

// setupRegistriesAndManager creates registries and service manager.
// All factories come from semstreams' componentregistry.Register.
func setupRegistriesAndManager(cfg *config.Config) (*component.Registry, *service.Manager, error) {
	componentRegistry := component.NewRegistry()

	if err := componentregistry.Register(componentRegistry); err != nil {
		return nil, nil, fmt.Errorf("register framework components: %w", err)
	}

	factories := componentRegistry.ListFactories()
	slog.Info("Component factories registered", "count", len(factories))

	serviceRegistry := service.NewServiceRegistry()
	if err := service.RegisterAll(serviceRegistry); err != nil {
		return nil, nil, fmt.Errorf("register services: %w", err)
	}

	manager := service.NewServiceManager(serviceRegistry)
	ensureServiceManagerConfig(cfg)
	ensureMetricsConfig(cfg)

	return componentRegistry, manager, nil
}

// ensureServiceManagerConfig ensures service-manager config exists with defaults
func ensureServiceManagerConfig(cfg *config.Config) {
	if cfg.Services == nil {
		cfg.Services = make(types.ServiceConfigs)
	}

	if _, exists := cfg.Services["service-manager"]; !exists {
		slog.Debug("Adding default service-manager config")
		defaultConfig := map[string]any{
			"http_port":  8080,
			"swagger_ui": true,
			"server_info": map[string]string{
				"title":       "SemStreams API",
				"description": "semantic stream processing framework - protocol and semantic layers",
				"version":     Version,
			},
		}
		defaultConfigJSON, _ := json.Marshal(defaultConfig)
		cfg.Services["service-manager"] = types.ServiceConfig{
			Name:    "service-manager",
			Enabled: true,
			Config:  defaultConfigJSON,
		}
		slog.Debug("Service-manager config added", "enabled", true)
	} else {
		slog.Debug("Service-manager config already exists", "enabled", cfg.Services["service-manager"].Enabled)
	}
}

// ensureMetricsConfig ensures metrics service is always present with defaults.
// Observability should not be opt-in — metrics are critical for tuning and SLA validation.
func ensureMetricsConfig(cfg *config.Config) {
	if _, exists := cfg.Services["metrics"]; !exists {
		slog.Debug("Adding default metrics config")
		defaultConfig := map[string]any{
			"port":               9090,
			"path":               "/metrics",
			"include_go_metrics": true,
		}
		defaultConfigJSON, _ := json.Marshal(defaultConfig)
		cfg.Services["metrics"] = types.ServiceConfig{
			Name:    "metrics",
			Enabled: true,
			Config:  defaultConfigJSON,
		}
		slog.Debug("Metrics config added", "port", 9090)
	}
}

// createServiceDependencies creates the Dependencies struct for services
func createServiceDependencies(
	natsClient *natsclient.Client,
	metricsRegistry *metric.MetricsRegistry,
	logger *slog.Logger,
	platform types.PlatformMeta,
	configManager *config.Manager,
	componentRegistry *component.Registry,
) *service.Dependencies {
	return &service.Dependencies{
		NATSClient:        natsClient,
		MetricsRegistry:   metricsRegistry,
		Logger:            logger,
		Platform:          platform,
		Manager:           configManager,
		ComponentRegistry: componentRegistry,
	}
}

// configureAndCreateServices configures the manager and creates all services
func configureAndCreateServices(
	cfg *config.Config,
	manager *service.Manager,
	svcDeps *service.Dependencies,
) error {
	slog.Debug("Configuring Manager")
	if err := manager.ConfigureFromServices(cfg.Services, svcDeps); err != nil {
		return fmt.Errorf("configure service manager: %w", err)
	}

	slog.Debug("Creating services from config", "count", len(cfg.Services))
	for name, svcConfig := range cfg.Services {
		if name == "service-manager" {
			slog.Debug("Skipping service-manager (configured directly)")
			continue
		}

		if err := createServiceIfEnabled(manager, name, svcConfig, svcDeps); err != nil {
			return err
		}
	}

	return nil
}

// createServiceIfEnabled creates a service if it's enabled and registered
func createServiceIfEnabled(
	manager *service.Manager,
	name string,
	svcConfig types.ServiceConfig,
	svcDeps *service.Dependencies,
) error {
	slog.Debug("Processing service config", "key", name, "name", svcConfig.Name, "enabled", svcConfig.Enabled)

	if !svcConfig.Enabled {
		slog.Info("Service disabled in config", "name", name)
		return nil
	}

	if !manager.HasConstructor(name) {
		slog.Warn("Service configured but not registered", "key", name, "available_constructors", manager.ListConstructors())
		return nil
	}

	slog.Debug("Creating service", "name", name, "has_constructor", true)
	if _, err := manager.CreateService(name, svcConfig.Config, svcDeps); err != nil {
		return fmt.Errorf("create service %s: %w", name, err)
	}

	slog.Info("Created service", "name", name, "config_name", svcConfig.Name)
	return nil
}

// runWithSignalHandling starts services and handles shutdown signals
func runWithSignalHandling(ctx context.Context, manager *service.Manager, shutdownTimeout time.Duration) error {
	slog.Debug("Setting up signal handling")
	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	slog.Info("Starting all services")
	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	slog.Info("All services started successfully")

	<-signalCtx.Done()
	slog.Info("Received shutdown signal")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := shutdown(shutdownCtx, manager, shutdownTimeout); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	slog.Info("SemStreams shutdown complete")
	return nil
}

// shutdown performs graceful shutdown of all services
func shutdown(ctx context.Context, manager *service.Manager, timeout time.Duration) error {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	if err := manager.StopAll(timeout); err != nil {
		slog.Error("Error stopping services", "error", err)
		return err
	}

	return nil
}

// printHelp prints help information
func printHelp() {
	printDetailedHelp()
}

// loadConfig loads configuration from the specified file path
func loadConfig(path string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

// startPProfServer starts the pprof HTTP server for profiling.
// The server runs on http.DefaultServeMux which has pprof handlers
// registered via the blank import of net/http/pprof.
func startPProfServer(port int) {
	addr := fmt.Sprintf(":%d", port)
	// Use a simple logger that works before slog is configured
	fmt.Printf("Starting pprof server on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed {
		fmt.Printf("pprof server error: %v\n", err)
	}
}
