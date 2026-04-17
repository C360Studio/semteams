//go:build integration

package teamsmemory_test

import (
	"crypto/rand"
	"encoding/hex"
)

// uniqueSuffix returns an 8-hex-char random suffix for JetStream consumer
// names. Used by onboarding integration tests so concurrent runs (and same-
// second runs across parallel test binaries) don't reuse durable consumer
// names. Collision space is 2^32 — effectively never collides for
// per-test-function cardinality.
func uniqueSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
