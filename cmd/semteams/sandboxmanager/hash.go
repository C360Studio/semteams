package sandboxmanager

import "fmt"

// fnv12 hashes s with FNV-1a 64 and returns the first 12 hex chars.
// Inline (no hash/fnv import) to keep the function self-contained
// and the value stable across Go versions (Go's hash/fnv constants
// are pinned in the stdlib). Used by both SandboxAPIRunner (for
// sandbox task_id derivation) and MockRunner (for deterministic
// ContainerID + ImageDigest).
func fnv12(s string) string {
	const (
		offset64 uint64 = 14695981039346656037
		prime64  uint64 = 1099511628211
	)
	h := offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return fmt.Sprintf("%012x", h)[:12]
}
