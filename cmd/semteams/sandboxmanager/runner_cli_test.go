package sandboxmanager

import (
	"os"
	"strings"
	"testing"
)

func TestParseUpResult_SingleObject(t *testing.T) {
	raw := []byte(`{"outcome":"success","containerId":"c123","remoteWorkspaceFolder":"/workspaces/x"}`)
	r, err := parseUpResult(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.ContainerID != "c123" || r.Outcome != "success" {
		t.Fatalf("parse drift: %+v", r)
	}
}

func TestParseUpResult_StreamedLines(t *testing.T) {
	raw := []byte(`{"phase":"pulling"}
{"phase":"creating"}
{"outcome":"success","containerId":"c456","remoteWorkspaceFolder":"/workspaces/y"}
`)
	r, err := parseUpResult(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.ContainerID != "c456" {
		t.Fatalf("expected last outcome line picked up, got %q", r.ContainerID)
	}
}

func TestParseUpResult_NoOutcome(t *testing.T) {
	raw := []byte(`{"phase":"pulling"}` + "\n")
	_, err := parseUpResult(raw)
	if err == nil {
		t.Fatalf("expected error when no outcome present")
	}
}

func TestParseUpResult_ErrorOutcome(t *testing.T) {
	raw := []byte(`{"outcome":"error","message":"workspace folder not found"}`)
	r, err := parseUpResult(raw)
	if err != nil {
		t.Fatalf("parse should succeed on error outcome (CLI emits valid JSON): %v", err)
	}
	if r.Outcome != "error" {
		t.Fatalf("outcome not propagated: %q", r.Outcome)
	}
	if !strings.Contains(r.Message, "not found") {
		t.Fatalf("message not propagated: %q", r.Message)
	}
}

func TestCLIRunner_BinaryDefaults(t *testing.T) {
	var r *CLIRunner
	if got := r.binary(); got != "devcontainer" {
		t.Fatalf("nil receiver default wrong: %q", got)
	}
	r2 := &CLIRunner{}
	if got := r2.binary(); got != "devcontainer" {
		t.Fatalf("empty Binary default wrong: %q", got)
	}
	r3 := &CLIRunner{Binary: "/usr/local/bin/devcontainer"}
	if got := r3.binary(); got != "/usr/local/bin/devcontainer" {
		t.Fatalf("explicit Binary not honored: %q", got)
	}
}

func TestCLIRunner_UpRejectsMissingHostSecret(t *testing.T) {
	// Per PR 4.1 finding C1: when env[K] == "" the runner reads
	// os.LookupEnv(K); if unset it must fail Up rather than silently
	// bring the container up with K="".
	r := NewCLIRunner()
	// Point at a non-existent binary so the test fails fast if the
	// secret-check logic doesn't short-circuit. The env-validation
	// step runs before exec.Run.
	r.Binary = "/nonexistent/devcontainer"
	if err := os.Unsetenv("SANDBOXMANAGER_TEST_MISSING"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	_, err := r.Up(t.Context(), "/tmp/wsf", "/tmp/cfg.json",
		map[string]string{"SANDBOXMANAGER_TEST_MISSING": ""})
	if err == nil {
		t.Fatalf("expected error when host env var unset; got nil")
	}
	if !strings.Contains(err.Error(), "SANDBOXMANAGER_TEST_MISSING") {
		t.Fatalf("error does not name the missing secret: %v", err)
	}
}

func TestCLIRunner_ExecRejectsEmptyHostWorkspaceFolder(t *testing.T) {
	// Per PR 4.1 finding M4: silent fallback to /workspaces/default
	// would target the wrong tenant. Explicit error instead. Pre-fix
	// refs that only set RemoteWorkspaceFolder are also rejected —
	// passing the container-internal path to --workspace-folder is
	// the smoke-#13 probe-exit-1 bug.
	r := NewCLIRunner()
	_, err := r.Exec(t.Context(), ContainerRef{ContainerID: "c1"}, "echo hi")
	if err == nil {
		t.Fatalf("expected error on empty HostWorkspaceFolder")
	}
	if !strings.Contains(err.Error(), "HostWorkspaceFolder") {
		t.Fatalf("error does not mention HostWorkspaceFolder: %v", err)
	}
}

func TestCLIRunner_ExecRejectsRemoteOnlyRef(t *testing.T) {
	// Defensive: pre-fix refs may set only RemoteWorkspaceFolder.
	// Reject loudly rather than fall back to the field that caused
	// the smoke-#13 silent failure (probes exit 1 because
	// devcontainer-cli can't find the container by the
	// container-internal path).
	r := NewCLIRunner()
	_, err := r.Exec(t.Context(),
		ContainerRef{ContainerID: "c1", RemoteWorkspaceFolder: "/workspaces/x"},
		"echo hi")
	if err == nil {
		t.Fatalf("expected error on RemoteWorkspaceFolder-only ref")
	}
	if !strings.Contains(err.Error(), "HostWorkspaceFolder") {
		t.Fatalf("error does not mention HostWorkspaceFolder: %v", err)
	}
}

func TestCLIRunner_ExecRejectsRelativeHostWorkspace(t *testing.T) {
	// Mirror Up's absolute-path discipline. A relative
	// HostWorkspaceFolder surfaces as the same opaque "config not
	// found" devcontainer-cli error that the wsf split fixed; reject
	// at the boundary instead.
	r := NewCLIRunner()
	_, err := r.Exec(t.Context(),
		ContainerRef{ContainerID: "c1", HostWorkspaceFolder: "relative/path"},
		"echo hi")
	if err == nil {
		t.Fatalf("expected error on relative HostWorkspaceFolder")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("error does not name the absolute-path rule: %v", err)
	}
}

func TestTruncateLine(t *testing.T) {
	short := truncateLine(strings.Repeat("a", 100))
	if strings.HasSuffix(short, "…") {
		t.Fatalf("short string truncated when it shouldn't be")
	}
	long := truncateLine(strings.Repeat("a", 700))
	if !strings.HasSuffix(long, "…") {
		t.Fatalf("long string not truncated")
	}
}
