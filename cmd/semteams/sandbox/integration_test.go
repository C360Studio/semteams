package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/processor/agentic-tools/executors"
)

// TestUpstreamBashExecutorReachesSandbox is the contract test that
// pins the wire-format match between upstream's BashExecutor (which
// uses runner.Client, renamed from sandbox.Client in semstreams
// beta.91 / PR #184) and our sandbox.Server. If upstream changes
// the wire format and this test still passes, we're still aligned;
// if it stops passing, our R3.6.1.1 conformance has drifted.
//
// End-to-end without Docker:
//   - Wraps our sandbox.Server in an httptest.Server
//   - Constructs upstream's BashExecutor pointing at that URL
//   - Invokes a bash tool call ("echo hello")
//   - Asserts the tool result includes the expected stdout
//
// The exec runs locally on the test host (sandbox.Server's execCommand
// uses os/exec); no toolchain image required for this test.
func TestUpstreamBashExecutorReachesSandbox(t *testing.T) {
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
	t.Cleanup(func() { _ = chmodTreeWritable(workspaceRoot) })

	// Construct upstream's BashExecutor with the test server as the
	// sandbox URL. Working dir is irrelevant when sandbox is set —
	// commands run inside the sandbox's workspace.
	bash := executors.NewBashExecutor("/", ts.URL)

	// Create the worktree first. Upstream's BashExecutor doesn't
	// auto-create; the framework's rule layer (R3.6.2.d) will fire
	// CreateWorktree on builder spawn. For the contract test we
	// drive it manually via a direct HTTP call to our sandbox to
	// match the production sequence.
	createWorktreeForBashTest(t, ts, "test.task.bash.contract")

	// This test bypasses the agentic-loop dispatcher, so we set
	// Metadata["task_id"] manually. In production, beta.39's
	// dispatchToolCall (handlers.go) stamps loop_id onto both
	// tc.LoopID and tc.Metadata["loop_id"] before publishing, so
	// callers naturally hit the right per-loop workspace without any
	// product-shell injection.
	result, err := bash.Execute(context.Background(), agentic.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		Metadata: map[string]any{"task_id": "test.task.bash.contract"},
		Arguments: map[string]any{
			"command": "echo hello-from-sandbox",
		},
	})
	if err != nil {
		t.Fatalf("BashExecutor.Execute returned error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool result error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "hello-from-sandbox") {
		t.Fatalf("expected stdout to contain 'hello-from-sandbox', got: %s", result.Content)
	}
}

// TestUpstreamBashExecutorReadOnlyPathsRefused exercises beta.37's
// per-call read_only_paths feature: with verify_clean=true plus a
// dirty path matching read_only_paths, the bash tool refuses the
// command before running it. Pins the contract that beta.37's bash
// flags interoperate with our sandbox's git-init'd workspace.
func TestUpstreamBashExecutorReadOnlyPathsRefused(t *testing.T) {
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
	t.Cleanup(func() { _ = chmodTreeWritable(workspaceRoot) })

	taskID := "test.task.bash.verify.clean"
	createWorktreeForBashTest(t, ts, taskID)
	bash := executors.NewBashExecutor("/", ts.URL)

	// Step 1: dirty the tree by creating a file in baseline/ via bash.
	// Initial commit was empty; this creates uncommitted changes.
	r1, err := bash.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-seed",
		Name:      "bash",
		Metadata:  map[string]any{"task_id": taskID},
		Arguments: map[string]any{"command": "mkdir -p baseline && echo content > baseline/x.txt"},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if r1.Error != "" {
		t.Fatalf("seed result error: %s", r1.Error)
	}

	// Step 2: invoke bash with verify_clean=true and read_only_paths
	// matching the dirty entry. Must refuse before running.
	r2, err := bash.Execute(context.Background(), agentic.ToolCall{
		ID:       "call-iter",
		Name:     "bash",
		Metadata: map[string]any{"task_id": taskID},
		Arguments: map[string]any{
			"command":         "echo iteration-attempt",
			"verify_clean":    true,
			"read_only_paths": []any{"baseline/"},
		},
	})
	if err != nil {
		t.Fatalf("verify_clean call: %v", err)
	}
	// Two separate assertions so a refactor of upstream's error wording
	// fails on the substring check (recoverable) rather than masking a
	// "no error returned at all" regression (silent passing).
	if r2.Error == "" {
		t.Fatalf("expected verify_clean to refuse the command; got success: %s", r2.Content)
	}
	// Substring lock-in is the same one upstream's own bash_test.go uses
	// (executors/bash_test.go:434). If upstream tweaks the wording, this
	// fails loudly; update the substring rather than the assertion shape.
	if !strings.Contains(strings.ToLower(r2.Error), "verify_clean") {
		t.Fatalf("error should mention verify_clean: %q", r2.Error)
	}
}

// createWorktreeForBashTest is local to integration_test.go to avoid
// import cycles in server_test.go's helpers (which take *testing.T from
// the same package; works fine here too, just duplicated for clarity).
func createWorktreeForBashTest(t *testing.T, ts *httptest.Server, taskID string) {
	t.Helper()
	body := strings.NewReader(`{"task_id":"` + taskID + `"}`)
	resp, err := http.Post(ts.URL+"/worktree", "application/json", body)
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create worktree status %d", resp.StatusCode)
	}
}
