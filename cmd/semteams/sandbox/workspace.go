package main

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

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
