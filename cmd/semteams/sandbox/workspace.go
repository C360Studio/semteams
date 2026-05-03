package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultBranch is the git branch initialised in every fresh
// workspace. Hard-coded rather than relying on the host git's
// init.defaultBranch so behavior is reproducible across hosts and
// CI runners.
const defaultBranch = "main"

// gitInitTimeout bounds how long the git init + initial commit step
// can take. Plenty for local fs operations; if it times out, the
// workspace is malformed and the create handler returns 500.
const gitInitTimeout = 10 * time.Second

// Path-confinement failure sentinels.
var (
	errAbsolutePath   = errors.New("absolute paths are not allowed")
	errPathEscapes    = errors.New("path escapes workspace")
	errSymlinkEscapes = errors.New("symlink escapes workspace")
)

// resolveTaskPath maps a workspace-relative request path to an
// absolute filesystem path inside the task's workspace, rejecting any
// attempt to escape via:
//   - absolute path
//   - .. traversal (filepath.Clean + prefix check)
//   - symlink whose target resolves outside the workspace
//   - parent-dir component that's a symlink
//
// Threat-model note for the per-component lstat: the per-task mutex
// held by handlers serialises every op within a task, so there's no
// race window between resolveTaskPath returning and the subsequent
// open. A bash exec on task X holds the mutex for the full command;
// a concurrent write_file on task X waits behind it.
func (s *Server) resolveTaskPath(taskID, relPath string) (string, error) {
	if !isValidTaskID(taskID) {
		return "", errors.New("invalid task_id")
	}
	if filepath.IsAbs(relPath) {
		return "", errAbsolutePath
	}

	taskRoot := filepath.Join(s.workspaceRoot, taskID)
	absPath := filepath.Join(taskRoot, filepath.Clean(relPath))

	if !strings.HasPrefix(absPath, taskRoot+string(filepath.Separator)) && absPath != taskRoot {
		return "", errPathEscapes
	}

	// Per-component lstat: walk every existing component from taskRoot
	// toward absPath; refuse if any is a symlink. Closes the parent-
	// dir-symlink gap a bash exec could otherwise plant.
	if absPath != taskRoot {
		rel := strings.TrimPrefix(absPath, taskRoot+string(filepath.Separator))
		cur := taskRoot
		for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
			if part == "" {
				continue
			}
			cur = filepath.Join(cur, part)
			info, err := os.Lstat(cur)
			if os.IsNotExist(err) {
				break
			}
			if err != nil {
				return "", err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", errSymlinkEscapes
			}
		}
	}

	// Belt-and-suspenders: EvalSymlinks check on the resolved target
	// catches the case the per-component walk doesn't (e.g., a chain
	// where the target component is itself a hostile symlink whose
	// target lives outside the workspace).
	if absPath != taskRoot {
		realPath, err := filepath.EvalSymlinks(absPath)
		if err == nil {
			realRoot, rootErr := filepath.EvalSymlinks(taskRoot)
			if rootErr != nil {
				realRoot = taskRoot
			}
			if !strings.HasPrefix(realPath, realRoot+string(filepath.Separator)) && realPath != realRoot {
				return "", errSymlinkEscapes
			}
		}
	}
	return absPath, nil
}

// createWorkspace creates the task's workspace dir, initialises a git
// repo on the default branch, and seeds an initial empty commit so
// `git status --porcelain` has a baseline to diff against (used by
// upstream's planned BashExecutor verify_clean flag).
//
// Returns true if the workspace was created, false if it already
// existed (idempotent — repeated creates return "exists" not error).
//
// Caller must hold the per-task mutex.
func (s *Server) createWorkspace(ctx context.Context, taskID string) (bool, error) {
	dir := filepath.Join(s.workspaceRoot, taskID)
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return false, &os.PathError{Op: "stat", Path: dir, Err: os.ErrExist}
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := initGitRepo(ctx, dir); err != nil {
		// Roll back the dir so a retry can re-create cleanly.
		_ = os.RemoveAll(dir)
		return false, err
	}
	return true, nil
}

// initGitRepo runs `git init -b main` + `git config user.{email,name}`
// + `git commit --allow-empty` so the workspace is a usable git repo
// with one commit on main. The commit makes verify_clean's
// `git status --porcelain` baseline meaningful (the worktree is clean
// when no further mutations happen vs the initial commit).
func initGitRepo(ctx context.Context, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, gitInitTimeout)
	defer cancel()

	steps := [][]string{
		{"git", "init", "-b", defaultBranch},
		{"git", "config", "user.email", "sandbox@semteams"},
		{"git", "config", "user.name", "Sandbox"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range steps {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = dir
		// Don't inherit the parent's env (unit tests on hosts with
		// idiosyncratic GIT_* vars would silently misbehave).
		cmd.Env = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=" + dir,
			"GIT_AUTHOR_NAME=Sandbox",
			"GIT_AUTHOR_EMAIL=sandbox@semteams",
			"GIT_COMMITTER_NAME=Sandbox",
			"GIT_COMMITTER_EMAIL=sandbox@semteams",
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return &os.PathError{Op: strings.Join(args, " "), Path: dir, Err: errFromOutput(err, out)}
		}
	}
	return nil
}

// errFromOutput wraps a git command failure with stderr context so
// debugging from logs is possible. Without this, "exit status 128" is
// the only signal a caller has. Uses fmt.Errorf %w so callers that
// errors.Is/As against ExitError still match.
func errFromOutput(err error, out []byte) error {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, trimmed)
}

// removeWorkspace deletes the task's workspace dir. No-op if it doesn't
// exist (idempotent — DELETE on a missing worktree returns success).
//
// Before RemoveAll, recursively chmod-w on every entry so chmod-locked
// read_only_paths can be removed. Without this, RemoveAll would fail
// with EACCES inside any frozen subtree.
//
// Caller must hold the per-task mutex.
func (s *Server) removeWorkspace(taskID string) error {
	dir := filepath.Join(s.workspaceRoot, taskID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	if err := chmodTreeWritable(dir); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// chmodTreeWritable walks dir and adds u+w (0200) to every file and
// dir, so a subsequent RemoveAll succeeds even when read_only_paths
// chmod'd things to 555/444.
func chmodTreeWritable(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Re-add u+w. For dirs we also need u+x (already present from
		// 555 → 755). For files we need u+w only.
		newMode := info.Mode() | 0o200
		if d.IsDir() {
			newMode |= 0o700
		}
		return os.Chmod(path, newMode.Perm())
	})
}

// workspaceExists reports whether the task's workspace dir exists.
// Caller must hold the per-task mutex if the result will be used to
// gate further filesystem ops.
func (s *Server) workspaceExists(taskID string) bool {
	dir := filepath.Join(s.workspaceRoot, taskID)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// applyReadOnlyChmod walks each path in roPaths (relative to the
// task's workspace root) and recursively chmods to 555 (dirs) / 444
// (files). Idempotent — already-frozen trees stay frozen. Paths that
// don't exist yet are silently skipped (chmod will apply on a future
// sweep once the path is materialised).
//
// Caller must hold the per-task mutex (we walk the filesystem while
// holding state-modification rights).
func (s *Server) applyReadOnlyChmod(taskID string, roPaths []string) error {
	if len(roPaths) == 0 {
		return nil
	}
	taskRoot := filepath.Join(s.workspaceRoot, taskID)
	var firstErr error
	for _, rel := range roPaths {
		abs := filepath.Join(taskRoot, filepath.Clean(rel))
		// Defense-in-depth prefix check; validateReadOnlyPath at create
		// time already rejected escape attempts, but a future code path
		// adding paths post-create could miss the check.
		if !strings.HasPrefix(abs, taskRoot+string(filepath.Separator)) && abs != taskRoot {
			if firstErr == nil {
				firstErr = errPathEscapes
			}
			continue
		}
		if _, statErr := os.Stat(abs); os.IsNotExist(statErr) {
			continue
		}
		if err := chmodTreeReadOnly(abs); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// chmodTreeReadOnly recursively chmods abs to 555 (dirs) / 444
// (files). Idempotent. Symlinks are skipped — os.Chmod follows them,
// which would attempt to mutate permissions on the target (potentially
// outside the workspace, EPERM as non-root). The file API refuses
// symlink writes anyway, so freezing them serves no purpose.
func chmodTreeReadOnly(abs string) error {
	return filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		var mode os.FileMode = 0o444
		if d.IsDir() {
			mode = 0o555
		}
		return os.Chmod(path, mode)
	})
}

// zipDir streams a deflate-compressed zip archive of dir's contents to
// w. Symlinks are skipped defensively. Read-only files (frozen via
// chmod 444) are still readable here — read access is intentional for
// forensics.
func zipDir(w io.Writer, dir string) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dir {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		if d.IsDir() {
			header.Name += "/"
		}
		zf, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(zf, f)
		return err
	})
}
