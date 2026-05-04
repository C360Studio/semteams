package evidence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Context is the read-only workspace view a Checker queries.
// Constructed once per gate run by the caller (the dvs-reviewer at
// R3.7.2.j′; for now, tests). Checkers receive a pointer to this
// struct; they MUST treat it as immutable — the gate is read-only.
//
// WorkspaceRoot is the absolute filesystem path to the root of the
// builder's sandbox workspace as seen from outside the sandbox
// (the host filesystem mount point). Checkers resolve relative
// paths against this root via the Resolve method, which guards
// against path traversal — same shape as sandbox.Server's
// resolveTaskPath.
//
// SurefireReportDir is an optional override for the directory
// surefire-reading checkers read XML from. Defaults to
// "target/surefire-reports" under WorkspaceRoot. Operators with
// non-standard layouts (mvn -Dproject.build.directory=... or a
// multi-module build) override this per gate run.
type Context struct {
	WorkspaceRoot     string
	SurefireReportDir string
}

// NewContext constructs a Context with the standard Maven
// surefire path defaulted under workspaceRoot. workspaceRoot must
// be absolute; callers passing relative paths get an error at
// construction rather than at first Resolve.
func NewContext(workspaceRoot string) (*Context, error) {
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root cannot be empty")
	}
	if !filepath.IsAbs(workspaceRoot) {
		return nil, fmt.Errorf("workspace root must be absolute (got %q)", workspaceRoot)
	}
	return &Context{
		WorkspaceRoot:     workspaceRoot,
		SurefireReportDir: filepath.Join(workspaceRoot, "target", "surefire-reports"),
	}, nil
}

// Resolve returns the absolute filesystem path for a workspace-
// relative path, refusing entries that would escape the workspace
// root. Mirrors sandbox.Server's resolveTaskPath: clean the path,
// reject absolute or "../"-prefixed entries, then join under root.
//
// Defense-in-depth on the resolved path: if it exists AND is a
// symlink, refuse — a builder-planted symlink (e.g.
// `target/surefire-reports/TEST-foo.xml -> /etc/passwd`) would
// otherwise let a checker read outside the workspace. Non-existent
// paths pass through (the checker gets a clean abs path to stat
// itself; that's the FileExistsChecker's signal anyway).
//
// Callers (Checkers) that read files MUST go through Resolve; using
// filepath.Join directly would let a malicious EvidenceRule.Args
// (an architect's mistake or a chain that's been compromised
// upstream) point a checker at /etc/passwd.
func (ec *Context) Resolve(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be workspace-relative (got %q)", rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace", rel)
	}
	abs := filepath.Join(ec.WorkspaceRoot, clean)
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("path %q is a symlink; gate refuses symlinked entries (defense-in-depth against workspace traversal)", rel)
	}
	return abs, nil
}
