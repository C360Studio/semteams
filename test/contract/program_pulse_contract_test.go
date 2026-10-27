package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/types"

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

func validateScheduledProgramPulseDefinition(def rule.Definition) error {
	if def.ID != "scheduled_program_pulse" {
		return fmt.Errorf("id = %q, want scheduled_program_pulse", def.ID)
	}
	if def.Type != rule.CronRuleType {
		return fmt.Errorf("type = %q, want %q", def.Type, rule.CronRuleType)
	}
	if def.Enabled {
		return fmt.Errorf("enabled = true, want false (opt-in by default via Pattern-B RULE_CONFIG KV)")
	}
	if strings.TrimSpace(def.Schedule) == "" {
		return fmt.Errorf("schedule must be non-empty")
	}
	if _, err := rule.NewCronRule(def); err != nil {
		return fmt.Errorf("NewCronRule validation failed: %w", err)
	}
	cooldown, err := time.ParseDuration(strings.TrimSpace(def.Cooldown))
	if err != nil || cooldown < time.Hour {
		return fmt.Errorf("cooldown = %q, want valid duration >= 1h", def.Cooldown)
	}
	if len(def.Actions) != 1 {
		return fmt.Errorf("len(actions) = %d, want 1", len(def.Actions))
	}
	act := def.Actions[0]
	if act.Type != rule.ActionTypePublishAgent {
		return fmt.Errorf("action.type = %q, want %q", act.Type, rule.ActionTypePublishAgent)
	}
	if act.Role != "coordinator" || act.Subject != "agent.task.coordinator" {
		return fmt.Errorf("action role/subject = %q/%q, want coordinator/agent.task.coordinator", act.Role, act.Subject)
	}
	n, err := strconv.Atoi(strings.TrimSpace(act.LoopMaxIterations))
	if err != nil || n != 8 {
		return fmt.Errorf("action.loop_max_iterations = %q, want 8", act.LoopMaxIterations)
	}
	if !strings.HasPrefix(strings.TrimSpace(act.Prompt), "@pulse") {
		return fmt.Errorf("action.prompt = %q, must start with @pulse", act.Prompt)
	}
	return nil
}

// TestScheduledProgramPulseRuleContract verifies that
// configs/rules/research/10-scheduled-program-pulse.json satisfies #268 and
// ADR-060 §7 containment invariants and is wired into both bootstrap configs.
func TestScheduledProgramPulseRuleContract(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../configs/rules/research/10-scheduled-program-pulse.json")
	if err != nil {
		t.Fatalf("read 10-scheduled-program-pulse.json: %v", err)
	}
	var def rule.Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		t.Fatalf("unmarshal 10-scheduled-program-pulse.json: %v", err)
	}
	if err := validateScheduledProgramPulseDefinition(def); err != nil {
		t.Fatalf("10-scheduled-program-pulse.json contract violation: %v", err)
	}

	const expectedPath = "/app/configs/rules/research/10-scheduled-program-pulse.json"
	for _, cfgPath := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		files := rulesFilesFromBootstrap(t, cfgPath)
		var found bool
		for _, f := range files {
			if f == expectedPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s rules_files missing %s", cfgPath, expectedPath)
		}
	}
}

func TestScheduledProgramPulseRuleContractSensitivity(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../configs/rules/research/10-scheduled-program-pulse.json")
	if err != nil {
		t.Fatalf("read 10-scheduled-program-pulse.json: %v", err)
	}
	base := func() rule.Definition {
		var d rule.Definition
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return d
	}

	t.Run("rejects_enabled_true_by_default", func(t *testing.T) {
		d := base()
		d.Enabled = true
		if err := validateScheduledProgramPulseDefinition(d); err == nil {
			t.Fatal("expected error when scheduled_program_pulse is enabled by default")
		}
	})
	t.Run("rejects_missing_cooldown", func(t *testing.T) {
		d := base()
		d.Cooldown = ""
		if err := validateScheduledProgramPulseDefinition(d); err == nil {
			t.Fatal("expected error when cooldown is missing")
		}
	})
	t.Run("rejects_missing_loop_max_iterations", func(t *testing.T) {
		d := base()
		d.Actions[0].LoopMaxIterations = ""
		if err := validateScheduledProgramPulseDefinition(d); err == nil {
			t.Fatal("expected error when loop_max_iterations is missing")
		}
	})
	t.Run("rejects_non_pulse_prompt", func(t *testing.T) {
		d := base()
		d.Actions[0].Prompt = "Summarize the portfolio without the pulse prefix."
		if err := validateScheduledProgramPulseDefinition(d); err == nil {
			t.Fatal("expected error when prompt does not start with @pulse")
		}
	})
}

// TestProgramPulsePartialRepoFailureOpenGapsContract verifies that when a cited
// github_read tool result includes partial repository Failures, VerifyDerivation
// rejects the research artifact unless open_gaps names the failed repository.
func TestProgramPulsePartialRepoFailureOpenGapsContract(t *testing.T) {
	t.Parallel()

	quote := "Issue #268 [open]: program-report: make Program Pulse scheduled, incremental, and replay-safe"
	ghResult := githubread.Result{
		NewCount: 1,
		Baseline: true,
		Records: []research.EvidenceRecord{{
			Channel:     research.ChannelGitHub,
			Locator:     "github://C360Studio/semteams/issues/268",
			OriginID:    "github://C360Studio/semteams/issues/268",
			SourceClass: research.SourceClassPrimary,
			RetrievedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
			Kind:        githubread.KindIssues,
			RawBody:     quote,
		}},
		Failures: []githubread.RepositoryFailure{{
			Repository: "C360Studio/semengine",
			Kind:       githubread.KindIssues,
			Error:      "GET /repos/C360Studio/semengine/issues returned status 502",
		}},
	}
	rawGHResult, err := json.Marshal(ghResult)
	if err != nil {
		t.Fatalf("marshal ghResult: %v", err)
	}

	resolver := &fixtureToolResultResolver{
		results: map[string]*emitartifact.ResolvedToolResult{
			"call_gh_partial": {
				Ref:      "call_gh_partial",
				CallID:   "call_gh_partial",
				LoopID:   "gather-loop-partial",
				ToolName: githubread.ToolName,
				Status:   string(agentic.TrajectoryStatusCompleted),
				Content:  string(rawGHResult),
				DispatchArguments: map[string]any{
					"program": "c360",
					"kinds":   []any{"issues"},
				},
				ObservedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	buildArtifact := func(openGaps []string) *research.Artifact {
		art := &research.Artifact{
			LoopID:     "synthesize-loop-partial",
			ProducedAt: time.Date(2026, 10, 18, 9, 5, 0, 0, time.UTC),
			Actors: []research.Actor{{
				Name: "C360Studio/semteams",
				Role: "Program manager shell",
				Evidence: []research.EvidenceRecord{{
					Channel:       research.ChannelGitHub,
					Locator:       "github://C360Studio/semteams/issues/268",
					SourceClass:   research.SourceClassPrimary,
					Quote:         quote,
					ToolResultRef: "call_gh_partial",
					Revision:      1,
				}},
			}},
			Tasks:    []string{"Recommendation: retry C360Studio/semengine once GitHub API recovers"},
			OpenGaps: openGaps,
		}
		art.PopulateCitationDefaults()
		return art
	}

	goodArtifact := buildArtifact([]string{"GitHub issues fetch failed for C360Studio/semengine (HTTP 502)"})
	if err := emitartifact.VerifyDerivation(context.Background(), resolver, agentic.ToolCall{
		ID:     "call_emit_good",
		Name:   emitartifact.ToolName,
		LoopID: "synthesize-loop-partial",
	}, goodArtifact); err != nil {
		t.Fatalf("expected artifact disclosing C360Studio/semengine in open_gaps to pass VerifyDerivation, got: %v", err)
	}
}

func TestProgramPulsePartialRepoFailureOpenGapsContractSensitivity(t *testing.T) {
	t.Parallel()

	quote := "Issue #268 [open]: program-report: make Program Pulse scheduled, incremental, and replay-safe"
	ghResult := githubread.Result{
		NewCount: 1,
		Baseline: true,
		Records: []research.EvidenceRecord{{
			Channel:     research.ChannelGitHub,
			Locator:     "github://C360Studio/semteams/issues/268",
			OriginID:    "github://C360Studio/semteams/issues/268",
			SourceClass: research.SourceClassPrimary,
			RetrievedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
			Kind:        githubread.KindIssues,
			RawBody:     quote,
		}},
		Failures: []githubread.RepositoryFailure{{
			Repository: "C360Studio/semengine",
			Kind:       githubread.KindIssues,
			Error:      "GET /repos/C360Studio/semengine/issues returned status 502",
		}},
	}
	rawGHResult, err := json.Marshal(ghResult)
	if err != nil {
		t.Fatalf("marshal ghResult: %v", err)
	}

	resolver := &fixtureToolResultResolver{
		results: map[string]*emitartifact.ResolvedToolResult{
			"call_gh_partial": {
				Ref:      "call_gh_partial",
				CallID:   "call_gh_partial",
				LoopID:   "gather-loop-partial",
				ToolName: githubread.ToolName,
				Status:   string(agentic.TrajectoryStatusCompleted),
				Content:  string(rawGHResult),
				DispatchArguments: map[string]any{
					"program": "c360",
					"kinds":   []any{"issues"},
				},
				ObservedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	badArtifact := &research.Artifact{
		LoopID:     "synthesize-loop-partial",
		ProducedAt: time.Date(2026, 10, 18, 9, 5, 0, 0, time.UTC),
		Actors: []research.Actor{{
			Name: "C360Studio/semteams",
			Role: "Program manager shell",
			Evidence: []research.EvidenceRecord{{
				Channel:       research.ChannelGitHub,
				Locator:       "github://C360Studio/semteams/issues/268",
				SourceClass:   research.SourceClassPrimary,
				Quote:         quote,
				ToolResultRef: "call_gh_partial",
				Revision:      1,
			}},
		}},
		Tasks:    []string{"Recommendation: proceed"},
		OpenGaps: []string{"Unrelated gap that omits the failed repository"},
	}
	badArtifact.PopulateCitationDefaults()

	err = emitartifact.VerifyDerivation(context.Background(), resolver, agentic.ToolCall{
		ID:     "call_emit_bad",
		Name:   emitartifact.ToolName,
		LoopID: "synthesize-loop-partial",
	}, badArtifact)
	if err == nil || !strings.Contains(err.Error(), "C360Studio/semengine") {
		t.Fatalf("expected VerifyDerivation to reject omitted partial failure for C360Studio/semengine, got: %v", err)
	}
}

type noopArtifactPublisher struct{}

func (noopArtifactPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func rulesFilesFromBootstrap(t *testing.T, cfgPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read %s: %v", cfgPath, err)
	}
	var root struct {
		Components map[string]struct {
			Config struct {
				RulesFiles []string `json:"rules_files"`
			} `json:"config"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal %s: %v", cfgPath, err)
	}
	return root.Components["rule"].Config.RulesFiles
}

// TestProgramPulseCursorCommitOnEmitArtifactContract verifies the end-to-end
// run-scoped cursor commit contract between github_read and emit_research_artifact:
//  1. github_read in a gather loop stages cursors without advancing the
//     committed store.
//  2. emit_research_artifact commits staged cursors for all producer loops
//     only after derivation verification and publication succeed.
//  3. Failed emit_research_artifact calls leave staged cursors uncommitted.
func TestProgramPulseCursorCommitOnEmitArtifactContract(t *testing.T) {
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

	store := githubread.NewMemoryCursorStore()
	t1 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	ghExec := githubread.NewExecutor(githubread.Config{
		Portfolio: pf,
		APIBase:   srv.URL(),
		Cursors:   store,
		Clock:     func() time.Time { return t1 },
	}, nil)

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
				_ = json.Unmarshal([]byte(resp.ToolCall.ArgumentsJSON), &ghCallArgs)
			case emitartifact.ToolName:
				emitRawJSON = resp.ToolCall.ArgumentsJSON
			}
		}
	}

	ghRes, err := ghExec.Execute(context.Background(), agentic.ToolCall{
		ID:        ghCallID,
		Name:      githubread.ToolName,
		Arguments: ghCallArgs,
		LoopID:    "gather-loop-commit-test",
	})
	if err != nil || ghRes.Error != "" {
		t.Fatalf("Execute(github_read): err=%v res.Error=%q", err, ghRes.Error)
	}

	// Before emit_research_artifact runs, the cursor must only be staged, not committed.
	if _, committed := store.Get("C360Studio/semteams", githubread.KindIssues); committed {
		t.Fatal("cursor must not be committed before emit_research_artifact succeeds")
	}

	resolver := &fixtureToolResultResolver{
		results: map[string]*emitartifact.ResolvedToolResult{
			ghCallID: {
				Ref:               ghCallID,
				CallID:            ghCallID,
				LoopID:            "gather-loop-commit-test",
				ToolName:          githubread.ToolName,
				Status:            string(agentic.TrajectoryStatusCompleted),
				Content:           ghRes.Content,
				DispatchArguments: ghCallArgs,
				ObservedAt:        t1,
			},
		},
	}

	emitExec := emitartifact.NewExecutor(
		&contractTripleRecorder{},
		noopArtifactPublisher{},
		types.PlatformMeta{Org: "c360", Platform: "semteams"},
		nil,
		t.TempDir(),
	)
	emitExec.SetToolResultResolver(resolver)
	emitExec.SetCursorCommitter(store)

	var validEmitArgs map[string]any
	if err := json.Unmarshal([]byte(emitRawJSON), &validEmitArgs); err != nil {
		t.Fatalf("unmarshal emitRawJSON: %v", err)
	}

	// First attempt: corrupt a quote so VerifyDerivation rejects the artifact;
	// staged cursors must remain uncommitted.
	var badEmitArgs map[string]any
	_ = json.Unmarshal([]byte(emitRawJSON), &badEmitArgs)
	if actors, ok := badEmitArgs["actors"].([]any); ok && len(actors) > 0 {
		if actor0, ok := actors[0].(map[string]any); ok {
			if evs, ok := actor0["evidence"].([]any); ok && len(evs) > 0 {
				if ev0, ok := evs[0].(map[string]any); ok {
					ev0["quote"] = "fabricated quote that does not appear anywhere in github_read output"
				}
			}
		}
	}
	badRes, err := emitExec.Execute(context.Background(), agentic.ToolCall{
		ID:        "call_emit_rejected",
		Name:      emitartifact.ToolName,
		LoopID:    "synthesize-loop-commit-test",
		Arguments: badEmitArgs,
	})
	if err != nil {
		t.Fatalf("emitExec.Execute(bad): %v", err)
	}
	if badRes.Error == "" {
		t.Fatal("expected VerifyDerivation rejection on fabricated quote")
	}
	if _, committed := store.Get("C360Studio/semteams", githubread.KindIssues); committed {
		t.Fatal("rejected emit_research_artifact must not commit staged cursors")
	}

	// Second attempt: valid artifact succeeds and commits staged cursors for gather-loop-commit-test.
	goodRes, err := emitExec.Execute(context.Background(), agentic.ToolCall{
		ID:        "call_emit_accepted",
		Name:      emitartifact.ToolName,
		LoopID:    "synthesize-loop-commit-test",
		Arguments: validEmitArgs,
	})
	if err != nil || goodRes.Error != "" {
		t.Fatalf("emitExec.Execute(good): err=%v res.Error=%q", err, goodRes.Error)
	}
	cur, committed := store.Get("C360Studio/semteams", githubread.KindIssues)
	if !committed || !cur.BaselineEstablished {
		t.Fatalf("expected committed cursor with BaselineEstablished=true after emit_research_artifact, got %+v (committed=%t)", cur, committed)
	}
}
