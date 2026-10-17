package research

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/payloadregistry"
)

func sampleCitation(now time.Time) EvidenceRecord {
	return EvidenceRecord{
		Channel:       ChannelGitHub,
		Locator:       "github://sensorhub-tools/osh-core/issues/1",
		OriginID:      "github://sensorhub-tools/osh-core/issues/1",
		SourceClass:   SourceClassPrimary,
		RetrievedAt:   now,
		Quote:         "IDriver observations over LoRa mesh transport",
		ToolResultRef: "call_gather_1",
		Revision:      1,
	}
}

func TestArtifact_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	ev := sampleCitation(now)

	orig := &Artifact{
		LoopID:   "loop_abc",
		Revision: 2,
		Actors: []Actor{
			{Name: "OSH driver framework", Role: "Implements IDriver", Evidence: []EvidenceRecord{ev}},
			{Name: "Meshtastic radio", Role: "LoRa mesh transport", Evidence: []EvidenceRecord{ev}},
		},
		IntegrationPoints: []IntegrationPoint{
			{From: "OSH driver framework", To: "OGC CS endpoints", Data: "observations", Direction: DirectionWrite, Evidence: []EvidenceRecord{ev}},
			{From: "Meshtastic radio", To: "OSH driver framework", Data: "MeshPacket payloads", Direction: DirectionRead, Evidence: []EvidenceRecord{ev}},
		},
		Tasks: []string{
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
				Timestamp:  now,
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
	if len(got.Actors[0].Evidence) != 1 || got.Actors[0].Evidence[0].OriginID != ev.OriginID {
		t.Errorf("actor[0].evidence round-trip mismatch: %+v", got.Actors[0].Evidence)
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

func TestArtifact_TestHarness_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	t.Run("test_harness set round-trips", func(t *testing.T) {
		t.Parallel()
		orig := &Artifact{
			LoopID:      "loop_abc",
			Revision:    1,
			TestHarness: "meshtasticd-2.x",
			ProducedAt:  now,
		}
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatal(err)
		}
		var got Artifact
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.TestHarness != orig.TestHarness {
			t.Errorf("test_harness: got %q, want %q", got.TestHarness, orig.TestHarness)
		}
	})

	t.Run("test_harness omitempty", func(t *testing.T) {
		t.Parallel()
		a := &Artifact{LoopID: "loop_abc", Revision: 1, ProducedAt: now}
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		// JSON omitempty: empty string TestHarness should not be present
		// in the wire bytes — guards against breaking older v1 consumers.
		if got := string(data); strings.Contains(got, `"test_harness"`) {
			t.Errorf("expected test_harness to be omitted when empty, got %s", got)
		}
	})

	t.Run("validate accepts both presence and absence", func(t *testing.T) {
		t.Parallel()
		// The Validate() boundary is structural — semantic either-or
		// (test_harness OR needs_test_harness gap) is the reviewer
		// persona's job. Both shapes must pass Validate.
		hit := Artifact{LoopID: "x", Revision: 1, TestHarness: "stub", ProducedAt: now}
		miss := Artifact{LoopID: "x", Revision: 1, OpenGaps: []string{"needs_test_harness: real Meshtastic radio"}, ProducedAt: now}
		if err := hit.Validate(); err != nil {
			t.Errorf("hit case: %v", err)
		}
		if err := miss.Validate(); err != nil {
			t.Errorf("miss case: %v", err)
		}
	})
}

func TestArtifact_Validate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	ev := sampleCitation(now)

	cases := []struct {
		name    string
		a       Artifact
		wantErr bool
	}{
		{
			// Smoke #8 run-9: Validate now requires either TestHarness OR
			// a needs_test_harness gap entry. Minimal happy path uses the
			// gap-flag escape hatch since the artifact has no integration
			// points worth picking a harness for.
			name: "happy path — minimal (test_harness gap flagged)",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				OpenGaps:   []string{"needs_test_harness: not applicable — no external integration in this work"},
			},
			wantErr: false,
		},
		{
			name: "happy path — with test_harness selected",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    1,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
			},
			wantErr: false,
		},
		{
			name: "happy path — with mutation",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    2,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				SubstrateMutations: []Mutation{
					{Tool: "add_source_repo", LoopID: "loop_abc_retry", Revision: 2, Status: MutationStatusExecuted, Timestamp: now},
				},
			},
			wantErr: false,
		},
		{
			name: "happy path — multi-channel evidence (github, web, semsource)",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    2,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				Actors: []Actor{
					{
						Name: "OSH driver",
						Role: "Driver host",
						Evidence: []EvidenceRecord{
							ev,
							{
								Channel:       ChannelWeb,
								Locator:       "https://docs.example.com/osh?utm_source=x",
								OriginID:      "https://docs.example.com/osh",
								SourceClass:   SourceClassOfficial,
								RetrievedAt:   now,
								Quote:         "OSH driver host specification",
								ToolResultRef: "call_web_1",
								Revision:      2,
							},
						},
					},
				},
				IntegrationPoints: []IntegrationPoint{
					{
						From:      "OSH driver",
						To:        "OGC CS",
						Data:      "observations",
						Direction: DirectionWrite,
						Evidence: []EvidenceRecord{
							{
								Channel:       ChannelSemSource,
								Locator:       "semsource://c360/semteams/entity/osh.driver",
								OriginID:      "semsource://c360/semteams/entity/osh.driver",
								SourceClass:   SourceClassPrimary,
								RetrievedAt:   now,
								Quote:         "writes observations to OGC CS",
								ToolResultRef: "call_semsource_1",
								Revision:      1,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "actor missing evidence is rejected",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    1,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				Actors: []Actor{
					{Name: "OSH driver", Role: "Driver host"},
				},
			},
			wantErr: true,
		},
		{
			name: "integration point missing evidence is rejected",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    1,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				IntegrationPoints: []IntegrationPoint{
					{From: "a", To: "b", Direction: DirectionRead},
				},
			},
			wantErr: true,
		},
		{
			name: "evidence with non-canonical origin_id is rejected",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    1,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				Actors: []Actor{
					{
						Name: "OSH driver",
						Role: "Driver host",
						Evidence: []EvidenceRecord{
							{
								Channel:       ChannelGitHub,
								Locator:       "github://SensorHub-Tools/osh-core/issues/1",
								OriginID:      "github://SensorHub-Tools/osh-core/issues/1",
								SourceClass:   SourceClassPrimary,
								RetrievedAt:   now,
								Quote:         "IDriver",
								ToolResultRef: "call_1",
								Revision:      1,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "evidence revision exceeding artifact revision is rejected",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    1,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				Actors: []Actor{
					{
						Name: "OSH driver",
						Role: "Driver host",
						Evidence: []EvidenceRecord{
							{
								Channel:       ChannelGitHub,
								Locator:       "github://sensorhub-tools/osh-core/issues/1",
								OriginID:      "github://sensorhub-tools/osh-core/issues/1",
								SourceClass:   SourceClassPrimary,
								RetrievedAt:   now,
								Quote:         "IDriver",
								ToolResultRef: "call_1",
								Revision:      2,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			// Smoke #8 run-9 negative case: researcher emits artifact
			// with neither test_harness nor a needs_test_harness gap.
			// Validate now rejects at the structural layer so the
			// architect doesn't have to discover the gap mid-chain
			// and wedge with needs_clarification (no recovery rule).
			name: "missing test_harness AND no needs_test_harness gap is rejected",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				// neither TestHarness nor OpenGaps has the marker
			},
			wantErr: true,
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
					{From: "a", To: "b", Direction: "sideways", Evidence: []EvidenceRecord{ev}},
				},
			},
			wantErr: true,
		},
		{
			name: "integration point with empty direction is allowed in-flight",
			a: Artifact{
				LoopID:      "loop_abc",
				Revision:    1,
				ProducedAt:  now,
				TestHarness: "meshtasticd-2.x",
				IntegrationPoints: []IntegrationPoint{
					{From: "a", To: "b", Direction: "", Evidence: []EvidenceRecord{ev}},
				},
			},
			wantErr: false,
		},
		{
			// hasNeedsTestHarnessGap matches the marker even with
			// leading whitespace; persona-emitted lists may indent.
			name: "needs_test_harness with leading whitespace is recognised",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				OpenGaps:   []string{"  needs_test_harness: catalog miss for OGC SensorThings"},
			},
			wantErr: false,
		},
		{
			// Marker is case-sensitive; "Needs_Test_Harness" doesn't match
			// (LLMs canonicalise their structured outputs to the persona's
			// example, which uses lowercase).
			name: "case-insensitive needs_test_harness is NOT recognised",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				OpenGaps:   []string{"NEEDS_TEST_HARNESS: should be lowercase per contract"},
			},
			wantErr: true,
		},
		{
			// Marker matched as substring of a non-marker line is NOT recognised —
			// HasPrefix discrimination, not Contains. A future "let's be more
			// lenient with substring match" refactor would fail this test loud.
			name: "needs_test_harness as substring (not prefix) is NOT recognised",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				OpenGaps:   []string{"reviewer note: needs_test_harness: addressed in next pass"},
			},
			wantErr: true,
		},
		{
			// Trailing whitespace before the colon ("needs_test_harness :") is
			// NOT the canonical marker — HasPrefix matches the literal string
			// `needs_test_harness:` (no space). Future grep tooling depends on
			// this exact shape.
			name: "needs_test_harness with space before colon is NOT recognised",
			a: Artifact{
				LoopID:     "loop_abc",
				Revision:   1,
				ProducedAt: now,
				OpenGaps:   []string{"needs_test_harness : space before colon"},
			},
			wantErr: true,
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

func TestArtifact_PopulateCitationDefaults(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	a := &Artifact{
		LoopID:      "loop_abc",
		Revision:    2,
		ProducedAt:  now,
		TestHarness: "meshtasticd-2.x",
		Actors: []Actor{
			{
				Name: "NATS Server",
				Role: "Message broker",
				Evidence: []EvidenceRecord{
					{
						Channel:       ChannelWeb,
						Locator:       "https://Docs.NATS.io/compare/?utm_source=test#sec",
						SourceClass:   SourceClassOfficial,
						Quote:         "NATS vs MQTT, Kafka, and other messaging systems",
						ToolResultRef: "call_web_1",
					},
				},
			},
		},
	}

	a.PopulateCitationDefaults()
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate after PopulateCitationDefaults failed: %v", err)
	}
	got := a.Actors[0].Evidence[0]
	if got.OriginID != "https://docs.nats.io/compare" {
		t.Errorf("OriginID = %q, want https://docs.nats.io/compare", got.OriginID)
	}
	if !got.RetrievedAt.Equal(now) {
		t.Errorf("RetrievedAt = %v, want %v", got.RetrievedAt, now)
	}
	if got.Revision != 2 {
		t.Errorf("Revision = %d, want 2", got.Revision)
	}
}
