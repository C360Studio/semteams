package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	workspaceRoot := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(workspaceRoot, logger)
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

	// Create a target outside the workspace and a symlink pointing to
	// it from inside. The zip endpoint must not archive the symlink.
	outside := filepath.Join(workspaceRoot, "..outside.secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
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
