package sandboxmanager

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
)

// Runner is the narrow seam between Manager and the realization
// layer (@devcontainers/cli shell-out). Production satisfies it
// with a CLIRunner; unit tests inject a fake to drive the manager
// orchestration without docker.
//
// Up brings up a devcontainer for the given workspace folder + config
// path and returns a ContainerRef the manager will pass back to Exec
// for preflight probes, plus the image digest (sha256:...) the
// attestation stamps. workspaceFolder is the manager-derived path
// inside the host's tenant root (e.g.
// /var/lib/semteams-tenants/<signature>/workspace); the runner shells
// out to `devcontainer up --workspace-folder <wsf> --config <configPath>`.
//
// Exec runs a probe command inside the materialized container and
// returns its result. The runner does NOT enforce ExpectExit — it
// returns the actual exit code; the manager (Attest) decides
// pass/fail. This keeps the runner stateless about probe semantics.
type Runner interface {
	Up(ctx context.Context, workspaceFolder, configPath string, env map[string]string) (ContainerRef, error)
	Exec(ctx context.Context, ref ContainerRef, command string) (ProbeResult, error)
}

// ContainerRef captures the runner's per-container identity. Opaque
// to the manager except for ImageDigest (stamped on attestation).
// The shape mirrors `devcontainer up`'s JSON output (containerId +
// remoteWorkspaceFolder + image), but we keep the struct narrow so
// alternate runners (DevPod, raw docker) can satisfy it.
type ContainerRef struct {
	ContainerID           string
	ImageDigest           string
	RemoteWorkspaceFolder string
	Properties            map[string]string
}

// Manager orchestrates the three-layer sandbox flow. Holds the
// profile catalog, the operator AdmissionPolicy, the Runner, and
// the TriplePublisher for stamping attestations. All fields are
// boot-required (no graceful-degradation mode); New panics if any
// is nil.
//
// One Manager per backend process. Concurrency-safe given a
// concurrency-safe Runner + TriplePublisher (both the production
// CLIRunner and natsTriplePublisher are).
type Manager struct {
	catalog      *Catalog
	policy       AdmissionPolicy
	runner       Runner
	publisher    agentictools.TriplePublisher
	tenantRoot   string
	probeTimeout time.Duration
	upTimeout    time.Duration
	now          func() time.Time
	logger       *slog.Logger
}

// ManagerConfig is the New() input. Required fields panic on zero
// value; optional fields default.
type ManagerConfig struct {
	// Catalog is the registered profile set. Required.
	Catalog *Catalog

	// Policy is the operator-level admission policy. Zero value is
	// default-deny across the privileged + public-network surfaces.
	Policy AdmissionPolicy

	// Runner shells out to @devcontainers/cli (production) or fakes
	// it (tests). Required.
	Runner Runner

	// Publisher writes attestation triples on the calling chain
	// entity. Required.
	Publisher agentictools.TriplePublisher

	// TenantRoot is the host filesystem path under which the manager
	// creates per-signature workspace folders. Defaults to
	// "/var/lib/semteams-tenants" (the production-canonical layout
	// matching /var/lib/docker). PR 4.5 F6 binds this path
	// symmetrically inside the sandbox container so devcontainer-cli's
	// DooD-spawned siblings resolve the same path on the host daemon.
	TenantRoot string

	// ProbeTimeout caps each preflight probe. Zero defaults to 30s.
	ProbeTimeout time.Duration

	// UpTimeout caps `devcontainer up`. Zero defaults to 5min
	// (devcontainer image pulls can be slow).
	UpTimeout time.Duration

	// Now is the clock injection seam for tests. Zero defaults to
	// time.Now (UTC).
	Now func() time.Time

	// Logger is the structured logger for manager-internal events.
	// Zero defaults to slog.Default.
	Logger *slog.Logger
}

// New constructs a Manager. Catalog, Runner, and Publisher are
// required; other fields default. Panics on missing required deps
// — this is boot-required wiring.
func New(cfg ManagerConfig) *Manager {
	if cfg.Catalog == nil {
		panic("sandboxmanager.New: Catalog must not be nil")
	}
	if cfg.Runner == nil {
		panic("sandboxmanager.New: Runner must not be nil")
	}
	if cfg.Publisher == nil {
		panic("sandboxmanager.New: Publisher must not be nil")
	}
	m := &Manager{
		catalog:      cfg.Catalog,
		policy:       cfg.Policy,
		runner:       cfg.Runner,
		publisher:    cfg.Publisher,
		tenantRoot:   cfg.TenantRoot,
		probeTimeout: cfg.ProbeTimeout,
		upTimeout:    cfg.UpTimeout,
		now:          cfg.Now,
		logger:       cfg.Logger,
	}
	if m.tenantRoot == "" {
		m.tenantRoot = "/var/lib/semteams-tenants"
	}
	if m.probeTimeout <= 0 {
		m.probeTimeout = 30 * time.Second
	}
	if m.upTimeout <= 0 {
		m.upTimeout = 5 * time.Minute
	}
	if m.now == nil {
		m.now = func() time.Time { return time.Now().UTC() }
	}
	if m.logger == nil {
		m.logger = slog.Default()
	}
	return m
}

// Request is the Layer 2 entry point. Orchestrates:
//
//  1. Validate the SandboxRequirements (caller should have, but we
//     enforce defensively — a malformed req at the manager seam is a
//     terminal Attestation, not an error).
//  2. MatchProfile against the catalog. No match → terminal.
//  3. AdmissionCheck. Denied → terminal. Pending → return early
//     (rule pack spawns admission-reviewer).
//  4. Runner.Up to materialize the container.
//  5. Runner.Exec to run each VerifyProbe in order.
//  6. Attest into a final Attestation.
//  7. Stamp attestation triples on chainEntityID via the publisher.
//  8. Return the Attestation.
//
// Error is returned only for publisher failures (the request itself
// completed; we just couldn't tell the graph about it). All
// requirements / match / admission / runner failures are encoded in
// the returned Attestation, NOT as errors — that's the
// Coordinator-facing contract: route on attestation shape, never on
// Go-error type.
//
// chainEntityID is the calling chain's 6-part entity id (where the
// attestation triples land). Empty string is allowed (e.g. unit
// tests); Triples() returns nil and we skip the publish step.
func (m *Manager) Request(ctx context.Context, chainEntityID string, req SandboxRequirements) (Attestation, error) {
	if err := req.Validate(); err != nil {
		att := AttestTerminalError(Profile{Name: "unmatched", Version: "v0"}, req, "requirements validation failed: "+err.Error(), m.now())
		return att, m.stamp(ctx, chainEntityID, att)
	}
	// Normalize after Validate so every code path below sees
	// post-normalize shape (lowercased probe names, trimmed
	// commands, sorted slices). Without this, runProbes would
	// execute the raw Command string from the caller — including any
	// leading/trailing whitespace — and Verified map keys could
	// drift from the form Coordinator substitutes. Per PR 4.1
	// finding C3.
	req = req.Normalize()

	profile, err := MatchProfile(req, m.catalog)
	if err != nil {
		att := AttestTerminalError(Profile{Name: "unmatched", Version: "v0"}, req, "no profile match: "+err.Error(), m.now())
		return att, m.stamp(ctx, chainEntityID, att)
	}

	admission := AdmissionCheck(req, profile, m.policy)
	if admission.Outcome != AdmissionAdmitted {
		att := AttestAdmissionFailure(profile, req, admission, m.now())
		m.logger.Info("sandbox admission gated",
			slog.String("profile", profile.ID()),
			slog.String("outcome", string(admission.Outcome)),
			slog.Any("reasons", admission.Reasons()))
		return att, m.stamp(ctx, chainEntityID, att)
	}

	workspaceFolder := m.workspaceFor(req, profile)
	upCtx, cancel := context.WithTimeout(ctx, m.upTimeout)
	ref, err := m.runner.Up(upCtx, workspaceFolder, profile.DevcontainerPath, m.secretEnv(req))
	cancel()
	if err != nil {
		att := AttestTerminalError(profile, req, "devcontainer up failed: "+err.Error(), m.now())
		m.logger.Error("sandbox devcontainer up failed",
			slog.String("profile", profile.ID()),
			slog.String("workspace_folder", workspaceFolder),
			slog.Any("error", err))
		return att, m.stamp(ctx, chainEntityID, att)
	}

	probes := m.runProbes(ctx, ref, req.Verification)
	att := Attest(profile, req, ref.ImageDigest, probes, m.now())
	m.logger.Info("sandbox attestation rendered",
		slog.String("profile", profile.ID()),
		slog.String("signature", att.Signature),
		slog.Bool("ready", att.Ready),
		slog.Bool("degraded", att.Degraded),
		slog.Int("failed_probes", len(att.FailedProbes)))
	return att, m.stamp(ctx, chainEntityID, att)
}

// runProbes executes each VerifyProbe in order against the
// materialized container. Each probe runs with its own probe-timeout
// context. Probes that exceed the timeout surface as ProbeResult
// with Err set; Attest treats them as failures.
//
// Probe order matches req.Verification order (post-Normalize, that's
// sorted by Name). Stable.
//
// ExpectExit is copied through onto the result so ProbeResult.OK can
// compare authoritatively. Runner-returned Stdout/Stderr is
// preserved even on the error path — a future Runner may populate
// both fields with an *exec.ExitError-style outcome (PR 4.1 finding
// M1+M2).
func (m *Manager) runProbes(ctx context.Context, ref ContainerRef, probes []VerifyProbe) []ProbeResult {
	out := make([]ProbeResult, 0, len(probes))
	for _, p := range probes {
		probeCtx, cancel := context.WithTimeout(ctx, m.probeTimeout)
		res, err := m.runner.Exec(probeCtx, ref, p.Command)
		cancel()
		res.Name = p.Name
		res.Command = p.Command
		res.ExpectExit = p.ExpectExit
		if err != nil && res.Err == nil {
			res.Err = err
		}
		out = append(out, res)
	}
	return out
}

// workspaceFor returns the host-side path for the tenant's writable
// workspace. Stable for a given (profile, requirements) pair so
// re-requests against the same logical work reuse the same on-disk
// state.
func (m *Manager) workspaceFor(req SandboxRequirements, profile Profile) string {
	sig := SignatureFor(profile, req)
	// Use a 16-char prefix to keep the path short; 16 hex chars is
	// 64 bits — collision probability negligible at any realistic
	// tenant scale.
	prefix := sig
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	return strings.TrimRight(m.tenantRoot, "/") + "/" + prefix + "/workspace"
}

// secretEnv pre-builds the env map the Runner.Up call passes to
// `devcontainer up`. The Runner is expected to translate this into
// devcontainer-native secret wiring (containerEnv / remoteEnv /
// `--secrets-file` etc.). For PR 4.1 we surface the names only; the
// PR 4.2 CLIRunner will resolve them against the operator's
// pre-provisioned env at exec time.
func (m *Manager) secretEnv(req SandboxRequirements) map[string]string {
	if len(req.Secrets) == 0 {
		return nil
	}
	out := make(map[string]string, len(req.Secrets))
	for _, s := range req.Secrets {
		// Mark as "pass through from host env" — the Runner reads
		// os.Getenv at exec time. Empty value sentinel keeps the
		// map shape consistent without leaking actual secret data
		// through the manager-layer logs.
		out[s] = ""
	}
	return out
}

// stamp publishes the Attestation's triples on chainEntityID. No-op
// when chainEntityID is empty (unit-test path). Publisher errors are
// returned so the caller knows the graph view diverged from the
// returned Attestation.
func (m *Manager) stamp(ctx context.Context, chainEntityID string, att Attestation) error {
	if chainEntityID == "" {
		return nil
	}
	triples := att.Triples(chainEntityID)
	if len(triples) == 0 {
		return nil
	}
	if err := m.publisher.AddTriplesBatch(ctx, triples); err != nil {
		m.logger.Error("sandbox attestation stamp failed",
			slog.String("chain_entity", chainEntityID),
			slog.Any("error", err))
		return fmt.Errorf("stamp attestation on %s: %w", chainEntityID, err)
	}
	return nil
}

// Catalog exposes the manager's profile catalog so query_sandbox_*
// tools (PR 4.2) can list profiles without dependency-injecting the
// catalog separately. Returns nil when the manager is nil-receiver
// (defensive).
func (m *Manager) Catalog() *Catalog {
	if m == nil {
		return nil
	}
	return m.catalog
}

// Policy returns a copy of the admission policy. Used by PR 4.2's
// admission-introspection tooling.
func (m *Manager) Policy() AdmissionPolicy {
	if m == nil {
		return AdmissionPolicy{}
	}
	return m.policy
}

// TenantRoot returns the resolved tenant-root path (env-supplied or
// default). Used by product_tools for boot-log visibility so operators
// can confirm the symmetric-bind path actually took effect.
func (m *Manager) TenantRoot() string {
	if m == nil {
		return ""
	}
	return m.tenantRoot
}
