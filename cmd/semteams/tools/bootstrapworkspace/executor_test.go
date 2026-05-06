package bootstrapworkspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/processor/agentic-tools/sandbox"
)

// ---------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------

// writeCall records one sandbox.WriteFile invocation.
type writeCall struct {
	taskID  string
	path    string
	content string
}

// fakeSandbox records CreateWorktree + WriteFile calls. Test sets
// createErr / writeErr to drive failure paths.
//
// writeHistory records every WriteFile call in order. The legacy scalar
// fields (writeTaskID, writePath, writeContent, writeCalls) track the
// LAST call for backward-compat with pre-sidecar tests; new tests read
// writeHistory directly.
type fakeSandbox struct {
	mu sync.Mutex

	createTaskID  string
	createOpts    sandbox.CreateWorktreeOptions
	createCalls   int
	createErr     error
	createReturns sandbox.WorktreeInfo

	writeTaskID  string
	writePath    string
	writeContent string
	writeCalls   int
	writeHistory []writeCall
	writeErr     error
}

func (f *fakeSandbox) CreateWorktree(_ context.Context, taskID string, opts sandbox.CreateWorktreeOptions) (*sandbox.WorktreeInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	f.createTaskID = taskID
	f.createOpts = opts
	if f.createErr != nil {
		return nil, f.createErr
	}
	info := f.createReturns
	if info.Status == "" {
		info.Status = "created"
	}
	if info.Path == "" {
		info.Path = "/workspace/" + taskID
	}
	if info.Branch == "" {
		info.Branch = "main"
	}
	return &info, nil
}

func (f *fakeSandbox) WriteFile(_ context.Context, taskID, path, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCalls++
	f.writeTaskID = taskID
	f.writePath = path
	f.writeContent = content
	f.writeHistory = append(f.writeHistory, writeCall{taskID: taskID, path: path, content: content})
	return f.writeErr
}

// writeHistorySnapshot returns a copy of writeHistory for assertion use.
func (f *fakeSandbox) writeHistorySnapshot() []writeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]writeCall, len(f.writeHistory))
	copy(out, f.writeHistory)
	return out
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

// newExecutorWithSpec writes a fixture spec to a tmp dir and constructs
// an Executor scoped to that dir. Returns the executor, the fake
// sandbox, and the absolute spec path the LLM would substitute.
func newExecutorWithSpec(t *testing.T, slug, content string) (*Executor, *fakeSandbox, string) {
	t.Helper()
	tmp := t.TempDir()
	relPath := filepath.Join(tmp, slug+".md")
	if err := os.WriteFile(relPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture spec: %v", err)
	}
	fs := &fakeSandbox{}
	exec, err := NewExecutor(fs, nil, tmp)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	return exec, fs, relPath
}

// newExecutorWithSpecAndSidecar is like newExecutorWithSpec but also writes a
// <slug>.commitments.json sidecar adjacent to the spec. commitmentsJSON must be
// valid JSON (it is written verbatim). Returns the sidecar path for assertions.
func newExecutorWithSpecAndSidecar(t *testing.T, slug, specContent, commitmentsJSON string) (*Executor, *fakeSandbox, string, string) {
	t.Helper()
	exec, fs, specPath := newExecutorWithSpec(t, slug, specContent)
	sidecarPath := commitmentsSidecarPath(specPath)
	if err := os.WriteFile(sidecarPath, []byte(commitmentsJSON), 0o644); err != nil {
		t.Fatalf("write fixture sidecar: %v", err)
	}
	return exec, fs, specPath, sidecarPath
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-001",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-builder-abc",
		TraceID:   "trace-xyz",
	}
}

// ---------------------------------------------------------------------
// ListTools — schema sanity
// ---------------------------------------------------------------------

func TestListTools_SchemaShape(t *testing.T) {
	fs := &fakeSandbox{}
	exec, err := NewExecutor(fs, nil, t.TempDir())
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	defs := exec.ListTools()
	if len(defs) != 1 {
		t.Fatalf("ListTools length = %d, want 1", len(defs))
	}
	def := defs[0]
	if def.Name != ToolName {
		t.Errorf("tool name = %q, want %q", def.Name, ToolName)
	}
	props, _ := def.Parameters["properties"].(map[string]any)
	if _, ok := props["spec_path"]; !ok {
		t.Errorf("missing spec_path in schema properties")
	}
	required, _ := def.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "spec_path" {
		t.Errorf("required = %v, want [spec_path]", required)
	}
}

// ---------------------------------------------------------------------
// Routing / preconditions
// ---------------------------------------------------------------------

func TestExecute_WrongToolName(t *testing.T) {
	exec, _, _ := newExecutorWithSpec(t, "slug", "# spec")
	call := defaultCall(map[string]any{"spec_path": "ignored"})
	call.Name = "something-else"
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestExecute_MissingLoopID(t *testing.T) {
	exec, _, specPath := newExecutorWithSpec(t, "slug", "# spec")
	call := defaultCall(map[string]any{"spec_path": specPath})
	call.LoopID = ""
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
	if !strings.Contains(res.Error, "loop_id") {
		t.Errorf("Result.Error = %q, want contains 'loop_id'", res.Error)
	}
}

func TestExecute_NilSandbox(t *testing.T) {
	tmp := t.TempDir()
	exec, err := NewExecutor(nil, nil, tmp)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": "x.md"}))
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
	if !strings.Contains(res.Error, "SANDBOX_URL") {
		t.Errorf("Result.Error = %q, want contains 'SANDBOX_URL'", res.Error)
	}
}

// ---------------------------------------------------------------------
// Argument validation
// ---------------------------------------------------------------------

func TestExecute_MissingSpecPath(t *testing.T) {
	exec, _, _ := newExecutorWithSpec(t, "slug", "# spec")
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "spec_path") {
		t.Errorf("Result.Error = %q, want contains 'spec_path'", res.Error)
	}
}

func TestExecute_EmptySpecPath(t *testing.T) {
	exec, _, _ := newExecutorWithSpec(t, "slug", "# spec")
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": "   "}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_PathTraversalRejected(t *testing.T) {
	exec, fs, _ := newExecutorWithSpec(t, "slug", "# spec")
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": "../../../etc/passwd",
	}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "outside") {
		t.Errorf("Result.Error = %q, want contains 'outside'", res.Error)
	}
	if fs.createCalls != 0 {
		t.Errorf("expected no CreateWorktree call on traversal rejection, got %d", fs.createCalls)
	}
}

func TestExecute_AbsolutePathOutsideSpecDirRejected(t *testing.T) {
	exec, fs, _ := newExecutorWithSpec(t, "slug", "# spec")
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": "/etc/passwd",
	}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if fs.createCalls != 0 {
		t.Errorf("expected no CreateWorktree call on absolute-outside rejection, got %d", fs.createCalls)
	}
}

// Spec-dir-prefix lookalikes (e.g. ".../docs/specs2/x.md" when specDir
// is ".../docs/specs") must not slip past the prefix check.
func TestExecute_PrefixLookalikeRejected(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "specs")
	if err := os.Mkdir(specDir, 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	lookalikeDir := filepath.Join(tmp, "specs2")
	if err := os.Mkdir(lookalikeDir, 0o755); err != nil {
		t.Fatalf("mkdir specs2: %v", err)
	}
	intruder := filepath.Join(lookalikeDir, "x.md")
	if err := os.WriteFile(intruder, []byte("# intruder"), 0o644); err != nil {
		t.Fatalf("write intruder: %v", err)
	}
	fs := &fakeSandbox{}
	exec, err := NewExecutor(fs, nil, specDir)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": intruder,
	}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if fs.createCalls != 0 {
		t.Errorf("expected no CreateWorktree call on lookalike rejection, got %d", fs.createCalls)
	}
}

func TestExecute_SpecFileNotFound(t *testing.T) {
	tmp := t.TempDir()
	fs := &fakeSandbox{}
	exec, err := NewExecutor(fs, nil, tmp)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": filepath.Join(tmp, "nonexistent.md"),
	}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "not found") {
		t.Errorf("Result.Error = %q, want contains 'not found'", res.Error)
	}
}

// ---------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------

func TestExecute_HappyPath(t *testing.T) {
	specBody := "# OSH Driver — Meshtastic\n\nGoal: ...\n"
	exec, fs, specPath := newExecutorWithSpec(t, "2026-05-03-osh-meshtastic-driver", specBody)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if fs.createCalls != 1 {
		t.Errorf("CreateWorktree calls = %d, want 1", fs.createCalls)
	}
	if fs.createTaskID != "loop-builder-abc" {
		t.Errorf("CreateWorktree task_id = %q, want %q", fs.createTaskID, "loop-builder-abc")
	}
	if fs.writeCalls != 1 {
		t.Errorf("WriteFile calls = %d, want 1", fs.writeCalls)
	}
	if fs.writePath != SpecFilename {
		t.Errorf("WriteFile path = %q, want %q", fs.writePath, SpecFilename)
	}
	if fs.writeContent != specBody {
		t.Errorf("WriteFile content mismatch:\n got: %q\nwant: %q", fs.writeContent, specBody)
	}
	if !strings.Contains(res.Content, "loop-builder-abc") {
		t.Errorf("Result.Content missing task_id: %s", res.Content)
	}
	if !strings.Contains(res.Content, SpecFilename) {
		t.Errorf("Result.Content missing spec workspace path: %s", res.Content)
	}
}

// Idempotent re-create: sandbox returns "exists" status, tool reports it
// transparently in the result and still seeds SPEC.md (overwriting).
func TestExecute_IdempotentReCreate(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec body")
	fs.createReturns = sandbox.WorktreeInfo{Status: "exists", Path: "/workspace/loop-builder-abc", Branch: "main"}
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error on existing worktree: %s", res.Error)
	}
	if !strings.Contains(res.Content, "exists") {
		t.Errorf("Result.Content should advertise workspace_status=exists; got: %s", res.Content)
	}
	if fs.writeCalls != 1 {
		t.Errorf("WriteFile must still seed SPEC.md on re-create; calls = %d, want 1", fs.writeCalls)
	}
}

// ---------------------------------------------------------------------
// Sandbox failure paths
// ---------------------------------------------------------------------

func TestExecute_CreateWorktreeError(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	fs.createErr = errors.New("sandbox: HTTP 500")
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if fs.writeCalls != 0 {
		t.Errorf("expected no WriteFile call after create failure, got %d", fs.writeCalls)
	}
}

func TestExecute_WriteFileError(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	fs.writeErr = errors.New("sandbox: HTTP 403")
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if fs.createCalls != 1 {
		t.Errorf("CreateWorktree should have run once before WriteFile failed, got %d", fs.createCalls)
	}
}

// ---------------------------------------------------------------------
// PR #1 — commitments sidecar seeding
// ---------------------------------------------------------------------

const fixtureCommitmentsJSON = `[
  {
    "target": "executor publishes graph.mutation.entity.add on success",
    "approach": "process-local-testcontainer",
    "harness": "nats-jetstream",
    "runtime": "go-testing-net",
    "convention": {"type": "filepath", "path": "cmd/semteams/sandbox/integration_test.go"},
    "evidence": [{"kind": "test_uses_build_tag", "args": {"tag": "integration"}}]
  },
  {
    "target": "executor returns ToolErrorInvalidArgs on malformed input",
    "approach": "in-process-unit",
    "convention": {"type": "filepath", "path": "cmd/semteams/tools/x/executor_test.go"}
  }
]`

// TestExecute_WithSidecar_BothFilesSeeded verifies that when a commitments
// sidecar exists adjacent to the spec, WriteFile is called twice: once for
// SPEC.md and once for .evidence/commitments.json with the sidecar content.
func TestExecute_WithSidecar_BothFilesSeeded(t *testing.T) {
	exec, fs, specPath, _ := newExecutorWithSpecAndSidecar(t,
		"2026-05-06-osh-driver",
		"# OSH Driver spec body\n",
		fixtureCommitmentsJSON,
	)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}

	history := fs.writeHistorySnapshot()
	if len(history) != 2 {
		t.Fatalf("WriteFile calls = %d, want 2 (SPEC.md + .evidence/commitments.json)", len(history))
	}
	// First call must seed SPEC.md.
	if history[0].path != SpecFilename {
		t.Errorf("write[0].path = %q, want %q", history[0].path, SpecFilename)
	}
	if history[0].content != "# OSH Driver spec body\n" {
		t.Errorf("write[0].content mismatch")
	}
	// Second call must seed .evidence/commitments.json with exact sidecar content.
	if history[1].path != CommitmentsFilename {
		t.Errorf("write[1].path = %q, want %q", history[1].path, CommitmentsFilename)
	}
	if history[1].content != fixtureCommitmentsJSON {
		t.Errorf("write[1].content mismatch:\n got: %q\nwant: %q", history[1].content, fixtureCommitmentsJSON)
	}
	// Both calls must use the same task_id.
	if history[0].taskID != "loop-builder-abc" || history[1].taskID != "loop-builder-abc" {
		t.Errorf("task_id mismatch: write[0]=%q write[1]=%q", history[0].taskID, history[1].taskID)
	}
}

// TestExecute_WithoutSidecar_OnlySpecSeeded verifies that when no commitments
// sidecar exists adjacent to the spec, the bootstrap still succeeds and only
// SPEC.md is seeded (one WriteFile call).
func TestExecute_WithoutSidecar_OnlySpecSeeded(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "2026-05-06-no-sidecar", "# spec body\n")
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if fs.writeCalls != 1 {
		t.Errorf("WriteFile calls = %d, want 1 (SPEC.md only; no sidecar)", fs.writeCalls)
	}
	if fs.writePath != SpecFilename {
		t.Errorf("write path = %q, want %q", fs.writePath, SpecFilename)
	}
}

// TestExecute_SidecarReadError_BootstrapSucceeds verifies that when the
// commitments sidecar file exists but is unreadable (e.g. chmod 000), the
// bootstrap still succeeds and only SPEC.md is seeded. The error is logged
// but does NOT abort the tool call.
func TestExecute_SidecarReadError_BootstrapSucceeds(t *testing.T) {
	exec, fs, specPath, sidecarPath := newExecutorWithSpecAndSidecar(t,
		"2026-05-06-read-error",
		"# spec\n",
		fixtureCommitmentsJSON,
	)
	// Make the sidecar unreadable.
	if err := os.Chmod(sidecarPath, 0o000); err != nil {
		t.Skipf("cannot chmod sidecar (may be running as root): %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sidecarPath, 0o644) }) // restore for cleanup

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s (bootstrap must succeed even if sidecar unreadable)", res.Error)
	}
	// Only SPEC.md should have been seeded.
	if fs.writeCalls != 1 {
		t.Errorf("WriteFile calls = %d, want 1 (sidecar read error must not abort bootstrap)", fs.writeCalls)
	}
}

// TestExecute_SidecarWriteError_BootstrapSucceeds verifies that when sandbox.WriteFile
// fails for the commitments sidecar, the bootstrap still succeeds (SPEC.md was
// already seeded successfully and the tool result is clean).
func TestExecute_SidecarWriteError_BootstrapSucceeds(t *testing.T) {
	// Use a custom fakeSandbox whose WriteFile returns an error only on the
	// second call (the sidecar write), not the first (SPEC.md).
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "2026-05-06-write-error.md")
	if err := os.WriteFile(specPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	sidecarPath := commitmentsSidecarPath(specPath)
	if err := os.WriteFile(sidecarPath, []byte(fixtureCommitmentsJSON), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	fs := &errorOnSecondWrite{}
	exec, err := NewExecutor(fs, nil, tmp)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	res, execErr := exec.Execute(context.Background(), defaultCall(map[string]any{
		"spec_path": specPath,
	}))
	if execErr != nil {
		t.Fatalf("Execute err = %v, want nil", execErr)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s (sidecar write failure must NOT abort bootstrap)", res.Error)
	}
	if fs.writeCalls != 2 {
		t.Errorf("WriteFile calls = %d, want 2 (SPEC.md ok + sidecar errored)", fs.writeCalls)
	}
}

// errorOnSecondWrite is a SandboxClient stub that fails only on the second
// WriteFile call. Used by TestExecute_SidecarWriteError_BootstrapSucceeds.
type errorOnSecondWrite struct {
	mu         sync.Mutex
	writeCalls int
	createInfo sandbox.WorktreeInfo
}

func (f *errorOnSecondWrite) CreateWorktree(_ context.Context, taskID string, _ sandbox.CreateWorktreeOptions) (*sandbox.WorktreeInfo, error) {
	info := sandbox.WorktreeInfo{Status: "created", Path: "/workspace/" + taskID, Branch: "main"}
	return &info, nil
}

func (f *errorOnSecondWrite) WriteFile(_ context.Context, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCalls++
	if f.writeCalls >= 2 {
		return errors.New("sandbox: simulated second-write error")
	}
	return nil
}

// TestCommitmentsSidecarPath pins the .md-only contract — non-.md inputs
// return "" so maybeWriteCommitmentsSidecar skips rather than synthesising
// a sidecar path the architect never wrote.
func TestCommitmentsSidecarPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"md", "/abs/docs/specs/2026-05-06-foo.md", "/abs/docs/specs/2026-05-06-foo.commitments.json"},
		{"no extension", "/abs/docs/specs/foo", ""},
		{"txt", "/abs/docs/specs/foo.txt", ""},
		{"md.bak", "/abs/docs/specs/foo.md.bak", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commitmentsSidecarPath(tt.in); got != tt.want {
				t.Errorf("commitmentsSidecarPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
