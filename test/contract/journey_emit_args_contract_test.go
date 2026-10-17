package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/processor/agentic-tools/executors"
	"gopkg.in/yaml.v3"

	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/portfolio"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
	"github.com/c360studio/semteams/cmd/semteams/tools/githubread"
	"github.com/c360studio/semteams/test/e2e/mock"
)

// TestJourneyFixtureEmitArgs_Validate walks every mock-llm journey
// fixture under test/fixtures/journeys and round-trips each
// `emit_research_artifact` / `emit_plan` tool call's arguments_json
// blob through the same parse + Validate + VerifyDerivation path the
// production executor uses.
//
// Why this exists: the fixtures encode the canonical "this is how a
// model following the persona contract calls the tool" example. If the
// fixture args drift away from the tool's schema (missing required
// field, wrong nested key shape, type mismatch) or from the stub
// web_search output format, the Playwright e2e fails at runtime — but
// only after spinning up the full Docker stack. This test catches drift
// in <1s without booting anything.
func TestJourneyFixtureEmitArgs_Validate(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("../fixtures/journeys")
	if err != nil {
		t.Fatalf("read journeys dir: %v", err)
	}

	sawFixture := false
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join("../fixtures/journeys", ent.Name())
		t.Run(ent.Name(), func(t *testing.T) {
			t.Parallel()
			validateJourneyFixture(t, path)
		})
		sawFixture = true
	}
	if !sawFixture {
		t.Fatal("no journey fixtures found under test/fixtures/journeys")
	}
}

type journeyTurn struct {
	ToolCall *struct {
		ID            string `yaml:"id,omitempty"`
		Name          string `yaml:"name"`
		ArgumentsJSON string `yaml:"arguments_json"`
	} `yaml:"tool_call,omitempty"`
}

type journeyFixture struct {
	Responses       []journeyTurn `yaml:"responses"`
	ResponsesByRole []struct {
		Match     string        `yaml:"match"`
		Responses []journeyTurn `yaml:"responses"`
	} `yaml:"responses_by_role"`
}

type fixtureToolResultResolver struct {
	results map[string]*emitartifact.ResolvedToolResult
}

func (r *fixtureToolResultResolver) ResolveToolResult(_ context.Context, _ agentic.ToolCall, ref string) (*emitartifact.ResolvedToolResult, error) {
	res, ok := r.results[ref]
	if !ok {
		return nil, fmt.Errorf("tool_result_ref %q not found in fixture tool calls", ref)
	}
	return res, nil
}

func validateJourneyFixture(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var fx journeyFixture
	if err := yaml.Unmarshal(data, &fx); err != nil {
		t.Fatalf("parse YAML %s: %v", path, err)
	}

	var allTurns []journeyTurn
	allTurns = append(allTurns, fx.Responses...)
	for _, bucket := range fx.ResponsesByRole {
		allTurns = append(allTurns, bucket.Responses...)
	}

	stubWebSearch := executors.NewStubWebSearchExecutor()
	var ghExec *githubread.Executor
	ensureGitHubExec := func() *githubread.Executor {
		if ghExec != nil {
			return ghExec
		}
		pf, err := portfolio.LoadFile("../../configs/portfolio-c360.json")
		if err != nil {
			t.Fatalf("%s: load portfolio-c360.json: %v", path, err)
		}
		srv := mock.NewOpenAIServer()
		if err := srv.Start(":0"); err != nil {
			t.Fatalf("%s: start mock github server: %v", path, err)
		}
		t.Cleanup(func() { _ = srv.Stop() })
		ghExec = githubread.NewExecutor(githubread.Config{
			Portfolio: pf,
			APIBase:   srv.URL(),
		}, nil)
		return ghExec
	}

	declaredCallIDs := make(map[string]bool)
	resolvedCalls := make(map[string]*emitartifact.ResolvedToolResult)
	for _, resp := range allTurns {
		if resp.ToolCall == nil || resp.ToolCall.ID == "" {
			continue
		}
		declaredCallIDs[resp.ToolCall.ID] = true
		switch resp.ToolCall.Name {
		case "web_search":
			var callArgs map[string]any
			if err := json.Unmarshal([]byte(resp.ToolCall.ArgumentsJSON), &callArgs); err != nil {
				t.Fatalf("%s: unmarshal web_search arguments_json for %q: %v", path, resp.ToolCall.ID, err)
			}
			res, err := stubWebSearch.Execute(context.Background(), agentic.ToolCall{
				ID:        resp.ToolCall.ID,
				Name:      "web_search",
				Arguments: callArgs,
				LoopID:    "fixture-gather-loop",
			})
			if err != nil || res.Error != "" {
				t.Fatalf("%s: execute stub web_search for %q: err=%v res.Error=%q", path, resp.ToolCall.ID, err, res.Error)
			}
			resolvedCalls[resp.ToolCall.ID] = &emitartifact.ResolvedToolResult{
				Ref:               resp.ToolCall.ID,
				CallID:            resp.ToolCall.ID,
				LoopID:            "fixture-gather-loop",
				ToolName:          "web_search",
				Status:            string(agentic.TrajectoryStatusCompleted),
				Content:           res.Content,
				DispatchArguments: callArgs,
				ObservedAt:        time.Now().UTC(),
			}
		case "github_read":
			var callArgs map[string]any
			if err := json.Unmarshal([]byte(resp.ToolCall.ArgumentsJSON), &callArgs); err != nil {
				t.Fatalf("%s: unmarshal github_read arguments_json for %q: %v", path, resp.ToolCall.ID, err)
			}
			res, err := ensureGitHubExec().Execute(context.Background(), agentic.ToolCall{
				ID:        resp.ToolCall.ID,
				Name:      "github_read",
				Arguments: callArgs,
				LoopID:    "fixture-gather-loop",
			})
			if err != nil || res.Error != "" {
				t.Fatalf("%s: execute mock github_read for %q: err=%v res.Error=%q", path, resp.ToolCall.ID, err, res.Error)
			}
			resolvedCalls[resp.ToolCall.ID] = &emitartifact.ResolvedToolResult{
				Ref:               resp.ToolCall.ID,
				CallID:            resp.ToolCall.ID,
				LoopID:            "fixture-gather-loop",
				ToolName:          "github_read",
				Status:            string(agentic.TrajectoryStatusCompleted),
				Content:           res.Content,
				DispatchArguments: callArgs,
				ObservedAt:        time.Now().UTC(),
			}
		}
	}

	resolver := &fixtureToolResultResolver{results: resolvedCalls}
	emitTurnsSeen := 0
	for i, resp := range allTurns {
		if resp.ToolCall == nil {
			continue
		}
		switch resp.ToolCall.Name {
		case "emit_research_artifact":
			emitTurnsSeen++
			validateResearchArtifactArgs(t, path, i, resp.ToolCall.ArgumentsJSON, declaredCallIDs, resolver)
		case "emit_plan":
			emitTurnsSeen++
			validatePlanArgs(t, path, i, resp.ToolCall.ArgumentsJSON)
		}
	}

	t.Logf("%s: validated %d emit_* turn(s)", filepath.Base(path), emitTurnsSeen)
}

// validateResearchArtifactArgs round-trips the LLM-supplied args
// through the same shape parseArgsIntoArtifact + PopulateCitationDefaults
// + Validate + VerifyDerivation (in cmd/semteams/tools/emitartifact) use
// at runtime.
func validateResearchArtifactArgs(
	t *testing.T,
	path string,
	idx int,
	raw string,
	declaredCallIDs map[string]bool,
	resolver emitartifact.ToolResultResolver,
) {
	t.Helper()

	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: arguments_json invalid JSON: %v", path, idx, err)
		return
	}
	args["loop_id"] = "fixture-loop-id"
	args["produced_at"] = time.Now().UTC()

	bytes, err := json.Marshal(args)
	if err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: re-marshal: %v", path, idx, err)
		return
	}

	var artifact research.Artifact
	if err := json.Unmarshal(bytes, &artifact); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: unmarshal into research.Artifact: %v", path, idx, err)
		return
	}
	artifact.PopulateCitationDefaults()
	if err := artifact.Validate(); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: Validate(): %v", path, idx, err)
		return
	}
	for ai, actor := range artifact.Actors {
		for ei, ev := range actor.Evidence {
			if !declaredCallIDs[ev.ToolResultRef] {
				t.Errorf("%s [response %d emit_research_artifact]: actors[%d].evidence[%d] cites tool_result_ref %q which is not declared on any tool_call.id in the fixture",
					path, idx, ai, ei, ev.ToolResultRef)
			}
		}
	}
	for ii, ip := range artifact.IntegrationPoints {
		for ei, ev := range ip.Evidence {
			if !declaredCallIDs[ev.ToolResultRef] {
				t.Errorf("%s [response %d emit_research_artifact]: integration_points[%d].evidence[%d] cites tool_result_ref %q which is not declared on any tool_call.id in the fixture",
					path, idx, ii, ei, ev.ToolResultRef)
			}
		}
	}

	if err := emitartifact.VerifyDerivation(context.Background(), resolver, agentic.ToolCall{
		ID:     "fixture-emit-call",
		Name:   emitartifact.ToolName,
		LoopID: "fixture-loop-id",
	}, &artifact); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: VerifyDerivation against fixture tool output failed: %v", path, idx, err)
	}
}

// validatePlanArgs round-trips the LLM-supplied args through the same
// shape parseArgsIntoPlan uses. Mirrors validateResearchArtifactArgs.
func validatePlanArgs(t *testing.T, path string, idx int, raw string) {
	t.Helper()

	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Errorf("%s [response %d emit_plan]: arguments_json invalid JSON: %v", path, idx, err)
		return
	}
	args["loop_id"] = "fixture-loop-id"
	args["produced_at"] = time.Now().UTC()

	bytes, err := json.Marshal(args)
	if err != nil {
		t.Errorf("%s [response %d emit_plan]: re-marshal: %v", path, idx, err)
		return
	}

	var plan devviaspec.Plan
	if err := json.Unmarshal(bytes, &plan); err != nil {
		t.Errorf("%s [response %d emit_plan]: unmarshal into devviaspec.Plan: %v", path, idx, err)
		return
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("%s [response %d emit_plan]: Validate(): %v", path, idx, err)
	}
}
