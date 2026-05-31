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
	var callIdx int
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		callIdx++
		switch callIdx {
		case 1:
			return sandboxExecResponse{
				ExitCode: 0,
				Stdout:   `{"outcome":"success","containerId":"c-abc","remoteWorkspaceFolder":"/workspaces/x"}`,
			}
		case 2:
			return sandboxExecResponse{ExitCode: 0, Stdout: "sha256:image123\n"}
		}
		t.Fatalf("unexpected /exec call %d", callIdx)
		return sandboxExecResponse{}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	ref, err := runner.Up(context.Background(), "/tenants/abc/workspace", "/app/.devcontainer/go-backend/devcontainer.json", nil)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if ref.ContainerID != "c-abc" {
		t.Fatalf("ContainerID wrong: %q", ref.ContainerID)
	}
	if ref.RemoteWorkspaceFolder != "/workspaces/x" {
		t.Fatalf("workspace wrong: %q", ref.RemoteWorkspaceFolder)
	}
	if ref.ImageDigest != "sha256:image123" {
		t.Fatalf("image digest wrong: %q", ref.ImageDigest)
	}
	if len(fs.requests) < 2 {
		t.Fatalf("expected at least 2 /exec calls (up + docker inspect); got %d", len(fs.requests))
	}
	if fs.requests[0].TaskID == "" {
		t.Fatalf("task_id not populated on Up call")
	}
	// Per PR 4.4 finding M4: both /exec calls (up + docker inspect)
	// post against the SAME task_id so the sandbox's per-task mutex
	// serializes them under a single chain identity.
	if fs.requests[0].TaskID != fs.requests[1].TaskID {
		t.Fatalf("up + docker-inspect task_id drift: %q vs %q",
			fs.requests[0].TaskID, fs.requests[1].TaskID)
	}
	// Verify Properties carries the same task_id forward for Exec.
	if got := ref.Properties["sandbox_task_id"]; got == "" {
		t.Fatalf("sandbox_task_id not stamped on ref.Properties: %v", ref.Properties)
	}
}

func TestSandboxAPIRunner_UpExec_ShareTaskID(t *testing.T) {
	// Per PR 4.4 finding M4: Up + Exec must POST against the same
	// task_id so the sandbox's per-task mutex serializes them.
	var callIdx int
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		callIdx++
		switch callIdx {
		case 1:
			return sandboxExecResponse{
				ExitCode: 0,
				Stdout:   `{"outcome":"success","containerId":"c-1","remoteWorkspaceFolder":"/workspaces/r"}`,
			}
		case 2:
			return sandboxExecResponse{ExitCode: 0, Stdout: "sha256:img"}
		case 3:
			return sandboxExecResponse{ExitCode: 0, Stdout: "go version"}
		}
		return sandboxExecResponse{ExitCode: 127}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	ref, err := runner.Up(context.Background(), "/tenants/abc/workspace", "/cfg.json", nil)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := runner.Exec(context.Background(), ref, "go version"); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(fs.requests) != 3 {
		t.Fatalf("expected 3 /exec calls (up + inspect + exec); got %d", len(fs.requests))
	}
	// All three calls must share the task_id minted at Up.
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
	_, err := runner.Up(context.Background(), "/tenants/abc/workspace", "/app/.devcontainer/x.json",
		map[string]string{"SANDBOXAPIRUNNER_TEST_MISSING": ""})
	if err == nil {
		t.Fatalf("expected error when host secret unset")
	}
	if !strings.Contains(err.Error(), "SANDBOXAPIRUNNER_TEST_MISSING") {
		t.Fatalf("err does not name the missing secret: %v", err)
	}
}

func TestSandboxAPIRunner_Up_NonZeroExit(t *testing.T) {
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse {
		return sandboxExecResponse{ExitCode: 1, Stderr: "docker daemon not running"}
	})
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	_, err := runner.Up(context.Background(), "/tenants/abc/workspace", "/app/.devcontainer/x.json", nil)
	if err == nil {
		t.Fatalf("expected error on non-zero exit")
	}
	if !strings.Contains(err.Error(), "daemon not running") {
		t.Fatalf("stderr not surfaced: %v", err)
	}
}

func TestSandboxAPIRunner_Up_HTTPError(t *testing.T) {
	fs := newFakeSandbox(func(_ sandboxExecRequest) sandboxExecResponse { return sandboxExecResponse{} })
	fs.statusErr = http.StatusInternalServerError
	defer fs.Close()

	runner := NewSandboxAPIRunner(fs.server.URL)
	_, err := runner.Up(context.Background(), "/tenants/abc/workspace", "/app/.devcontainer/x.json", nil)
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
	res, err := runner.Exec(context.Background(),
		ContainerRef{ContainerID: "c-abc", RemoteWorkspaceFolder: "/workspaces/x"},
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
}

func TestSandboxAPIRunner_Exec_EmptyContainerID(t *testing.T) {
	runner := NewSandboxAPIRunner("http://placeholder")
	_, err := runner.Exec(context.Background(), ContainerRef{}, "echo hi")
	if err == nil {
		t.Fatalf("expected error on empty ContainerID")
	}
}

func TestSandboxAPIRunner_Exec_EmptyWorkspace(t *testing.T) {
	runner := NewSandboxAPIRunner("http://placeholder")
	_, err := runner.Exec(context.Background(), ContainerRef{ContainerID: "c1"}, "echo hi")
	if err == nil {
		t.Fatalf("expected error on empty RemoteWorkspaceFolder")
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
	a := taskIDFor("/tenants/abc/workspace")
	b := taskIDFor("/tenants/abc/workspace/")
	if a != b {
		t.Fatalf("trailing-slash variants drifted: %q vs %q", a, b)
	}
	if a == taskIDFor("/tenants/different/workspace") {
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
