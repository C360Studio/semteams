package research

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/payloadregistry"
)

func TestArtifact_RoundTrip(t *testing.T) {
	t.Parallel()

	orig := &Artifact{
		LoopID:   "loop_abc",
		Revision: 2,
		Actors: []Actor{
			{Name: "OSH driver framework", Role: "Implements IDriver"},
			{Name: "Meshtastic radio", Role: "LoRa mesh transport"},
		},
		IntegrationPoints: []IntegrationPoint{
			{From: "OSH driver framework", To: "OGC CS endpoints", Data: "observations", Direction: DirectionWrite},
			{From: "Meshtastic radio", To: "OSH driver framework", Data: "MeshPacket payloads", Direction: DirectionRead},
		},
		SeedRequirements: []string{
			"Implement OSH IDriver backed by Meshtastic radio events",
		},
		AddressedGaps: []string{
			"Registered https://github.com/sensorhub-tools/osh-core via add_source_repo",
		},
		SubstrateMutations: []Mutation{
			{
				Tool:       "add_source_repo",
				Args:       json.RawMessage(`{"url":"https://github.com/sensorhub-tools/osh-core","branch":"main","namespace":"research"}`),
				LoopID:     "loop_abc_retry",
				Revision:   2,
				ApprovedBy: "alice",
				Status:     MutationStatusExecuted,
				Result:     "created=true",
				Timestamp:  time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
			},
		},
		ProducedAt: time.Date(2026, 4, 30, 12, 5, 0, 0, time.UTC),
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Artifact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.LoopID != orig.LoopID {
		t.Errorf("loop_id: got %q, want %q", got.LoopID, orig.LoopID)
	}
	if got.Revision != orig.Revision {
		t.Errorf("revision: got %d, want %d", got.Revision, orig.Revision)
	}
	if len(got.Actors) != len(orig.Actors) {
		t.Fatalf("actors len: got %d, want %d", len(got.Actors), len(orig.Actors))
	}
	if len(got.SubstrateMutations) != 1 {
		t.Fatalf("substrate_mutations len: got %d, want 1", len(got.SubstrateMutations))
	}
	if got.SubstrateMutations[0].Status != MutationStatusExecuted {
		t.Errorf("mutation status: got %q, want %q", got.SubstrateMutations[0].Status, MutationStatusExecuted)
	}
	if !got.ProducedAt.Equal(orig.ProducedAt) {
		t.Errorf("produced_at: got %v, want %v", got.ProducedAt, orig.ProducedAt)
	}
}

func TestArtifact_Validate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	cases := []struct {
		name    string
		a       Artifact
		wantErr bool
	}{
		{
			name: "happy path — minimal",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
			},
			wantErr: false,
		},
		{
			name: "happy path — with mutation",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   2,
				ProducedAt: now,
				SubstrateMutations: []Mutation{
					{Tool: "add_source_repo", LoopID: "loop_abc_retry", Revision: 2, Status: MutationStatusExecuted, Timestamp: now},
				},
			},
			wantErr: false,
		},
		{
			name:    "missing loop_id",
			a:       Artifact{Revision: 1, ProducedAt: now},
			wantErr: true,
		},
		{
			name:    "zero revision",
			a:       Artifact{LoopID: "loop_abc", Revision: 0, ProducedAt: now},
			wantErr: true,
		},
		{
			name:    "missing produced_at",
			a:       Artifact{LoopID: "loop_abc", Revision: 1},
			wantErr: true,
		},
		{
			name: "mutation with empty tool",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				SubstrateMutations: []Mutation{
					{LoopID: "x", Revision: 1, Status: MutationStatusExecuted, Timestamp: now},
				},
			},
			wantErr: true,
		},
		{
			name: "mutation with invalid status",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				SubstrateMutations: []Mutation{
					{Tool: "x", LoopID: "y", Revision: 1, Status: "weird", Timestamp: now},
				},
			},
			wantErr: true,
		},
		{
			name: "mutation with zero timestamp",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				SubstrateMutations: []Mutation{
					{Tool: "x", LoopID: "y", Revision: 1, Status: MutationStatusExecuted},
				},
			},
			wantErr: true,
		},
		{
			name: "integration point with invalid direction",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				IntegrationPoints: []IntegrationPoint{
					{From: "a", To: "b", Direction: "sideways"},
				},
			},
			wantErr: true,
		},
		{
			name: "integration point with empty direction is allowed in-flight",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				IntegrationPoints: []IntegrationPoint{
					{From: "a", To: "b", Direction: ""},
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.a.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

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

func TestArtifact_LatestRevisionMutationCount(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	a := &Artifact{
		LoopID:     "loop_abc",
		Revision:   3,
		ProducedAt: now,
		SubstrateMutations: []Mutation{
			{Tool: "add_source_repo", LoopID: "x", Revision: 1, Status: MutationStatusExecuted, Timestamp: now},
			{Tool: "add_source_repo", LoopID: "x", Revision: 2, Status: MutationStatusExecuted, Timestamp: now},
			{Tool: "add_source_repo", LoopID: "x", Revision: 2, Status: MutationStatusFailed, Timestamp: now},
			// Latest revision (3) had zero new mutations — the stabilisation case.
		},
	}

	if got, want := a.LatestRevisionMutationCount(), 0; got != want {
		t.Errorf("revision 3 (no new mutations): got %d, want %d", got, want)
	}

	a.Revision = 2
	if got, want := a.LatestRevisionMutationCount(), 2; got != want {
		t.Errorf("revision 2: got %d, want %d", got, want)
	}
}

func TestRegisterPayloads(t *testing.T) {
	t.Parallel()

	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}
	// Calling again should error — duplicate registration is a
	// boot-time misconfiguration.
	if err := RegisterPayloads(reg); err == nil {
		t.Errorf("RegisterPayloads: expected duplicate-registration error on second call")
	}
}
