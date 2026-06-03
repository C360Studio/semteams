package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/processor/agentic-tools/runner"
)

// fakeExecer captures the (taskID, command, timeoutMs) tuple from a
// single Exec call so tests can assert the bash command shape the
// sandbox /exec endpoint would receive. Drives all three branches:
// happy-path, timeout, and non-zero exit.
type fakeExecer struct {
	taskID    string
	command   string
	timeoutMs int
	res       *runner.ExecResult
	err       error
}

func (f *fakeExecer) Exec(_ context.Context, taskID, command string, timeoutMs int) (*runner.ExecResult, error) {
	f.taskID = taskID
	f.command = command
	f.timeoutMs = timeoutMs
	if f.err != nil {
		return nil, f.err
	}
	if f.res != nil {
		return f.res, nil
	}
	return &runner.ExecResult{ExitCode: 0}, nil
}

// TestSandboxArtifactWriter_HappyPath verifies the writer composes a
// mkdir + printf | base64 -d command targeting the absolute path under
// hostWsf. The base64-encoded payload must decode back to the original
// content, and the abs path components must be present as
// single-quoted tokens (so a wsf containing spaces or shell metachars
// round-trips safely).
func TestSandboxArtifactWriter_HappyPath(t *testing.T) {
	exec := &fakeExecer{}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	content := []byte("# Autoresearch\n\nbaseline: 1.04s\nbest: 0.10s\n")
	err := w.WriteFile(context.Background(),
		"/var/lib/semteams-tenants/sigxyz/workspace",
		".artifacts/autoresearch/2026-06-03-foo.md",
		content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.taskID != sandboxWriterTaskID {
		t.Errorf("taskID = %q, want %q", exec.taskID, sandboxWriterTaskID)
	}
	if exec.timeoutMs != sandboxWriterTimeoutMs {
		t.Errorf("timeoutMs = %d, want %d", exec.timeoutMs, sandboxWriterTimeoutMs)
	}
	wantDir := "'/var/lib/semteams-tenants/sigxyz/workspace/.artifacts/autoresearch'"
	wantAbs := "'/var/lib/semteams-tenants/sigxyz/workspace/.artifacts/autoresearch/2026-06-03-foo.md'"
	if !strings.Contains(exec.command, "mkdir -p "+wantDir) {
		t.Errorf("command missing mkdir token %q; got:\n%s", wantDir, exec.command)
	}
	if !strings.Contains(exec.command, "| base64 -d > "+wantAbs) {
		t.Errorf("command missing redirect to %q; got:\n%s", wantAbs, exec.command)
	}
	// Extract the base64 payload between the printf single quotes and
	// verify it round-trips back to content.
	const marker = "printf '%s' '"
	i := strings.Index(exec.command, marker)
	if i < 0 {
		t.Fatalf("command missing printf token; got:\n%s", exec.command)
	}
	rest := exec.command[i+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		t.Fatalf("command missing closing quote on base64 payload; got:\n%s", exec.command)
	}
	decoded, err := base64.StdEncoding.DecodeString(rest[:end])
	if err != nil {
		t.Fatalf("payload not valid base64: %v", err)
	}
	if string(decoded) != string(content) {
		t.Errorf("decoded payload mismatch\nwant: %q\ngot:  %q", content, decoded)
	}
}

// TestSandboxArtifactWriter_HostWsfWithTrailingSlash asserts the
// concat handles wsf=".../workspace/" (trailing slash) without
// producing a double-slash in the abs path. Defensive: callers might
// stamp either form.
func TestSandboxArtifactWriter_HostWsfWithTrailingSlash(t *testing.T) {
	exec := &fakeExecer{}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	err := w.WriteFile(context.Background(), "/wsf/", ".artifacts/foo.md", []byte("x"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(exec.command, "//") {
		t.Errorf("command contains double-slash; got:\n%s", exec.command)
	}
}

// TestSandboxArtifactWriter_Timeout surfaces the TimedOut flag as a
// distinct error message — TimedOut=true with ExitCode=0 would
// otherwise look like success.
func TestSandboxArtifactWriter_Timeout(t *testing.T) {
	exec := &fakeExecer{res: &runner.ExecResult{TimedOut: true}}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	err := w.WriteFile(context.Background(), "/wsf", ".artifacts/f.md", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error; got %v", err)
	}
}

// TestSandboxArtifactWriter_NonZeroExit surfaces a non-zero exit as
// an error with stderr embedded for debuggability.
func TestSandboxArtifactWriter_NonZeroExit(t *testing.T) {
	exec := &fakeExecer{res: &runner.ExecResult{ExitCode: 1, Stderr: "permission denied"}}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	err := w.WriteFile(context.Background(), "/wsf", ".artifacts/f.md", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "exit 1") || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected exit-1 error with stderr; got %v", err)
	}
}

// TestSandboxArtifactWriter_ExecErrorPropagates surfaces a transport
// error (network down, sandbox unreachable) as a wrapped error.
func TestSandboxArtifactWriter_ExecErrorPropagates(t *testing.T) {
	exec := &fakeExecer{err: errors.New("connection refused")}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	err := w.WriteFile(context.Background(), "/wsf", ".artifacts/f.md", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected wrapped transport error; got %v", err)
	}
}

// TestSandboxArtifactWriter_RejectsEmptyInputs guards against callers
// accidentally passing empty wsf or relPath — would otherwise produce
// `mkdir -p ”` (mkdir error) or `> ”` (write to cwd as empty path).
// Fail-fast at the API boundary.
func TestSandboxArtifactWriter_RejectsEmptyInputs(t *testing.T) {
	exec := &fakeExecer{}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	if err := w.WriteFile(context.Background(), "", "rel.md", []byte("x")); err == nil {
		t.Error("expected error on empty hostWsf")
	}
	if err := w.WriteFile(context.Background(), "/wsf", "", []byte("x")); err == nil {
		t.Error("expected error on empty relPath")
	}
}

// TestSandboxArtifactWriter_RejectsOversize guards against a future
// caller stamping a multi-MB artifact. Linux argv has a ~128KB cap;
// printf with a giant base64 arg would silently truncate or fail
// with E2BIG, which is harder to debug than a typed cap error.
func TestSandboxArtifactWriter_RejectsOversize(t *testing.T) {
	exec := &fakeExecer{}
	w := &sandboxArtifactWriter{execer: exec, logger: slog.Default()}
	huge := make([]byte, sandboxWriterMaxContentBytes+1)
	err := w.WriteFile(context.Background(), "/wsf", "rel.md", huge)
	if err == nil || !strings.Contains(err.Error(), "exceeds cap") {
		t.Errorf("expected size-cap error; got %v", err)
	}
}

// TestShellSingleQuote pins the escape semantics: outer single quotes
// always; embedded single quotes become the canonical
// `'closing backslash-quote opening'` pattern.
func TestShellSingleQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "''"},
		{"plain", "'plain'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"$evil; rm -rf /", "'$evil; rm -rf /'"},
		{"`backtick`", "'`backtick`'"},
	}
	for _, c := range cases {
		if got := shellSingleQuote(c.in); got != c.want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
