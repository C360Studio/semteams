package main

import (
	"log/slog"
	"testing"

	"github.com/c360studio/semstreams/types"
)

// TestBuildBashExecutor_LocalMode covers the no-SANDBOX_URL path: no
// remote runner is configured, BashExecutor falls back to local exec.
// This is the dev-shell shape (stack not up, running `task build` from
// a plain terminal). Pre-Phase-D and post-Phase-D behavior must be
// identical.
func TestBuildBashExecutor_LocalMode(t *testing.T) {
	t.Setenv(envSandboxURL, "")

	mode, exec := buildBashExecutor(nil, types.PlatformMeta{Org: "c360", Platform: "ops"}, slog.Default())

	if mode != "local" {
		t.Fatalf("mode wrong: got %q want %q", mode, "local")
	}
	if exec == nil {
		t.Fatalf("buildBashExecutor returned nil executor")
	}
}

// TestBuildBashExecutor_SandboxOnlyMode covers SANDBOX_URL-set,
// no-NATS path: remote runner is the default *runner.Client (always-
// warm sandbox), no AttestationRunner wrap because we can't read
// chain entities without NATS. Pre-Phase-D and post-Phase-D behavior
// must be identical here too — guards against a regression that would
// silently activate attestation routing without a working chain
// reader.
func TestBuildBashExecutor_SandboxOnlyMode(t *testing.T) {
	t.Setenv(envSandboxURL, "http://sandbox:8090")

	mode, exec := buildBashExecutor(nil, types.PlatformMeta{Org: "c360", Platform: "ops"}, slog.Default())

	if mode != "sandbox" {
		t.Fatalf("mode wrong: got %q want %q", mode, "sandbox")
	}
	if exec == nil {
		t.Fatalf("buildBashExecutor returned nil executor")
	}
}

// Attestation mode (SANDBOX_URL set + natsClient non-nil) is exercised
// end-to-end by the Phase E real-LLM autoresearch smoke. A unit test
// here would require constructing a real *natsclient.Client (the
// function signature takes the concrete type, not an interface), which
// in turn requires a NATS server to dial. The wiring inside
// buildBashExecutor for that branch is straightforward and reviewer-
// audited; deferring its coverage to the integration path matches the
// effort-to-confidence ratio.
