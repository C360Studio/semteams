//go:build integration

// Package integration is the home for cross-process / cross-container
// smokes that exist purely to verify that ADR-decided trust boundaries
// or wire contracts behave correctly when assembled. Tests here MUST
// be guarded by an explicit env var so a default `task test:integration`
// run doesn't silently require infrastructure (running stack, mounted
// docker.sock, etc.) that isn't always present.
package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSandboxDooDSmoke is the R3.7.2.d′ acceptance smoke for ADR-034
// §addendum #2 (sandbox Docker-out-of-Docker, DooD). It proves the
// trust-boundary delta: an unprivileged `sandbox` user (uid 1001,
// supplementary group 0 via compose group_add) running inside the
// hardened sandbox container can talk to the host's mounted
// /var/run/docker.sock and spawn a sibling container that's reachable
// on the shared compose network.
//
// What this exercises end-to-end:
//  1. `docker` CLI binary is present in the sandbox image (sandbox
//     Dockerfile change).
//  2. /var/run/docker.sock is bind-mounted from the host (compose
//     change) and the sandbox process can read it (group_add 0).
//  3. The host docker daemon will run a sibling container under the
//     sandbox process's request (the actual DooD trust boundary).
//  4. Sibling container is reachable from the sandbox on
//     `${SANDBOX_DOOD_SMOKE_NETWORK}` (default `ui_agentic-net`).
//
// Drives via `docker exec` against the running sandbox container
// rather than the sandbox HTTP API: the smoke target boots only the
// sandbox service (no backend, no port publish), and exec is the
// thinnest path that exercises the sandbox user identity. Production
// chains hit the same code path through the sandbox HTTP /exec
// endpoint; both run their bash through the same sandbox-server +
// `bash -c` plumbing, so the trust-boundary surface is identical.
//
// Skipped unless SANDBOX_DOOD_SMOKE=1 so the default integration
// suite (`task test:integration`) doesn't require a running compose
// stack.
func TestSandboxDooDSmoke(t *testing.T) {
	if os.Getenv("SANDBOX_DOOD_SMOKE") != "1" {
		t.Skip("set SANDBOX_DOOD_SMOKE=1 (with sandbox container up + --docker-mode=dood) to run; see ui/Taskfile.yml test:e2e:sandbox:dood:smoke")
	}

	container := envOr("SANDBOX_DOOD_SMOKE_CONTAINER", "semteams-ui-agentic-sandbox")
	// Default matches the COMPOSE_PROJECT_NAME=semteams-agentic set in
	// ui/Taskfile.yml; override via env when running against a stack
	// brought up under a different project name.
	network := envOr("SANDBOX_DOOD_SMOKE_NETWORK", "semteams-agentic_agentic-net")
	natsImage := envOr("SANDBOX_DOOD_SMOKE_NATS_IMAGE", "nats:2.10-alpine")
	natsName := fmt.Sprintf("dood-smoke-nats-%d", time.Now().UnixNano())
	// Label every spawned sibling so the cleanup task in
	// ui/Taskfile.yml can sweep stragglers if the test SIGKILL's
	// between `docker run -d` and `docker stop`. `--rm` only fires
	// when the container exits cleanly; the label sweep covers the
	// crash path. Single label value because this smoke is
	// single-tenant by design.
	const smokeLabel = "org.semteams.dood-smoke=1"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: docker CLI inside sandbox can talk to the host daemon.
	// `docker version` exits non-zero if the socket is unreadable
	// (EACCES) or absent — the cleanest single-call probe for "DooD
	// is wired".
	out, err := dockerExec(ctx, t, container, "docker version --format '{{.Server.Version}}'")
	if err != nil {
		t.Fatalf("docker version inside sandbox failed (DooD socket not reachable?): %v\n--- output ---\n%s", err, out)
	}
	serverVersion := strings.TrimSpace(out)
	if serverVersion == "" {
		t.Fatalf("docker version returned empty server version — daemon not reachable")
	}
	t.Logf("DooD reachable; host docker server version=%s", serverVersion)

	// Step 2: spawn a sibling nats container on the shared compose
	// network. `--rm` plus the t.Cleanup belt-and-suspenders covers
	// both happy-path and crash-path teardown.
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Best-effort; ignore errors — the container may already be
		// gone if the test passed cleanly.
		_, _ = dockerExec(stopCtx, t,
			container,
			fmt.Sprintf("docker stop %s >/dev/null 2>&1 || true", natsName))
	})

	spawn := fmt.Sprintf(
		"docker run -d --rm --label %s --name %s --network %s %s >/dev/null && echo spawned",
		smokeLabel, natsName, network, natsImage,
	)
	out, err = dockerExec(ctx, t, container, spawn)
	if err != nil {
		t.Fatalf("spawn nats sibling: %v\n--- output ---\n%s", err, out)
	}
	if !strings.Contains(out, "spawned") {
		t.Fatalf("spawn nats sibling — expected 'spawned' confirmation, got: %s", out)
	}

	// Step 3: from the sandbox, open a TCP connection to the sibling
	// container's NATS port. Using bash's /dev/tcp redirect avoids
	// depending on `nc` or any extra tool in the sandbox image — the
	// kernel handles the connect and bash exits 0 iff the socket
	// completed the TCP handshake.
	//
	// Probe text is deliberately metacharacter-free so the
	// docker-exec → sh -c → bash -c double-shell-parse stays
	// invariant. Adding `$VAR`, backticks, or quoted strings here
	// will silently break under one of the parse layers; either keep
	// it metacharacter-free or pass `bash -c` argv via dockerExecArgs
	// (no shell on the docker exec side).
	probe := fmt.Sprintf(
		`for i in $(seq 1 30); do (echo > /dev/tcp/%s/4222) 2>/dev/null && echo nats-ready && exit 0; sleep 1; done; echo nats-timeout && exit 1`,
		natsName,
	)
	out, err = dockerExecArgs(ctx, t, container, "bash", "-c", probe)
	if err != nil {
		t.Fatalf("nats TCP probe from sandbox: %v\n--- output ---\n%s", err, out)
	}
	if !strings.Contains(out, "nats-ready") {
		t.Fatalf("nats not reachable on %s:4222 from sandbox; got: %s", natsName, out)
	}
	t.Logf("trust-boundary verified: sandbox spawned sibling %q on %q and completed TCP handshake on :4222", natsName, network)
}

// dockerExec runs `docker exec <container> sh -c <command>` and
// returns combined stdout+stderr. Use this when the command is a
// shell pipeline / one-liner. For commands that should bypass the
// docker-exec shell layer (e.g. bash -c with embedded scripts whose
// metacharacters would otherwise be parsed twice), use
// dockerExecArgs instead.
func dockerExec(ctx context.Context, t *testing.T, container, command string) (string, error) {
	t.Helper()
	return dockerExecArgs(ctx, t, container, "sh", "-c", command)
}

// dockerExecArgs runs `docker exec <container> <argv...>` with no
// intermediate shell. Each element of argv is delivered as a
// distinct C-level argument to docker exec, so the only shell parse
// is whatever the explicit interpreter (e.g. `bash -c <script>`) in
// argv[0..1] performs on argv[2]. Prefer this for scripts with
// metacharacters; the inner script is parsed exactly once.
func dockerExecArgs(ctx context.Context, t *testing.T, container string, argv ...string) (string, error) {
	t.Helper()
	full := append([]string{"exec", container}, argv...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// envOr reads an env var, returning fallback on empty/unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
