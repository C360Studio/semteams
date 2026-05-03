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
//   - parent-dir component that's a symlink (R3.6.1.c hardening)
//
// Threat-model note for the per-component lstat: the per-chain mutex
// held by handlers serialises every op within a chain, so there's no
// race window between resolveChainPath returning and the subsequent
// open / read / write. A bash exec on chain X holds the mutex for the
// full command; a concurrent write_file on chain X waits behind it.
// Cross-chain ops touch independent mutexes and can't influence each
// other's directory state.
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

	// Per-component lstat: walk every existing component from chainRoot
	// toward absPath; refuse if any is a symlink. Closes the
	// parent-dir-symlink gap left open in R3.6.1.b (when only the file
	// API existed and couldn't plant symlinks). R3.6.1.c bash exec can
	// plant them, so this becomes load-bearing.
	if absPath != chainRoot {
		rel := strings.TrimPrefix(absPath, chainRoot+string(filepath.Separator))
		cur := chainRoot
		for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
			if part == "" {
				continue
			}
			cur = filepath.Join(cur, part)
			info, err := os.Lstat(cur)
			if os.IsNotExist(err) {
				// Remaining components don't exist yet. MkdirAll/WriteFile
				// will create real dirs/files; symlinks can't appear in the
				// gap because the chain mutex serialises ops.
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
