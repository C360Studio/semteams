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

// mintPointSuffixes are the exactly-3 root spawns that mint an AgentRun in
// Phase 2 (run_scope="new"). The coordinator's own loop entity is the run root;
// downstream spawns inherit transitively via the default inherit branch
// (agentic-loop stamps agent.run on every spawned loop), so they carry NO
// run_scope. Path suffixes (matched against the walked, slash-normalized paths)
// rather than full relative paths to stay robust to the test's CWD.
var mintPointSuffixes = []string{
	"configs/rules/research/01-coordinator-research-spawn.json",
	"configs/rules/autoresearch/01-coordinator-autoresearch-spawn.json",
	"configs/rules/dev-via-test/01-coordinator-dev-via-test-spawn.json",
}

// TestPhase2RunScopeOnlyAtMintPoints is the ADR-053 Phase-2 topology guard.
//
// Two structural invariants of the mint phase:
//  1. run_scope="new" appears at EXACTLY the 3 coordinator root spawns and
//     nowhere else. A stray run_scope on a downstream rule would mint a second
//     run (mis-anchoring the chain's run identity) or mint off a non-loop
//     entity (silent inherit-fallback). Every run_scope value must be "new" —
//     "inherit"/"none" are the framework default/opt-out and would be noise here.
//  2. NO lifecycle_* action types yet. Those (lifecycle_transition/complete/fail)
//     are Phase 4 — and Phase 4 is where the agentrun.MilestoneSubscriber gets
//     wired (its D3 zombie guard only has runs to act on once a transition rule
//     advances them past "dispatched"). This half of the guard trips the day a
//     lifecycle_* rule lands, pointing the Phase-4 author at the subscriber
//     wiring (mirror upstream cmd/semstreams/main.go §10d).
//
// Phase 2 itself wires NO subscriber: minted runs sit inert in "dispatched"
// with no consumer, so minting is additive/safe. See docs/adr/053-adoption-plan.md.
func TestPhase2RunScopeOnlyAtMintPoints(t *testing.T) {
	// Mint points live in the rule packs; the two flow configs are scanned too
	// so a stray run_scope pasted into an inline flow rule is still caught.
	files := collectJSONFiles(t,
		"../../configs/rules",
		"../../configs/flow-bootstrap.json",
		"../../configs/e2e-flow-bootstrap.json",
	)
	if len(files) == 0 {
		t.Fatal("no rule/flow JSON files found — test would pass vacuously; check the scan roots")
	}

	runScopeFiles := map[string]bool{}
	var badValues, lifecycleActions []string
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // test-controlled config paths
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", f, err)
		}
		norm := filepath.ToSlash(f)
		scanRunTokens(doc, norm, runScopeFiles, &badValues, &lifecycleActions)
	}

	// Invariant 1a: every run_scope is "new".
	if len(badValues) > 0 {
		t.Errorf("run_scope values other than \"new\" found (use the default for inherit/none):\n  %s",
			strings.Join(badValues, "\n  "))
	}
	// Invariant 1b: run_scope appears at exactly the 3 mint points.
	for _, suffix := range mintPointSuffixes {
		if !hasSuffixIn(runScopeFiles, suffix) {
			t.Errorf("expected run_scope=\"new\" at mint point %q, but none found there", suffix)
		}
	}
	for f := range runScopeFiles {
		if !matchesAnySuffix(f, mintPointSuffixes) {
			t.Errorf("run_scope found OUTSIDE the 3 mint points: %s — a downstream spawn must inherit "+
				"(omit run_scope), not mint a second run", f)
		}
	}
	// Invariant 2: no lifecycle_* actions until Phase 4 (which also wires the subscriber).
	if len(lifecycleActions) > 0 {
		t.Fatalf("lifecycle_* action(s) found — this is Phase 4 territory:\n  %s\n\n"+
			"Phase 4 adds terminal-authority transitions AND must wire the agentrun.MilestoneSubscriber\n"+
			"in cmd/semteams/main.go (upstream §10d) so D3 zombie-prevention is live. Do both, then\n"+
			"update this guard.", strings.Join(lifecycleActions, "\n  "))
	}
}

func hasSuffixIn(files map[string]bool, suffix string) bool {
	for f := range files {
		if strings.HasSuffix(f, suffix) {
			return true
		}
	}
	return false
}

func matchesAnySuffix(file string, suffixes []string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(file, s) {
			return true
		}
	}
	return false
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

// scanRunTokens recursively records, for the given file: which files carry a
// run_scope key (runScopeFiles), any run_scope value other than "new"
// (badValues), and any action with a lifecycle_* type (lifecycleActions).
func scanRunTokens(v any, file string, runScopeFiles map[string]bool, badValues, lifecycleActions *[]string) {
	switch node := v.(type) {
	case map[string]any:
		if rs, ok := node["run_scope"]; ok {
			runScopeFiles[file] = true
			if s, _ := rs.(string); s != "new" {
				*badValues = append(*badValues, fmt.Sprintf("%s: run_scope=%v", file, rs))
			}
		}
		if typ, ok := node["type"].(string); ok && strings.HasPrefix(typ, "lifecycle_") {
			*lifecycleActions = append(*lifecycleActions, fmt.Sprintf("%s: action type %q", file, typ))
		}
		for _, val := range node {
			scanRunTokens(val, file, runScopeFiles, badValues, lifecycleActions)
		}
	case []any:
		for _, item := range node {
			scanRunTokens(item, file, runScopeFiles, badValues, lifecycleActions)
		}
	}
}
