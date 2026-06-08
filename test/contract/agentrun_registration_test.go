package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic/agentrun"
	"github.com/c360studio/semstreams/pkg/lifecycle"
)

// ADR-053 adoption Phase 1 wiring contract (docs/adr/053-adoption-plan.md).
//
// cmd/semteams/main.go §10a constructs lifecycle.NewManager(...) and calls
// agentrun.Register on it, plumbing the result onto svcDeps.LifecycleManager
// so the rule processor factory installs it (factory.go SetLifecycleManager).
// These tests pin that boot-time contract so a silent wiring regression —
// the class of bug [[structural-contract-tests-insufficient]] warns about —
// fails the fast `task test` lane rather than only surfacing in e2e.
//
// Hermetic by construction: Manager.Register and GetWorkflowDefinition touch
// no NATS (registration is an in-memory map keyed by workflow name; the graph
// emitter only fires on Create/Transition). A nil client is therefore safe
// and keeps these tests out of the testcontainers integration lane.

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestAgentRunWorkflowRegisters exercises the exact NewManager → Register
// sequence main.go performs and asserts the agent-run workflow becomes
// resolvable with the ADR-053 D2 entity pattern. The pre-Register negative
// control proves the test actually drives registration rather than passing
// vacuously.
func TestAgentRunWorkflowRegisters(t *testing.T) {
	mgr := lifecycle.NewManager(nil, discardLogger())

	// Negative control: unregistered workflow must not resolve.
	if _, err := mgr.GetWorkflowDefinition(agentrun.WorkflowName); !errors.Is(err, lifecycle.ErrWorkflowNotRegistered) {
		t.Fatalf("pre-Register GetWorkflowDefinition(%q): want ErrWorkflowNotRegistered, got %v", agentrun.WorkflowName, err)
	}

	if err := agentrun.Register(mgr); err != nil {
		t.Fatalf("agentrun.Register: unexpected error: %v", err)
	}

	def, err := mgr.GetWorkflowDefinition(agentrun.WorkflowName)
	if err != nil {
		t.Fatalf("post-Register GetWorkflowDefinition(%q): %v", agentrun.WorkflowName, err)
	}
	if def.Workflow != agentrun.WorkflowName {
		t.Errorf("WorkflowDef.Workflow = %q, want %q", def.Workflow, agentrun.WorkflowName)
	}
	// The entity pattern is the run-anchor shape the Phase 3 rule subject
	// substitution ($entity.triple.agent.run.entity_id) resolves against;
	// pin it so a downstream upstream change is caught here.
	if def.EntityIDPattern != agentrun.EntityIDPattern {
		t.Errorf("WorkflowDef.EntityIDPattern = %q, want %q", def.EntityIDPattern, agentrun.EntityIDPattern)
	}
}

// TestAgentRunRegisterRejectsDoubleWiring documents the register-once-at-boot
// contract: a second Register call (e.g. an accidental double-wire in main.go)
// surfaces as ErrWorkflowAlreadyRegistered instead of silently succeeding.
func TestAgentRunRegisterRejectsDoubleWiring(t *testing.T) {
	mgr := lifecycle.NewManager(nil, discardLogger())

	if err := agentrun.Register(mgr); err != nil {
		t.Fatalf("first agentrun.Register: unexpected error: %v", err)
	}
	if err := agentrun.Register(mgr); !errors.Is(err, lifecycle.ErrWorkflowAlreadyRegistered) {
		t.Fatalf("second agentrun.Register: want ErrWorkflowAlreadyRegistered, got %v", err)
	}
}

// TestPhase1RulePacksUseNoRunScopeOrLifecycleActions is the Phase-1↔Phase-2
// tripwire. Phase 1 wires the lifecycle.Manager but deliberately does NOT start
// the agentrun.MilestoneSubscriber (D3 terminal authority) — safe ONLY while no
// rule mints a run (run_scope="new") or drives a terminal transition
// (lifecycle_* actions). This test pins that invariant structurally so the
// behavior-neutrality claim can't silently rot: the day a Phase-2 author adds
// run_scope or a lifecycle_* action, this fails and points them at the
// MilestoneSubscriber wiring obligation. When that wiring lands, update or
// retire this guard. See docs/adr/053-adoption-plan.md Phases 2 + 4.
func TestPhase1RulePacksUseNoRunScopeOrLifecycleActions(t *testing.T) {
	// Scan the rule packs (where publish_agent / lifecycle_* actions live) and
	// the two flow configs that load them.
	files := collectJSONFiles(t,
		"../../configs/rules",
		"../../configs/flow-bootstrap.json",
		"../../configs/e2e-flow-bootstrap.json",
	)
	if len(files) == 0 {
		t.Fatal("no rule/flow JSON files found — test would pass vacuously; check the scan roots")
	}

	var violations []string
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // test-controlled config paths
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", f, err)
		}
		scanPhase2Tokens(doc, f, &violations)
	}

	if len(violations) > 0 {
		t.Fatalf("ADR-053 Phase-1 invariant broken — found run_scope / lifecycle_* usage:\n  %s\n\n"+
			"Phase 1 wires lifecycle.Manager but NOT agentrun.MilestoneSubscriber (D3). Adding\n"+
			"run_scope=\"new\" or a lifecycle_* action mints/transitions runs that need the\n"+
			"subscriber's terminal authority. Wire it in cmd/semteams/main.go (mirror upstream\n"+
			"cmd/semstreams/main.go §10d) and then update/retire this guard.",
			strings.Join(violations, "\n  "))
	}
}

// collectJSONFiles expands each root into a list of *.json files (a directory
// is walked recursively; a file path is taken as-is).
func collectJSONFiles(t *testing.T, roots ...string) []string {
	t.Helper()
	var out []string
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat %s: %v", root, err)
		}
		if !info.IsDir() {
			out = append(out, root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".json") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return out
}

// scanPhase2Tokens recursively records any "run_scope" key or any action with a
// "type" of the lifecycle_* family — the two markers that break the Phase-1
// behavior-neutrality invariant.
func scanPhase2Tokens(v any, file string, found *[]string) {
	switch node := v.(type) {
	case map[string]any:
		if rs, ok := node["run_scope"]; ok {
			*found = append(*found, fmt.Sprintf("%s: run_scope=%v", file, rs))
		}
		if typ, ok := node["type"].(string); ok && strings.HasPrefix(typ, "lifecycle_") {
			*found = append(*found, fmt.Sprintf("%s: action type %q", file, typ))
		}
		for _, val := range node {
			scanPhase2Tokens(val, file, found)
		}
	case []any:
		for _, item := range node {
			scanPhase2Tokens(item, file, found)
		}
	}
}
