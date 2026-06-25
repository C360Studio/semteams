package contract

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/c360studio/semteams/cmd/semteams/openspec"
)

// TestRootOpenSpecChange_Readable pins the repo-root OpenSpec change as a real
// artifact, not just prose docs. It is the seed brownfield fixture for the
// future ingest path.
func TestRootOpenSpecChange_Readable(t *testing.T) {
	change, err := openspec.ReadChange(filepath.Join(
		"..", "..", "openspec", "changes", "spec-driven-dev-readiness-hitl",
	))
	if err != nil {
		t.Fatalf("ReadChange: %v", err)
	}
	if change.Slug != "spec-driven-dev-readiness-hitl" {
		t.Fatalf("Slug = %q", change.Slug)
	}
	if change.Proposal == nil {
		t.Fatal("Proposal is nil")
	}
	if change.Design == nil {
		t.Fatal("Design is nil")
	}
	if change.Tasks == nil || len(change.Tasks.Sections) != 6 {
		t.Fatalf("Tasks sections = %d, want 6", len(change.Tasks.Sections))
	}
	if len(change.Deltas) != 1 {
		t.Fatalf("Deltas = %d, want 1", len(change.Deltas))
	}

	delta := change.Deltas[0]
	if delta.Capability != "agentic-sdd" {
		t.Fatalf("Delta capability = %q, want agentic-sdd", delta.Capability)
	}
	if len(delta.Added) != 12 {
		t.Fatalf("Added requirements = %d, want 12", len(delta.Added))
	}
	for _, req := range delta.Added {
		if req.Statement == "" {
			t.Fatalf("requirement %q has empty statement", req.Name)
		}
		if len(req.Scenarios) == 0 {
			t.Fatalf("requirement %q has no scenarios", req.Name)
		}
	}

	dst := filepath.Join(t.TempDir(), change.Slug)
	if err := openspec.WriteChange(dst, change); err != nil {
		t.Fatalf("WriteChange: %v", err)
	}
	roundTripped, err := openspec.ReadChange(dst)
	if err != nil {
		t.Fatalf("re-ReadChange: %v", err)
	}
	if diff := cmp.Diff(change, roundTripped, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("root OpenSpec change round-trip mismatch (-want +got):\n%s", diff)
	}

	rendered := openspec.RenderChangeFolder(change)
	for _, want := range []string{
		"# OpenSpec change: spec-driven-dev-readiness-hitl",
		"<!-- openspec/changes/spec-driven-dev-readiness-hitl/proposal.md -->",
		"<!-- openspec/changes/spec-driven-dev-readiness-hitl/design.md -->",
		"<!-- openspec/changes/spec-driven-dev-readiness-hitl/specs/agentic-sdd/spec.md -->",
		"<!-- openspec/changes/spec-driven-dev-readiness-hitl/tasks.md -->",
		"### Skill Import Adapter",
		"Imported markdown is source material, not executable authority.",
		"The system SHALL allow an operator to review, edit, approve, reject, or request revision",
		"The system SHALL export generated OpenSpec artifacts",
		"The system SHALL provide contextual slash-command shortcuts",
		"The system SHALL model claims, proof dependencies, harness profiles, readiness records, evidence, and waivers",
		"Rejected harness profile blocks implementation",
		"The system SHALL preserve a single authority stack for what \"done\" means",
		"Approved spec owns done",
		"Ralph converges but does not redefine done",
		"CBG judges final done",
		"The system SHALL route to autoresearch only when the objective has a specific scalar metric",
		"Non-scalar objective is not autoresearch",
		"Measurement, not prose, decides kept work",
		"Guardrails constrain optimization pressure",
		"Flow contracts remain the control surface",
		"NATS remains the reactive transport",
		"The system SHALL represent spec-driven planning and execution progress as SemStreams flow-native graph facts",
		"NATS transport does not bypass flow contracts",
		"proof.claim.<id>.requires",
		"proof.harness_profile.<id>.smoke_command",
		"proof.readiness.<id>.attestation_ref",
		"proof.evidence.<id>.covers",
		"proof.waiver.<id>.expires_at",
		"formal_claims.status",
		"formal_claims.finding.<id>.route",
		"formal_claims.route.test_harness",
		"proof_readiness.implementation_ready=true",
		"proof_readiness.test_harness_required=true",
		"Sandbox admission blocks execution routing",
		"The system SHALL validate model-dependent behavior with Gemini first",
		"- [x] 1.1 Pin the `create_change` proposal, delta, task, and graph-only execution-field contracts.",
		"- [x] 1.2 Add ingest and render round-trip coverage for a repo-root `openspec/` tree.",
		"- [x] 1.3 Add UI spec review with edit, approve, reject, and request-revision actions.",
		"- [x] 1.4 Add export for the OpenSpec change folder and rendered single-document projection.",
		"- [x] 1.5 Add slash-command shortcuts that invoke the same governed actions as visible UI controls.",
		"- [x] 2.1 Define the claim, proof dependency, harness profile, readiness record, evidence, and waiver fact model.",
		"- [x] 2.2 Implement a deterministic proof-readiness analyzer that emits `formal_claims.*` findings onto the run entity.",
		"- [x] 2.3 Route missing proof dependencies to test-harness and passed readiness to implementation.",
		"- [x] 2.4 Add UI cards for proof dependencies, readiness records, evidence freshness, and waivers.",
		"- [x] 3.1 Project approved `change.<slug>.task.*` facts into `plan.task.*` plus the chain-level acceptance command.",
		"- [x] 3.2 Dispatch one ready task at a time through the existing Ralph execution loop.",
		"- [x] 3.3 Preserve CBG as the final acceptance gate and surface rejected gates to the coordinator and UI.",
		"- [x] 3.4 Enforce the definition-of-done authority stack in coordinator, Ralph, CBG, and projection contracts.",
		"- [x] 4.1 Build run health from graph facts, lifecycle, active loops, approvals, proof findings, and evidence freshness.",
		"- [x] 4.2 Display run health as working, waiting, blocked, failing, or complete with the current gate and next action.",
		"- [x] 4.3 Keep raw trajectories, logs, and graph triples as drill-down evidence behind the summary.",
		"Prometheus metrics supplement run health",
		"metrics as observability evidence rather than the authority for routing, task completion, or CBG",
		"Metric gaps are visible",
		"Prometheus metrics are operational evidence",
		"- [x] 4.4 Add Prometheus metric freshness, component pressure, queue depth, latency, and error-rate evidence to run health.",
		"- [x] 5.2 Produce or reject a reusable harness profile before feature code is released.",
		"- [x] 6.4 Add e2e coverage that autoresearch refuses vague/non-scalar goals and preserves metric guardrails.",
		"- [x] 6.5 Add e2e coverage that non-ready sandbox admission fails closed before execution routing.",
		"- [x] 6.1 Add a Gemini-backed real-LLM smoke path for model-dependent routing and prompt behavior.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered OpenSpec change missing %q\n--- rendered ---\n%s", want, rendered)
		}
	}
}
