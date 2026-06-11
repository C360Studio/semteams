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

	"github.com/c360studio/semstreams/agentic/agentrun"
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
	"github.com/c360studio/semstreams/pkg/lifecycle"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/processor/agentic-tools/executors"
	rulepkg "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/chainpause"
	"github.com/c360studio/semteams/cmd/semteams/flowtemplates"
	"github.com/c360studio/semteams/cmd/semteams/portresolver"
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

	// 9b. Load the persona fragment corpus from disk into the PERSONAS
	// KV bucket. The test_harness catalog + rendered researcher
	// fragment (ADR-033 R3.7.1) retired alongside the dev-via-spec
	// arc in the ADR-042 MVP-7 follow-up sweep, so persona load is
	// now a single-step.
	personaMgr := loadPersonaFragments(ctx, natsClient, cliCfg.PersonaFragmentsPath)

	// 9b-bis. Seed the FLOW_TEMPLATES KV bucket from configs/flow-templates/.
	// ADR-042 Phase 1. Must run before setupToolsAndPreprocessor so the
	// list_flow_templates tool sees the inventory on first call. Manager
	// is threaded through to RegisterBuiltins via the deps struct.
	flowTemplateMgr := loadFlowTemplates(ctx, natsClient, cliCfg.FlowTemplatesPath, slog.Default())

	// 9c–9f. Build + register tool executors, start the evidence preprocessor
	// (ADR-036 §Phase 2), and start the chain-pause subscriber (ADR-037 v1).
	// Extracted to keep run() under revive's function-length threshold while
	// keeping the ordering invariant: tools before svcDeps, preprocessors after.
	toolRegistry, chainPauseHTTP, err := setupToolsAndPreprocessor(ctx, cfg, natsClient, platform, configManager, componentRegistry, personaMgr, flowTemplateMgr, metricsRegistry, slog.Default())
	if err != nil {
		return err
	}

	// 10. Create service dependencies, plumbing the shared registries so
	// every component constructed by the service manager sees the same
	// tool + payload registry instances.
	svcDeps := createServiceDependencies(natsClient, metricsRegistry, logger, platform, configManager, componentRegistry)
	svcDeps.ToolRegistry = toolRegistry
	svcDeps.PayloadRegistry = payloadReg

	// 10a. Wire the agent-run substrate: the shared Lifecycle harness Manager +
	// agent-run workflow (ADR-053 Phase 1) AND the MilestoneSubscriber (Phase 4a
	// D3 terminal authority). Must run before configureAndCreateServices so the
	// rule processor factory installs the manager via SetLifecycleManager.
	if err := wireAgentRunSubstrate(ctx, svcDeps, natsClient, platform, logger); err != nil {
		return err
	}

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
	manager.UseHTTPMiddleware(productMiddleware(chainPauseHTTP, slog.Default())...)

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
	metricsRegistry *metric.MetricsRegistry,
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
		NATSClient:              natsClient,
		Platform:                platform,
		Logger:                  logger,
		RuleManager:             buildRuleManager(ctx, natsClient, configManager, logger),
		FlowManager:             flowMgr,
		PersonaManager:          personaMgr,
		FlowTemplateManager:     flowTemplateMgr,
		FlowEngineManager:       flowEngine,
		ComponentRegistry:       componentRegistry,
		LoopsBucket:             extractLoopsBucket(cfg),
		RestrictedDecideActions: extractRestrictedDecideActions(cfg, logger),
		SkipBuiltins:            []string{"bash"},
	}); err != nil {
		return nil, nil, fmt.Errorf("register builtin tools: %w", err)
	}

	// 9d. Product-shell-local tool executors (add_source_repo,
	// emit_research_artifact, emit_plan, chain-scoped bash wrapper).
	// Each tool's registration comment lives in registerProductTools;
	// see ADR-029 + tools/README.md for discipline. The dev-via-spec-arc
	// executors (emit_dev_via_spec_artifact, builder_decide,
	// bootstrap_workspace) retired in the ADR-042 MVP-7 follow-up sweep.
	if err := registerProductTools(toolRegistry, natsClient, platform, logger); err != nil {
		return nil, nil, fmt.Errorf("register product tools: %w", err)
	}

	// 9e. Chain-pause subscriber + HTTP handler (ADR-037 v1).
	// Subscribes to agent.failed.> and stamps §D5 audit triples when a
	// managed-arc loop fails. Returns the HTTP handler for POST
	// /teams-loop/chain-pause/decide — mounted in productMiddleware.
	chainPauseHTTP, err := startChainPauseSubscriber(ctx, cfg, natsClient, platform, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("start chain-pause subscriber: %w", err)
	}

	// 9g. (ADR-053) The hand-rolled chain milestone stampers were RETIRED
	// here — they wrote chain.* projections onto the canonical
	// agent.chain.execution.<id> entity, which is now owned exclusively by
	// the agent-run lifecycle substrate (run_scope=new mint + the agent-run
	// transition rules). The dual-write was the adoption-plan hedge; keeping
	// it meant the DispatchedStamper raced the mint for the same entity and
	// the lifecycle-attach CAS lost, so no run was ever minted. Removing the
	// stampers makes agent-run the sole writer of the run entity. The
	// resolver/lineage helpers survive for chainbash + the sandbox tools
	// (migrated to typed RunID in the follow-up slice).
	return toolRegistry, chainPauseHTTP, nil
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

// extractRestrictedDecideActions reads the deployment-level decide-action
// restriction policy (semstreams#239, beta.104) from the agentic-tools
// component config. The decide tool bars these action names for EVERY
// coordinator task — front-door and rule-spawned alike — composing with and
// taking precedence over the per-task action_allowlist. This is the
// run-level CLARIFICATION POLICY seam (ADR-053 Phase 4b): empty = interactive
// (ask_user available, the default); ["ask_user"] = autonomous (the
// coordinator must resolve without deferring to the user). The framework owns
// the generic restrict-list; this product maps its run mode onto it.
//
// Independent reimplementation of upstream cmd/semstreams/main.go per ADR-029
// — not an import. A malformed policy is logged (not silently dropped) so an
// operator-requested gate never disappears unnoticed.
func extractRestrictedDecideActions(cfg *config.Config, logger *slog.Logger) []string {
	for _, cc := range cfg.Components {
		if cc.Name != "agentic-tools" || !cc.Enabled {
			continue
		}
		var tcfg struct {
			RestrictedDecideActions []string `json:"restricted_decide_actions"`
		}
		if err := json.Unmarshal(cc.Config, &tcfg); err != nil {
			logger.Warn("agentic-tools config unparseable for restricted_decide_actions — clarification policy NOT applied",
				"error", err)
			continue
		}
		if len(tcfg.RestrictedDecideActions) > 0 {
			return tcfg.RestrictedDecideActions
		}
	}
	return nil
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

// wireAgentRunSubstrate builds the shared Lifecycle harness Manager (ADR-047),
// registers the agent-run workflow (ADR-053 D2), plumbs the manager onto
// svcDeps.LifecycleManager so the rule processor factory installs it (its Setup
// calls SetLifecycleManager when deps.LifecycleManager is non-nil), and wires the
// MilestoneSubscriber. With the manager wired, lifecycle_* rule actions and the
// run-entity substitution ($entity.triple.agent.run.entity_id) resolve at
// evaluation time instead of failing closed.
//
// ADR-053 adoption history: Phase 1 made the manager available; Phase 2 added
// run_scope="new" at the 3 coordinator root spawns (minting an AgentRun per
// chain, sitting inert in "dispatched"); Phase 4a (this revision) adds the
// MilestoneSubscriber + the agent-run transition rules together — the subscriber
// is co-wired with the rules because without dispatched→executing advancing a
// healthy run past "dispatched", D3 would spuriously fail every run on the
// coordinator's normal terminal. The existing lineage threading
// (cmd/semteams/chain) stays untouched (dual-write) until Phase 5. Mirrors
// upstream cmd/semstreams/main.go §10b–10d boot order (ADR-029).
//
// Upstream inlines these statements directly against svcDeps; the helper here is
// purely to keep run() under revive's function-length limit, NOT a semantic
// divergence — a reader diffing against upstream main.go should treat it as the
// same wiring.
func wireAgentRunSubstrate(ctx context.Context, svcDeps *service.Dependencies, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	mgr := lifecycle.NewManager(natsClient, logger)
	if err := agentrun.Register(mgr); err != nil {
		return fmt.Errorf("register agent-run workflow: %w", err)
	}
	svcDeps.LifecycleManager = mgr
	// ADR-053 Phase 4a: wire the MilestoneSubscriber (D3 terminal authority)
	// with the manager, co-landed with the agent-run transition rules. Phase 1
	// deliberately deferred it — without the dispatched→executing rule advancing
	// a healthy run past "dispatched", D3 would spuriously fail every run on the
	// coordinator's normal terminal.
	return startAgentRunMilestoneSubscriber(ctx, mgr, natsClient, platform, logger)
}

// startAgentRunMilestoneSubscriber wires the ADR-053 D3 terminal-authority
// subscriber (Phase 4a). It subscribes to terminal loop events
// (agent.complete.* / agent.failed.*) on durable JetStream consumers, resolves
// the run, and applies the narrow zombie-prevention fallback: a dispatch-ROOT
// loop that terminates while its run is still "dispatched" (no confirmed
// handoff) transitions the run to failed/cancelled. All other terminal
// transitions are product rules (configs/rules/agent-run/) — the framework
// only observes; the coordinator/rules decide (ADR-053 §D3).
//
// Wired together with the agent-run transition rules (Phase 4a), NOT in
// Phase 2: without the dispatched→executing rule advancing a healthy run past
// "dispatched", this subscriber's D3 guard would fire on every coordinator's
// normal terminal. No product MilestoneHandlers yet — Phase 5 re-platforms the
// chain stampers as handlers via AddHandler. Mirrors upstream
// cmd/semstreams/main.go §10c boot order (ADR-029).
func startAgentRunMilestoneSubscriber(ctx context.Context, mgr *lifecycle.Manager, natsClient *natsclient.Client, platform types.PlatformMeta, logger *slog.Logger) error {
	reader := agentrun.NewNATSLoopTripleReader(natsClient)
	pub := agentrun.NewNATSTriplePublisher(natsClient)
	sub := agentrun.NewMilestoneSubscriber(mgr, reader, pub, platform.Org, platform.Platform, logger)
	// StreamName is the JetStream stream holding agent.complete.*/agent.failed.*
	// (the agentic-loop default "AGENT"). The filter subjects are owned by
	// Start, not configurable product-side — an operator who rewires teams-loop's
	// publish subjects must also patch this stream/filter pairing upstream.
	if _, err := sub.Start(ctx, natsClient, agentrun.StartConfig{StreamName: agentrun.AgentStreamName}); err != nil {
		return fmt.Errorf("agent-run milestone subscriber start: %w", err)
	}
	logger.Info("Started agent-run milestone subscriber (ADR-053 Phase 4a — D3 terminal authority)",
		slog.String("stream", agentrun.AgentStreamName))
	return nil
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
