package sandboxmanager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSandbox mimics cmd/semteams/sandbox's /exec endpoint enough
// to drive SandboxAPIRunner unit tests without a real sandbox
// container.
type fakeSandbox struct {
	server    *httptest.Server
	respond   func(req sandboxExecRequest) sandboxExecResponse
	statusErr int
	requests  []sandboxExecRequest
}

func newFakeSandbox(respond func(req sandboxExecRequest) sandboxExecResponse) *fakeSandbox {
	fs := &fakeSandbox{respond: respond}
	fs.server = httptest.NewServer(http.HandlerFunc(fs.handle))
	return fs
}

func (fs *fakeSandbox) Close() { fs.server.Close() }

func (fs *fakeSandbox) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/exec" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var req sandboxExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fs.requests = append(fs.requests, req)
	if fs.statusErr != 0 {
		w.WriteHeader(fs.statusErr)
		return
	}
	resp := fs.respond(req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func TestSandboxAPIRunner_Up_HappyPath(t *testing.T) {
	// Per PR 4.4 finding M5: branch on call sequence, not substring
	// of the escaped command body — refactors of joinShell would
	// otherwise silently break the substring matches and surface as
	// confusing "unknown command" failures instead of "joinShell
	// shape changed."
	//
	// PR 4.5 F5: the first /exec is the mkdir+cp stage call that
	// materialises the catalog profile into the tenant workspace.
	// Sequence is now: 1=stage, 2=devcontainer up, 3=docker inspect.
	var callIdx int
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		callIdx++
		switch callIdx {
		case 1:
			return sandboxExecResponse{ExitCode: 0}
		case 2:
			return sandboxExecResponse{
				ExitCode: 0,
				Stdout:   `{"outcome":"success","containerId":"c-abc","remoteWorkspaceFolder":"/workspaces/x"}`,
			}
		case 3:
			return sandboxExecResponse{ExitCode: 0, Stdout: "sha256:image123\n"}
		}
		t.Fatalf("unexpected /exec call %d", callIdx)
		return sandboxExecResponse{}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	wsf := "/var/lib/semteams-tenants/abc/workspace"
	ref, err := runner.Up(context.Background(), wsf, "/app/.devcontainer/go-backend/devcontainer.json", nil)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if ref.ContainerID != "c-abc" {
		t.Fatalf("ContainerID wrong: %q", ref.ContainerID)
	}
	if ref.RemoteWorkspaceFolder != "/workspaces/x" {
		t.Fatalf("remote workspace wrong: %q", ref.RemoteWorkspaceFolder)
	}
	// The runner must stamp the HOST wsf on the ref so subsequent
	// Exec passes the same value to `--workspace-folder` that Up
	// used (devcontainer-cli's container lookup keys on it).
	if ref.HostWorkspaceFolder != wsf {
		t.Fatalf("host workspace wrong: %q (want %q)", ref.HostWorkspaceFolder, wsf)
	}
	if ref.ImageDigest != "sha256:image123" {
		t.Fatalf("image digest wrong: %q", ref.ImageDigest)
	}
	if len(fs.requests) != 3 {
		t.Fatalf("expected 3 /exec calls (stage + up + docker inspect); got %d", len(fs.requests))
	}
	// PR 4.5 F5: the stage call materialises the catalog profile under
	// <workspaceFolder>/.devcontainer/devcontainer.json so the lockfile
	// devcontainer-cli writes lands in tenant-writable space, not the
	// :ro catalog mount. Assert both halves of the staging command.
	stagedConfig := wsf + "/.devcontainer/devcontainer.json"
	stageCmd := fs.requests[0].Command
	if !strings.Contains(stageCmd, "mkdir") || !strings.Contains(stageCmd, wsf+"/.devcontainer") {
		t.Fatalf("stage call missing mkdir of <wsf>/.devcontainer: %q", stageCmd)
	}
	if !strings.Contains(stageCmd, "cp") || !strings.Contains(stageCmd, stagedConfig) {
		t.Fatalf("stage call missing cp to staged config path: %q", stageCmd)
	}
	if !strings.Contains(stageCmd, "/app/.devcontainer/go-backend/devcontainer.json") {
		t.Fatalf("stage call missing source configPath: %q", stageCmd)
	}
	// PR 4.5 F6: --workspace-folder uses workspaceFolder VERBATIM (was
	// previously rewritten to /workspace/<task_id>, breaking DooD path
	// translation when the sandbox tried to spawn a sibling container).
	upCmd := fs.requests[1].Command
	if !strings.Contains(upCmd, "'devcontainer' 'up' '--workspace-folder' '"+wsf+"'") {
		t.Fatalf("up call did not pass workspaceFolder verbatim: %q", upCmd)
	}
	// PR 4.5 F5: --config points at the staged path under the
	// tenant workspace, not the :ro catalog mount path.
	if !strings.Contains(upCmd, "'--config' '"+stagedConfig+"'") {
		t.Fatalf("up call did not target staged config: %q", upCmd)
	}
	// Per PR 4.4 finding M4: every /exec call posts against the SAME
	// task_id so the sandbox's per-task mutex serialises them under a
	// single chain identity.
	taskID := fs.requests[0].TaskID
	if taskID == "" {
		t.Fatalf("task_id not populated on stage call")
	}
	for i, r := range fs.requests {
		if r.TaskID != taskID {
			t.Fatalf("call %d task_id drift: %q vs %q", i, r.TaskID, taskID)
		}
	}
	// Verify Properties carries the same task_id forward for Exec.
	if got := ref.Properties["sandbox_task_id"]; got != taskID {
		t.Fatalf("sandbox_task_id not stamped on ref.Properties: %v", ref.Properties)
	}
}

func TestSandboxAPIRunner_UpExec_ShareTaskID(t *testing.T) {
	// Per PR 4.4 finding M4: Up + Exec must POST against the same
	// task_id so the sandbox's per-task mutex serializes them.
	// PR 4.5 F5: a leading stage call lands first, shifting the
	// sequence to 1=stage, 2=up, 3=inspect, 4=exec.
	var callIdx int
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		callIdx++
		switch callIdx {
		case 1:
			return sandboxExecResponse{ExitCode: 0}
		case 2:
			return sandboxExecResponse{
				ExitCode: 0,
				Stdout:   `{"outcome":"success","containerId":"c-1","remoteWorkspaceFolder":"/workspaces/r"}`,
			}
		case 3:
			return sandboxExecResponse{ExitCode: 0, Stdout: "sha256:img"}
		case 4:
			return sandboxExecResponse{ExitCode: 0, Stdout: "go version"}
		}
		return sandboxExecResponse{ExitCode: 127}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	ref, err := runner.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/cfg.json", nil)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := runner.Exec(context.Background(), ref, "go version"); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(fs.requests) != 4 {
		t.Fatalf("expected 4 /exec calls (stage + up + inspect + exec); got %d", len(fs.requests))
	}
	// All four calls must share the task_id minted at Up.
	taskID := fs.requests[0].TaskID
	for i, r := range fs.requests {
		if r.TaskID != taskID {
			t.Fatalf("call %d task_id drift: %q vs %q", i, r.TaskID, taskID)
		}
	}
}

func TestSandboxAPIRunner_Up_HostSecretMissingFailsFast(t *testing.T) {
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		t.Fatalf("Up must not POST when host env var is unset")
		return sandboxExecResponse{}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	// Empty value = pass-through from host. SAFETY_TEST_MISSING is unset.
	_, err := runner.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/app/.devcontainer/x.json",
		map[string]string{"SANDBOXAPIRUNNER_TEST_MISSING": ""})
	if err == nil {
		t.Fatalf("expected error when host secret unset")
	}
	if !strings.Contains(err.Error(), "SANDBOXAPIRUNNER_TEST_MISSING") {
		t.Fatalf("err does not name the missing secret: %v", err)
	}
}

func TestSandboxAPIRunner_Up_NonZeroExit(t *testing.T) {
	// PR 4.5 F5: stage call is first; succeed it so the up call (which
	// surfaces the simulated daemon failure) actually runs.
	var callIdx int
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		callIdx++
		if callIdx == 1 {
			return sandboxExecResponse{ExitCode: 0}
		}
		return sandboxExecResponse{ExitCode: 1, Stderr: "docker daemon not running"}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	_, err := runner.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/app/.devcontainer/x.json", nil)
	if err == nil {
		t.Fatalf("expected error on non-zero exit")
	}
	if !strings.Contains(err.Error(), "daemon not running") {
		t.Fatalf("stderr not surfaced: %v", err)
	}
}

func TestSandboxAPIRunner_Up_StageFailureSurfacesEarly(t *testing.T) {
	// PR 4.5 F5: if mkdir+cp fails (e.g. EROFS on the workspace bind),
	// Up must surface that as a clear error BEFORE attempting
	// `devcontainer up`. The pre-PR-4.5 path conflated stage failures
	// with "devcontainer up" failures, hiding the production gap that
	// motivated this change.
	var callIdx int
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		callIdx++
		if callIdx == 1 {
			return sandboxExecResponse{ExitCode: 1, Stderr: "cp: cannot create regular file ‘/var/lib/semteams-tenants/abc/workspace/.devcontainer/devcontainer.json’: Read-only file system"}
		}
		t.Fatalf("stage failure must short-circuit Up; got call %d", callIdx)
		return sandboxExecResponse{}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	_, err := runner.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/app/.devcontainer/go-backend/devcontainer.json", nil)
	if err == nil {
		t.Fatalf("expected stage-failure error")
	}
	// PR 4.5 review M3: assert on the wrapper, not the specific stderr
	// fragment. Real-world failures include EROFS (catalog mount) AND
	// EACCES (uid mismatch under macOS Docker Desktop virtiofs). Locking
	// the test to "Read-only file system" would mask the EACCES class.
	if !strings.Contains(err.Error(), "stage devcontainer profile") {
		t.Fatalf("err does not name the staging step: %v", err)
	}
}

func TestSandboxAPIRunner_Up_EmptyWorkspaceFolder(t *testing.T) {
	// PR 4.5 F6: workspaceFolder is passed verbatim to `--workspace-folder`
	// AND used as the stage-call mkdir target. An empty string would
	// invoke `mkdir -p /.devcontainer && cp ... /.devcontainer/devcontainer.json`
	// (writes to the sandbox container's root fs, which is read-only).
	// Fail-fast rejects the call before any /exec POST.
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		t.Fatalf("must not POST on empty workspaceFolder")
		return sandboxExecResponse{}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	_, err := runner.Up(context.Background(), "", "/cfg.json", nil)
	if err == nil {
		t.Fatalf("expected error on empty workspaceFolder")
	}
}

func TestSandboxAPIRunner_Up_HTTPError(t *testing.T) {
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse { return sandboxExecResponse{} })
	fs.statusErr = http.StatusInternalServerError
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	_, err := runner.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/app/.devcontainer/x.json", nil)
	if err == nil {
		t.Fatalf("expected error on 500")
	}
}

func TestSandboxAPIRunner_Exec_ReturnsProbeResult(t *testing.T) {
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		return sandboxExecResponse{ExitCode: 0, Stdout: "go version go1.25.4"}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	hostWsf := "/var/lib/semteams-tenants/abc/workspace"
	res, err := runner.Exec(context.Background(),
		ContainerRef{ContainerID: "c-abc", HostWorkspaceFolder: hostWsf, RemoteWorkspaceFolder: "/workspaces/x"},
		"go version")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code wrong: %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "go1.25.4") {
		t.Fatalf("stdout not propagated: %q", res.Stdout)
	}
	// The shelled command must pass the HOST wsf to
	// `--workspace-folder` (not RemoteWorkspaceFolder). Failure
	// shape if regressed: "Dev container config not found" from
	// devcontainer-cli, surfaced as exit 1 on every probe.
	if len(fs.requests) != 1 {
		t.Fatalf("expected 1 /exec call, got %d", len(fs.requests))
	}
	cmd := fs.requests[0].Command
	if !strings.Contains(cmd, "'--workspace-folder' '"+hostWsf+"'") {
		t.Fatalf("Exec did not pass HostWorkspaceFolder to --workspace-folder: %q", cmd)
	}
	if strings.Contains(cmd, "'--workspace-folder' '/workspaces/x'") {
		t.Fatalf("Exec passed RemoteWorkspaceFolder (the smoke-#13 bug): %q", cmd)
	}
}

func TestSandboxAPIRunner_Exec_EmptyContainerID(t *testing.T) {
	runner := NewSandboxAPIRunner("http://placeholder")
	_, err := runner.Exec(context.Background(), ContainerRef{}, "echo hi")
	if err == nil {
		t.Fatalf("expected error on empty ContainerID")
	}
}

func TestSandboxAPIRunner_Exec_EmptyHostWorkspace(t *testing.T) {
	// Pre-fix refs only set RemoteWorkspaceFolder; that's no longer
	// accepted because passing it to `--workspace-folder` is the
	// smoke-#13 probe-exit-1 bug. Fail loudly on bare ContainerID.
	runner := NewSandboxAPIRunner("http://placeholder")
	_, err := runner.Exec(context.Background(), ContainerRef{ContainerID: "c1"}, "echo hi")
	if err == nil {
		t.Fatalf("expected error on empty HostWorkspaceFolder")
	}
	if !strings.Contains(err.Error(), "HostWorkspaceFolder") {
		t.Fatalf("error does not mention HostWorkspaceFolder: %v", err)
	}
}

func TestSandboxAPIRunner_Exec_RejectsRemoteOnlyRef(t *testing.T) {
	// Defensive: even with a non-empty RemoteWorkspaceFolder, the
	// runner must NOT fall back to it for --workspace-folder. The
	// pre-fix code did, which made every probe exit 1 in smoke #13.
	runner := NewSandboxAPIRunner("http://placeholder")
	_, err := runner.Exec(context.Background(),
		ContainerRef{ContainerID: "c1", RemoteWorkspaceFolder: "/workspaces/x"},
		"echo hi")
	if err == nil {
		t.Fatalf("expected error on RemoteWorkspaceFolder-only ref")
	}
	if !strings.Contains(err.Error(), "HostWorkspaceFolder") {
		t.Fatalf("error does not mention HostWorkspaceFolder: %v", err)
	}
}

func TestSandboxAPIRunner_Exec_RejectsRelativeHostWorkspace(t *testing.T) {
	// Mirror Up's absolute-path discipline. A relative
	// HostWorkspaceFolder surfaces as the same opaque "config not
	// found" devcontainer-cli error that the wsf split fixed; reject
	// at the boundary instead.
	runner := NewSandboxAPIRunner("http://placeholder")
	_, err := runner.Exec(context.Background(),
		ContainerRef{ContainerID: "c1", HostWorkspaceFolder: "relative/path"},
		"echo hi")
	if err == nil {
		t.Fatalf("expected error on relative HostWorkspaceFolder")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("error does not name the absolute-path rule: %v", err)
	}
}

func TestNewSandboxAPIRunner_EmptyURL(t *testing.T) {
	if r := NewSandboxAPIRunner(""); r != nil {
		t.Fatalf("expected nil for empty URL")
	}
	if r := NewSandboxAPIRunner("   "); r != nil {
		t.Fatalf("expected nil for whitespace URL")
	}
}

func TestTaskIDFor_Stable(t *testing.T) {
	a := taskIDFor("/var/lib/semteams-tenants/abc/workspace")
	b := taskIDFor("/var/lib/semteams-tenants/abc/workspace/")
	if a != b {
		t.Fatalf("trailing-slash variants drifted: %q vs %q", a, b)
	}
	if a == taskIDFor("/var/lib/semteams-tenants/different/workspace") {
		t.Fatalf("distinct paths collided")
	}
}

func TestJoinShell_EscapesSingleQuotes(t *testing.T) {
	got := joinShell([]string{"echo", "it's fine"})
	want := `'echo' 'it'\''s fine'`
	if got != want {
		t.Fatalf("escape wrong: %q want %q", got, want)
	}
}
