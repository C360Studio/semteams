package devviaspec

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/payloadregistry"
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
		SeedRequirements: []SeedRequirement{
			{Title: "Implement IDriver", Scope: "backend", GroundsActors: []string{"OSH driver framework"}},
		},
		Provenance: Provenance{
			ResearchArtifactLoop: "loop-research-001",
			PlannerLoop:          "loop-planner-001",
			ReviewerLoop:         "loop-reviewer-001",
			ChallengerLoop:       "loop-challenger-001",
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
		SeedRequirements: []SeedRequirement{
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
			ChallengerLoop:       "loop-challenger-001",
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
	if len(got.SeedRequirements) != len(orig.SeedRequirements) {
		t.Fatalf("seed_requirements len: got %d, want %d", len(got.SeedRequirements), len(orig.SeedRequirements))
	}
	if got.SeedRequirements[0].Title != orig.SeedRequirements[0].Title {
		t.Errorf("seed_requirements[0].title: got %q, want %q", got.SeedRequirements[0].Title, orig.SeedRequirements[0].Title)
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
			name:            "no seed requirements",
			mutate:          func(a *Artifact) { a.SeedRequirements = nil },
			wantErrContains: "seed_requirements",
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
		{
			name:            "missing planner_loop",
			mutate:          func(a *Artifact) { a.Provenance.PlannerLoop = "" },
			wantErrContains: "planner_loop",
		},
		{
			name:            "missing reviewer_loop",
			mutate:          func(a *Artifact) { a.Provenance.ReviewerLoop = "" },
			wantErrContains: "reviewer_loop",
		},
		{
			name:            "missing challenger_loop",
			mutate:          func(a *Artifact) { a.Provenance.ChallengerLoop = "" },
			wantErrContains: "challenger_loop",
		},
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
