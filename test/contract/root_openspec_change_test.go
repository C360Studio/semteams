package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/c360studio/semteams/cmd/semteams/openspec"
)

// TestArchivedSpecDrivenDevOpenSpecChange_Readable pins the accepted
// spec-driven development OpenSpec change as a real archived artifact, not just
// prose docs. It remains the seed brownfield fixture for the future ingest path.
func TestArchivedSpecDrivenDevOpenSpecChange_Readable(t *testing.T) {
	change, err := openspec.ReadChange(filepath.Join(
		"..", "..", "openspec", "changes", "archive", "spec-driven-dev-readiness-hitl",
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
	if change.Tasks == nil || len(change.Tasks.Sections) != 7 {
		t.Fatalf("Tasks sections = %d, want 7", len(change.Tasks.Sections))
	}
	if len(change.Deltas) != 1 {
		t.Fatalf("Deltas = %d, want 1", len(change.Deltas))
	}

	delta := change.Deltas[0]
	if delta.Capability != "agentic-sdd" {
		t.Fatalf("Delta capability = %q, want agentic-sdd", delta.Capability)
	}
	if len(delta.Added) != 13 {
		t.Fatalf("Added requirements = %d, want 13", len(delta.Added))
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
		"The system SHALL distinguish black-box demo evidence from fixture-seeded bridge tests",
		"Black-box journeys avoid graph seeding",
		"Fixture-seeded journeys are labeled",
		"MAVLink-hard spec production is the MVP goal",
		"A demo evidence pack that separates black-box product journeys from fixture-seeded bridge proof.",
		"Counting direct NATS or graph seeding as black-box evidence for product behavior.",
		"Demo evidence has trust tiers",
		"MAVLink-hard MVP means spec production first",
		"- [x] 5.2 Produce or reject a reusable harness profile before feature code is released.",
		"- [x] 6.4 Add e2e coverage that autoresearch refuses vague/non-scalar goals and preserves metric guardrails.",
		"- [x] 6.5 Add e2e coverage that non-ready sandbox admission fails closed before execution routing.",
		"- [x] 6.1 Add a Gemini-backed real-LLM smoke path for model-dependent routing and prompt behavior.",
		"- [x] 7.1 Publish demo MVP claims, non-claims, evidence tiers, and the no-cheat e2e rule.",
		"- [x] 7.2 Add a black-box capstone task that excludes fixture-seeded graph and NATS writes.",
		"- [x] 7.3 Add a MAVLink-hard spec-production journey as the hard-domain artifact goal.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered OpenSpec change missing %q\n--- rendered ---\n%s", want, rendered)
		}
	}
}

// TestRootOpenSpecSpecs_Readable pins the accepted spec-driven development MVP
// requirements as baseline living OpenSpec specs. Future changes should build
// from this specs/ tree instead of mutating the archived seed proposal.
func TestRootOpenSpecSpecs_Readable(t *testing.T) {
	specs, err := openspec.ReadSpecs(filepath.Join("..", "..", "openspec", "specs"))
	if err != nil {
		t.Fatalf("ReadSpecs: %v", err)
	}
	spec := findOpenSpecByCapability(t, specs, "agentic-sdd")
	if spec.Title != "Agentic SDD Specification" {
		t.Fatalf("Spec title = %q", spec.Title)
	}
	if len(spec.Requirements) != 14 {
		t.Fatalf("Requirements = %d, want 14", len(spec.Requirements))
	}
	assertOpenSpecReadable(t, spec)

	names := make([]string, 0, len(spec.Requirements))
	for _, req := range spec.Requirements {
		names = append(names, req.Name)
	}
	for _, want := range []string{
		"OpenSpec Change Artifact",
		"Human Spec Review",
		"OpenSpec Artifact Export",
		"Command Shortcut Surface",
		"Proof Fact Model",
		"Proof Readiness Gate",
		"Dev From Task Dispatch",
		"Definition Of Done Authority",
		"Autoresearch Metric Guardrails",
		"Governed State Instead Of Private Workflow Buckets",
		"Run Health Surface",
		"Demo MVP Evidence Pack",
		"Real LLM And Playwright Validation",
		"Conversational Front Door",
	} {
		found := false
		for _, got := range names {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("baseline spec missing requirement %q; got %v", want, names)
		}
	}

	governance := findOpenSpecByCapability(t, specs, "repository-governance")
	assertOpenSpecReadable(t, governance)

	dst := filepath.Join(t.TempDir(), "specs")
	if err := openspec.WriteSpecs(dst, specs); err != nil {
		t.Fatalf("WriteSpecs: %v", err)
	}
	roundTripped, err := openspec.ReadSpecs(dst)
	if err != nil {
		t.Fatalf("re-ReadSpecs: %v", err)
	}
	if diff := cmp.Diff(specs, roundTripped, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("root OpenSpec specs round-trip mismatch (-want +got):\n%s", diff)
	}
}

func findOpenSpecByCapability(t *testing.T, specs []openspec.Spec, capability string) openspec.Spec {
	t.Helper()
	for _, spec := range specs {
		if spec.Capability == capability {
			return spec
		}
	}
	t.Fatalf("living OpenSpec capability %q not found", capability)
	return openspec.Spec{}
}

func assertOpenSpecReadable(t *testing.T, spec openspec.Spec) {
	t.Helper()
	if spec.Title == "" || spec.Purpose == "" || len(spec.Requirements) == 0 {
		t.Fatalf("spec %q is incomplete: title=%q purpose=%q requirements=%d",
			spec.Capability, spec.Title, spec.Purpose, len(spec.Requirements))
	}
	for _, req := range spec.Requirements {
		if req.Statement == "" {
			t.Fatalf("spec %q requirement %q has empty statement", spec.Capability, req.Name)
		}
		if len(req.Scenarios) == 0 {
			t.Fatalf("spec %q requirement %q has no scenarios", spec.Capability, req.Name)
		}
	}
}

// TestRepoReadinessInitOpenSpecChange_Retired keeps the unimplemented
// repo-readiness proposal out of the active OpenSpec queue. Issue #258 owns
// the retirement; issue #260 owns any future resumption, which must begin as
// a fresh claimed change.
func TestRepoReadinessInitOpenSpecChange_Retired(t *testing.T) {
	path := filepath.Join("..", "..", "openspec", "changes", "repo-readiness-init")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("retired OpenSpec change still exists at %s (stat error: %v)", path, err)
	}
}
