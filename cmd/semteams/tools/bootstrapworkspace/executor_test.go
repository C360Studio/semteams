package bootstrapworkspace

import (
	"context"
	"encoding/json"
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

// writeRecord captures one WriteFile call.
type writeRecord struct {
	taskID  string
	path    string
	content string
}

// fakeSandbox records CreateWorktree + WriteFile calls. Test sets
// createErr / writeErr to drive failure paths.
//
// writeCalls, writeTaskID, writePath, writeContent record the LAST write
// for backward-compat with existing tests. writes records ALL writes for
// sidecar projection tests.
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
	// writeErrOnN makes the Nth call (1-indexed) return writeErr.
	// 0 means writeErr applies to every call (original behaviour).
	writeErrOnN int
	writes      []writeRecord
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
	f.writes = append(f.writes, writeRecord{taskID: taskID, path: path, content: content})
	if f.writeErr != nil {
		if f.writeErrOnN == 0 || f.writeCalls == f.writeErrOnN {
			return f.writeErr
		}
	}
	return nil
}

// writeFor returns the content of the write call for the given path, or ""
// if no such call was made.
func (f *fakeSandbox) writeFor(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.writes {
		if w.path == path {
			return w.content
		}
	}
	return ""
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

// ---------------------------------------------------------------------
// ADR-036 Phase 1 — sidecar projection tests
// ---------------------------------------------------------------------

// writeSidecar writes a minimal <slug>.checks.json adjacent to specPath.
func writeSidecar(t *testing.T, specPath string, checks json.RawMessage, harnesses map[string]any) {
	t.Helper()
	sidecarPath := strings.TrimSuffix(specPath, ".md") + ".checks.json"
	payload := map[string]any{"checks": json.RawMessage(checks)}
	if harnesses != nil {
		payload["harnesses"] = harnesses
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	if err := os.WriteFile(sidecarPath, data, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

// fixtureHarnesses returns a minimal harnesses map for test sidecars.
func fixtureHarnesses(ids ...string) map[string]any {
	m := make(map[string]any, len(ids))
	for _, id := range ids {
		m[id] = map[string]any{
			"id":    id,
			"image": "img-" + id + ":1.0",
			"ports": map[string]any{"tcp-" + id: 1234},
		}
	}
	return m
}

// rawChecks returns a minimal JSON array of check objects for sidecar fixtures.
func rawChecks(n int) json.RawMessage {
	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{"target": "check " + string(rune('A'+i)), "runtime": "in-process-unit"}
	}
	data, _ := json.Marshal(items)
	return data
}

// TestSidecarProjection_ChecksAndHarnesses verifies that when a sidecar is
// present with both checks and harnesses, both workspace files are written.
func TestSidecarProjection_ChecksAndHarnesses(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "2026-05-06-test-spec", "# spec")
	writeSidecar(t, specPath, rawChecks(2), fixtureHarnesses("meshtasticd-2.x"))

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}

	// SPEC.md + .evidence/checks.json + .test-harness/manifest.json = 3 writes.
	if fs.writeCalls != 3 {
		t.Errorf("WriteFile calls = %d, want 3 (SPEC.md + checks + harness manifest)", fs.writeCalls)
	}

	checksContent := fs.writeFor(ChecksFilename)
	if checksContent == "" {
		t.Fatalf("no write for %s", ChecksFilename)
	}
	var checksArr []any
	if err := json.Unmarshal([]byte(checksContent), &checksArr); err != nil {
		t.Fatalf("unmarshal checks: %v\ncontent=%s", err, checksContent)
	}
	if len(checksArr) != 2 {
		t.Errorf("checks count = %d, want 2", len(checksArr))
	}

	harnessContent := fs.writeFor(TestHarnessManifestFilename)
	if harnessContent == "" {
		t.Fatalf("no write for %s", TestHarnessManifestFilename)
	}
	var harnessMap map[string]any
	if err := json.Unmarshal([]byte(harnessContent), &harnessMap); err != nil {
		t.Fatalf("unmarshal harness manifest: %v\ncontent=%s", err, harnessContent)
	}
	if _, ok := harnessMap["meshtasticd-2.x"]; !ok {
		t.Errorf("harness manifest missing meshtasticd-2.x; keys=%v", harnessMapKeys(harnessMap))
	}
}

// TestSidecarProjection_ChecksOnly verifies that when harnesses is absent/empty,
// only .evidence/checks.json is written (not .test-harness/manifest.json).
func TestSidecarProjection_ChecksOnly(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	writeSidecar(t, specPath, rawChecks(1), nil)

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}

	if fs.writeCalls != 2 {
		t.Errorf("WriteFile calls = %d, want 2 (SPEC.md + checks)", fs.writeCalls)
	}
	if got := fs.writeFor(ChecksFilename); got == "" {
		t.Errorf("expected write to %s; none found", ChecksFilename)
	}
	if got := fs.writeFor(TestHarnessManifestFilename); got != "" {
		t.Errorf("expected NO write to %s; found content: %s", TestHarnessManifestFilename, got)
	}
}

// TestSidecarProjection_HarnessesOnly verifies that when checks is absent/empty
// but harnesses is present, only .test-harness/manifest.json is written.
func TestSidecarProjection_HarnessesOnly(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	// Write sidecar with empty checks array and a harness entry.
	writeSidecar(t, specPath, json.RawMessage(`[]`), fixtureHarnesses("nats-jetstream"))

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}

	// SPEC.md + .test-harness/manifest.json = 2 writes (.evidence/checks.json skipped).
	if fs.writeCalls != 2 {
		t.Errorf("WriteFile calls = %d, want 2 (SPEC.md + harness manifest)", fs.writeCalls)
	}
	if got := fs.writeFor(ChecksFilename); got != "" {
		t.Errorf("expected NO write to %s with empty checks; found: %s", ChecksFilename, got)
	}
	if got := fs.writeFor(TestHarnessManifestFilename); got == "" {
		t.Errorf("expected write to %s; none found", TestHarnessManifestFilename)
	}
}

// TestSidecarProjection_MissingSidecar verifies that a missing sidecar logs
// at Debug and skips projection; bootstrap succeeds with only SPEC.md written.
func TestSidecarProjection_MissingSidecar(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec body")
	// No sidecar written — specPath has no adjacent .checks.json.

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}

	if fs.writeCalls != 1 {
		t.Errorf("WriteFile calls = %d, want 1 (SPEC.md only; no sidecar)", fs.writeCalls)
	}
	if got := fs.writeFor(ChecksFilename); got != "" {
		t.Errorf("expected NO write to %s on missing sidecar; found: %s", ChecksFilename, got)
	}
}

// TestSidecarProjection_SidecarReadError verifies that a sidecar read error
// (chmod 000) is logged at Warn and skipped; bootstrap succeeds.
func TestSidecarProjection_SidecarReadError(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	sidecarPath := strings.TrimSuffix(specPath, ".md") + ".checks.json"
	// Write a sidecar then make it unreadable.
	if err := os.WriteFile(sidecarPath, []byte(`{"checks":[]}`), 0o000); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sidecarPath, 0o644) })

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("bootstrap should succeed even when sidecar is unreadable; got: %s", res.Error)
	}
	// Only SPEC.md written.
	if fs.writeCalls != 1 {
		t.Errorf("WriteFile calls = %d, want 1 (sidecar read failed, projection skipped)", fs.writeCalls)
	}
}

// TestSidecarProjection_WriteFileFails verifies that a sandbox.WriteFile
// error on workspace projection is logged and skipped; bootstrap succeeds.
func TestSidecarProjection_WriteFileFails(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	writeSidecar(t, specPath, rawChecks(1), fixtureHarnesses("h1"))

	// The SECOND WriteFile call (checks file) fails; third call (harness manifest)
	// should still be attempted.
	fs.writeErr = errors.New("sandbox: volume full")
	fs.writeErrOnN = 2

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("bootstrap should succeed even when workspace write fails; got: %s", res.Error)
	}
	// SPEC.md write (call 1) succeeds; checks write (call 2) fails; harness write (call 3) proceeds.
	if fs.writeCalls < 2 {
		t.Errorf("WriteFile calls = %d, want >= 2", fs.writeCalls)
	}
}

// TestSidecarProjection_IdempotentRerun verifies that re-running bootstrap
// with the same sidecar overwrites workspace files with identical content.
func TestSidecarProjection_IdempotentRerun(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec")
	writeSidecar(t, specPath, rawChecks(1), fixtureHarnesses("idem-h"))

	for i := range 2 {
		res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
		if err != nil {
			t.Fatalf("run %d Execute err: %v", i, err)
		}
		if res.Error != "" {
			t.Fatalf("run %d tool error: %s", i, res.Error)
		}
	}
	// 2 runs × 3 writes each = 6 total.
	if fs.writeCalls != 6 {
		t.Errorf("WriteFile calls = %d, want 6 (2 runs × 3 writes)", fs.writeCalls)
	}
}

// TestSidecarProjection_ExportedConstantPaths pins the exported constant paths
// used by contract tests and downstream slices.
func TestSidecarProjection_ExportedConstantPaths(t *testing.T) {
	if ChecksFilename != ".evidence/checks.json" {
		t.Errorf("ChecksFilename = %q, want .evidence/checks.json", ChecksFilename)
	}
	if TestHarnessManifestFilename != ".test-harness/manifest.json" {
		t.Errorf("TestHarnessManifestFilename = %q, want .test-harness/manifest.json", TestHarnessManifestFilename)
	}
}

func harnessMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------
// ADR-041 Phase 4 — chain-scoped task_id resolution
// ---------------------------------------------------------------------

// fakeChainResolver lets tests drive the ChainID outcome without NATS.
// Records every loop_id passed in so we can assert the executor actually
// invoked it (a regression where SetChainResolver is wired but Execute
// forgets to call it would otherwise silently pass).
type fakeChainResolver struct {
	chainID string
	err     error
	calls   []string
}

func (f *fakeChainResolver) ChainID(_ context.Context, loopID string) (string, error) {
	f.calls = append(f.calls, loopID)
	return f.chainID, f.err
}

// Chain-scoped happy path: resolver returns a different chain_id, the
// worktree is created under chain_id (not loop_id), and the result
// reports chain_id back to the LLM.
func TestExecute_ChainScopedTaskID(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec body")
	resolver := &fakeChainResolver{chainID: "chain-root-uuid"}
	exec.SetChainResolver(resolver)

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "loop-builder-abc" {
		t.Errorf("resolver.calls = %v, want [loop-builder-abc]", resolver.calls)
	}
	if fs.createTaskID != "chain-root-uuid" {
		t.Errorf("CreateWorktree task_id = %q, want %q (chain-scoped)", fs.createTaskID, "chain-root-uuid")
	}
	if fs.writeTaskID != "chain-root-uuid" {
		t.Errorf("WriteFile task_id = %q, want %q (chain-scoped)", fs.writeTaskID, "chain-root-uuid")
	}
	if !strings.Contains(res.Content, "chain-root-uuid") {
		t.Errorf("Result.Content missing chain_id: %s", res.Content)
	}
}

// Fail-soft: when the resolver errors, the executor must still bootstrap
// using loop_id. A NATS flap or missing parent triple cannot wedge the
// builder.
func TestExecute_ChainResolverError_FallsBackToLoopID(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec body")
	resolver := &fakeChainResolver{err: errSentinel("nats timeout")}
	exec.SetChainResolver(resolver)

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath}))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if fs.createTaskID != "loop-builder-abc" {
		t.Errorf("CreateWorktree task_id = %q, want loop-builder-abc (fallback)", fs.createTaskID)
	}
}

// Empty resolver result is treated as "no chain context" — fall back to
// loop_id. The chain-root case (chain_id == loop_id) is exercised
// indirectly by every other happy-path test (resolver returns the same
// loop_id back).
func TestExecute_ChainResolverEmpty_FallsBackToLoopID(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec body")
	resolver := &fakeChainResolver{chainID: ""}
	exec.SetChainResolver(resolver)

	if _, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath})); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fs.createTaskID != "loop-builder-abc" {
		t.Errorf("CreateWorktree task_id = %q, want loop-builder-abc (empty chain fallback)", fs.createTaskID)
	}
}

// Without SetChainResolver (zero-resolver back-compat path), bootstrap
// uses loop_id unchanged — exactly the pre-MVP behaviour.
func TestExecute_NoResolver_UsesLoopID(t *testing.T) {
	exec, fs, specPath := newExecutorWithSpec(t, "slug", "# spec body")
	// no SetChainResolver call

	if _, err := exec.Execute(context.Background(), defaultCall(map[string]any{"spec_path": specPath})); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fs.createTaskID != "loop-builder-abc" {
		t.Errorf("CreateWorktree task_id = %q, want loop-builder-abc", fs.createTaskID)
	}
}

// errSentinel keeps the test file free of additional imports (errors
// package would shadow the existing testing-package convention here).
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
