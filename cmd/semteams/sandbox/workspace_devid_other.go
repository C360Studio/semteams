//go:build !unix

package main

import "os"

// sameDevice is the best-effort fallback on platforms without
// syscall.Stat_t.Dev (e.g. Windows). Always returns true — the bind-mount
// detection is simply disabled, and zipDir behaves as it did before the
// ADR-040 guardrail was added. The primary supported platforms
// (Linux + macOS) use workspace_devid_unix.go.
//
// ADR-040 addendum 2026-05-10 §item 6 — bind-mount skip guardrail for
// zipDir / handleZipWorkspace.
func sameDevice(_ os.FileInfo, _ os.FileInfo) bool {
	return true
}
