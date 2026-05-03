package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:      workspaceRoot,
		MaxFileSize:        1 * 1024 * 1024,
		MaxReadBytes:       256 * 1024,
		MaxOutputBytes:     64 * 1024,
		DefaultExecTimeout: 5 * time.Second,
		MaxExecTimeout:     10 * time.Second,
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	t.Cleanup(func() {
		// Workspaces may have read-only subtrees from read_only_paths
		// tests; t.TempDir's auto-cleanup uses os.RemoveAll which fails
		// against 555/444. chmod everything writable first.
		_ = chmodTreeWritable(workspaceRoot)
	})
	return ts, workspaceRoot
}

// =============================================================================
// HELPERS — each maps to one upstream sandbox.Client method shape so a
// future integration test can swap the helpers for sandbox.Client calls
// without touching the assertions.
// =============================================================================

func createWorktree(t *testing.T, ts *httptest.Server, taskID string, readOnlyPaths []string) *http.Response {
	t.Helper()
	body := struct {
		TaskID        string   `json:"task_id"`
		ReadOnlyPaths []string `json:"read_only_paths,omitempty"`
	}{TaskID: taskID, ReadOnlyPaths: readOnlyPaths}
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+"/worktree", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST /worktree: %v", err)
	}
	return resp
}

func createWorktreeForTest(t *testing.T, ts *httptest.Server, taskID string) {
	t.Helper()
	r := createWorktree(t, ts, taskID, nil)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("create worktree %q: status %d", taskID, r.StatusCode)
	}
}

func deleteWorktree(t *testing.T, ts *httptest.Server, taskID string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/worktree/"+taskID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /worktree/%s: %v", taskID, err)
	}
	return resp
}

func writeFile(t *testing.T, ts *httptest.Server, taskID, path, content string) *http.Response {
	t.Helper()
	body := struct {
		TaskID  string `json:"task_id"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}{TaskID: taskID, Path: path, Content: content}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/file", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /file: %v", err)
	}
	return resp
}

func readFile(t *testing.T, ts *httptest.Server, taskID, path string) *http.Response {
	t.Helper()
	q := url.Values{"task_id": {taskID}, "path": {path}}
	resp, err := http.Get(ts.URL + "/file?" + q.Encode())
	if err != nil {
		t.Fatalf("GET /file: %v", err)
	}
	return resp
}

func postExec(t *testing.T, ts *httptest.Server, taskID, command string, timeoutMs int) *http.Response {
	t.Helper()
	body := struct {
		TaskID    string `json:"task_id"`
		Command   string `json:"command"`
		TimeoutMs int    `json:"timeout_ms,omitempty"`
	}{TaskID: taskID, Command: command, TimeoutMs: timeoutMs}
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+"/exec", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST /exec: %v", err)
	}
	return resp
}

func archiveWorktree(t *testing.T, ts *httptest.Server, taskID string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + "/worktree/" + taskID + "/archive")
	if err != nil {
		t.Fatalf("GET /worktree/%s/archive: %v", taskID, err)
	}
	return resp
}

// =============================================================================
// WORKSPACE LIFECYCLE
// =============================================================================

func TestHealth(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCreateWorktreeReturnsUpstreamShape(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := createWorktree(t, ts, "test.task.001", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var got worktreeInfo
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "created" {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Branch != defaultBranch {
		t.Fatalf("branch = %q, want %q", got.Branch, defaultBranch)
	}
	if !strings.HasSuffix(got.Path, "test.task.001") {
		t.Fatalf("path = %q, want suffix test.task.001", got.Path)
	}
}

func TestCreateWorktreeIdempotent(t *testing.T) {
	ts, _ := newTestServer(t)
	r1 := createWorktree(t, ts, "test.task.idempotent", nil)
	r1.Body.Close()
	r2 := createWorktree(t, ts, "test.task.idempotent", nil)
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("status %d", r2.StatusCode)
	}
	var got worktreeInfo
	json.NewDecoder(r2.Body).Decode(&got)
	if got.Status != "exists" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestCreateWorktreeInitializesGit(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	createWorktreeForTest(t, ts, "test.task.git")

	dir := filepath.Join(workspaceRoot, "test.task.git")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected .git dir: %v", err)
	}
	// Initial commit exists; status --porcelain is empty.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected clean status, got: %s", out)
	}
	// Branch is the default we configured.
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != defaultBranch {
		t.Fatalf("branch = %q, want %q", got, defaultBranch)
	}
}

func TestCreateWorktreeInvalidTaskID(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := createWorktree(t, ts, "with..dotdot", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateWorktreeRejectsInvalidReadOnlyPath(t *testing.T) {
	ts, _ := newTestServer(t)
	cases := []struct {
		name string
		path string
	}{
		{"absolute", "/etc"},
		{"traversal", "../escape"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := createWorktree(t, ts, "test.task.ro."+tc.name, []string{tc.path})
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestDeleteWorktree(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.delete"
	createWorktreeForTest(t, ts, taskID)

	resp := deleteWorktree(t, ts, taskID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, taskID)); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed; stat err: %v", err)
	}
}

func TestDeleteWorktreeMissing(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := deleteWorktree(t, ts, "test.task.never.created")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 idempotent, got %d", resp.StatusCode)
	}
}

func TestDeleteWorktreeWithReadOnlyPaths(t *testing.T) {
	// DELETE must succeed even when read_only_paths chmod'd things to
	// 555/444 — chmodTreeWritable runs first.
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.delete.ro"
	r := createWorktree(t, ts, taskID, []string{"frozen"})
	r.Body.Close()
	// Write a file via bash (which triggers chmod sweep on completion).
	exec := postExec(t, ts, taskID, "mkdir frozen && echo content > frozen/file.txt", 0)
	exec.Body.Close()
	// Verify it's actually frozen.
	if err := os.WriteFile(filepath.Join(workspaceRoot, taskID, "frozen/file.txt"), []byte("x"), 0o644); err == nil {
		t.Fatalf("frozen file should be chmod 444 — write succeeded unexpectedly")
	}
	// Delete should still succeed.
	resp := deleteWorktree(t, ts, taskID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("delete status %d: %s", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, taskID)); !os.IsNotExist(err) {
		t.Fatalf("worktree not removed: %v", err)
	}
}

// =============================================================================
// FILE OPS
// =============================================================================

func TestWriteFileSuccess(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.write"
	createWorktreeForTest(t, ts, taskID)

	resp := writeFile(t, ts, taskID, "main.go", "package main")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	got, err := os.ReadFile(filepath.Join(workspaceRoot, taskID, "main.go"))
	if err != nil {
		t.Fatalf("read on disk: %v", err)
	}
	if string(got) != "package main" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteFileCreatesParentDirs(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.subdir"
	createWorktreeForTest(t, ts, taskID)

	resp := writeFile(t, ts, taskID, "src/main/java/Foo.java", "public class Foo {}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, taskID, "src/main/java/Foo.java")); err != nil {
		t.Fatalf("expected file: %v", err)
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.overwrite"
	createWorktreeForTest(t, ts, taskID)

	r1 := writeFile(t, ts, taskID, "f.txt", "first")
	r1.Body.Close()
	r2 := writeFile(t, ts, taskID, "f.txt", "second")
	r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("overwrite status %d", r2.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(workspaceRoot, taskID, "f.txt"))
	if string(got) != "second" {
		t.Fatalf("content = %q", got)
	}
}

func TestReadFileRoundtrip(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.roundtrip"
	createWorktreeForTest(t, ts, taskID)

	w := writeFile(t, ts, taskID, "hi.txt", "hello world")
	w.Body.Close()
	r := readFile(t, ts, taskID, "hi.txt")
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("status %d", r.StatusCode)
	}
	var got fileReadResponse
	json.NewDecoder(r.Body).Decode(&got)
	if got.Content != "hello world" {
		t.Fatalf("content = %q", got.Content)
	}
}

func TestReadFileTruncation(t *testing.T) {
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:  workspaceRoot,
		MaxFileSize:    1 * 1024 * 1024,
		MaxReadBytes:   8,
		MaxOutputBytes: 64 * 1024,
		Logger:         logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	taskID := "test.task.truncate"
	createWorktreeForTest(t, ts, taskID)
	w := writeFile(t, ts, taskID, "big.txt", "this is way more than 8 bytes")
	w.Body.Close()
	r := readFile(t, ts, taskID, "big.txt")
	defer r.Body.Close()
	var got fileReadResponse
	json.NewDecoder(r.Body).Decode(&got)
	if !got.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if got.Size != 8 {
		t.Fatalf("size = %d", got.Size)
	}
	if got.Content != "this is " {
		t.Fatalf("content = %q", got.Content)
	}
}

func TestReadFileMissing(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.read.missing"
	createWorktreeForTest(t, ts, taskID)
	r := readFile(t, ts, taskID, "nothing.txt")
	defer r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", r.StatusCode)
	}
}

func TestWriteFileMissingPath(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.empty.path"
	createWorktreeForTest(t, ts, taskID)
	resp := writeFile(t, ts, taskID, "", "x")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestWriteFileOversized(t *testing.T) {
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:  workspaceRoot,
		MaxFileSize:    16,
		MaxReadBytes:   256 * 1024,
		MaxOutputBytes: 64 * 1024,
		Logger:         logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	taskID := "test.task.oversize"
	createWorktreeForTest(t, ts, taskID)
	resp := writeFile(t, ts, taskID, "f.txt", "this content is more than sixteen bytes long")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

func TestWriteFileMissingWorktree(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := writeFile(t, ts, "test.task.no.worktree", "f.txt", "x")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestReadFileMissingWorktree(t *testing.T) {
	ts, _ := newTestServer(t)
	r := readFile(t, ts, "test.task.no.worktree", "f.txt")
	defer r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", r.StatusCode)
	}
}

// =============================================================================
// PATH CONFINEMENT
// =============================================================================

func TestWriteFileAbsolutePath(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.abs"
	createWorktreeForTest(t, ts, taskID)
	resp := writeFile(t, ts, taskID, "/etc/passwd", "x")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestWriteFileTraversal(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.traversal"
	createWorktreeForTest(t, ts, taskID)
	resp := writeFile(t, ts, taskID, "../../../etc/passwd", "x")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "..", "..", "..", "etc", "passwd-fake")); err == nil {
		t.Fatalf("traversal escaped — file written outside workspace")
	}
}

func TestReadFileAbsolutePath(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.read.abs"
	createWorktreeForTest(t, ts, taskID)
	r := readFile(t, ts, taskID, "/etc/passwd")
	defer r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", r.StatusCode)
	}
}

func TestReadFileTraversal(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.read.traversal"
	createWorktreeForTest(t, ts, taskID)
	r := readFile(t, ts, taskID, "../../../etc/passwd")
	defer r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", r.StatusCode)
	}
}

func TestReadFileSymlinkRefused(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.symlink.read"
	createWorktreeForTest(t, ts, taskID)

	outside := filepath.Join(filepath.Dir(workspaceRoot), "outside.target")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	t.Cleanup(func() { os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(workspaceRoot, taskID, "leak")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := readFile(t, ts, taskID, "leak")
	defer r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", r.StatusCode)
	}
}

func TestWriteFileRefusesExistingSymlink(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.write.symlink"
	createWorktreeForTest(t, ts, taskID)

	dir := filepath.Join(workspaceRoot, taskID)
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("real"), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(dir, "alias.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resp := writeFile(t, ts, taskID, "alias.txt", "tampered")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "real.txt"))
	if string(got) != "real" {
		t.Fatalf("real file clobbered: %q", got)
	}
}

func TestReadFileIsDirectory(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.read.dir"
	createWorktreeForTest(t, ts, taskID)
	if err := os.MkdirAll(filepath.Join(workspaceRoot, taskID, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	r := readFile(t, ts, taskID, "subdir")
	defer r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", r.StatusCode)
	}
}

// =============================================================================
// EXEC
// =============================================================================

func TestExecEcho(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.exec.echo"
	createWorktreeForTest(t, ts, taskID)
	resp := postExec(t, ts, taskID, "echo hello", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.ExitCode != 0 {
		t.Fatalf("exit %d", got.ExitCode)
	}
	if !strings.Contains(got.Stdout, "hello") {
		t.Fatalf("stdout = %q", got.Stdout)
	}
}

func TestExecExitCodePropagated(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.exec.exit"
	createWorktreeForTest(t, ts, taskID)
	resp := postExec(t, ts, taskID, "exit 7", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.ExitCode != 7 {
		t.Fatalf("exit %d", got.ExitCode)
	}
}

func TestExecStderrCaptured(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.exec.stderr"
	createWorktreeForTest(t, ts, taskID)
	resp := postExec(t, ts, taskID, "echo oops 1>&2", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if !strings.Contains(got.Stderr, "oops") {
		t.Fatalf("stderr = %q", got.Stderr)
	}
}

func TestExecChdirToWorkspace(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.exec.pwd"
	createWorktreeForTest(t, ts, taskID)
	resp := postExec(t, ts, taskID, "pwd", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	expected := filepath.Join(workspaceRoot, taskID)
	resolved, _ := filepath.EvalSymlinks(expected)
	if !strings.Contains(got.Stdout, resolved) && !strings.Contains(got.Stdout, expected) {
		t.Fatalf("pwd = %q, want workspace path %q", got.Stdout, expected)
	}
}

func TestExecTimeoutKillsProcess(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.exec.timeout"
	createWorktreeForTest(t, ts, taskID)

	start := time.Now()
	resp := postExec(t, ts, taskID, "sleep 30", 100)
	defer resp.Body.Close()
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout slow; elapsed %v", elapsed)
	}
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if !got.TimedOut {
		t.Fatalf("expected timed_out=true")
	}
}

func TestExecOutputTruncation(t *testing.T) {
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:      workspaceRoot,
		MaxFileSize:        1024,
		MaxReadBytes:       1024,
		MaxOutputBytes:     16,
		DefaultExecTimeout: 2 * time.Second,
		MaxExecTimeout:     10 * time.Second,
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	taskID := "test.task.exec.trunc"
	createWorktreeForTest(t, ts, taskID)
	resp := postExec(t, ts, taskID, "printf 'this is a long string that exceeds 16 bytes'", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Stdout) > 16 {
		t.Fatalf("expected stdout truncated to 16 bytes, got %d (%q)", len(got.Stdout), got.Stdout)
	}
}

func TestExecMissingWorktree(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := postExec(t, ts, "test.task.no.worktree", "echo x", 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestExecMissingCommand(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.exec.empty"
	createWorktreeForTest(t, ts, taskID)
	resp := postExec(t, ts, taskID, "", 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestExecTimeoutCappedByMax(t *testing.T) {
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:      workspaceRoot,
		MaxFileSize:        1024,
		MaxReadBytes:       1024,
		MaxOutputBytes:     1024,
		DefaultExecTimeout: 1 * time.Second,
		MaxExecTimeout:     200 * time.Millisecond,
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	taskID := "test.task.exec.cap"
	createWorktreeForTest(t, ts, taskID)

	start := time.Now()
	resp := postExec(t, ts, taskID, "sleep 5", 30000)
	defer resp.Body.Close()
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("MaxExecTimeout cap not honored; elapsed %v", elapsed)
	}
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if !got.TimedOut {
		t.Fatalf("expected timed_out=true")
	}
}

// =============================================================================
// READ_ONLY_PATHS — R3.6.1.1 new behavior
// =============================================================================

func TestReadOnlyPaths_FreezeAfterSeed(t *testing.T) {
	// The autoresearch flow: create with read_only_paths, seed via bash,
	// verify subsequent writes fail with EACCES → 403.
	ts, _ := newTestServer(t)
	taskID := "test.task.ro.freeze"
	r := createWorktree(t, ts, taskID, []string{"baseline"})
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("create: %d", r.StatusCode)
	}

	// Seed via bash; chmod sweep applied after exec.
	seed := postExec(t, ts, taskID, "mkdir baseline && echo content > baseline/foo.txt", 0)
	defer seed.Body.Close()
	var seedResult execResponse
	json.NewDecoder(seed.Body).Decode(&seedResult)
	if seedResult.ExitCode != 0 {
		t.Fatalf("seed exit %d, stderr %q", seedResult.ExitCode, seedResult.Stderr)
	}

	// Subsequent write_file to baseline/* must be refused with 403.
	resp := writeFile(t, ts, taskID, "baseline/sneak.py", "evil")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 403 (read-only), got %d: %s", resp.StatusCode, body)
	}

	// Subsequent bash write attempt against an existing read-only file
	// returns non-zero (EACCES from kernel).
	r2 := postExec(t, ts, taskID, "echo tampered > baseline/foo.txt", 0)
	defer r2.Body.Close()
	var r2Result execResponse
	json.NewDecoder(r2.Body).Decode(&r2Result)
	if r2Result.ExitCode == 0 {
		t.Fatalf("expected non-zero exit when writing to frozen file, got 0; stdout=%q stderr=%q",
			r2Result.Stdout, r2Result.Stderr)
	}
}

func TestReadOnlyPaths_NonFrozenPathsStillWritable(t *testing.T) {
	// Files outside read_only_paths must remain writable.
	ts, _ := newTestServer(t)
	taskID := "test.task.ro.unfrozen"
	r := createWorktree(t, ts, taskID, []string{"baseline"})
	r.Body.Close()

	seed := postExec(t, ts, taskID, "mkdir baseline output && echo seed > baseline/x.txt && echo o > output/y.txt", 0)
	seed.Body.Close()

	// output/* is not in read_only_paths; subsequent write should succeed.
	resp := writeFile(t, ts, taskID, "output/iter.txt", "iteration content")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestReadOnlyPaths_NoMetadataNoOp(t *testing.T) {
	// Without read_only_paths, no chmod sweep; everything writable.
	ts, _ := newTestServer(t)
	taskID := "test.task.ro.none"
	createWorktreeForTest(t, ts, taskID)

	seed := postExec(t, ts, taskID, "mkdir baseline && echo content > baseline/foo.txt", 0)
	seed.Body.Close()

	resp := writeFile(t, ts, taskID, "baseline/foo.txt", "rewrite ok")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestReadOnlyPaths_FreezeOnCreate(t *testing.T) {
	// If read_only_paths exist at create time (pre-populated workspace
	// dir from a prior run), the chmod sweep applies immediately.
	ts, workspaceRoot := newTestServer(t)
	taskID := "test.task.ro.precreate"

	// Pre-populate the workspace dir before the create call. Simulates
	// the case where a workspace volume has prior content.
	dir := filepath.Join(workspaceRoot, taskID)
	if err := os.MkdirAll(filepath.Join(dir, "baseline"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "baseline/x.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Hack: we need git init to NOT run because the dir exists. But our
	// createWorkspace() detects existing dir and returns "exists"
	// without git-initing. So the create call below treats it as
	// existing. Subtle.
	r := createWorktree(t, ts, taskID, []string{"baseline"})
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("create status %d", r.StatusCode)
	}

	// baseline/x.txt should be chmod 444 immediately.
	info, err := os.Stat(filepath.Join(dir, "baseline/x.txt"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o444 {
		t.Fatalf("expected mode 0444, got %o", mode)
	}
}

// =============================================================================
// ARCHIVE
// =============================================================================

func TestArchiveWorktreeEmpty(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.archive.empty"
	createWorktreeForTest(t, ts, taskID)
	resp := archiveWorktree(t, ts, taskID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content-type = %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	// Worktree contains .git/ but no other files. The zip walks the
	// worktree (skipping symlinks); .git/* will be present.
	hasGit := false
	for _, zf := range zr.File {
		if strings.HasPrefix(zf.Name, ".git/") || zf.Name == ".git/" {
			hasGit = true
			break
		}
	}
	if !hasGit {
		t.Fatalf("expected .git/ in zip; entries: %v", zipNames(zr.File))
	}
}

func TestArchiveWorktreeWithFiles(t *testing.T) {
	ts, _ := newTestServer(t)
	taskID := "test.task.archive.full"
	createWorktreeForTest(t, ts, taskID)
	w := writeFile(t, ts, taskID, "src/main.go", "package main")
	w.Body.Close()

	resp := archiveWorktree(t, ts, taskID)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	found := false
	for _, zf := range zr.File {
		if zf.Name == "src/main.go" {
			rc, _ := zf.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			if string(b) != "package main" {
				t.Fatalf("zip content = %q", b)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected src/main.go in zip; entries: %v", zipNames(zr.File))
	}
}

func TestArchiveWorktreeMissing(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := archiveWorktree(t, ts, "test.task.archive.never")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func zipNames(files []*zip.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

// =============================================================================
// CONCURRENCY + VALIDATORS
// =============================================================================

func TestConcurrentTasksIndependent(t *testing.T) {
	ts, _ := newTestServer(t)
	const taskA = "test.task.concurrent.a"
	const taskB = "test.task.concurrent.b"
	createWorktreeForTest(t, ts, taskA)
	createWorktreeForTest(t, ts, taskB)

	const iters = 20
	done := make(chan error, 2)
	hammer := func(taskID string) {
		var lastErr error
		for i := range iters {
			path := "f" + strconv.Itoa(i) + ".txt"
			content := taskID + "-iter-" + strconv.Itoa(i)
			r := writeFile(t, ts, taskID, path, content)
			r.Body.Close()
			if r.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("%s write %d: status %d", taskID, i, r.StatusCode)
				continue
			}
			rd := readFile(t, ts, taskID, path)
			var got fileReadResponse
			json.NewDecoder(rd.Body).Decode(&got)
			rd.Body.Close()
			if got.Content != content {
				lastErr = fmt.Errorf("%s read %d: cross-task leak — want %q got %q",
					taskID, i, content, got.Content)
			}
		}
		done <- lastErr
	}
	go hammer(taskA)
	go hammer(taskB)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent task hammer: %v", err)
		}
	}
}

func TestIsValidTaskID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"a", true},
		{"c360.prod.app.task.abc123", true},
		{"task-with-hyphens", true},
		{"task_with_underscores", true},
		{"with/slash", false},
		{"with space", false},
		{"with..traversal", false},
		{".", false},
		{"..", false},
		{"with$dollar", false},
		{strings.Repeat("a", 256), true},
		{strings.Repeat("a", 257), false},
	}
	for _, tc := range cases {
		if got := isValidTaskID(tc.id); got != tc.want {
			t.Errorf("isValidTaskID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestValidateReadOnlyPath(t *testing.T) {
	cases := []struct {
		path string
		ok   bool
	}{
		{"", false},
		{"baseline", true},
		{"src/main", true},
		{"/etc", false},
		{"../escape", false},
		{".", true}, // odd but cleans to "."
	}
	for _, tc := range cases {
		err := validateReadOnlyPath(tc.path)
		if tc.ok && err != nil {
			t.Errorf("validateReadOnlyPath(%q) = %v, want nil", tc.path, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("validateReadOnlyPath(%q) = nil, want error", tc.path)
		}
	}
}

func TestResolveTaskPath(t *testing.T) {
	root := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot: root,
		MaxFileSize:   1024,
		MaxReadBytes:  1024,
		Logger:        logger,
	})
	taskID := "test.resolve"
	if err := os.MkdirAll(filepath.Join(root, taskID), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"simple", "main.go", false},
		{"subdir", "src/main.go", false},
		{"absolute", "/etc/passwd", true},
		{"traversal", "../../etc/passwd", true},
		{"clean-traversal", "src/../../../etc/passwd", true},
		{"current-dir", ".", false},
		{"trailing-slash", "subdir/", false},
		{"unicode-traversal", "日本語/../../../etc/passwd", true},
		{"unicode-name", "src/日本語.txt", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := srv.resolveTaskPath(taskID, tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveTaskPathRejectsParentSymlink(t *testing.T) {
	root := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot: root,
		MaxFileSize:   1024,
		MaxReadBytes:  1024,
		Logger:        logger,
	})
	taskID := "test.task.parent.symlink"
	taskDir := filepath.Join(root, taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside.target")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(outside) })
	if err := os.Symlink(outside, filepath.Join(taskDir, "evil-dir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	_, err := srv.resolveTaskPath(taskID, "evil-dir/anything.txt")
	if err == nil {
		t.Fatalf("expected error for parent-symlink path")
	}
}
