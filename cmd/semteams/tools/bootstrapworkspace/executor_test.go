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

// fakeSandbox records CreateWorktree + WriteFile calls. Test sets
// createErr / writeErr to drive failure paths.
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
	return f.writeErr
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

// newExecutorWithSpec writes a fixture spec to a tmp dir and constructs
// an Executor scoped to that dir. Returns the executor, the fake
// sandbox, the relative-to-cwd spec path the LLM would substitute, and
// the absolute path for assertions.
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
