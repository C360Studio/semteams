package devviaspec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// minimalValidArtifact returns the smallest Artifact that passes Validate.
// Tests that need to exercise a specific failure path start here and
// mutate a single field, so the failure is attributable to that field alone.
func minimalValidArtifact() Artifact {
	return Artifact{
		Title:   "OSH Meshtastic Driver",
		Goal:    "Implement an OSH IDriver backed by Meshtastic radio events.",
		Context: "The OSH platform exposes observation endpoints; Meshtastic provides LoRa mesh transport.",
		Actors: []Actor{
			{Name: "OSH driver framework", Role: "host of the IDriver interface"},
		},
		Tasks: []Task{
			{Title: "Implement IDriver", Scope: "backend", GroundsActors: []string{"OSH driver framework"}},
		},
		Provenance: Provenance{
			ResearchArtifactLoop: "loop-research-001",
			PlannerLoop:          "loop-planner-001",
			ReviewerLoop:         "loop-reviewer-001",
		},
	}
}

// ---------------------------------------------------------------------
// Round-trip
// ---------------------------------------------------------------------

func TestArtifact_RoundTrip(t *testing.T) {
	t.Parallel()

	orig := &Artifact{
		Title:   "OSH Meshtastic Driver",
		Goal:    "Implement an OSH IDriver backed by Meshtastic radio events.",
		Context: "The OSH platform exposes observation endpoints.",
		Actors: []Actor{
			{Name: "OSH driver framework", Role: "host of the IDriver interface"},
			{Name: "Meshtastic radio", Role: "LoRa mesh transport"},
		},
		IntegrationPoints: []IntegrationPoint{
			{From: "Meshtastic radio", To: "OSH driver framework", Direction: DirectionRead, Data: "MeshPacket payloads"},
			{From: "OSH driver framework", To: "OGC CS endpoints", Direction: DirectionWrite, Data: "SensorML observations"},
		},
		Tasks: []Task{
			{
				Title:                    "Implement IDriver",
				Scope:                    "backend",
				GroundsActors:            []string{"OSH driver framework"},
				GroundsIntegrationPoints: []string{"Meshtastic radio→OSH driver framework"},
			},
			{
				Title:         "Expose OGC CS endpoints",
				Scope:         "backend",
				GroundsActors: []string{"OSH driver framework"},
			},
		},
		Provenance: Provenance{
			ResearchArtifactLoop: "loop-research-001",
			PlannerLoop:          "loop-planner-001",
			ReviewerLoop:         "loop-reviewer-001",
			ArchitectLoop:        "loop-architect-001",
		},
		GeneratedAt: "2026-04-30T12:00:00Z",
		Slug:        "2026-04-30-osh-meshtastic-driver",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Artifact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Title != orig.Title {
		t.Errorf("title: got %q, want %q", got.Title, orig.Title)
	}
	if got.Goal != orig.Goal {
		t.Errorf("goal: got %q, want %q", got.Goal, orig.Goal)
	}
	if len(got.Actors) != len(orig.Actors) {
		t.Fatalf("actors len: got %d, want %d", len(got.Actors), len(orig.Actors))
	}
	if got.Actors[0].Name != orig.Actors[0].Name {
		t.Errorf("actors[0].name: got %q, want %q", got.Actors[0].Name, orig.Actors[0].Name)
	}
	if len(got.IntegrationPoints) != len(orig.IntegrationPoints) {
		t.Fatalf("integration_points len: got %d, want %d", len(got.IntegrationPoints), len(orig.IntegrationPoints))
	}
	if got.IntegrationPoints[0].Direction != DirectionRead {
		t.Errorf("integration_points[0].direction: got %q, want %q", got.IntegrationPoints[0].Direction, DirectionRead)
	}
	if len(got.Tasks) != len(orig.Tasks) {
		t.Fatalf("tasks len: got %d, want %d", len(got.Tasks), len(orig.Tasks))
	}
	if got.Tasks[0].Title != orig.Tasks[0].Title {
		t.Errorf("tasks[0].title: got %q, want %q", got.Tasks[0].Title, orig.Tasks[0].Title)
	}
	if got.Provenance.ResearchArtifactLoop != orig.Provenance.ResearchArtifactLoop {
		t.Errorf("provenance.research_artifact_loop: got %q, want %q", got.Provenance.ResearchArtifactLoop, orig.Provenance.ResearchArtifactLoop)
	}
	if got.Provenance.ArchitectLoop != orig.Provenance.ArchitectLoop {
		t.Errorf("provenance.architect_loop: got %q, want %q", got.Provenance.ArchitectLoop, orig.Provenance.ArchitectLoop)
	}
	if got.Slug != orig.Slug {
		t.Errorf("slug: got %q, want %q", got.Slug, orig.Slug)
	}
	if got.GeneratedAt != orig.GeneratedAt {
		t.Errorf("generated_at: got %q, want %q", got.GeneratedAt, orig.GeneratedAt)
	}
}

// ---------------------------------------------------------------------
// Validate — happy paths
// ---------------------------------------------------------------------

func TestArtifact_Validate_HappyPath_Minimal(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestArtifact_Validate_HappyPath_EmptyIntegrationPointDirection(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	a.IntegrationPoints = []IntegrationPoint{
		{From: "A", To: "B", Direction: ""},
	}
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() unexpected error for empty direction (allowed in-flight): %v", err)
	}
}

func TestArtifact_Validate_HappyPath_WithProviderArchitectLoop(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	a.Provenance.ArchitectLoop = "loop-architect-001"
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------
// Validate — rejection cases
// ---------------------------------------------------------------------

func TestArtifact_Validate_Table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		mutate          func(a *Artifact)
		wantErrContains string
	}{
		{
			name:            "empty title",
			mutate:          func(a *Artifact) { a.Title = "" },
			wantErrContains: "title",
		},
		{
			name:            "empty goal",
			mutate:          func(a *Artifact) { a.Goal = "" },
			wantErrContains: "goal",
		},
		{
			name:            "empty context",
			mutate:          func(a *Artifact) { a.Context = "" },
			wantErrContains: "context",
		},
		{
			name:            "no actors",
			mutate:          func(a *Artifact) { a.Actors = nil },
			wantErrContains: "actors",
		},
		{
			name:            "no tasks",
			mutate:          func(a *Artifact) { a.Tasks = nil },
			wantErrContains: "tasks",
		},
		{
			name: "invalid integration point direction",
			mutate: func(a *Artifact) {
				a.IntegrationPoints = []IntegrationPoint{
					{From: "A", To: "B", Direction: "sideways"},
				}
			},
			wantErrContains: "direction",
		},
		{
			name:            "missing research_artifact_loop",
			mutate:          func(a *Artifact) { a.Provenance.ResearchArtifactLoop = "" },
			wantErrContains: "research_artifact_loop",
		},
		// PlannerLoop + ReviewerLoop empty is permitted under ADR-041
		// MVP — the legacy planner/reviewer hops were folded into
		// researcher-architect, so there is no upstream loop to cite.
		// Validate() must accept empties on those slots; the
		// happy-path test (TestArtifact_Validate_Happy below) covers
		// the non-empty case.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := minimalValidArtifact()
			tc.mutate(&a)
			err := a.Validate()
			if err == nil {
				t.Fatalf("Validate() expected error containing %q, got nil", tc.wantErrContains)
			}
			if tc.wantErrContains != "" {
				found := false
				msg := err.Error()
				for i := 0; i < len(msg)-len(tc.wantErrContains)+1; i++ {
					if msg[i:i+len(tc.wantErrContains)] == tc.wantErrContains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Validate() error = %q, want contains %q", msg, tc.wantErrContains)
				}
			}
		})
	}
}

// TestArtifact_Validate_AcceptsEmptyPlannerAndReviewerLoops pins the
// ADR-041 Phase 3 rule: PlannerLoop + ReviewerLoop are wire-retained
// slots from the legacy dev-via-spec arc (planner → reviewer →
// challenger → architect) that MVP folded into researcher-architect.
// Validate() must accept empty strings on both so the architect's
// emit doesn't fail under MVP (no upstream planner/reviewer loop
// exists to cite). research_artifact_loop stays required — the
// researcher-synthesize phase output is still a real upstream loop.
func TestArtifact_Validate_AcceptsEmptyPlannerAndReviewerLoops(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	a.Provenance.PlannerLoop = ""
	a.Provenance.ReviewerLoop = ""
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() with empty PlannerLoop+ReviewerLoop: got error %v, want nil (ADR-041 MVP must accept)", err)
	}
}

// ---------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------

func TestArtifact_Schema(t *testing.T) {
	t.Parallel()
	a := &Artifact{}
	s := a.Schema()
	if s.Domain != Domain {
		t.Errorf("Schema().Domain: got %q, want %q", s.Domain, Domain)
	}
	if s.Category != CategoryArtifact {
		t.Errorf("Schema().Category: got %q, want %q", s.Category, CategoryArtifact)
	}
	if s.Version != SchemaVersion {
		t.Errorf("Schema().Version: got %q, want %q", s.Version, SchemaVersion)
	}
}

// ---------------------------------------------------------------------
// RegisterPayloads
// ---------------------------------------------------------------------

func TestRegisterPayloads(t *testing.T) {
	t.Parallel()
	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}
	// Second call must error — duplicate registration is a boot-time misconfiguration.
	if err := RegisterPayloads(reg); err == nil {
		t.Errorf("RegisterPayloads: expected duplicate-registration error on second call")
	}
}

// ---------------------------------------------------------------------
// R3.7.2.b — Checks wiring
// ---------------------------------------------------------------------

// validCheck is a happy-path check used across the check-related tests.
// Brownfield ref pointing at the repo's own existing integration test pattern.
func validCheck() verification.Check {
	return verification.Check{
		Target:      "executor publishes the expected subject on success",
		Runtime:     verification.RuntimeProcessLocalTestcontainer,
		TestHarness: "nats-jetstream",
		TestRuntime: "go-testing-net",
		Ref: verification.Ref{
			Type: verification.RefFilepath,
			Path: "cmd/semteams/sandbox/integration_test.go",
		},
		Evidence: []verification.EvidenceRule{
			{Kind: "test_uses_build_tag", Args: map[string]any{"tag": "integration"}},
		},
	}
}

func TestArtifact_Checks_RoundTrip(t *testing.T) {
	t.Parallel()
	orig := minimalValidArtifact()
	orig.Checks = []verification.Check{
		validCheck(),
		{
			Target:  "executor returns ToolErrorInvalidArgs for malformed args",
			Runtime: verification.RuntimeInProcessUnit,
			Ref: verification.Ref{
				Type: verification.RefFilepath,
				Path: "cmd/semteams/tools/x/executor_test.go",
			},
			// Evidence required per smoke #8 run-8 fix — fixture would
			// fail Validate() without it.
			Evidence: []verification.EvidenceRule{
				{Kind: "test_file_exists", Args: map[string]any{"path": "cmd/semteams/tools/x/executor_test.go"}},
			},
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Artifact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Checks) != 2 {
		t.Fatalf("checks len: got %d, want 2", len(got.Checks))
	}
	if got.Checks[0].Runtime != verification.RuntimeProcessLocalTestcontainer {
		t.Errorf("checks[0].runtime: got %q", got.Checks[0].Runtime)
	}
	if got.Checks[0].TestHarness != "nats-jetstream" {
		t.Errorf("checks[0].test_harness: got %q", got.Checks[0].TestHarness)
	}
	if got.Checks[1].Runtime != verification.RuntimeInProcessUnit {
		t.Errorf("checks[1].runtime: got %q", got.Checks[1].Runtime)
	}
}

func TestArtifact_Checks_OmitemptyWhenNil(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	// Checks unset (zero-value nil slice).
	data, err := json.Marshal(&a)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, `"checks"`) {
		t.Errorf("expected checks to be omitted when nil; body=%s", got)
	}
}

func TestArtifact_Checks_OmitemptyWhenEmptySlice(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	a.Checks = []verification.Check{}
	data, err := json.Marshal(&a)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, `"checks"`) {
		t.Errorf("expected checks to be omitted when empty slice; body=%s", got)
	}
}

func TestArtifact_Validate_CascadesIntoChecks(t *testing.T) {
	t.Parallel()
	a := minimalValidArtifact()
	a.Checks = []verification.Check{
		validCheck(),
		{
			// Invalid: testcontainer runtime without a test_harness.
			// Evidence is populated so the test isolates the
			// test_harness violation; without it, the test would
			// fire on whichever Validate check happens to come first
			// (which would survive a Validate-ordering change but
			// silently shift the test's contract).
			Target:      "x",
			Runtime:     verification.RuntimeProcessLocalTestcontainer,
			TestRuntime: "go-testing-net",
			Ref: verification.Ref{
				Type: verification.RefFilepath,
				Path: "x_test.go",
			},
			Evidence: []verification.EvidenceRule{
				{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}},
			},
		},
	}
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error from cascade into checks[1], got nil")
	}
	if !strings.Contains(err.Error(), "checks[1]") {
		t.Errorf("expected error to name checks[1], got %v", err)
	}
	if !strings.Contains(err.Error(), "requires a test_harness") {
		t.Errorf("expected error to include underlying validate message, got %v", err)
	}
}

func TestArtifact_Validate_AcceptsNoChecks(t *testing.T) {
	t.Parallel()
	// The architect-persona contract that requires checks for
	// external-actor work lands in R3.7.2.f. At the structural
	// validation boundary, an artifact with zero checks is
	// still valid (e.g. truly pure work, or transition window).
	a := minimalValidArtifact()
	if err := a.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
