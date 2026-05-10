//go:build unix

package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestZipDir_HappyPath verifies that zipDir includes all files in a normal
// (non-bind-mounted) directory tree.
func TestZipDir_HappyPath(t *testing.T) {
	dir := t.TempDir()

	// Create a small tree: one file at root, one in a subdir.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write hello.txt: %v", err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "world.txt"), []byte("world"), 0o644); err != nil {
		t.Fatalf("write sub/world.txt: %v", err)
	}

	var buf bytes.Buffer
	if err := zipDir(&buf, dir, nil); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	entries := zipEntries(t, &buf)
	if !entries["hello.txt"] {
		t.Error("zip missing hello.txt")
	}
	if !entries["sub/world.txt"] {
		t.Error("zip missing sub/world.txt")
	}
}

// TestZipDir_SkipsFakeBindMount verifies that zipDir skips directory subtrees
// where the injected sameDeviceFunc returns false — simulating a bind-mount
// detection without requiring root privileges.
//
// ADR-040 addendum 2026-05-10 §item 6 — bind-mount skip guardrail.
func TestZipDir_SkipsFakeBindMount(t *testing.T) {
	dir := t.TempDir()

	// Regular workspace file.
	if err := os.WriteFile(filepath.Join(dir, "artifact.md"), []byte("# result"), 0o644); err != nil {
		t.Fatalf("write artifact.md: %v", err)
	}

	// "Bind-mounted" sources directory: a real directory on disk but our
	// fake sameDevice will pretend its device ID differs.
	sourcesDir := filepath.Join(dir, "sources")
	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcesDir, "huge-file.java"), []byte("// 3 GB of Java"), 0o644); err != nil {
		t.Fatalf("write sources/huge-file.java: %v", err)
	}

	// Install a fake sameDeviceFunc that returns false for sourcesDir and
	// its children, true for everything else.
	orig := sameDeviceFunc
	t.Cleanup(func() { sameDeviceFunc = orig })
	sameDeviceFunc = func(_ os.FileInfo, info os.FileInfo) bool {
		// Return false for the "bind mount" directory.
		return info.Name() != "sources"
	}

	var buf bytes.Buffer
	if err := zipDir(&buf, dir, nil); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	entries := zipEntries(t, &buf)
	if !entries["artifact.md"] {
		t.Error("zip must include artifact.md (regular workspace file)")
	}
	if entries["sources/"] {
		t.Error("zip must NOT include sources/ directory entry (simulated bind mount)")
	}
	if entries["sources/huge-file.java"] {
		t.Error("zip must NOT include sources/huge-file.java (simulated bind mount subtree)")
	}
}

// TestZipDir_SkipsSymlinks verifies the pre-existing symlink-skip behaviour
// is preserved after the bind-mount guardrail was added.
func TestZipDir_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("real"), 0o644); err != nil {
		t.Fatalf("write real.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "external.txt"), []byte("external"), 0o644); err != nil {
		t.Fatalf("write target/external.txt: %v", err)
	}
	// Symlink from inside the workspace to an external directory.
	if err := os.Symlink(target, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("cannot create symlink (likely non-unix): %v", err)
	}

	var buf bytes.Buffer
	if err := zipDir(&buf, dir, nil); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	entries := zipEntries(t, &buf)
	if !entries["real.txt"] {
		t.Error("zip must include real.txt")
	}
	if entries["linked"] || entries["linked/"] {
		t.Error("zip must NOT include the symlink 'linked'")
	}
	if entries["linked/external.txt"] {
		t.Error("zip must NOT include content through a symlink")
	}
}

// zipEntries reads a zip from buf and returns a set of all entry names.
func zipEntries(t *testing.T, buf *bytes.Buffer) map[string]bool {
	t.Helper()
	data := buf.Bytes()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	out := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		out[f.Name] = true
		// Also read the content so we catch any read errors early.
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", f.Name, err)
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			t.Fatalf("read zip entry %q: %v", f.Name, err)
		}
		rc.Close()
	}
	return out
}
