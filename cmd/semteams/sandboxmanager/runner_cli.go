package sandboxmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// NoopRunner is the default Runner the product shell wires when
// the operator has not opted into a production runner via
// SEMTEAMS_SANDBOX_RUNNER. It returns a clear error from Up so the
// Manager produces a Terminal attestation explaining the situation
// — the Coordinator then routes to respond_direct surfacing the
// gap to the user. Better than silently 500-ing on first call.
//
// Production deployments swap to CLIRunner (direct shell-out from
// backend; requires Docker-socket access) or a future
// SandboxAPIRunner (posts to the sandbox container's HTTP API).
type NoopRunner struct{}

// Up returns a documented "no production runner configured" error.
func (NoopRunner) Up(_ context.Context, _, _ string, _ map[string]string) (ContainerRef, error) {
	return ContainerRef{}, fmt.Errorf("sandboxmanager: no production runner configured (set SEMTEAMS_SANDBOX_RUNNER=cli once the backend container has docker-socket access, or wire a sandbox-API-backed runner)")
}

// Exec mirrors Up — same documented error.
func (NoopRunner) Exec(_ context.Context, _ ContainerRef, _ string) (ProbeResult, error) {
	return ProbeResult{}, fmt.Errorf("sandboxmanager: no production runner configured")
}

// CLIRunner is the production Runner that shells out to the
// @devcontainers/cli reference implementation. ADR-043 §Realization
// layer commits to "shell-out, not library import" so our
// dependency posture stays "pinned external binary, not Go-module
// import" — the CLI version pins live in docker/Dockerfile
// (operator-visible), not in go.sum.
//
// The runner is stateless across calls: each Up / Exec invocation
// shells out fresh. Safe to call concurrently — @devcontainers/cli
// is itself concurrency-safe per workspace folder, and we use
// distinct workspace folders per signature.
//
// Production deployment requirement: the process running CLIRunner
// must have @devcontainers/cli on PATH AND Docker daemon access
// (typically /var/run/docker.sock). The semteams backend container
// does NOT mount the docker socket today; the production wiring
// either (a) extends docker/Dockerfile to mount the socket, or (b)
// routes through a future SandboxAPI-backed Runner that posts to
// the sandbox container (which DOES have the socket). PR 4.4
// makes the choice empirically.
type CLIRunner struct {
	// Binary is the path to the devcontainer executable. Empty
	// defaults to "devcontainer" (resolved against $PATH).
	Binary string

	// DockerBinary is the path to the docker executable. Empty
	// defaults to "docker". Used only to resolve image digests
	// (devcontainer up doesn't surface the digest directly).
	DockerBinary string

	// Logger is the structured logger for runner-level events.
	// Zero defaults to slog.Default.
	Logger *slog.Logger
}

// NewCLIRunner constructs a CLIRunner with default binary paths +
// the package's default logger.
func NewCLIRunner() *CLIRunner {
	return &CLIRunner{
		Binary:       "devcontainer",
		DockerBinary: "docker",
		Logger:       slog.Default(),
	}
}

// devcontainerUpResult mirrors the JSON shape `devcontainer up`
// returns. Only the fields the manager consumes are declared —
// extra fields are tolerated. Schema reference:
// https://containers.dev/implementors/reference/ (output of
// `devcontainer up`).
type devcontainerUpResult struct {
	Outcome               string `json:"outcome"`
	ContainerID           string `json:"containerId"`
	RemoteWorkspaceFolder string `json:"remoteWorkspaceFolder"`
	Message               string `json:"message,omitempty"`
}

// Up materializes the devcontainer. Runs `devcontainer up
// --workspace-folder <wsf> --config <path>` (plus log-format json
// for structured parsing + the non-blocking-command bypass to keep
// up time bounded) and parses the JSON stdout. env is exposed via
// `--remote-env <K>=<V>` flags per key with non-empty value; empty
// values are passed-through from the host environment (the operator
// pre-provisioned them).
//
// Host-env passthrough enforcement: when v == "" the runner reads
// os.LookupEnv(k); if the host env var is unset, Up returns an
// error rather than bringing up a container with a silently-empty
// secret. Per PR 4.1 finding C1 — `Ready=true` on an empty secret
// would silently route work to an environment where probes pass
// but the actual workload (which uses the secret at runtime) 401s.
//
// Returns a ContainerRef whose ImageDigest is best-effort: we shell
// out to `docker inspect` afterward to resolve the digest. A failed
// inspect leaves ImageDigest empty rather than failing Up — image
// digest is operational metadata, not a correctness gate.
func (r *CLIRunner) Up(ctx context.Context, workspaceFolder, configPath string, env map[string]string) (ContainerRef, error) {
	wsf := strings.TrimRight(strings.TrimSpace(workspaceFolder), "/")
	if wsf == "" {
		return ContainerRef{}, errors.New("CLIRunner.Up: empty workspaceFolder")
	}
	if !strings.HasPrefix(wsf, "/") {
		return ContainerRef{}, fmt.Errorf("CLIRunner.Up: workspaceFolder must be absolute, got %q", workspaceFolder)
	}

	// Resolve host-secret passthrough BEFORE staging so a missing
	// secret fails fast without leaving behind a half-staged
	// .devcontainer directory. Mirrors SandboxAPIRunner's ordering.
	remoteEnvArgs := make([]string, 0, len(env)*2)
	for k, v := range env {
		if v == "" {
			// Empty value means "pass-through from host env" per the
			// devcontainer CLI convention. Verify the host actually
			// has the secret set; without this the container comes
			// up with k="" silently and probes can pass against an
			// empty secret.
			hostVal, ok := os.LookupEnv(k)
			if !ok {
				return ContainerRef{}, fmt.Errorf("devcontainer up: secret %q declared but not present in host environment (operator AvailableSecrets policy promised it; export %s before invoking)", k, k)
			}
			remoteEnvArgs = append(remoteEnvArgs, "--remote-env", k+"="+hostVal)
			continue
		}
		remoteEnvArgs = append(remoteEnvArgs, "--remote-env", k+"="+v)
	}

	// PR 4.5 review M2: stage the catalog profile into the tenant's
	// writable workspace so devcontainer-cli's sibling lockfile write
	// lands in writable space — same fix SandboxAPIRunner applies via
	// /exec mkdir+cp. Without this, CLIRunner against a :ro-mounted
	// catalog hits the same EROFS class the API runner does.
	stagedConfig, err := stageDevcontainerProfile(wsf, configPath)
	if err != nil {
		return ContainerRef{}, fmt.Errorf("stage devcontainer profile into %s: %w", wsf, err)
	}
	args := append([]string{
		"up",
		"--workspace-folder", wsf,
		"--config", stagedConfig,
		"--log-format", "json",
		"--remove-existing-container=false",
	}, remoteEnvArgs...)

	cmd := exec.CommandContext(ctx, r.binary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		r.logger().Error("devcontainer up failed",
			slog.String("workspace_folder", workspaceFolder),
			slog.String("config", configPath),
			slog.String("stderr", stderr.String()),
			slog.Any("error", err))
		return ContainerRef{}, fmt.Errorf("devcontainer up: %w (stderr: %s)", err, truncateLine(stderr.String()))
	}

	// `devcontainer up` writes log lines to stdout when --log-format
	// is json; the final line is the structured result. Scan for the
	// success/error JSON object.
	result, parseErr := parseUpResult(stdout.Bytes())
	if parseErr != nil {
		return ContainerRef{}, fmt.Errorf("parse devcontainer up output: %w; stdout:\n%s", parseErr, truncateLine(stdout.String()))
	}
	if !strings.EqualFold(result.Outcome, "success") {
		return ContainerRef{}, fmt.Errorf("devcontainer up outcome=%s: %s", result.Outcome, result.Message)
	}

	ref := ContainerRef{
		ContainerID:           result.ContainerID,
		RemoteWorkspaceFolder: result.RemoteWorkspaceFolder,
	}
	ref.ImageDigest = r.resolveImageDigest(ctx, result.ContainerID)
	return ref, nil
}

// Exec runs a probe command inside the materialized container.
// Shells `devcontainer exec --workspace-folder <wsf> -- bash -lc
// <command>` so probe authors can use shell features (pipes,
// redirects). Exit code is mapped from the underlying command's
// exit code, not from `devcontainer exec` itself — a non-zero
// command exit is NOT an Exec error (it's a probe failure for
// Attest to interpret).
//
// ref.RemoteWorkspaceFolder is used for the --workspace-folder flag
// because that's what devcontainer keys on (not the host-side
// workspace folder). Falls back to a sentinel if RemoteWorkspaceFolder
// is empty (defensive — shouldn't happen post-Up).
func (r *CLIRunner) Exec(ctx context.Context, ref ContainerRef, command string) (ProbeResult, error) {
	if ref.ContainerID == "" {
		return ProbeResult{}, errors.New("CLIRunner.Exec: ContainerRef has no ContainerID (Up was not called)")
	}
	// Per PR 4.1 finding M4: don't fall back to a sentinel workspace
	// path. A missing RemoteWorkspaceFolder means Up returned a
	// malformed ref; running probes against a guessed path silently
	// targets the wrong tenant or a non-existent directory.
	if ref.RemoteWorkspaceFolder == "" {
		return ProbeResult{}, errors.New("CLIRunner.Exec: ContainerRef.RemoteWorkspaceFolder unset (devcontainer up did not populate)")
	}

	cmd := exec.CommandContext(ctx, r.binary(),
		"exec",
		"--workspace-folder", ref.RemoteWorkspaceFolder,
		"bash", "-lc", command,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := ProbeResult{
		Command: command,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
	}
	if err == nil {
		res.ExitCode = 0
		return res, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	// Real exec error (binary not found, context canceled). Surface
	// as Exec error — the manager attaches probe identity.
	return ProbeResult{Command: command}, fmt.Errorf("devcontainer exec: %w", err)
}

// resolveImageDigest queries `docker inspect` for the container's
// image digest. Empty return on any failure — image digest is
// operational metadata, not load-bearing. Logged on failure so
// drift is visible to ops.
func (r *CLIRunner) resolveImageDigest(ctx context.Context, containerID string) string {
	if containerID == "" {
		return ""
	}
	cmd := exec.CommandContext(ctx, r.dockerBinary(),
		"inspect", containerID, "--format", "{{.Image}}",
	)
	out, err := cmd.Output()
	if err != nil {
		r.logger().Warn("docker inspect image digest failed",
			slog.String("container_id", containerID),
			slog.Any("error", err))
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (r *CLIRunner) binary() string {
	if r == nil || r.Binary == "" {
		return "devcontainer"
	}
	return r.Binary
}

func (r *CLIRunner) dockerBinary() string {
	if r == nil || r.DockerBinary == "" {
		return "docker"
	}
	return r.DockerBinary
}

func (r *CLIRunner) logger() *slog.Logger {
	if r == nil || r.Logger == nil {
		return slog.Default()
	}
	return r.Logger
}

// parseUpResult scans the json-format stdout for the structured
// result object. @devcontainers/cli emits one JSON object per log
// event; the final line is the outcome. We accept either the
// "outcome" envelope shape or a stream with the result somewhere
// inside — defensive against minor CLI-version drift.
func parseUpResult(raw []byte) (devcontainerUpResult, error) {
	// Try the simple case: the entire output is one JSON object.
	var result devcontainerUpResult
	if err := json.Unmarshal(bytes.TrimSpace(raw), &result); err == nil && result.Outcome != "" {
		return result, nil
	}

	// Fallback: scan line-by-line for the last line with outcome.
	var last devcontainerUpResult
	scanner := bytes.SplitSeq(raw, []byte("\n"))
	for line := range scanner {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var candidate devcontainerUpResult
		if err := json.Unmarshal(line, &candidate); err != nil {
			continue
		}
		if candidate.Outcome != "" {
			last = candidate
		}
	}
	if last.Outcome == "" {
		return devcontainerUpResult{}, errors.New("no outcome field found in devcontainer up output")
	}
	return last, nil
}

func truncateLine(s string) string {
	s = strings.TrimSpace(s)
	const limit = 512
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
