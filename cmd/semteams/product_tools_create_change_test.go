package main

import (
	"log/slog"
	"testing"

	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/tools/emitchange"
)

// TestRegisterCreateChangeTools_NilNATSSkips covers the nil-NATS boot
// path: a deployment without a live NATS client can't honor the
// triple-stamp contract, so emit_change must be skipped (not registered)
// rather than registered-but-broken. Mirrors the nil-NATS posture of
// registerDevViaTestTools / registerAutoresearchTools.
//
// The happy registration path (natsClient present → tool registered) is
// covered by the emitchange package's executor tests + the create_change
// mock-LLM journey gate (the same effort-to-confidence split as
// product_tools_bash_test.go's attestation-mode note): constructing a
// real *natsclient.Client here would require a live NATS server.
func TestRegisterCreateChangeTools_NilNATSSkips(t *testing.T) {
	reg := agentictools.NewExecutorRegistry()

	err := registerCreateChangeTools(reg, nil, types.PlatformMeta{Org: "c360", Platform: "ops"}, slog.Default())
	if err != nil {
		t.Fatalf("registerCreateChangeTools(nil nats) returned error: %v", err)
	}
	if got := reg.GetTool(emitchange.ToolName); got != nil {
		t.Errorf("emit_change registered despite nil NATS client; want skipped (got %T)", got)
	}
}
