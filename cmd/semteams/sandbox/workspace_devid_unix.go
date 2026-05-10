//go:build unix

package main

import (
	"os"
	"syscall"
)

// sameDevice reports whether info and rootInfo describe entries on the
// same block device. A bind mount surfaces as a different device ID
// from the workspace root, so this check detects (and allows skipping)
// any subtree that has been mounted from an external source.
//
// Implementation uses syscall.Stat_t.Dev, which is available on Linux
// and macOS (both covered by the "unix" build tag). See also
// workspace_devid_other.go for the !unix fallback.
//
// ADR-040 addendum 2026-05-10 §item 6 — bind-mount skip guardrail for
// zipDir / handleZipWorkspace.
func sameDevice(rootInfo os.FileInfo, info os.FileInfo) bool {
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return true // can't compare; assume same device (safe fallback)
	}
	infoStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return rootStat.Dev == infoStat.Dev
}
