package sandboxmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// SandboxAPIRunner shells `devcontainer up` and `devcontainer exec`
// through the SemTeams sandbox container's HTTP API
// (cmd/semteams/sandbox/server.go) rather than os/exec-ing locally.
// Preserves the existing DooD trust boundary: the backend container
// has no Docker socket; the sandbox container does AND has
// @devcontainers/cli installed (PR 4.1 sandbox Dockerfile change).
//
// Wire shape: POST /exec body is {task_id, command, timeout_ms};
// response is {stdout, stderr, exit_code, timed_out}. The runner
// derives a stable task_id from the workspaceFolder argument so
// repeated Ups against the same logical signature reuse the same
// task workspace inside the sandbox (sandbox's lazy-autocreate
// makes the first call idempotent — no separate /worktree POST
// required).
//
// Image digest resolution: the runner shells `docker inspect
// <container_id> --format '{{.Image}}'` via the same /exec path.
// Best-effort — empty digest doesn't fail Up.
type SandboxAPIRunner struct {
	// BaseURL is the sandbox HTTP API root (e.g.
	// "http://sandbox:8090"). Required.
	BaseURL string

	// HTTPClient overrides the default http.Client for tests.
	// Zero defaults to a client with a 10-minute timeout (the
	// outer per-request context cancels earlier).
	HTTPClient *http.Client

	// Logger is the structured logger for runner-level events.
	// Zero defaults to slog.Default.
	Logger *slog.Logger
}

// NewSandboxAPIRunner constructs a runner bound to baseURL.
// Returns nil if baseURL is empty — operator config error;
// product_tools.go falls back to NoopRunner with a Warn log.
func NewSandboxAPIRunner(baseURL string) *SandboxAPIRunner {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &SandboxAPIRunner{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 10 * time.Minute},
		Logger:     slog.Default(),
	}
}

// Up materializes the devcontainer via `devcontainer up` over the
// sandbox HTTP API. The workspaceFolder argument is the host-side
// path the manager computes (e.g. /tenants/<sig>/workspace); the
// runner translates it to a sandbox-internal path via taskIDFor and
// passes the profile config path verbatim — devcontainer config
// paths must be reachable inside the sandbox container, typically
// via a compose mount of .devcontainer/ at the same path the
// backend uses.
func (r *SandboxAPIRunner) Up(ctx context.Context, workspaceFolder, configPath string, env map[string]string) (ContainerRef, error) {
	taskID := taskIDFor(workspaceFolder)
	args := []string{
		"devcontainer", "up",
		"--workspace-folder", sandboxWorkspacePath(taskID),
		"--config", configPath,
		"--log-format", "json",
		"--remove-existing-container=false",
	}
	for k, v := range env {
		if v == "" {
			hostVal, ok := os.LookupEnv(k)
			if !ok {
				return ContainerRef{}, fmt.Errorf("devcontainer up: secret %q declared but not present in host environment (operator AvailableSecrets policy promised it; export %s before invoking)", k, k)
			}
			args = append(args, "--remote-env", k+"="+hostVal)
			continue
		}
		args = append(args, "--remote-env", k+"="+v)
	}

	res, err := r.exec(ctx, taskID, joinShell(args), 5*time.Minute)
	if err != nil {
		return ContainerRef{}, fmt.Errorf("sandbox /exec devcontainer up: %w", err)
	}
	if res.ExitCode != 0 {
		return ContainerRef{}, fmt.Errorf("devcontainer up: exit %d; stderr: %s", res.ExitCode, truncateLine(res.Stderr))
	}
	result, parseErr := parseUpResult([]byte(res.Stdout))
	if parseErr != nil {
		return ContainerRef{}, fmt.Errorf("parse devcontainer up output: %w; stdout:\n%s", parseErr, truncateLine(res.Stdout))
	}
	if !strings.EqualFold(result.Outcome, "success") {
		return ContainerRef{}, fmt.Errorf("devcontainer up outcome=%s: %s", result.Outcome, result.Message)
	}

	ref := ContainerRef{
		ContainerID:           result.ContainerID,
		RemoteWorkspaceFolder: result.RemoteWorkspaceFolder,
		// Per PR 4.4 finding M4: stash the host-side task_id on the
		// ref so Exec posts against the SAME sandbox per-task mutex
		// as Up. Deriving Exec's task_id from RemoteWorkspaceFolder
		// would key on the container-internal path and break the
		// serialization-per-chain claim.
		Properties: map[string]string{"sandbox_task_id": taskID},
	}
	ref.ImageDigest = r.resolveImageDigest(ctx, taskID, ref.ContainerID)
	return ref, nil
}

// Exec runs a probe command inside the materialized devcontainer
// via the sandbox HTTP API. The probe command is wrapped in
// `devcontainer exec --workspace-folder <wsf> -- bash -lc <command>`
// so probe authors can use shell features.
func (r *SandboxAPIRunner) Exec(ctx context.Context, ref ContainerRef, command string) (ProbeResult, error) {
	if ref.ContainerID == "" {
		return ProbeResult{}, errors.New("SandboxAPIRunner.Exec: ContainerRef has no ContainerID (Up was not called)")
	}
	if ref.RemoteWorkspaceFolder == "" {
		return ProbeResult{}, errors.New("SandboxAPIRunner.Exec: ContainerRef.RemoteWorkspaceFolder unset (devcontainer up did not populate)")
	}
	// Use the SAME task_id Up stamped on the ref (per PR 4.4 finding
	// M4). The sandbox's per-task mutex then serializes Up + every
	// Exec against the same lifecycle. Fall back to deriving from
	// RemoteWorkspaceFolder when Properties is absent (defensive —
	// shouldn't happen post-Up).
	taskID := ""
	if ref.Properties != nil {
		taskID = ref.Properties["sandbox_task_id"]
	}
	if taskID == "" {
		taskID = taskIDFor(ref.RemoteWorkspaceFolder)
	}
	wrapped := joinShell([]string{
		"devcontainer", "exec",
		"--workspace-folder", ref.RemoteWorkspaceFolder,
		"bash", "-lc", command,
	})
	res, err := r.exec(ctx, taskID, wrapped, 60*time.Second)
	if err != nil {
		return ProbeResult{Command: command}, fmt.Errorf("sandbox /exec devcontainer exec: %w", err)
	}
	return ProbeResult{
		Command:  command,
		ExitCode: res.ExitCode,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
	}, nil
}

// resolveImageDigest queries `docker inspect` via the sandbox API.
// Best-effort: any failure leaves the digest empty.
func (r *SandboxAPIRunner) resolveImageDigest(ctx context.Context, taskID, containerID string) string {
	if containerID == "" {
		return ""
	}
	cmd := joinShell([]string{"docker", "inspect", containerID, "--format", "{{.Image}}"})
	res, err := r.exec(ctx, taskID, cmd, 15*time.Second)
	if err != nil || res.ExitCode != 0 {
		r.logger().Warn("docker inspect image digest failed via sandbox API",
			slog.String("container_id", containerID),
			slog.Int("exit_code", res.ExitCode),
			slog.Any("error", err))
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// sandboxExecRequest mirrors cmd/semteams/sandbox/server.go
// execRequest. Keep field names in lockstep — drift breaks the
// wire contract silently.
type sandboxExecRequest struct {
	TaskID    string `json:"task_id"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// sandboxExecResponse mirrors execResponse from the sandbox server.
type sandboxExecResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

// exec POSTs a single command to the sandbox /exec endpoint and
// returns the decoded response. The HTTP timeout is the outer ctx;
// timeoutMs is the per-command sandbox-side cap.
func (r *SandboxAPIRunner) exec(ctx context.Context, taskID, command string, timeout time.Duration) (sandboxExecResponse, error) {
	body, _ := json.Marshal(sandboxExecRequest{
		TaskID:    taskID,
		Command:   command,
		TimeoutMs: int(timeout.Milliseconds()),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/exec", bytes.NewReader(body))
	if err != nil {
		return sandboxExecResponse{}, fmt.Errorf("build /exec request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return sandboxExecResponse{}, fmt.Errorf("POST /exec: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return sandboxExecResponse{}, fmt.Errorf("/exec status %d: %s", resp.StatusCode, truncateLine(string(raw)))
	}
	var out sandboxExecResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return sandboxExecResponse{}, fmt.Errorf("decode /exec response: %w", err)
	}
	if out.TimedOut {
		return out, fmt.Errorf("/exec command timed out after %s", timeout)
	}
	return out, nil
}

func (r *SandboxAPIRunner) httpClient() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return http.DefaultClient
}

func (r *SandboxAPIRunner) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

// taskIDFor derives a deterministic, sandbox-valid task_id from a
// host workspace folder path. The fnv12 hex output is alphanumeric,
// well within the sandbox's accepted charset (alphanumerics + `.`
// + `-` + `_`).
func taskIDFor(workspaceFolder string) string {
	clean := strings.TrimSpace(workspaceFolder)
	if clean == "" {
		return "sandbox-mgr-default"
	}
	clean = strings.TrimRight(clean, "/")
	return "sandbox-mgr-" + fnv12(clean)
}

// sandboxWorkspacePath returns the canonical path inside the sandbox
// container for a given taskID. Mirrors cmd/semteams/sandbox/main.go's
// --workspace flag (typically /workspace).
func sandboxWorkspacePath(taskID string) string {
	return "/workspace/" + taskID
}

// joinShell concatenates argv into a bash-runnable string with
// single-quote escaping. Inside the sandbox /exec endpoint the
// command runs via `bash -c`, so each token must round-trip
// through that wrapper both without word-splitting AND without
// allowing command injection from embedded metacharacters. The
// single-quote-escape pattern (`'arg'\”with_quote'`) is the
// canonical bash-safe escape: an arg containing literal `;rm -rf /`
// remains inert because the entire string is enclosed in single
// quotes before substitution. This IS the security boundary
// between LLM-controlled arg content and the sandbox shell.
func joinShell(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}
