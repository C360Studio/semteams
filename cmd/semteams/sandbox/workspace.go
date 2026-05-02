package main

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Path-confinement failure sentinels. All map to HTTP 403 in handlers
// today; distinct values exist so logs and future tests can
// distinguish failure shapes.
var (
	errAbsolutePath   = errors.New("absolute paths are not allowed")
	errPathEscapes    = errors.New("path escapes workspace")
	errSymlinkEscapes = errors.New("symlink escapes workspace")
)

// resolveChainPath maps a workspace-relative request path to an
// absolute filesystem path inside the chain's workspace, rejecting
// any attempt to escape via:
//   - absolute path
//   - .. traversal (filepath.Clean + prefix check)
//   - symlink whose target resolves outside the workspace
//
// Forward-flag for R3.6.1.c (when bash exec lands and CAN create
// symlinks): this check resolves the target's symlink chain via
// EvalSymlinks but does not lstat each parent component. A bash
// command that creates a symlinked subdir could let a subsequent
// write_file follow that symlink. R3.6.1.c should add per-component
// lstat or O_NOFOLLOW on the underlying open. R3.6.1.b's file API
// alone cannot create symlinks, so the gap is theoretical for this
// slice.
func (s *Server) resolveChainPath(chainID, relPath string) (string, error) {
	// Handlers validate chainID before reaching here; defense-in-depth
	// re-check guards future callers that forget. Returning an error
	// rather than panicking keeps this drop-in for tests and tools.
	if !isValidChainID(chainID) {
		return "", errors.New("invalid chain ID")
	}
	if filepath.IsAbs(relPath) {
		return "", errAbsolutePath
	}

	chainRoot := filepath.Join(s.workspaceRoot, chainID)
	absPath := filepath.Join(chainRoot, filepath.Clean(relPath))

	// Prefix check: after Clean, absPath must be chainRoot itself or
	// a descendant. The Separator suffix is required so e.g.
	// "/workspace/chain1" doesn't false-positive on
	// "/workspace/chain12".
	if !strings.HasPrefix(absPath, chainRoot+string(filepath.Separator)) && absPath != chainRoot {
		return "", errPathEscapes
	}

	// Symlink-escape check: if the target exists, EvalSymlinks
	// resolves any symlinks in the path; verify the resolved path is
	// still inside the workspace. If the target doesn't exist (write
	// to a new file), EvalSymlinks fails with ENOENT and the prefix
	// check above is the only line of defense for this slice.
	if absPath != chainRoot {
		realPath, err := filepath.EvalSymlinks(absPath)
		if err == nil {
			realRoot, rootErr := filepath.EvalSymlinks(chainRoot)
			if rootErr != nil {
				realRoot = chainRoot
			}
			if !strings.HasPrefix(realPath, realRoot+string(filepath.Separator)) && realPath != realRoot {
				return "", errSymlinkEscapes
			}
		}
	}
	return absPath, nil
}

// createWorkspace creates an empty workspace dir for chainID. The bool
// return is true if the directory was created, false if it already
// existed (idempotent — repeated POSTs return "exists" not error).
//
// Caller must hold the per-chain mutex. The stat-then-mkdir pair would
// race otherwise.
func (s *Server) createWorkspace(chainID string) (bool, error) {
	dir := filepath.Join(s.workspaceRoot, chainID)
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
	return true, nil
}

// removeWorkspace deletes the workspace dir for chainID. No-op if it
// doesn't exist (idempotent — DELETE on a missing workspace returns
// success).
//
// Caller must hold the per-chain mutex.
func (s *Server) removeWorkspace(chainID string) error {
	dir := filepath.Join(s.workspaceRoot, chainID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dir)
}

// workspaceExists reports whether the workspace dir for chainID exists.
// Caller must hold the per-chain mutex if the result will be used to
// gate further filesystem ops (the stat-then-act pattern).
func (s *Server) workspaceExists(chainID string) bool {
	dir := filepath.Join(s.workspaceRoot, chainID)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// zipDir streams a deflate-compressed zip archive of dir's contents to
// w. Symlinks are skipped defensively — the file API in R3.6.1.b
// rejects symlink writes, but archiving any pre-existing symlink would
// be a workspace-escape footgun.
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
