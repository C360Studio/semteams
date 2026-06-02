package sandboxruntime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semstreams/processor/agentic-tools/runner"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

// EntityTripleReader is the narrow read surface AttestationRunner
// uses to look up sandbox attestation triples on the chain entity.
// chain.NATSEntityReader satisfies it in production; tests inject
// fakes. Consumer-defines-interface — same pattern as the existing
// querysandboxattestation.EntityTripleReader.
type EntityTripleReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// DefaultRunner is the upstream-runner contract AttestationRunner
// delegates to. *runner.Client (the only production implementation
// today) satisfies it; tests inject fakes. Mirrors the upstream
// Runner interface that semstreams' BashExecutor will accept via
// WithRunner once the next beta tags — Phase D wires the
// AttestationRunner into BashExecutor at boot.
type DefaultRunner interface {
	Exec(ctx context.Context, taskID, command string, timeoutMs int) (*runner.ExecResult, error)
}

// AttestationRunner wraps a DefaultRunner with attestation-aware
// routing. When the calling chain has an attested per-tenant
// devcontainer (sandbox.attestation.ready=true +
// sandbox.attestation.host_workspace_folder present on the chain
// entity), Exec wraps the command in `devcontainer exec
// --workspace-folder <wsf> bash -lc <cmd>` so it runs INSIDE the
// per-tenant container. When no attestation is present, Exec
// passthrough-delegates to the default runner unchanged.
//
// This sits between chainbash (which rewrites task_id → chain_id) and
// upstream's BashExecutor. Phase D injects an instance via
// `executors.WithRunner(...)` once upstream's Runner interface tags.
type AttestationRunner struct {
	defaultRunner DefaultRunner
	entities      EntityTripleReader
	platform      types.PlatformMeta
	logger        *slog.Logger
}

// NewAttestationRunner constructs a runner. defaultRunner + entities
// are boot-required (no graceful-degrade — without both, this whole
// package is dead weight; better to surface the config gap at boot).
// platform.Org + platform.Platform must be set so the chain entity
// ID can be constructed. logger nil defaults to slog.Default.
func NewAttestationRunner(defaultRunner DefaultRunner, entities EntityTripleReader, platform types.PlatformMeta, logger *slog.Logger) *AttestationRunner {
	if defaultRunner == nil {
		panic("sandboxruntime.NewAttestationRunner: defaultRunner must not be nil")
	}
	if entities == nil {
		panic("sandboxruntime.NewAttestationRunner: entities reader must not be nil")
	}
	if platform.Org == "" || platform.Platform == "" {
		panic("sandboxruntime.NewAttestationRunner: platform.Org and platform.Platform must be set for chain entity ID construction")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AttestationRunner{
		defaultRunner: defaultRunner,
		entities:      entities,
		platform:      platform,
		logger:        logger,
	}
}

// Exec implements the upstream Runner contract. taskID is the chain
// identity rewritten by chainbash (Metadata["task_id"] = chain_id) —
// we use it to construct the chain entity ID for the attestation
// lookup. timeoutMs is passed through to the default runner.
func (r *AttestationRunner) Exec(ctx context.Context, taskID, command string, timeoutMs int) (*runner.ExecResult, error) {
	wsf := r.resolveWorkspaceFolder(ctx, taskID)
	if wsf == "" {
		return r.defaultRunner.Exec(ctx, taskID, command, timeoutMs)
	}
	wrapped := wrapDevcontainerExec(wsf, command)
	r.logger.Debug("sandboxruntime: routing command into per-tenant devcontainer",
		slog.String("task_id", taskID),
		slog.String("host_workspace_folder", wsf))
	return r.defaultRunner.Exec(ctx, taskID, wrapped, timeoutMs)
}

// resolveWorkspaceFolder reads the chain entity for attestation
// triples and returns the host workspace folder when ready+present.
// Returns "" on any condition that should route via passthrough:
//
//   - taskID empty (defensive; chainbash sets it but bash calls outside
//     a chain context may not)
//   - chain entity not found / read error (chain-root calls where
//     task_id == loop_id, or pre-stamped state)
//   - sandbox.attestation.ready != true (attestation exists but
//     degraded/admission-denied/terminal)
//   - sandbox.attestation.host_workspace_folder empty/missing (Phase A
//     omit-when-empty discipline — admission failed before Up ran)
//
// All of these resolve identically: passthrough. The runner doesn't
// distinguish between them because they all mean "no per-tenant
// container to route into" from the caller's perspective.
//
// Log levels are deliberate:
//   - ctx-cancelled / deadline-exceeded → no log (the caller is going
//     to surface the same ctx error from the default runner; logging
//     it here would just duplicate the noise).
//   - other read errors (NATS down, malformed entity ID) → Warn. These
//     are signal, not noise: an operator investigating "why did my
//     per-tenant sandbox not get used?" should see this at default log
//     level without cranking to Debug.
func (r *AttestationRunner) resolveWorkspaceFolder(ctx context.Context, taskID string) string {
	if taskID == "" {
		return ""
	}
	entityID := fmt.Sprintf("%s.%s.agent.chain.execution.%s", r.platform.Org, r.platform.Platform, taskID)
	triples, err := r.entities.ReadEntity(ctx, entityID)
	if err != nil {
		// Don't double-log a cancelled context — the default-runner
		// delegation will surface the same ctx.Err() and the caller
		// (upstream BashExecutor) attaches it to the tool result.
		// Per reviewer Rec: distinguishing ctx-cancelled from genuine
		// read failure also prevents misclassifying a routing decision
		// (would have been: per-tenant) as if it were a passthrough
		// decision (always-warm sandbox).
		if ctx.Err() == nil {
			r.logger.Warn("sandboxruntime: chain entity read failed; passthrough to default runner",
				slog.String("entity_id", entityID),
				slog.Any("error", err))
		}
		return ""
	}
	if triples == nil {
		return ""
	}

	// ready must be true to route — degraded or terminal attestations
	// have a container but no guarantee the workload's capabilities
	// are present. Phase A keeps the ready triple as a strict gate.
	// The bool cast fails-closed if a future drift stamps ready as a
	// string ("true") instead of a native bool — wrong-type → false →
	// passthrough, which is the safe default.
	ready, _ := triples[sandboxmanager.PredicateAttestationReady].(bool)
	if !ready {
		return ""
	}
	wsf, _ := triples[sandboxmanager.PredicateAttestationHostWorkspaceFolder].(string)
	return wsf
}

// wrapDevcontainerExec wraps a user command in a devcontainer-exec
// invocation that runs it inside the per-tenant container mounted at
// wsf. The output runs through bash -c <wrapped> on the always-warm
// sandbox's /exec endpoint:
//
//	bash -c "devcontainer exec --workspace-folder '<wsf>' bash -lc '<cmd>'"
//
// wsf and cmd are single-quote-escaped so user-controlled content
// (file paths with spaces, commands with shell metacharacters)
// round-trips without injection. This IS the security boundary
// between LLM-generated bash content and the per-tenant devcontainer
// shell — mirrors the same discipline as
// sandboxmanager/runner_api.go's joinShell helper.
func wrapDevcontainerExec(wsf, cmd string) string {
	// Quote every token uniformly — matches
	// sandboxmanager/runner_api.go's joinShell pattern. Even though
	// the static tokens (`devcontainer`, `exec`, `--workspace-folder`,
	// `bash`, `-lc`) carry no shell metacharacters today, the uniform
	// quoting keeps the security-boundary discipline obvious to
	// readers: every token in this string IS user-untrusted unless
	// proven otherwise. A future refactor that splices in dynamic
	// content (e.g., a per-tenant `--remote-env KEY=VAL` flag) would
	// silently break if we'd only quoted the "interesting" tokens.
	tokens := []string{
		"devcontainer", "exec",
		"--workspace-folder", wsf,
		"bash", "-lc", cmd,
	}
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = shellQuote(t)
	}
	return strings.Join(quoted, " ")
}

// shellQuote single-quote-escapes a string for safe inclusion in a
// bash command line. Single quotes inside the input become the
// canonical 'closing-quote backslash-quote opening-quote' pattern —
// `it's` → `'it'\”s'`. The result is always wrapped in outer single
// quotes regardless of input; even an empty string round-trips as `”`
// (valid bash for "empty positional argument," which is exactly what
// we want for an unconfigured wsf — though that's blocked upstream by
// the "" → passthrough check).
//
// Duplicated from sandboxmanager.joinShell's per-token escape. Kept
// local rather than exported across packages because the escape rule
// is small + the shared utility would couple sandboxmanager to
// sandboxruntime gratuitously.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
