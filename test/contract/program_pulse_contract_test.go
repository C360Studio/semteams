package contract

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"

	"github.com/c360studio/semteams/cmd/semteams/portfolio"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/githubread"
	"github.com/c360studio/semteams/test/e2e/mock"
)

// TestProgramPulseJourney_Contract verifies the end-to-end Program Pulse
// (/pulse) contract over the existing research pack (#267):
//  1. test/fixtures/journeys/program-pulse.yaml is a valid role-keyed mock
//     fixture that routes through coordinator -> researcher-research-plan
//     (2-project fan-out) -> researcher-research-gather (github_read) ->
//     researcher-research-synthesize (emit_research_artifact) ->
//     reviewer-research -> coordinator.
//  2. github_read against mock-llm's read-only GitHub REST endpoints produces
//     verified channel="github", source_class="primary" EvidenceRecords for
//     multiple configured C360 projects (semteams + semengine) while failing
//     closed on unconfigured repositories.
//  3. emitartifact.VerifyDerivation mechanically verifies every citation in
//     the Program Pulse artifact against the github_read tool result and
//     preserves the separation of observed facts (actors/integration_points),
//     labeled recommendations (tasks), and explicit missing-evidence
//     disclosures (open_gaps).
func TestProgramPulseJourney_Contract(t *testing.T) {
	t.Parallel()

	fx, err := mock.LoadFixture("../fixtures/journeys/program-pulse.yaml")
	if err != nil {
		t.Fatalf("LoadFixture(program-pulse.yaml): %v", err)
	}

	pf, err := portfolio.LoadFile("../../configs/portfolio-c360.json")
	if err != nil {
		t.Fatalf("LoadFile(portfolio-c360.json): %v", err)
	}

	srv := mock.NewOpenAIServer().WithFixture(fx)
	if err := srv.Start(":0"); err != nil {
		t.Fatalf("srv.Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	ghExec := githubread.NewExecutor(githubread.Config{
		Portfolio: pf,
		APIBase:   srv.URL(),
	}, nil)

	// Unconfigured repository must fail closed before any network call.
	unconfiguredRes, err := ghExec.Execute(context.Background(), agentic.ToolCall{
		ID:   "call_unconfigured",
		Name: githubread.ToolName,
		Arguments: map[string]any{
			"repository": "unconfigured/repo",
		},
	})
	if err != nil {
		t.Fatalf("Execute(unconfigured): unexpected error: %v", err)
	}
	if unconfiguredRes.Error == "" || unconfiguredRes.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Fatalf("Execute(unconfigured) = %+v, want ToolErrorInvalidArgs error", unconfiguredRes)
	}

	// Execute the fixture's github_read call and verify emit_research_artifact derivation.
	var (
		ghCallID    string
		ghCallArgs  map[string]any
		emitRawJSON string
	)
	for _, bucket := range fx.ResponsesByRole {
		for _, resp := range bucket.Responses {
			if resp.ToolCall == nil {
				continue
			}
			switch resp.ToolCall.Name {
			case githubread.ToolName:
				ghCallID = resp.ToolCall.ID
				if err := json.Unmarshal([]byte(resp.ToolCall.ArgumentsJSON), &ghCallArgs); err != nil {
					t.Fatalf("unmarshal github_read args: %v", err)
				}
			case emitartifact.ToolName:
				emitRawJSON = resp.ToolCall.ArgumentsJSON
			}
		}
	}
	if ghCallID == "" {
		t.Fatal("program-pulse.yaml missing github_read tool_call with deterministic id")
	}
	if emitRawJSON == "" {
		t.Fatal("program-pulse.yaml missing emit_research_artifact tool_call")
	}

	ghRes, err := ghExec.Execute(context.Background(), agentic.ToolCall{
		ID:        ghCallID,
		Name:      githubread.ToolName,
		Arguments: ghCallArgs,
		LoopID:    "gather-loop-1",
	})
	if err != nil || ghRes.Error != "" {
		t.Fatalf("Execute(github_read): err=%v res.Error=%q", err, ghRes.Error)
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(emitRawJSON), &args); err != nil {
		t.Fatalf("unmarshal emit_research_artifact args: %v", err)
	}
	args["loop_id"] = "synthesize-loop-1"
	args["produced_at"] = time.Now().UTC()
	rawBytes, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}

	var artifact research.Artifact
	if err := json.Unmarshal(rawBytes, &artifact); err != nil {
		t.Fatalf("unmarshal research.Artifact: %v", err)
	}
	artifact.PopulateCitationDefaults()
	if err := artifact.Validate(); err != nil {
		t.Fatalf("artifact.Validate(): %v", err)
	}

	resolver := &fixtureToolResultResolver{
		results: map[string]*emitartifact.ResolvedToolResult{
			ghCallID: {
				Ref:               ghCallID,
				CallID:            ghCallID,
				LoopID:            "gather-loop-1",
				ToolName:          githubread.ToolName,
				Status:            string(agentic.TrajectoryStatusCompleted),
				Content:           ghRes.Content,
				DispatchArguments: ghCallArgs,
				ObservedAt:        time.Now().UTC(),
			},
		},
	}
	if err := emitartifact.VerifyDerivation(context.Background(), resolver, agentic.ToolCall{
		ID:     "call_emit_pulse",
		Name:   emitartifact.ToolName,
		LoopID: "synthesize-loop-1",
	}, &artifact); err != nil {
		t.Fatalf("VerifyDerivation: %v", err)
	}

	if len(artifact.Actors) < 2 {
		t.Fatalf("Program Pulse artifact actors = %d, want >= 2 configured projects", len(artifact.Actors))
	}
	for _, actor := range artifact.Actors {
		for _, ev := range actor.Evidence {
			if ev.Channel != research.ChannelGitHub {
				t.Errorf("actor %q evidence channel = %q, want %q", actor.Name, ev.Channel, research.ChannelGitHub)
			}
			if ev.SourceClass != research.SourceClassPrimary {
				t.Errorf("actor %q evidence source_class = %q, want %q", actor.Name, ev.SourceClass, research.SourceClassPrimary)
			}
			if ev.Kind == "" {
				t.Errorf("actor %q evidence kind was not stamped by VerifyDerivation", actor.Name)
			}
		}
	}

	if len(artifact.Tasks) == 0 || !strings.HasPrefix(artifact.Tasks[0], "Recommendation:") {
		t.Errorf("Program Pulse artifact tasks = %v, want labeled Recommendation entries", artifact.Tasks)
	}

	var sawReleaseGap bool
	for _, gap := range artifact.OpenGaps {
		if strings.Contains(gap, "C360Studio/semengine") && strings.Contains(gap, "releases") {
			sawReleaseGap = true
			break
		}
	}
	if !sawReleaseGap {
		t.Errorf("Program Pulse open_gaps = %v, want explicit disclosure of missing C360Studio/semengine releases", artifact.OpenGaps)
	}
}
