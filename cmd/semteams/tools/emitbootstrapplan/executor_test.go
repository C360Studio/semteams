package emitbootstrapplan

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/sandboxfleet"
)

// platform returns a deterministic PlatformMeta for tests. Mirrors
// the sibling verify/committed tools' test helper.
func platform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

type fakePub struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (f *fakePub) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakePub) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, ts...)
	return nil
}

func (f *fakePub) findPredicate(pred string) (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.triples {
		if t.Predicate == pred {
			return t.Object, true
		}
	}
	return nil, false
}

type fakeRegistry struct {
	calls []registryCall
	err   error
}

type registryCall struct {
	sig      sandboxfleet.TargetSignature
	planHash string
}

func (f *fakeRegistry) MarkProvisioning(_ context.Context, sig sandboxfleet.TargetSignature, planHash string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, registryCall{sig: sig, planHash: planHash})
	return nil
}

func baseArgs() map[string]any {
	return map[string]any{
		"command":                  "task test:integration",
		"repo_url":                 "https://github.com/c360studio/semteams",
		"repo_ref":                 "main",
		"toolchain":                map[string]any{"go": "1.26.0"},
		"base_image":               "ubuntu:24.04",
		"clone_command":            "git clone https://github.com/c360studio/semteams /workspace",
		"install_steps":            []any{"apt-get update", "apt-get install -y golang"},
		"volume_mounts":            []any{"semteams-tenant-x-workspace:/workspace"},
		"docker_socket_mount":      true,
		"verify_command":           "go test ./internal/quickcheck/...",
		"expected_smoke_signature": "exit 0",
		"plan_action":              "provision",
		"plan_revision":            float64(1),
	}
}

// callingLoopID is the bare-UUID loop ID assigned to the plan loop
// in test fixtures. The Execute path consumes it via call.LoopID
// to construct the dual-stamp subject entity ID
// (c360.ops.agent.agentic-loop.execution.<callingLoopID>).
const callingLoopID = "plan-loop-A"

func runEntityMetadata() map[string]any {
	return map[string]any{
		agentic.MetadataKeyRelatedLoops: map[string]any{
			"run-loop-entity-id": "c360.ops.agent.agentic-loop.execution.coord-1",
			"bootstrap-run":      "coord-1",
		},
	}
}

func TestExecutor_HappyProvision(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{}
	e := NewExecutor(pub, reg, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID:        "c1",
		LoopID:    callingLoopID,
		Name:      ToolName,
		Arguments: baseArgs(),
		Metadata:  runEntityMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	// Registry was transitioned to provisioning.
	if len(reg.calls) != 1 {
		t.Fatalf("expected 1 MarkProvisioning call, got %d", len(reg.calls))
	}
	if reg.calls[0].planHash == "" {
		t.Errorf("MarkProvisioning called with empty plan_hash")
	}

	// PR 3.1 dual-target stamping: every plan triple appears twice —
	// once with subject=runEntity, once with subject=callingLoop.
	// Run entity stays authoritative for the registry namespace;
	// calling-loop stamp enables rule 02b's $entity.triple.* spawn-
	// prompt substitution to resolve at fire time. See Execute's
	// PR 3.1 comment for the why.
	const runEntity = "c360.ops.agent.agentic-loop.execution.coord-1"
	const callingLoopEntity = "c360.ops.agent.agentic-loop.execution.plan-loop-A"
	runCount, callingCount := 0, 0
	for i, tr := range pub.triples {
		switch tr.Subject {
		case runEntity:
			runCount++
		case callingLoopEntity:
			callingCount++
		default:
			t.Errorf("triple[%d] subject = %q, want one of run/calling-loop entity", i, tr.Subject)
		}
	}
	if runCount == 0 || callingCount == 0 {
		t.Errorf("expected dual-target stamping (run=%d, calling=%d), got asymmetric counts", runCount, callingCount)
	}
	if runCount != callingCount {
		t.Errorf("dual-target stamp count mismatch: run=%d calling=%d (every predicate must land on both subjects)", runCount, callingCount)
	}

	// Key predicates stamped.
	mustHave := []string{
		"sandbox.tenant.signature",
		"sandbox.tenant.container_name",
		"sandbox.tenant.image",
		"sandbox.tenant.workspace",
		"sandbox.tenant.plan_hash",
		"sandbox.tenant.plan_action",
		"sandbox.tenant.canonical_command",
		"sandbox.tenant.canonical_repo_url",
		"sandbox.tenant.canonical_repo_ref",
		"sandbox.tenant.install_steps",
		"sandbox.tenant.verify_command",
	}
	for _, pred := range mustHave {
		if _, ok := pub.findPredicate(pred); !ok {
			t.Errorf("missing predicate %q", pred)
		}
	}

	// container_name and signature must agree with sig
	sig, _ := sandboxfleet.Canonicalize(sandboxfleet.CanonicalizeInput{
		Command:   "task test:integration",
		RepoURL:   "https://github.com/c360studio/semteams",
		RepoRef:   "main",
		Toolchain: map[string]string{"go": "1.26.0"},
		BaseImage: "ubuntu:24.04",
	})
	gotSig, _ := pub.findPredicate("sandbox.tenant.signature")
	if gotSig != sig.Hash() {
		t.Errorf("signature triple = %v, want %v", gotSig, sig.Hash())
	}
	gotName, _ := pub.findPredicate("sandbox.tenant.container_name")
	if gotName != sig.ContainerName() {
		t.Errorf("container_name triple = %v, want %v", gotName, sig.ContainerName())
	}

	// Result body must echo signature + container_name.
	var body map[string]any
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["signature"] != sig.Hash() {
		t.Errorf("body signature = %v, want %v", body["signature"], sig.Hash())
	}
}

func TestExecutor_SkipDoesNotTouchRegistry(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{}
	e := NewExecutor(pub, reg, platform(), slog.Default())

	args := baseArgs()
	args["plan_action"] = "skip"
	// Skip path doesn't require verify_command/expected_smoke either;
	// strip them to confirm validate accepts.
	delete(args, "verify_command")
	delete(args, "expected_smoke_signature")

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", LoopID: callingLoopID, Name: ToolName, Arguments: args, Metadata: runEntityMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if len(reg.calls) != 0 {
		t.Errorf("skip path should not touch registry; got %d calls", len(reg.calls))
	}
	// But plan triples must still stamp for audit.
	if _, ok := pub.findPredicate("sandbox.tenant.signature"); !ok {
		t.Errorf("signature should stamp on skip path too")
	}
}

func TestExecutor_ReprovisionMarksProvisioning(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{}
	e := NewExecutor(pub, reg, platform(), slog.Default())

	args := baseArgs()
	args["plan_action"] = "reprovision"
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c3", LoopID: callingLoopID, Name: ToolName, Arguments: args, Metadata: runEntityMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if len(reg.calls) != 1 {
		t.Errorf("reprovision should mark provisioning; got %d calls", len(reg.calls))
	}
}

func TestExecutor_MissingRunEntityFails(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{}
	e := NewExecutor(pub, reg, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c4", Name: ToolName, Arguments: baseArgs(),
		// no Metadata → no related_loops → no run entity
	})
	if res.Error == "" {
		t.Errorf("expected error when run-loop-entity-id missing")
	}
	if !strings.Contains(res.Error, "run-loop-entity-id") {
		t.Errorf("error should mention missing key, got %q", res.Error)
	}
}

func TestExecutor_InvalidPlanAction(t *testing.T) {
	e := NewExecutor(&fakePub{}, &fakeRegistry{}, platform(), slog.Default())
	args := baseArgs()
	args["plan_action"] = "garbage"
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c5", LoopID: callingLoopID, Name: ToolName, Arguments: args, Metadata: runEntityMetadata(),
	})
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecutor_MissingVerifyCommandOnProvisionFails(t *testing.T) {
	e := NewExecutor(&fakePub{}, &fakeRegistry{}, platform(), slog.Default())
	args := baseArgs()
	delete(args, "verify_command")
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c6", LoopID: callingLoopID, Name: ToolName, Arguments: args, Metadata: runEntityMetadata(),
	})
	if res.Error == "" {
		t.Errorf("expected error when verify_command missing on provision")
	}
}

func TestExecutor_RegistryErrorIsNetwork(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{err: errors.New("nats timeout")}
	e := NewExecutor(pub, reg, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c7", LoopID: callingLoopID, Name: ToolName, Arguments: baseArgs(), Metadata: runEntityMetadata(),
	})
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorNetwork)
	}
}

// TestPlanHash_StableAcrossVariants pins: prose variants of the same
// (signature, recipe) produce the same plan_hash. Order of install
// steps within the LLM-supplied list doesn't matter because the
// canonicalizer sorts them.
func TestPlanHash_StableAcrossVariants(t *testing.T) {
	sig, _ := sandboxfleet.Canonicalize(sandboxfleet.CanonicalizeInput{
		Command:   "task t",
		BaseImage: "ubuntu:24.04",
	})

	p1, err := parseArgs(map[string]any{
		"command":                  "task t",
		"base_image":               "ubuntu:24.04",
		"plan_action":              "provision",
		"verify_command":           "go test",
		"expected_smoke_signature": "ok",
		"install_steps":            []any{"a", "b"},
	})
	if err != nil {
		t.Fatalf("p1: %v", err)
	}
	p2, err := parseArgs(map[string]any{
		"command":                  "task t",
		"base_image":               "ubuntu:24.04",
		"plan_action":              "provision",
		"verify_command":           "go test",
		"expected_smoke_signature": "ok",
		"install_steps":            []any{"b", "a"},
	})
	if err != nil {
		t.Fatalf("p2: %v", err)
	}
	if p1.planHash(sig) != p2.planHash(sig) {
		t.Errorf("plan_hash drift across install_steps order: %s vs %s",
			p1.planHash(sig), p2.planHash(sig))
	}
}

// TestPlanHash_WorkspaceIrrelevant pins reviewer H3: workspace is
// a provisioning side-output, not an input. Two prose framings
// ("/workspace" vs "/workspace/") must NOT fragment the cache.
func TestPlanHash_WorkspaceIrrelevant(t *testing.T) {
	sig, _ := sandboxfleet.Canonicalize(sandboxfleet.CanonicalizeInput{
		Command:   "task t",
		BaseImage: "ubuntu:24.04",
	})

	p1, _ := parseArgs(map[string]any{
		"command":                  "task t",
		"base_image":               "ubuntu:24.04",
		"plan_action":              "provision",
		"verify_command":           "go test",
		"expected_smoke_signature": "ok",
		"workspace":                "/workspace",
	})
	p2, _ := parseArgs(map[string]any{
		"command":                  "task t",
		"base_image":               "ubuntu:24.04",
		"plan_action":              "provision",
		"verify_command":           "go test",
		"expected_smoke_signature": "ok",
		"workspace":                "/workspace/",
	})
	if p1.planHash(sig) != p2.planHash(sig) {
		t.Errorf("workspace variants fragmented plan_hash (reviewer H3 regression): /workspace=%s /workspace/=%s",
			p1.planHash(sig), p2.planHash(sig))
	}
}

// TestPlanHash_DivergesOnRecipeChange pins inverse: changing a
// material recipe field produces a different plan_hash even when the
// signature is the same.
func TestPlanHash_DivergesOnRecipeChange(t *testing.T) {
	sig, _ := sandboxfleet.Canonicalize(sandboxfleet.CanonicalizeInput{
		Command:   "task t",
		BaseImage: "ubuntu:24.04",
	})

	p1, _ := parseArgs(map[string]any{
		"command":                  "task t",
		"base_image":               "ubuntu:24.04",
		"plan_action":              "provision",
		"verify_command":           "go test",
		"expected_smoke_signature": "ok",
		"install_steps":            []any{"a", "b"},
	})
	p2, _ := parseArgs(map[string]any{
		"command":                  "task t",
		"base_image":               "ubuntu:24.04",
		"plan_action":              "provision",
		"verify_command":           "go test",
		"expected_smoke_signature": "ok",
		"install_steps":            []any{"a", "b", "extra"},
	})
	if p1.planHash(sig) == p2.planHash(sig) {
		t.Errorf("expected different plan_hash on different install_steps; both = %s", p1.planHash(sig))
	}
}
