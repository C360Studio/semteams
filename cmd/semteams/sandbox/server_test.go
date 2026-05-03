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
	"os"
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
	return ts, workspaceRoot
}

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

func TestCreateWorkspace(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)

	resp, err := http.Post(ts.URL+"/workspace/test.chain.001", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
	}
	var got map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != "created" {
		t.Fatalf("expected status=created, got %q", got["status"])
	}

	dir := filepath.Join(workspaceRoot, "test.chain.001")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat workspace: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workspace path is not a directory")
	}
}

func TestCreateWorkspaceIdempotent(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.idempotent"

	r1, err := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	if err != nil {
		t.Fatalf("first post: %v", err)
	}
	r1.Body.Close()

	r2, err := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	if err != nil {
		t.Fatalf("second post: %v", err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(r2.Body)
		t.Fatalf("unexpected status %d: %s", r2.StatusCode, body)
	}
	var got map[string]string
	if err := json.NewDecoder(r2.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != "exists" {
		t.Fatalf("expected status=exists, got %q", got["status"])
	}
}

func TestCreateWorkspaceInvalidID(t *testing.T) {
	ts, _ := newTestServer(t)
	// The validator unit test (TestIsValidChainID) covers the full
	// charset surface. Here we just verify the wire path returns 400
	// for an obviously-invalid ID that survives URL routing intact.
	resp, err := http.Post(ts.URL+"/workspace/with..dotdot", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDeleteWorkspace(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.delete"

	r, _ := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	r.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/workspace/"+chainID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	dir := filepath.Join(workspaceRoot, chainID)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected workspace removed; stat err: %v", err)
	}
}

func TestDeleteWorkspaceMissing(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/workspace/test.chain.never.created", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 idempotent, got %d", resp.StatusCode)
	}
}

func TestZipWorkspaceEmpty(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.zip.empty"

	r, _ := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	r.Body.Close()

	resp, err := http.Get(ts.URL + "/workspace/" + chainID)
	if err != nil {
		t.Fatalf("get zip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/zip" {
		t.Fatalf("expected zip content-type, got %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if len(zr.File) != 0 {
		t.Fatalf("expected empty zip, got %d files", len(zr.File))
	}
}

func TestZipWorkspaceWithFiles(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.zip.full"

	r, _ := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	r.Body.Close()

	dir := filepath.Join(workspaceRoot, chainID)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subdir/nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	resp, err := http.Get(ts.URL + "/workspace/" + chainID)
	if err != nil {
		t.Fatalf("get zip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	files := map[string]string{}
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", zf.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[zf.Name] = string(b)
	}
	if got, want := files["file.txt"], "hello"; got != want {
		t.Errorf("file.txt = %q, want %q", got, want)
	}
	if got, want := files["subdir/nested.txt"], "nested"; got != want {
		t.Errorf("subdir/nested.txt = %q, want %q", got, want)
	}
}

func TestZipWorkspaceSkipsSymlinks(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.zip.symlink"

	r, _ := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	r.Body.Close()

	// Create a target genuinely outside the workspace root and a
	// symlink pointing to it from inside. The zip endpoint must not
	// archive the symlink.
	outside := filepath.Join(filepath.Dir(workspaceRoot), "outside.secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	t.Cleanup(func() { os.Remove(outside) })
	dir := filepath.Join(workspaceRoot, chainID)
	if err := os.Symlink(outside, filepath.Join(dir, "leak")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "regular.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write regular: %v", err)
	}

	resp, err := http.Get(ts.URL + "/workspace/" + chainID)
	if err != nil {
		t.Fatalf("get zip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	for _, zf := range zr.File {
		if zf.Name == "leak" {
			t.Fatalf("zip contained symlink entry %q (should be skipped)", zf.Name)
		}
	}
	// regular.txt must still be present so we know the walk progressed.
	found := false
	for _, zf := range zr.File {
		if zf.Name == "regular.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected regular.txt in zip; entries: %v", zipNames(zr.File))
	}
}

func TestZipWorkspaceMissing(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/workspace/test.chain.never")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestIsValidChainID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"a", true},
		{"c360.prod.app.chain.abc123", true},
		{"chain-with-hyphens", true},
		{"chain_with_underscores", true},
		{"with/slash", false},
		{"with space", false},
		{"with..traversal", false},
		{".", false},
		{"..", false},
		{"with$dollar", false},
		{"with@at", false},
		{strings.Repeat("a", 256), true},
		{strings.Repeat("a", 257), false},
	}
	for _, tc := range cases {
		if got := isValidChainID(tc.id); got != tc.want {
			t.Errorf("isValidChainID(%q) = %v, want %v", tc.id, got, tc.want)
		}
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
// FILE OPS — R3.6.1.b
// =============================================================================

// createWorkspaceForTest is a helper that POSTs to create a workspace
// and asserts the response was successful. Keeps file-ops tests focused
// on what they're testing instead of repeating the setup.
func createWorkspaceForTest(t *testing.T, ts *httptest.Server, chainID string) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/workspace/"+chainID, "application/json", nil)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create workspace: status %d", resp.StatusCode)
	}
}

func writeFile(t *testing.T, ts *httptest.Server, chainID, path, content string) *http.Response {
	t.Helper()
	body := strings.NewReader(`{"path":"` + path + `","content":"` + content + `"}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/workspace/"+chainID+"/files", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	return resp
}

func readFile(t *testing.T, ts *httptest.Server, chainID, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + "/workspace/" + chainID + "/files?path=" + path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return resp
}

func TestWriteFileSuccess(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.write"
	createWorkspaceForTest(t, ts, chainID)

	resp := writeFile(t, ts, chainID, "main.go", "package main")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	got, err := os.ReadFile(filepath.Join(workspaceRoot, chainID, "main.go"))
	if err != nil {
		t.Fatalf("read on disk: %v", err)
	}
	if string(got) != "package main" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestWriteFileCreatesParentDirs(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.subdir"
	createWorkspaceForTest(t, ts, chainID)

	resp := writeFile(t, ts, chainID, "src/main/java/Foo.java", "public class Foo {}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	abs := filepath.Join(workspaceRoot, chainID, "src/main/java/Foo.java")
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("expected file at %s: %v", abs, err)
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.overwrite"
	createWorkspaceForTest(t, ts, chainID)

	r1 := writeFile(t, ts, chainID, "f.txt", "first")
	r1.Body.Close()
	r2 := writeFile(t, ts, chainID, "f.txt", "second")
	r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("overwrite: %d", r2.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(workspaceRoot, chainID, "f.txt"))
	if string(got) != "second" {
		t.Fatalf("expected overwritten content, got %q", got)
	}
}

func TestReadFileRoundtrip(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.roundtrip"
	createWorkspaceForTest(t, ts, chainID)

	w := writeFile(t, ts, chainID, "hi.txt", "hello world")
	w.Body.Close()

	r := readFile(t, ts, chainID, "hi.txt")
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("read status %d", r.StatusCode)
	}
	var got fileReadResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Content != "hello world" {
		t.Fatalf("content = %q", got.Content)
	}
	if got.Truncated {
		t.Fatalf("expected non-truncated")
	}
}

func TestReadFileTruncation(t *testing.T) {
	// Use a small maxReadBytes so we can exercise the truncation path
	// without writing megabytes through the API.
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:  workspaceRoot,
		MaxFileSize:    1 * 1024 * 1024,
		MaxReadBytes:   8, // exercise truncation path
		MaxOutputBytes: 64 * 1024,
		Logger:         logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	chainID := "test.chain.truncate"
	createWorkspaceForTest(t, ts, chainID)
	w := writeFile(t, ts, chainID, "big.txt", "this is way more than 8 bytes")
	w.Body.Close()

	r := readFile(t, ts, chainID, "big.txt")
	defer r.Body.Close()
	var got fileReadResponse
	json.NewDecoder(r.Body).Decode(&got)
	if !got.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if got.Size != 8 {
		t.Fatalf("expected size=8 (truncated), got %d", got.Size)
	}
	if got.Content != "this is " {
		t.Fatalf("expected first 8 bytes, got %q", got.Content)
	}
}

func TestReadFileMissing(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.read.missing"
	createWorkspaceForTest(t, ts, chainID)

	r := readFile(t, ts, chainID, "nothing.txt")
	defer r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", r.StatusCode)
	}
}

func TestWriteFileMissingPath(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.empty.path"
	createWorkspaceForTest(t, ts, chainID)

	resp := writeFile(t, ts, chainID, "", "content")
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
		MaxFileSize:    16, // exercise the 413 path
		MaxReadBytes:   256 * 1024,
		MaxOutputBytes: 64 * 1024,
		Logger:         logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	chainID := "test.chain.oversize"
	createWorkspaceForTest(t, ts, chainID)

	// "this content is more than sixteen bytes long" > 16 bytes
	resp := writeFile(t, ts, chainID, "f.txt", "this content is more than sixteen bytes long")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

func TestWriteFileMissingWorkspace(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := writeFile(t, ts, "test.chain.no.workspace", "f.txt", "x")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestReadFileMissingWorkspace(t *testing.T) {
	ts, _ := newTestServer(t)
	r := readFile(t, ts, "test.chain.no.workspace", "f.txt")
	defer r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", r.StatusCode)
	}
}

func TestWriteFileAbsolutePath(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.abs"
	createWorkspaceForTest(t, ts, chainID)

	// JSON body with an absolute path.
	body := strings.NewReader(`{"path":"/etc/passwd","content":"x"}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/workspace/"+chainID+"/files", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestWriteFileTraversal(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.traversal"
	createWorkspaceForTest(t, ts, chainID)

	body := strings.NewReader(`{"path":"../../../etc/passwd","content":"x"}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/workspace/"+chainID+"/files", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	// Verify nothing was written.
	if _, err := os.Stat(filepath.Join(workspaceRoot, "..", "..", "..", "etc", "passwd-fake")); err == nil {
		t.Fatalf("traversal escaped — file written outside workspace")
	}
}

func TestReadFileAbsolutePath(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.read.abs"
	createWorkspaceForTest(t, ts, chainID)

	r := readFile(t, ts, chainID, "/etc/passwd")
	defer r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", r.StatusCode)
	}
}

func TestReadFileTraversal(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.read.traversal"
	createWorkspaceForTest(t, ts, chainID)

	r := readFile(t, ts, chainID, "../../../etc/passwd")
	defer r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", r.StatusCode)
	}
}

func TestWriteFileSymlinkParentEscapes(t *testing.T) {
	// Pre-create a symlink inside the workspace pointing genuinely
	// outside it, then attempt to read through it. The file API can't
	// create symlinks itself; this simulates the post-R3.6.1.c case
	// where bash could have planted one. resolveChainPath's
	// EvalSymlinks check must catch the escape.
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.symlink"
	createWorkspaceForTest(t, ts, chainID)

	outside := filepath.Join(filepath.Dir(workspaceRoot), "outside.target")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	t.Cleanup(func() { os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(workspaceRoot, chainID, "leak")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := readFile(t, ts, chainID, "leak")
	defer r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 (refusing symlink), got %d", r.StatusCode)
	}
}

func TestWriteFileRefusesExistingSymlink(t *testing.T) {
	// Pre-create a symlink inside the workspace (target inside the
	// workspace is fine — the defense is "file API never honors
	// symlinks", regardless of where they point). Write through the
	// symlink path must be refused so a planted alias can't be used
	// to clobber the real target.
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.write.symlink"
	createWorkspaceForTest(t, ts, chainID)

	dir := filepath.Join(workspaceRoot, chainID)
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("real"), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(dir, "alias.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resp := writeFile(t, ts, chainID, "alias.txt", "tampered")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 (refusing to write through symlink), got %d", resp.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "real.txt"))
	if string(got) != "real" {
		t.Fatalf("real file clobbered through symlink: %q", got)
	}
}

func TestReadFileIsDirectory(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.read.dir"
	createWorkspaceForTest(t, ts, chainID)

	if err := os.MkdirAll(filepath.Join(workspaceRoot, chainID, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	r := readFile(t, ts, chainID, "subdir")
	defer r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 (path is dir), got %d", r.StatusCode)
	}
}

// =============================================================================
// EXEC — R3.6.1.c
// =============================================================================

func execRequestBody(command string, timeoutMs int) *strings.Reader {
	if timeoutMs > 0 {
		return strings.NewReader(`{"command":` + jsonString(command) + `,"timeout_ms":` + strconv.Itoa(timeoutMs) + `}`)
	}
	return strings.NewReader(`{"command":` + jsonString(command) + `}`)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func postExec(t *testing.T, ts *httptest.Server, chainID, command string, timeoutMs int) *http.Response {
	t.Helper()
	body := execRequestBody(command, timeoutMs)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/workspace/"+chainID+"/exec", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return resp
}

func TestExecEcho(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.exec.echo"
	createWorkspaceForTest(t, ts, chainID)

	resp := postExec(t, ts, chainID, "echo hello", 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
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
	chainID := "test.chain.exec.exit"
	createWorkspaceForTest(t, ts, chainID)

	resp := postExec(t, ts, chainID, "exit 7", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.ExitCode != 7 {
		t.Fatalf("expected exit=7, got %d", got.ExitCode)
	}
}

func TestExecStderrCaptured(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.exec.stderr"
	createWorkspaceForTest(t, ts, chainID)

	resp := postExec(t, ts, chainID, "echo oops 1>&2", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.ExitCode != 0 {
		t.Fatalf("exit %d", got.ExitCode)
	}
	if !strings.Contains(got.Stderr, "oops") {
		t.Fatalf("stderr = %q", got.Stderr)
	}
	if strings.Contains(got.Stdout, "oops") {
		t.Fatalf("stdout leaked stderr: %q", got.Stdout)
	}
}

func TestExecChdirToWorkspace(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.exec.pwd"
	createWorkspaceForTest(t, ts, chainID)

	resp := postExec(t, ts, chainID, "pwd", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	expected := filepath.Join(workspaceRoot, chainID)
	// macOS may resolve /var to /private/var; compare via EvalSymlinks.
	resolved, _ := filepath.EvalSymlinks(expected)
	if !strings.Contains(got.Stdout, resolved) && !strings.Contains(got.Stdout, expected) {
		t.Fatalf("pwd output %q didn't include workspace path %q (or its resolved form %q)",
			got.Stdout, expected, resolved)
	}
}

func TestExecTimeoutKillsProcess(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.exec.timeout"
	createWorkspaceForTest(t, ts, chainID)

	start := time.Now()
	resp := postExec(t, ts, chainID, "sleep 30", 100) // 100ms timeout vs 30s sleep
	defer resp.Body.Close()
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("timeout didn't kill process in time; elapsed %v", elapsed)
	}
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if !got.TimedOut {
		t.Fatalf("expected timed_out=true, got %+v", got)
	}
}

func TestExecOutputTruncation(t *testing.T) {
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot:      workspaceRoot,
		MaxFileSize:        1024,
		MaxReadBytes:       1024,
		MaxOutputBytes:     16, // tiny cap
		DefaultExecTimeout: 2 * time.Second,
		MaxExecTimeout:     10 * time.Second,
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	chainID := "test.chain.exec.trunc"
	createWorkspaceForTest(t, ts, chainID)
	resp := postExec(t, ts, chainID, "printf 'this is a long string that exceeds 16 bytes'", 0)
	defer resp.Body.Close()
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Stdout) > 16 {
		t.Fatalf("expected stdout truncated to 16 bytes, got %d (%q)", len(got.Stdout), got.Stdout)
	}
}

func TestExecMissingWorkspace(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := postExec(t, ts, "test.chain.exec.no.workspace", "echo x", 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestExecMissingCommand(t *testing.T) {
	ts, _ := newTestServer(t)
	chainID := "test.chain.exec.empty"
	createWorkspaceForTest(t, ts, chainID)

	resp := postExec(t, ts, chainID, "", 0)
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
		MaxExecTimeout:     200 * time.Millisecond, // cap below request
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	chainID := "test.chain.exec.cap"
	createWorkspaceForTest(t, ts, chainID)

	start := time.Now()
	// Request 30s; cap is 200ms; sleep 5s should be killed at 200ms.
	resp := postExec(t, ts, chainID, "sleep 5", 30000)
	defer resp.Body.Close()
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Fatalf("MaxExecTimeout cap not honored; elapsed %v", elapsed)
	}
	var got execResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if !got.TimedOut {
		t.Fatalf("expected timed_out=true")
	}
}

// =============================================================================
// PARENT-SYMLINK HARDENING — R3.6.1.c
// =============================================================================

func TestResolveChainPathRejectsParentSymlink(t *testing.T) {
	root := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot: root,
		MaxFileSize:   1024,
		MaxReadBytes:  1024,
		Logger:        logger,
	})

	chainID := "test.chain.parent.symlink"
	chainDir := filepath.Join(root, chainID)
	if err := os.MkdirAll(chainDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	outside := filepath.Join(filepath.Dir(root), "outside.target")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(outside) })

	// Plant a parent-dir symlink inside the chain workspace (simulates
	// what bash exec could do in R3.6.1.c).
	if err := os.Symlink(outside, filepath.Join(chainDir, "evil-dir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// Path through the parent symlink must be refused.
	_, err := srv.resolveChainPath(chainID, "evil-dir/anything.txt")
	if err == nil {
		t.Fatalf("expected error for parent-symlink path; got nil")
	}
}

func TestWriteFileRejectsParentSymlink(t *testing.T) {
	ts, workspaceRoot := newTestServer(t)
	chainID := "test.chain.write.parent.symlink"
	createWorkspaceForTest(t, ts, chainID)

	outside := filepath.Join(filepath.Dir(workspaceRoot), "outside.parent")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(outside) })

	if err := os.Symlink(outside, filepath.Join(workspaceRoot, chainID, "alias-dir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resp := writeFile(t, ts, chainID, "alias-dir/leak.txt", "x")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	// Ensure no file was written outside.
	if _, err := os.Stat(filepath.Join(outside, "leak.txt")); err == nil {
		t.Fatalf("leak file written outside workspace despite 403")
	}
}

// TestConcurrentChainsIndependent exercises the per-chain mutex map.
// Two chains running file ops + exec in parallel must not block each
// other and must not see each other's state. The race detector catches
// any unsynchronised access to the chainMutexes map itself.
func TestConcurrentChainsIndependent(t *testing.T) {
	ts, _ := newTestServer(t)
	const chainA = "test.chain.concurrent.a"
	const chainB = "test.chain.concurrent.b"
	createWorkspaceForTest(t, ts, chainA)
	createWorkspaceForTest(t, ts, chainB)

	const iters = 20
	done := make(chan error, 2)

	hammer := func(chainID string) {
		var lastErr error
		for i := range iters {
			path := "f" + strconv.Itoa(i) + ".txt"
			content := chainID + "-iter-" + strconv.Itoa(i)
			r := writeFile(t, ts, chainID, path, content)
			r.Body.Close()
			if r.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("%s write %d: status %d", chainID, i, r.StatusCode)
				continue
			}
			rd := readFile(t, ts, chainID, path)
			var got fileReadResponse
			json.NewDecoder(rd.Body).Decode(&got)
			rd.Body.Close()
			if got.Content != content {
				lastErr = fmt.Errorf("%s read %d: cross-chain leak — want %q got %q",
					chainID, i, content, got.Content)
			}
		}
		done <- lastErr
	}

	go hammer(chainA)
	go hammer(chainB)

	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent chain hammer: %v", err)
		}
	}
}

func TestResolveChainPath(t *testing.T) {
	root := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(ServerConfig{
		WorkspaceRoot: root,
		MaxFileSize:   1024,
		MaxReadBytes:  1024,
		Logger:        logger,
	})
	chainID := "test.resolve"
	if err := os.MkdirAll(filepath.Join(root, chainID), 0o755); err != nil {
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
			_, err := srv.resolveChainPath(chainID, tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
