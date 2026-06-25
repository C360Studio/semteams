package requestsandbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

type fakeManager struct {
	lastReq           sandboxmanager.SandboxRequirements
	lastChainEntityID string
	att               sandboxmanager.Attestation
	err               error
	catalog           *sandboxmanager.Catalog
}

func (f *fakeManager) Request(_ context.Context, chainEntityID string, req sandboxmanager.SandboxRequirements) (sandboxmanager.Attestation, error) {
	f.lastChainEntityID = chainEntityID
	f.lastReq = req
	return f.att, f.err
}

func (f *fakeManager) Catalog() *sandboxmanager.Catalog {
	return f.catalog
}

func newPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

func happyAttestation() sandboxmanager.Attestation {
	return sandboxmanager.Attestation{
		ProfileID:        "go-backend@v1",
		ImageDigest:      "sha256:abc",
		Ready:            true,
		AttestedAt:       time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC),
		AdmissionOutcome: sandboxmanager.AdmissionAdmitted,
		Verified:         map[string]string{"go": "go version go1.25.4"},
		RequirementsHash: "reqhash",
		Signature:        "sig123",
		TTL:              24 * time.Hour,
	}
}

func makeCall(args map[string]any, related map[string]any, loopID string) agentic.ToolCall {
	call := agentic.ToolCall{
		ID:        "call-1",
		Name:      ToolName,
		Arguments: args,
		LoopID:    loopID,
		Metadata:  map[string]any{},
	}
	if related != nil {
		call.Metadata[agentic.MetadataKeyRelatedLoops] = related
	}
	return call
}

func TestExecute_HappyPath_StampsAndReturnsAttestation(t *testing.T) {
	mgr := &fakeManager{att: happyAttestation()}
	exec := NewExecutor(mgr, newPlatform(), nil)

	call := makeCall(
		map[string]any{
			"languages": []any{"go"},
			"verification": []any{
				map[string]any{"name": "go", "command": "go version"},
			},
		},
		map[string]any{
			"chain-entity-id": "c360.ops.agent.chain.execution.testchain",
		},
		"loop-1",
	)

	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("expected no error, got %q (%s)", res.Error, res.ErrorKind)
	}
	if mgr.lastChainEntityID != "c360.ops.agent.chain.execution.testchain" {
		t.Fatalf("chain entity not from related_loops: %q", mgr.lastChainEntityID)
	}
	if len(mgr.lastReq.Languages) != 1 || mgr.lastReq.Languages[0] != "go" {
		t.Fatalf("language not parsed: %+v", mgr.lastReq.Languages)
	}
	var wire attestationWire
	if err := json.Unmarshal([]byte(res.Content), &wire); err != nil {
		t.Fatalf("content not valid json: %v", err)
	}
	if !wire.Ready {
		t.Fatalf("ready=false in wire: %+v", wire)
	}
	if wire.Profile != "go-backend@v1" {
		t.Fatalf("profile wrong: %q", wire.Profile)
	}
	if got := res.Metadata["ready"]; got != true {
		t.Fatalf("metadata ready wrong: %v", got)
	}
}

func TestExecute_RunAnchorFallback_DerivesChainEntityFromMetadata(t *testing.T) {
	// No related_loops pin → fall back to the dispatch-stamped run anchor on
	// ToolCall.Metadata (semstreams#250). With only MetadataKeyRunID present,
	// runanchor reconstructs the 6-part entity from org/platform.
	mgr := &fakeManager{att: happyAttestation()}
	exec := NewExecutor(mgr, newPlatform(), nil)

	call := makeCall(map[string]any{"languages": []any{"go"}}, nil, "loop-with-chain")
	call.Metadata[agentic.MetadataKeyRunID] = "resolvedchain"

	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("expected no error, got %q", res.Error)
	}
	want := "c360.ops.agent.chain.execution.resolvedchain"
	if mgr.lastChainEntityID != want {
		t.Fatalf("chain entity ID wrong: got %q want %q", mgr.lastChainEntityID, want)
	}
}

func TestExecute_FrontDoorLoopIDFallback_DerivesExistingLoopEntity(t *testing.T) {
	mgr := &fakeManager{att: happyAttestation()}
	exec := NewExecutor(mgr, newPlatform(), nil)

	call := makeCall(map[string]any{"languages": []any{"go"}}, nil, "loop-1")
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("expected no error, got %q", res.Error)
	}
	want := "c360.ops.agent.agentic-loop.execution.loop-1"
	if mgr.lastChainEntityID != want {
		t.Fatalf("chain entity ID wrong: got %q want %q", mgr.lastChainEntityID, want)
	}
}

func TestExecute_NoResolverNoRelatedAndNoLoopID_Errors(t *testing.T) {
	mgr := &fakeManager{att: happyAttestation()}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := makeCall(map[string]any{"languages": []any{"go"}}, nil, "")
	res, _ := exec.Execute(context.Background(), call)
	if res.Error == "" {
		t.Fatalf("expected error when no chain entity is resolvable")
	}
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Fatalf("wrong error kind: %s", res.ErrorKind)
	}
	if !strings.Contains(res.Error, "chain-entity-id") {
		t.Fatalf("error should mention chain-entity-id: %q", res.Error)
	}
}

func TestExecute_AdmissionPending_ReturnsNormally(t *testing.T) {
	mgr := &fakeManager{att: sandboxmanager.Attestation{
		ProfileID:        "full-stack-e2e@v1",
		AttestedAt:       time.Now().UTC(),
		AdmissionOutcome: sandboxmanager.AdmissionPending,
		AdmissionReasons: []string{"privilege:docker-socket requires chain admission-reviewer approval"},
		RequirementsHash: "rh",
		Signature:        "sig",
		TTL:              24 * time.Hour,
	}}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := makeCall(
		map[string]any{
			"languages":  []any{"go", "node"},
			"privileges": []any{"docker-socket"},
		},
		map[string]any{"chain-entity-id": "c360.ops.agent.chain.execution.c1"},
		"loop-1",
	)
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("admission_pending must NOT surface as ToolResult.Error; got %q", res.Error)
	}
	var wire attestationWire
	if err := json.Unmarshal([]byte(res.Content), &wire); err != nil {
		t.Fatalf("json: %v", err)
	}
	if wire.AdmissionOutcome != string(sandboxmanager.AdmissionPending) {
		t.Fatalf("admission outcome not propagated: %s", wire.AdmissionOutcome)
	}
}

func TestExecute_AdmissionDenied_Terminal_ReturnsNormally(t *testing.T) {
	mgr := &fakeManager{att: sandboxmanager.Attestation{
		ProfileID:        "go-backend@v1",
		AttestedAt:       time.Now().UTC(),
		AdmissionOutcome: sandboxmanager.AdmissionDenied,
		Terminal:         true,
		TerminalReason:   "admission denied: secret:OPENAI_API_KEY not pre-provisioned by operator",
		AdmissionReasons: []string{"secret:OPENAI_API_KEY not pre-provisioned by operator"},
		RequirementsHash: "rh",
		Signature:        "sig",
		TTL:              24 * time.Hour,
	}}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := makeCall(
		map[string]any{
			"languages": []any{"go"},
			"secrets":   []any{"OPENAI_API_KEY"},
		},
		map[string]any{"chain-entity-id": "c360.ops.agent.chain.execution.c1"},
		"loop-1",
	)
	res, _ := exec.Execute(context.Background(), call)
	if res.Error != "" {
		t.Fatalf("denied attestation must NOT surface as ToolResult.Error; Coordinator routes on Terminal field")
	}
	var wire attestationWire
	json.Unmarshal([]byte(res.Content), &wire)
	if !wire.Terminal {
		t.Fatalf("Terminal not propagated to wire")
	}
}

func TestExecute_ManagerErr_SurfacesAsNetwork(t *testing.T) {
	mgr := &fakeManager{att: happyAttestation(), err: errors.New("publish failed")}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := makeCall(
		map[string]any{"languages": []any{"go"}},
		map[string]any{"chain-entity-id": "c360.ops.agent.chain.execution.c1"},
		"loop-1",
	)
	res, _ := exec.Execute(context.Background(), call)
	if res.Error == "" || res.ErrorKind != agentic.ToolErrorNetwork {
		t.Fatalf("expected network error, got %q (%s)", res.Error, res.ErrorKind)
	}
}

func TestExecute_ManagerInvalidErr_SurfacesAsInternal(t *testing.T) {
	mgr := &fakeManager{
		att: happyAttestation(),
		err: errs.ClassifiedCode(
			errs.ErrorInvalid,
			"entity_not_found",
			errors.New("entity not found"),
		),
	}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := makeCall(
		map[string]any{"languages": []any{"go"}},
		map[string]any{"chain-entity-id": "c360.ops.agent.chain.execution.c1"},
		"loop-1",
	)
	res, _ := exec.Execute(context.Background(), call)
	if res.Error == "" || res.ErrorKind != agentic.ToolErrorInternal {
		t.Fatalf("expected internal error, got %q (%s)", res.Error, res.ErrorKind)
	}
}

func TestExecute_BadArgs_InvalidArgs(t *testing.T) {
	mgr := &fakeManager{att: happyAttestation()}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := makeCall(
		map[string]any{"languages": []any{42}}, // not a string
		map[string]any{"chain-entity-id": "c360.ops.agent.chain.execution.c1"},
		"loop-1",
	)
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Fatalf("expected invalid_args, got %s; err=%q", res.ErrorKind, res.Error)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	mgr := &fakeManager{att: happyAttestation()}
	exec := NewExecutor(mgr, newPlatform(), nil)
	call := agentic.ToolCall{ID: "c", Name: "not_request_sandbox"}
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Fatalf("expected not_found, got %s", res.ErrorKind)
	}
}

func TestParseArgs_ParsesAllFields(t *testing.T) {
	args := map[string]any{
		"languages":  []any{"go", "node"},
		"tools":      []any{"task"},
		"services":   []any{"nats"},
		"network":    "restricted",
		"secrets":    []any{"OPENAI_API_KEY"},
		"mounts":     []any{"workspace-write"},
		"privileges": []any{"docker-socket"},
		"verification": []any{
			map[string]any{"name": "go", "command": "go version", "expect_exit": float64(0)},
			map[string]any{"name": "task", "command": "task --list"},
		},
	}
	out, err := parseArgs(args)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(out.Languages) != 2 || out.Languages[1] != "node" {
		t.Fatalf("languages wrong: %v", out.Languages)
	}
	if out.Network != sandboxmanager.NetworkRestricted {
		t.Fatalf("network wrong: %q", out.Network)
	}
	if len(out.Privileges) != 1 || out.Privileges[0] != sandboxmanager.PrivilegeDockerSocket {
		t.Fatalf("privileges wrong: %v", out.Privileges)
	}
	if len(out.Verification) != 2 {
		t.Fatalf("verification len wrong: %d", len(out.Verification))
	}
	if out.Verification[1].ExpectExit != 0 {
		t.Fatalf("default expect_exit wrong: %d", out.Verification[1].ExpectExit)
	}
}

func TestParseArgs_ProbeExpectExitTypeFlexible(t *testing.T) {
	cases := []any{float64(2), int(2), json.Number("2")}
	for _, v := range cases {
		args := map[string]any{
			"verification": []any{
				map[string]any{"name": "x", "command": "echo", "expect_exit": v},
			},
		}
		out, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parse %T: %v", v, err)
		}
		if out.Verification[0].ExpectExit != 2 {
			t.Fatalf("ExpectExit %T did not parse to 2: got %d", v, out.Verification[0].ExpectExit)
		}
	}
}

func TestParseArgs_RejectsBadVerificationShape(t *testing.T) {
	args := map[string]any{
		"verification": []any{"not an object"},
	}
	if _, err := parseArgs(args); err == nil {
		t.Fatalf("expected error on bad probe shape")
	}
}

func TestNew_PanicsOnMissingDeps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil manager")
		}
	}()
	NewExecutor(nil, newPlatform(), nil)
}

func TestNew_PanicsOnEmptyPlatform(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on empty platform")
		}
	}()
	NewExecutor(&fakeManager{}, types.PlatformMeta{}, nil)
}
